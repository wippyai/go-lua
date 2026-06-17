package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// DynamicIndexReadbackIntent describes which dynamic-index evidence a later
// applicator should read back after resolving the source paths/products.
type DynamicIndexReadbackIntent uint8

const (
	DynamicIndexReadbackUnknown DynamicIndexReadbackIntent = iota
	DynamicIndexReadbackNone
	DynamicIndexReadbackKey
	DynamicIndexReadbackValue
	DynamicIndexReadbackKeyAndValue
)

// DynamicIndexWrite describes a write through a dynamic table index at a CFG
// point. Key/value products remain unresolved ValueSource evidence here.
type DynamicIndexWrite struct {
	tablePath path.Path
	keySource ValueSource
	source    ValueSource

	admission      dynamicindex.Admission
	readbackIntent DynamicIndexReadbackIntent
}

// NewDynamicIndexWrite creates a dynamic-index write event.
func NewDynamicIndexWrite(
	tablePath path.Path,
	keySource ValueSource,
	source ValueSource,
	admission dynamicindex.Admission,
	readbackIntent DynamicIndexReadbackIntent,
) DynamicIndexWrite {
	return DynamicIndexWrite{
		tablePath:      tablePath.Clone(),
		keySource:      keySource,
		source:         source,
		admission:      admission,
		readbackIntent: readbackIntent,
	}
}

// TablePath returns the table path receiving the dynamic index write.
func (w DynamicIndexWrite) TablePath() path.Path { return w.tablePath.Clone() }

// KeySource returns the source evidence for the dynamic key.
func (w DynamicIndexWrite) KeySource() ValueSource { return w.keySource }

// Source returns the source evidence for the value written at the dynamic key.
func (w DynamicIndexWrite) Source() ValueSource { return w.source }

// Admission returns the typed admission intent for this write.
func (w DynamicIndexWrite) Admission() dynamicindex.Admission { return w.admission }

// ReadbackIntent returns the typed post-write readback intent for this write.
func (w DynamicIndexWrite) ReadbackIntent() DynamicIndexReadbackIntent {
	return w.readbackIntent
}

func (w DynamicIndexWrite) copy() DynamicIndexWrite {
	w.tablePath = w.tablePath.Clone()
	return w
}

func copyDynamicIndexWriteMap(in map[cfg.Point]DynamicIndexWrite) map[cfg.Point]DynamicIndexWrite {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]DynamicIndexWrite, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}
