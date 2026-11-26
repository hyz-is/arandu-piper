# Upgrade Guide

## Unreleased

## 0.6.0

No exported name was removed or changed: `apidiff` against 0.5.0 shows only additions (`LetterName`, `SpellAcronym`, `ReadsAsWord`, `ParseLexicon`, `LexiconJSON`, `LexiconVersion`, `Validate`, `Normalizer.Validate`, `Normalizer.Check`, `Issue`, `IssueKind`, `Severity` and their constants, `HasErrors`). What changes is what a `*Client` says, on purpose, and `NormalizerVersion`.

- **`NormalizerVersion` is `"ptbr-2.lex1"`** (it was `"ptbr-1"`). It is a constant string: a comparison with the old value, or a cache key that was built with it, moves with it. Put it in `RemoteConfig.KeyTag` as before.
- **Cache keys change** for any text with an acronym, a domain, an e-mail, an identifier (plate, IP, version, protocol), a range of hours, `24/7`, `D+1`, a unit of data, a foreign word of the dictionary, `§` or `#`. The audio stored under the old keys is not wrong, it is the 0.5.0 speech, and a `Cache` makes it again on the first call. Nothing to migrate; a text of words and plain numbers keeps its key.
- **The speech changes where the engine guessed.** "ajuda@ctrl.fit" was "ajuda arroba ctrl ponto fit" and is now "ajuda arroba cê tê érre éle ponto fít"; "CPF" was "cê pê efe" and is "cê pê éfe"; "RPE" was "érre pê é" and is "érre pê êh"; "08h às 17h" was "de oito horas às dezessete horas" and is "das oito às dezessete horas"; "US$ 10" is "dez dólares americanos"; `FC` is "éfe cê". Read the table of section 3 of `docs/speech-ptbr.md`, and `CHANGELOG.md`, for the list.
- **An application that passes its own `Speech.Lexicon`** keeps winning: its entries run first. Check the ones that repeat a term of the shared dictionary (CPF, IMC, WhatsApp...): the dictionary now spells with `êh`, `ôh`, `i` and `u` where the spoken letter names have an accent, because espeak-ng spells a lone accented vowel out ("i agudo"). If your lexicon spells with "í", "ê", "ô" or "ú" alone, replace them. `ParseLexicon` reads a lexicon from a JSON file in the format of `lexicon/ptbr.json` into the `[]LexiconEntry` that `Speech.Lexicon` takes.
- **Five defects of 0.5.0 are fixed** (see "Fixed" in `CHANGELOG.md`), so an application that worked around them can drop the workaround: "dia" is not said twice before a date, `parcela 3/12` is "três de doze", the lexicon of the application applies inside domains and e-mails (matching without accents, so a lexicon with "Corujão" speaks `ajuda@corujao.ai` as "corujão"), a relative date next to the word it becomes is not said twice ("ontem, 26/11" is "ontem"), and `e-mail` and `link` are "imêil" and "linque". The audio of a text with any of them changes, and so does its key.
- **`MaxChars`** still counts the text as given; an address or an identifier says more words than it has characters, so the spoken text may be longer than before (the limit of four times still holds).
- **To validate**, call `n.Check(reply, now)` where you used `n.Speakable(reply, now)`, and refuse the audio when `piper.HasErrors(issues)` (send the text alone). A warning is something a person may want to look at, not a failure.
- **The system prompt**: section 4 of `docs/speech-ptbr.md` is now the 17-rule guideline of the study of 26/11/2026; replace the earlier text in the prompt. The model writes plain notation (`R$ 24,90`, `3x12`, `06:30`) and the code says it.

## 0.5.0

No exported name was removed or changed: `apidiff` against 0.4.0 shows only additions. What changes is what a `*Client` says, on purpose.

- **A `*Client` now makes the text speakable before speaking it** (`Speakable`: numbers, money, dates, times, units and symbols in words, formatting and emoji removed, each line a sentence). A text such as `"Próximo treino: supino 3x10 às 06:30."` was read by espeak-ng as it could; now it is read as `"... supino três séries de dez às seis e meia."`. This is the right thing for every text a person typed; to keep the old behaviour of a client, set `Config.Speech.Raw: true`.
- **Cache keys.** `Key` hashes the text that is spoken, so a key changes only when what is said changes, and for these texts it does: any text with a digit, a symbol, a mark of formatting, an emoji, a list or a line that did not end in a mark, and any text that the lexicon touches. A text of plain words that already ends in `.`, `!` or `?` keeps its key. The audio stored under the old keys is not wrong, it is the old speech, and a `Cache` simply makes it again on the first call. Nothing to migrate; if you want the old audio kept, set `Raw`.
- **A `Remote` does not normalize**: the node it calls does, and `Remote.Key` hashes the text it was given. After a node is upgraded the same text is spoken differently under the same key. Put the version in the key: `RemoteConfig{KeyTag: piper.NormalizerVersion}` (or `client.NormalizerVersion()` of a local client with the same `Speech`), and the first calls after the upgrade make the audio again.
- `MaxChars` still counts the text as given, before the rules run. The spoken text may be up to four times as long (`R$ 1.234.567,89` is five times as long in words); a longer one is `ErrTextTooLong`.
- An application that already makes a spoken text itself (as Corujão.ai does): pass it with `Speech.Raw: true` to speak it as it is, or delete the code and use `Normalizer` with `Speech.Lexicon` holding the domain's words, `MaxWords` and `RestNote`. `Speakable` of a spoken text is that text, so a text that has been through both is not changed twice.
- Store `Normalizer.Version()` beside the spoken text you keep, so a voice note can be audited against the rules that made it.
- To make a model write text that is easy to hear, put the guideline of `docs/speech-ptbr.md`, section 4, in its system prompt.

## 0.4.0

Nothing to change: the API of 0.3.2 is intact and a client that does not set `Config.Kokoro` is the client it was. The keys of Piper audio are the ones 0.3.2 made.

- To add the female Brazilian voice, install the Kokoro runtime and model in the service image (`docs/Dockerfile.kokoro.snippet`: the packages of `docs/kokoro-requirements.txt` by `pip --target`, `kokoro-v1.0.onnx` and `voices-v1.0.bin`, checked by sha256, and `docs/kokoro-speak.py` with the interpreter on its first line), and set `Config.Kokoro` with the three paths and `Env: PYTHONPATH=/opt/kokoro`. The service's configuration then serves `pf_dora`, `pm_alex` and `pm_santa` beside the Piper voices; give `DefaultVoice` the voice you want when none is named.
- A node of a service that will not serve Kokoro needs nothing, but an application whose `Remote` calls it for a Kokoro voice gets `ErrUnknownVoice` once every node has refused it. Call `Remote.Voices(ctx)` at boot to see which node serves what.
- `Remote` now asks the next node when one answers `unknown_voice` (see the changelog). Code that relied on the first refusal being final sees the same `errors.Is(err, piper.ErrUnknownVoice)`, after one request to each node.
- A `Handler` answers `GET` with a valid token with the list of voices. Nothing calls it that did not before; a proxy that forbids `GET` on the path only loses `Remote.Voices`, and the calls themselves still work.

## 0.3.2

Nothing to change.

## 0.3.1

Nothing to change. `Remote` now waits up to `BusyWait` (5 seconds) when every node is busy, where it used to fail at once; set `BusyWait: -1` to keep the old behaviour. If you limited Piper's threads with `OMP_NUM_THREADS`, it never took effect: see `docs/sitecustomize.py`.

## 0.3.0

Nothing to change: every addition is optional and the API of 0.2.0 is intact.

- To run speech as a service, build a `*Client` with `Format: piper.FormatOpus` and mount `piper.NewHandler` where the service listens.
- To use a service from an application, build `piper.NewRemote` and pass it where the application passed the `*Client` (a `piper.Speaker`; it is also `piper.Keyed`, so `piper.NewCache` takes it). Its `Key` is not the `*Client`'s: the keys of audio stored by a cache over a local client do not match the remote's, so the first calls after switching make the audio again.
- The manifest now declares `network = true`. An application whose own manifest or policy forbids network use by its dependencies should be told.

## 0.2.0

Nothing to change: `NoiseScale` and `NoiseW` are new fields whose zero value is the old behaviour, and the keys of a client that does not set them are the ones 0.1.0 made.

## 0.1.0

The first release: nothing to upgrade from. Install it as the README says.
