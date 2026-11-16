package piper

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Keyed is a Speaker that can name the audio a call would make before making
// it. *Client, *Fake and *Cache are Keyed.
type Keyed interface {
	Speaker
	// Key names the audio SpeakAs would make of text with voice: the same
	// audio, the same key. See (*Client).Key.
	Key(voice Voice, text string) (string, error)
}

// Recording is one audio a Cache keeps for reuse, with what made it.
type Recording struct {
	// Key is the audio's name, from Keyed.Key: 64 hexadecimal characters,
	// usable as the object name in a bucket.
	Key string
	// Voice is the voice asked for; empty means the speaker's default voice
	// (the key already tells the voices apart).
	Voice Voice
	// Text is the text that was spoken, with its spacing cleaned as the
	// speaker cleans it.
	Text string
	// Audio is the recording.
	Audio Audio
}

// Store keeps recordings for a Cache. The package does not decide where: the
// application implements it over its own table and disk -- the audio in its
// bucket under Key, a row with the key, the voice, the text, the object, the
// MIME type, the duration and when it was last used. MemoryStore keeps them in
// memory.
//
// Find and Save are called concurrently. A Store error never fails a call: the
// Cache makes the audio anyway and reports the error in Result.StoreErr.
type Store interface {
	// Find returns the audio saved under key, and false when there is none.
	Find(ctx context.Context, key string) (Audio, bool, error)
	// Save keeps a recording. Saving a key that is already kept may replace
	// it or keep the first; both hold the same audio.
	Save(ctx context.Context, r Recording) error
}

// Result is what Cache.Say answers.
type Result struct {
	// Audio is the speech.
	Audio Audio
	// Key names it, as Recording.Key.
	Key string
	// Reused is true when the audio came from the Store, or from an identical
	// call that was running at the same moment, and false when it was made by
	// this call.
	Reused bool
	// StoreErr is set when the Store failed to find or to save. The audio is
	// still good; the application decides whether to log it.
	StoreErr error
}

// Cache makes each audio once: before speaking it asks the Store for the
// audio of the same key, and after speaking it saves what was made, so a reply
// the assistant gives every day ("Anotado. Está certo?") costs the CPU and the
// bucket once. Identical calls that arrive while the audio is being made wait
// for that one instead of making their own.
//
// The audio is reused only when the key is the same, that is, when the very
// same words would be said by the same voice with the same settings. A text
// that only means the same thing -- another number, another name -- is
// another audio, as reusing it would say something that is not in the text.
//
//	speaker := piper.NewCache(client, store)
//	res, err := speaker.Say(ctx, piper.VoiceCadu, reply)
//	// res.Reused, res.Key
type Cache struct {
	speaker Keyed
	store   Store

	mu      sync.Mutex
	flights map[string]*flight
}

type flight struct {
	done chan struct{}
	res  Result
	err  error
}

var _ Keyed = (*Cache)(nil)

// NewCache builds a Cache over speaker and store. Both are required.
func NewCache(speaker Keyed, store Store) *Cache {
	if speaker == nil || store == nil {
		panic("piper: NewCache needs a speaker and a store")
	}
	return &Cache{speaker: speaker, store: store, flights: map[string]*flight{}}
}

// Speak is Say with the default voice, keeping only the audio.
func (c *Cache) Speak(ctx context.Context, text string) (Audio, error) {
	res, err := c.Say(ctx, "", text)
	return res.Audio, err
}

// SpeakAs is Say, keeping only the audio.
func (c *Cache) SpeakAs(ctx context.Context, voice Voice, text string) (Audio, error) {
	res, err := c.Say(ctx, voice, text)
	return res.Audio, err
}

// Key is the speaker's key.
func (c *Cache) Key(voice Voice, text string) (string, error) {
	return c.speaker.Key(voice, text)
}

// Say answers the audio of text with voice (the default voice when empty):
// the stored one when the Store has its key, or a new one, which it then
// saves. It refuses what the speaker refuses, before asking the Store.
func (c *Cache) Say(ctx context.Context, voice Voice, text string) (Result, error) {
	key, err := c.speaker.Key(voice, text)
	if err != nil {
		return Result{}, err
	}
	for {
		c.mu.Lock()
		if f, ok := c.flights[key]; ok {
			c.mu.Unlock()
			select {
			case <-f.done:
			case <-ctx.Done():
				return Result{}, contextError(ctx)
			}
			// The call being waited on was cancelled by its own caller; this
			// one may still have time, so it tries again.
			if f.err != nil && isContextError(f.err) && ctx.Err() == nil {
				continue
			}
			res := f.res
			res.Reused = f.err == nil
			res.Audio.Data = append([]byte(nil), f.res.Audio.Data...)
			return res, f.err
		}
		f := &flight{done: make(chan struct{})}
		c.flights[key] = f
		c.mu.Unlock()

		f.res, f.err = c.make(ctx, key, voice, text)

		c.mu.Lock()
		delete(c.flights, key)
		c.mu.Unlock()
		close(f.done)
		return f.res, f.err
	}
}

// make finds the audio in the Store or speaks and saves it.
func (c *Cache) make(ctx context.Context, key string, voice Voice, text string) (Result, error) {
	res := Result{Key: key}
	audio, ok, err := c.store.Find(ctx, key)
	if err != nil {
		res.StoreErr = err
	} else if ok && len(audio.Data) > 0 {
		res.Audio, res.Reused = audio, true
		return res, nil
	}

	audio, err = c.speaker.SpeakAs(ctx, voice, text)
	if err != nil {
		return Result{}, err
	}
	res.Audio = audio
	rec := Recording{Key: key, Voice: voice, Text: normalize(text), Audio: audio}
	if err := c.store.Save(ctx, rec); err != nil {
		res.StoreErr = errors.Join(res.StoreErr, err)
	}
	return res, nil
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func contextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrTimeout, ctx.Err())
	}
	return ctx.Err()
}

// MemoryStore keeps recordings in memory, for tests and for a process whose
// recordings need not outlive it. It is safe for concurrent use.
type MemoryStore struct {
	mu   sync.Mutex
	recs map[string]Recording
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore builds an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{recs: map[string]Recording{}} }

// Find returns a copy of the audio saved under key.
func (m *MemoryStore) Find(_ context.Context, key string) (Audio, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[key]
	if !ok {
		return Audio{}, false, nil
	}
	a := r.Audio
	a.Data = append([]byte(nil), a.Data...)
	return a, true, nil
}

// Save keeps a copy of r.
func (m *MemoryStore) Save(_ context.Context, r Recording) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.Audio.Data = append([]byte(nil), r.Audio.Data...)
	m.recs[r.Key] = r
	return nil
}

// Recordings lists what is kept, in no particular order.
func (m *MemoryStore) Recordings() []Recording {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Recording, 0, len(m.recs))
	for _, r := range m.recs {
		r.Audio.Data = append([]byte(nil), r.Audio.Data...)
		out = append(out, r)
	}
	return out
}
