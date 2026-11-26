# Speech in Brazilian Portuguese: normalizing text for a voice, and writing text to be heard

This is the study behind `Speakable`, the verbalizer that every voice of this
package (Piper `pt_BR-faber-medium`, `pt_BR-cadu-medium`, Kokoro `pf_dora`,
`pm_alex`, `pm_santa`) speaks through by default. It has four parts: how text
normalization works, what the two engines do with the text they are given, the
rules this package applies and why, and a guideline to paste into the system
prompt of a model whose answer will be heard.

Everything here is for **Brazilian** Portuguese (`pt-BR`): "dezesseis" and not
"dezasseis", "você" and not "tu", "R$" and not "€", the date as day, month, year.
The engines are called with the Brazilian espeak-ng voice, `pt-br`, never the
European `pt` (see "What the two engines do").

## 1. How text normalization for speech works

A voice model is trained on text that is already spoken: words. What the person
types is not: "3x10", "R$ 24,90", "06:30", "kg", "26/11/2026", "IMC". Turning
the second into the first is **text normalization** (the front end of a
text-to-speech system), and the literature splits it in two steps:

1. **Tokenization and classification**: find the tokens that are not ordinary
   words, the "non-standard words" of Sproat et al. (2001), and decide which
   *semiotic class* each is: cardinal, ordinal, decimal, money, date, time,
   measure, telephone, abbreviation, acronym, symbol. The same characters are
   different things in different places: "1/2" is a date or a fraction, "10h" a
   time of day or a duration, "x" a letter or a multiplication, "ME" a pronoun
   or a kind of company.
2. **Verbalization**: write the words that a person would say for that class,
   with the agreement the language needs: "duas séries" and not "dois séries",
   "um quilo" and "dois quilos", "vinte e quatro reais e noventa centavos".

Google's production system (Kestrel) is exactly this: weighted finite-state
grammars for classification and verbalization, written by hand per language
(Ebden and Sproat, 2015). Sproat and Jaitly (2016) trained neural networks on
its output and found that they reach high average accuracy but make errors
that "convey completely the wrong message" (a wrong number, said fluently), so
the neural model needed a finite-state filter to be safe. Zhang et al. (2019)
confirm the same for neural normalizers in general. The practical lesson for a
product that speaks a person's money and training load: **numbers are verbalized
by deterministic code, never by the language model that wrote the answer**. This
package is that code. It is a pure function, with no network, no model and no
randomness, tested table by table.

Hard parts that every normalizer meets, and what this one does:

| Problem | Example | What `Speakable` does |
| --- | --- | --- |
| Class by context | "7h30" is a clock after "às", a duration elsewhere | "às 7h30" is "às sete e meia"; "treino de 1h30" is "uma hora e meia" |
| Gender and number agreement | "2 séries", "1 kg", "2 kg" | the number takes the gender of the unit or of the next noun (a list of feminine nouns), and the unit the plural |
| Locale conventions | "1.200,50" is one thousand two hundred and a half in pt-BR | dot for thousands, comma for decimals; "42.5" with one or two digits after a dot is a decimal |
| Ambiguity that cannot be solved | "1/2" date or fraction, "x" séries or times | a rule is picked and written down (below); an application that means the other writes it out |
| Homographs | "colher" (noun or verb), "sede", "jogo" | not solved by rules; the espeak-ng dictionary of the engine decides, and the application fixes a bad one in its lexicon |
| Names and acronyms | "CPF", "IMC", "WhatsApp" | a lexicon that says how each is pronounced, case-sensitive for acronyms |
| Long numbers | CPF, telephone, protocol | said digit by digit in groups, because a quantity cannot be written down by the listener |

Commercial engines expose this through SSML. Google Cloud Text-to-Speech has
`<say-as interpret-as="cardinal|ordinal|fraction|unit|date|time|telephone|currency|characters|verbatim">`
and `<sub alias>`, Azure has the same plus `duration`, a custom lexicon in the
W3C Pronunciation Lexicon format and "phoneme" elements, and notes that
`interpret-as` values besides `characters` work for Portuguese. ElevenLabs
normalizes by default, and its own advice for an LLM-written text is "ensure that
numbers, symbols, and abbreviations are expanded for clarity when read aloud",
with regular expressions or a number-to-words library when the text is
preprocessed in code. That last advice is this package, for Portuguese.

## 2. What the two engines do with the text

Both engines turn **spelling into phonemes with espeak-ng** and only then run a
neural model. Facts below were checked in the sources of the versions the module
is tested with (piper-tts 1.8.0, kokoro-onnx 0.6.1) and in each engine's docs.

**Language.** Piper's `pt_BR` voices carry `"espeak": {"voice": "pt-br"}` in their
`.onnx.json`. Kokoro's documentation (`VOICES.md`) lists Brazilian Portuguese as
`lang_code='p'`, espeak-ng `pt-br`, voices `pf_dora`, `pm_alex`, `pm_santa`. This
package passes `--lang pt-br` to `kokoro-speak` for every Kokoro voice and a test
holds it (`TestAKokoroVoiceGetsTheSpeakableText`): it must never become `pt`,
which is European Portuguese and changes the vowels, the "r", the "s" and the
"t/d" before "i".

**No text normalization to count on.** Neither engine documents any. Piper's
CLI documents `--no-normalize` and `normalize_audio`, and both are about
**volume**. kokoro-onnx's `tokenizer.normalize_text` only strips the text. What
espeak-ng does with digits it knows how to say, but without agreement ("dois
séries"), without units ("quilos"), money ("R$"), times ("06:30") or dates, as the
measurements in section 5 show.

**No SSML.** espeak-ng itself reads SSML (its `markup.md` lists `speak`, `voice`,
`prosody`, `say-as`, `mark`, `s`, `p`, `sub`, `audio`, `emphasis`, `break`), but
neither Piper nor Kokoro passes markup through: Piper's docs do not mention SSML,
and Kokoro takes plain text. Piper has one escape hatch, `[[ phonemes ]]` blocks
of raw espeak-ng phonemes, which Kokoro does not have, so this package does not
use it. A word that a voice reads wrong is fixed **in the spelling**: the
pronunciation lexicon spells it as a Brazilian says it ("WhatsApp" is "uótsap").
That is what the Microsoft `alias` of a custom lexicon does too.

**Punctuation is the prosody.** There is no other control, so the text must be
punctuated for the ear:

- Piper (`phonemize_espeak.py`): espeak-ng splits the text into clauses and
  returns the terminator of each. A comma, colon or semicolon **does not end the
  sentence**: it is kept as punctuation, followed by a space, inside the same
  chunk, where the model puts a short pause. Any other terminator (`.`, `!`, `?`)
  ends the sentence, and **each sentence is synthesized as its own audio chunk**,
  with `--sentence-silence` seconds of silence between them (not after the last).
  A text with no final mark is one last sentence.
- Kokoro (`kokoro-onnx` `chunker.py` and `pauses.py`): the phonemizer is called
  with `preserve_punctuation=True`; `.` `!` `?` `…` are sentence marks and `,` `;`
  `:` are clause marks; the model's context is **510 phonemes**, and a longer text
  is cut at a sentence mark first, then at a clause mark, then at a space, and
  "only as a last resort mid-word". The pause a mark calls for is topped up after
  the model (`--sentence-pause`). Newlines are not in the vocabulary: they are
  spaces, which is why a list must be turned into sentences before it is spoken.
- In practice a question mark raises the final intonation and a period lowers
  it. A long sentence without commas is read in one breath, and past the 510
  phonemes of Kokoro's context it is cut at a space.

So: short sentences, one idea each, ending in `.` `?` or `!`; a comma where a
person would breathe; no brackets, slashes or symbols, which espeak-ng either
drops or spells.

## 3. The rules `Speakable` applies

Applied line by line, from the largest unit to the smallest, so a rule never
takes apart what an earlier one has said. Anything not listed is kept as it is.

| Class | Written | Said |
| --- | --- | --- |
| Formatting, emoji | `*bold*`, `_italic_`, `~x~`, `` `code` ``, headings, quotes, 🎉, → | removed (what `Plain` does); an arrow is a comma |
| Lists | `- a`, `• b`, `1. c` | one sentence per item, a full stop at the end of each |
| Parentheses and brackets | `(aqui)` | commas: ", aqui," |
| Links | `[texto](url)`, `https://...` | the text of the link; a bare address is "o link" ("mostrado na conversa" after the word link or site) |
| E-mail | `ajuda@hyz.is` | "ajuda arroba hyz ponto is" |
| Cardinals | `3`, `21`, `101`, `1.200`, `2026` | "três", "vinte e um", "cento e um", "mil e duzentos", "dois mil e vinte e seis" |
| Agreement | `2 séries`, `1 hora`, `300 repetições` | "duas séries", "uma hora", "trezentas repetições" (feminine nouns list) |
| Decimals | `3,2`, `0,05`, `42.5` | "três vírgula dois", "zero vírgula zero cinco", "quarenta e dois vírgula cinco" |
| Negative, positive | `-2,5`, `+3` | "menos dois vírgula cinco", "mais três" |
| Round millions | `2.000.000 de passos` | "dois milhões de passos" |
| Ordinals | `1º`, `2ª`, `21º`, `nº 5`, `#1` | "primeiro", "segunda", "vigésimo primeiro", "número cinco", "número um" |
| Money | `R$ 24,90`, `R$ 1,00`, `R$ 0,50`, `R$ 2 mil`, `-R$ 50`, `US$ 10`, `€ 1,50` | "vinte e quatro reais e noventa centavos", "um real", "cinquenta centavos", "dois mil reais", "menos cinquenta reais", "dez dólares", "um euro e cinquenta centavos" |
| Per | `R$ 19,00/mês`, `kcal/dia` | "dezenove reais por mês", "calorias por dia" |
| Percent | `45%`, `12,5 %` | "quarenta e cinco por cento", "doze vírgula cinco por cento" |
| Times of day | `06:30`, `18:45`, `07:00`, `01:00`, `00:00`, `12:00`, `7h`, `às 7h30` | "seis e meia", "dezoito e quarenta e cinco", "sete horas", "uma hora", "meia-noite", "meio-dia", "sete horas", "sete e meia" |
| Durations | `1h30`, `2h15`, `1:02:30`, `5:30/km` | "uma hora e meia", "duas horas e quinze minutos", "uma hora, dois minutos e trinta segundos", "cinco minutos e trinta segundos por quilômetro" |
| Dates | `26/11/2026`, `20/12`, `2026-11-26` | "vinte e seis de novembro de dois mil e vinte e seis", "vinte de dezembro"; day 1 is "primeiro" |
| Dates relative to now | the same, with a moment `now` | "hoje", "ontem", "anteontem", "amanhã", "depois de amanhã", "dia vinte", "dia vinte de dezembro" |
| Not a date | `26/27` (a harvest), `31/02` | "vinte e seis, vinte e sete" |
| Fractions | `1/2 xícara`, `3/4` | "meia xícara", "três quartos" |
| Ranges | `8-12`, `8–12 repetições`, `60-90 s`, `70-80%` | "de oito a doze", "de oito a doze repetições", "de sessenta a noventa segundos", "de setenta a oitenta por cento"; after "entre": "entre oito e doze" |
| Fitness notation | `3x10`, `4x12-15`, `1x15`, `4x por semana` | "três séries de dez", "quatro séries de doze a quinze", "uma série de quinze", "quatro vezes por semana" |
| Units | `kg g mg km m cm mm ml l L min h s kcal bpm rpm km/h m/s kg/m² m² °C °` | "quilo", "grama", "miligrama", "quilômetro", "metro", "centímetro", "milímetro", "mililitro", "litro", "minuto", "hora", "segundo", "calorias", "batimentos por minuto", "rotações por minuto", "quilômetros por hora", "metros por segundo", "quilos por metro quadrado", "metros quadrados", "graus Celsius", "graus"; the singular only for exactly one |
| Documents, telephone | `123.456.789-09`, `(11) 99999-8888`, `01310-100`, a run of 9 digits or more | digit by digit, in groups separated by a comma |
| Acronyms | `CPF CNPJ RG CNH CEP IRPF INSS FGTS MEI PIX IMC RPE RIR PDF API SMS ...` | the base lexicon (`BaseLexicon()`): "cê pê efe", "i ême cê", "érre pê é", "pics", ... |
| Abbreviations | `aprox. p.ex. ex. etc. Sr. Sra. Dr. Dra. Prof. obs. pág. tel. máx. mín. vs c/ s/ p/ vc pq tb hj` | "aproximadamente", "por exemplo", "etcétera", "senhor", ... |
| Symbols | `+ & = ≈ ~300 ≥ ≤ e/ou / – — · |` | "mais", "e", "igual a", "cerca de", "maior ou igual a", "menor ou igual a", "e ou", a comma |
| Capitals | `SIM`, `DESFAZER` | "sim", "desfazer": a word typed in capitals is a word, after the lexicon has spelled the acronyms it knows |
| Repeated marks | `!!!`, `???`, `?!`, `...` | one mark |
| Prosody | a sentence over 16 words with little punctuation | a comma before "mas", "porque", "quando", "enquanto", "então", "depois" nearest the middle |
| Length | optional `MaxWords` | cut at the last sentence that fits, and `RestNote` said after it |

### Choices that were made, and why

- **`kcal` is "calorias".** It is what a Brazilian says about food and training,
  and what a nutrition label means by "Caloria" with a capital C. "Quilocalorias"
  is formally exact but heavy to hear. An application that wants it puts
  `{Written: "kcal", Said: "quilocalorias"}` in `Speech.Lexicon`: the domain
  lexicon runs first and wins.
- **The gram is masculine** ("duzentos gramas"), as the Brazilian unit
  nomenclature has it. The symbols of units are invariable and the names follow
  the plural rules of Portuguese (Inmetro, Quadro Geral de Unidades de Medida,
  Portaria 615/2023); this package says the singular only for exactly one, so
  "1,5 kg" is "um vírgula cinco quilos".
- **"3x10" is "séries de".** In a fitness text "3x10" is three sets of ten, and
  "×" is read the same. A multiplication is written "vezes".
- **"1/2" is a fraction, "3/4" too**, when each side is one digit and the
  numerator is smaller; "03/04" or "20/12" are dates. A date such as 1/2 (the
  first of February) is written with two digits.
- **"7h30" is a clock after "às/as/à/das/até" and a duration otherwise**, and
  "HH:MM" is always a clock, except before "/km" (a pace). "Às 06:30" is "às
  seis e meia": the half hour is "meia", the others are "e quinze", "e quarenta e
  cinco".
- **The first day of the month is "primeiro"**, as in the Brazilian style
  manuals ("1º de fevereiro"), and the other days are cardinals. Years are said
  as cardinals ("dois mil e vinte e seis"). The conjunction "e" is put before the
  last group when it is below one hundred or a round hundred ("mil e duzentos",
  "dois mil e cinquenta") and left out between groups otherwise ("mil duzentos e
  trinta e quatro"), as the style guides teach.
- **A round million of money takes "de"** ("dois milhões de reais"), and R$ 0,00
  is "zero real".
- **Dates relative to a moment are opt-in**: the package never reads the clock.
  Pass `now` to `Normalizer.Speakable`; without it a date is said in full, so the
  result depends only on the text.

### What is not done

Roman numerals ("século XX"), scores ("2-1" stays as typed), a word that is a
homograph, hyphenated compounds with numbers, e-mail local parts that spell words,
English text inside Portuguese (the voice reads it with Portuguese phonemes),
formulas, and the grammatical gender of a noun that is not in the list: after an
unknown noun a number is masculine. Each is a line in a lexicon or a rule to add
with a test.

## 4. Writing for speech: a guideline for the system prompt

Normalization repairs what is written for the eye; it is not a license to write
for the eye. A model that knows its answer will be heard writes shorter, plainer
text, and the result is better than any rule can make of a table. Paste this in
the system prompt of an assistant whose reply will be spoken (Control Fit,
Corujão and any other application that calls `Speak` with a reply):

```text
Sua resposta será falada em voz alta, em português do Brasil. Escreva para o ouvido, não para a leitura.

- Frases curtas, de até 20 palavras, com uma ideia cada. Termine cada frase com ponto, interrogação ou exclamação. Use vírgula onde uma pessoa respiraria.
- Sem listas, tópicos, tabelas, títulos, negrito, itálico, emojis, setas, símbolos, parênteses, barras ("e/ou", "km/h" escrito com barra) nem links. Se precisar enumerar, diga "primeiro", "depois" e "por fim", ou separe em frases.
- Fale com "você", em tom de conversa, com vocabulário brasileiro ("café da manhã", "treino", "celular", "ônibus"). Nada de "tu", "vós" nem de português de Portugal.
- Arredonde quando a precisão não importa: diga "cerca de trezentas calorias", não "292 kcal"; "quase dois quilos", não "1,87 kg". Mantenha o número exato só quando a pessoa precisa dele (dose, valor a pagar, data de vencimento).
- No máximo três números por resposta. Prefira o resultado a todo o cálculo.
- Escreva as unidades por extenso: "quilos", "quilômetros por hora", "minutos", "reais". Não use "kg", "km/h", "min" nem "R$".
- Dinheiro e porcentagem: "vinte e quatro reais e noventa centavos", "doze por cento". Séries e repetições: "três séries de dez repetições", não "3x10".
- Horas e datas como se fala: "às seis e meia", "amanhã de manhã", "dia vinte de dezembro". Não use "06:30" nem "26/11/2026".
- Evite siglas e abreviações. Se usar uma sigla conhecida (IMC, CPF), use a que o ouvinte já conhece e diga o nome por extenso na primeira vez.
- Comece pela resposta. Se a pessoa fez uma pergunta, responda na primeira frase. Termine com uma única pergunta ou próximo passo, nunca com uma despedida longa.
- Respostas faladas têm no máximo cerca de 75 palavras (meio minuto). Se o assunto pede mais, resuma em voz e diga que o resto está por escrito.
```

Even when the model follows this, keep the verbalizer on: it is the safety net
for the digit that gets through, and it is deterministic where the model is not.
Do not ask the model for a separate "spoken" field with numbers already written
out: it would have to be trusted with the numbers, and the ones a person hears
about their own money or training must come from the code that added them up.

## 5. Measured with the real engines

`Speakable` was checked against the real engines on this machine with six
sentences of the products (Control Fit and Corujão style), two voices (Kokoro
`pf_dora` fp32 and Piper `pt_BR-faber-medium`), each spoken with the raw text and
with the normalized text, and transcribed with whisper.cpp `ggml-small`,
`-l pt`. The score is word accuracy against the intended spoken text
(`1 - word edit distance / reference words`, accents and case ignored); the words
the recognizer heard also go through `Speakable`, so a "10" it writes is compared
with "dez".

| Voice | Raw text | Normalized text |
| --- | --- | --- |
| Kokoro `pf_dora` | 74.6% | 94.3% |
| Piper `pt_BR-faber-medium` | 65.0% | 93.1% |

What the raw text did: "R$ 228,00" became "Real 228,00 real" and "Flano
Anual"; "3x10" became "3x" and "630"; "50 min" became "50 mil"; "60-90 s" became
"60 menos 90s"; "IMC" became "INC"; "5:58/km" became "558km".
What is left after normalization is mostly the recognizer's own habit of writing
"seis e meia" as "6,5" and "cento e quarenta" as "100 e 40", which the score
counts as errors although the voice said the right words. Six sentences per voice
is a small sample: it shows the direction and the kind of failure, not a precise
rate. The audio and the table are in the evidence folder of Control Fit
(`plans/evidence/voice-samples-2026-11-26/normalized/`).

## 6. Using it

```go
// Default: every Speak and SpeakAs runs the text through Speakable, with dates in full.
c, _ := piper.New(piper.Config{ /* ... */ })
audio, err := c.Speak(ctx, reply)

// A domain lexicon, a length limit, and dates counted from a moment:
n := piper.NewNormalizer(piper.Speech{
    Lexicon:  []piper.LexiconEntry{{Written: "supino", Said: "supíno", AnyCase: true}},
    MaxWords: 75,
    RestNote: "O resto eu deixei por escrito, na conversa.",
})
spoken := n.Speakable(reply, time.Now())  // store it beside the original text
version := n.Version()                    // store it too, and put it in the cache key
audio, err = c.Speak(ctx, spoken)         // with Config.Speech.Raw: true, it is spoken as it is

// Both pass through Speakable again unchanged, so a client that is not Raw can be given
// an already normalized text: Speakable(Speakable(x)) == Speakable(x).
```

`Key` hashes the spoken text, so two writings that are spoken the same make the
same audio and the same key. A `Remote` hashes the text it sends and not the
spoken text (the node makes it): put `NormalizerVersion` in `RemoteConfig.KeyTag`
so a node upgrade does not leave the old audio under the new rules.

## Sources

Text normalization
- Sproat, R., Black, A., Chen, S., Kumar, S., Ostendorf, M., Richards, C. (2001).
  Normalization of non-standard words. *Computer Speech & Language* 15(3).
- Ebden, P. and Sproat, R. (2015). The Kestrel TTS text normalization system.
  *Natural Language Engineering* 21(3). Catalogue record:
  <https://eld.bl.uk/catalog/100025905687_0x000001>
- Sproat, R. and Jaitly, N. (2016). RNN approaches to text normalization: a
  challenge. arXiv:1611.00068 <https://arxiv.org/abs/1611.00068>; and An RNN model
  of text normalization, Interspeech 2017
  <https://research.google/pubs/an-rnn-model-of-text-normalization/>
- Zhang, H., Sproat, R., Ng, A. H., Stahlberg, F., Peng, X., Gorman, K., Roark, B.
  (2019). Neural models of text normalization for speech applications.
  *Computational Linguistics* 45(2) <https://aclanthology.org/J19-2004/>
- Transformer-based models of text normalization for speech applications (2022).
  arXiv:2202.00153 <https://arxiv.org/abs/2202.00153>
- Dissertation on a Brazilian Portuguese TTS system with text normalization on
  MARY (UFPA) <https://www.repositorio.ufpa.br/bitstream/2011/2639/1/Dissertacao_DesenvolvimentoRecursosConstrucao.pdf>

The engines
- Kokoro-82M, VOICES.md (Brazilian Portuguese: `lang_code='p'`, espeak-ng `pt-br`,
  `pf_dora`, `pm_alex`, `pm_santa`)
  <https://huggingface.co/hexgrad/Kokoro-82M/blob/main/VOICES.md>
- kokoro-onnx 0.6.1, `tokenizer.py`, `chunker.py`, `pauses.py`
  <https://github.com/thewh1teagle/kokoro-onnx>
- Piper (OHF-Voice/piper1-gpl): README, `docs/CLI.md`, and `piper/phonemize_espeak.py`
  and `piper/voice.py` of piper-tts 1.8.0 <https://github.com/OHF-Voice/piper1-gpl>
- espeak-ng, SSML markup <https://github.com/espeak-ng/espeak-ng/blob/master/docs/markup.md>

How commercial engines take it
- Google Cloud Text-to-Speech, SSML <https://docs.cloud.google.com/text-to-speech/docs/ssml>
- Azure Speech, pronunciation with SSML (say-as, sub, custom lexicon)
  <https://learn.microsoft.com/en-us/azure/ai-services/speech-service/speech-synthesis-markup-pronunciation>
- W3C Pronunciation Lexicon Specification 1.0 <https://www.w3.org/TR/pronunciation-lexicon/>
- ElevenLabs, text to speech best practices (normalization)
  <https://elevenlabs.io/docs/overview/capabilities/text-to-speech/best-practices>

Brazilian conventions
- Inmetro, Quadro Geral de Unidades de Medida (symbols are invariable, names are
  written in lower case) <https://www.gov.br/inmetro/pt-br/centrais-de-conteudo/publicacoes/documentos-tecnicos-em-metrologia/quadro-geral-de-unidades-de-medida-no-brasil.pdf>
  and Portaria Inmetro 615/2023 <https://www.normasbrasil.com.br/norma/portaria-615-2023_453815.html>
- Manual de Comunicação, "Números" (first day of the month as "1º", no zero
  before an hour, a date as day, month name, year)
  <https://www12.senado.leg.br/manualdecomunicacao/estilos/numeros>
- Poder Judiciário de Santa Catarina, how to write numbers in full (the "e"
  between hundreds, tens and units)
  <https://www.tjsc.jus.br/web/servidor/dicas-de-portugues/-/asset_publisher/0rjJEBzj2Oes/content/como-escrever-numeros-por-extenso->
- Dicio, how to write numbers in full <https://www.dicio.com.br/como-escrever-numeros-por-extenso/>
- Spelling of the numerals follows the Orthographic Agreement of 1990
  (Vocabulário Ortográfico da Língua Portuguesa, Academia Brasileira de Letras):
  dezesseis, dezessete, catorze.
