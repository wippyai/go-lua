package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

type stubStore struct{}

func (s stubStore) LookupConstructorFields(cfg.SymbolID) map[string]typ.Type { return nil }

func TestEnrichTableTypeWithFuncTypes_NilInputs(t *testing.T) {
	result := EnrichTableTypeWithFuncTypes(nil, nil, nil, nil)
	if rec, ok := result.(*typ.Record); !ok || rec != nil {
		t.Error("expected nil record for nil inputs")
	}
}

func TestEnrichTableTypeWithFuncTypes_NilRecord(t *testing.T) {
	result := EnrichTableTypeWithFuncTypes(nil, nil, &cfg.Graph{}, nil)
	if rec, ok := result.(*typ.Record); !ok || rec != nil {
		t.Error("expected nil record for nil record input")
	}
}

func TestCollectCapturedFieldAssignments_NilGraph(t *testing.T) {
	result := CollectCapturedFieldAssignments(nil, nil, nil)
	if result == nil {
		t.Error("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestCollectCapturedFieldAssignments_EmptyCapturedSyms(t *testing.T) {
	result := CollectCapturedFieldAssignments(&cfg.Graph{}, map[cfg.SymbolID]bool{}, nil)
	if result == nil {
		t.Error("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilInputs(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, 0, nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilSelfType(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, 1, nil)
	if result != nil {
		t.Error("expected nil for nil selfType")
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
	fields := map[string]typ.Type{"x": typ.String}
	result := mergeFieldsIntoSelfType(selfType, fields)
	if result != selfType {
		t.Errorf("expected original selfType for non-record/interface, got %v", result)
	}
}
