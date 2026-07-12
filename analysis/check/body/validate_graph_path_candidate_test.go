package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func TestValidateGraphBoundaryPathReadCandidateCensus(t *testing.T) {
	prepared, _ := validateGraphSemanticProgramFixture(t)
	descendantPaths := 0
	prepared.facts.ForEachExpressionPath(func(_ factflow.ExprRef, path pathdom.Path) bool {
		if path.Version == 0 && len(path.Segments) != 0 {
			if _, ok := prepared.operationPlan.BoundaryParamIndex(path.Symbol); ok {
				descendantPaths++
			}
		}
		return true
	})
	dynamicTables := 0
	prepared.facts.ForEachDynamicIndexExpression(func(_ factflow.ExprRef, read factflow.DynamicIndexExpression) bool {
		path := read.TablePathRef()
		if path.Version == 0 {
			if _, ok := prepared.operationPlan.BoundaryParamIndex(path.Symbol); ok {
				dynamicTables++
			}
		}
		return true
	})
	if descendantPaths != 11 || dynamicTables != 1 {
		t.Fatalf("validate_graph source-owned boundary path candidates = descendants %d, dynamic tables %d; want 11/1", descendantPaths, dynamicTables)
	}
}
