package typ

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

const annotatedHashSalt = 0x9e3779b97f4a7c15

// Annotated wraps a type with runtime validation annotations.
// The underlying type determines structural typing while annotations
// add runtime constraints like @min(0), @max(100), @pattern("^.+$").
type Annotated struct {
	Inner       Type
	Annotations []annotation.Annotation
	hash        uint64
	typeProperties
	strCache stringCache
}

// NewAnnotated creates an annotated type wrapper.
// Returns the inner type directly if no annotations are provided.
func NewAnnotated(inner Type, annotations []annotation.Annotation) Type {
	if len(annotations) == 0 {
		return inner
	}
	if inner == nil {
		inner = Unknown
	}
	h := hash.MixHash(inner.Hash(), annotatedHashSalt)
	for _, ann := range annotations {
		h = hash.MixHash(h, ann.Hash())
	}
	return &Annotated{
		Inner:          inner,
		Annotations:    annotations,
		hash:           h,
		typeProperties: typePropertiesOf(inner),
	}
}

func (a *Annotated) Kind() kind.Kind {
	if a.Inner == nil {
		return kind.Unknown
	}
	return a.Inner.Kind()
}

func (a *Annotated) String() string {
	return a.strCache.get(func() string { return renderTypeString(a) })
}

func (a *Annotated) Hash() uint64 {
	return a.hash
}

func (a *Annotated) Equals(other Type) bool {
	o, ok := other.(*Annotated)
	if !ok {
		return false
	}
	if !a.Inner.Equals(o.Inner) {
		return false
	}
	if len(a.Annotations) != len(o.Annotations) {
		return false
	}
	for i, ann := range a.Annotations {
		if !ann.Equal(o.Annotations[i]) {
			return false
		}
	}
	return true
}

func unwrapAnnotated(t Type) Type {
	if a, ok := t.(*Annotated); ok {
		if a.Inner == nil {
			return Unknown
		}
		return a.Inner
	}
	return t
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
