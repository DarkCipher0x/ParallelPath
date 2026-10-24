// Package help is ParallelPath's built-in help assistant: ask a question in your own words and it finds the
// answer in ParallelPath's own guide and FAQ. It runs on the device (Windows, Android and the web app alike), in a
// few milliseconds, with no network and no generative model: answers are always the guide's own words, so it
// can't make anything up.
//
// The knowledge (kb/*.md) is a copy of docs/USER-GUIDE.md and docs/FAQ.md; TestKnowledgeMatchesDocs keeps them
// identical, and tools/build-release.ps1 refreshes the copy.
package help

import (
	"embed"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

//go:embed kb/*.md
var kb embed.FS

// Answer is one result.
type Answer struct {
	Title   string  `json:"title"`   // the FAQ question, or the guide section's heading
	Text    string  `json:"text"`    // the answer (plain text; **bold** and `code` markers removed)
	Source  string  `json:"source"`  // "FAQ" or "User guide"
	Section string  `json:"section"` // the guide section it belongs to, e.g. "11. Voice and video calls"
	Score   float64 `json:"score"`
}

type doc struct {
	Answer
	terms  map[string]int
	length int
	title  map[string]bool
}

type index struct {
	docs []doc
	df   map[string]int
	avg  float64
}

var (
	once sync.Once
	idx  *index
)

func load() *index {
	once.Do(func() {
		ix := &index{df: map[string]int{}}
		for _, f := range []struct{ file, source string }{{"kb/FAQ.md", "FAQ"}, {"kb/USER-GUIDE.md", "User guide"}} {
			b, err := kb.ReadFile(f.file)
			if err != nil {
				continue
			}
			for _, a := range split(string(b), f.source) {
				d := doc{Answer: a, terms: map[string]int{}, title: map[string]bool{}}
				for _, t := range terms(a.Title + " " + a.Text) {
					d.terms[t]++
					d.length++
				}
				for _, t := range terms(a.Title) {
					d.title[t] = true
				}
				for t := range d.terms {
					ix.df[t]++
				}
				ix.docs = append(ix.docs, d)
			}
		}
		total := 0
		for _, d := range ix.docs {
			total += d.length
		}
		ix.avg = float64(total) / math.Max(1, float64(len(ix.docs)))
		idx = ix
	})
	return idx
}

var heading = regexp.MustCompile(`^(#{2,3}) (.+)$`)

// split cuts a guide into sections (## and ###); in the FAQ each ### is a question and its answer.
func split(md, source string) []Answer {
	var out []Answer
	var cur *Answer
	var body []string
	section := ""
	flush := func() {
		if cur != nil {
			cur.Text = clean(strings.Join(body, "\n"))
			if cur.Text != "" {
				out = append(out, *cur)
			}
		}
		cur, body = nil, nil
	}
	for _, l := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if m := heading.FindStringSubmatch(l); m != nil {
			flush()
			if m[1] == "##" {
				section = m[2]
			}
			if strings.EqualFold(m[2], "Contents") {
				continue
			}
			cur = &Answer{Title: m[2], Source: source, Section: section}
			continue
		}
		if cur != nil {
			body = append(body, l)
		}
	}
	flush()
	return out
}

var (
	mdLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	blanks = regexp.MustCompile(`\n{3,}`)
)

func clean(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("**", "", "`", "", "---", "").Replace(s)
	return strings.TrimSpace(blanks.ReplaceAllString(s, "\n\n"))
}

// synonyms map everyday words to the guide's own.
var synonyms = map[string][]string{
	"ex": {"parent"}, "partner": {"parent"}, "mum": {"parent"}, "mom": {"parent"}, "dad": {"parent"}, "father": {"parent"},
	"mother": {"parent"}, "kid": {"child"}, "kids": {"children", "child"}, "son": {"child"}, "daughter": {"child"},
	"password": {"passphrase"}, "pin": {"passphrase"}, "forgot": {"forget"}, "phone": {"device", "android"},
	"mobile": {"device", "android"}, "pc": {"windows", "device"}, "computer": {"windows", "device"}, "laptop": {"windows", "device"},
	"browser": {"web"}, "website": {"web"}, "delete": {"remove", "withdraw"}, "erase": {"remove"}, "unsend": {"withdraw"},
	"change": {"edit"}, "fix": {"edit"}, "lawyer": {"professional", "solicitor"}, "attorney": {"professional", "solicitor"},
	"mediator": {"professional"}, "court": {"report"}, "judge": {"report"}, "evidence": {"report", "signed"}, "pdf": {"report"},
	"export": {"report"}, "print": {"report"}, "money": {"expense"}, "pay": {"expense", "payment"}, "owe": {"expense", "balance"},
	"bill": {"expense"}, "receipt": {"expense"}, "schedule": {"calendar"}, "custody": {"parenting", "schedule"},
	"visitation": {"parenting", "schedule"}, "swap": {"change", "request"}, "holiday": {"event", "calendar"},
	"appointment": {"event"}, "remind": {"reminder"}, "alert": {"notification"}, "notify": {"notification"},
	"ring": {"call"}, "phonecall": {"call"}, "facetime": {"video", "call"}, "chat": {"message"}, "text": {"message"},
	"dm": {"private", "conversation"}, "private": {"conversation"}, "rude": {"tone"}, "angry": {"tone"}, "nasty": {"tone"},
	"hostile": {"tone"}, "abuse": {"tone"}, "secure": {"encrypt"}, "safe": {"encrypt"}, "hack": {"encrypt", "security"},
	"spy": {"encrypt"}, "read": {"see"}, "file": {"document"}, "photo": {"document", "attachment"}, "picture": {"attachment"},
	"diary": {"journal"}, "note": {"journal"}, "allergy": {"info"}, "doctor": {"info"}, "gp": {"info"}, "school": {"info"},
	"lost": {"recovery"}, "stolen": {"recovery", "revoke"}, "new": {"link"}, "second": {"link"}, "another": {"link"},
	"certificate": {"certificate", "fingerprint"}, "warning": {"certificate", "trust"}, "invite": {"invite", "code"},
	"join": {"invite"}, "permission": {"permission", "allow"}, "allow": {"permission"}, "block": {"permission"},
}

var stop = map[string]bool{"a": true, "an": true, "the": true, "i": true, "my": true, "me": true, "to": true, "do": true, "how": true,
	"can": true, "is": true, "it": true, "of": true, "and": true, "or": true, "in": true, "on": true, "for": true, "what": true,
	"with": true, "be": true, "are": true, "does": true, "if": true, "this": true, "that": true, "you": true, "your": true, "we": true,
	"our": true, "at": true, "so": true, "there": true, "way": true, "want": true, "need": true, "get": true, "not": true, "from": true}

// stem trims common English endings so "calls", "calling" and "called" meet.
func stem(w string) string {
	for _, suf := range []string{"ing", "ed", "es", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			return strings.TrimSuffix(w, suf)
		}
	}
	return w
}

func terms(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stop[w] || len(w) < 2 {
			continue
		}
		out = append(out, stem(w))
	}
	return out
}

func queryTerms(q string) map[string]float64 {
	out := map[string]float64{}
	for _, w := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stop[w] || len(w) < 2 {
			continue
		}
		out[stem(w)] = 1
		for _, s := range synonyms[w] {
			if _, have := out[stem(s)]; !have {
				out[stem(s)] = 0.6 // a synonym counts, a little less than the word itself
			}
		}
	}
	return out
}

// Ask returns up to max answers to a question, best first (BM25, with FAQ questions and section titles that match
// counting extra). Nothing found: an empty list.
func Ask(question string, max int) []Answer {
	ix := load()
	if ix == nil || len(ix.docs) == 0 {
		return nil
	}
	q := queryTerms(question)
	if len(q) == 0 {
		return []Answer{}
	}
	const k1, b = 1.4, 0.75
	n := float64(len(ix.docs))
	type hit struct {
		i int
		s float64
	}
	var hits []hit
	for i, d := range ix.docs {
		var s float64
		matched := 0
		for t, w := range q {
			tf := float64(d.terms[t])
			if tf == 0 {
				continue
			}
			matched++
			idf := math.Log(1 + (n-float64(ix.df[t])+0.5)/(float64(ix.df[t])+0.5))
			s += w * idf * tf * (k1 + 1) / (tf + k1*(1-b+b*float64(d.length)/ix.avg))
			if d.title[t] {
				s += w * idf * 0.8
			}
		}
		if s <= 0 {
			continue
		}
		if d.Source == "FAQ" {
			s *= 1.25
		}
		s *= 0.6 + 0.4*float64(matched)/float64(len(q)) // more of the question covered, better
		hits = append(hits, hit{i, s})
	}
	sort.Slice(hits, func(a, c int) bool { return hits[a].s > hits[c].s })
	out := []Answer{}
	for _, h := range hits {
		if len(out) == max {
			break
		}
		a := ix.docs[h.i].Answer
		a.Score = math.Round(h.s*100) / 100
		out = append(out, a)
	}
	return out
}
