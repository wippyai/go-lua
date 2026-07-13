package transformer

import (
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"testing"
)

func TestStructuralPredicatePurityClassificationIsExhaustive(t *testing.T) {
	for _, kind := range operationplan.Kinds() {
		if _, known := structuralPredicatePointKindPurity(kind); !known {
			t.Fatalf("operation kind %v is unclassified", kind)
		}
	}
	for _, kind := range []operationplan.Kind{operationplan.CallSite, operationplan.RootAssignment, operationplan.ObjectLiteral, operationplan.ChannelSelect} {
		if pure, _ := structuralPredicatePointKindPurity(kind); pure {
			t.Fatalf("executable kind %v classified pure", kind)
		}
	}
}
