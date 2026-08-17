package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every table-library export, and the effect
// row each one exercises. Several of these mutate their subject, and what they
// mutate is stated about the argument POSITIONS the callable declares, so the
// statement rides the value the contract attaches to rather than a name a
// consumer rebuilt from the callee.

// tableElement is one fresh element type parameter for an export that relates
// the element type of its list argument to its result.
func tableElement() *typ.TypeParam { return typ.NewTypeParam("T", nil) }

var tableSignatures = map[string]signature.Function{
	"concat": authored(typ.Func().
		Param("list", typ.Any).
		OptParam("sep", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.String).
		Build(),
		ownership.BorrowAll{}),
	"create": authored(typ.Func().
		Param("narray", typ.Integer).
		OptParam("nhash", typ.Integer).
		Returns(typetable.NewRecord().Build()).
		Build()),
	"freeze": tableFreeze(),
	// getn and maxn read a length out of their subject and write nothing, so
	// each borrows its argument and returns a number: getn answers the sequence
	// length and maxn the largest positive numeric key, which is a number rather
	// than an integer because a table may be keyed by one.
	"getn": authored(typ.Func().
		Param("list", typ.Any).
		Returns(typ.Number).
		Build(),
		ownership.BorrowAll{}),
	// table.insert takes its value at the last argument position, whichever
	// arity it was called at, which is why the mutation and the ownership
	// transfer both name the tail position rather than a fixed ordinal.
	"insert": authored(typ.Func().
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
		}),
	"isfrozen": authored(typ.Func().
		Param("t", typ.Any).
		Returns(typ.Boolean).
		Build(),
		ownership.BorrowAll{}),
	"maxn": authored(typ.Func().
		Param("list", typ.Any).
		Returns(typ.Number).
		Build(),
		ownership.BorrowAll{}),
	"move": authored(typ.Func().
		Param("a1", typ.Any).
		Param("f", typ.Integer).
		Param("e", typ.Integer).
		Param("t", typ.Integer).
		OptParam("a2", typ.Any).
		Returns(typ.Any).
		Build()),
	// table.pack collects its arguments into a fresh table: the arguments land
	// under integer keys and n records how many were passed. Both halves are
	// published, so a caller reads the count as an integer and an entry as the
	// argument type the call site supplied.
	"pack":   tablePack(),
	"remove": tableRemove(),
	"sort": authored(typ.Func().
		Param("list", typ.Any).
		OptParam("comp", typ.Any).
		Build(),
		mutation.Mutate{
			Target:    effect.ParamRef{Index: 0},
			Transform: mutation.Unchanged{},
		}),
	"unpack": tableUnpack(),
}

func tableFreeze() signature.Function {
	frozen := typ.NewTypeParam("T", nil)
	return authored(typ.Func().
		TypeParamRef(frozen).
		Param("t", frozen).
		Returns(frozen).
		Build())
}

func tablePack() signature.Function {
	elem := tableElement()
	return authored(typ.Func().
		TypeParamRef(elem).
		Variadic(elem).
		Returns(typetable.NewRecord().
			Field("n", typ.Integer).
			MapComponent(typ.Integer, elem).
			Build()).
		Build())
}

func tableRemove() signature.Function {
	elem := tableElement()
	return authored(typ.Func().
		TypeParamRef(elem).
		Param("list", typ.NewArray(elem)).
		OptParam("pos", typ.Integer).
		Returns(normalize.Optional(elem)).
		Build(),
		mutation.Mutate{
			Target:    effect.ParamRef{Index: 0},
			Transform: mutation.Unchanged{},
		},
		mutation.LengthChange{
			Target: effect.ParamRef{Index: 0},
			Delta:  -1,
		})
}

func tableUnpack() signature.Function {
	elem := tableElement()
	return authored(typ.Func().
		TypeParamRef(elem).
		Param("list", typ.NewArray(elem)).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(normalize.Optional(elem)).
		Build(),
		ownership.BorrowAll{})
}
