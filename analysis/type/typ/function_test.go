package typ

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestFunctionEmpty(t *testing.T) {
	f := Func().Build()

	if f.Kind() != kind.Function {
		t.Errorf("Kind: got %v, want Function", f.Kind())
	}

	if len(f.Params) != 0 {
		t.Errorf("Params: got %d, want 0", len(f.Params))
	}

	if len(f.Returns) != 0 {
		t.Errorf("Returns: got %d, want 0", len(f.Returns))
	}

	if f.String() != "fun()" {
		t.Errorf("String: got %q, want %q", f.String(), "fun()")
	}
}

func TestFunctionWithParams(t *testing.T) {
	f := Func().
		Param("x", Number).
		Param("y", String).
		Build()

	if len(f.Params) != 2 {
		t.Errorf("Params: got %d, want 2", len(f.Params))
	}

	if f.Params[0].Name != "x" || f.Params[0].Type != Number {
		t.Error("first param should be x: number")
	}

	if f.Params[1].Name != "y" || f.Params[1].Type != String {
		t.Error("second param should be y: string")
	}

	if f.String() != "fun(x: number, y: string)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionWithOptParam(t *testing.T) {
	f := Func().
		Param("x", Number).
		OptParam("y", String).
		Build()

	if !f.Params[1].Optional {
		t.Error("second param should be optional")
	}

	if f.String() != "fun(x: number, y: string?)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionWithVariadic(t *testing.T) {
	f := Func().
		Param("x", Number).
		Variadic(String).
		Build()

	if f.Variadic != String {
		t.Error("Variadic should be String")
	}

	if f.String() != "fun(x: number, ...string)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionWithReturns(t *testing.T) {
	f := Func().
		Param("x", Number).
		Returns(Boolean).
		Build()

	if len(f.Returns) != 1 {
		t.Errorf("Returns: got %d, want 1", len(f.Returns))
	}

	if f.String() != "fun(x: number) -> boolean" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionMultiReturn(t *testing.T) {
	f := Func().
		Returns(Number, String, Boolean).
		Build()

	if len(f.Returns) != 3 {
		t.Errorf("Returns: got %d, want 3", len(f.Returns))
	}

	if f.String() != "fun() -> (number, string, boolean)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionReturnTupleCanonicalizesToReturnList(t *testing.T) {
	single := Func().
		Returns(NewTuple(String)).
		Build()
	if len(single.Returns) != 1 || single.Returns[0] != String {
		t.Fatalf("single tuple returns = %#v, want scalar string return", single.Returns)
	}
	if single.String() != "fun() -> string" {
		t.Fatalf("single tuple string = %q, want scalar return display", single.String())
	}

	multi := Func().
		Returns(Boolean, NewTuple(Number, String)).
		Build()
	if len(multi.Returns) != 3 || multi.Returns[0] != Boolean || multi.Returns[1] != Number || multi.Returns[2] != String {
		t.Fatalf("multi tuple returns = %#v, want flattened return list", multi.Returns)
	}
	if multi.String() != "fun() -> (boolean, number, string)" {
		t.Fatalf("multi tuple string = %q, want flattened return display", multi.String())
	}
}

func TestFunctionEquality(t *testing.T) {
	f1 := Func().Param("x", Number).Returns(Boolean).Build()
	f2 := Func().Param("y", Number).Returns(Boolean).Build()
	f3 := Func().Param("x", String).Returns(Boolean).Build()
	f4 := Func().Param("x", Number).Returns(String).Build()

	if !f1.Equals(f2) {
		t.Error("functions with same types should be equal (names ignored)")
	}

	if f1.Equals(f3) {
		t.Error("functions with different param types should not be equal")
	}

	if f1.Equals(f4) {
		t.Error("functions with different return types should not be equal")
	}
}

func TestFunctionEqualityOptional(t *testing.T) {
	f1 := Func().Param("x", Number).Build()
	f2 := Func().OptParam("x", Number).Build()

	if f1.Equals(f2) {
		t.Error("required vs optional param should differ")
	}
}

func TestFunctionEqualityVariadic(t *testing.T) {
	f1 := Func().Variadic(Number).Build()
	f2 := Func().Build()
	f3 := Func().Variadic(String).Build()

	if f1.Equals(f2) {
		t.Error("variadic vs non-variadic should differ")
	}

	if f1.Equals(f3) {
		t.Error("different variadic types should differ")
	}
}

func TestFunctionHashConsistency(t *testing.T) {
	f1 := Func().Param("x", Number).Returns(Boolean).Build()
	f2 := Func().Param("y", Number).Returns(Boolean).Build()

	if f1.Hash() != f2.Hash() {
		t.Error("functions with same signature should have same hash")
	}

	f3 := Func().Param("x", Number).Build()
	f4 := Func().Param("x", String).Build()

	if f3.Hash() == f4.Hash() {
		t.Error("functions with different param types should have different hash")
	}
}

func TestFunctionNotEqualToPrimitive(t *testing.T) {
	f := Func().Returns(Number).Build()
	if f.Equals(Number) {
		t.Error("function should not equal primitive")
	}
}

func TestFunctionBuild_PanicsOnNilReturn(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in returns; normalize before building", func() {
		Func().Returns(Number, nil, String).Build()
	})
}

func TestFunctionBuild_PanicsOnTypedNilReturn(t *testing.T) {
	var nilRecord *Record
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in returns; normalize before building", func() {
		Func().Returns(Number, nilRecord, String).Build()
	})
}

func TestFunctionBuild_PanicsOnNilParamType(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in params; normalize before building", func() {
		Func().Param("x", nil).Build()
	})
}

func TestFunctionBuild_PanicsOnTypedNilParamType(t *testing.T) {
	var nilRecord *Record
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in params; normalize before building", func() {
		Func().Param("x", nilRecord).Build()
	})
}

func TestFunctionBuild_TreatsTypedNilVariadicAsAbsent(t *testing.T) {
	var nilRecord *Record
	f := Func().Variadic(nilRecord).Build()

	if f.Variadic != nil {
		t.Fatalf("typed nil variadic should normalize to absent, got %v", f.Variadic)
	}
	if f.String() != "fun()" {
		t.Fatalf("typed nil variadic should not render a variadic slot: %q", f.String())
	}
}

func TestRebuildFunction_PanicsOnNilTypeParam(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in type params; normalize before building", func() {
		RebuildFunction(FunctionParts{TypeParams: []*TypeParam{nil}})
	})
}

func TestRebuildFunction_PanicsOnNilReturn(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in returns; normalize before building", func() {
		RebuildFunction(FunctionParts{Returns: []Type{Number, nil, String}})
	})
}

func expectFunctionBuildPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok || msg != want {
			t.Fatalf("unexpected panic message: %v", r)
		}
	}()

	fn()
}

func TestFunctionBuild_ValidReturns(t *testing.T) {
	f := Func().Returns(Number, String).Build()
	if len(f.Returns) != 2 {
		t.Fatalf("expected 2 returns, got %d", len(f.Returns))
	}
	if f.Returns[0] != Number {
		t.Errorf("expected Number at index 0, got %v", f.Returns[0])
	}
	if f.Returns[1] != String {
		t.Errorf("expected String at index 1, got %v", f.Returns[1])
	}
}

func TestFunctionReceiverMetadataDerivesAndSurvivesConstructionBoundaries(t *testing.T) {
	built := Func().Param("self", Self).Param("value", String).Build()
	if !built.Params[0].Receiver || built.Params[1].Receiver {
		t.Fatalf("built receiver flags = %v/%v", built.Params[0].Receiver, built.Params[1].Receiver)
	}
	rebuilt := RebuildFunction(FunctionParts{
		Params:  []Param{{Name: "ctx", Type: Any, Receiver: true}, {Name: "value", Type: String}},
		Returns: []Type{built},
	})
	if !rebuilt.Params[0].Receiver || rebuilt.Params[0].Name != "ctx" || rebuilt.String() != "fun(ctx: any, value: string) -> fun(self: self, value: string)" {
		t.Fatalf("rebuilt receiver metadata/display = %#v / %s", rebuilt.Params[0], rebuilt)
	}
	cloned := CloneFunction(rebuilt)
	if !cloned.Params[0].Receiver || cloned.Params[0].Name != "ctx" || !TypeEquals(cloned, rebuilt) || cloned.Hash() != rebuilt.Hash() {
		t.Fatalf("clone lost receiver metadata: %#v", cloned.Params[0])
	}
}

func TestFunctionPresentationAndSemanticViewsAreImmutableAndSeparated(t *testing.T) {
	fn := Func().Param("self", Self).OptParam("payload", String).Returns(Boolean).Build()
	presentation := fn.Presentation()
	if got := presentation.ParamNames(); len(got) != 2 || got[0] != "self" || got[1] != "payload" {
		t.Fatalf("presentation labels = %v", got)
	}
	labels := presentation.ParamNames()
	labels[1] = "mutated"
	if got, _ := presentation.ParamName(1); got != "payload" {
		t.Fatalf("presentation exposed mutable labels: %q", got)
	}

	semantic := fn.SemanticType()
	if semantic == nil || semantic != fn.SemanticType() || len(semantic.Params) != 2 || len(semantic.Returns) != 1 {
		t.Fatal("semantic view is not stable/O(1)")
	}
	self := semantic.Params[0]
	if !self.Receiver || self.Type != Self || self.Optional || self.Name != "self" {
		t.Fatalf("semantic self = %#v", self)
	}
	payload := semantic.Params[1]
	if payload.Receiver || payload.Type != String || !payload.Optional || payload.Name != "" {
		t.Fatalf("semantic payload = %#v", payload)
	}
	if ret := semantic.Returns[0]; ret != Boolean {
		t.Fatalf("semantic return = %v", ret)
	}
}

func TestFunctionSemanticViewSharesRecursiveGenericChildren(t *testing.T) {
	param := NewTypeParam("T", String)
	recursive := NewRecursive("Node", func(self Type) Type { return NewArray(self) })
	fn := RebuildFunction(FunctionParts{
		TypeParams: []*TypeParam{param},
		Params:     []Param{{Name: "node", Type: recursive}},
		Variadic:   param,
		Returns:    []Type{recursive},
	})
	semantic := fn.SemanticType()
	if semantic.Params[0].Type != recursive || semantic.Variadic != param || semantic.TypeParams[0] != param || semantic.Returns[0] != recursive {
		t.Fatal("semantic view copied recursive/generic child nodes")
	}
	clone := CloneFunction(fn)
	if clone.SemanticType() == semantic || !TypeEquals(clone.SemanticType(), semantic) || clone.SemanticType().Hash() != semantic.Hash() {
		t.Fatal("clone semantic view ownership/identity is invalid")
	}
	if label, _ := clone.Presentation().ParamName(0); label != "node" {
		t.Fatalf("clone presentation label = %q", label)
	}
}

func TestFunctionSemanticTypeDocumentsCompositeChildBoundary(t *testing.T) {
	nested := Func().Param("payload", String).Build()
	record := RebuildRecord(RecordParts{Fields: []Field{{Name: "run", Type: nested}}})
	outer := Func().Param("wrapper", record).Build()
	semanticRecord, ok := outer.SemanticType().Params[0].Type.(*Record)
	if !ok || semanticRecord != record {
		t.Fatal("PR2a must share the composite child without a graph walk")
	}
	field := semanticRecord.GetField("run")
	if field == nil || field.Type != nested || field.Type == nested.SemanticType() {
		t.Fatal("test no longer demonstrates the bounded composite-child limitation")
	}
}

func TestFunctionSemanticTypeConcurrentPublicationIsStable(t *testing.T) {
	fn := Func().Param("payload", String).Returns(Any).Build()
	const workers = 32
	got := make([]*Function, workers)
	var group sync.WaitGroup
	for i := range got {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			got[index] = fn.SemanticType()
		}(i)
	}
	group.Wait()
	for i := 1; i < len(got); i++ {
		if got[i] != got[0] {
			t.Fatalf("semantic publication differs at %d", i)
		}
	}
}

func TestFunctionReceiverConventionParticipatesInIdentity(t *testing.T) {
	receiver := RebuildFunction(FunctionParts{Params: []Param{{Name: "ctx", Type: Any, Receiver: true}}})
	ordinary := RebuildFunction(FunctionParts{Params: []Param{{Name: "ctx", Type: Any}}})
	if TypeEquals(receiver, ordinary) || receiver.Hash() == ordinary.Hash() {
		t.Fatal("receiver convention collapsed in function identity")
	}
	labelVariant := RebuildFunction(FunctionParts{Params: []Param{{Name: "actor", Type: Any, Receiver: true}}})
	if !TypeEquals(receiver, labelVariant) || receiver.Hash() != labelVariant.Hash() {
		t.Fatal("presentation-only receiver label changed function identity")
	}
}

func TestRecursiveGenericFunctionReceiverIdentityIsStable(t *testing.T) {
	build := func(receiver bool) Type {
		param := NewTypeParam("T", String)
		return NewRecursive("Node", func(self Type) Type {
			method := RebuildFunction(FunctionParts{
				TypeParams: []*TypeParam{param},
				Params:     []Param{{Name: "ctx", Type: self, Receiver: receiver}, {Name: "value", Type: param}},
				Returns:    []Type{self},
			})
			return NewArray(method)
		})
	}
	left, right := build(true), build(true)
	if !TypeEquals(left, right) || EqualityHash(left) != EqualityHash(right) {
		t.Fatal("equal recursive generic receiver functions are unstable")
	}
	if plain := build(false); TypeEquals(left, plain) || EqualityHash(left) == EqualityHash(plain) {
		t.Fatal("recursive receiver convention was ignored")
	}
}

func BenchmarkFunctionReceiverConstruction(b *testing.B) {
	parts := FunctionParts{Params: []Param{{Name: "self", Type: Self}, {Name: "value", Type: String}}, Returns: []Type{Any}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = RebuildFunction(parts)
	}
}

func BenchmarkFunctionSemanticViewSteadyState(b *testing.B) {
	fn := Func().Param("self", Self).Param("payload", String).Returns(Any).Build()
	want := fn.SemanticType()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		semantic := fn.SemanticType()
		if semantic != want || semantic.Params[1].Type != String || semantic.Hash() == 0 {
			b.Fatal("invalid semantic view")
		}
	}
}

func BenchmarkFunctionSemanticTypeFirstMaterialization(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fn := Func().Param("payload", String).Returns(Any).Build()
		_ = fn.SemanticType()
	}
}
