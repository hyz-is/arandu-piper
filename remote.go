package piper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The values NewRemote applies when RemoteConfig leaves a field at zero.
const (
	// DefaultRemoteTimeout bounds one attempt on one node.
	DefaultRemoteTimeout = 60 * time.Second
	// DefaultFailureThreshold is how many failures in a row take a node out of
	// the rotation.
	DefaultFailureThreshold = 3
	// DefaultCooldown is how long a node stays out of the rotation before it is
	// tried again.
	DefaultCooldown = 30 * time.Second
	// DefaultRemotePath is where Handler is mounted on the node.
	DefaultRemotePath = "/speak"
	// DefaultHealthPath is the health endpoint Remote.Health reads.
	DefaultHealthPath = "/_arandu/health"
	// DefaultBusyWait is how long a call keeps trying when every node answered
	// busy.
	DefaultBusyWait = 5 * time.Second
	// DefaultVoiceTTL is how long Remote remembers that a node does not serve a
	// voice, or which voices a node listed, before it asks again.
	DefaultVoiceTTL = 5 * time.Minute
	// busyBackoff is the first pause between two rounds over busy nodes; it
	// doubles up to maxBusyBackoff.
	busyBackoff    = 200 * time.Millisecond
	maxBusyBackoff = time.Second
)

// errNodeBusy marks a node's 429: it is working at its limit, not unwell.
var errNodeBusy = errors.New("busy")

// errNodeNoVoice marks a node's refusal of a voice: it is well, it does not
// have that voice, and another node may.
var errNodeNoVoice = errors.New("does not serve the voice")

// maxRemoteAudio bounds the audio read back from a node.
const maxRemoteAudio = maxOutputBytes

// ErrNoNodes means every node was tried and none answered.
var ErrNoNodes = errors.New("piper: no node answered")

// Node is one server that runs Handler.
type Node struct {
	// Name identifies the node in health reports and logs. It must not change.
	Name string
	// URL is the node's base address, such as "https://one.api.corujao.ai".
	URL string
}

// ParseNodes reads the nodes from configuration: name=url pairs separated by
// commas ("one=https://one.example,two=https://two.example"), or bare URLs,
// which are named node1, node2 and so on, or a single URL.
func ParseNodes(s string) ([]Node, error) {
	var nodes []Node
	for i, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, raw := fmt.Sprintf("node%d", i+1), item
		if before, after, ok := strings.Cut(item, "="); ok && !strings.Contains(before, "/") && !strings.Contains(before, ":") {
			name, raw = strings.TrimSpace(before), strings.TrimSpace(after)
		}
		nodes = append(nodes, Node{Name: name, URL: raw})
	}
	if len(nodes) == 0 {
		return nil, errors.New("piper: no node in the list")
	}
	return nodes, nil
}

// RemoteConfig is what NewRemote needs.
type RemoteConfig struct {
	// Nodes are the servers, each running Handler. At least one; names are
	// unique and URLs are http or https. Calls are spread over them in turn,
	// and a node that fails is taken out of the rotation for a while.
	Nodes []Node
	// Token is the bearer token this client was given. Required. It is sent
	// only to the nodes, and never logged or put in an error.
	Token string
	// Path is where Handler is mounted. Empty means DefaultRemotePath.
	Path string
	// HealthPath is the node's health endpoint. Empty means DefaultHealthPath.
	HealthPath string
	// Timeout bounds one attempt on one node. Zero means DefaultRemoteTimeout.
	Timeout time.Duration
	// FailureThreshold is how many failures in a row take a node out of the
	// rotation. Zero means DefaultFailureThreshold.
	FailureThreshold int
	// Cooldown is how long a node stays out. Zero means DefaultCooldown.
	Cooldown time.Duration
	// MaxChars is the longest text sent, refused here before any network. Zero
	// means DefaultMaxChars.
	MaxChars int
	// BusyWait is how long a call keeps going round the nodes when no node could take it and at
	// least one answered 429 (busy, not failing): each round waits a little longer,
	// up to BusyWait in all, and the call fails with ErrNoNodes only after it. A
	// burst of more calls than the nodes can take at once then waits its turn
	// instead of failing. Zero means DefaultBusyWait; a negative value turns the
	// wait off and a call fails as soon as one round found every node busy.
	BusyWait time.Duration
	// VoiceTTL is how long a node's refusal of a voice, or the list of voices
	// it gave to Voices, is trusted before the node is asked again: nodes may
	// serve different voices, and one that is upgraded to serve a new voice is
	// found again after this time. Zero means DefaultVoiceTTL.
	VoiceTTL time.Duration
	// KeyTag is part of every Key. The audio depends on settings only the
	// nodes know (the voice files, the pace, the format), so change the tag
	// when they change and a Cache stops answering with the old audio. Empty
	// is a valid tag.
	KeyTag string
	// HTTPClient carries the calls. Nil means a client with a pooled transport
	// and no timeout of its own (Timeout ends each attempt).
	HTTPClient *http.Client
}

// Remote is a Speaker that calls nodes running Handler. It is what an
// application passes where it passed a *Client: the same interface, and Keyed,
// so a Cache works over it. It is safe for concurrent use.
type Remote struct {
	nodes      []*remoteNode
	token      string
	path       string
	healthPath string
	timeout    time.Duration
	threshold  int
	cooldown   time.Duration
	maxChars   int
	keyTag     string
	busyWait   time.Duration
	voiceTTL   time.Duration
	http       *http.Client
	next       atomic.Uint64
	now        func() time.Time
}

type remoteNode struct {
	Node

	mu        sync.Mutex
	failures  int
	downUntil time.Time
	lastErr   string
	// catalog is what the node listed to Voices, nil while it is not known, and
	// trusted until catalogUntil. refused holds the voices the node answered
	// unknown_voice to, until when.
	catalog      map[Voice]bool
	catalogUntil time.Time
	refused      map[Voice]time.Time
}

var (
	_ Speaker = (*Remote)(nil)
	_ Keyed   = (*Remote)(nil)
)

// NewRemote validates cfg and returns the client. Nothing is called here.
func NewRemote(cfg RemoteConfig) (*Remote, error) {
	if len(cfg.Nodes) == 0 {
		return nil, errors.New("piper: RemoteConfig.Nodes needs at least one node")
	}
	if len(cfg.Token) < MinTokenLength {
		return nil, fmt.Errorf("piper: RemoteConfig.Token is shorter than %d characters", MinTokenLength)
	}
	if cfg.Timeout < 0 || cfg.FailureThreshold < 0 || cfg.Cooldown < 0 || cfg.MaxChars < 0 || cfg.VoiceTTL < 0 {
		return nil, errors.New("piper: RemoteConfig limits must not be negative")
	}
	r := &Remote{
		token:      cfg.Token,
		path:       cfg.Path,
		healthPath: cfg.HealthPath,
		timeout:    cfg.Timeout,
		threshold:  cfg.FailureThreshold,
		cooldown:   cfg.Cooldown,
		maxChars:   cfg.MaxChars,
		keyTag:     cfg.KeyTag,
		busyWait:   cfg.BusyWait,
		voiceTTL:   cfg.VoiceTTL,
		http:       cfg.HTTPClient,
		now:        time.Now,
	}
	seen := map[string]bool{}
	for _, n := range cfg.Nodes {
		name := strings.TrimSpace(n.Name)
		if name == "" || seen[name] {
			return nil, fmt.Errorf("piper: node name %q is empty or repeated", n.Name)
		}
		seen[name] = true
		base, err := resolveBaseURL(n.URL)
		if err != nil {
			return nil, fmt.Errorf("piper: node %q URL: %w", name, err)
		}
		r.nodes = append(r.nodes, &remoteNode{Node: Node{Name: name, URL: base}})
	}
	if r.path == "" {
		r.path = DefaultRemotePath
	}
	if r.healthPath == "" {
		r.healthPath = DefaultHealthPath
	}
	if r.timeout == 0 {
		r.timeout = DefaultRemoteTimeout
	}
	if r.threshold == 0 {
		r.threshold = DefaultFailureThreshold
	}
	if r.cooldown == 0 {
		r.cooldown = DefaultCooldown
	}
	if r.maxChars == 0 {
		r.maxChars = DefaultMaxChars
	}
	if r.busyWait == 0 {
		r.busyWait = DefaultBusyWait
	}
	if r.voiceTTL == 0 {
		r.voiceTTL = DefaultVoiceTTL
	}
	if r.http == nil {
		r.http = &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
			ForceAttemptHTTP2:   true,
		}}
	}
	return r, nil
}

// resolveBaseURL checks the address of a node: an http or https URL with a
// host, and nothing that could carry a credential or a query.
func resolveBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("is not a URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("must be an http or https URL with a host")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("must not carry credentials, a query or a fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// Speak is SpeakAs with the nodes' default voice.
func (r *Remote) Speak(ctx context.Context, text string) (Audio, error) {
	return r.SpeakAs(ctx, "", text)
}

// SpeakAs sends the text to the next healthy node. A node that is busy,
// unreachable, timed out or failing is skipped for this call and, after
// FailureThreshold failures in a row, for Cooldown; the next node is tried at
// once, and when every node was busy the round is repeated for up to BusyWait.
// A refusal another node would repeat (empty or too long text) is returned as
// the same sentinel error the local client gives. Nodes may serve different
// voices: a node that does not have the voice is skipped (and remembered for
// VoiceTTL, or known from Voices), and when no node has it the error is
// ErrUnknownVoice, naming the nodes. When every node failed the error wraps
// ErrNoNodes and the last node's error.
func (r *Remote) SpeakAs(ctx context.Context, voice Voice, text string) (Audio, error) {
	spoken := normalize(text)
	if !speakable(spoken) {
		return Audio{}, ErrEmptyText
	}
	if n := len([]rune(spoken)); n > r.maxChars {
		return Audio{}, fmt.Errorf("%w: %d characters, the limit is %d", ErrTextTooLong, n, r.maxChars)
	}
	body, err := json.Marshal(wireRequest{Text: text, Voice: string(voice)})
	if err != nil {
		return Audio{}, err
	}
	deadline := time.Now().Add(r.busyWait)
	pause := busyBackoff
	for {
		var last error
		anyBusy := false
		for _, n := range r.order() {
			if err := ctx.Err(); err != nil {
				return Audio{}, err
			}
			if voice != "" && !n.mayServe(voice, r.now()) {
				continue
			}
			audio, retry, err := r.call(ctx, n, body)
			if err == nil {
				n.succeed(voice)
				return audio, nil
			}
			if !retry {
				return Audio{}, err
			}
			if errors.Is(err, errNodeNoVoice) {
				// Not a failure: the node is well and another may have the voice.
				n.refuse(voice, r.now().Add(r.voiceTTL))
				continue
			}
			if errors.Is(err, errNodeBusy) {
				anyBusy = true
			}
			last = err
		}
		if last == nil {
			// No node failed and none was busy: each one does not have the voice.
			return Audio{}, r.noNodeServes(voice)
		}
		// A round in which some node was busy is worth another, whatever the
		// others did: a busy node frees, while a node that failed was already
		// skipped for the next call and waiting does not mend it. A round of
		// failures alone is not waited for.
		remaining := time.Until(deadline)
		if !anyBusy || r.busyWait < 0 || remaining <= 0 {
			return Audio{}, fmt.Errorf("%w: %w", ErrNoNodes, last)
		}
		select {
		case <-time.After(min(pause, remaining)):
		case <-ctx.Done():
			return Audio{}, ctx.Err()
		}
		pause = min(pause*2, maxBusyBackoff)
	}
}

// Key names the audio SpeakAs would make of text with voice, without making
// it: a hash of the voice, the cleaned text and KeyTag. It refuses what SpeakAs
// refuses before the network, with the same errors.
func (r *Remote) Key(voice Voice, text string) (string, error) {
	spoken := normalize(text)
	if !speakable(spoken) {
		return "", ErrEmptyText
	}
	if n := len([]rune(spoken)); n > r.maxChars {
		return "", fmt.Errorf("%w: %d characters, the limit is %d", ErrTextTooLong, n, r.maxChars)
	}
	h := sha256.New()
	fmt.Fprintf(h, "arandu-piper/remote/1\x00%s\x00%s\x00", voice, r.keyTag)
	_, _ = h.Write([]byte(spoken))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (r *Remote) order() []*remoteNode {
	start := int(r.next.Add(1)-1) % len(r.nodes)
	var up, down []*remoteNode
	now := r.now()
	for i := range r.nodes {
		n := r.nodes[(start+i)%len(r.nodes)]
		if n.isDown(now) {
			down = append(down, n)
		} else {
			up = append(up, n)
		}
	}
	return append(up, down...)
}

func (n *remoteNode) isDown(now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return now.Before(n.downUntil)
}

func (n *remoteNode) succeed(voice Voice) {
	n.mu.Lock()
	n.failures, n.downUntil, n.lastErr = 0, time.Time{}, ""
	delete(n.refused, voice)
	n.mu.Unlock()
}

// mayServe is false when the node is known not to have the voice: it refused it
// not long ago, or it listed its voices (not long ago) and the voice is not
// among them. A node that said nothing is assumed to have it, and is asked.
func (n *remoteNode) mayServe(voice Voice, now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if until, ok := n.refused[voice]; ok && now.Before(until) {
		return false
	}
	if n.catalog != nil && now.Before(n.catalogUntil) && !n.catalog[voice] {
		return false
	}
	return true
}

func (n *remoteNode) refuse(voice Voice, until time.Time) {
	n.mu.Lock()
	if n.refused == nil {
		n.refused = map[Voice]time.Time{}
	}
	n.refused[voice] = until
	n.mu.Unlock()
}

func (r *Remote) fail(n *remoteNode, why string) {
	n.mu.Lock()
	n.failures++
	n.lastErr = why
	if n.failures >= r.threshold {
		n.downUntil = r.now().Add(r.cooldown)
	}
	n.mu.Unlock()
}

// call makes one attempt on one node. retry says whether another node could do
// better.
func (r *Remote) call(ctx context.Context, n *remoteNode, body []byte) (Audio, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL+r.path, bytes.NewReader(body))
	if err != nil {
		return Audio{}, false, fmt.Errorf("piper: building the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Audio{}, false, ctx.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			r.fail(n, "timeout")
			return Audio{}, true, fmt.Errorf("piper: node %s: %w", n.Name, ErrTimeout)
		}
		r.fail(n, "unreachable")
		return Audio{}, true, fmt.Errorf("piper: node %s is unreachable", n.Name)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteAudio+1))
		if err != nil || len(data) == 0 || len(data) > maxRemoteAudio {
			r.fail(n, "unreadable answer")
			return Audio{}, true, fmt.Errorf("piper: node %s: %w", n.Name, ErrBadOutput)
		}
		ms, _ := strconv.ParseInt(resp.Header.Get(DurationHeader), 10, 64)
		return Audio{Data: data, MIMEType: resp.Header.Get("Content-Type"), Duration: time.Duration(ms) * time.Millisecond}, false, nil
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var we wireError
	_ = json.Unmarshal(raw, &we)
	switch we.Code {
	case codeEmptyText:
		return Audio{}, false, ErrEmptyText
	case codeTextTooLong, codeTooLarge:
		return Audio{}, false, fmt.Errorf("%w: refused by node %s", ErrTextTooLong, n.Name)
	case codeUnknownVoc:
		// Not final: another node may have the voice. SpeakAs says so when none does.
		return Audio{}, true, fmt.Errorf("piper: node %s %w", n.Name, errNodeNoVoice)
	case codeBadRequest:
		return Audio{}, false, fmt.Errorf("piper: node %s refused the request", n.Name)
	case codeUnauth:
		// A wrong token is wrong on every node, and retrying cannot fix it.
		return Audio{}, false, fmt.Errorf("piper: node %s refused the token", n.Name)
	case codeBusy:
		// Busy is not sick: skip the node for this call and leave it in the
		// rotation.
		return Audio{}, true, fmt.Errorf("piper: node %s is %w", n.Name, errNodeBusy)
	case codeCanceled:
		return Audio{}, false, context.Canceled
	case codeTimeout:
		r.fail(n, "timeout")
		return Audio{}, true, fmt.Errorf("piper: node %s: %w", n.Name, ErrTimeout)
	case codeConvert:
		r.fail(n, "convert failed")
		return Audio{}, true, fmt.Errorf("piper: node %s: %w", n.Name, ErrConvertFailed)
	}
	r.fail(n, fmt.Sprintf("status %d", resp.StatusCode))
	return Audio{}, true, fmt.Errorf("piper: node %s: %w: status %d", n.Name, ErrFailed, resp.StatusCode)
}

// NodeHealth is what Remote.Health reports for one node.
type NodeHealth struct {
	// Name and URL are the node's, as configured.
	Name, URL string
	// OK is true when the node's health endpoint answered 200 just now.
	OK bool
	// InRotation is false while the node is out for failing calls, whatever
	// the health endpoint says.
	InRotation bool
	// Latency is how long the health call took.
	Latency time.Duration
	// Failures are the failures in a row of real calls.
	Failures int
	// Error is a short reason when OK is false, or the last call's failure.
	Error string
}

// Health asks every node's health endpoint, in parallel, and reports each one
// with its place in the rotation. A node that answers 200 here and has been
// failing real calls stays out until its cooldown ends: the endpoint says the
// process is up, not that it speaks.
func (r *Remote) Health(ctx context.Context) []NodeHealth {
	out := make([]NodeHealth, len(r.nodes))
	var wg sync.WaitGroup
	for i, n := range r.nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := NodeHealth{Name: n.Name, URL: n.URL}
			n.mu.Lock()
			h.Failures, h.Error = n.failures, n.lastErr
			n.mu.Unlock()
			h.InRotation = !n.isDown(r.now())

			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			started := time.Now()
			req, err := http.NewRequestWithContext(cctx, http.MethodGet, n.URL+r.healthPath, nil)
			if err == nil {
				var resp *http.Response
				if resp, err = r.http.Do(req); err == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
					resp.Body.Close()
					h.OK = resp.StatusCode == http.StatusOK
					if !h.OK {
						h.Error = fmt.Sprintf("health status %d", resp.StatusCode)
					}
				} else {
					h.Error = "health unreachable"
				}
			}
			h.Latency = time.Since(started)
			out[i] = h
		}()
	}
	wg.Wait()
	return out
}

// noNodeServes is the answer when every node refused the voice, or is known not
// to have it: ErrUnknownVoice, with what each node said, so the fix (install the
// engine on a node, or name another voice) is plain.
func (r *Remote) noNodeServes(voice Voice) error {
	now := r.now()
	var parts []string
	for _, n := range r.nodes {
		n.mu.Lock()
		switch {
		case n.catalog != nil && now.Before(n.catalogUntil):
			var have []string
			for _, v := range sortedVoices(n.catalog) {
				have = append(have, string(v))
			}
			parts = append(parts, fmt.Sprintf("%s serves %s", n.Name, strings.Join(have, ", ")))
		default:
			parts = append(parts, fmt.Sprintf("%s refused it", n.Name))
		}
		n.mu.Unlock()
	}
	if voice == "" {
		return fmt.Errorf("%w: no node has a default voice (%s)", ErrUnknownVoice, strings.Join(parts, "; "))
	}
	return fmt.Errorf("%w: no node serves the voice %q (%s)", ErrUnknownVoice, voice, strings.Join(parts, "; "))
}

// NodeVoices is what Remote.Voices reports for one node.
type NodeVoices struct {
	// Name and URL are the node's, as configured.
	Name, URL string
	// OK is true when the node listed its voices just now.
	OK bool
	// Voices are the voices the node serves, and Default the one it uses when
	// a call names none. Empty when OK is false.
	Voices  []Voice
	Default Voice
	// Error is a short reason when OK is false: an older node that does not
	// list its voices is still used, and asked for the voice in each call.
	Error string
}

// Voices asks every node which voices it serves, in parallel, and remembers the
// answers for VoiceTTL: a call for a voice then goes only to the nodes that
// serve it, and a voice no node serves is refused at once, naming the nodes.
// Call it at boot, and again to refresh. A node that cannot answer (a node of an
// older version, an unreachable one) is not wrong and not taken out of the
// rotation: it is asked for the voice in the call itself.
func (r *Remote) Voices(ctx context.Context) []NodeVoices {
	out := make([]NodeVoices, len(r.nodes))
	var wg sync.WaitGroup
	for i, n := range r.nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = r.listVoices(ctx, n)
		}()
	}
	wg.Wait()
	return out
}

func (r *Remote) listVoices(ctx context.Context, n *remoteNode) NodeVoices {
	res := NodeVoices{Name: n.Name, URL: n.URL}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.URL+r.path, nil)
	if err != nil {
		res.Error = "the request could not be built"
		return res
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	resp, err := r.http.Do(req)
	if err != nil {
		res.Error = "unreachable"
		return res
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		res.Error = "the token was refused"
		return res
	case resp.StatusCode != http.StatusOK:
		res.Error = fmt.Sprintf("the node does not list its voices (status %d)", resp.StatusCode)
		return res
	}
	var got wireVoices
	if json.Unmarshal(raw, &got) != nil || len(got.Voices) == 0 {
		res.Error = "the node did not say which voices it serves"
		return res
	}
	catalog := make(map[Voice]bool, len(got.Voices))
	for _, v := range got.Voices {
		res.Voices = append(res.Voices, Voice(v))
		catalog[Voice(v)] = true
	}
	res.Default = Voice(got.Default)
	res.OK = true
	n.mu.Lock()
	n.catalog, n.catalogUntil = catalog, r.now().Add(r.voiceTTL)
	n.mu.Unlock()
	return res
}
