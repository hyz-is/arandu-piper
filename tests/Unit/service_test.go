package unit_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

const tokenOne = "0123456789abcdef0123456789abcdef-one"
const tokenTwo = "fedcba9876543210fedcba9876543210-two"

func newHandler(t *testing.T, sp piper.Speaker, mod func(*piper.HandlerConfig)) *piper.Handler {
	t.Helper()
	cfg := piper.HandlerConfig{Speaker: sp, Tokens: map[string]string{"corujao": tokenOne, "other": tokenTwo}}
	if mod != nil {
		mod(&cfg)
	}
	h, err := piper.NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func post(h http.Handler, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/speak", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func opus(data string) piper.Answer {
	return piper.Answer{Audio: piper.Audio{Data: []byte(data), MIMEType: piper.MIMEOpus, Duration: 2500 * time.Millisecond}}
}

func TestHandlerSpeaksWithAValidToken(t *testing.T) {
	fake := piper.NewFake(opus("OggS-audio"))
	h := newHandler(t, fake, nil)
	rec := post(h, tokenTwo, `{"text":"Anotado. Está certo?","voice":"pt_BR-cadu-medium"}`)
	if rec.Code != 200 || rec.Body.String() != "OggS-audio" {
		t.Fatalf("%d %q", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Type") != piper.MIMEOpus || rec.Header().Get(piper.DurationHeader) != "2500" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers %v", rec.Header())
	}
	if c := fake.Calls()[0]; c.Voice != piper.VoiceCadu || c.Text != "Anotado. Está certo?" {
		t.Fatalf("%+v", c)
	}
}

func TestHandlerRefusesWithoutAValidToken(t *testing.T) {
	fake := piper.NewFake(opus("x"))
	h := newHandler(t, fake, nil)
	for _, token := range []string{"", "wrong", tokenOne + "x", tokenOne[:len(tokenOne)-1]} {
		if rec := post(h, token, `{"text":"oi"}`); rec.Code != 401 {
			t.Fatalf("token %q: %d", token, rec.Code)
		}
	}
	if len(fake.Calls()) != 0 {
		t.Fatal("an unauthenticated call reached the speaker")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/speak", nil))
	if rec.Code != 405 {
		t.Fatalf("GET: %d", rec.Code)
	}
}

func TestHandlerMapsEveryErrorAndNeverEchoesTheCause(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{piper.ErrEmptyText, 400, "empty_text"},
		{piper.ErrUnknownVoice, 400, "unknown_voice"},
		{piper.ErrTextTooLong, 413, "text_too_long"},
		{piper.ErrTimeout, 504, "timeout"},
		{piper.ErrBadOutput, 502, "bad_output"},
		{piper.ErrConvertFailed, 502, "convert_failed"},
		{fmt.Errorf("%w: segredo do paciente", piper.ErrFailed), 502, "failed"},
		{errors.New("segredo do paciente"), 502, "failed"},
	}
	for _, c := range cases {
		h := newHandler(t, piper.NewFake(piper.Answer{Err: c.err}), nil)
		rec := post(h, tokenOne, `{"text":"oi"}`)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), `"error":"`+c.code+`"`) {
			t.Fatalf("%v: %d %s", c.err, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "segredo") {
			t.Fatalf("the cause leaked: %s", rec.Body)
		}
	}
}

func TestHandlerRejectsBadBodiesBeforeSpeaking(t *testing.T) {
	fake := piper.NewFake(opus("x"))
	h := newHandler(t, fake, func(c *piper.HandlerConfig) { c.MaxBodyBytes = 64 })
	if rec := post(h, tokenOne, `not json`); rec.Code != 400 || !strings.Contains(rec.Body.String(), "bad_request") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := post(h, tokenOne, `{"text":"`+strings.Repeat("a", 100)+`"}`); rec.Code != 413 {
		t.Fatalf("declared: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader(`{"text":"`+strings.Repeat("a", 100)+`"}`)))
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer "+tokenOne)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 413 {
		t.Fatalf("undeclared: %d", rec.Code)
	}
	if len(fake.Calls()) != 0 {
		t.Fatal("a bad body reached the speaker")
	}
}

type slow struct {
	release chan struct{}
	started atomic.Int64
}

func (s *slow) Speak(ctx context.Context, text string) (piper.Audio, error) {
	return s.SpeakAs(ctx, "", text)
}

func (s *slow) SpeakAs(ctx context.Context, _ piper.Voice, _ string) (piper.Audio, error) {
	s.started.Add(1)
	select {
	case <-s.release:
		return piper.Audio{Data: []byte("ok"), MIMEType: piper.MIMEWAV}, nil
	case <-ctx.Done():
		return piper.Audio{}, ctx.Err()
	}
}

func TestHandlerAnswersBusyWhenEverySlotIsTaken(t *testing.T) {
	sp := &slow{release: make(chan struct{})}
	h := newHandler(t, sp, func(c *piper.HandlerConfig) { c.MaxConcurrent = 1; c.QueueWait = 30 * time.Millisecond })
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); post(h, tokenOne, `{"text":"oi"}`) }()
	for sp.started.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	rec := post(h, tokenOne, `{"text":"oi"}`)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	close(sp.release)
	wg.Wait()
	if h.InFlight() != 0 {
		t.Fatalf("InFlight %d", h.InFlight())
	}
}

func TestHandlerLogsTheClientAndNeverTheText(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	h := newHandler(t, piper.NewFake(opus("x")), func(c *piper.HandlerConfig) { c.Logger = log })
	post(h, tokenOne, `{"text":"dado sigiloso"}`)
	out := logs.String()
	if !strings.Contains(out, "client=corujao") || !strings.Contains(out, "status=200") || !strings.Contains(out, "chars=13") {
		t.Fatalf("log: %s", out)
	}
	for _, leak := range []string{"sigiloso", tokenOne} {
		if strings.Contains(out, leak) {
			t.Fatalf("the log carries %q: %s", leak, out)
		}
	}
}

func TestNewHandlerValidates(t *testing.T) {
	fake := piper.NewFake()
	for name, cfg := range map[string]piper.HandlerConfig{
		"no speaker":  {Tokens: map[string]string{"a": tokenOne}},
		"no tokens":   {Speaker: fake},
		"short token": {Speaker: fake, Tokens: map[string]string{"a": "short"}},
		"no name":     {Speaker: fake, Tokens: map[string]string{" ": tokenOne}},
		"negative":    {Speaker: fake, Tokens: map[string]string{"a": tokenOne}, MaxBodyBytes: -1},
	} {
		if _, err := piper.NewHandler(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

type node struct {
	srv   *httptest.Server
	calls atomic.Int64
	mu    sync.Mutex
	mode  string
}

func newNode(t *testing.T, name string) *node {
	t.Helper()
	n := &node{mode: "ok"}
	mux := http.NewServeMux()
	mux.HandleFunc("/speak", func(w http.ResponseWriter, r *http.Request) {
		n.calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		if r.Header.Get("Authorization") != "Bearer "+tokenOne {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}
		n.mu.Lock()
		mode := n.mode
		n.mu.Unlock()
		switch mode {
		case "fail":
			w.WriteHeader(502)
			fmt.Fprint(w, `{"error":"failed"}`)
		case "busy":
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":"busy"}`)
		case "voice":
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"unknown_voice"}`)
		case "hang":
			<-r.Context().Done()
		default:
			w.Header().Set("Content-Type", piper.MIMEOpus)
			w.Header().Set(piper.DurationHeader, "1200")
			fmt.Fprintf(w, "audio-of-%s", name)
		}
	})
	mux.HandleFunc("/_arandu/health", func(http.ResponseWriter, *http.Request) {})
	n.srv = httptest.NewServer(mux)
	t.Cleanup(n.srv.Close)
	return n
}

func (n *node) set(mode string) { n.mu.Lock(); n.mode = mode; n.mu.Unlock() }

func remote(t *testing.T, mod func(*piper.RemoteConfig), nodes ...*node) *piper.Remote {
	t.Helper()
	cfg := piper.RemoteConfig{Token: tokenOne, Cooldown: time.Hour}
	for i, n := range nodes {
		cfg.Nodes = append(cfg.Nodes, piper.Node{Name: fmt.Sprintf("n%d", i+1), URL: n.srv.URL})
	}
	if mod != nil {
		mod(&cfg)
	}
	r, err := piper.NewRemote(cfg)
	if err != nil {
		t.Fatalf("NewRemote: %v", err)
	}
	return r
}

func TestRemoteSpreadsCallsOverTheNodesAndReturnsTheAudio(t *testing.T) {
	a, b, c := newNode(t, "a"), newNode(t, "b"), newNode(t, "c")
	r := remote(t, nil, a, b, c)
	for range 9 {
		audio, err := r.Speak(context.Background(), "Anotado.")
		if err != nil || !strings.HasPrefix(string(audio.Data), "audio-of-") || audio.MIMEType != piper.MIMEOpus || audio.Duration != 1200*time.Millisecond {
			t.Fatalf("%+v %v", audio, err)
		}
	}
	for _, n := range []*node{a, b, c} {
		if got := n.calls.Load(); got != 3 {
			t.Fatalf("a node got %d of 9 calls", got)
		}
	}
}

func TestRemoteFailsOverAndTakesAFailingNodeOutOfTheRotation(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("fail")
	r := remote(t, func(c *piper.RemoteConfig) { c.FailureThreshold = 2 }, a, b)
	for range 10 {
		audio, err := r.Speak(context.Background(), "Anotado.")
		if err != nil || string(audio.Data) != "audio-of-b" {
			t.Fatalf("%+v %v", audio, err)
		}
	}
	if got := a.calls.Load(); got != 2 {
		t.Fatalf("the failing node was called %d times", got)
	}
	for _, h := range r.Health(context.Background()) {
		if h.Name == "n1" && (h.InRotation || h.Failures < 2) {
			t.Fatalf("%+v", h)
		}
	}
}

func TestRemoteBringsANodeBackAfterTheCooldown(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("fail")
	r := remote(t, func(c *piper.RemoteConfig) { c.FailureThreshold = 1; c.Cooldown = 40 * time.Millisecond }, a, b)
	for range 4 {
		_, _ = r.Speak(context.Background(), "Anotado.")
	}
	a.set("ok")
	time.Sleep(60 * time.Millisecond)
	seen := false
	for range 4 {
		if au, _ := r.Speak(context.Background(), "Anotado."); string(au.Data) == "audio-of-a" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("the node never came back")
	}
}

func TestRemoteTriesTheDownNodesWhenAllAreDown(t *testing.T) {
	a := newNode(t, "a")
	a.set("fail")
	r := remote(t, func(c *piper.RemoteConfig) { c.FailureThreshold = 1 }, a)
	_, err := r.Speak(context.Background(), "Anotado.")
	if !errors.Is(err, piper.ErrNoNodes) || !errors.Is(err, piper.ErrFailed) {
		t.Fatalf("%v", err)
	}
	a.set("ok")
	if au, err := r.Speak(context.Background(), "Anotado."); err != nil || string(au.Data) != "audio-of-a" {
		t.Fatalf("%+v %v", au, err)
	}
}

func TestRemoteSkipsABusyNodeWithoutTakingItOut(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("busy")
	r := remote(t, func(c *piper.RemoteConfig) { c.FailureThreshold = 1 }, a, b)
	for range 6 {
		if au, err := r.Speak(context.Background(), "Anotado."); err != nil || string(au.Data) != "audio-of-b" {
			t.Fatalf("%+v %v", au, err)
		}
	}
	a.set("ok")
	seen := false
	for range 4 {
		if au, _ := r.Speak(context.Background(), "Anotado."); string(au.Data) == "audio-of-a" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("a busy node was taken out of the rotation")
	}
}

func TestRemoteDoesNotRetryARefusalAndChecksTheTextBeforeTheNetwork(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("voice")
	b.set("voice")
	r := remote(t, func(c *piper.RemoteConfig) { c.MaxChars = 10 }, a, b)
	if _, err := r.SpeakAs(context.Background(), "nope", "Anotado."); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("%v", err)
	}
	if a.calls.Load()+b.calls.Load() != 1 {
		t.Fatal("a refusal was retried")
	}
	if _, err := r.Speak(context.Background(), " ... "); !errors.Is(err, piper.ErrEmptyText) {
		t.Fatalf("%v", err)
	}
	if _, err := r.Speak(context.Background(), "uma frase bem longa demais"); !errors.Is(err, piper.ErrTextTooLong) {
		t.Fatalf("%v", err)
	}
	if a.calls.Load()+b.calls.Load() != 1 {
		t.Fatal("the network was used for a text that is refused")
	}
}

func TestRemoteTimesOutAHangingNodeAndTriesTheNext(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("hang")
	r := remote(t, func(c *piper.RemoteConfig) { c.Timeout = 80 * time.Millisecond }, a, b)
	if au, err := r.Speak(context.Background(), "Anotado."); err != nil || string(au.Data) != "audio-of-b" {
		t.Fatalf("%+v %v", au, err)
	}
}

func TestRemoteAWrongTokenIsNotRetriedAndNeverShown(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	r := remote(t, func(c *piper.RemoteConfig) { c.Token = tokenTwo }, a, b)
	_, err := r.Speak(context.Background(), "Anotado.")
	if err == nil || strings.Contains(err.Error(), tokenTwo) || a.calls.Load()+b.calls.Load() != 1 {
		t.Fatalf("%v", err)
	}
}

func TestRemoteHonoursACancelledContext(t *testing.T) {
	a := newNode(t, "a")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := remote(t, nil, a).Speak(ctx, "Anotado."); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestRemoteHealthReportsEveryNode(t *testing.T) {
	a, dead := newNode(t, "a"), newNode(t, "b")
	dead.srv.Close()
	h := remote(t, nil, a, dead).Health(context.Background())
	if len(h) != 2 || !h[0].OK || h[1].OK || h[1].Error == "" {
		t.Fatalf("%+v", h)
	}
}

func TestRemoteKeyServesACacheWithoutTheNetwork(t *testing.T) {
	a := newNode(t, "a")
	r := remote(t, nil, a)
	k1, err := r.Key(piper.VoiceCadu, "Anotado.  Está certo?")
	k2, _ := r.Key(piper.VoiceCadu, " Anotado.\nEstá certo? ")
	k3, _ := r.Key(piper.VoiceFaber, "Anotado. Está certo?")
	k4, _ := remote(t, func(c *piper.RemoteConfig) { c.KeyTag = "v2" }, a).Key(piper.VoiceCadu, "Anotado. Está certo?")
	if err != nil || len(k1) != 64 || k1 != k2 || k1 == k3 || k1 == k4 {
		t.Fatalf("%q %q %q %q %v", k1, k2, k3, k4, err)
	}
	cache := piper.NewCache(r, piper.NewMemoryStore())
	for range 3 {
		if res, err := cache.Say(context.Background(), piper.VoiceCadu, "Anotado. Está certo?"); err != nil || string(res.Audio.Data) != "audio-of-a" {
			t.Fatalf("%+v %v", res, err)
		}
	}
	if a.calls.Load() != 1 {
		t.Fatalf("the node was called %d times for one audio", a.calls.Load())
	}
}

func TestNewRemoteValidatesAndParseNodes(t *testing.T) {
	good := []piper.Node{{Name: "a", URL: "https://a.example"}}
	for name, cfg := range map[string]piper.RemoteConfig{
		"no nodes":    {Token: tokenOne},
		"short token": {Nodes: good, Token: "x"},
		"bad url":     {Nodes: []piper.Node{{Name: "a", URL: "ftp://a"}}, Token: tokenOne},
		"credentials": {Nodes: []piper.Node{{Name: "a", URL: "https://u:p@a.example"}}, Token: tokenOne},
		"repeated":    {Nodes: append(good, good...), Token: tokenOne},
		"negative":    {Nodes: good, Token: tokenOne, Cooldown: -1},
	} {
		if _, err := piper.NewRemote(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	got, err := piper.ParseNodes("one=https://one.example, two=https://two.example:8443/")
	if err != nil || len(got) != 2 || got[0].Name != "one" || got[1].URL != "https://two.example:8443/" {
		t.Fatalf("%+v %v", got, err)
	}
	if got, _ := piper.ParseNodes("https://a.example,https://b.example"); got[0].Name != "node1" || got[1].URL != "https://b.example" {
		t.Fatalf("%+v", got)
	}
	if _, err := piper.ParseNodes(" , "); err == nil {
		t.Fatal("an empty list was accepted")
	}
}

func TestRemoteAgainstARealHandler(t *testing.T) {
	fake := piper.NewFake(opus("OggS"), piper.Answer{Err: piper.ErrUnknownVoice})
	mux := http.NewServeMux()
	mux.Handle("/speak", newHandler(t, fake, nil))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	r, err := piper.NewRemote(piper.RemoteConfig{Nodes: []piper.Node{{Name: "one", URL: srv.URL}}, Token: tokenOne})
	if err != nil {
		t.Fatal(err)
	}
	au, err := r.SpeakAs(context.Background(), piper.VoiceCadu, "Anotado.")
	if err != nil || string(au.Data) != "OggS" || au.MIMEType != piper.MIMEOpus || au.Duration != 2500*time.Millisecond {
		t.Fatalf("%+v %v", au, err)
	}
	if _, err := r.SpeakAs(context.Background(), "x", "Anotado."); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("%v", err)
	}
}

func TestRemoteWaitsWhenEveryNodeIsBusyAndGoesThroughWhenOneFrees(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("busy")
	b.set("busy")
	r := remote(t, func(c *piper.RemoteConfig) { c.BusyWait = 3 * time.Second }, a, b)
	go func() { time.Sleep(300 * time.Millisecond); b.set("ok") }()
	au, err := r.Speak(context.Background(), "Anotado.")
	if err != nil || string(au.Data) != "audio-of-b" {
		t.Fatalf("%+v %v", au, err)
	}
	if a.calls.Load()+b.calls.Load() < 3 {
		t.Fatal("the round was not repeated")
	}
}

func TestRemoteGivesUpAfterBusyWaitAndCanBeToldNotToWait(t *testing.T) {
	a := newNode(t, "a")
	a.set("busy")
	start := time.Now()
	_, err := remote(t, func(c *piper.RemoteConfig) { c.BusyWait = 600 * time.Millisecond }, a).Speak(context.Background(), "Anotado.")
	if !errors.Is(err, piper.ErrNoNodes) || time.Since(start) < 400*time.Millisecond || time.Since(start) > 3*time.Second {
		t.Fatalf("%v after %s", err, time.Since(start))
	}
	before := a.calls.Load()
	start = time.Now()
	_, err = remote(t, func(c *piper.RemoteConfig) { c.BusyWait = -1 }, a).Speak(context.Background(), "Anotado.")
	if !errors.Is(err, piper.ErrNoNodes) || a.calls.Load()-before != 1 || time.Since(start) > 300*time.Millisecond {
		t.Fatalf("%v, %d calls, after %s", err, a.calls.Load()-before, time.Since(start))
	}
}

func TestRemoteDoesNotWaitForANodeThatFails(t *testing.T) {
	a, b := newNode(t, "a"), newNode(t, "b")
	a.set("busy")
	b.set("fail")
	start := time.Now()
	_, err := remote(t, func(c *piper.RemoteConfig) { c.BusyWait = 3 * time.Second }, a, b).Speak(context.Background(), "Anotado.")
	if !errors.Is(err, piper.ErrNoNodes) || time.Since(start) > time.Second {
		t.Fatalf("%v after %s", err, time.Since(start))
	}
}

func TestRemoteTheWaitStopsWithTheCallersContext(t *testing.T) {
	a := newNode(t, "a")
	a.set("busy")
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := remote(t, func(c *piper.RemoteConfig) { c.BusyWait = time.Minute }, a).Speak(ctx, "Anotado.")
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("%v after %s", err, time.Since(start))
	}
}
