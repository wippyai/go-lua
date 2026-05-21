package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewCallPipeline(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Func().Build(),
	}
	pipeline := NewCallPipeline(ctx, def, 1)
	if pipeline == nil {
		t.Fatal("expected non-nil pipeline")
	}
	if pipeline.ctx != ctx {
		t.Fatal("context mismatch")
	}
}

func TestCallPipeline_WithReSynth(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Func().Build(),
	}
	pipeline := NewCallPipeline(ctx, def, 0)

	reSynth := func(idx int, expected typ.Type) typ.Type {
		return typ.String
	}

	result := pipeline.WithReSynth(reSynth)
	if result != pipeline {
		t.Fatal("expected same pipeline returned")
	}
	if pipeline.reSynth == nil {
		t.Fatal("expected reSynth to be set")
	}
}

func TestCallPipeline_WithExpected(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Func().Build(),
	}
	pipeline := NewCallPipeline(ctx, def, 0)

	expected := typ.String
	result := pipeline.WithExpected(expected)
	if result != pipeline {
		t.Fatal("expected same pipeline returned")
	}
	if pipeline.def.ExpectedReturn != expected {
		t.Fatal("expected return type not set")
	}
}

func TestCallPipeline_Infer(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}
	pipeline := NewCallPipeline(ctx, def, 0)

	infer := pipeline.Infer()
	if infer.Callee == nil {
		t.Fatal("expected callee to be resolved")
	}
}

func TestCallPipeline_ExpectedArgType_InRange(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().
		Param("x", typ.Integer).
		Param("y", typ.String).
		Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String},
	}
	pipeline := NewCallPipeline(ctx, def, 0)
	pipeline.Infer()

	arg0 := pipeline.ExpectedArgType(0)
	if arg0 != typ.Integer {
		t.Fatalf("got %v, want integer", arg0)
	}
	arg1 := pipeline.ExpectedArgType(1)
	if arg1 != typ.String {
		t.Fatalf("got %v, want string", arg1)
	}
}

func TestCallPipeline_ExpectedArgType_OutOfRange(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().
		Param("x", typ.Integer).
		Variadic(typ.String).
		Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String, typ.String},
	}
	pipeline := NewCallPipeline(ctx, def, 0)
	pipeline.Infer()

	arg5 := pipeline.ExpectedArgType(5)
	if arg5 != typ.String {
		t.Fatalf("got %v, want string (variadic)", arg5)
	}
}

func TestCallPipeline_ExpectedArgType_Intersection(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fnA := typ.Func().Param("x", typ.String).Returns(typ.Any).Build()
	fnB := typ.Func().Param("x", typ.String).Returns(typ.Unknown).Build()
	def := CallDef{
		Callee: typ.NewIntersection(fnA, fnB),
		Args:   []typ.Type{typ.NewOptional(typ.String)},
	}
	pipeline := NewCallPipeline(ctx, def, 1)
	pipeline.Infer()

	arg0 := pipeline.ExpectedArgType(0)
	if arg0 != typ.String {
		t.Fatalf("got %v, want string", arg0)
	}
}

func TestCallPipeline_IntersectionReSynthesizesLogicalArg(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fnA := typ.Func().Param("x", typ.String).Returns(typ.Any).Build()
	fnB := typ.Func().Param("x", typ.String).Returns(typ.Unknown).Build()
	def := CallDef{
		Callee: typ.NewIntersection(fnA, fnB),
		Args:   []typ.Type{typ.NewOptional(typ.String)},
	}
	pipeline := NewCallPipeline(ctx, def, 1).
		WithReSynth(func(idx int, expected typ.Type) typ.Type {
			if idx != 0 {
				t.Fatalf("got idx %d, want 0", idx)
			}
			if expected != typ.String {
				t.Fatalf("got expected %v, want string", expected)
			}
			return typ.String
		})

	result := pipeline.Run()
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors after contextual re-synthesis, got %v", result.Errors)
	}
}

func TestCallPipeline_ReSynthAndReInfer_NoReSynth(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Build()
	def := CallDef{Callee: fn}
	pipeline := NewCallPipeline(ctx, def, 0)
	pipeline.Infer()

	changed := pipeline.ReSynthAndReInfer()
	if changed {
		t.Fatal("expected no change without reSynth")
	}
}

func TestCallPipeline_ReSynthAndReInfer_NoArgs(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Build()
	def := CallDef{Callee: fn}
	pipeline := NewCallPipeline(ctx, def, 0)
	pipeline.WithReSynth(func(idx int, expected typ.Type) typ.Type {
		return typ.String
	})
	pipeline.Infer()

	changed := pipeline.ReSynthAndReInfer()
	if changed {
		t.Fatal("expected no change without args")
	}
}

func TestCallPipeline_ReSynthAndReInfer_UnchangedArgDoesNotReInfer(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Param("value", typ.String).Returns(typ.Any).Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.String},
	}
	pipeline := NewCallPipeline(ctx, def, 1).
		WithReSynth(func(idx int, expected typ.Type) typ.Type {
			if idx != 0 {
				t.Fatalf("unexpected re-synth arg idx=%d", idx)
			}
			if expected != typ.String {
				t.Fatalf("expected arg type = %v, want string", expected)
			}
			return typ.String
		})
	pipeline.Infer()

	if changed := pipeline.ReSynthAndReInfer(); changed {
		t.Fatal("unchanged contextual type should not force re-inference")
	}
}

func TestCallPipeline_ReSynthAndReInfer_DoesNotWeakenExistingArg(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Param("value", typ.String).Returns(typ.Any).Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.String},
	}
	pipeline := NewCallPipeline(ctx, def, 1).
		WithReSynth(func(idx int, expected typ.Type) typ.Type {
			return typ.Any
		})
	pipeline.Infer()

	if changed := pipeline.ReSynthAndReInfer(); changed {
		t.Fatal("weaker contextual type should not replace existing argument evidence")
	}
	if pipeline.def.Args[0] != typ.String {
		t.Fatalf("arg type = %v, want string", pipeline.def.Args[0])
	}
}

func TestCallPipeline_ReSynthAndReInfer_FillsUnknownArg(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Param("value", typ.String).Returns(typ.Any).Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Unknown},
	}
	pipeline := NewCallPipeline(ctx, def, 1).
		WithReSynth(func(idx int, expected typ.Type) typ.Type {
			return typ.String
		})
	pipeline.Infer()

	if changed := pipeline.ReSynthAndReInfer(); !changed {
		t.Fatal("more precise contextual type should replace unknown argument evidence")
	}
	if pipeline.def.Args[0] != typ.String {
		t.Fatalf("arg type = %v, want string", pipeline.def.Args[0])
	}
}

func TestCallPipeline_Finish(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Returns(typ.String).Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{},
	}
	pipeline := NewCallPipeline(ctx, def, 0)
	pipeline.Infer()

	result := pipeline.Finish()
	if result.Type == nil {
		t.Fatal("expected non-nil result type")
	}
	if !pipeline.finished {
		t.Fatal("expected finished flag to be set")
	}
}

func TestCallPipeline_Run(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Returns(typ.Integer).Build()
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{},
	}
	pipeline := NewCallPipeline(ctx, def, 0)

	result := pipeline.Run()
	if result.Type == nil {
		t.Fatal("expected non-nil result type")
	}
}
