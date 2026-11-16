package piper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Fake answers from a script, in memory, and records what it was asked to say.
// It is what an application's tests pass where production passes *Client.
//
//	fake := piper.NewFake(piper.Answer{Duration: 4 * time.Second})
//	svc := services.NewAssistant(db, fake)
//	...
//	fake.Requests()[0] // what the service asked to be said
//	fake.Calls()[0].Voice // and with which voice
//
// It speaks with any voice it is asked for; an empty voice is recorded as
// empty, meaning the default one.
type Fake struct {
	mu     sync.Mutex
	script []Answer
	calls  []Call
}

// Call is one request a Fake received.
type Call struct {
	// Voice is the voice asked for; empty for Speak and for SpeakAs with the
	// default voice.
	Voice Voice
	// Text is the text, as it was given.
	Text string
}

// Answer is one scripted answer: an audio, or an error.
type Answer struct {
	// Audio is what Speak returns. When Data is empty, a silent WAV of 100 ms
	// stands in for it, and an empty MIMEType means MIMEWAV.
	Audio Audio
	// Err, when set, is returned instead of audio.
	Err error
}

var _ Speaker = (*Fake)(nil)

// ErrScriptExhausted is what a Fake answers once its script has run out.
var ErrScriptExhausted = errors.New("piper: the fake has no scripted answer left")

// NewFake builds a fake that gives the answers in order.
func NewFake(answers ...Answer) *Fake { return &Fake{script: answers} }

// Push appends answers to the script.
func (f *Fake) Push(answers ...Answer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = append(f.script, answers...)
}

// Speak is SpeakAs with the default voice.
func (f *Fake) Speak(ctx context.Context, text string) (Audio, error) {
	return f.SpeakAs(ctx, "", text)
}

// SpeakAs records the call and gives the next scripted answer. Like the real
// client, it answers ErrEmptyText for text with nothing to say and
// ErrTextTooLong over DefaultMaxChars, and those calls do not use up an
// answer.
func (f *Fake) SpeakAs(_ context.Context, voice Voice, text string) (Audio, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Voice: voice, Text: text})
	if _, err := fakeSpoken(text); err != nil {
		return Audio{}, err
	}
	if len(f.script) == 0 {
		return Audio{}, ErrScriptExhausted
	}
	a := f.script[0]
	f.script = f.script[1:]
	if a.Err != nil {
		return Audio{}, a.Err
	}
	audio := a.Audio
	if len(audio.Data) == 0 {
		audio.Data = silentWAV(100 * time.Millisecond)
		if audio.Duration == 0 {
			audio.Duration = 100 * time.Millisecond
		}
	}
	if audio.MIMEType == "" {
		audio.MIMEType = MIMEWAV
	}
	return audio, nil
}

// Requests are the texts received, in order, as they were given.
func (f *Fake) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.Text
	}
	return out
}

// Calls are the requests received, in order, with their voices.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Key names the audio as the real client does, from the voice and the cleaned
// text alone, so a Cache over a Fake reuses what the same text produced. It
// refuses what SpeakAs refuses.
func (f *Fake) Key(voice Voice, text string) (string, error) {
	spoken, err := fakeSpoken(text)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte("arandu-piper/fake\x00" + string(voice) + "\x00" + spoken))
	return hex.EncodeToString(h[:]), nil
}

func fakeSpoken(text string) (string, error) {
	spoken := normalize(text)
	if !speakable(spoken) {
		return "", ErrEmptyText
	}
	if len([]rune(spoken)) > DefaultMaxChars {
		return "", ErrTextTooLong
	}
	return spoken, nil
}

// silentWAV is a 22 050 Hz mono 16-bit PCM WAV of silence, the shape of what a
// medium voice produces.
func silentWAV(d time.Duration) []byte {
	const rate = 22050
	n := int(d.Seconds()*rate) * 2
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+n))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(n))
	b.Write(make([]byte, n))
	return b.Bytes()
}
