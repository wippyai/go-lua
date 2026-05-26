package functionfact

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func paramAt(params []typ.Type, idx int) typ.Type {
	if idx < 0 || idx >= len(params) {
		return nil
	}
	return params[idx]
}

// RefinementGuaranteesParamType reports whether normal return proves parameter idx has type t.
func RefinementGuaranteesParamType(refinement *constraint.FunctionRefinement, idx int, t typ.Type) bool {
	if refinement == nil || t == nil {
		return false
	}
	path := constraint.ParamPath(idx)
	for _, c := range refinement.OnReturn.MustConstraints() {
		has, ok := c.(constraint.HasType)
		if !ok || !has.Path.Equal(path) {
			continue
		}
		if typeKeyCoversType(has.Type, t) {
			return true
		}
	}
	return false
}

func typeKeyCoversType(key narrow.TypeKey, t typ.Type) bool {
	if key.IsZero() || t == nil {
		return false
	}
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	switch key.Kind {
	case narrow.TypeKeyHash:
		return key.Hash == t.Hash()
	case narrow.TypeKeyBuiltin:
		return builtinTypeKeyCoversType(key, t)
	default:
		return false
	}
}

func builtinTypeKeyCoversType(key narrow.TypeKey, t typ.Type) bool {
	k, ok := key.BuiltinKind()
	if !ok {
		return false
	}
	switch k {
	case kind.Nil:
		return unwrap.IsNilType(t)
	case kind.Boolean:
		return subtype.IsSubtype(t, typ.Boolean)
	case kind.Number:
		return subtype.IsSubtype(t, typ.Number)
	case kind.String:
		return subtype.IsSubtype(t, typ.String)
	case kind.Function:
		return unwrap.Function(t) != nil
	case kind.Record:
		switch unwrap.Alias(t).(type) {
		case *typ.Array, *typ.Record, *typ.Map:
			return true
		default:
			return false
		}
	default:
		return false
	}
}
