package unit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// voiceSpeaker is a Speaker and a VoiceLister: it speaks the voices it was
// given and refuses the others as a *Client does.
type voiceSpeaker struct {
	name   string
	voices []piper.Voice
	calls  atomic.Int64
}

func (s *voiceSpeaker) Speak(ctx context.Context, text string) (piper.Audio, error) {
	return s.SpeakAs(ctx, "", text)
}

func (s *voiceSpeaker) SpeakAs(_ context.Context, voice piper.Voice, _ string) (piper.Audio, error) {
	s.calls.Add(1)
	if voice == "" {
		voice = s.DefaultVoice()
	}
	for _, v := range s.voices {
		if v == voice {
			return piper.Audio{Data: []byte(fmt.Sprintf("audio-of-%s-as-%s", s.name, voice)), MIMEType: piper.MIMEOpus, Duration: time.Second}, nil
		}
	}
	return piper.Audio{}, fmt.Errorf("%w: %q", piper.ErrUnknownVoice, voice)
}

func (s *voiceSpeaker) Voices() []piper.Voice     { return s.voices }
func (s *voiceSpeaker) DefaultVoice() piper.Voice { return s.voices[0] }

// voiceNode serves a voiceSpeaker with the real Handler.
func voiceNode(t *testing.T, name string, voices ...piper.Voice) (*httptest.Server, *voiceSpeaker) {
	t.Helper()
	sp := &voiceSpeaker{name: name, voices: voices}
	h := newHandler(t, sp, nil)
	mux := http.NewServeMux()
	mux.Handle("/speak", h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, sp
}

func voiceRemote(t *testing.T, mod func(*piper.RemoteConfig), nodes ...*httptest.Server) *piper.Remote {
	t.Helper()
	cfg := piper.RemoteConfig{Token: tokenOne, Cooldown: time.Hour}
	for i, n := range nodes {
		cfg.Nodes = append(cfg.Nodes, piper.Node{Name: fmt.Sprintf("n%d", i+1), URL: n.URL})
	}
	if mod != nil {
		mod(&cfg)
	}
	r, err := piper.NewRemote(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestHandlerListsItsVoicesToACallerWithAToken(t *testing.T) {
	sp := &voiceSpeaker{name: "a", voices: []piper.Voice{piper.VoiceCadu, piper.VoiceDora}}
	h := newHandler(t, sp, nil)

	req := httptest.NewRequest(http.MethodGet, "/speak", nil)
	req.Header.Set("Authorization", "Bearer "+tokenOne)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"voices":["pt_BR-cadu-medium","pf_dora"]`) || !strings.Contains(body, `"default":"pt_BR-cadu-medium"`) {
		t.Fatalf("%s", body)
	}
	if sp.calls.Load() != 0 {
		t.Fatal("listing the voices spoke")
	}

	// A speaker that cannot say lists nothing, which is "unknown", not "none".
	h = newHandler(t, piper.NewFake(), nil)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/speak", nil)
	req.Header.Set("Authorization", "Bearer "+tokenOne)
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestRemoteSendsAVoiceToTheNodeThatServesIt(t *testing.T) {
	piperNode, piperSpeaker := voiceNode(t, "piper-node", piper.VoiceCadu)
	kokoroNode, kokoroSpeaker := voiceNode(t, "kokoro-node", piper.VoiceCadu, piper.VoiceDora)
	r := voiceRemote(t, nil, piperNode, kokoroNode)

	listed := r.Voices(context.Background())
	if len(listed) != 2 || !listed[0].OK || !listed[1].OK || len(listed[0].Voices) != 1 || len(listed[1].Voices) != 2 || listed[1].Default != piper.VoiceCadu {
		t.Fatalf("Voices = %+v", listed)
	}

	for range 6 {
		got, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia.")
		if err != nil || string(got.Data) != "audio-of-kokoro-node-as-pf_dora" {
			t.Fatalf("%q %v", got.Data, err)
		}
	}
	if piperSpeaker.calls.Load() != 0 {
		t.Errorf("the node without the voice was called %d times", piperSpeaker.calls.Load())
	}
	if kokoroSpeaker.calls.Load() != 6 {
		t.Errorf("the node with the voice was called %d times", kokoroSpeaker.calls.Load())
	}
	// The voice both serve is still spread over both.
	seen := map[string]bool{}
	for range 6 {
		got, err := r.SpeakAs(context.Background(), piper.VoiceCadu, "Bom dia.")
		if err != nil {
			t.Fatal(err)
		}
		seen[string(got.Data)] = true
	}
	if len(seen) != 2 {
		t.Errorf("a voice both nodes serve went to %v", seen)
	}
}

func TestRemoteFindsTheNodeThatServesAVoiceWithoutBeingTold(t *testing.T) {
	// No call to Voices: the first node refuses, the second serves, and the
	// refusal is remembered so the first is not asked again.
	piperNode, piperSpeaker := voiceNode(t, "piper-node", piper.VoiceCadu)
	kokoroNode, _ := voiceNode(t, "kokoro-node", piper.VoiceDora)
	r := voiceRemote(t, nil, piperNode, kokoroNode)
	for range 5 {
		got, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia.")
		if err != nil || string(got.Data) != "audio-of-kokoro-node-as-pf_dora" {
			t.Fatalf("%q %v", got.Data, err)
		}
	}
	if n := piperSpeaker.calls.Load(); n > 1 {
		t.Errorf("the node without the voice was asked %d times", n)
	}
}

func TestRemoteSaysClearlyWhenNoNodeServesAVoice(t *testing.T) {
	a, _ := voiceNode(t, "a", piper.VoiceCadu)
	b, _ := voiceNode(t, "b", piper.VoiceFaber)
	r := voiceRemote(t, nil, a, b)
	r.Voices(context.Background())

	_, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia.")
	if !errors.Is(err, piper.ErrUnknownVoice) || errors.Is(err, piper.ErrNoNodes) {
		t.Fatalf("err = %v, want ErrUnknownVoice and not ErrNoNodes", err)
	}
	for _, want := range []string{`"pf_dora"`, "n1 serves pt_BR-cadu-medium", "n2 serves pt_BR-faber-medium"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error lacks %q: %v", want, err)
		}
	}
}

func TestRemoteForgetsARefusalAfterTheVoiceTTL(t *testing.T) {
	// The node refuses the voice, is upgraded to serve it, and is asked again
	// once the time is up.
	sp := &voiceSpeaker{name: "a", voices: []piper.Voice{piper.VoiceCadu}}
	h := newHandler(t, sp, nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	r, err := piper.NewRemote(piper.RemoteConfig{
		Nodes: []piper.Node{{Name: "a", URL: srv.URL}}, Token: tokenOne, Path: "/", VoiceTTL: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia."); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("%v", err)
	}
	sp.voices = append(sp.voices, piper.VoiceDora)
	if _, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia."); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("the refusal was not remembered: %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	if got, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia."); err != nil || !strings.Contains(string(got.Data), "pf_dora") {
		t.Fatalf("%q %v", got.Data, err)
	}
}

func TestRemoteUsesANodeThatDoesNotListItsVoices(t *testing.T) {
	// A node of an older version has no list of voices to give (it answers the
	// GET with something else): it is not wrong, and it is asked for the voice
	// in the call.
	old := newNode(t, "old")
	r := remote(t, nil, old)
	listed := r.Voices(context.Background())
	if len(listed) != 1 || listed[0].Name != "n1" {
		t.Fatalf("%+v", listed)
	}
	got, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia.")
	if err != nil || string(got.Data) != "audio-of-old" {
		t.Fatalf("%q %v", got.Data, err)
	}
}

func TestRemoteVoicesReportsAnUnreachableNodeAndKeepsGoing(t *testing.T) {
	a, _ := voiceNode(t, "a", piper.VoiceDora)
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	r := voiceRemote(t, nil, a, down)
	listed := r.Voices(context.Background())
	if !listed[0].OK || listed[1].OK || listed[1].Error == "" {
		t.Fatalf("%+v", listed)
	}
	if got, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia."); err != nil || !strings.Contains(string(got.Data), "pf_dora") {
		t.Fatalf("%q %v", got.Data, err)
	}
}

func TestRemoteAFailingNodeAndARefusingNodeIsNotAVoiceError(t *testing.T) {
	// One node refuses the voice and the other is down: the voice may exist on
	// the node that is down, so the answer is "no node answered".
	a, _ := voiceNode(t, "a", piper.VoiceCadu)
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	r := voiceRemote(t, nil, a, down)
	_, err := r.SpeakAs(context.Background(), piper.VoiceDora, "Bom dia.")
	if !errors.Is(err, piper.ErrNoNodes) {
		t.Fatalf("err = %v, want ErrNoNodes", err)
	}
}
