// Package typ is the legacy public import path for go-lua types.
//
// New code should import analysis/type/typ, analysis/type/table, and
// analysis/type/typeexpr directly. This package is a migration facade for
// consumers that still use the pre-analysis namespace.
package typ

import (
	"github.com/wippyai/go-lua/analysis/type/table"
	core "github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

type Type = core.Type
type Param = core.Param
type Function = core.Function
type FunctionBuilder = core.FunctionBuilder
type Field = core.Field
type Record = core.Record
type StaticMember = core.StaticMember
type StaticMemberKind = core.StaticMemberKind
type Array = core.Array
type Map = core.Map
type ReadonlyMap = core.ReadonlyMap
type Tuple = core.Tuple
type Optional = core.Optional
type Union = core.Union
type Intersection = core.Intersection
type Method = core.Method
type Interface = core.Interface
type TypeParam = core.TypeParam
type Generic = core.Generic
type Instantiated = core.Instantiated
type Ref = core.Ref
type Alias = core.Alias
type Meta = core.Meta
type Literal = core.Literal
type RecordBuilder = table.RecordBuilder

const (
	StaticMemberStringIndex = core.StaticMemberStringIndex
	StaticMemberIntIndex    = core.StaticMemberIntIndex
)

var (
	Nil     = core.Nil
	Boolean = core.Boolean
	Number  = core.Number
	Integer = core.Integer
	String  = core.String
	Any     = core.Any
	Unknown = core.Unknown
	Never   = core.Never
	Self    = core.Self
	True    = core.True
	False   = core.False

	// LuaError is the standard Wippy error interface exposed by module
	// manifests. It intentionally remains structural.
	LuaError Type = NewInterface("Error", []Method{
		{Name: "kind", Type: Func().Param("self", Self).Returns(String).Build()},
		{Name: "retryable", Type: Func().Param("self", Self).Returns(Boolean).Build()},
		{Name: "details", Type: Func().Param("self", Self).Returns(Any).Build()},
		{Name: "message", Type: Func().Param("self", Self).Returns(String).Build()},
		{Name: "stack", Type: Func().Param("self", Self).Returns(String).Build()},
	})
)

func Func() *FunctionBuilder { return core.Func() }

func NewRecord() *RecordBuilder { return table.NewRecord() }

func NewArray(elem Type) *Array { return core.NewArray(elem) }

func NewMap(key, value Type) *Map { return core.NewMap(key, value) }

func NewReadonlyMap(key, value Type) *ReadonlyMap {
	return core.NewReadonlyMap(key, value)
}

func NewTuple(elems ...Type) *Tuple { return core.NewTuple(elems...) }

func NewOptional(inner Type) Type { return typeexpr.Optional(inner) }

func NewUnion(members ...Type) Type { return typeexpr.Union(members...) }

func NewIntersection(members ...Type) Type {
	return typeexpr.Intersection(members...)
}

func NewInterface(name string, methods []Method) *Interface {
	return core.NewInterface(name, methods)
}

func NewTypeParam(name string, constraint Type) *TypeParam {
	return core.NewTypeParam(name, constraint)
}

func NewGeneric(name string, params []*TypeParam, body Type) *Generic {
	return core.NewGeneric(name, params, body)
}

func Instantiate(g *Generic, args ...Type) *Instantiated {
	return core.Instantiate(g, args...)
}

func NewRef(module, name string) *Ref { return core.NewRef(module, name) }

func NewAlias(name string, target Type) *Alias {
	return core.NewAlias(name, target)
}

func NewMeta(of Type) *Meta { return core.NewMeta(of) }

func LiteralBool(v bool) *Literal { return core.LiteralBool(v) }

func LiteralInt(v int64) *Literal { return core.LiteralInt(v) }

func LiteralNumber(v float64) *Literal { return core.LiteralNumber(v) }

func LiteralString(v string) *Literal { return core.LiteralString(v) }

func TypeEquals(a, b Type) bool { return core.TypeEquals(a, b) }

func IsAny(t Type) bool { return core.IsAny(t) }

func IsUnknown(t Type) bool { return core.IsUnknown(t) }

func IsNever(t Type) bool { return core.IsNever(t) }

func EqualityHash(t Type) uint64 { return core.EqualityHash(t) }
