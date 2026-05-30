package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZGenInferProbe reconstructs the gradual-typing-adversarial `ok<T>` call
// to confirm whether bidirectional inference binds T from the expected return
// Validation<Config>. Read-only diagnostic probe.
func TestZZGenInferProbe(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)

	// Validation<T> = {ok: true, value: T} | {ok: false, error: string}
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", tp).Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).Build()
	validationBody := typ.NewUnion(okVariant, errVariant)
	validation := typ.NewGeneric("Validation", []*typ.TypeParam{tp}, validationBody)

	// ok<T>(value: T): Validation<T>
	okFn := typ.Func().
		TypeParam("T", nil).
		Param("value", tp).
		Returns(typ.Instantiate(validation, tp)).
		Build()
	t.Logf("okFn TypeParams=%d returns[0]=%s", len(okFn.TypeParams), okFn.Returns[0].String())

	// Config = {id, retries, labels, metadata}
	config := typ.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Field("labels", typ.NewArray(typ.String)).
		Field("metadata", typ.NewMap(typ.String, typ.String)).Build()
	configAlias := typ.NewAlias("Config", config)

	// Argument record literal: {id, retries, labels, metadata} (concrete)
	argRec := typ.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Field("labels", typ.NewArray(typ.String)).
		Field("metadata", typ.NewMap(typ.String, typ.String)).Build()

	expectedReturn := typ.Instantiate(validation, configAlias)
	t.Logf("expectedReturn=%s", expectedReturn.String())

	// Inference with expected return (bidirectional).
	args, err := InferTypeArgsWithExpectedAndMode(okFn, []typ.Type{argRec}, false, nil, expectedReturn, false)
	t.Logf("infer-with-expected: err=%v args=%v", err, fmtTypes(args))

	// Inference without expected return (arg-only).
	args2, err2 := InferTypeArgsWithExpectedAndMode(okFn, []typ.Type{argRec}, false, nil, nil, false)
	t.Logf("infer-arg-only: err=%v args=%v", err2, fmtTypes(args2))

	if len(args) == 1 {
		inst := InstantiateFunction(okFn, args)
		t.Logf("instantiated-with-expected returns[0]=%s", inst.Returns[0].String())
	}
	if len(args2) == 1 {
		inst := InstantiateFunction(okFn, args2)
		t.Logf("instantiated-arg-only returns[0]=%s", inst.Returns[0].String())
	}

	// Arg whose labels field carries a residual TypeParam (simulating an
	// un-substituted prior generic-call value field leaking into the arg).
	argWithTP := typ.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Field("labels", typ.NewArray(tp)).
		Field("metadata", typ.NewMap(typ.String, typ.String)).Build()
	a3, e3 := InferTypeArgsWithExpectedAndMode(okFn, []typ.Type{argWithTP}, false, nil, expectedReturn, false)
	t.Logf("arg-with-typeparam-labels: err=%v args=%v", e3, fmtTypes(a3))
	if e3 == nil && len(a3) == 1 {
		t.Logf("  instantiated returns[0]=%s", InstantiateFunction(okFn, a3).Returns[0].String())
	}

	// Arg whose labels field is unknown (degraded read).
	argUnk := typ.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Field("labels", typ.Unknown).
		Field("metadata", typ.NewMap(typ.String, typ.String)).Build()
	a4, e4 := InferTypeArgsWithExpectedAndMode(okFn, []typ.Type{argUnk}, false, nil, expectedReturn, false)
	t.Logf("arg-with-unknown-labels: err=%v args=%v", e4, fmtTypes(a4))
	if e4 == nil && len(a4) == 1 {
		t.Logf("  instantiated returns[0]=%s", InstantiateFunction(okFn, a4).Returns[0].String())
	}
}

func fmtTypes(ts []typ.Type) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		if t == nil {
			out[i] = "<nil>"
		} else {
			out[i] = t.String()
		}
	}
	return out
}
