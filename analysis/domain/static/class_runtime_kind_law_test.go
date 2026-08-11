package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
)

func TestClassMayRuntimeKindsIsSealedAndSound(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type NilValue = nil
type BooleanValue = boolean
type NumberValue = number
type StringValue = string
type TableValue = { field: string }
type FunctionValue = (value: string) -> number
type OptionalValue = string?
type UnionValue = nil | number | string
type DynamicValue = any
type EmptyValue = never
type Node = { next: Node? }
`)
	classes := authority.Classes()
	if classes == nil {
		t.Fatal("ClassSet unavailable")
	}

	for _, law := range []struct {
		name string
		want runtimekind.Set
	}{
		{"NilValue", runtimekind.Bit(runtimekind.Nil)},
		{"BooleanValue", runtimekind.Bit(runtimekind.Boolean)},
		{"NumberValue", runtimekind.Bit(runtimekind.Number)},
		{"StringValue", runtimekind.Bit(runtimekind.String)},
		{"TableValue", runtimekind.Bit(runtimekind.Table)},
		{"FunctionValue", runtimekind.Bit(runtimekind.Function)},
		{"OptionalValue", runtimekind.Bit(runtimekind.Nil) | runtimekind.Bit(runtimekind.String)},
		{"UnionValue", runtimekind.Bit(runtimekind.Nil) | runtimekind.Bit(runtimekind.Number) | runtimekind.Bit(runtimekind.String)},
		{"DynamicValue", runtimekind.All},
		{"EmptyValue", 0},
		// Recursive authored aliases remain an opaque Static residual at this
		// boundary.  The projection must retain all possibilities rather than
		// inspect a type graph during recurrent queries.
		{"Node", runtimekind.All},
	} {
		_, term := aliasNamed(t, p, law.name)
		class, ok := classes.ClassForStatic(resultFor(t, authority, p, term))
		if !ok {
			t.Fatalf("%s has no sealed Class", law.name)
		}
		got, ok := classes.MayRuntimeKinds(class)
		if !ok || got != law.want {
			t.Fatalf("%s runtime kinds = %#x/%v, want %#x", law.name, got, ok, law.want)
		}
	}

	anyKinds, ok := classes.MayRuntimeKinds(classes.AnyValue())
	if !ok || anyKinds != runtimekind.All || !anyKinds.Contains(runtimekind.Thread) || !anyKinds.Contains(runtimekind.Userdata) {
		t.Fatal("ClassAnyValue lost closed runtime-family coverage")
	}
	if _, ok := classes.MayRuntimeKinds(Class{}); ok {
		t.Fatal("foreign Class entered runtime-kind projection")
	}
}

func TestClassMayRuntimeKindsDoesNotAllocate(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type NumberValue = number
type Node = { next: Node? }
type DynamicValue = any
`)
	classes := authority.Classes()
	queries := make([]Class, 0, 4)
	queries = append(queries, classes.AnyValue())
	for _, name := range []string{"NumberValue", "Node", "DynamicValue"} {
		_, term := aliasNamed(t, p, name)
		class, ok := classes.ClassForStatic(resultFor(t, authority, p, term))
		if !ok {
			t.Fatalf("%s Class", name)
		}
		queries = append(queries, class)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		for _, class := range queries {
			if _, ok := classes.MayRuntimeKinds(class); !ok {
				t.Fatal("sealed Class projection unavailable")
			}
		}
	}); allocations != 0 {
		t.Fatalf("MayRuntimeKinds allocations = %v, want 0", allocations)
	}
}
