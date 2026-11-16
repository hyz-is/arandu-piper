# Changelog

Everything worth knowing about a release of Piper is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

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
