package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
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
