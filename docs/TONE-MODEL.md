# The tone model

ParallelPath checks the tone of what people write before it goes into the family's permanent record: messages,
edits, reasons, decline comments and change-request notes. It points out wording that may come across as hostile,
suggests a calmer way to put it and, where it can, offers a calmer version. The person always decides.

**Everything happens on the device.** The check runs inside ParallelPath's own engine on Windows, Android and in the
web app. No text is sent anywhere, there's no cloud AI and no model download, and it works offline.

## How it works

Two layers, combined:

1. **A trained model** (`engine/tone`) scores the text for seven kinds of hostility: toxicity, severe toxicity,
   obscenity, threats, insults, identity attacks and sexual content. It catches hostility even when it's phrased in
   ordinary words ("you are a complete failure as a mother").
2. **Co-parenting rules** (`engine/app/tone.go`) catch what a general model doesn't know about: blame ("you always",
   "your fault"), legal threats ("see you in court"), dismissiveness ("whatever"), shouting in capitals and "!!!".

The result is a level (calm, careful, heated), the specific words to reconsider with a suggestion for each, and a
**calmer version**. That version keeps the sentences about arrangements and drops the ones that attack, with
shouting and "!!!" toned down. It's a starting point the person edits; it's never sent by itself.

It also gives a **meter** from 0 (calm) to 1 (very hostile) for the live tone bar under the message box: the
highest of the model's insult, toxicity, threat (x1.4), obscene (x0.8) and identity-attack scores, plus 0.25 for
each insult or threat found, 0.15 for blame, dismissiveness, hostility or obscenity, and 0.08 for shouting or "!!!",
capped at 1. The apps check the draft 300 ms after the person stops typing; a check takes microseconds.

The **Advice** dialog shows the issues, the calmer version and writing tips, and answers questions from the built-in
help (BM25 search over the user guide and FAQ, `engine/help`): also on the device, with no cloud AI.

## The model

| | |
|---|---|
| Type | Multi-output logistic regression over hashed features |
| Features | Words, word pairs, and character 3-5-grams of each word (catch misspellings and disguises), hashed into 262,144 buckets; text is normalised first (lower case, "@ss"/"sh1t"-style disguises undone, long letter runs squeezed) |
| Weights | int8, one scale per output: **1.8 MB**, compiled into the engine |
| Speed | Loads in **about 2 ms** (once); checks a message in **about 8 µs** on one CPU core (AMD Ryzen 9 5950X) |
| Code | `engine/tone/tone.go`: the same `Features` function is used for training and on the device |

### Why the CPU, not a GPU or NPU

The model is small enough that a GPU or NPU wouldn't make it faster. Handing work to an accelerator (DirectML,
NNAPI, WebGPU) costs more time to set up than the whole check takes on the CPU. A compiled-in model on the CPU is
instant, the same on every device, and needs no drivers. The engine's interface leaves room for a larger
accelerated model if one is ever worth it.

### Android's on-device AI (not used)

Gemini Nano through Google's ML Kit GenAI APIs runs on the phone itself. But Google's ML Kit data disclosure says
the SDK sends device and app information, installation identifiers and usage metrics to Google, with no setting to
turn that off. That conflicts with ParallelPath's rule that nothing leaves the device, so ML Kit isn't used.
Android uses the same built-in model as every other platform.

## Training data

- **Google's Civil Comments** corpus (https://huggingface.co/datasets/google/civil_comments), released under **CC0
  1.0** (public domain): 1,804,874 training comments, 97,320 validation and 97,320 test, each labelled by several
  human raters with the share who chose each label.
- Downloaded once to the developer's PC (`%LOCALAPPDATA%\ParallelPath-tone-data`, 422 MB of parquet files). The
  corpus is never shipped; only the trained weights are.

## Training

`tools/tonetrain` (its own Go module; it isn't part of any app or the server):

```
go run ./tools/tonetrain <data folder> engine/tone/model.bin
```

- Soft labels (the share of raters), binary cross-entropy, with positive examples weighted up for the rare labels.
- AdaGrad, learning rate 0.15, L2 1e-7, 4 epochs over the training split, using every CPU core. It takes about 30
  seconds.
- Quantised to int8 per output, and evaluated *after* quantisation, exactly as the engine uses it.

## Results (held-out test split, 97,320 comments)

ROC-AUC per label ("positive" = at least half the raters chose the label):

| Label | AUC | Test positives |
|---|---|---|
| toxicity | 0.949 | 7,777 |
| insult | 0.961 | 5,776 |
| threat | 0.979 | 221 |
| identity attack | 0.971 | 687 |
| obscene | 0.983 | 543 |
| sexual content | 0.986 | 240 |

(Severe toxicity has no test comments at the 50% rater threshold, so it's only used together with the others.)

Cut-offs in the engine (`toneCutoffs` in `engine/app/tone.go`): insult and toxicity 40% (precision about 0.68,
recall about 0.56), threat 30%, obscene and sexual content 40%, identity attack 50%. At these levels most ordinary
messages aren't flagged, and a flag is a prompt to look again, never a block.

The current `model.bin` has SHA-256 `58e3d2d95fd0b91b4d13f0424256c104827dee6715eb19d31a0605bbb7c36980`. Training
uses parallel updates, so each run's weights differ slightly; retraining gives the same quality, not the same file.

## Limits

- Civil Comments are comments on news articles, not co-parenting messages. The co-parenting rules cover the
  difference, and the model is a second pair of eyes, not a judge.
- English only.
- Sarcasm and politely worded hostility can get through. Very blunt but fair messages can occasionally be flagged.
- The check can be turned off in Settings (it's on by default).

## Tests

`engine/tone/tone_test.go` (calm messages score low, hostile ones high, disguises are undone, culprit words found,
plus a benchmark) and `engine/app/tone_test.go` (levels, rules, the model catching plain-word hostility, the calmer
version).
