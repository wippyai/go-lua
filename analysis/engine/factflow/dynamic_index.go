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
	keyPath   path.Path
	valuePath path.Path
	keySource ValueSource
	source    ValueSource

	admission      dynamicindex.Admission
	readbackIntent DynamicIndexReadbackIntent
	hasKeyPath     bool
	hasValuePath   bool
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

// TablePathRef returns the dynamic-index table path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (w DynamicIndexWrite) TablePathRef() path.Path { return w.tablePath }

// WithKeyPath returns a copy carrying the statically resolved path for the
// dynamic key operand, when the key expression itself has stable identity.
func (w DynamicIndexWrite) WithKeyPath(keyPath path.Path) DynamicIndexWrite {
	if keyPath.IsEmpty() {
		w.keyPath = path.Path{}
		w.hasKeyPath = false
		return w
	}
	w.keyPath = keyPath.Clone()
	w.hasKeyPath = true
	return w
}

// KeyPath returns the statically resolved path for the dynamic key operand.
func (w DynamicIndexWrite) KeyPath() (path.Path, bool) {
	if !w.hasKeyPath {
		return path.Path{}, false
	}
	return w.keyPath.Clone(), true
}

// KeyPathRef returns the dynamic key path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (w DynamicIndexWrite) KeyPathRef() (path.Path, bool) {
	if !w.hasKeyPath {
		return path.Path{}, false
	}
	return w.keyPath, true
}

// WithValuePath returns a copy carrying the statically resolved path for the
// value operand, when the assigned value expression itself has stable identity.
func (w DynamicIndexWrite) WithValuePath(valuePath path.Path) DynamicIndexWrite {
	if valuePath.IsEmpty() {
		w.valuePath = path.Path{}
		w.hasValuePath = false
		return w
	}
	w.valuePath = valuePath.Clone()
	w.hasValuePath = true
	return w
}

// ValuePath returns the statically resolved path for the assigned value operand.
func (w DynamicIndexWrite) ValuePath() (path.Path, bool) {
	if !w.hasValuePath {
		return path.Path{}, false
	}
	return w.valuePath.Clone(), true
}

// ValuePathRef returns the assigned value path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (w DynamicIndexWrite) ValuePathRef() (path.Path, bool) {
	if !w.hasValuePath {
		return path.Path{}, false
	}
	return w.valuePath, true
}

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
	w.keyPath = w.keyPath.Clone()
	w.valuePath = w.valuePath.Clone()
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
