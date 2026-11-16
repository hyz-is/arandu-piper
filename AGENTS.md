# Working on Piper

This is an Arandu package that is a library: it puts Piper, the text-to-speech
engine, behind one call, `Speak`, and an interface, `Speaker`, with a `Fake`. It
owns no table, no route and no policy, and it does not import the framework, so
an application on any framework version can install it. It is a Go module
somebody `go get`s and builds by hand in their own `bootstrap/app.go`.

Read `.agents/skills/` before writing code.

## The gates

Nothing is finished until all four exit zero.

```sh
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

`GOWORK=off` is required: this checkout may sit beside a Go workspace that does
not list it.

## What this repository holds

| file | role |
| --- | --- |
| `piper.go` | `Audio`, `Speaker`, `Speak`, `SpeakAs`, `Key`, the errors, `ProcessError`, the WAV reader and the stderr summary |
| `config.go` | `Config`, `Format`, the defaults, `New` and the validation of every path |
| `voice.go` | `Voice`, `VoiceFaber`, `VoiceCadu`, `ParseVoice`, `VoicesIn`, `VoiceFiles` and the check of each voice's files |
| `cache.go` | `Keyed`, `Cache`, `Store`, `Recording`, `Result`, `MemoryStore`: an audio made once and reused for the same key |
| `plain.go` | `Plain`, which strips WhatsApp and Markdown formatting and emojis before the text is spoken |
| `fake.go` | `Fake`, the scripted `Speaker` for an application's tests |
| `docs/Dockerfile.snippet` | the build and runtime stages that put Piper and a voice in an application's image; it was built and run, keep it that way when a digest or version changes |
| `tests/Unit/piper_test.go` | the client against stand-in `piper` and `ffmpeg` scripts, against the real ffmpeg when present, against the real Piper when `PIPER_BIN` and `PIPER_MODEL` are set, and the fake |
| `tests/Unit/voice_test.go` | several voices in one client, `ParseVoice`, `VoicesIn` and what changes the key |
| `tests/Unit/cache_test.go` | the reuse, a failing store, identical calls at the same time |
| `tests/Unit/plain_test.go` | `Plain` |
| `tests/Unit/manifest_test.go` | `arandu.mod.toml` held against the imports |

## Rules

- **Programs are started with `exec.CommandContext`, never through a shell,
  with fixed arguments.** The text never reaches an argument list: it goes in
  through standard input. Everything else that varies (the voice, the speaker,
  the pace) is one argument of its own, and `Config` validates each so it can
  never look like a flag.
- **The program does not inherit the application's environment.** `environment`
  gives it a minimum and `Config.Env`; a secret the application holds is never
  handed to Piper or ffmpeg.
- **Nothing the person said goes into an error, a log or a file that outlives
  the call.** Standard output is discarded. An error carries the exit status and
  a summarized, truncated standard error with the working directory replaced
  and any line that repeats the text replaced by `<text>`.
- **One private working directory per call**, under `Config.TempDir`, removed
  with `defer` on every path, including timeout and cancellation.
- **Limits are checked before the expensive step.** The length of the text is
  checked before anything runs, and a guard bounds the size of what is read back.
- **A key changes with everything that changes the audio, and with nothing
  else.** Whatever new setting reaches the command line goes into `Key` too, or
  a cache answers with audio that no longer matches.
- **The timeout kills the process.** `Config.Timeout` covers Piper and the
  conversion together; an elapsed deadline is `ErrTimeout`, a cancelled caller
  is `context.Canceled`.
- **The facts about Piper's command line live in `synthesize`**: `-m`, `-c`,
  `-f`, and optionally `-s`, `--length_scale` and `--sentence_silence`, with the
  text on standard input. They are accepted by the `piper` of the `piper-tts`
  package (OHF-Voice/piper1-gpl, GPL-3.0) and by the standalone binary of
  rhasspy/piper (MIT, archived); do not add `-q`, which only the second knows.
  Check both against the release in use before changing them.
- **The voice decides the engine.** The `pt_BR` voices that are licensed for
  commercial use were retrained in 2026 and need `piper-tts` 1.5 or later; the
  standalone binary fails on them with "Phonemes must be one codepoint". The
  voices it does run are derived from the Lessac corpus, whose license excludes
  commercial use. Say so wherever a voice is recommended.
- **No framework import and no third-party dependency.** `go.mod` has no
  `require`. If one is ever needed, the `framework` floor in `arandu.mod.toml`
  moves with it and every installer is told in `UPGRADE.md`.
- **A test never needs Piper or a voice.** It uses the stand-in scripts or the
  `Fake`; the one test that runs the real program skips itself with the reason.

## Commits

The identity is set in this checkout: `HYZIS <admin@hyz.is>`. No AI attribution
of any kind. Work goes to `develop`, and releases are tags on `main`.
