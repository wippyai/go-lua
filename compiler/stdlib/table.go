package stdlib

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// tableInsertSpec models mutation effects of table.insert for flow analysis.
var tableInsertSpec = contract.NewSpec().
	WithEffects(
		effect.Mutate{
			Target:      effect.ParamRef{Index: 0},
			Transform:   effect.ElementUnion{Source: effect.ParamRef{Index: -1}},
			LengthDelta: constraint.C(1),
		},
		effect.TableMutator{
			Target: effect.ParamRef{Index: 0},
			Value:  effect.ParamRef{Index: -1},
		},
	)

var tableMethods = typ.NewRecord().
	Field("remove", func() typ.Type {
		elem := typ.NewTypeParam("T", nil)
		return typ.NewIntersection(
			typ.Func().TypeParam("T", nil).Param("list", typ.NewArray(elem)).
				Returns(typ.NewOptional(elem)).Effects(effect.Mutates(0, effect.Unchanged{})).Build(),
			typ.Func().TypeParam("T", nil).Param("list", typ.NewArray(elem)).Param("pos", typ.Integer).
				Returns(typ.NewOptional(elem)).Effects(effect.Mutates(0, effect.Unchanged{})).Build(),
		)
	}()).
	Field("concat", typ.Func().
		Param("list", typ.Any).
		OptParam("sep", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.String).
		Effects(effect.BorrowsOnly()).
		Build()).
	Field("create", typ.Func().
		Param("narray", typ.Integer).
		OptParam("nhash", typ.Integer).
		Returns(typ.NewRecord().Build()).
		Build()).
	Field("freeze", func() typ.Type {
		tp := typ.NewTypeParam("T", nil)
		return typ.Func().
			TypeParam("T", nil).
			Param("t", tp).
			Returns(tp).
			Build()
	}()).
	Field("insert", typ.NewIntersection(
		typ.Func().Param("list", typ.Any).Param("value", typ.Any).
			Effects(effect.StoresParam(-1, 0)).Spec(tableInsertSpec).Build(),
		typ.Func().Param("list", typ.Any).Param("pos", typ.Integer).Param("value", typ.Any).
			Effects(effect.StoresParam(-1, 0)).Spec(tableInsertSpec).Build(),
	)).
	Field("move", typ.Func().
		Param("a1", typ.Any).
		Param("f", typ.Integer).
		Param("e", typ.Integer).
		Param("t", typ.Integer).
		OptParam("a2", typ.Any).
		Returns(typ.Any).
		Build()).
	Field("pack", typ.Func().
		Variadic(typ.Any).
		Returns(typ.Any).
		Build()).
	Field("sort", func() typ.Type {
		elem := typ.NewTypeParam("T", nil)
		return typ.NewIntersection(
			typ.Func().TypeParam("T", nil).Param("list", typ.NewArray(elem)).
				Effects(effect.Mutates(0, effect.Unchanged{})).Build(),
			typ.Func().TypeParam("T", nil).Param("list", typ.NewArray(elem)).
				Param("comp", typ.Func().Param("a", elem).Param("b", elem).Returns(typ.Boolean).Build()).
				Effects(effect.Mutates(0, effect.Unchanged{})).Build(),
		)
	}()).
	Field("unpack", typ.Func().
		Param("list", typ.Any).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.Any).
		Effects(effect.BorrowsOnly()).
		Build()).
	Build()

// TableLib provides types for Lua's table manipulation functions.
var TableLib typ.Type = tableMethods
