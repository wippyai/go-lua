package typ

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Annotated wraps a type with runtime validation annotations.
// The underlying type determines structural typing while annotations
// add runtime constraints like @min(0), @max(100), @pattern("^.+$").
type Annotated struct {
	Inner                 Type
	Annotations           []annotation.Annotation
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
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
	h := hash.HashCombine(inner.Hash(), uint64(kind.Platform)<<32)
	for _, ann := range annotations {
		h = hash.HashCombine(h, hash.FnvString(ann.Name))
	}
	return &Annotated{
		Inner:                 inner,
		Annotations:           annotations,
		hash:                  h,
		containsAny:           knownContainsAny(inner),
		containsNever:         knownContainsNever(inner),
		containsTypeParam:     knownContainsTypeParam(inner),
		containsInstantiated:  knownContainsInstantiated(inner),
		containsRecursive:     knownContainsRecursive(inner),
		containsOpenRecursive: knownContainsOpenRecursive(inner),
	}
}

func (a *Annotated) Kind() kind.Kind {
	if a.Inner == nil {
		return kind.Unknown
	}
	return a.Inner.Kind()
}

func (a *Annotated) String() string {
	return a.strCache.get(func() string {
		var sb strings.Builder
		if a.Inner != nil {
			sb.WriteString(a.Inner.String())
		} else {
			sb.WriteString("unknown")
		}
		for _, ann := range a.Annotations {
			sb.WriteString(" @")
			sb.WriteString(ann.Name)
			if ann.Arg != nil {
				sb.WriteString("(")
				switch v := ann.Arg.(type) {
				case string:
					sb.WriteString("\"")
					sb.WriteString(v)
					sb.WriteString("\"")
				case float64:
					sb.WriteString(formatFloat(v))
				case int64:
					sb.WriteString(strconv.FormatInt(v, 10))
				case int:
					sb.WriteString(strconv.FormatInt(int64(v), 10))
				default:
					sb.WriteString("...")
				}
				sb.WriteString(")")
			}
		}
		return sb.String()
	})
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
		if ann.Name != o.Annotations[i].Name {
			return false
		}
	}
	return true
}

// UnwrapAnnotated returns the inner type, stripping annotations.
func UnwrapAnnotated(t Type) Type {
	if a, ok := t.(*Annotated); ok {
		if a.Inner == nil {
			return Unknown
		}
		return a.Inner
	}
	return t
}

// UnwrapAnnotations strips all Annotated wrappers, returning the innermost
// non-Annotated type.
func UnwrapAnnotations(t Type) Type {
	for {
		ann, ok := t.(*Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}

// GetAnnotations returns annotations from a type, or nil if none.
func GetAnnotations(t Type) []annotation.Annotation {
	if a, ok := t.(*Annotated); ok {
		return a.Annotations
	}
	return nil
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
