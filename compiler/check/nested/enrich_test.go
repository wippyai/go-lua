package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEnrichTableTypeWithFunctionLookup_NilInputs(t *testing.T) {
	result := EnrichTableTypeWithFunctionLookup(nil, nil, nil, nil)
	if rec, ok := result.(*typ.Record); !ok || rec != nil {
		t.Error("expected nil record for nil inputs")
	}
}

func TestEnrichTableTypeWithFunctionLookup_NilRecord(t *testing.T) {
	result := EnrichTableTypeWithFunctionLookup(nil, nil, &cfg.Graph{}, nil)
	if rec, ok := result.(*typ.Record); !ok || rec != nil {
		t.Error("expected nil record for nil record input")
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilInputs(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilSelfType(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, nil)
	if result != nil {
		t.Error("expected nil for nil selfType")
	}
}

func TestMethodSelfTypeFromReceiverSurface_UsesReceiverAsPrototype(t *testing.T) {
	receiver := typ.NewRecord().
		Field("__index", typ.Any).
		Field("run", typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()).
		Build()

	selfType := MethodSelfTypeFromReceiverSurface(receiver)
	if _, ok := querycore.Method(selfType, "run"); !ok {
		t.Fatalf("instance self should resolve prototype method, got %s", typ.FormatShort(selfType))
	}
}

func TestNormalizeMethodSelfType_FoldsSelfReturningReceiverFamily(t *testing.T) {
	seed := typ.NewRecord().
		Field("new", typ.Func().Returns(typ.Unknown).Build()).
		Field("add", typ.Func().Returns(typ.Unknown).Build()).
		Field("value", typ.Integer).
		Build()
	selfType := typ.NewRecord().
		Field("new", typ.Func().Returns(seed).Build()).
		Field("add", typ.Func().Param("self", seed).Returns(seed).Build()).
		Field("value", typ.Integer).
		Build()

	normalized := NormalizeMethodSelfType(selfType)
	rec, ok := normalized.(*typ.Recursive)
	if !ok {
		t.Fatalf("NormalizeMethodSelfType() = %T, want recursive receiver product", normalized)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive receiver body = %T, want record", rec.Body)
	}
	add := body.GetField("add")
	if add == nil {
		t.Fatal("recursive receiver lost add method")
	}
	method, ok := add.Type.(*typ.Function)
	if !ok {
		t.Fatalf("add method = %T, want function", add.Type)
	}
	if len(method.Params) != 1 || !typ.IsRecursiveRef(method.Params[0].Type, rec) {
		t.Fatalf("method self param should be recursive receiver ref, got %#v", method.Params)
	}
	if len(method.Returns) != 1 || !typ.IsRecursiveRef(method.Returns[0], rec) {
		t.Fatalf("method return should be recursive receiver ref, got %#v", method.Returns)
	}
}

func TestMergeFieldsIntoSelfType_EmptyFields(t *testing.T) {
	selfType := typ.Number
	result := mergeFieldsIntoSelfType(selfType, nil)
	if result != selfType {
		t.Errorf("expected original selfType for empty fields, got %v", result)
	}
}

func TestMergeFieldsIntoSelfType_NonRecordNonInterface(t *testing.T) {
	selfType := typ.Number
	fields := interproc.LiftTypeFieldMap(map[string]typ.Type{"x": typ.String})
	result := mergeFieldsIntoSelfType(selfType, fields)
	if result != selfType {
		t.Errorf("expected original selfType for non-record/interface, got %v", result)
	}
}
