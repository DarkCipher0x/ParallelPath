// Package tone is ParallelPath's on-device tone model: a small linear classifier over hashed word and character
// n-grams, trained on Google's Civil Comments corpus (CC0; see tools/tonetrain). It scores a draft for toxicity,
// insults, threats and so on in well under a millisecond on any CPU, with nothing to load: the weights (about
// 1.8 MB, int8) are compiled into the engine. Everything runs on the device; no text ever leaves it.
//
// The same Features function is used for training and for inference, so the two can never drift apart.
package tone

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// Buckets is the size of the hashed feature space.
const Buckets = 1 << 18

// Labels are the model's outputs, in order (the Civil Comments columns).
var Labels = []string{"toxicity", "severe_toxicity", "obscene", "threat", "insult", "identity_attack", "sexual_explicit"}

// Outputs is len(Labels).
const Outputs = 7

// leet undoes the usual disguises ("@ss", "sh1t", "f*ck") before features are taken.
var leet = strings.NewReplacer("@", "a", "$", "s", "0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t", "*", "u")

// Normalize lower-cases, undoes disguises inside words, and squeezes long letter runs ("soooo" -> "soo").
func Normalize(s string) string {
	var b strings.Builder
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return unicode.IsSpace(r) })
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		// Only inside words that also have letters: plain numbers ("3:30", "50%") stay as they are.
		if strings.IndexFunc(w, unicode.IsLetter) >= 0 {
			w = leet.Replace(w)
		}
		var last rune
		run := 0
		for _, r := range w {
			if r == last {
				run++
				if run >= 2 && unicode.IsLetter(r) {
					continue
				}
			} else {
				last, run = r, 0
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Tokens splits normalised text into words (letters and digits; apostrophes kept inside words).
func Tokens(norm string) []string {
	return strings.FieldsFunc(norm, func(r rune) bool { return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'') })
}

func hash(kind byte, s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte{kind})
	h.Write([]byte(s))
	return h.Sum32() % Buckets
}

// Features are the hashed feature indices of a text: words, word pairs, and character 3-5-grams of each word
// (which catch misspellings and new forms of the same insult). Duplicates are kept: they count more.
func Features(text string) []uint32 {
	toks := Tokens(Normalize(text))
	out := make([]uint32, 0, len(toks)*8)
	for i, t := range toks {
		out = append(out, hash('w', t))
		if i > 0 {
			out = append(out, hash('b', toks[i-1]+" "+t))
		}
		w := []rune("<" + t + ">")
		for n := 3; n <= 5; n++ {
			for j := 0; j+n <= len(w); j++ {
				out = append(out, hash('c', string(w[j:j+n])))
			}
		}
	}
	return out
}

// Model is a trained tone model.
type Model struct {
	Bias  [Outputs]float32
	Scale [Outputs]float32
	W     []int8 // Buckets * Outputs, feature-major
}

// Logits are the raw outputs for a feature list (sum of weights over sqrt(count), plus bias).
func (m *Model) Logits(f []uint32) [Outputs]float32 {
	var acc [Outputs]int32
	for _, x := range f {
		row := m.W[int(x)*Outputs : int(x)*Outputs+Outputs]
		for k := range Outputs {
			acc[k] += int32(row[k])
		}
	}
	norm := float32(1)
	if len(f) > 0 {
		norm = float32(1 / math.Sqrt(float64(len(f))))
	}
	var out [Outputs]float32
	for k := range Outputs {
		out[k] = m.Bias[k] + float32(acc[k])*m.Scale[k]*norm
	}
	return out
}

// Scores are the probabilities (0-1) for each label.
func (m *Model) Scores(text string) [Outputs]float32 {
	l := m.Logits(Features(text))
	for k := range l {
		l[k] = float32(1 / (1 + math.Exp(-float64(l[k]))))
	}
	return l
}

// Culprits are the words in a text that push a label's score up most (to show the person what to change).
func (m *Model) Culprits(text string, label int, max int) []string {
	type cw struct {
		w string
		v float32
	}
	var list []cw
	seen := map[string]bool{}
	for _, t := range Tokens(Normalize(text)) {
		if seen[t] {
			continue
		}
		seen[t] = true
		f := Features(t)
		var v int32
		for _, x := range f {
			v += int32(m.W[int(x)*Outputs+label])
		}
		if s := float32(v) * m.Scale[label]; s > 0.6 {
			list = append(list, cw{t, s})
		}
	}
	for i := 1; i < len(list); i++ { // a handful of words: insertion sort
		for j := i; j > 0 && list[j].v > list[j-1].v; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	var out []string
	for i := 0; i < len(list) && i < max; i++ {
		out = append(out, list[i].w)
	}
	return out
}

const magic = "PPTONE1\x00"

// MarshalBinary writes the model: magic, then bias and scale (float32 LE), then the int8 weights.
func (m *Model) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, len(magic)+Outputs*8+len(m.W))
	b = append(b, magic...)
	for k := range Outputs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(m.Bias[k]))
	}
	for k := range Outputs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(m.Scale[k]))
	}
	for _, w := range m.W {
		b = append(b, byte(w))
	}
	return b, nil
}

// UnmarshalBinary reads a model written by MarshalBinary.
func (m *Model) UnmarshalBinary(b []byte) error {
	want := len(magic) + Outputs*8 + Buckets*Outputs
	if len(b) != want || string(b[:len(magic)]) != magic {
		return errors.New("tone: not a tone model")
	}
	p := len(magic)
	for k := range Outputs {
		m.Bias[k] = math.Float32frombits(binary.LittleEndian.Uint32(b[p:]))
		p += 4
	}
	for k := range Outputs {
		m.Scale[k] = math.Float32frombits(binary.LittleEndian.Uint32(b[p:]))
		p += 4
	}
	m.W = make([]int8, Buckets*Outputs)
	for i := range m.W {
		m.W[i] = int8(b[p+i])
	}
	return nil
}
