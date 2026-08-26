package memberdefinition

import (
	"testing"

	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule/codegen"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	capture "github.com/wippyai/go-lua/domain/placement/capture"
)

func TestCaptureContributionNamesTheCanonicalReducer(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || len(contribution.Reducers) != 1 {
		t.Fatalf("contribution=%+v, want one available reducer", contribution)
	}
	reducer := contribution.Reducers[0]
	if reducer.Implementation.Name != "CaptureFold" || reducer.Implementation.PackagePath != "github.com/wippyai/go-lua/domain/placement/capture" {
		t.Fatalf("implementation=%+v, want capture.CaptureFold", reducer.Implementation)
	}
	carrierDefinition := memberdefinition.Definition{Carriers: contribution.Carriers}
	args, results, ok := carrierDefinition.ReducerSignature(
		reducer,
		memberdefinition.GoType{PackagePath: "github.com/wippyai/go-lua/analysis/schema/structure", Name: "ReductionOutcome"},
		codegen.SelectionCellType,
		codegen.SummaryVectorType,
	)
	// The routed input declares the coordinate it publishes at, so the derived
	// call hands the fold that key before the tag it was correlated by: the
	// parent cell, the route, its tag, and the routed cell.
	if !ok || len(args) != 4 || args[0].Type.Name != "Fact" || args[1].Type.Name != "Key" ||
		args[2].Type.Name != "uint64" || args[3].Type.Name != "Fact" ||
		len(results) != 2 || results[0].Name != "Fact" || results[1].Name != "ReductionOutcome" {
		t.Fatalf("derived signature args=%+v results=%+v ok=%t", args, results, ok)
	}
	var _ func(placementdomain.Fact, heapdomain.Key, uint64, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = capture.CaptureFold
}
