# Changelog

Everything worth knowing about a release of Piper is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

## [0.6.0] - 2026-11-26

### Added

- **The phonetic layer of the verbalizer.** What a voice says is now also written the way a Brazilian says it, in the spelling, because neither engine takes SSML. Acronyms are spelled with the names of the letters ("GPU" is "gê pê u", "ALPR" "á éle pê érre", "IMC" "i ême cê") or said as a word ("NASA", "SaaS", "JSON", "PIX", "HIIT"), by the dictionary first and by the shape of the syllables when the dictionary does not know them (`ReadsAsWord`); a plural of a spelled acronym adds "s". Domains, e-mails and short paths are said out ("alpr.sh" is "á éle pê érre ponto ésse agá", "ajuda@ctrl.fit" "ajuda arroba cê tê érre éle ponto fít", "exemplo.io/login" "exemplo ponto i ôh barra lóguin"); an address that cannot be dictated stays "o link". Foreign words and brands are written as they are said ("whey", "crossfit", "check-in", "feedback", "framework", "webhook", "Wi-Fi", "WhatsApp"). Identifiers are dictated digit by digit and letter by letter: plates (`ABC1D23`), IPv4 addresses, versions (`V2.1`), protocols and codes after "protocolo", "código", "pedido", toll-free (`0800 123 4567`) and international telephones. `24/7` is "vinte e quatro horas por dia, sete dias por semana" (a date after "em", "dia", "até"), `D+1` is "dê mais um", and a range of hours is "das oito às dezessete horas". New units: `GB MB KB TB Gbps Mbps FPS Hz kHz MHz GHz V W kW kWh mAh`; bytes are said "guigabáite", "megabáite". New abbreviations and symbols: `Ltda.`, `S.A.`, `Cia.`, `Av.`, `Ex.`, `§`, `§§`, `#meta` (hashtag), a lone `#` (cerquilha). `US$` is "dólares americanos".
- **A shared dictionary**, `lexicon/ptbr.json`, embedded in the module: the pronunciation of the acronyms, documents, products, brands and borrowed words every product says the same way. `LexiconJSON()` gives the file (for a program that is not written in Go), `LexiconVersion()` its version, `ParseLexicon(data)` reads a file of the same format (a string, or `{"said": "...", "case": "any" | "exact"}` per term, the longest term first) into the `[]LexiconEntry` that `Speech.Lexicon` takes. `BaseLexicon()` is now this file.
- **`Validate` and `Normalizer.Check`**: the validator stage. `Validate(display, speech)` (and `Normalizer.Validate`) compares the text a person reads with the text a voice says and reports, as `[]Issue` (`Kind`, `Severity`, `Text`, `Detail`), every number, date, hour, identifier, unit, currency or address of the first that was lost or changed in the second (an error), and what can be read two ways or was guessed (a warning); `HasErrors` is the test a caller uses to refuse the audio. `Normalizer.Check(text, now)` makes the speech text and validates it in one step.
- `LetterName`, `SpellAcronym` and `ReadsAsWord`: the names of the letters in Brazilian Portuguese, the spelling of a sequence of letters and digits, and the decision between spelling a token and saying it as a word.
- Section 4 of `docs/speech-ptbr.md` is the 17-rule guideline for the system prompt of a model whose reply will be spoken, with the measurement of this release in section 5.

### Fixed

Five defects of 0.5.0, reported by the Corujão.ai session with examples, each with its own test in `tests/Unit/speech_defects_test.go`:

1. **"dia" said twice before a date.** `"vence dia 20/12"` was spoken "vence dia dia vinte de dezembro" (also `dia 1/12`, `em dia`, `no dia 05/12/2026`): a date counted from `now` is said with its own "dia", and the one already written was said as well. The date no longer repeats it.
2. **An installment was read as a date.** `"Parcela 3/12"` was "parcela três de dezembro". After "parcela", "prestação" or "mensalidade" (or before "parcelas"), with an optional "nº", and in a list ("parcelas 10/12 e 11/12"), `x/y` with `x <= y <= 48` is now "três de doze"; anything else is a date or a fraction as before. `Validate` takes it for two numbers and not for a date.
3. **The lexicon of the application did not reach into addresses.** E-mails and domains were said before the lexicon ran and by terms in lower case, so `"ajuda@corujao.ai"` never spoke "corujão" for a lexicon that has "Corujão". The index of the lexicon now matches without accents and in any case, and it is asked for the domain, each of its names and each name of the part before the arroba (`login@...` is "lóguin"); the application's entries still come first.
4. **A relative date said twice.** `"ontem, 26/11"` was "ontem, ontem" (also `hoje, 27/11`, `amanhã, 28/11`, `ontem 26/11`, `ontem (26/11)`): when the word that the date would become is already next to it, the date is dropped.
5. **Words missing from the shared dictionary:** `app` ("áp"), `login` ("lóguin") and `checkout` ("tchék aut") were already in it; `e-mail` is now "imêil" and `link` "linque" (it was "i mêiu" and "línk", and "o link" for an address that cannot be said is "o linque").

### Changed

- `NormalizerVersion` is `"ptbr-2.lex1"`: the number of the rules and the version of the dictionary. `Normalizer.Version()` still adds a hash of the application's lexicon. The speech of a text with an acronym, a domain, an e-mail, an identifier, a range of hours, a unit of data or a foreign word changed, and so did the `Key` of its audio. See `UPGRADE.md`.
- `BaseLexicon()` holds more terms and is ordered longest term first; a spelled acronym is said with the names of the letters ("cê pê éfe", "i ême cê", "érre pê êh") and not with the letters of an older spelling ("cê pê efe", "érre pê é"). Entries of an application's `Speech.Lexicon` still run first and win.
- An e-mail's domain is said as a domain ("hyz ponto i ésse") and not letter by letter of its words.
- An address is said when it is a short domain with at most two plain words of path; before, every address was "o link".
- Measured with both engines and whisper.cpp `small` on 21 sentences (78 hard items): the word error of Kokoro `pf_dora` fell from 25.2% (0.5.0) to 11.4% and that of Piper `faber` from 22.5% to 16.8%; the table, and what is not solved, are in section 5 of `docs/speech-ptbr.md`. Among the findings, espeak-ng **spells out a lone accented vowel** ("í" is read "i agudo", "ê" "e circunflexo"), so the names of the letters i, u, e and o are written "i", "u", "êh" and "ôh"; a test holds that no output is one of those four letters on its own.

## [0.5.0] - 2026-11-26

### Added

- **A Brazilian Portuguese verbalizer**, standard library only, deterministic and applied by default before synthesis by both engines: `Speakable(text)`, `Normalizer` (`NewNormalizer(Speech)`, `Normalizer.Speakable(text, now)`, `Normalizer.Version()`), `Speech`, `LexiconEntry`, `BaseLexicon()`, `NormalizerVersion`. It says cardinals and ordinals with gender and number agreement, decimals ("vírgula"), thousands separators, negatives, money (`R$ 24,90`, `US$`, `€`), times (`06:30`, `7h30`, paces and durations), dates (in full, or relative to a moment: "hoje", "amanhã", "dia vinte"), percent, ranges (`8-12`), fitness notation (`3x10`, `4x`), units (kg, g, km, m, cm, mm, kcal as "calorias", ml, l, km/h, bpm, min, h, s, kg/m², °C...), documents and telephones digit by digit, e-mails and links, abbreviations, symbols and acronyms; turns list items into sentences and parentheses into commas; drops formatting and emoji; collapses repeated marks; gives a long sentence a breath; and can cut a long text at a sentence. The research, the rules, the choices and a guideline for the system prompt of a model whose answer will be spoken are in `docs/speech-ptbr.md`.
- `Config.Speech` (`Speech`: `Raw`, `Lexicon`, `MaxWords`, `RestNote`, `SkipLinePrefixes`, `KeepCapitals`). The application's lexicon is applied before the base one and wins; it touches only the spoken text.
- `Client.Speakable(text)`, `Client.Normalizer()` and `Client.NormalizerVersion()`: the text a call would say, the normalizer, and the version of its rules, to store beside the text a voice note was made from.
- `NumberInWords`, `NumberInWordsFeminine`, `OrdinalInWords`, `MoneyInWords` and `SayDay`, the pieces of the verbalizer a caller may want on their own.

### Changed

- `Speak`, `SpeakAs` and `Key` of a `*Client` make the text speakable before they speak or hash it, unless `Config.Speech.Raw` is set. The audio of any text with numbers, symbols, formatting, emoji or no final punctuation changes, and so does its key (the key hashes what is said). The limit `MaxChars` is checked on the text as given, before the rules run, and the spoken text may be up to four times as long. `Plain` is unchanged. See `UPGRADE.md`.

## [0.4.0] - 2026-11-26

### Added

- The **Kokoro engine**, inside this package: [Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M) (Apache-2.0) for the voices Piper does not have. `Config.Kokoro` (`KokoroConfig`: `ProgramPath`, `ModelPath`, `VoicesPath`, `Voices`, `Threads` (default 2, at most 16), `Env`) turns it on; the zero value leaves the client with Piper only, as before. A voice belongs to an engine and the voice a call names decides which one speaks, so one client holds Piper voices and Kokoro voices together (`Voice.Engine`, `EnginePiper`, `EngineKokoro`).
- The voices `VoiceDora` (`pf_dora`, female), `VoiceAlex` (`pm_alex`) and `VoiceSanta` (`pm_santa`), `KokoroVoices`, and `ParseVoice` reading `dora`, `alex`, `santa` and the Kokoro names.
- `docs/kokoro-speak.py`, the program the Kokoro voices run: the text on standard input (never in an argument), a mono 24 kHz WAV out, `kokoro-onnx` 0.6.1 (MIT) as the runtime, the threads of onnxruntime set from `--threads`, onnxruntime telemetry off, and an error line that never carries the text. `docs/kokoro-requirements.txt` pins its packages by sha256 and `docs/Dockerfile.kokoro.snippet` builds an image with them and the model (checked by sha256); it was built, and `TestRealKokoro` was run inside it as a non-root user with two CPUs, on arm64.
- The same guarantees as the Piper path: a missing program, model, voices file or voice fails at boot (`New` opens the voices file and checks every voice is in it), no inherited environment, a private directory per call, the timeout kills the process, the text limit is checked first, and errors never carry the text. `Config.LengthScale` (the pace is `1/LengthScale`) and `Config.SentenceSilence` apply to the Kokoro voices; `NoiseScale` and `NoiseW` are Piper's. A Kokoro key hashes the voice, the model's size and the voices file, the pace, the pause, the format and the text, and nothing else.
- `Handler` answers a `GET` with a valid token on its path with `{"voices": [...], "default": "..."}`, when its `Speaker` is a `VoiceLister` (a `*Client` is; a `Fake` and a `Cache` are not and answer `{}`). Without a token the answer is the 405 it always was.
- `Remote.Voices(ctx)` reads the voices of every node, in parallel, and remembers them for `RemoteConfig.VoiceTTL` (default `DefaultVoiceTTL`, 5 minutes): a call for a voice goes only to the nodes that serve it. `NodeVoices` is what it reports.
- `TestRealKokoro`, behind `KOKORO_BIN`, `KOKORO_MODEL` and `KOKORO_VOICES` (and `KOKORO_ENV`, `KOKORO_FFMPEG`, `KOKORO_SAMPLES`).

### Changed

- `Remote` no longer returns at the first node that answers `unknown_voice`: nodes may serve different voices, so it asks the next node, remembers the refusal for `VoiceTTL`, and fails with `ErrUnknownVoice` only when no node serves the voice, naming each node in the message. A node that is down while the others refuse makes the error `ErrNoNodes`, since the voice may be on that node. With identical nodes (the case until now) a wrong voice costs one request to each node, once per `VoiceTTL`.
- `Config.BinaryPath` and `Config.Voices` are not required when `Config.Kokoro` is set and neither of them is: the client then speaks with Kokoro only. Any other configuration is checked exactly as before, and a Kokoro voice in `Config.Voices` is refused with the reason.

## [0.3.2] - 2026-11-25

### Fixed

- `Remote` did not wait for `BusyWait` when a node of the round had failed (a node that was stopped, for instance) beside the busy ones: it gave up at once with `ErrNoNodes` although the busy node would free in a moment. It now repeats the round when at least one node was busy, whatever the others did; a round of failures alone is still not waited for.

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
