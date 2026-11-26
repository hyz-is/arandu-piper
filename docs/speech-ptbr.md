# Speech in Brazilian Portuguese: normalizing text for a voice, and writing text to be heard

This is the study behind `Speakable`, the verbalizer that every voice of this
package (Piper `pt_BR-faber-medium`, `pt_BR-cadu-medium`, Kokoro `pf_dora`,
`pm_alex`, `pm_santa`) speaks through by default. It has four parts: how text
normalization works, what the two engines do with the text they are given, the
rules this package applies and why (since 0.6.0 including the phonetic layer:
names of the letters, acronyms, domains, identifiers and a shared dictionary), and
a guideline to paste into the system prompt of a model whose answer will be heard.

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
| Links | `[texto](url)`, `https://...` | the text of the link; a bare address that can be dictated is said as a domain; one that cannot is "o link" ("mostrado na conversa" after the word link or site) |
| E-mail | `ajuda@hyz.is`, `ajuda@ctrl.fit` | "ajuda arroba hyz ponto i ésse", "ajuda arroba cê tê érre éle ponto fít" |
| Domains and short paths | `alpr.sh`, `exemplo.com.br`, `exemplo.io/login`, `www.exemplo.com` | "á éle pê érre ponto ésse agá", "exemplo ponto com ponto bê érre", "exemplo ponto i ôh barra lóguin", "dáblio dáblio dáblio ponto exemplo ponto com"; an address that cannot be dictated (`?id=7`, digits in the path) stays "o link" |
| Cardinals | `3`, `21`, `101`, `1.200`, `2026` | "três", "vinte e um", "cento e um", "mil e duzentos", "dois mil e vinte e seis" |
| Agreement | `2 séries`, `1 hora`, `300 repetições` | "duas séries", "uma hora", "trezentas repetições" (feminine nouns list) |
| Decimals | `3,2`, `0,05`, `42.5` | "três vírgula dois", "zero vírgula zero cinco", "quarenta e dois vírgula cinco" |
| Negative, positive | `-2,5`, `+3` | "menos dois vírgula cinco", "mais três" |
| Round millions | `2.000.000 de passos` | "dois milhões de passos" |
| Ordinals | `1º`, `2ª`, `21º`, `nº 5`, `#1` | "primeiro", "segunda", "vigésimo primeiro", "número cinco", "número um" |
| Money | `R$ 24,90`, `R$ 1,00`, `R$ 0,50`, `R$ 2 mil`, `-R$ 50`, `US$ 10`, `$ 10`, `€ 1,50` | "vinte e quatro reais e noventa centavos", "um real", "cinquenta centavos", "dois mil reais", "menos cinquenta reais", "dez dólares americanos", "dez dólares", "um euro e cinquenta centavos" |
| Per | `R$ 19,00/mês`, `kcal/dia` | "dezenove reais por mês", "calorias por dia" |
| Percent | `45%`, `12,5 %` | "quarenta e cinco por cento", "doze vírgula cinco por cento" |
| Times of day | `06:30`, `18:45`, `07:00`, `01:00`, `00:00`, `12:00`, `7h`, `às 7h30` | "seis e meia", "dezoito e quarenta e cinco", "sete horas", "uma hora", "meia-noite", "meio-dia", "sete horas", "sete e meia" |
| Durations | `1h30`, `2h15`, `1:02:30`, `5:30/km` | "uma hora e meia", "duas horas e quinze minutos", "uma hora, dois minutos e trinta segundos", "cinco minutos e trinta segundos por quilômetro" |
| Dates | `26/11/2026`, `20/12`, `2026-11-26` | "vinte e seis de novembro de dois mil e vinte e seis", "vinte de dezembro"; day 1 is "primeiro" |
| Dates relative to now | the same, with a moment `now` | "hoje", "ontem", "anteontem", "amanhã", "depois de amanhã", "dia vinte", "dia vinte de dezembro" |
| Words already there | `dia 20/12`, `ontem, 26/11`, `hoje (27/11)` with `now` | "dia vinte de dezembro" (no second "dia"), "ontem" (the date is dropped when the word it would become is next to it) |
| Installments | `parcela 3/12`, `parcelas 10/12 e 11/12`, `5/10 parcelas`, `prestação 7/24` | "três de doze", "dez de doze e onze de doze", "cinco de dez", "sete de vinte e quatro": `x/y` with `x <= y <= 48` after or before the word of an installment; any other `x/y` is a date or a fraction |
| Not a date | `26/27` (a harvest), `31/02` | "vinte e seis, vinte e sete" |
| Fractions | `1/2 xícara`, `3/4` | "meia xícara", "três quartos" |
| Ranges | `8-12`, `8–12 repetições`, `60-90 s`, `70-80%` | "de oito a doze", "de oito a doze repetições", "de sessenta a noventa segundos", "de setenta a oitenta por cento"; after "entre": "entre oito e doze" |
| Fitness notation | `3x10`, `4x12-15`, `1x15`, `4x por semana` | "três séries de dez", "quatro séries de doze a quinze", "uma série de quinze", "quatro vezes por semana" |
| Units | `kg g mg km m cm mm ml l L min h s kcal bpm rpm km/h m/s kg/m² m² °C ° GB MB KB TB Gbps Mbps FPS Hz kHz MHz GHz V W kW kWh mAh` | "quilo", "grama", "miligrama", "quilômetro", "metro", "centímetro", "milímetro", "mililitro", "litro", "minuto", "hora", "segundo", "calorias", "batimentos por minuto", "rotações por minuto", "quilômetros por hora", "metros por segundo", "quilos por metro quadrado", "metros quadrados", "graus Celsius", "graus", "guigabáites", "megabáites", "quadros por segundo", "volts", "watts"; the singular only for exactly one |
| Documents, telephone | `123.456.789-09`, `(11) 99999-8888`, `01310-100`, a run of 9 digits or more | digit by digit, in groups separated by a comma |
| Acronyms, spelled | `CPF GPU ALPR SDK RTSP IMC RPE RIR`, a plural `APIs` | with the names of the letters (`LetterName`): "cê pê éfe", "gê pê u", "á éle pê érre", "i ême cê", "érre pê êh"; a plural adds "s" ("á pê is") |
| Acronyms, said as a word | `NASA SaaS JSON PIX HIIT SIM` | "nása", "sás", "djêison", "pics", "rít", "sim": the dictionary first, then the shape of the syllables (`ReadsAsWord`) |
| Foreign words, brands | `whey crossfit check-in feedback framework webhook Wi-Fi WhatsApp` | the way a Brazilian says them: "uêi", "cróssfit", "tchék ín", "fídbék", "frêim uârk", "uéb ruk", "uái fái", "uóts áp" |
| Identifiers | `ABC1D23`, `192.168.0.1`, `V2.1`, `protocolo 20261126-001`, `código AB12CD`, `0800 123 4567`, `+55 11 99999-8888` | digit by digit and letter by letter: "á bê cê um dê dois três", "um nove dois, ponto, um seis oito, ponto, zero, ponto, um", "versão dois ponto um", "...zero zero um", "mais cinco cinco, um um, ..." |
| Service hours | `24/7`, `D+1`, `das 08h às 17h`, `de 08:00 às 17:30` | "vinte e quatro horas por dia, sete dias por semana" (a date after "em", "dia", "até"), "dê mais um", "das oito às dezessete horas", "das oito às dezessete e meia" |
| Abbreviations | `aprox. p.ex. ex. etc. Ltda. S.A. Cia. Av. Sr. Sra. Dr. Dra. Prof. obs. pág. tel. máx. mín. vs c/ s/ p/ vc pq tb hj` | "aproximadamente", "por exemplo", "etcétera", "limitada", "sociedade anônima", "companhia", "avenida", "senhor", ... |
| Symbols | `+ & = ≈ ~300 ≥ ≤ e/ou / – — · | § §§ #1 #meta #` | "mais", "e", "igual a", "cerca de", "maior ou igual a", "menor ou igual a", "e ou", a comma, "parágrafo", "parágrafos", "número um", "hashtag meta", "cerquilha" |
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

- **The names of the letters follow the engine, not only the alphabet.** The
  accents carry the stress and the open vowels ("éfe", "éle", "érre", "agá"), with
  one exception found by measuring: espeak-ng **spells out** a lone letter that
  has an accent on a vowel, so "í pê" was read "i agudo pê" and "ê" "e
  circunflexo" (whisper wrote "iagudo p192" for an IP address). The vowels are
  therefore written "i", "u" (stressed when alone), "êh" and "ôh"; "á", "é" and "ó"
  are read as letters and stay. A test holds that no word of the dictionary, no
  letter and no output is one of those four letters on its own.
- **An acronym is spelled or said as a word by the dictionary, and by the shape of
  the syllables when it is not there.** `ReadsAsWord` takes a token in capitals as
  a word when it can be split into Portuguese syllables ("NASA", "SIM", "UFO")
  and as letters when it cannot ("GPU", "ALPR", "RTSP", "XYZ"); a name with no
  vowel in a domain ("ctrl.fit") is spelled. It is a heuristic: a wrong guess is
  fixed by a line in the dictionary, and `Validate` warns of every acronym that
  was guessed.
- **A quantity is a number and an identifier is a string.** A phone, a document, a
  plate, a protocol, an address of a network and a version are dictated digit by
  digit, because the listener has to write them down; "pedido 15" is a number
  and "protocolo 20261126-001" is not.
- **"Byte" is "báite".** Measured: "gigabyte" came back from whisper as
  "gigabits" and "Girobytes", "guigabáite" came back right with both voices. The
  rule is the last of a line, so the unit of "10 GB" and the word "gigabytes" typed
  by a model are said the same way.
- **The dictionary is shared.** `lexicon/ptbr.json` is the one list of how the
  products say a name ("ALPR.sh", "WhatsApp", "HIIT", "webhook"), embedded in the
  module, versioned (`LexiconVersion`) and part of `NormalizerVersion`; a program
  that is not written in Go reads it with `LexiconJSON`. An application adds its
  own words with `Speech.Lexicon` (or `ParseLexicon` on a file of the same format)
  and its words win. Nothing in it belongs to one business.
- **No number may change.** `Validate` reads every number, date, hour and
  identifier of the text a person reads, reads back the numbers the speech text
  says in words, and reports what was lost or altered (an error) or could be read
  two ways (a warning). A caller refuses the audio on `HasErrors`.

### What is not done

Roman numerals ("século XX"), scores ("2-1" stays as typed), a word that is a
homograph, hyphenated compounds with numbers, English text inside Portuguese (the
voice reads it with Portuguese phonemes), formulas, SSML (neither engine takes it),
IPA (espeak-ng reads the spelling, so the pronunciation is a respelling), and the grammatical gender of a noun that is not in the list: after an
unknown noun a number is masculine. Each is a line in a lexicon or a rule to add
with a test.

## 4. Writing for speech: a guideline for the system prompt

Normalization repairs what is written for the eye; it is not a license to write
for the eye. A model that knows its answer will be heard writes shorter, plainer
text, and the result is better than any rule can make of a table. The division of
labour is this: **the model writes the reply once, in plain notation, and the
code says it** (`Speakable`). The model does not respell words, spell acronyms or
write numbers in words: it could get a number wrong, and the number a person
hears about their own money or training must come from the code that added it up.

These are the 17 rules of the study of 26/11/2026 (`ESTUDO-normalizacao-fonetica-tts`)
as they apply to a model that writes a reply for this package. Paste them in the
system prompt of an assistant whose reply will be spoken (Control Fit, Corujão and
any other application that calls `Speak` with a reply):

```text
Sua resposta será falada em voz alta, em português do Brasil, e também lida na tela. Escreva para o ouvido. Um sistema converte o texto em fala: você escreve o texto normal e ele cuida da pronúncia.

1. Preserve o significado e as informações. Nunca altere, invente nem arredonde sem avisar um número, valor, data, hora, placa ou protocolo. Arredonde só o que é aproximação e diga "cerca de".
2. Siglas: escreva como são (CPF, IMC, API, GPU). O sistema soletra com os nomes das letras. Na primeira vez que usar uma sigla que o ouvinte pode não conhecer, diga o nome por extenso.
3. Siglas que se leem como palavra (NASA, PIX, SaaS, HIIT): escreva como são. Não escreva a pronúncia.
4. Estrangeirismos: escreva na grafia original (whey, deploy, login, check-in). Nunca invente grafia fonética, como "uêi" ou "diplói"; o dicionário do sistema faz isso.
5. Não use acentos nem letras trocadas para guiar a voz. Escreva a palavra certa, com a acentuação normal.
6. Números, valores, porcentagens, datas, horas e ordinais: escreva do jeito normal, em algarismos (R$ 24,90, 3x12, 22,5 kg, 45%, 06:30, 26/11/2026, 2º). Não escreva por extenso nem separado em letras.
7. Quantidade é número; identificador é código. Escreva telefone, placa, protocolo, IP e versão completos, com a palavra que diz o que são ("a placa ABC1D23", "o protocolo 20261126-001"). O sistema dita um a um.
8. Endereços: escreva e-mails e domínios completos (ajuda@ctrl.fit, alpr.sh). Para um link longo, não o repita: diga "o link está na conversa".
9. Unidades e abreviações: use as usuais (kg, km/h, min, kcal) ou por extenso. Evite abreviação que só você conhece.
10. Pontuação é a respiração: frases curtas, de até 20 palavras, com uma ideia cada, terminadas em ponto, interrogação ou exclamação. Vírgula onde uma pessoa respiraria.
11. Fale como conversa: "você", vocabulário do Brasil ("café da manhã", "treino", "celular"), sem listas, tópicos, tabelas, títulos, parênteses, travessões nem barras ("e/ou"). Se precisar enumerar, diga "primeiro", "depois" e "por fim". No máximo três números por resposta.
12. Escreva os nomes de produtos e marcas exatamente como o dicionário da organização os tem (Control Fit, ctrl.fit, WhatsApp).
13. Não invente pronúncia de marca que já tem pronúncia oficial.
14. Não altere dados técnicos: valores, unidades, nomes de exercícios, doses e prazos ficam como estão.
15. Preserve o que a voz já fala bem: palavras comuns em português, sem trocar por sinônimos estranhos nem por grafia especial.
16. Nada de instruções, comentários, Markdown, emojis, setas nem símbolos que seriam falados em voz alta ("asterisco", "seta"). Escreva só o que a pessoa deve ouvir.
17. Se um trecho pode ser lido de dois jeitos ("1/2", "24/7", "10h"), escreva de modo que sobre um só: "meia xícara", "atendimento 24 horas por dia, 7 dias por semana", "às 10h".

Comece pela resposta. Se a pessoa fez uma pergunta, responda na primeira frase. Termine com uma única pergunta ou próximo passo, nunca com uma despedida longa. Respostas faladas têm no máximo cerca de 75 palavras (meio minuto); se o assunto pede mais, resuma em voz e diga que o resto está por escrito.
```

The numbering is that of the study's section 8. Where the study asks the model to
do the work (spell the acronyms, respell the foreign words, put the accents, write
the numbers in words), this guideline leaves it to the code, which does it the same
way every time, and `Validate` checks that it did not change a number.

Even when the model follows this, keep the verbalizer on: it is the safety net for
the digit that gets through, and it is deterministic where the model is not. Do not
ask the model for a separate "spoken" field with the numbers already written out.
To catch the day the code or a text goes wrong, run the check before the audio is
made:

```go
speech, issues := n.Check(reply, time.Now()) // the text a voice would say, and what is wrong with it
if piper.HasErrors(issues) {
    // a number, a date, an hour, an identifier or a unit of the reply is not in the speech:
    // send the text without the audio, and look at issues
}
```

## 5. Measured with the real engines

### 0.5.0: six sentences

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
counts as errors although the voice said the right words.

### 0.6.0: the phonetic layer, 21 sentences

The same method, on 21 sentences that hold the hard cases of the study (acronyms,
domains, e-mails, foreign words, plates, IP addresses, versions, hours, units) and
of the products (supino `3x12` with `22,5 kg`, `ajuda@ctrl.fit`, HIIT, whey, IMC
`24,7`, RPE and RIR, an installment price), each in three versions: the raw text,
the speech of 0.5.0 and the speech of 0.6.0, with both voices. Each sentence has
the words that must be heard and a list of hard items (78 in all); an item counts
when the recognizer wrote it, in any of the spellings the sentence accepts.

| Hard items heard right | Voice | Raw text | 0.5.0 | 0.6.0 |
| --- | --- | --- | --- | --- |
| Acronyms (API, SDK, GPU, IMC, RPE, RIR, JSON, PIX, HIIT) | Kokoro | 9/15 | 7/15 | **13/15** |
| | Piper | 10/15 | 11/15 | 11/15 |
| Domains and e-mails | Kokoro | 0/8 | 0/8 | **5/8** |
| | Piper | 3/8 | 2/8 | 3/8 |
| Foreign words (whey, framework, webhook, login...) | Kokoro | 10/14 | 10/14 | 12/14 |
| | Piper | 10/14 | 12/14 | 10/14 |
| Identifiers (plate, IP, telephone, CPF) | Kokoro | 1/6 | 0/6 | 2/6 |
| | Piper | 0/6 | 2/6 | 2/6 |
| Numbers, units, hours | Kokoro | 20/35 | 25/35 | **33/35** |
| | Piper | 20/35 | 26/35 | **33/35** |
| **All 78 items** | Kokoro | 40 | 42 | **65 (83%)** |
| | Piper | 43 | 53 | **59 (76%)** |
| **Word error of the sentence** | Kokoro | 26.1% | 25.2% | **11.4%** |
| | Piper | 26.5% | 22.5% | **16.8%** |

Kokoro gained the most: the word error fell from 25% to 11% and the numbers and
units are heard right in 33 of 35 cases. Piper `faber` gained on numbers and units
(26 to 33 of 35) but little on the rest: it mis-hears the spelled acronyms, the
e-mails and the identifiers whatever the spelling.

What the measurement found, and changed:

- **A lone accented vowel is spelled out by the engine** ("í" is read "i agudo", "ê"
  "e circunflexo"): the first version of the letter names had them and whisper
  wrote "iagudo p192" for an IP address and "rpsircumflexo" for RPE. The letters
  are now written "i", "u", "êh" and "ôh" (see section 3); the second measurement
  has no such error.
- **"HIIT" as "hít" was read "ítch"** (the engine drops the h) and came back as
  "isso" or "itch"; "rít" came back as "hit" with both voices. **"webhook" as "uéb
  huk"** came back as "e book" and "iclope"; "uéb ruk" came back as "webhook" with
  Piper. **"gigabyte"** came back as "gigabits" and "Girobytes"; "guigabáite" came
  back right with both voices (the table counts "guigabaites" as heard right).
  Those spellings are in the dictionary and the units.
- **Not solved:** "whey" (neither "uêi" nor "uei" nor "ruêi" came back as "whey": the
  voices say "uêi" and the recognizer writes "way", "wei" or "aí", which a person
  hears as the word); the telephone and the CPF digit by digit, which whisper writes
  as a number ("cento e vinte e três") although the voice said the digits; a plate,
  of which whisper drops the first letter. The recognizer is the weak instrument
  for these, not necessarily the voice: **listen to them** before changing a
  spelling for a score.
- **A sample of one run**, 21 sentences, one recognizer: two runs of the same
  speech moved the totals by two to three points. It shows the direction and the
  kind of failure, not a precise rate. The sentences, the speech of each version,
  what the recognizer heard, the score and the scripts are in the evidence folder
  of Control Fit (`plans/evidence/voice-phonetic-2026-11-26/`).

## 6. Using it

The pieces a caller may want on their own: `LetterName`, `SpellAcronym`, `ReadsAsWord`,
`LexiconJSON`, `LexiconVersion`, `ParseLexicon`, `Normalizer.Check` and `Validate`.

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
