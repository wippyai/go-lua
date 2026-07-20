package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPathStoreOwnsObjectCovariantAndDeferredPresencePolicies(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(79)
	container := symbol.ID(779)
	target := pathdom.NewPath(container, "container").Field("value")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 779, HasExpr: true}
	callSource := factflow.ValueSource{Kind: factflow.ValueSourceCall, CallPoint: 3, HasCallPoint: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, symbol.ID(780), pathdom.NewPath(symbol.ID(780), "result"), callSource),
		},
		PathAssignments: map[cfg.Point]factflow.PathAssignment{point: factflow.NewPathAssignment(target, source)},
		ObjectLiterals:  map[factflow.ExprRef]factflow.ObjectLiteral{source.ExprRef: factflow.NewObjectLiteral(nil).WithIdentity(testTableLiteralID(source.ExprRef))},
		CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
			point: {factflow.NewCovariantExposure(pathdom.NewPath(container, "container"), product.Top(), factflow.CovariantExposureRecord)},
		},
	})
	transaction, ok := PlanPathStoreTransaction(facts, point)
	if !ok || !transaction.HasObjectLiteralSidecar() || !transaction.HasCovariantProofPolicy() || !transaction.HasAssignmentGroupPresenceStep() || !transaction.Valid(reg) {
		t.Fatalf("path-store ownership = %#v/%t", transaction, ok)
	}
}
