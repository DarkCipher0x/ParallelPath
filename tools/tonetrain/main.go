// Command tonetrain trains the on-device tone model (engine/tone) from Google's Civil Comments corpus (CC0 1.0,
// https://huggingface.co/datasets/google/civil_comments). It runs on a developer PC only; the corpus is never
// shipped, only the trained weights (engine/tone/model.bin).
//
//	tonetrain DATA_DIR OUT_MODEL
//
// DATA_DIR holds the corpus's parquet files (train-*, validation-*, test-*). Training: multi-output logistic
// regression over engine/tone's hashed features, soft labels (the share of raters who chose each label), AdaGrad,
// several epochs, then int8 quantisation. It prints ROC-AUC per label on the held-out test split, for the
// quantised model exactly as the engine will use it.
package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"

	"pp/engine/tone"
)

type row struct {
	Text           string  `parquet:"text"`
	Toxicity       float32 `parquet:"toxicity"`
	SevereToxicity float32 `parquet:"severe_toxicity"`
	Obscene        float32 `parquet:"obscene"`
	Threat         float32 `parquet:"threat"`
	Insult         float32 `parquet:"insult"`
	IdentityAttack float32 `parquet:"identity_attack"`
	SexualExplicit float32 `parquet:"sexual_explicit"`
}

func (r *row) labels() [tone.Outputs]float32 {
	return [tone.Outputs]float32{r.Toxicity, r.SevereToxicity, r.Obscene, r.Threat, r.Insult, r.IdentityAttack, r.SexualExplicit}
}

type sample struct {
	f []uint32
	y [tone.Outputs]float32
}

func load(dir, prefix string) []sample {
	files, _ := filepath.Glob(filepath.Join(dir, prefix+"-*.parquet"))
	slices.Sort(files)
	var out []sample
	for _, f := range files {
		rows, err := parquet.ReadFile[row](f)
		if err != nil {
			fail(err)
		}
		// Feature extraction in parallel: the bulk of the work.
		part := make([]sample, len(rows))
		var wg sync.WaitGroup
		n := runtime.NumCPU()
		for w := range n {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := w; i < len(rows); i += n {
					part[i] = sample{f: tone.Features(rows[i].Text), y: rows[i].labels()}
				}
			}(w)
		}
		wg.Wait()
		out = append(out, part...)
		fmt.Printf("loaded %s: %d rows\n", filepath.Base(f), len(rows))
	}
	return out
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: tonetrain DATA_DIR OUT_MODEL")
		os.Exit(2)
	}
	dir, outPath := os.Args[1], os.Args[2]
	t0 := time.Now()
	train := load(dir, "train")
	test := load(dir, "test")
	fmt.Printf("features ready in %s\n", time.Since(t0).Round(time.Second))

	// Toxic comments are rare (about 8%); weight positive targets up so the rare labels (threat) are learned.
	var pos [tone.Outputs]float64
	for _, s := range train {
		for k := range tone.Outputs {
			pos[k] += float64(s.y[k])
		}
	}
	var wpos [tone.Outputs]float32
	for k := range tone.Outputs {
		p := pos[k] / float64(len(train))
		wpos[k] = float32(math.Min(20, math.Max(1, math.Sqrt((1-p)/math.Max(p, 1e-6)))))
		fmt.Printf("%-16s base rate %.4f, positive weight %.1f\n", tone.Labels[k], p, wpos[k])
	}

	w := make([]float32, tone.Buckets*tone.Outputs)
	g := make([]float32, tone.Buckets*tone.Outputs) // AdaGrad accumulators
	var bias, gb [tone.Outputs]float32
	const lr, l2 = 0.15, 1e-7
	epochs := 4
	order := make([]int, len(train))
	for i := range order {
		order[i] = i
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for ep := range epochs {
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		var wg sync.WaitGroup
		n := runtime.NumCPU()
		var lossMu sync.Mutex
		var loss float64
		for wk := range n { // Hogwild: workers update the shared weights without locks (sparse updates rarely collide)
			wg.Add(1)
			go func(wk int) {
				defer wg.Done()
				var l float64
				for i := wk; i < len(order); i += n {
					s := &train[order[i]]
					if len(s.f) == 0 {
						continue
					}
					norm := float32(1 / math.Sqrt(float64(len(s.f))))
					var z [tone.Outputs]float32
					for _, x := range s.f {
						base := int(x) * tone.Outputs
						for k := range tone.Outputs {
							z[k] += w[base+k]
						}
					}
					var grad [tone.Outputs]float32
					for k := range tone.Outputs {
						p := 1 / (1 + float32(math.Exp(-float64(bias[k]+z[k]*norm))))
						y := s.y[k]
						cw := 1 + (wpos[k]-1)*y
						grad[k] = (p - y) * cw
						l += -float64(cw) * (float64(y)*math.Log(float64(p)+1e-7) + float64(1-y)*math.Log(float64(1-p)+1e-7))
					}
					for k := range tone.Outputs {
						gb[k] += grad[k] * grad[k]
						bias[k] -= lr * grad[k] / float32(math.Sqrt(float64(gb[k])+1e-6))
					}
					for _, x := range s.f {
						base := int(x) * tone.Outputs
						for k := range tone.Outputs {
							gk := grad[k]*norm + l2*w[base+k]
							g[base+k] += gk * gk
							w[base+k] -= lr * gk / float32(math.Sqrt(float64(g[base+k])+1e-6))
						}
					}
				}
				lossMu.Lock()
				loss += l
				lossMu.Unlock()
			}(wk)
		}
		wg.Wait()
		fmt.Printf("epoch %d: loss %.4f (%s)\n", ep+1, loss/float64(len(train)), time.Since(t0).Round(time.Second))
	}

	// Quantise to int8 per output, as the engine stores it.
	m := &tone.Model{W: make([]int8, len(w))}
	for k := range tone.Outputs {
		var max float32
		for i := k; i < len(w); i += tone.Outputs {
			max = float32(math.Max(float64(max), math.Abs(float64(w[i]))))
		}
		m.Scale[k] = max / 127
		m.Bias[k] = bias[k]
		for i := k; i < len(w); i += tone.Outputs {
			m.W[i] = int8(math.Round(float64(w[i] / m.Scale[k])))
		}
	}
	b, _ := m.MarshalBinary()
	if err := os.WriteFile(outPath, b, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", outPath, len(b))

	// Held-out test: ROC-AUC per label (binary truth: half or more of the raters chose the label).
	for k := range tone.Outputs {
		type sc struct {
			s   float32
			pos bool
		}
		list := make([]sc, 0, len(test))
		for _, s := range test {
			list = append(list, sc{m.Logits(s.f)[k], s.y[k] >= 0.5})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].s < list[j].s })
		var np, nn, rank float64
		for i, x := range list {
			if x.pos {
				np++
				rank += float64(i + 1)
			} else {
				nn++
			}
		}
		auc := (rank - np*(np+1)/2) / math.Max(np*nn, 1)
		fmt.Printf("test %-16s AUC %.4f (%d positives)\n", tone.Labels[k], auc, int(np))
		// Precision and recall at probability thresholds, for choosing the engine's cut-offs.
		for _, th := range []float64{0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9} {
			logit := float32(math.Log(th / (1 - th)))
			var tp, fp float64
			for _, x := range list {
				if x.s >= logit {
					if x.pos {
						tp++
					} else {
						fp++
					}
				}
			}
			fmt.Printf("    p>=%.2f precision %.2f recall %.2f flagged %.2f%%\n", th, tp/math.Max(tp+fp, 1), tp/math.Max(np, 1),
				100*(tp+fp)/float64(len(list)))
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tonetrain:", err)
	os.Exit(1)
}
