package flow

import (
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// IteratorVarProjection is the semantic result of projecting generic-for
// variables from an iterator source. Empty distinguishes a recognized iterator
// over a source with no present entries from an unrecognized iterator.
type IteratorVarProjection struct {
	Types []typ.Type
	Empty bool
}

// IteratorVarTypes projects generic-for loop variable types from the iterator
// kind and source container.
func IteratorVarTypes(kind IteratorKind, count int, source typ.Type) ([]typ.Type, bool) {
	proj, ok := ProjectIteratorVarTypes(kind, count, source)
	if !ok || proj.Empty {
		return nil, false
	}
	return proj.Types, true
}

// ProjectIteratorVarTypes projects generic-for loop variable types and preserves
// the recognized-empty case. Producers lower AST/contract-specific iterator
// evidence before entering this law.
func ProjectIteratorVarTypes(kind IteratorKind, count int, source typ.Type) (IteratorVarProjection, bool) {
	if count <= 0 {
		return IteratorVarProjection{}, false
	}
	if typ.IsAny(source) && kind == IterateKeyed {
		out := make([]typ.Type, count)
		for i := range out {
			out[i] = typ.Any
		}
		return IteratorVarProjection{Types: out}, true
	}
	if typ.IsNever(source) {
		return IteratorVarProjection{Empty: true}, true
	}
	if source == nil || typ.IsAbsentOrUnknown(source) {
		return IteratorVarProjection{}, false
	}
	out := make([]typ.Type, count)
	switch kind {
	case IterateIndexed:
		out[0] = typ.Integer
		if count > 1 {
			out[1] = querycore.ElementType(source)
			if out[1] == nil && isPlaceholder(unwrap.Underlying(source)) {
				out[1] = typ.Any
			}
		}
	case IterateKeyed:
		out[0] = querycore.EntryKeyType(source)
		if out[0] == nil {
			if IsUniformKeyedContainer(source) {
				return IteratorVarProjection{Empty: true}, true
			}
			return IteratorVarProjection{}, false
		}
		if count > 1 {
			out[1] = querycore.EntryValueType(source)
			if out[1] == nil {
				return IteratorVarProjection{Empty: true}, true
			}
		}
	default:
		return IteratorVarProjection{}, false
	}
	return IteratorVarProjection{Types: out}, true
}

// IsUniformKeyedContainer reports whether pairs-style iteration may soundly
// project yielded entries through EntryKeyType/EntryValueType.
func IsUniformKeyedContainer(t typ.Type) bool {
	switch v := unwrap.Underlying(t).(type) {
	case *typ.Map, *typ.ReadonlyMap:
		return true
	case *typ.Array, *typ.Tuple:
		return true
	case *typ.Record:
		return true
	case *typ.Optional:
		return IsUniformKeyedContainer(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !IsUniformKeyedContainer(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isPlaceholder(t typ.Type) bool {
	return t != nil && t.Kind().IsPlaceholder()
}
