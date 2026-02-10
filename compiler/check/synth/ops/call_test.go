package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallWithGenericInference_Simple(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Param("y", typ.String).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type != typ.Boolean {
		t.Errorf("expected boolean, got %v", result.Type)
	}

	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestCallWithGenericInference_TooFewArgs(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Param("y", typ.String).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected wrong arity error")
	}
}

func TestCallWithGenericInference_TooManyArgs(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String, typ.Boolean},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected wrong arity error")
	}
}

func TestCallWithGenericInference_Variadic(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Variadic(typ.String).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String, typ.String, typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) > 0 {
		t.Errorf("variadic should accept extra args: %v", result.Errors)
	}
}

func TestCallWithGenericInference_Mismatch(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected type mismatch error")
	}
}

func TestCallWithGenericInference_NotCallable(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Integer,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected not callable error")
	}
}

func TestCallWithGenericInference_NilCallee(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: nil,
		Args:   []typ.Type{},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected error for nil callee")
	}
}

func TestCallWithGenericInference_AnyCallee(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Any,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type != typ.Any {
		t.Errorf("calling any should return any, got %v", result.Type)
	}
}

func TestCallWithGenericInference_MultiReturn(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String, typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type == nil {
		t.Fatal("expected non-nil result type")
	}

	tuple, ok := result.Type.(*typ.Tuple)
	if !ok {
		t.Fatalf("expected tuple, got %T", result.Type)
	}

	if len(tuple.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(tuple.Elements))
	}

	if tuple.Elements[0] != typ.String {
		t.Errorf("first return should be string, got %v", tuple.Elements[0])
	}

	if tuple.Elements[1] != typ.Boolean {
		t.Errorf("second return should be boolean, got %v", tuple.Elements[1])
	}
}

func TestCallWithGenericInference_NoReturn(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type != typ.Nil {
		t.Errorf("function with no return should return nil, got %v", result.Type)
	}
}

func TestCallDef_Fields(t *testing.T) {
	def := CallDef{
		Callee:     typ.Func().Build(),
		Args:       []typ.Type{typ.Integer},
		IsMethod:   true,
		Receiver:   typ.String,
		MethodName: "test",
	}
	if def.Callee == nil {
		t.Error("callee should not be nil")
	}

	if len(def.Args) != 1 {
		t.Error("should have 1 arg")
	}

	if !def.IsMethod {
		t.Error("should be method call")
	}

	if def.Receiver != typ.String {
		t.Error("receiver should be string")
	}

	if def.MethodName != "test" {
		t.Error("method name should be test")
	}
}

func TestCallErrorKind_Constants(t *testing.T) {
	if ErrNotCallable != 0 {
		t.Error("ErrNotCallable should be 0")
	}

	if ErrWrongArity != 1 {
		t.Error("ErrWrongArity should be 1")
	}

	if ErrTypeMismatch != 2 {
		t.Error("ErrTypeMismatch should be 2")
	}

	if ErrOptionalCall != 3 {
		t.Error("ErrOptionalCall should be 3")
	}
}

func TestCallWithGenericInference_OptionalCallee(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	optFn := typ.NewOptional(fn)

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: optFn,
		Args:   []typ.Type{},
	}

	result := CallWithGenericInference(ctx, def)
	hasOptError := false

	for _, err := range result.Errors {
		if err.Kind == ErrOptionalCall {
			hasOptError = true
		}
	}

	if !hasOptError {
		t.Error("should report optional call error")
	}
}

func TestInferCall_Function(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	infer := InferCall(ctx, def)
	if infer.Kind != InferKindFunction {
		t.Errorf("expected InferKindFunction, got %v", infer.Kind)
	}

	if infer.Instantiated == nil {
		t.Fatal("expected non-nil instantiated")
	}

	if len(infer.ExpectedArgs) != 1 {
		t.Fatalf("expected 1 expected arg, got %d", len(infer.ExpectedArgs))
	}

	if infer.ExpectedArgs[0] != typ.Integer {
		t.Errorf("expected arg 0 = integer, got %v", infer.ExpectedArgs[0])
	}
}

func TestFinishCall_ShortCircuit(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Any,
		Args:   []typ.Type{},
	}

	infer := InferCall(ctx, def)
	result := FinishCall(ctx, def, infer)

	if result.Type != typ.Any {
		t.Errorf("expected any, got %v", result.Type)
	}
}

func TestReInfer_NonGeneric(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	infer := InferCall(ctx, def)
	reInferred := ReInfer(ctx, def, infer)

	if reInferred.Kind != infer.Kind {
		t.Error("non-generic should return same result")
	}
}

func TestHasExplicitSelfSimple_RejectsTopLikeFirstParam(t *testing.T) {
	receiver := typ.NewRecord().Field("id", typ.String).Build()

	anyFirst := typ.Func().
		Param("options", typ.Any).
		Returns(typ.Boolean).
		Build()
	if hasExplicitSelfSimple(anyFirst, receiver) {
		t.Fatal("any first param should not be treated as explicit self")
	}

	unknownFirst := typ.Func().
		Param("value", typ.Unknown).
		Returns(typ.Boolean).
		Build()
	if hasExplicitSelfSimple(unknownFirst, receiver) {
		t.Fatal("unknown first param should not be treated as explicit self")
	}
}

func TestHasExplicitSelfSimple_StillAcceptsExplicitPatterns(t *testing.T) {
	receiver := typ.NewRecord().Field("id", typ.String).Build()

	namedSelf := typ.Func().
		Param("self", typ.Any).
		Returns(typ.Boolean).
		Build()
	if !hasExplicitSelfSimple(namedSelf, receiver) {
		t.Fatal("parameter named self should be treated as explicit self")
	}

	matchingType := typ.Func().
		Param("receiver", receiver).
		Returns(typ.Boolean).
		Build()
	if !hasExplicitSelfSimple(matchingType, receiver) {
		t.Fatal("receiver-compatible first param should be treated as explicit self")
	}
}
