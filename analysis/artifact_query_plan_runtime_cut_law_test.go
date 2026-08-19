package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// An uncalled callable body is a reusable Program interior, not a Link-wide
// result root. It must remain outside the ordinary runtime demand until a call
// or explicit observation selects its formal boundary.
func TestArtifactQueryPlanDoesNotDemandUncalledCallableInterior(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local function dormant(value)
  local retained = value
  return retained
end
return 42`, contract)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || len(plan.state.artifacts.mounts) != 1 {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	defer plan.Close()
	if plan.state.committed.program != nil || len(plan.state.querySites) != 0 {
		t.Fatal("Compile instantiated the runtime query plane")
	}
	if diagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated {
		t.Fatalf("runtime topology = %+v", diagnostic)
	}
	mounted := plan.state.artifacts.mounts[0]
	callableBodies := make(map[identity.ContentID]struct{})
	rootBodies := make(map[identity.ContentID]struct{})
	for bodyIndex := 0; bodyIndex < mounted.snapshot.BodyCount(); bodyIndex++ {
		body, bodyOK := mounted.snapshot.BodyAt(bodyIndex)
		if !bodyOK {
			t.Fatal("body row")
		}
		if body.Callable() {
			callableBodies[body.ID()] = struct{}{}
		} else {
			rootBodies[body.ID()] = struct{}{}
		}
	}
	callablePoints := make(map[identity.ContentID]struct{})
	rootPoints := make(map[identity.ContentID]struct{})
	for occurrenceIndex := 0; occurrenceIndex < mounted.snapshot.OccurrenceCount(); occurrenceIndex++ {
		occurrence, occurrenceOK := mounted.snapshot.OccurrenceAt(occurrenceIndex)
		body, bodyOK := occurrence.BodyID()
		if !occurrenceOK || !bodyOK {
			continue
		}
		for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
			point, pointOK := occurrence.PointAt(pointIndex)
			if !pointOK {
				t.Fatal("occurrence point")
			}
			if _, callable := callableBodies[body]; callable {
				callablePoints[point] = struct{}{}
			}
			if _, root := rootBodies[body]; root {
				rootPoints[point] = struct{}{}
			}
		}
	}
	if len(callablePoints) == 0 || len(rootPoints) == 0 || len(plan.state.querySites) == 0 {
		t.Fatalf("fixture/query geometry = callable %d root %d queries %d", len(callablePoints), len(rootPoints), len(plan.state.querySites))
	}
	perPoint := make(map[identity.ContentID]int)
	for _, row := range plan.state.querySites {
		if _, forbidden := callablePoints[row.Point]; forbidden {
			t.Fatal("uncalled callable interior became an unconditional query root")
		}
		if _, root := rootPoints[row.Point]; !root {
			t.Fatal("query plan escaped the non-callable Program root")
		}
		perPoint[row.Point]++
	}
	for point, count := range perPoint {
		if count != 2 {
			t.Fatalf("root point %v query lanes = %d", point, count)
		}
	}
	if result, solveStatus := plan.Solve(context.Background()); solveStatus != AnalyzeComplete || result == nil {
		t.Fatalf("root-only solve = %v/%v", solveStatus, result)
	}
}

// A sealed control-fault chunk is still a Program root. Query planning must
// admit it so later diagnostic collection can name the fault instead of
// failing construction.
func TestArtifactQueryPlanAdmitsControlFaultChunk(t *testing.T) {
	for _, name := range []string{"functions/break-outside-loop", "functions/goto-backward"} {
		t.Run(name, func(t *testing.T) {
			plan, status := Compile(fixtureLink(t, name))
			if status != CompileComplete || plan == nil || plan.state == nil || len(plan.state.artifacts.mounts) == 0 {
				t.Fatalf("compile = %v/%v", status, plan)
			}
			defer plan.Close()
			sealed, sealedOK := linkArtifactRows(plan.state.artifacts.mounts)
			sites, queryOK := composite.SelectedQuerySites(sealed)
			queryOK = sealedOK && queryOK
			if !queryOK || len(sites) == 0 {
				t.Fatalf("control-fault chunk has no query plan: ok=%t rows=%d", queryOK, len(sites))
			}
		})
	}
}

// A declared query with no folded row is proven absent on the sealed column.
// Detach must project that absence rather than fail construction.
func TestAnalyzeCompletesDeclaredProvenAbsence(t *testing.T) {
	for _, name := range []string{"core/control-for-loop", "core/query-zero-row"} {
		t.Run(name, func(t *testing.T) {
			result, status := Analyze(context.Background(), fixtureLink(t, name))
			if status != AnalyzeComplete || result == nil {
				t.Fatalf("Analyze %s = %v result=%t", name, status, result != nil)
			}
		})
	}
}
