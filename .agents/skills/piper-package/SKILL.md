---
name: piper-package
description: Install, wire and use the Piper package (github.com/hyz-is/arandu-piper) in an Arandu application. Use when the request is to "speak a reply", "text to speech", "answer with a voice note", "WhatsApp voice note from text", "use Piper", "use Kokoro", "a female voice", "pf_dora", "add piper to the app", "go get github.com/hyz-is/arandu-piper", "wire piper into bootstrap/app.go", "test code that speaks", "piper not found", "kokoro-speak", "ErrTimeout", "ErrTextTooLong", "ErrEmptyText", "ErrUnknownVoice", or when a project's go.mod already requires github.com/hyz-is/arandu-piper. Covers the paths in the application's config, the one construction in bootstrap, Speak, Speakable and Plain, the errors, the Fake for tests, what must be installed on the server and in the image (Piper, and Kokoro for the voices it owns), which voices may be used commercially, which node of a service serves which voice, and why no text or audio goes into a log.
license: MIT
metadata:
  audience: app
---

# Using Piper

A library: `piper.New(Config)` builds a `*piper.Client`, which implements
`piper.Speaker`. It has no table, no route and no policy, so there is no
`k.Register`, no migration and no permission to open. It runs Piper, Kokoro for
the voices that belong to it, and ffmpeg for voice notes, on the server, so the
deployment has to carry them.

**A voice belongs to an engine, by its name.** Piper: `piper.VoiceFaber`,
`piper.VoiceCadu` and any catalogue name. Kokoro (0.4.0): `piper.VoiceDora`
(female), `piper.VoiceAlex`, `piper.VoiceSanta`. A client holds voices of both,
and the voice in `SpeakAs` picks the engine. For a female Brazilian voice use
Kokoro's `dora`; Piper has none.

## Install

```bash
go get github.com/hyz-is/arandu-piper@<latest tag>   # gh release list -R hyz-is/arandu-piper
```

On the server (the README and `docs/Dockerfile.snippet` build all of it):

- `piper`, the program of the `piper-tts` Python package (1.5 or later), with
  Python 3.9 or later. It is GPL-3.0: this package runs it, never links it.
- a voice: the `.onnx` and its `.onnx.json`. **Mind the license.** A voice is
  fine-tuned from other voices, and several carry non-commercial terms. For
  commercial use take `pt_BR-faber-medium` (credit Hi-Fi TTS, CC BY 4.0) or
  `pt_BR-cadu-medium` (CC0), at the 2026 retrain: its `MODEL_CARD` must not say
  "lessac". The old standalone Piper binary only runs the old, lessac-derived
  voices.
- `ffmpeg` with libopus, only for `piper.FormatOpus`.

For the Kokoro voices (`docs/Dockerfile.kokoro.snippet` builds all of it, and
the README has the sha256 of each file):

- `docs/kokoro-speak.py` with its first line set to the interpreter that has the
  packages (Python 3.10 to 3.13), and the packages of
  `docs/kokoro-requirements.txt` (`pip install --require-hashes --no-deps`).
- `kokoro-v1.0.onnx` (fp32; the int8 one is half the time, listen first) and
  `voices-v1.0.bin`, from the `model-files-v1.0` release of kokoro-onnx.
  Kokoro-82M is Apache-2.0; keep its notice with the model. Allow about 700 MB
  of memory per call and several seconds of CPU on two cores: a job, not a call
  that waits on the line.

## Wire it, in bootstrap/app.go

1. Put the paths in the application's config (`PIPER_BIN`, `PIPER_MODEL`,
   optionally `PIPER_PYTHONPATH`, `FFMPEG_BIN`), and add the empty variables to
   `.env.example`.
2. Build the client once, only when configured:

   ```go
   var voice piper.Speaker
   if cfg.Piper.Configured() {
       c, err := piper.New(piper.Config{
           BinaryPath: cfg.Piper.Binary,
           ModelPath:  cfg.Piper.Model,
           Format:     piper.FormatOpus, // a WhatsApp voice note
           FFmpegPath: cfg.Piper.FFmpeg,
           Env:        []string{"PYTHONPATH=" + cfg.Piper.PythonPath}, // only when installed with pip --target
       })
       if err != nil {
           return App{}, err
       }
       voice = c
   }
   ```

   For the Kokoro voices the configuration is:

   ```go
   piper.Config{
       Kokoro: piper.KokoroConfig{
           ProgramPath: cfg.Kokoro.Bin,    // KOKORO_BIN
           ModelPath:   cfg.Kokoro.Model,  // KOKORO_MODEL
           VoicesPath:  cfg.Kokoro.Voices, // KOKORO_VOICES
           Env:         []string{"PYTHONPATH=" + cfg.Kokoro.PythonPath},
       },
       DefaultVoice: piper.VoiceDora, // ParseVoice reads PIPER_VOICE=dora
       Format:       piper.FormatOpus,
   }
   ```

   Kokoro alone needs no `BinaryPath` or `Voices`; with both, the client holds
   the voices of the two engines. A missing program, model, voices file or voice
   fails in `New`, at boot.
3. Give the service the `piper.Speaker` interface, never `*piper.Client`.

## Use it

- `audio, err := voice.Speak(ctx, reply)`. Since 0.5.0 a `*Client` makes the
  text speakable first (`piper.Speakable`: numbers, money, dates, times, units,
  acronyms and symbols in words, formatting and emoji removed, list items as
  sentences), for Brazilian Portuguese only, which is every voice of the package.
  `Config.Speech` adds the application's lexicon (`Lexicon`), a cut for long
  answers (`MaxWords`, `RestNote`), lines to skip, or `Raw: true` to speak the
  text as given. `Normalizer.Speakable(text, now)` says dates relative to `now`;
  store its text and `Version()` beside the original, and put the version in
  `RemoteConfig.KeyTag`. `Plain` alone only removes WhatsApp and Markdown
  formatting, links and emojis. For text that will be heard, give the model the
  guideline of `docs/speech-ptbr.md` section 4. `audio.Data` is the file, `audio.MIMEType` is
  `audio/ogg; codecs=opus` or `audio/wav`, `audio.Duration` its length.
- Errors: `ErrEmptyText` (nothing speakable: send the text instead),
  `ErrTextTooLong` (shorten or split, then speak), `ErrTimeout`, `ErrFailed`
  (with `errors.As` to a `*piper.ProcessError`), `ErrConvertFailed`,
  `ErrBadOutput`, and `context.Canceled` when the caller cancels. Always have a
  text fallback: a voice reply that fails must not lose the answer.
- Over a service (`piper.NewHandler`, `piper.NewRemote`), nodes may serve
  different voices. `Remote.Voices(ctx)` at boot reads what each node serves;
  a call for a voice goes only to the nodes that have it, and when none does it
  fails with `ErrUnknownVoice` naming the nodes. The fix is on a node (install
  the engine), never in the caller.
- It is CPU bound and starts Piper each time (about half a second on a laptop,
  more on a small server): call it from a job, bound how many run at once, and
  set `Timeout` from a measured run on the server.
- The program does not inherit the application's environment. What it needs
  (`PYTHONPATH`, `OMP_NUM_THREADS`) goes in `Config.Env`.
- Never log the text, the audio or a `ProcessError`'s fields beyond the exit
  status: they can carry personal data. The package itself logs nothing.

## Test with it

```go
fake := piper.NewFake(piper.Answer{}, piper.Answer{Err: piper.ErrTimeout})
// pass fake where production passes the client
fake.Requests() // the texts it was asked to say, in order
```

An `Answer{}` returns a short silent WAV, so code that uploads or sends the
audio has real bytes to work with.
