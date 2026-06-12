// Package stdlib exposes clean effect signatures for bounded standard-library
// functions used by module metadata.
package stdlib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const (
	Assert      = "assert"
	Require     = "require"
	Type        = "type"
	Pairs       = "pairs"
	IPairs      = "ipairs"
	PCall       = "pcall"
	XPCall      = "xpcall"
	TableInsert = "table.insert"
)

var registry = map[string]signature.Function{
	Assert: sig(
		typ.Func().
			Param("v", typ.Any).
			OptParam("message", typ.Any).
			Returns(typ.Any).
			Build(),
		control.Throw{},
		postcondition.NormalReturnRefinement{
			Target:     effect.ParamRef{Index: 0},
			Refinement: postcondition.Present{},
		},
	),
	Require: sig(
		typ.Func().
			Param("modname", typ.String).
			Returns(typ.Any).
			Build(),
		dispatch.ModuleLoad{},
		control.Throw{},
	),
	Type: sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(typ.String).
			Build(),
		dispatch.TypePredicate{},
		ownership.BorrowAll{},
	),
	Pairs: sig(
		typ.Func().
			Param("t", typ.Any).
			Returns(typ.Any, typ.Any, typ.Nil).
			Build(),
		iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IterateKeyed,
		},
	),
	IPairs: sig(
		typ.Func().
			Param("t", typ.Any).
			Returns(typ.Any, typ.Any, typ.Integer).
			Build(),
		iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IterateIndexed,
		},
	),
	PCall: sig(
		typ.Func().
			Param("f", typ.Any).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.Any).
			Build(),
		ownership.BorrowAll{},
		returns.Return{
			ReturnIndex: 1,
			Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		},
	),
	XPCall: sig(
		typ.Func().
			Param("f", typ.Any).
			Param("msgh", typ.Any).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.Any).
			Build(),
		ownership.BorrowAll{},
		returns.Return{
			ReturnIndex: 1,
			Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		},
	),
	TableInsert: sig(
		typ.Func().
			Param("list", typ.Any).
			Param("pos_or_value", typ.Any).
			OptParam("value", typ.Any).
			Build(),
		mutation.TableMutator{
			Target: effect.ParamRef{Index: 0},
			Value:  effect.ParamRef{Index: -1},
		},
		mutation.LengthChange{
			Target: effect.ParamRef{Index: 0},
			Delta:  1,
		},
		ownership.Store{
			Param: effect.ParamRef{Index: -1},
			Into:  effect.ParamRef{Index: 0},
		},
	),
}

// Lookup returns a cloned effect signature for a known stdlib function name.
func Lookup(name string) (signature.Function, bool) {
	sig, ok := registry[name]
	if !ok {
		return signature.Function{}, false
	}
	return sig.Clone(), true
}

// Signatures returns cloned registry entries keyed by stable stdlib names.
func Signatures() map[string]signature.Function {
	out := make(map[string]signature.Function, len(registry))
	for name, sig := range registry {
		out[name] = sig.Clone()
	}
	return out
}

func sig(fn *typ.Function, labels ...effect.Label) signature.Function {
	return signature.Function{
		Type:   fn,
		Effect: effect.Row{Labels: labels},
	}
}
