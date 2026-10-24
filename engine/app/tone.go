package app

import (
	"regexp"
	"strings"
	"unicode"

	"pp/engine/tone"
)

// ToneIssue is one thing in a draft message that may come across as hostile, with a gentler way to put it.
type ToneIssue struct {
	Kind       string `json:"kind"`  // shouting | punctuation | insult | blame | threat | dismissive | hostile | obscene
	Match      string `json:"match"` // the words in the draft
	Suggestion string `json:"suggestion"`
}

// ToneResult is the tone check of a draft. It runs on the device (the built-in model plus a few co-parenting
// rules): the draft never leaves it.
type ToneResult struct {
	Issues []ToneIssue `json:"issues"`
	Level  string      `json:"level"` // calm | careful | heated
	// Meter is the tone on a 0-1 scale for a live bar as the person types (0 calm, 1 very hostile).
	Meter float32 `json:"meter"`
	// Scores are the model's probabilities (0-1) per label (tone.Labels), for anyone curious.
	Scores map[string]float32 `json:"scores,omitempty"`
	// Calmer is a suggested version: the sentences that attack dropped, shouting and "!!!" toned down. Empty when
	// there's nothing to change or nothing would be left.
	Calmer string `json:"calmer,omitempty"`
}

type tonePattern struct {
	kind, suggestion string
	re               *regexp.Regexp
}

func tp(kind, suggestion string, words ...string) tonePattern {
	return tonePattern{kind, suggestion, regexp.MustCompile(`(?i)\b(` + strings.Join(words, "|") + `)\b`)}
}

const (
	suggestPerson   = "Describe the problem, not the person."
	suggestBlame    = "Try \"I'd like…\" or \"Can we…\" - it keeps the focus on what happens next."
	suggestThreat   = "Leave legal steps to your solicitor; keep messages to arrangements."
	suggestDismiss  = "A short reason helps the other parent understand."
	suggestLanguage = "Messages are part of the family's permanent record - plain words come across better."
)

// The rules catch what a general model doesn't know about co-parenting: blame, legal threats, dismissiveness. The
// model (engine/tone) catches insults and hostility however they're phrased.
var tonePatterns = []tonePattern{
	tp("insult", suggestPerson,
		`idiot`, `stupid`, `moron`, `pathetic`, `useless`, `liar`, `lying`, `selfish`, `disgusting`, `incompetent`, `crazy`, `psycho`,
		`loser`, `joke`, `clueless`, `lazy`, `bitch`, `bastard`, `prick`, `twat`, `dickhead`, `asshole`, `arsehole`, `f+u+c+k+\w*`, `shit\w*`, `crap`),
	tp("blame", suggestBlame,
		`you always`, `you never`, `your fault`, `you don't care`, `you do not care`, `typical of you`, `as usual`, `how dare you`,
		`what is wrong with you`, `what's wrong with you`, `you ruined`, `because of you`),
	tp("threat", suggestThreat,
		`or else`, `you'll regret`, `you will regret`, `see you in court`, `my lawyer will`, `my solicitor will`, `you'll never see`,
		`you will never see`, `i'll make sure you`, `i will make sure you`, `watch yourself`),
	tp("dismissive", suggestDismiss,
		`whatever`, `shut up`, `get over it`, `not my problem`, `deal with it`, `grow up`, `don't bother`, `do not bother`),
}

var (
	heavyPunct = regexp.MustCompile(`[!?]{3,}`)
	wordRe     = regexp.MustCompile(`\p{L}{3,}`)
	sentenceRe = regexp.MustCompile(`[^.!?\n]+[.!?]*\s*`)
)

// Model cut-offs, chosen on the held-out test set (docs/TONE-MODEL.md): about 2 in 3 flags are right at these
// levels, and ordinary messages are rarely flagged; the person always decides.
var toneCutoffs = []struct {
	label      int
	at, heated float32
	kind, sug  string
}{
	{4, 0.40, 0.75, "insult", suggestPerson},    // insult
	{0, 0.40, 0.80, "hostile", suggestPerson},   // toxicity
	{3, 0.30, 0.60, "threat", suggestThreat},    // threat
	{2, 0.40, 0.80, "obscene", suggestLanguage}, // obscene
	{5, 0.50, 0.80, "insult", suggestPerson},    // identity attack
	{6, 0.40, 0.80, "obscene", suggestLanguage}, // sexual
}

// CheckTone looks at a draft for wording likely to escalate things. It never blocks sending; the app shows the
// issues and the person decides. Typical time: well under a millisecond.
func CheckTone(text string) ToneResult {
	res := ToneResult{Issues: []ToneIssue{}}
	seen := map[string]bool{}
	hot := 0
	add := func(kind, match, suggestion string) {
		k := kind + "/" + strings.ToLower(match)
		if !seen[k] {
			seen[k] = true
			res.Issues = append(res.Issues, ToneIssue{Kind: kind, Match: match, Suggestion: suggestion})
		}
	}
	for _, p := range tonePatterns {
		for _, m := range p.re.FindAllString(text, -1) {
			add(p.kind, m, p.suggestion)
			if p.kind == "insult" || p.kind == "threat" {
				hot++
			}
		}
	}
	if m := tone.Default(); m != nil && strings.TrimSpace(text) != "" {
		scores := m.Scores(text)
		res.Scores = map[string]float32{}
		for k, l := range tone.Labels {
			res.Scores[l] = scores[k]
		}
		for _, c := range toneCutoffs {
			s := scores[c.label]
			if s < c.at {
				continue
			}
			words := m.Culprits(text, c.label, 3)
			match := strings.Join(words, ", ")
			if match == "" {
				match = "the message as a whole"
			}
			add(c.kind, match, c.sug)
			if s >= c.heated {
				hot++
			}
		}
	}
	// Shouting: three or more words in capitals, or most of a longer message.
	var caps []string
	words := wordRe.FindAllString(text, -1)
	for _, w := range words {
		if isUpper(w) {
			caps = append(caps, w)
		}
	}
	if len(caps) >= 3 || (len(words) >= 4 && len(caps)*2 > len(words)) {
		add("shouting", strings.Join(caps[:min(len(caps), 4)], " "), "Capitals read as shouting - normal case comes across calmer.")
	}
	if m := heavyPunct.FindString(text); m != "" {
		add("punctuation", m, "One ! or ? is enough.")
	}
	switch {
	case len(res.Issues) == 0:
		res.Level = "calm"
	case hot > 0 || len(res.Issues) >= 3:
		res.Level = "heated"
	default:
		res.Level = "careful"
	}
	res.Meter = meter(res)
	if len(res.Issues) > 0 {
		res.Calmer = calmer(text)
	}
	return res
}

// meter blends the model's scores and the rules into one 0-1 figure for the live tone bar.
func meter(r ToneResult) float32 {
	var m float32
	if s := r.Scores; s != nil {
		m = max(s["insult"], s["toxicity"], min(1, s["threat"]*1.4), s["obscene"]*0.8, s["identity_attack"])
	}
	for _, i := range r.Issues {
		switch i.Kind {
		case "insult", "threat":
			m += 0.25
		case "blame", "dismissive", "hostile", "obscene":
			m += 0.15
		default: // shouting, punctuation
			m += 0.08
		}
	}
	return min(1, m)
}

// calmer keeps the sentences that are about arrangements and drops the ones that attack (by the rules or the
// model), then tones down capitals and "!!!". It's a starting point the person edits - never sent by itself.
func calmer(text string) string {
	m := tone.Default()
	var kept []string
	for _, s := range sentenceRe.FindAllString(text, -1) {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		drop := false
		for _, p := range tonePatterns {
			if p.re.MatchString(t) && p.kind != "dismissive" {
				drop = true
			}
		}
		if m != nil && !drop {
			sc := m.Scores(t)
			drop = sc[4] >= 0.5 || sc[0] >= 0.5 || sc[3] >= 0.4
		}
		if !drop {
			kept = append(kept, soften(t))
		}
	}
	out := strings.TrimSpace(strings.Join(kept, " "))
	if out == "" || out == strings.TrimSpace(text) {
		return ""
	}
	return out
}

// soften lower-cases shouted words and squeezes "!!!" / "???".
func soften(s string) string {
	s = heavyPunct.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "?") {
			return "?"
		}
		return "."
	})
	// A sentence that's mostly capitals is shouted as a whole: write it normally.
	up, letters := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				up++
			}
		}
	}
	if letters >= 6 && up*10 > letters*6 {
		r := []rune(strings.ToLower(s))
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	}
	return wordRe.ReplaceAllStringFunc(s, func(w string) string {
		if isUpper(w) && len(w) > 3 {
			return strings.ToLower(w)
		}
		return w
	})
}

func isUpper(w string) bool {
	for _, r := range w {
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}
