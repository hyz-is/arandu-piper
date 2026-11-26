package piper

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// The values NewHandler applies when HandlerConfig leaves a field at zero.
const (
	// DefaultMaxConcurrent is how many syntheses run at once. Piper is CPU
	// bound, so this is also how many cores a burst of calls can take.
	DefaultMaxConcurrent = 2
	// DefaultQueueWait is how long a request waits for a free slot before it
	// is answered 429, so a caller with another node tries it at once.
	DefaultQueueWait = 2 * time.Second
	// DefaultMaxBodyBytes is the largest request body: a text of
	// DefaultMaxChars characters, in JSON, with room to spare.
	DefaultMaxBodyBytes int64 = 64 << 10
	// MinTokenLength is the shortest token NewHandler accepts.
	MinTokenLength = 32
)

// Error codes of the wire contract. The body of every non-200 answer is
// {"error": code, "message": text}; Remote maps each code back to the error the
// local client would have returned.
const (
	codeEmptyText   = "empty_text"
	codeTextTooLong = "text_too_long"
	codeBadRequest  = "bad_request"
	codeUnknownVoc  = "unknown_voice"
	codeTimeout     = "timeout"
	codeBusy        = "busy"
	codeFailed      = "failed"
	codeBadOutput   = "bad_output"
	codeConvert     = "convert_failed"
	codeUnauth      = "unauthorized"
	codeMethod      = "method_not_allowed"
	codeTooLarge    = "too_large"
	codeCanceled    = "canceled"
)

// DurationHeader carries the length of the speech, in milliseconds, on a
// successful answer.
const DurationHeader = "X-Audio-Duration-Ms"

type wireRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice,omitempty"`
}

// VoiceLister is what a Speaker implements to say which voices it can speak
// with: *Client does. Handler answers a GET with it, so a Remote knows which
// node serves which voice.
type VoiceLister interface {
	// Voices lists the voices, in a stable order.
	Voices() []Voice
	// DefaultVoice is the voice used when a call names none.
	DefaultVoice() Voice
}

var _ VoiceLister = (*Client)(nil)

// wireVoices is the answer to a GET: the voices the node serves. Both fields
// are left out when the node's Speaker cannot say (a Fake, a Cache), which a
// Remote reads as "unknown", not as "none".
type wireVoices struct {
	Voices  []string `json:"voices,omitempty"`
	Default string   `json:"default,omitempty"`
}

type wireError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

// HandlerConfig is what NewHandler needs.
type HandlerConfig struct {
	// Speaker does the work: a *Client in production, a *Fake in tests.
	// Required.
	Speaker Speaker
	// Tokens are the clients allowed to call, by name: the name is only for
	// the log, the token is what the caller sends as "Authorization: Bearer
	// <token>". At least one is required and each token has at least
	// MinTokenLength characters. A token is never logged.
	Tokens map[string]string
	// MaxConcurrent is how many calls speak at once; the rest wait up to
	// QueueWait. Zero means DefaultMaxConcurrent.
	MaxConcurrent int
	// QueueWait is how long a call waits for a slot before it is answered 429
	// with Retry-After. Zero means DefaultQueueWait.
	QueueWait time.Duration
	// MaxBodyBytes is the largest request body, checked before it is read.
	// Zero means DefaultMaxBodyBytes.
	MaxBodyBytes int64
	// Logger receives one line per call: the client's name, the status, the
	// number of characters, the audio's duration and the time spent. Never the
	// text or the audio. Nil logs nothing.
	Logger *slog.Logger
}

// Handler serves a Speaker over HTTP, for another application's Remote to call.
// It answers POST with {"text": ..., "voice": ...} (the voice is optional) with
// the audio as the body, on whatever path it is mounted, and the audio's length
// in DurationHeader. A GET with a valid token on the same path answers
// {"voices": [...], "default": "..."}: the voices the node serves, when its
// Speaker is a VoiceLister. It checks the token, limits the body and the
// concurrency, and keeps nothing: the Speaker's private directory is the only
// place the speech touches, and it is removed before the call returns.
type Handler struct {
	speaker   Speaker
	tokens    []namedToken
	slots     chan struct{}
	queueWait time.Duration
	maxBody   int64
	log       *slog.Logger
	inFlight  atomic.Int64
}

type namedToken struct {
	name string
	sum  [sha256.Size]byte
}

var _ http.Handler = (*Handler)(nil)

// NewHandler validates cfg and returns the handler.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if cfg.Speaker == nil {
		return nil, errors.New("piper: HandlerConfig.Speaker is required")
	}
	if len(cfg.Tokens) == 0 {
		return nil, errors.New("piper: HandlerConfig.Tokens needs at least one client")
	}
	if cfg.MaxConcurrent < 0 || cfg.QueueWait < 0 || cfg.MaxBodyBytes < 0 {
		return nil, errors.New("piper: HandlerConfig limits must not be negative")
	}
	h := &Handler{speaker: cfg.Speaker, queueWait: cfg.QueueWait, maxBody: cfg.MaxBodyBytes, log: cfg.Logger}
	for name, token := range cfg.Tokens {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("piper: a token has no client name")
		}
		if len(token) < MinTokenLength {
			return nil, fmt.Errorf("piper: the token of client %q is shorter than %d characters", name, MinTokenLength)
		}
		h.tokens = append(h.tokens, namedToken{name: name, sum: sha256.Sum256([]byte(token))})
	}
	n := cfg.MaxConcurrent
	if n == 0 {
		n = DefaultMaxConcurrent
	}
	h.slots = make(chan struct{}, n)
	if h.queueWait == 0 {
		h.queueWait = DefaultQueueWait
	}
	if h.maxBody == 0 {
		h.maxBody = DefaultMaxBodyBytes
	}
	return h, nil
}

// InFlight is how many calls are speaking right now.
func (h *Handler) InFlight() int { return int(h.inFlight.Load()) }

func (h *Handler) authenticate(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(token))
	found := ""
	for _, t := range h.tokens {
		if subtle.ConstantTimeCompare(sum[:], t.sum[:]) == 1 {
			found = t.name
		}
	}
	return found, found != ""
}

// ServeHTTP answers one synthesis.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		// The list of voices is for a caller with a token. Without one the
		// answer is what it always was, so the path says nothing to a stranger.
		if _, ok := h.authenticate(r); ok {
			h.listVoices(w)
			return
		}
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		h.fail(w, "", http.StatusMethodNotAllowed, codeMethod, "use POST", 0, start)
		return
	}
	client, ok := h.authenticate(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		h.fail(w, "", http.StatusUnauthorized, codeUnauth, "a valid bearer token is required", 0, start)
		return
	}
	if r.ContentLength > h.maxBody {
		h.fail(w, client, http.StatusRequestEntityTooLarge, codeTooLarge, "the request is larger than the limit", 0, start)
		return
	}

	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		timer := time.NewTimer(h.queueWait)
		defer timer.Stop()
		select {
		case h.slots <- struct{}{}:
			defer func() { <-h.slots }()
		case <-timer.C:
			w.Header().Set("Retry-After", "1")
			h.fail(w, client, http.StatusTooManyRequests, codeBusy, "all slots are busy", 0, start)
			return
		case <-r.Context().Done():
			return
		}
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			h.fail(w, client, http.StatusRequestEntityTooLarge, codeTooLarge, "the request is larger than the limit", 0, start)
			return
		}
		h.fail(w, client, http.StatusBadRequest, codeBadRequest, "the body could not be read", 0, start)
		return
	}
	var req wireRequest
	if json.Unmarshal(raw, &req) != nil {
		h.fail(w, client, http.StatusBadRequest, codeBadRequest, `the body must be JSON like {"text": "...", "voice": "..."}`, 0, start)
		return
	}
	chars := len([]rune(req.Text))

	h.inFlight.Add(1)
	audio, err := h.speaker.SpeakAs(r.Context(), Voice(req.Voice), req.Text)
	h.inFlight.Add(-1)
	if err != nil {
		status, code := classify(err)
		h.fail(w, client, status, code, publicMessage(err), chars, start)
		return
	}

	mime := audio.MIMEType
	if mime == "" {
		mime = MIMEWAV
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set(DurationHeader, strconv.FormatInt(audio.Duration.Milliseconds(), 10))
	w.Header().Set("Content-Length", strconv.Itoa(len(audio.Data)))
	_, _ = w.Write(audio.Data)
	h.record(client, http.StatusOK, "", chars, audio.Duration, start)
}

func (h *Handler) listVoices(w http.ResponseWriter) {
	var out wireVoices
	if l, ok := h.speaker.(VoiceLister); ok {
		for _, v := range l.Voices() {
			out.Voices = append(out.Voices, string(v))
		}
		out.Default = string(l.DefaultVoice())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, ErrEmptyText):
		return http.StatusBadRequest, codeEmptyText
	case errors.Is(err, ErrUnknownVoice):
		return http.StatusBadRequest, codeUnknownVoc
	case errors.Is(err, ErrTextTooLong):
		return http.StatusRequestEntityTooLarge, codeTextTooLong
	case errors.Is(err, ErrTimeout):
		return http.StatusGatewayTimeout, codeTimeout
	case errors.Is(err, context.Canceled):
		return 499, codeCanceled
	case errors.Is(err, ErrBadOutput):
		return http.StatusBadGateway, codeBadOutput
	case errors.Is(err, ErrConvertFailed):
		return http.StatusBadGateway, codeConvert
	default:
		return http.StatusBadGateway, codeFailed
	}
}

// publicMessage is what the caller is told: the sentinel's own text, which
// never carries what was said, and not a ProcessError's standard error.
func publicMessage(err error) string {
	for _, s := range []error{ErrEmptyText, ErrUnknownVoice, ErrTextTooLong, ErrTimeout, ErrBadOutput, ErrConvertFailed, ErrFailed} {
		if errors.Is(err, s) {
			return s.Error()
		}
	}
	return "piper: synthesis failed"
}

func (h *Handler) fail(w http.ResponseWriter, client string, status int, code, message string, chars int, start time.Time) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(wireError{Code: code, Message: message})
	h.record(client, status, code, chars, 0, start)
}

func (h *Handler) record(client string, status int, code string, chars int, audio time.Duration, start time.Time) {
	if h.log == nil {
		return
	}
	attrs := []any{
		"client", client,
		"status", status,
		"chars", chars,
		"audio_ms", audio.Milliseconds(),
		"took_ms", time.Since(start).Milliseconds(),
	}
	if code != "" {
		attrs = append(attrs, "error", code)
	}
	h.log.Info("piper.speak", attrs...)
}
