# Changelog

Everything worth knowing about a release of Piper is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

## [0.3.1] - 2026-11-25

### Added

- `RemoteConfig.BusyWait` (default `DefaultBusyWait`, 5 seconds): when every node answered 429, the round is repeated, with a pause that grows from 200 ms to 1 s, until a node frees or `BusyWait` is over, and only then does the call fail with `ErrNoNodes`. A burst of more calls than the nodes take at once waits its turn instead of failing. A negative value restores the old behaviour. A node that fails (not busy) is still not waited for.
- `docs/sitecustomize.py`: limits the threads of onnxruntime from `PIPER_ORT_THREADS` and turns its telemetry off. Put it in the directory `PYTHONPATH` names (`/opt/piper`), and pass `PIPER_ORT_THREADS=<n>` in `Config.Env`.

### Fixed

- The documentation of `Config.Env` suggested `OMP_NUM_THREADS=2` to limit the CPU Piper uses. The onnxruntime of the `piper-tts` wheel does not read it: it starts a thread per core it sees, and under a CPU quota of two cores a sentence of six seconds took eight to synthesize where two threads take one and a half (measured on a Xeon E5-2667 v4 shared with other work). The comment now says so and names the shim.

## [0.3.0] - 2026-11-24

### Added

- `Handler` (`NewHandler(HandlerConfig)`): serves any `Speaker` over HTTP. `POST` with `{"text": "...", "voice": "..."}` (the voice is optional) answered with the audio as the body, its MIME type as `Content-Type` and its length in milliseconds in `X-Audio-Duration-Ms` (`DurationHeader`). It checks a bearer token per client (compared in constant time, at least 32 characters, never logged), limits the body before and while reading it, limits the concurrency and answers `429` with `Retry-After` when no slot frees within `QueueWait`, and logs one line per call with the client, status, number of characters and time, never the text or the audio. Errors are `{"error": code, "message": text}` with a stable code per sentinel error.
- `Remote` (`NewRemote(RemoteConfig)`): a `Speaker`, and `Keyed`, that calls nodes running `Handler`, so an application switches between the local `*Client` and a remote service by configuration, and a `Cache` works over it. It spreads calls over the nodes in turn, tries the next node at once when one is busy, unreachable, timed out or failing, takes a node out of the rotation after `FailureThreshold` failures in a row for `Cooldown`, tries the nodes that are out as a last resort when all are, and returns a refusal another node would repeat (empty or too long text, an unknown voice) as the same sentinel error the local client gives. `Remote.Key` hashes the voice, the cleaned text and `RemoteConfig.KeyTag`, with no network. `Remote.Health` asks every node's `/_arandu/health` and reports the place of each in the rotation.
- `ParseNodes` reads the nodes from one configuration value: `one=https://one.example,two=https://two.example`, bare URLs, or a single URL.

### Changed

- `arandu.mod.toml` declares `network = true`: the module can now use the network, and only when `Handler` or `Remote` is built. Nothing changes for an application that only uses `New`.

## [0.2.0] - 2026-11-16

### Added

- `Config.NoiseScale` and `Config.NoiseW`, passed to Piper as `--noise_scale` and `--noise_w` (both accepted by the `piper-tts` program and by the standalone binary). They set how much the voice varies from phrase to phrase and how much the length of each sound varies, which is what separates a lively voice from a mechanical one. Zero leaves the voice's own values; `New` refuses a negative, NaN or above-2 value.
- Both enter `Client.Key` when set. A client that leaves them at zero keeps the keys of 0.1.0, so audio already stored stays valid.

## [0.1.0] - 2026-11-15

### Added

- `New(Config)` builds a `*Client`, refusing a missing or non-executable `piper`, a missing or empty voice, a voice configuration that is not JSON, a speaker the voice does not have, an out-of-range `LengthScale`, `SentenceSilence` or `OpusBitrate`, an unknown `Format`, a missing `ffmpeg` when Opus is asked for, a negative limit and a `TempDir` that is not a directory.
- `Speak(ctx, text)` runs Piper with the text on standard input and the WAV written to a private temporary directory, and returns an `Audio` with the bytes, the MIME type and the duration read from the WAV header. The directory is removed on every path. The arguments (`-m`, `-c`, `-f`, and optionally `-s`, `--length_scale`, `--sentence_silence`) are accepted by both the `piper` of the `piper-tts` package and the standalone Piper binary.
- `FormatOpus` converts the speech with ffmpeg to mono 16 kHz Ogg Opus, the format of a WhatsApp voice note, at `DefaultOpusBitrate` (24 kbit/s) or `Config.OpusBitrate`.
- Limits and protections: `MaxChars` (default 1500 characters) checked before anything runs, `Timeout` (default one minute) covering the whole call and killing the running program, and a program environment reduced to `PATH`, `LANG`, `PYTHONUTF8`, `HOME` and `TMPDIR` plus `Config.Env`, so the application's secrets are not handed to Piper.
- Errors to test with `errors.Is`: `ErrEmptyText`, `ErrTextTooLong`, `ErrTimeout`, `ErrFailed`, `ErrBadOutput`, `ErrConvertFailed`. A failing program is a `*ProcessError` with its exit status and a short stderr summary in which any line that repeats the spoken text is replaced by `<text>`.
- `Plain(text)` removes the formatting of WhatsApp and Markdown (bold, italic, strikethrough, code, headings, quotes, bullets, numbered items, links, web addresses, emojis, arrows) and joins the lines into sentences, so none of it is read aloud.
- `Speaker` and `Fake`, a scripted `Speaker` that records the texts it was asked to say, for an application's tests.
- Several voices in one client: `Voice`, with `VoiceFaber` (`pt_BR-faber-medium`) and `VoiceCadu` (`pt_BR-cadu-medium`); `Config.Voices`, a `map[Voice]VoiceFiles` (`ModelPath`, `ConfigPath`, `SpeakerID` per voice), and `Config.DefaultVoice`, required when there is more than one; `VoicesIn(dir, voices...)` for files that keep their catalogue names; `ParseVoice` for configuration, accepting `faber`, `cadu` or a catalogue name; `Client.Voices` and `Client.DefaultVoice`.
- `SpeakAs(ctx, voice, text)` on every `Speaker`; `Speak` is the default voice. A voice the client does not hold is `ErrUnknownVoice`, before anything runs.
- `Client.Key(voice, text)`: a hash of everything that changes the audio (the voice and its files, speaker, pace, pauses, format, bit rate and the cleaned text).
- `Cache`, over a `Keyed` speaker and a `Store` the application implements: each audio is made once and reused for the same key; identical calls running at once make it once; a store failure never fails the call (`Result.StoreErr`). `Recording` is what is saved, `Result` says whether it was `Reused`, and `MemoryStore` keeps recordings in memory.
- `Fake.Calls` and `Call` record the voice of each request, and `Fake.Key` lets a `Cache` run over a `Fake`.
