<p align="center">
  <img src=".github/logo.svg" alt="Arandu" width="180">
</p>

# Piper for Arandu

Text to speech on your own server. A reply goes in, a voice note comes out, and
nothing leaves the machine: it runs [Piper](https://github.com/OHF-Voice/piper1-gpl),
a fast neural text-to-speech engine that works on a CPU, so there is no
per-minute cost and no text sent to a third party. Since 0.4.0 it also runs
[Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M), a second engine with a
female Brazilian voice (`piper.VoiceDora`) and two male ones, in the same client:
a voice belongs to an engine, and the voice a call names decides which one
speaks. It is the counterpart of
[`arandu-whisper`](https://github.com/hyz-is/arandu-whisper), which turns speech
into text.

It is a thin layer over a program you install. `piper` (or, for the Kokoro
voices, `kokoro-speak`, a small program in `docs/`) makes a WAV from the text it
reads on its standard input; `ffmpeg`, optionally, turns that WAV into the
mono 16 kHz Ogg Opus that WhatsApp plays as a voice note. This package adds what
an application otherwise writes every time:

- a configuration refused at boot, so a missing program or voice fails at the
  first restart and not on the first reply;
- a limit on the text and a timeout that kills the running process;
- a private temporary directory per call, removed on every path;
- a program that does not inherit the application's environment, so its secrets
  are never handed to Piper;
- errors that carry the exit status and a short summary, never what was said;
- `Plain`, which removes the formatting of WhatsApp and Markdown and the emojis
  from a message, so a voice does not read "asterisk" aloud;
- `Speakable`, the Brazilian Portuguese verbalizer every voice speaks through by
  default: numbers, money, dates, times, units, acronyms and symbols in words,
  list items as sentences, a pronunciation lexicon you extend (see "Speech");
- several voices in one client, named by a type (`piper.VoiceFaber`,
  `piper.VoiceCadu`, and the Kokoro ones `piper.VoiceDora`, `piper.VoiceAlex`,
  `piper.VoiceSanta`), with a default one and a choice per call;
- `Key` and `Cache`, so an audio already made for the same words is reused
  instead of made again;
- the `Speaker` interface, with a `Fake`, so the code that speaks is tested
  without the program or a voice.

It owns no table, no route and no policy, and it does not import the framework:
an application on any framework version installs it. It has no third-party
dependency. It executes Piper and Kokoro and does not link them, so their
licenses (below) apply to the programs you install, and not to this package.

## Requirements

| what | notes |
| --- | --- |
| Piper | the `piper` program of the `piper-tts` package, 1.5 or later, with Python 3.9 or later; only for the Piper voices |
| one or more voices | an `.onnx` file and its `.onnx.json` each; see **Choose a voice** before taking one |
| Kokoro | only for the Kokoro voices: `docs/kokoro-speak.py`, Python 3.10 to 3.13 with the packages of `docs/kokoro-requirements.txt`, the model and the voices file; see **Kokoro voices** |
| ffmpeg | only for `FormatOpus`, built with libopus (the common static builds are) |

## Install

```bash
go get github.com/hyz-is/arandu-piper@<latest tag>   # gh release list -R hyz-is/arandu-piper
```

### Get Piper

Piper has two lines, and the choice matters because of the voices.

| | `piper-tts` (use this) | standalone binary |
| --- | --- | --- |
| source | <https://github.com/OHF-Voice/piper1-gpl>, `pip install piper-tts` | <https://github.com/rhasspy/piper>, release `2023.11.14-2` |
| license | GPL-3.0-or-later | MIT (the espeak-ng it bundles is GPL-3.0) |
| state | maintained (1.8.0 in September 2026) | archived in August 2025 |
| runs | every voice, including the 2026 ones | only the old voices, which descend from a non-commercial corpus |
| arm64 macOS | wheel for macOS 11 and later | the "aarch64" macOS release is an x86_64 build without its libraries and does not start |

The module's command line (`-m`, `-c`, `-f`, `-s`, `--length_scale`,
`--noise_scale`, `--noise_w`, `--sentence_silence`, text on standard input) is the part both accept, so
`BinaryPath` can be either. The license note below says why you will want the
first.

```bash
python3 -m venv /opt/piper-venv
/opt/piper-venv/bin/pip install piper-tts==1.8.0
# the program: /opt/piper-venv/bin/piper
```

In an image, install it with `pip install --target` and copy the directory, as
the Dockerfile below does; then give the program `PYTHONPATH` through
`Config.Env`.

### Choose a voice

Voices are single files from <https://huggingface.co/rhasspy/piper-voices>. The
Portuguese ones are in `pt/pt_BR/`. Take the voice from a fixed revision, not
from `main`, and keep its `MODEL_CARD` beside it.

**A voice is a model trained on recordings, and it inherits the licenses of
every recording in its chain**: its own dataset, and the dataset of the voice it
was fine-tuned from, and so on. The `License` line of a `MODEL_CARD` names only
the voice's own dataset. Until November 2026 `pt_BR-faber-medium`,
`pt_BR-cadu-medium` and `pt_BR-jeff-medium` were fine-tuned from
`en_US-lessac-medium`, whose corpus license allows research and excludes
commercial use. They were retrained in November 2026 from CC0 and CC BY bases
and keep their names, so **a copy downloaded earlier is the old one**. Check the
`Training` section of the `MODEL_CARD` of the file you have: it must not say
"lessac". The Piper project documents this in
[`docs/LICENSING.md`](https://github.com/OHF-Voice/piper1-gpl/blob/main/docs/LICENSING.md)
and [`docs/COMMERCIAL_USE.md`](https://github.com/OHF-Voice/piper1-gpl/blob/main/docs/COMMERCIAL_USE.md),
and says it is not legal advice. Neither is this README: have a lawyer read it
for a commercial product.

The three Brazilian Portuguese medium voices, at revision
`e9ab671f2a0e50a701b34361136ba3e6a3a9eb0c` of `rhasspy/piper-voices`
(22 050 Hz, one speaker, 60 MiB, `piper_version` 1.5.0). The voices repository
is itself tagged MIT; what governs a voice is its `MODEL_CARD` and its chain.

| voice | training chain | what it asks of you |
| --- | --- | --- |
| `pt_BR-faber-medium` | CC0 recordings, fine-tuned from Hi-Fi TTS reader 92 (CC BY 4.0) | credit the Hi-Fi TTS dataset |
| `pt_BR-cadu-medium` | CC0 recordings, fine-tuned from the CC0 `nl_BE-rdh` | nothing |
| `pt_BR-jeff-medium` | CC0 recordings, fine-tuned from the CC0 `nl_BE-rdh` | nothing |

| voice | sha256 of the `.onnx` | sha256 of the `.onnx.json` |
| --- | --- | --- |
| `pt_BR-faber-medium` | `75e913eb463db0b6ac7ddf68d3e4c9ac19ace36dc3b710a81039b448352b8ffc` | `fa8a8fecb01ae198bc416381e368d3206f85bad1449a0f11e1f6a9edf85a32f1` |
| `pt_BR-cadu-medium` | `d5a662e3f606d5a49dd29ba9ce90c304ae45dd9543f8b0c773bcb0de282499de` | `9fa837b7c1237ea5978bcd7be38c130798e8faa064f754873bb4ce5fc498bd88` |
| `pt_BR-jeff-medium` | `82ddcdb01a0342f078bd89ce2815eda27185818d15e15f06fb9a72ac3bce94e1` | `078ea4e42a680ca3cb662a8929237c32e2c0d581f5363f2f55f09bc266497bf6` |

On one sentence ("Pronto, seu Antônio. Anotei mil e duzentos reais de adubo no
talhão três. Está certo?") and four more, each voice was spoken by Piper and
transcribed back with whisper.cpp `small`: faber came back intact in four of the
five, cadu in three, jeff in none. That is a rough measure of how clear a
voice is, and it is not a judgement of how pleasant it sounds. Listen to the
voices, with the person who will hear them, before choosing.

```bash
REV=e9ab671f2a0e50a701b34361136ba3e6a3a9eb0c
BASE=https://huggingface.co/rhasspy/piper-voices/resolve/$REV/pt/pt_BR/faber/medium
curl -fLO $BASE/pt_BR-faber-medium.onnx
curl -fLO $BASE/pt_BR-faber-medium.onnx.json
curl -fLO $BASE/MODEL_CARD
echo "75e913eb463db0b6ac7ddf68d3e4c9ac19ace36dc3b710a81039b448352b8ffc  pt_BR-faber-medium.onnx" | sha256sum -c -
```

## Kokoro voices

[Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M) is a second engine, for
a voice Piper does not have: a female Brazilian Portuguese one. A voice belongs
to one engine, by its name, and a client holds voices of both:

| voice | engine | |
| --- | --- | --- |
| `piper.VoiceDora` (`pf_dora`) | Kokoro | female |
| `piper.VoiceAlex` (`pm_alex`) | Kokoro | male |
| `piper.VoiceSanta` (`pm_santa`) | Kokoro | male |
| `piper.VoiceFaber`, `piper.VoiceCadu`, any catalogue name | Piper | male |

`ParseVoice` reads `dora`, `alex` and `santa` (and `pf_dora`...) from
configuration, as it reads `faber` and `cadu`. `voice.Engine()` says which engine
speaks a voice.

**How it runs.** Like Piper: a program installed on the host or in the image,
started as a child process with no shell, no inherited environment, the text on
standard input, a private directory per call, the timeout killing it, and the same
limits and errors that never carry what was said. The program is
[`docs/kokoro-speak.py`](docs/kokoro-speak.py), about 100 lines, which reads the
text from standard input, runs the model through
[kokoro-onnx](https://github.com/thewh1teagle/kokoro-onnx) and writes a mono
24 kHz WAV; `ffmpeg` then makes the Opus note as for Piper. The WAV goes back
through the same reader, so `Audio.Duration` is exact.

**Why that runtime.** The choice was made by running the sentence of the next
section through each candidate and transcribing it back with whisper.cpp:

- `kokoro-onnx` 0.6.1 (MIT): reads Portuguese correctly through espeak-ng
  (`pt-br`), is the reference implementation of Kokoro's tokenizer, puts the
  pauses between sentences itself, and lets this package set the threads of
  onnxruntime directly. Chosen.
- `sherpa-onnx` (Apache-2.0), whose `sherpa-onnx-offline-tts` is a ready binary:
  it takes the text as an **argument**, which would show every message in the
  process list and break this package's rule, and its 2.x releases no longer send
  Portuguese through espeak-ng (the words come out "OOV word skipped" and the
  audio is a third of the length). 1.13.8 and earlier work, but a program pinned
  to a release before a regression is the worse choice of the two.

**What it asks of the machine.** Python 3.10 to 3.13 and the packages of
`docs/kokoro-requirements.txt`, pinned by sha256 (kokoro-onnx, onnxruntime, numpy
and the espeak-ng library and data in the `espeakng-loader` wheel), plus two
files from the `model-files-v1.0` release of kokoro-onnx, which are
Kokoro-82M v1.0 converted to ONNX:

| file | size | sha256 |
| --- | --- | --- |
| `kokoro-v1.0.onnx` (fp32, the reference) | 326 MB | `7d5df8ecf7d4b1878015a32686053fd0eebe2bc377234608764cc0ef3636a6c5` |
| `kokoro-v1.0.int8.onnx` (int8) | 92 MB | `6e742170d309016e5891a994e1ce1559c702a2ccd0075e67ef7157974f6406cb` |
| `voices-v1.0.bin` | 28 MB | `bca610b8308e8d99f32e6fe4197e7ec01679264efed0cac9140fe9c29f1fbf7d` |

```bash
python3 -m venv /opt/kokoro-venv
/opt/kokoro-venv/bin/pip install --require-hashes --no-deps -r docs/kokoro-requirements.txt
# the program: docs/kokoro-speak.py with its first line set to #!/opt/kokoro-venv/bin/python
sed '1s|.*|#!/opt/kokoro-venv/bin/python|' docs/kokoro-speak.py > /opt/kokoro-venv/bin/kokoro-speak
chmod 0755 /opt/kokoro-venv/bin/kokoro-speak
```

In an image, `docs/Dockerfile.kokoro.snippet` does it with `pip --target`, on
the same `distroless/python3-debian12` as Piper, so one image can hold both.

**Speed and memory, measured** (the sentence of 8 seconds below, mono Opus note,
two threads; Apple M4 Max for the first column, the module's real test inside the
distroless image with `--cpus 2`, arm64, for the second):

| | this Mac | the image |
| --- | --- | --- |
| Kokoro fp32 | 2.9 s | 3.3 s |
| Kokoro int8 | 1.5 s | not measured |
| Piper `faber` | 0.4 s | not measured |

Kokoro is about seven times slower than Piper on the same machine, and each call
loads the model again and holds it while it speaks: the peak memory of one call
measured 690 MB with the fp32 model and 460 MB with int8, so a handler with
`MaxConcurrent` 2 needs about 1.4 GB. It suits a voice note after a reply, from a
job, not a call that waits on the line. Set `Timeout` from a measured run on the
server (a Xeon E5-2667 v4 is slower than an M4). The int8 model halves the time
and takes a third less memory; **listen to it before choosing it**.

The pt-BR voices have the thinnest training data of Kokoro's catalogue (the
model's `VOICES.md` gives them no grade). Transcribed back with whisper.cpp
`small`, the sample sentence came back whole from `pm_alex`, from `pf_dora` (with
"polichinellos", a spelling of the same word) and from the int8 `pf_dora`, and
`pm_santa` turned "dez polichinelos" into "despole chinelos". That measures how
clear a voice is, not how pleasant it sounds: listen to each with the person who
will hear it.

```go
c, err := piper.New(piper.Config{
    Kokoro: piper.KokoroConfig{
        ProgramPath: cfg.Kokoro.Bin,    // KOKORO_BIN, e.g. /opt/kokoro/bin/kokoro-speak
        ModelPath:   cfg.Kokoro.Model,  // KOKORO_MODEL
        VoicesPath:  cfg.Kokoro.Voices, // KOKORO_VOICES
        // Voices:   []piper.Voice{piper.VoiceDora}, // default: all three
        // Threads:  2,                               // default 2, at most 16
        Env: []string{"PYTHONPATH=" + cfg.Kokoro.PythonPath}, // when installed with pip --target
    },
    DefaultVoice: piper.VoiceDora,
    Format:       piper.FormatOpus,
})
```

With `Kokoro` set and no `BinaryPath` or `Voices`, the client speaks with Kokoro
only. With both, it holds the Piper voices and the Kokoro ones, and
`SpeakAs(ctx, piper.VoiceDora, text)` and `SpeakAs(ctx, piper.VoiceCadu, text)` go
to different programs. `New` refuses, at boot, a missing program, model or voices
file, a voices file that is not Kokoro's, a voice that is not in it, and a
`Threads` out of range. `LengthScale` (the pace is `1/LengthScale`) and
`SentenceSilence` apply to the Kokoro voices; `NoiseScale` and `NoiseW` are
Piper's and do not reach Kokoro, nor do they change a Kokoro key.

## Wire it

The paths come from the application's own configuration; this package never
reads the environment. One client holds every voice it is given; `DefaultVoice`
is the one `Speak` uses, and `SpeakAs` picks another per call.

```go
// bootstrap/app.go
var voice piper.Speaker
if cfg.Piper.Configured() {
    def, err := piper.ParseVoice(cfg.Piper.Voice) // PIPER_VOICE: "faber", "cadu" or a catalogue name
    if err != nil {
        return App{}, err
    }
    c, err := piper.New(piper.Config{
        BinaryPath: cfg.Piper.Binary, // PIPER_BIN, e.g. /opt/piper/bin/piper
        // PIPER_VOICES_DIR, e.g. /opt/voices, holding pt_BR-faber-medium.onnx,
        // pt_BR-cadu-medium.onnx and their .onnx.json.
        Voices:       piper.VoicesIn(cfg.Piper.VoicesDir, piper.VoiceFaber, piper.VoiceCadu),
        DefaultVoice: def,
        Format:       piper.FormatOpus, // a WhatsApp voice note; the default is FormatWAV
        FFmpegPath:   cfg.Piper.FFmpeg, // empty means "ffmpeg" in PATH
        // LengthScale: 1.15,           // slower than the voices' own pace
        Env: []string{"PYTHONPATH=" + cfg.Piper.PythonPath}, // when installed with pip --target
    })
    if err != nil {
        return App{}, err
    }
    voice = c
}
assistant := services.NewAssistant(db, voice) // takes a piper.Speaker
```

A voice whose files do not keep their catalogue names goes in the map by hand:
`Voices: map[piper.Voice]piper.VoiceFiles{piper.VoiceCadu: {ModelPath: "...", ConfigPath: "..."}}`.
With a single voice `DefaultVoice` may stay empty.

Add `PIPER_BIN`, `PIPER_VOICES_DIR`, `PIPER_VOICE` and, if used,
`PIPER_PYTHONPATH` to `.env.example`. There is no migration, no route and no
`k.Register` line.

## Use it

```go
audio, err := voice.Speak(ctx, piper.Plain(reply))                     // the default voice
audio, err = voice.SpeakAs(ctx, piper.VoiceFaber, piper.Plain(reply))  // a chosen one
switch {
case errors.Is(err, piper.ErrUnknownVoice):  // a voice the client was not given
case errors.Is(err, piper.ErrEmptyText):     // nothing speakable: send the text
case errors.Is(err, piper.ErrTextTooLong):   // over Config.MaxChars: shorten or split
case errors.Is(err, piper.ErrTimeout):       // Config.Timeout elapsed; the process was killed
case errors.Is(err, piper.ErrFailed):        // piper failed: errors.As(err, &perr)
case errors.Is(err, piper.ErrConvertFailed): // ffmpeg could not make the Opus file
case errors.Is(err, piper.ErrBadOutput):     // piper wrote nothing readable
case err != nil:                             // context cancelled, disk
}
// audio.Data is the file, audio.MIMEType "audio/ogg; codecs=opus" (or "audio/wav"),
// audio.Duration the length of the speech, which is what a voice note's header needs.
```

`Speak` makes the text speakable itself (see "Speech" below), so `Plain` is
only needed when the application wants the formatting removed and nothing else.
It removes `*bold*`, `_italic_`, `~strikethrough~`, `` `code` `` and the `**double**`
forms, the markers of headings, quotes, bullets and numbered items, web
addresses, emojis and arrows, keeps the words of a `[link](address)`, and makes
every line a sentence, so list items are separated by a pause. It does not spell
out numbers, money or abbreviations.

A text is read straight through: line breaks become spaces, control characters
are dropped, and the limit counts characters (`MaxChars`, 1500 by default, about
a minute and a half of speech). For a longer answer, speak a summary and send
the whole text.

A call starts Piper, which loads the voice from disk each time: about half a
second on a laptop and the first call after boot slower. It is CPU bound, so call
it from a job and decide how many may run at once. Set `Timeout` from a measured
run on the server.

Do not log the text, the audio or a `ProcessError`'s fields beyond what you need:
they can carry personal data. The package itself never logs.

## Speech: what is said is not what is written

A voice reads the spelling through espeak-ng, and neither engine takes SSML or
documents any text normalization: digits are read as they can be, with no
agreement ("dois séries"), no units, no money, no times and no dates, and
"3x10", "R$ 228,00" or "50 min" come out wrong. So `Speak` and `SpeakAs` first
run the text through `Speakable`, a deterministic verbalizer for Brazilian
Portuguese written with the standard library only: cardinals and ordinals with
agreement ("duas séries", "um quilo"), decimals, money, times, dates, percent,
ranges, fitness notation (`3x10`), units, documents, acronyms, abbreviations,
symbols, and the punctuation the engines turn into pauses. The rules, the sources
they come from and the measured effect are in
[docs/speech-ptbr.md](docs/speech-ptbr.md): with the raw text the words heard were
65% to 75% right on six real sentences, and 93% to 94% after it.

```go
piper.Speakable("Próximo treino: supino 3x10 com 42,5kg às 06:30.")
// "Próximo treino: supino três séries de dez com quarenta e dois vírgula cinco quilos às seis e meia."
piper.Speakable("Plano anual R$ 228,00 (R$ 19,00/mês).")
// "Plano anual duzentos e vinte e oito reais, dezenove reais por mês."
```

It is on by default for every voice of the package, all of which speak Brazilian
Portuguese; Kokoro is always called with `pt-br`, never the European `pt`. Set
`Config.Speech` to adapt it, or `Config.Speech.Raw` to speak the text as given
because the application has already made the spoken text:

```go
n := piper.NewNormalizer(piper.Speech{
    Lexicon:          []piper.LexiconEntry{{Written: "supino", Said: "supíno", AnyCase: true}}, // your domain
    MaxWords:         75,                                      // cut a long answer at a sentence...
    RestNote:         "O resto eu deixei por escrito.",        // ...and say so
    SkipLinePrefixes: []string{"fonte:"},
})
spoken := n.Speakable(reply, time.Now()) // dates as people say them: "hoje", "amanhã", "dia vinte"
store(reply, spoken, n.Version())        // the original, what was said, and by which rules
```

- The base lexicon (`BaseLexicon()`) holds the acronyms and borrowed words that
  are said the same in every product (CPF, CNPJ, IRPF, INSS, FGTS, MEI, PIX, IMC,
  RPE, RIR, WhatsApp...); the application's `Speech.Lexicon` is applied first and
  wins. The lexicon touches the spoken text only, never the chat text.
- `NormalizerVersion` (and `Normalizer.Version()`, which adds a hash of the
  lexicon and options when there are any) names the rules. Store it with the
  spoken text; for a `Remote` put it in `RemoteConfig.KeyTag`.
- `Key` hashes the spoken text, so it changes exactly when what is said changes.
- `Client.Speakable(text)` is the text a call would say; `Client.Normalizer()` is
  the normalizer, to count dates from a moment of your own.
- `MaxChars` counts the text as given; the spoken text may be up to four times as
  long.
- To make an assistant write text that is easy to hear, paste the guideline of
  section 4 of `docs/speech-ptbr.md` into its system prompt.

## Reuse audio

An assistant says some sentences every day: "Anotado. Está certo?", the
greeting, the help. `Cache` makes each audio once. Before speaking it asks a
`Store` for the audio of the same key; after speaking it saves what was made.
Identical calls that arrive while an audio is being made wait for that one, so
the CPU makes it once and the bucket keeps it once.

```go
speaker := piper.NewCache(client, store) // client is a *piper.Client, store your Store
res, err := speaker.Say(ctx, piper.VoiceCadu, piper.Plain(reply))
// res.Audio   the speech
// res.Key     its name, 64 hexadecimal characters
// res.Reused  true when it came from the store (or from an identical call running at the same moment)
// res.StoreErr set when the store failed; the audio is still good, log it if you want
```

`*Cache` is itself a `Speaker`, so it goes wherever the client went.

**What counts as the same audio.** `Key(voice, text)` is a hash of everything
that changes the sound: the voice and its files (a new version of a voice is a
new key), the speaker, the pace, the pauses, the format and its bit rate, and
the text with its spacing and line breaks cleaned. Case and punctuation are
kept, as they change how the voice reads. **Only the same words reuse an
audio.** A reply that means the same thing with another number or another name
is another audio: reusing it would say something the text does not say, and a
listener who does not read would never notice. To reuse more, write the fixed
sentences as fixed sentences, and say the figures in a sentence of their own.

**The Store is yours.** The package keeps nothing and decides nothing about
where; it calls two methods:

```go
type Store interface {
    Find(ctx context.Context, key string) (piper.Audio, bool, error)
    Save(ctx context.Context, r piper.Recording) error // r.Key, r.Voice, r.Text, r.Audio
}
```

`MemoryStore` keeps them in memory, for tests and short-lived processes. In an
application, keep the audio as an object in its private bucket (Cloudflare R2
in the HYZIS projects) under `Key`, and a row in a table of its own:

| column | |
| --- | --- |
| `key` | primary key, `Recording.Key` |
| `voice` | `Recording.Voice`; empty is the default voice |
| `text` | `Recording.Text`, what was said |
| `object` | the object's name in the bucket |
| `mime_type`, `duration_ms`, `size` | from `Recording.Audio` |
| `created_at`, `last_used_at`, `uses` | `Find` updates the last two on a hit |

`Find` reads the row and the object; `Save` writes the object, then the row.
Serve the audio through the application, never from a public bucket address.
The text is what the person heard: if it can carry personal data (a name, a
figure of their farm), give the rows an expiry by `last_used_at` and remove
them with the person's data when they ask for erasure. A sentence that is
never repeated gains nothing from being kept.

## Test with it

```go
fake := piper.NewFake(
    piper.Answer{},                              // a short silent WAV
    piper.Answer{Err: piper.ErrTimeout},
)
svc := services.NewAssistant(db, fake)
// ... exercise svc ...
fake.Requests()[0]     // the text it was asked to say
fake.Calls()[0].Voice  // and the voice; empty is the default one
```

`*Fake` also has `Key`, so `piper.NewCache(fake, piper.NewMemoryStore())` tests
the reuse without Piper.

## As a service

Speech can be made on a server of its own and requested over HTTP by the
applications that need it.

**A handler.** `NewHandler` serves any `Speaker`:

```go
h, err := piper.NewHandler(piper.HandlerConfig{
    Speaker:       client,              // a *piper.Client with Format: piper.FormatOpus
    Tokens:        map[string]string{"corujao": os.Getenv("VOICE_TOKEN_CORUJAO")}, // a client per token
    MaxConcurrent: 2,                   // the rest wait QueueWait, then get 429
    Logger:        slog.Default(),      // client, status, characters, time: never the text or the audio
})
mux.Handle("/speak", h)
```

`POST /speak` with `Authorization: Bearer <token>` and `{"text": "...", "voice": "pt_BR-cadu-medium"}`
(the voice is optional). The audio is the body, with its MIME type as
`Content-Type` and its length in `X-Audio-Duration-Ms`. Errors are
`{"error": code, "message": text}` with `400 empty_text`, `unknown_voice` or
`bad_request`, `401 unauthorized`, `413 text_too_long` or `too_large`, `429 busy`
(with `Retry-After`), `502 failed`, `bad_output` or `convert_failed` and
`504 timeout`. Tokens have at least 32 characters, are compared in constant time
and never logged. Nothing is kept: the client's private directory is removed
before the call returns.

**A remote client.** `NewRemote` is a `Speaker` (and `Keyed`, so `NewCache` works
over it) that calls one or more nodes running the handler, so the application
chooses local or remote by configuration:

```go
var speaker piper.Keyed
if nodes := cfg.Voice.Nodes; nodes != "" {            // VOICE_API_NODES=one=https://voz1.example,two=https://voz2.example
    list, err := piper.ParseNodes(nodes)
    if err != nil { return App{}, err }
    speaker, err = piper.NewRemote(piper.RemoteConfig{Nodes: list, Token: cfg.Voice.Token, KeyTag: "v1"})
    if err != nil { return App{}, err }
} else {
    speaker, err = piper.New(localConfig)
    if err != nil { return App{}, err }
}
cache := piper.NewCache(speaker, store)
```

Calls go to the nodes in turn. A node that is busy (429), unreachable, timed out
or failing is skipped at once and the next one is tried; after
`FailureThreshold` failures in a row (3) it leaves the rotation for `Cooldown`
(30 s), and when every node is out they are all tried as a last resort. A refusal
another node would repeat (empty or too long text, a wrong token) is returned as
it is, as the same `errors.Is` sentinel the local client returns.

**Nodes may serve different voices** (a node with Kokoro, a node with Piper
only). A node advertises its voices: `GET` on the handler's path with a valid
token answers `{"voices": ["pf_dora", ...], "default": "..."}` when its `Speaker`
is a `VoiceLister` (a `*Client` is). `Remote.Voices(ctx)` reads them from every
node, in parallel, and remembers them for `VoiceTTL` (5 minutes); call it at boot
and to refresh. A call for a voice then goes only to the nodes that serve it.
Without that list (an older node, or `Voices` not called) the node is asked in the
call, and when it answers `unknown_voice` the next node is tried and the refusal
is remembered for `VoiceTTL`. When no node serves the voice the call fails at once
with `ErrUnknownVoice` and a message naming each node ("no node serves the voice
"pf_dora" (n1 serves pt_BR-cadu-medium; n2 refused it)"), which is the fix:
install the engine on a node, or choose another voice. A node that is down, and
could be the one with the voice, makes it `ErrNoNodes` instead. When every node answers 429 the round is repeated for up to `BusyWait` (5 s) before the call fails. `Remote.Key` has no network: it hashes the voice, the cleaned text and
`KeyTag`, which you change when the nodes' voices or settings change.
`Remote.Health(ctx)` asks every node's `/_arandu/health`.

### Limit Piper's threads

The onnxruntime of the `piper-tts` wheel starts one thread per core it sees. On a 32-core host in a container limited to two CPUs that is a sentence of six seconds taking eight to synthesize; with two threads it takes about one and a half. `OMP_NUM_THREADS` does not reach it. Copy `docs/sitecustomize.py` into the directory `PYTHONPATH` names and pass `PIPER_ORT_THREADS=2` in `Config.Env`. The file also turns onnxruntime's telemetry off.

## Config

| field | default | |
| --- | --- | --- |
| `BinaryPath` | required, unless Kokoro only | `piper`: a path, or a bare name looked up in PATH |
| `Voices` | required (at least one), unless Kokoro only | the Piper voices: `map[piper.Voice]piper.VoiceFiles`; `piper.VoicesIn(dir, voices...)` builds it |
| `Kokoro` | off | `piper.KokoroConfig`, below; turns the Kokoro voices on |
| `DefaultVoice` | the only voice | required when the client holds more than one voice (of either engine); the voice of `Speak` |

Each `VoiceFiles`:

| field | default | |
| --- | --- | --- |
| `ModelPath` | required | the voice's `.onnx` file |
| `ConfigPath` | `ModelPath` + `.json` | must be JSON; its `num_speakers` is checked against `SpeakerID` |
| `SpeakerID` | 0 | the speaker of a multi-speaker voice |

Each `KokoroConfig` (all zero means off; a part set without the others is refused):

| field | default | |
| --- | --- | --- |
| `ProgramPath` | required | `kokoro-speak`: `docs/kokoro-speak.py` with its first line set to the interpreter |
| `ModelPath` | required | `kokoro-v1.0.onnx` (or the int8 one) |
| `VoicesPath` | required | `voices-v1.0.bin`; every voice served must be in it |
| `Voices` | `KokoroVoices()` | which Kokoro voices to serve |
| `Threads` | 2 | onnxruntime's threads for one call, 1 to 16 |
| `Env` | none | extra `KEY=VALUE`, after `Config.Env`: `PYTHONPATH=/opt/kokoro` |

The rest of `Config` applies to every voice (`NoiseScale` and `NoiseW` to the
Piper ones only):

| field | default | |
| --- | --- | --- |
| `LengthScale` | the voice's own | 0.25 to 4; above 1 is slower |
| `NoiseScale` | the voice's own (0.667) | above 0 to 2; the variation of the voice from phrase to phrase: lower is flatter, higher is livelier and less stable |
| `NoiseW` | the voice's own (0.8) | above 0 to 2; the variation in the length of each sound: higher breaks the metronome rhythm, too high stumbles |
| `SentenceSilence` | Piper's own, 0.2 s | the pause after each sentence |
| `Format` | `FormatWAV` | or `FormatOpus` |
| `FFmpegPath` | `ffmpeg` in PATH | only when `Format` is `FormatOpus` |
| `OpusBitrate` | 24000 | bits per second, 6000 to 128000 |
| `Timeout` | 1 minute | Piper and the conversion together |
| `MaxChars` | 1500 | longer text is `ErrTextTooLong` |
| `TempDir` | the system's | where each call makes its private directory |
| `Env` | none | extra `KEY=VALUE` for the program; it gets `PATH`, `LANG`, `PYTHONUTF8`, `HOME` and `TMPDIR` and nothing else of the application's |

## Docker

An Arandu application image is usually `gcr.io/distroless/static-debian12`. **Piper
cannot run there.** What each way of running Piper needs from the image:

| | needs | notes |
| --- | --- | --- |
| `piper-tts` | glibc 2.28 or later, libstdc++, libgcc_s, a Python 3.9+ interpreter with its standard library, and the packages numpy, onnxruntime, protobuf, flatbuffers, packaging and pathvalidate | its wheel brings `libespeak-ng` and `espeak-ng-data`; onnxruntime and numpy bring their own native libraries |
| standalone binary | glibc 2.17 or later, libstdc++, libgcc_s, libm, libpthread, libdl, librt | it brings `libonnxruntime` 1.14.1, `libespeak-ng`, `libpiper_phonemize` and `espeak-ng-data` beside it, found through `$ORIGIN`, so keep the directory whole |

`static-debian12` has none of these: the program fails with "no such file or
directory", which is the missing dynamic loader. `gcr.io/distroless/cc-debian12`
has glibc, libstdc++ and libgcc_s, which is all the standalone binary needs, but
no Python. **`gcr.io/distroless/python3-debian12:nonroot` has both** and is what
this package was run on: the Go application is a static binary and does not care
what it sits on. Both distroless images were run with the module's real test
(`piper-tts` on the Python image, the standalone binary on the `cc` one), on
arm64. The image is large, about 500 MB, because Python, numpy, onnxruntime and a
60 MiB voice are in it.

The snippet below is also in `docs/Dockerfile.snippet`. A build stage fetches and
verifies (sha256) the wheel and the voice, installs the wheel with `pip --target`,
and the runtime image only copies the result. It was built and run with the
module's test inside the final image. Replace the application stage with your
own and add ffmpeg if you use `FormatOpus`.

```dockerfile
# syntax=docker/dockerfile:1
# Piper in the image of an Arandu application.
#
# What this snippet does, and why it looks like this:
#
# * Piper is the "piper-tts" Python package (OHF-Voice/piper1-gpl): its wheel
#   carries the compiled phonemizer (espeak-ng) and asks pip for onnxruntime
#   and numpy. The `piper` program that pip writes is a script, so the runtime
#   needs a Python interpreter. gcr.io/distroless/python3-debian12 is that:
#   Debian 12 glibc, libstdc++ and Python 3.11, no shell, no package manager,
#   non-root. A distroless/static image cannot run Piper (no glibc, no
#   libstdc++, no Python), and distroless/cc has the libraries but not Python.
# * Everything is fetched and verified in a build stage: the wheel and the
#   voices are checked against a sha256 written here, so a changed file fails
#   the build instead of reaching production. Update the digests when you
#   upgrade the version or a voice.
# * The Python dependencies are pinned to the versions this was tested with
#   (constraints.txt, written in the build stage).
# * Nothing is installed in the runtime image by pip or apt: the packages are
#   copied from the build stage, and the application reaches them with
#   PIPER_BIN=/opt/piper/bin/piper and PYTHONPATH=/opt/piper (see the README
#   for the matching piper.Config).
# * The application's own stage and ffmpeg are yours: replace them. ffmpeg is
#   only needed for piper.FormatOpus and has to be built with libopus (the
#   static builds usually are); the image that already carries ffmpeg for
#   arandu-whisper has it.
#
# The sha256 of the wheel is chosen by TARGETARCH (amd64 and arm64). It was
# built and run on arm64; the amd64 digest is the one PyPI publishes.

FROM golang:1.26 AS app
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/app .

FROM debian:bookworm-slim AS piper
ARG TARGETARCH
# piper-tts 1.8.0, https://github.com/OHF-Voice/piper1-gpl/releases/tag/v1.8.0
ARG PIPER_TTS_VERSION=1.8.0
ARG PIPER_TTS_SHA256_AMD64=25b4d3f31ff70c8fa7151908e00aaa5650cbdf16bca8fcf21299f3941b89a7d3
ARG PIPER_TTS_SHA256_ARM64=3f60c1917de6d8e8033f395878ad3f88f6dfee88a8b05f98971a275f76a38484
# The voices, from a fixed revision of https://huggingface.co/rhasspy/piper-voices.
# Both are installed, and the application picks one with PIPER_VOICE (or per
# call with SpeakAs). pt_BR-faber-medium: CC0 recordings, fine-tuned from
# Hi-Fi TTS reader 92 (CC BY 4.0, which asks for attribution).
# pt_BR-cadu-medium: CC0 recordings, fine-tuned from the CC0 nl_BE-rdh; no
# attribution requirement.
ARG VOICE_REV=e9ab671f2a0e50a701b34361136ba3e6a3a9eb0c
ARG FABER_ONNX_SHA256=75e913eb463db0b6ac7ddf68d3e4c9ac19ace36dc3b710a81039b448352b8ffc
ARG FABER_JSON_SHA256=fa8a8fecb01ae198bc416381e368d3206f85bad1449a0f11e1f6a9edf85a32f1
ARG CADU_ONNX_SHA256=d5a662e3f606d5a49dd29ba9ce90c304ae45dd9543f8b0c773bcb0de282499de
ARG CADU_JSON_SHA256=9fa837b7c1237ea5978bcd7be38c130798e8faa064f754873bb4ce5fc498bd88

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl python3 python3-pip \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp/fetch
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) arch=x86_64;  sha="$PIPER_TTS_SHA256_AMD64" ;; \
      arm64) arch=aarch64; sha="$PIPER_TTS_SHA256_ARM64" ;; \
      *) echo "unsupported architecture $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    wheel="piper_tts-${PIPER_TTS_VERSION}-cp39-abi3-manylinux_2_17_${arch}.manylinux2014_${arch}.manylinux_2_28_${arch}.whl"; \
    curl -fsSL -o "$wheel" "https://github.com/OHF-Voice/piper1-gpl/releases/download/v${PIPER_TTS_VERSION}/${wheel}"; \
    echo "${sha}  ${wheel}" | sha256sum -c -; \
    printf '%s\n' numpy==2.4.6 onnxruntime==1.32.0 protobuf==7.36.2 flatbuffers==25.12.19 packaging==26.3 pathvalidate==3.3.1 > /tmp/constraints.txt; \
    pip install --no-cache-dir --break-system-packages --target /opt/piper -c /tmp/constraints.txt "./${wheel}"

RUN set -eu; \
    mkdir -p /opt/voices; \
    fetch() { \
      voice="$1"; path="$2"; onnx_sha="$3"; json_sha="$4"; \
      base="https://huggingface.co/rhasspy/piper-voices/resolve/${VOICE_REV}/${path}"; \
      curl -fsSL -o "/opt/voices/${voice}.onnx"       "${base}/${voice}.onnx"; \
      curl -fsSL -o "/opt/voices/${voice}.onnx.json"  "${base}/${voice}.onnx.json"; \
      curl -fsSL -o "/opt/voices/${voice}.MODEL_CARD" "${base}/MODEL_CARD"; \
      echo "${onnx_sha}  /opt/voices/${voice}.onnx"      >> /tmp/voices.sha256; \
      echo "${json_sha}  /opt/voices/${voice}.onnx.json" >> /tmp/voices.sha256; \
    }; \
    fetch pt_BR-faber-medium pt/pt_BR/faber/medium "$FABER_ONNX_SHA256" "$FABER_JSON_SHA256"; \
    fetch pt_BR-cadu-medium  pt/pt_BR/cadu/medium  "$CADU_ONNX_SHA256"  "$CADU_JSON_SHA256"; \
    sha256sum -c /tmp/voices.sha256

# The licenses go with the files: GPL-3.0 for Piper, the MODEL_CARD for each voice.
RUN set -eu; \
    mkdir -p /opt/licenses; \
    cp /opt/piper/piper_tts-*.dist-info/licenses/* /opt/licenses/ 2>/dev/null || true

FROM gcr.io/distroless/python3-debian12:nonroot
COPY --from=piper /opt/piper    /opt/piper
COPY --from=piper /opt/voices   /opt/voices
COPY --from=piper /opt/licenses /usr/share/licenses/piper
COPY --from=app   /out/app      /app
# COPY --from=ffmpeg /ffmpeg /usr/local/bin/ffmpeg   # only for piper.FormatOpus
ENV PIPER_BIN=/opt/piper/bin/piper \
    PIPER_VOICES_DIR=/opt/voices \
    PIPER_VOICE=cadu \
    PIPER_PYTHONPATH=/opt/piper
# distroless/python3 starts the interpreter; this image starts the application.
ENTRYPOINT ["/app"]
```

The application starts Piper itself, so the image needs no `CMD` for it. Point
the configuration at it: `PIPER_BIN=/opt/piper/bin/piper`,
`PIPER_VOICES_DIR=/opt/voices`, `PIPER_VOICE=cadu` (or `faber`), and
`Env: []string{"PYTHONPATH=/opt/piper"}`. Both voices are in the image, about
120 MiB together. Run the image as a non-root user,
as `:nonroot` does; Piper writes only to the call's private directory, so
`/tmp` must be writable or `TempDir` must point somewhere that is.

## Kokoro in the image

`docs/Dockerfile.kokoro.snippet` is the Kokoro counterpart of the Piper snippet
above: a build stage installs the packages with `--require-hashes`, fetches the
model and the voices file and checks their sha256, writes `kokoro-speak` with the
interpreter of the runtime image on its first line, and the runtime stage
(`gcr.io/distroless/python3-debian12:nonroot`) only copies the result. What the
application gets:

```dockerfile
ENV KOKORO_BIN=/opt/kokoro/bin/kokoro-speak \
    KOKORO_MODEL=/opt/kokoro/model/kokoro.onnx \
    KOKORO_VOICES=/opt/kokoro/model/voices-v1.0.bin \
    KOKORO_PYTHONPATH=/opt/kokoro
```

and `Kokoro.Env: []string{"PYTHONPATH=/opt/kokoro"}` in the configuration. To
serve Piper and Kokoro from one image, put both build stages in it and both sets
of `COPY` lines (the paths do not overlap: `/opt/piper`, `/opt/voices`,
`/opt/kokoro`). `--build-arg KOKORO_MODEL=kokoro-v1.0.int8.onnx` builds with the
int8 model. The image is 1 GB with the fp32 model. It was built, and the
module's `TestRealKokoro` was run inside it as a non-root user with two CPUs, on
arm64; the amd64 packages are the ones PyPI publishes for Python 3.11, with the
hashes of `kokoro-requirements.txt`.

## Licenses

- This package: MIT, see [LICENSE.md](LICENSE.md).
- Kokoro: the weights of Kokoro-82M are **Apache-2.0**
  ([hexgrad/Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M)), and so are
  the voices (`pf_dora`, `pm_alex`, `pm_santa`). `kokoro-onnx`, which runs them,
  is MIT, as are onnxruntime and espeakng-loader; numpy, joblib, cloudpickle and
  protobuf are BSD, flatbuffers Apache-2.0. The two GPL-3.0 parts are the ones
  Piper already brings: espeak-ng (the library and the data in `espeakng-loader`)
  and `phonemizer`. They are run by this package, never linked into it, so the
  rule is Piper's: an image you distribute makes you a distributor of GPL
  software, so keep their license texts with it (the snippet copies them to
  `/usr/share/licenses/kokoro`) and be ready to provide the source, which is
  public; a service you only run does not distribute it. Keep the Apache-2.0
  notice of Kokoro-82M with the model.
- Piper (`piper-tts`): GPL-3.0-or-later. Running it as a separate program does not
  make this package or your application GPL. Putting it in an image you
  distribute makes you a distributor of GPL software: keep its license text with
  it (the snippet copies `COPYING`) and be ready to provide its source, which is
  public. A service you only run does not distribute it.
- Piper standalone (`rhasspy/piper`): MIT, but its binary bundles espeak-ng,
  which is GPL-3.0, so the same applies to the archive.
- The voices: see **Choose a voice**. Keep each voice's `MODEL_CARD` with it.
  `pt_BR-faber-medium` asks for credit to the Hi-Fi TTS dataset (CC BY 4.0),
  for example on the page that describes the assistant: "Voz Faber, treinada a
  partir do conjunto Hi-Fi TTS (CC BY 4.0)". `pt_BR-cadu-medium` is CC0 and
  asks for nothing.

## Run it against the real thing

The tests never need Piper. One of them runs the real program when you ask:

```bash
PIPER_BIN=/opt/piper-venv/bin/piper PIPER_MODEL=/opt/voices/pt_BR-faber-medium.onnx \
PIPER_FFMPEG=/usr/bin/ffmpeg PIPER_SAMPLES=/tmp/samples \
  go test -run TestRealPiper -v ./tests/Unit
# PIPER_CONFIG for a voice JSON that is not beside the model,
# PIPER_ENV=PYTHONPATH=/opt/piper for a pip --target install
```

It speaks one sentence, checks the WAV, makes Opus notes at the voice's pace and
slower, and keeps all of them in `PIPER_SAMPLES` for listening. Without the
variables it skips and says why.

The Kokoro voices have their own, which runs the real program and model:

```bash
KOKORO_BIN=/opt/kokoro/bin/kokoro-speak KOKORO_MODEL=/opt/kokoro/model/kokoro.onnx \
KOKORO_VOICES=/opt/kokoro/model/voices-v1.0.bin KOKORO_ENV=PYTHONPATH=/opt/kokoro \
KOKORO_FFMPEG=/usr/bin/ffmpeg KOKORO_SAMPLES=/tmp/samples \
  go test -run TestRealKokoro -v ./tests/Unit
```

It speaks a sentence with each Kokoro voice, checks the WAV and the length,
makes an Opus note, and keeps them in `KOKORO_SAMPLES`. `KOKORO_ENV` is a
comma-separated list of `KEY=VALUE`, needed for a `pip --target` install. Without
the variables it skips and says why.

## License

MIT. See [LICENSE.md](LICENSE.md).
