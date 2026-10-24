package tone

import (
	_ "embed"
	"sync"
)

// model.bin is produced by tools/tonetrain (see docs/TONE-MODEL.md for the training data, settings and results).
//
//go:embed model.bin
var modelBin []byte

var (
	once     sync.Once
	defModel *Model
)

// Default is the built-in model, parsed on first use (about a millisecond).
func Default() *Model {
	once.Do(func() {
		m := &Model{}
		if m.UnmarshalBinary(modelBin) == nil {
			defModel = m
		}
	})
	return defModel
}
