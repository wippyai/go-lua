package static

import (
	"testing"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// This file is the stage-2 equivalence gate for the may-runtime-kind column.
// It carries a verbatim copy of the pre-column recursive projection and states
// how the column relates to it: never larger, and smaller only where the
// column resolves a cycle the old walker abandoned to the whole vocabulary.
// It is deleted with the walker it certifies.

func gateMayRuntimeKinds(value typ.Type, active map[typ.Type]bool) runtimekind.Set {
	value = typ.UnwrapStructuralWrappers(typ.NormalizeNil(value))
	if value == nil {
		return runtimekind.All
	}
	if active[value] {
		return runtimekind.All
	}

	switch typed := value.(type) {
	case *typ.Optional:
		active[value] = true
		result := runtimekind.Bit(runtimekind.Nil) | gateMayRuntimeKinds(typed.Inner, active)
		delete(active, value)
		return result
	case *typ.Union:
		active[value] = true
		var result runtimekind.Set
		for _, member := range typed.Members {
			result |= gateMayRuntimeKinds(member, active)
		}
		delete(active, value)
		return result
	case *typ.Intersection:
		active[value] = true
		result := runtimekind.All
		for _, member := range typed.Members {
			result &= gateMayRuntimeKinds(member, active)
		}
		delete(active, value)
		return result
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(typed)
		if expanded == nil || expanded == value {
			return runtimekind.All
		}
		active[value] = true
		result := gateMayRuntimeKinds(expanded, active)
		delete(active, value)
		return result
	case *typ.Recursive:
		if typed.Body == nil {
			return runtimekind.All
		}
		active[value] = true
		result := gateMayRuntimeKinds(typed.Body, active)
		delete(active, value)
		return result
	case *typ.Literal:
		return gateRuntimeKindForBase(typed.Base())
	}

	switch value.Kind() {
	case kind.Never:
		return 0
	case kind.Nil:
		return runtimekind.Bit(runtimekind.Nil)
	case kind.Boolean:
		return runtimekind.Bit(runtimekind.Boolean)
	case kind.Number, kind.Integer:
		return runtimekind.Bit(runtimekind.Number)
	case kind.String:
		return runtimekind.Bit(runtimekind.String)
	case kind.Function:
		return runtimekind.Bit(runtimekind.Function)
	case kind.Array, kind.Map, kind.ReadonlyMap, kind.Record:
		return runtimekind.Bit(runtimekind.Table)
	case kind.Any, kind.Unknown:
		return runtimekind.All
	default:
		return runtimekind.All
	}
}

func gateRuntimeKindForBase(base kind.Kind) runtimekind.Set {
	switch base {
	case kind.Boolean:
		return runtimekind.Bit(runtimekind.Boolean)
	case kind.Number, kind.Integer:
		return runtimekind.Bit(runtimekind.Number)
	case kind.String:
		return runtimekind.Bit(runtimekind.String)
	default:
		return runtimekind.All
	}
}

func gateRecord(fields ...typ.Field) typ.Type {
	return typ.RebuildRecord(typ.RecordParts{Fields: fields})
}

func gateRuntimeKindCorpus() []struct {
	name  string
	build func() []typ.Type
} {
	function, _ := typ.BuiltinPrimitiveType("function")
	var cases []struct {
		name  string
		build func() []typ.Type
	}
	add := func(name string, build func() []typ.Type) {
		cases = append(cases, struct {
			name  string
			build func() []typ.Type
		}{name, build})
	}

	leaves := func() []typ.Type {
		return []typ.Type{
			typ.Any, typ.Unknown, typ.Never, typ.Nil, typ.String, typ.Number,
			typ.Integer, typ.Boolean, function,
			typ.NewArray(typ.String), typ.NewMap(typ.String, typ.Number),
			typ.NewReadonlyMap(typ.String, typ.Number),
			gateRecord(typ.Field{Name: "x", Type: typ.Number}),
			typ.NewTuple(typ.String, typ.Number),
			typ.NewTypeParam("T", nil),
			typ.NewInterface("I", []typ.Method{{Name: "m", Type: typ.Func().Returns(typ.String).Build()}}),
			typ.NewMeta(typ.String),
			typ.NewAlias("A", typ.String),
			typ.LiteralString("s"),
			typ.LiteralBool(true),
			typ.LiteralInt(3),
			typ.LiteralNumber(1.5),
		}
	}

	add("leaves", leaves)
	add("wrapped leaves", func() []typ.Type {
		var out []typ.Type
		for _, leaf := range leaves() {
			out = append(out,
				typeexpr.Optional(leaf),
				typeexpr.Union(leaf, typ.String),
				typeexpr.Union(leaf, typ.Nil),
				typ.MaterializeIntersection([]typ.Type{leaf, typ.String}),
				typ.MaterializeIntersection([]typ.Type{leaf, gateRecord(typ.Field{Name: "y", Type: typ.String})}),
				typ.NewArray(leaf),
			)
		}
		return out
	})
	add("union of every leaf", func() []typ.Type {
		return []typ.Type{typeexpr.Union(leaves()...)}
	})
	add("closed self recursion", func() []typ.Type {
		node := typ.NewRecursivePlaceholder("Node")
		node.SetBody(gateRecord(
			typ.Field{Name: "value", Type: typ.String},
			typ.Field{Name: "next", Type: node, Optional: true},
		))
		return []typ.Type{node, typeexpr.Optional(node), typeexpr.Union(node, typ.String)}
	})
	add("recursive union", func() []typ.Type {
		node := typ.NewRecursivePlaceholder("Node")
		node.SetBody(typeexpr.Union(typ.String, node))
		other := typ.NewRecursivePlaceholder("Other")
		other.SetBody(typeexpr.Union(typeexpr.Optional(other), typ.Number))
		return []typ.Type{node, other, typeexpr.Union(node, other)}
	})
	add("recursive intersection", func() []typ.Type {
		node := typ.NewRecursivePlaceholder("Node")
		node.SetBody(typ.MaterializeIntersection([]typ.Type{
			gateRecord(typ.Field{Name: "a", Type: typ.String}),
			typeexpr.Optional(node),
		}))
		return []typ.Type{node}
	})
	add("mutual recursion", func() []typ.Type {
		left := typ.NewRecursivePlaceholder("Left")
		right := typ.NewRecursivePlaceholder("Right")
		left.SetBody(typeexpr.Union(typ.String, right))
		right.SetBody(typeexpr.Union(typ.Number, left))
		return []typ.Type{left, right}
	})
	add("open placeholder", func() []typ.Type {
		child := typ.NewRecursivePlaceholder("Child")
		return []typ.Type{child, typeexpr.Optional(child), typeexpr.Union(child, typ.String)}
	})
	add("generic applications", func() []typ.Type {
		param := typ.NewTypeParam("T", nil)
		identity := typ.NewGeneric("Id", []*typ.TypeParam{param}, param)
		boxed := typ.NewGeneric("Box", []*typ.TypeParam{param}, gateRecord(typ.Field{Name: "v", Type: param}))
		maybe := typ.NewGeneric("Maybe", []*typ.TypeParam{param}, typeexpr.Optional(param))
		either := typ.NewGeneric("Either", []*typ.TypeParam{param}, typeexpr.Union(param, typ.String))
		return []typ.Type{
			identity, boxed, maybe, either,
			typ.Instantiate(identity, typ.String),
			typ.Instantiate(identity, typ.Number),
			typ.Instantiate(identity, typ.Any),
			typ.Instantiate(identity, typ.Never),
			typ.Instantiate(boxed, typ.String),
			typ.Instantiate(maybe, typ.String),
			typ.Instantiate(either, typ.Number),
			typ.Instantiate(either, typ.Any),
			typeexpr.Optional(typ.Instantiate(identity, typ.String)),
			typeexpr.Union(typ.Instantiate(identity, typ.String), typ.Instantiate(identity, typ.Number)),
		}
	})
	add("self application", func() []typ.Type {
		param := typ.NewTypeParam("T", nil)
		list := typ.NewGeneric("List", []*typ.TypeParam{param}, nil)
		list.SetBody(gateRecord(
			typ.Field{Name: "head", Type: param},
			typ.Field{Name: "tail", Type: typ.Instantiate(list, param), Optional: true},
		))
		return []typ.Type{list, typ.Instantiate(list, typ.String)}
	})
	add("generic body reaching recursion", func() []typ.Type {
		node := typ.NewRecursivePlaceholder("Node")
		param := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Box", []*typ.TypeParam{param}, nil)
		generic.SetBody(typeexpr.Union(param, typeexpr.Optional(node)))
		node.SetBody(typeexpr.Union(typ.String, typ.Instantiate(generic, typ.Number)))
		return []typ.Type{node, generic, typ.Instantiate(generic, typ.String)}
	})

	return cases
}

// TestMayRuntimeKindsColumnRefinesThePreColumnProjection is the deletion gate
// for staticTypeMayRuntimeKinds.
func TestMayRuntimeKindsColumnRefinesThePreColumnProjection(t *testing.T) {
	compared, refined := 0, 0
	for _, shape := range gateRuntimeKindCorpus() {
		for index, subject := range shape.build() {
			if subject == nil {
				t.Fatalf("%s: corpus entry %d is unavailable", shape.name, index)
			}
			reference := gateMayRuntimeKinds(subject, make(map[typ.Type]bool))
			column := MayRuntimeKinds(subject)
			compared++
			if !column.Valid() {
				t.Fatalf("%s entry %d (%s): column produced a set outside the vocabulary: %d",
					shape.name, index, subject.String(), column)
			}
			if column == reference {
				continue
			}
			if column&^reference != 0 {
				t.Fatalf("%s entry %d (%s): column = %d admits a family the pre-column projection excludes (%d)",
					shape.name, index, subject.String(), column, reference)
			}
			// The column is strictly smaller. That is the least-fixed-point
			// resolution of a cycle the recursive walker abandoned to the whole
			// vocabulary, so it may only happen on a graph that has one.
			if !typ.ContainsRecursive(subject) && !typ.ContainsInstantiated(subject) {
				t.Fatalf("%s entry %d (%s): column = %d is narrower than %d on an acyclic graph",
					shape.name, index, subject.String(), column, reference)
			}
			refined++
		}
	}
	if compared < 100 {
		t.Fatalf("gate compared only %d projections; the corpus is too small to certify a deletion", compared)
	}
	t.Logf("gate compared %d projections, %d refined by the least fixed point", compared, refined)
}

// TestMayRuntimeKindsResolvesCyclesExactly pins what the least fixed point buys
// over the pre-column walker, which answered the whole vocabulary for any node
// it re-entered.
func TestMayRuntimeKindsResolvesCyclesExactly(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	node.SetBody(typeexpr.Union(typ.String, node))
	if got, want := MayRuntimeKinds(node), runtimekind.Bit(runtimekind.String); got != want {
		t.Fatalf("MayRuntimeKinds(mu X. string | X) = %d, want %d", got, want)
	}

	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(typeexpr.Union(typ.String, right))
	right.SetBody(typeexpr.Union(typ.Number, left))
	want := runtimekind.Bit(runtimekind.String) | runtimekind.Bit(runtimekind.Number)
	if got := MayRuntimeKinds(left); got != want {
		t.Fatalf("MayRuntimeKinds of a mutually recursive union = %d, want %d", got, want)
	}
	if got := MayRuntimeKinds(right); got != want {
		t.Fatalf("MayRuntimeKinds of the peer declaration = %d, want %d", got, want)
	}
}
