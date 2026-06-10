package annotation

import (
	"math"
	"reflect"

	"github.com/wippyai/go-lua/analysis/internal/hash"
)

// Annotation describes a runtime validation payload attached to a type wrapper.
type Annotation struct {
	Name string
	Arg  any
}

// Equal reports whether two annotations carry the same validation payload.
func (a Annotation) Equal(other Annotation) bool {
	return a.Name == other.Name && reflect.DeepEqual(a.Arg, other.Arg)
}

// Hash returns a stable identity hash for the supported annotation payloads.
// Unknown payload shapes intentionally collapse by Go type; Equal still
// distinguishes them, while equal payloads keep the required same-hash property.
func (a Annotation) Hash() uint64 {
	h := hash.FnvString(a.Name)
	if a.Arg == nil {
		return hash.HashCombine(h, hash.FnvString("<nil>"))
	}
	switch v := a.Arg.(type) {
	case string:
		h = hash.HashCombine(h, hash.FnvString("string"))
		return hash.HashCombine(h, hash.FnvString(v))
	case bool:
		h = hash.HashCombine(h, hash.FnvString("bool"))
		if v {
			return hash.HashCombine(h, 1)
		}
		return hash.HashCombine(h, 0)
	case int:
		h = hash.HashCombine(h, hash.FnvString("int"))
		return hash.HashCombine(h, uint64(int64(v)))
	case int64:
		h = hash.HashCombine(h, hash.FnvString("int64"))
		return hash.HashCombine(h, uint64(v))
	case float64:
		h = hash.HashCombine(h, hash.FnvString("float64"))
		return hash.HashCombine(h, math.Float64bits(v))
	default:
		t := reflect.TypeOf(a.Arg)
		if t == nil {
			return hash.HashCombine(h, hash.FnvString("<nil>"))
		}
		h = hash.HashCombine(h, hash.FnvString("opaque"))
		return hash.HashCombine(h, hash.FnvString(t.String()))
	}
}
