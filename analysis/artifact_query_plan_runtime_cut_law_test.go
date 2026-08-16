package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
)

// An uncalled callable body is a reusable Program interior, not a Link-wide
// result root. It must remain outside the ordinary runtime demand until a call
// or explicit observation selects its formal boundary.
func TestArtifactQueryPlanDoesNotDemandUncalledCallableInterior(t *testing.T) {
	contract, err := profile.Contract()
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
	if plan.state.graph != nil || plan.state.queryPlan != nil {
		t.Fatal("Compile instantiated the runtime query plane")
	}
	if diagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated {
		t.Fatalf("runtime topology = %+v", diagnostic)
	}
	mounted := plan.state.artifacts.mounts[0]
	callableBodies := make(map[identity.ContentID]struct{})
	rootBodies := make(map[identity.ContentID]struct{})
	for bodyIndex := 0; bodyIndex < mounted.artifact.BodyCount(); bodyIndex++ {
		body, bodyOK := mounted.artifact.BodyAt(bodyIndex)
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
	for occurrenceIndex := 0; occurrenceIndex < mounted.artifact.OccurrenceCount(); occurrenceIndex++ {
		occurrence, occurrenceOK := mounted.artifact.OccurrenceAt(occurrenceIndex)
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
	if len(callablePoints) == 0 || len(rootPoints) == 0 || plan.state.queryPlan == nil || len(plan.state.queryPlan.rows) == 0 {
		t.Fatalf("fixture/query geometry = callable %d root %d queries %d", len(callablePoints), len(rootPoints), len(plan.state.queryPlan.rows))
	}
	perPoint := make(map[identity.ContentID]int)
	for _, row := range plan.state.queryPlan.rows {
		if _, forbidden := callablePoints[row.point]; forbidden {
			t.Fatal("uncalled callable interior became an unconditional query root")
		}
		if _, root := rootPoints[row.point]; !root {
			t.Fatal("query plan escaped the non-callable Program root")
		}
		perPoint[row.point]++
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
