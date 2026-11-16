package unit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

func TestTheCacheMakesEachAudioOnceAndReusesIt(t *testing.T) {
	fake := piper.NewFake(piper.Answer{Audio: piper.Audio{Data: []byte("first"), MIMEType: piper.MIMEOpus, Duration: time.Second}})
	store := piper.NewMemoryStore()
	cache := piper.NewCache(fake, store)
	ctx := context.Background()

	first, err := cache.Say(ctx, piper.VoiceCadu, "Anotado. Está certo?")
	if err != nil || first.Reused || string(first.Audio.Data) != "first" || first.StoreErr != nil {
		t.Fatalf("first = %+v, %v", first, err)
	}
	again, err := cache.Say(ctx, piper.VoiceCadu, " Anotado.\nEstá certo? ")
	if err != nil || !again.Reused || string(again.Audio.Data) != "first" || again.Key != first.Key {
		t.Fatalf("again = %+v, %v", again, err)
	}
	if n := len(fake.Requests()); n != 1 {
		t.Fatalf("the speaker was called %d times, want once", n)
	}

	recs := store.Recordings()
	if len(recs) != 1 {
		t.Fatalf("recordings = %d", len(recs))
	}
	if r := recs[0]; r.Key != first.Key || r.Voice != piper.VoiceCadu || r.Text != "Anotado. Está certo?" || r.Audio.MIMEType != piper.MIMEOpus || r.Audio.Duration != time.Second {
		t.Fatalf("recording = %+v", r)
	}

	// Another voice, or other words, is another audio.
	fake.Push(piper.Answer{}, piper.Answer{})
	if res, err := cache.Say(ctx, piper.VoiceFaber, "Anotado. Está certo?"); err != nil || res.Reused {
		t.Fatalf("other voice = %+v, %v", res, err)
	}
	if res, err := cache.Say(ctx, piper.VoiceCadu, "Anotado. Está errado?"); err != nil || res.Reused {
		t.Fatalf("other words = %+v, %v", res, err)
	}
	if n := len(store.Recordings()); n != 3 {
		t.Fatalf("recordings = %d, want 3", n)
	}
}

func TestTheCacheRefusesWhatTheSpeakerRefusesWithoutTheStore(t *testing.T) {
	store := &brokenStore{}
	cache := piper.NewCache(piper.NewFake(), store)
	if _, err := cache.Say(context.Background(), "", "  "); !errors.Is(err, piper.ErrEmptyText) {
		t.Fatalf("err = %v", err)
	}
	if store.finds != 0 || store.saves != 0 {
		t.Fatalf("the store was asked: %d finds, %d saves", store.finds, store.saves)
	}
}

func TestAStoreThatFailsNeverFailsTheCall(t *testing.T) {
	store := &brokenStore{err: errors.New("bucket down")}
	cache := piper.NewCache(piper.NewFake(piper.Answer{}), store)
	res, err := cache.Say(context.Background(), "", "Bom dia.")
	if err != nil || len(res.Audio.Data) == 0 || res.Reused {
		t.Fatalf("res = %+v, %v", res, err)
	}
	if !errors.Is(res.StoreErr, store.err) || store.finds != 1 || store.saves != 1 {
		t.Fatalf("StoreErr = %v, finds %d, saves %d", res.StoreErr, store.finds, store.saves)
	}
}

func TestASpeakerErrorIsNotSaved(t *testing.T) {
	boom := errors.New("boom")
	store := piper.NewMemoryStore()
	cache := piper.NewCache(piper.NewFake(piper.Answer{Err: boom}), store)
	if _, err := cache.Speak(context.Background(), "Bom dia."); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(store.Recordings()); n != 0 {
		t.Fatalf("recordings = %d", n)
	}
}

func TestIdenticalCallsAtTheSameTimeMakeOneAudio(t *testing.T) {
	speaker := &slowSpeaker{Fake: piper.NewFake(piper.Answer{}), release: make(chan struct{})}
	cache := piper.NewCache(speaker, piper.NewMemoryStore())
	const n = 8
	var wg sync.WaitGroup
	results := make([]piper.Result, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = cache.Say(context.Background(), piper.VoiceCadu, "Anotado. Está certo?")
		}()
	}
	// Let the calls pile up on the one being made, then let it finish.
	for speaker.started() == 0 {
		time.Sleep(time.Millisecond) //nolint:forbidigo // waiting for goroutines of this test only
	}
	close(speaker.release)
	wg.Wait()

	made := 0
	for i := range n {
		if errs[i] != nil || len(results[i].Audio.Data) == 0 {
			t.Fatalf("call %d = %+v, %v", i, results[i], errs[i])
		}
		if !results[i].Reused {
			made++
		}
	}
	if made != 1 || len(speaker.Requests()) != 1 {
		t.Fatalf("%d calls made audio and the speaker ran %d times, want 1 and 1", made, len(speaker.Requests()))
	}
}

func TestTheCacheIsASpeaker(t *testing.T) {
	var s piper.Speaker = piper.NewCache(piper.NewFake(piper.Answer{}), piper.NewMemoryStore())
	if _, err := s.SpeakAs(context.Background(), piper.VoiceFaber, "Bom dia."); err != nil {
		t.Fatal(err)
	}
}

type brokenStore struct {
	mu           sync.Mutex
	err          error
	finds, saves int
}

func (b *brokenStore) Find(context.Context, string) (piper.Audio, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finds++
	return piper.Audio{}, false, b.err
}

func (b *brokenStore) Save(context.Context, piper.Recording) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saves++
	return b.err
}

// slowSpeaker holds every call until release is closed.
type slowSpeaker struct {
	*piper.Fake
	release chan struct{}
	mu      sync.Mutex
	n       int
}

func (s *slowSpeaker) SpeakAs(ctx context.Context, v piper.Voice, text string) (piper.Audio, error) {
	s.mu.Lock()
	s.n++
	s.mu.Unlock()
	<-s.release
	return s.Fake.SpeakAs(ctx, v, text)
}

func (s *slowSpeaker) started() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
