# Working on Piper

This is an Arandu package that is a library: it puts text-to-speech engines
(Piper, and Kokoro for the voices that belong to it) behind one call, `Speak`,
and an interface, `Speaker`, with a `Fake`. It
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
| `voice.go` | `Voice`, `Engine`, `VoiceFaber`, `VoiceCadu`, `VoiceDora`, `VoiceAlex`, `VoiceSanta`, `ParseVoice`, `VoicesIn`, `VoiceFiles` and the check of each Piper voice's files |
| `kokoro.go` | `KokoroConfig`, the boot check of the Kokoro program, model and voices file (an `.npz`, opened to see that every voice is in it) and `synthesizeKokoro`, the facts about `kokoro-speak`'s command line |
| `cache.go` | `Keyed`, `Cache`, `Store`, `Recording`, `Result`, `MemoryStore`: an audio made once and reused for the same key |
| `handler.go` | `Handler`: `POST` text and voice, the audio back, `GET` the voices the node serves (`VoiceLister`), bearer tokens per client, body and concurrency limits, the wire error codes |
| `remote.go` | `Remote`, `ParseNodes`, `Health`, `Voices`: a `Speaker` and `Keyed` over several nodes with failover, a cooldown and routing of a voice to the nodes that serve it |
| `plain.go` | `Plain`, which strips WhatsApp and Markdown formatting and emojis before the text is spoken |
| `speech.go` | `Speech`, `Normalizer`, `Speakable` and `NormalizerVersion`: the Brazilian Portuguese verbalizer every voice speaks through by default (dates, times, money, ranges, fitness notation, symbols, prosody), line by line in the order of the rules |
| `speech_numbers.go` | `NumberInWords`, `NumberInWordsFeminine`, `OrdinalInWords`, `MoneyInWords` and the currencies |
| `speech_lexicon.go` | `LexiconEntry`, `BaseLexicon`, the units of measure and the feminine nouns |
| `docs/speech-ptbr.md` | the study behind the verbalizer, with its sources, the rules and choices, the measured accuracy, and the guideline an application pastes into the system prompt of a model whose answer will be spoken |
| `fake.go` | `Fake`, the scripted `Speaker` for an application's tests |
| `docs/sitecustomize.py` | the shim that limits onnxruntime's threads and turns its telemetry off, for the image of an application |
| `docs/Dockerfile.snippet` | the build and runtime stages that put Piper and a voice in an application's image; it was built and run, keep it that way when a digest or version changes |
| `docs/kokoro-speak.py` | the program the Kokoro voices run: text on stdin, WAV out, `kokoro-onnx` inside; its command line is the contract with `synthesizeKokoro` |
| `docs/kokoro-requirements.txt` | its packages, pinned by sha256 (`uv pip compile --universal --python-version 3.11 --generate-hashes`) |
| `docs/Dockerfile.kokoro.snippet` | the same for Kokoro and its model; built and run with `TestRealKokoro` inside, keep it that way when a digest or version changes |
| `tests/Unit/service_test.go` | the handler, the remote (rotation, failover, cooldown, key, health) and the two together |
| `tests/Unit/kokoro_test.go` | the Kokoro voices against a stand-in `kokoro-speak`, an `.npz` voices file and a stand-in `ffmpeg`, the boot checks, the key, and the real program when `KOKORO_BIN`, `KOKORO_MODEL` and `KOKORO_VOICES` are set |
| `tests/Unit/voices_test.go` | the list of voices a handler gives and the routing of a voice over nodes that serve different ones |
| `tests/Unit/piper_test.go` | the client against stand-in `piper` and `ffmpeg` scripts, against the real ffmpeg when present, against the real Piper when `PIPER_BIN` and `PIPER_MODEL` are set, and the fake |
| `tests/Unit/voice_test.go` | several voices in one client, `ParseVoice`, `VoicesIn` and what changes the key |
| `tests/Unit/cache_test.go` | the reuse, a failing store, identical calls at the same time |
| `tests/Unit/plain_test.go` | `Plain` |
| `tests/Unit/speech_test.go` | every rule of the verbalizer, table by table, with real fitness and finance sentences, and that `Speakable` of a spoken text is itself |
| `tests/Unit/speech_domain_test.go` | the finance lexicon and the cases of Corujão.ai's own tests, through the module |
| `tests/Unit/speech_client_test.go` | the client: both engines get the speakable text, `pt-br`, `Raw`, the key, the limit |
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
  `-f`, and optionally `-s`, `--length_scale`, `--noise_scale`, `--noise_w` and
  `--sentence_silence`, with the
  text on standard input. They are accepted by the `piper` of the `piper-tts`
  package (OHF-Voice/piper1-gpl, GPL-3.0) and by the standalone binary of
  rhasspy/piper (MIT, archived); do not add `-q`, which only the second knows.
  Check both against the release in use before changing them.
- **A voice belongs to one engine, by its name** (`Voice.Engine`): the voices of
  `KokoroVoices` run `kokoro-speak`, any other name runs Piper. The Kokoro
  program takes the same care as Piper's: the text only on standard input, every
  other value one argument that `New` has checked, no inherited environment,
  and a failure that prints a class name, never a message that could quote the
  text. Whatever new setting reaches the `kokoro-speak` command line goes into
  its key too, and what does not change the audio (threads, environment) stays
  out. Kokoro's runtime is `kokoro-onnx` and the reasons are in the README: do
  not swap it for `sherpa-onnx`, which takes the text as an argument.
- **The voice decides the engine.** The `pt_BR` voices that are licensed for
  commercial use were retrained in 2026 and need `piper-tts` 1.5 or later; the
  standalone binary fails on them with "Phonemes must be one codepoint". The
  voices it does run are derived from the Lessac corpus, whose license excludes
  commercial use. Say so wherever a voice is recommended.
- **What a voice says is decided by `Speakable`, in code.** A number, a date or an
  amount of money is never left to the model that wrote the answer, nor to
  espeak-ng. A change to a rule that changes what a text is turned into raises
  `NormalizerVersion`, gets a row in the table of `tests/Unit/speech_test.go` and
  a line in `docs/speech-ptbr.md`; every voice of this package speaks Brazilian
  Portuguese, and Kokoro is called with `pt-br`, never `pt`. The verbalizer reads
  no clock (a date is relative only when the caller passes `now`) and imports
  nothing but the standard library.
- **No framework import and no third-party dependency.** `go.mod` has no
  `require`. If one is ever needed, the `framework` floor in `arandu.mod.toml`
  moves with it and every installer is told in `UPGRADE.md`.
- **A test never needs Piper or a voice.** It uses the stand-in scripts or the
  `Fake`; the one test that runs the real program skips itself with the reason.

## Commits

The identity is set in this checkout: `HYZIS <admin@hyz.is>`. No AI attribution
of any kind. Work goes to `develop`, and releases are tags on `main`.
