package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// DynamicIndexTarget is the single typed identity of a write through a
// dynamic member, including any static suffix after the key.
type DynamicIndexTarget struct {
	tablePath path.Path
	keySource ValueSource
	suffix    []segment.Segment
	valid     bool
}

func NewDynamicIndexTarget(tablePath path.Path, keySource ValueSource, suffix []segment.Segment) DynamicIndexTarget {
	return DynamicIndexTarget{
		tablePath: tablePath.Clone(), keySource: keySource,
		suffix: append([]segment.Segment(nil), suffix...),
		valid:  !tablePath.IsEmpty() && keySource.Valid(),
	}
}

func (t DynamicIndexTarget) Valid() bool {
	return t.valid && !t.tablePath.IsEmpty() && t.keySource.Valid()
}
func (t DynamicIndexTarget) TablePathRef() path.Path      { return t.tablePath }
func (t DynamicIndexTarget) KeySource() ValueSource       { return t.keySource }
func (t DynamicIndexTarget) SuffixRef() []segment.Segment { return t.suffix }
func (t DynamicIndexTarget) Equal(other DynamicIndexTarget) bool {
	if !t.Valid() || !other.Valid() || !t.tablePath.Equal(other.tablePath) || t.keySource != other.keySource || len(t.suffix) != len(other.suffix) {
		return false
	}
	for index := range t.suffix {
		if t.suffix[index] != other.suffix[index] {
			return false
		}
	}
	return true
}
func (t DynamicIndexTarget) copy() DynamicIndexTarget {
	t.tablePath = t.tablePath.Clone()
	t.suffix = append([]segment.Segment(nil), t.suffix...)
	return t
}

// DynamicIndexReadbackIntent describes which dynamic-index evidence a later
// applicator should read back after resolving the source paths/products.
type DynamicIndexReadbackIntent uint8

const (
	DynamicIndexReadbackNone DynamicIndexReadbackIntent = iota + 1
	DynamicIndexReadbackKey
	DynamicIndexReadbackValue
	DynamicIndexReadbackKeyAndValue
)

// DynamicIndexWrite describes a write through a dynamic table index at a CFG
// point. Key/value products remain unresolved ValueSource evidence here.
type DynamicIndexWrite struct {
	target    DynamicIndexTarget
	keyPath   path.Path
	valuePath path.Path
	source    ValueSource

	admission        dynamicindex.Admission
	readbackIntent   DynamicIndexReadbackIntent
	targetSpan       SourceSpan
	containerSpan    SourceSpan
	hasTargetSpan    bool
	hasContainerSpan bool
	hasKeyPath       bool
	hasValuePath     bool
}

// NewDynamicIndexWrite creates a dynamic-index write event.
func NewDynamicIndexWrite(
	target DynamicIndexTarget,
	source ValueSource,
	admission dynamicindex.Admission,
	readbackIntent DynamicIndexReadbackIntent,
) DynamicIndexWrite {
	return DynamicIndexWrite{
		target:         target.copy(),
		source:         source,
		admission:      admission,
		readbackIntent: readbackIntent,
	}
}

// TablePath returns the table path receiving the dynamic index write.
func (w DynamicIndexWrite) TablePath() path.Path { return w.target.tablePath.Clone() }

// TablePathRef returns the dynamic-index table path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (w DynamicIndexWrite) TablePathRef() path.Path { return w.target.tablePath }

func (w DynamicIndexWrite) TargetRef() DynamicIndexTarget { return w.target }

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
func (w DynamicIndexWrite) KeySource() ValueSource { return w.target.keySource }

func (w DynamicIndexWrite) TargetSuffixRef() []segment.Segment { return w.target.suffix }

// Source returns the source evidence for the value written at the dynamic key.
func (w DynamicIndexWrite) Source() ValueSource { return w.source }

// TargetSpan returns the lowered source range for the dynamic-index assignment
// target.
func (w DynamicIndexWrite) TargetSpan() (SourceSpan, bool) {
	return w.targetSpan, w.hasTargetSpan
}

// WithTargetSpan returns a copy carrying target-location display metadata.
func (w DynamicIndexWrite) WithTargetSpan(span SourceSpan) DynamicIndexWrite {
	w.targetSpan = span
	w.hasTargetSpan = sourceSpanValid(span)
	return w
}

// ContainerSpan returns the lowered source range for the dynamic-index
// assignment container.
func (w DynamicIndexWrite) ContainerSpan() (SourceSpan, bool) {
	return w.containerSpan, w.hasContainerSpan
}

// WithContainerSpan returns a copy carrying container-location display metadata.
func (w DynamicIndexWrite) WithContainerSpan(span SourceSpan) DynamicIndexWrite {
	w.containerSpan = span
	w.hasContainerSpan = sourceSpanValid(span)
	return w
}

// Admission returns the typed admission intent for this write.
func (w DynamicIndexWrite) Admission() dynamicindex.Admission { return w.admission }

// ReadbackIntent returns the typed post-write readback intent for this write.
func (w DynamicIndexWrite) ReadbackIntent() DynamicIndexReadbackIntent {
	return w.readbackIntent
}

func (w DynamicIndexWrite) copy() DynamicIndexWrite {
	w.target = w.target.copy()
	w.keyPath = w.keyPath.Clone()
	w.valuePath = w.valuePath.Clone()
	return w
}
