package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRootAssignmentScalarComponentsMatchCanonicalStateTransaction(t *testing.T) {
	point := cfg.Point(9791)
	target := symbol.ID(9791)
	sourceSymbol := symbol.ID(9792)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(point), HasExpr: true}
	assignment := factflow.NewRootAssignment(
		factflow.RootAssignmentOrdinaryRootWrite,
		target,
		pathdom.NewPath(target, "component-target"),
		source,
	)
	authority, plan, _ := prepareRootAssignmentFactorFixture(t, point, target, assignment, typevalue.LiteralNumber(standard.Registry(), 1))
	domain := authority.domain
	keys, ok := plan.PathKeySpace()
	if !ok {
		t.Fatal("RootAssignment plan has no keyspace")
	}
	inventory, err := domain.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	program, err := plan.FactorProgram()
	if err != nil {
		t.Fatal(err)
	}
	components, err := program.RootAssignmentFactorComponents(RootAssignmentFactorComponentInventory{
		Coordinates: inventory,
		SourceValues: []statekey.Value{
			statekey.SymbolValue(sourceSymbol),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	transaction, ok := plan.ScalarFactorTransaction()
	if !ok {
		t.Fatal("RootAssignment plan has no scalar transaction")
	}
	pointEntry, current := domain.Lattice().Bottom(), domain.Lattice().Bottom()
	want, err := domain.ApplyRootAssignmentScalarTransfer(transaction, pointEntry, current)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, component := range components {
		if component.Kind() != RootAssignmentFactorComponentScalar {
			continue
		}
		seen++
		currentSelection, _ := component.CurrentInputs()
		pointSelection, _ := component.PointEntryInputs()
		outputSelection, _ := component.Outputs()
		currentFrame, projectErr := domain.ProjectProductFactorFrame(current, currentSelection)
		if projectErr != nil {
			t.Fatal(projectErr)
		}
		pointFrame, projectErr := domain.ProjectProductFactorFrame(pointEntry, pointSelection)
		if projectErr != nil {
			t.Fatal(projectErr)
		}
		outputBase, projectErr := domain.ProjectProductFactorFrame(current, outputSelection)
		if projectErr != nil {
			t.Fatal(projectErr)
		}
		output, applyErr := component.ApplyComponent(RootAssignmentFactorComponentInput{
			Current: currentFrame, PointEntry: pointFrame, OutputBase: outputBase,
		})
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		current, applyErr = domain.PatchProductFactorFrame(current, outputSelection, output)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
	}
	if seen == 0 {
		t.Fatal("RootAssignment component program omitted registered scalar hyperedges")
	}
	if !domain.Lattice().Equal(current, want) {
		t.Fatal("factor components differ from canonical RootAssignment scalar transaction")
	}
}

var _ = state.ProductFactorFrame{}
