package typ

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
)

// The scope law pins the admission verdict of the one cold static recurrence
// law across the shapes that exercise binder scoping: lexical ownership,
// nested binders, a subgraph reached through two distinct binder chains, and
// every reject condition the law adjudicates. Scope identity inside the walk
// is an implementation concern; the verdict below is the law.
type staticRecurrenceScopeCase struct {
	name   string
	build  func() (Type, []*TypeParam)
	scoped bool
	open   bool
	admits bool
}

func staticRecurrenceScopeCases() []staticRecurrenceScopeCase {
	return []staticRecurrenceScopeCase{
		{
			name:   "nil root carries no recurrence",
			build:  func() (Type, []*TypeParam) { return nil, nil },
			admits: true,
		},
		{
			name:   "closed primitive",
			build:  func() (Type, []*TypeParam) { return NewTuple(String, Number), nil },
			admits: true,
		},
		{
			name: "function binder owns its formal",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				return Func().TypeParamRef(param).Param("value", param).Returns(param).Build(), nil
			},
			admits: true,
		},
		{
			name: "function binder owns a constrained formal",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", String)
				return Func().TypeParamRef(param).Variadic(NewArray(param)).Returns(param).Build(), nil
			},
			admits: true,
		},
		{
			name: "nested binder reads the enclosing formal",
			build: func() (Type, []*TypeParam) {
				outer := NewTypeParam("T", nil)
				inner := NewTypeParam("U", nil)
				innerGeneric := NewGeneric("Inner", []*TypeParam{inner}, NewTuple(outer, inner))
				return NewGeneric("Outer", []*TypeParam{outer}, NewArray(innerGeneric)), nil
			},
			admits: true,
		},
		{
			name: "shared declaration reached through two binder chains",
			build: func() (Type, []*TypeParam) {
				shared := NewTypeParam("S", nil)
				sharedGeneric := NewGeneric("Shared", []*TypeParam{shared}, NewArray(shared))
				first := NewTypeParam("T", nil)
				second := NewTypeParam("U", nil)
				firstGeneric := NewGeneric("First", []*TypeParam{first}, NewTuple(Instantiate(sharedGeneric, first), Instantiate(sharedGeneric, String)))
				secondGeneric := NewGeneric("Second", []*TypeParam{second}, NewTuple(Instantiate(sharedGeneric, second), NewArray(firstGeneric)))
				return NewTuple(NewArray(firstGeneric), NewArray(secondGeneric)), nil
			},
			admits: true,
		},
		{
			name: "nested builder chain of shared applications",
			build: func() (Type, []*TypeParam) {
				return staticRecurrenceBuilderChain(6), nil
			},
			admits: true,
		},
		{
			name: "productive self recurrence through a record",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				node := NewGeneric("Node", []*TypeParam{param}, nil)
				node.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(node, param)}}}))
				return node, nil
			},
			admits: true,
		},
		{
			name: "annotated binder owns its formal",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				return staticRecurrenceAnnotate(NewGeneric("Annotated", []*TypeParam{param}, NewArray(param))), nil
			},
			admits: true,
		},
		{
			name: "annotated binder inside a tuple owns its formal",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				binder := NewGeneric("Annotated", []*TypeParam{param}, NewArray(param))
				return NewTuple(String, staticRecurrenceAnnotate(binder)), nil
			},
			admits: true,
		},
		{
			name: "annotated binder does not own a foreign formal",
			build: func() (Type, []*TypeParam) {
				foreign := NewTypeParam("External", nil)
				binder := NewGeneric("Annotated", []*TypeParam{NewTypeParam("T", nil)}, NewArray(foreign))
				return staticRecurrenceAnnotate(binder), nil
			},
			admits: false,
		},
		{
			name: "typed nil interior node",
			build: func() (Type, []*TypeParam) {
				// A typed nil element cannot be produced by NewTuple, which
				// dereferences every element. The law adjudicates the graph it
				// receives, so the malformed node is stated directly.
				return &Tuple{Elements: []Type{String, (*Array)(nil)}}, nil
			},
			admits: false,
		},
		{
			name: "binder omits a formal",
			build: func() (Type, []*TypeParam) {
				// NewGeneric hashes every formal, so an absent one is stated
				// directly: a binder with no formal to bind has no ownership to
				// give and the law must say so rather than fault.
				return &Generic{Name: "Absent", TypeParams: []*TypeParam{nil}, Body: String}, nil
			},
			admits: false,
		},
		{
			name: "opaque declaration without a body",
			build: func() (Type, []*TypeParam) {
				return NewGeneric("Opaque", []*TypeParam{NewTypeParam("T", nil)}, nil), nil
			},
			admits: true,
		},
		{
			name: "explicit mu delimits a generic relation",
			build: func() (Type, []*TypeParam) {
				firstParam := NewTypeParam("T", nil)
				secondParam := NewTypeParam("U", nil)
				first := NewGeneric("First", []*TypeParam{firstParam}, nil)
				second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
				first.SetBody(NewRecursive("FirstBoundary", func(Type) Type {
					return Instantiate(second, firstParam)
				}))
				second.SetBody(Instantiate(first, secondParam))
				return first, nil
			},
			admits: true,
		},
		{
			name: "mu delimited mutual recurrence owns every formal",
			build: func() (Type, []*TypeParam) {
				first, second := staticRecurrenceMutualPair(nil)
				return NewTuple(first, second), nil
			},
			admits: true,
		},
		{
			name: "mu delimited mutual recurrence leaks a formal",
			build: func() (Type, []*TypeParam) {
				first, second := staticRecurrenceMutualPair(NewTypeParam("Leak", nil))
				return NewTuple(first, second), nil
			},
			admits: false,
		},
		{
			name: "declared external formal occurs free",
			build: func() (Type, []*TypeParam) {
				external := NewTypeParam("E", String)
				return NewTuple(external, String), []*TypeParam{external}
			},
			scoped: true,
			admits: true,
		},
		{
			name: "external formal reaches a nested binder body",
			build: func() (Type, []*TypeParam) {
				external := NewTypeParam("E", nil)
				local := NewTypeParam("T", nil)
				return NewGeneric("Holder", []*TypeParam{local}, NewTuple(local, external)), []*TypeParam{external}
			},
			scoped: true,
			admits: true,
		},
		{
			name: "free formal without a declared scope",
			build: func() (Type, []*TypeParam) {
				return NewTuple(NewTypeParam("E", nil), String), nil
			},
			admits: false,
		},
		{
			name: "free formal outside the declared external set",
			build: func() (Type, []*TypeParam) {
				declared := NewTypeParam("E", nil)
				foreign := NewTypeParam("F", nil)
				return NewTuple(declared, foreign), []*TypeParam{declared}
			},
			scoped: true,
			admits: false,
		},
		{
			name: "duplicate declared external formal",
			build: func() (Type, []*TypeParam) {
				declared := NewTypeParam("E", nil)
				return NewTuple(declared, String), []*TypeParam{declared, declared}
			},
			scoped: true,
			admits: false,
		},
		{
			name: "sibling binder does not own a foreign formal",
			build: func() (Type, []*TypeParam) {
				foreign := NewTypeParam("External", nil)
				return NewGeneric("Sibling", []*TypeParam{NewTypeParam("U", nil)}, NewArray(foreign)), nil
			},
			admits: false,
		},
		{
			name: "formal escapes its binder through a sibling arm",
			build: func() (Type, []*TypeParam) {
				inner := NewTypeParam("U", nil)
				innerGeneric := NewGeneric("Inner", []*TypeParam{inner}, NewArray(inner))
				return NewTuple(innerGeneric, inner), nil
			},
			admits: false,
		},
		{
			name: "duplicate formals in one binder",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				return NewGeneric("Duplicate", []*TypeParam{param, param}, NewArray(param)), nil
			},
			admits: false,
		},
		{
			name: "mutual generic group",
			build: func() (Type, []*TypeParam) {
				firstParam := NewTypeParam("T", nil)
				secondParam := NewTypeParam("U", nil)
				first := NewGeneric("First", []*TypeParam{firstParam}, nil)
				second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
				first.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(second, firstParam)}}}))
				second.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(first, secondParam)}}}))
				return first, nil
			},
			admits: false,
		},
		{
			name: "unproductive self equation",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				loop := NewGeneric("Loop", []*TypeParam{param}, nil)
				loop.SetBody(Instantiate(loop, param))
				return loop, nil
			},
			admits: false,
		},
		{
			name: "instantiation arity mismatch",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				generic := NewGeneric("One", []*TypeParam{param}, NewArray(param))
				return Instantiate(generic), nil
			},
			admits: false,
		},
		{
			name: "recurrence hidden in an unused constraint",
			build: func() (Type, []*TypeParam) {
				param := NewTypeParam("T", nil)
				generic := NewGeneric("Constrained", []*TypeParam{param}, String)
				param.Constraint = Instantiate(generic, param)
				return generic, nil
			},
			admits: false,
		},
		{
			name: "open scope admits an undeclared free formal",
			build: func() (Type, []*TypeParam) {
				return NewTuple(NewTypeParam("E", nil), String), nil
			},
			open:   true,
			admits: true,
		},
		{
			name: "open scope still rejects a mutual generic group",
			build: func() (Type, []*TypeParam) {
				firstParam := NewTypeParam("T", nil)
				secondParam := NewTypeParam("U", nil)
				first := NewGeneric("First", []*TypeParam{firstParam}, nil)
				second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
				first.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(second, firstParam)}}}))
				second.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(first, NewTypeParam("Free", nil))}}}))
				return first, nil
			},
			open:   true,
			admits: false,
		},
		{
			name: "absent declared external formal",
			build: func() (Type, []*TypeParam) {
				return NewTuple(String, Number), []*TypeParam{nil}
			},
			scoped: true,
			admits: false,
		},
	}
}

// staticRecurrenceAnnotate wraps a node in a transparent annotation. The
// wrapper carries presentation only, so the law's verdict on the node beneath
// it is the law's verdict on the wrapper.
func staticRecurrenceAnnotate(inner Type) Type {
	return NewAnnotated(inner, []annotation.Annotation{{Name: "presentation", Arg: annotation.Int64Arg(1)}})
}

// staticRecurrenceMutualPair builds two generic declarations that reach each
// other through an explicit mu boundary, so the generic relation is admitted
// and the formal ownership verdict is the only one left to decide. A non-nil
// leak occurs in the second declaration without a binder that owns it, and is
// visible from the first declaration only through the cycle.
func staticRecurrenceMutualPair(leak *TypeParam) (*Generic, *Generic) {
	firstParam := NewTypeParam("T", nil)
	secondParam := NewTypeParam("U", nil)
	first := NewGeneric("First", []*TypeParam{firstParam}, nil)
	second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
	first.SetBody(NewRecursive("FirstBoundary", func(Type) Type {
		return Instantiate(second, firstParam)
	}))
	second.SetBody(NewRecursive("SecondBoundary", func(Type) Type {
		if leak == nil {
			return NewTuple(Instantiate(first, secondParam), String)
		}
		return NewTuple(Instantiate(first, secondParam), leak)
	}))
	return first, second
}

// staticRecurrenceBuilderChain grows the fluent-builder shape: every level is a
// record of methods that each return the level below, so one shared subgraph is
// reached under many distinct lexical binder chains while the whole graph stays
// closed.
func staticRecurrenceBuilderChain(depth int) Type {
	current := Type(String)
	for level := 0; level < depth; level++ {
		plain := Func().Param("value", String).Returns(current).Build()
		param := NewTypeParam("T", nil)
		generic := Func().TypeParamRef(param).Param("value", param).Returns(current).Build()
		current = RebuildRecord(RecordParts{Fields: []Field{
			{Name: "with", Type: plain},
			{Name: "map", Type: generic},
		}})
	}
	return current
}

func TestStaticGenericRecurrenceScopeLaw(t *testing.T) {
	for _, testCase := range staticRecurrenceScopeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			root, formals := testCase.build()
			var err error
			switch {
			case testCase.scoped:
				err = ValidateStaticGenericRecurrenceWithFormals(root, formals)
			case testCase.open:
				err = ValidateStaticGenericRecurrenceOpen(root)
			default:
				err = ValidateStaticGenericRecurrence(root)
			}
			if testCase.admits {
				if err != nil {
					t.Fatalf("admitted shape rejected: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
				t.Fatalf("rejected shape admitted: err=%v", err)
			}
		})
	}
}

// BenchmarkStaticGenericRecurrenceBuilderChain reports the cost of the single
// cold derivation over the fluent-builder shape. The walk visits each node once
// per round, and the rounds are bounded by the free-set fixpoint convergence,
// so the receipt is the law's real cost curve.
func BenchmarkStaticGenericRecurrenceBuilderChain(b *testing.B) {
	root := staticRecurrenceBuilderChain(9)
	if err := ValidateStaticGenericRecurrence(root); err != nil {
		b.Fatalf("builder chain rejected: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidateStaticGenericRecurrence(root); err != nil {
			b.Fatal(err)
		}
	}
}
