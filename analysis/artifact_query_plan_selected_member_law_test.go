package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// A direct call selects its callee. That callee's interior is a Snapshot.Query
// subject. An uncalled sibling stays off the query plan.
func TestDirectCalleeInteriorIsASelectedQuerySubject(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local function dormant(value)
  local retained = value
  return retained
end
local function use(x)
  return x
end
return use(1)`, contract)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	defer plan.Close()
	if _, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated {
		t.Fatal("runtime topology")
	}
	mounted := plan.state.artifacts.mounts[0]
	bodyPoints := callableOccurrencePoints(t, mounted.artifact)
	useBody, dormantBody := selectedDirectCalleeAndSibling(t, mounted.artifact)
	if !useBody.Available() || !dormantBody.Available() {
		t.Fatal("fixture lost the direct callee or its unused sibling")
	}
	if len(bodyPoints[useBody]) == 0 || len(bodyPoints[dormantBody]) == 0 {
		t.Fatal("callable bodies published no occurrence points")
	}
	selected := make(map[identity.ContentID]int)
	for _, row := range plan.state.queryPlan.rows {
		if _, inside := bodyPoints[useBody][row.point]; inside {
			selected[row.point]++
		}
		if _, forbidden := bodyPoints[dormantBody][row.point]; forbidden {
			t.Fatal("uncalled sibling became a query root")
		}
	}
	if len(selected) == 0 {
		t.Fatal("direct callee interior is not a query subject")
	}
	for _, count := range selected {
		if count != 2 {
			t.Fatalf("selected callee point query lanes = %d", count)
		}
	}
	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || result == nil {
		t.Fatalf("selected-callee solve = %v/%v", solveStatus, result)
	}
	solve := solveThroughReceipts(t, linked)
	queryPlan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.queryFamily)
	if !opens {
		t.Fatal("solve published no query column")
	}
	addressed := false
	for _, summary := range solve.summaries {
		if _, inside := bodyPoints[useBody][summary.point.point]; !inside {
			continue
		}
		if _, read := snapshot.Query(&solve.published, queryPlan, summary.subject); read == snapshot.ReadMiss {
			t.Fatalf("selected callee subject %x is a miss", summary.subject[:4])
		}
		addressed = true
	}
	if !addressed {
		t.Fatal("solve published no query subject inside the direct callee")
	}
}

func callableOccurrencePoints(t *testing.T, artifact *programartifact.Artifact) map[identity.ContentID]map[identity.ContentID]struct{} {
	t.Helper()
	bodyPoints := make(map[identity.ContentID]map[identity.ContentID]struct{})
	for bodyIndex := 0; bodyIndex < artifact.BodyCount(); bodyIndex++ {
		body, bodyOK := artifact.BodyAt(bodyIndex)
		if !bodyOK || !body.Callable() {
			continue
		}
		bodyPoints[body.ID()] = make(map[identity.ContentID]struct{})
	}
	for occurrenceIndex := 0; occurrenceIndex < artifact.OccurrenceCount(); occurrenceIndex++ {
		occurrence, occurrenceOK := artifact.OccurrenceAt(occurrenceIndex)
		owner, ownerOK := occurrence.BodyID()
		if !occurrenceOK || !ownerOK {
			continue
		}
		points, callable := bodyPoints[owner]
		if !callable {
			continue
		}
		for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
			point, pointOK := occurrence.PointAt(pointIndex)
			if !pointOK {
				t.Fatal("occurrence point")
			}
			points[point] = struct{}{}
		}
	}
	return bodyPoints
}

func selectedDirectCalleeAndSibling(t *testing.T, artifact *programartifact.Artifact) (identity.ContentID, identity.ContentID) {
	t.Helper()
	rootBodies := make(map[identity.ContentID]struct{})
	callable := make(map[identity.ContentID]struct{})
	for bodyIndex := 0; bodyIndex < artifact.BodyCount(); bodyIndex++ {
		body, bodyOK := artifact.BodyAt(bodyIndex)
		if !bodyOK {
			t.Fatal("body row")
		}
		if body.Callable() {
			callable[body.ID()] = struct{}{}
			continue
		}
		rootBodies[body.ID()] = struct{}{}
	}
	var callee identity.ContentID
	for callIndex := 0; callIndex < artifact.CallCount(); callIndex++ {
		call, callOK := artifact.CallAt(callIndex)
		target, targetOK := call.DirectTargetBody()
		if !callOK || !targetOK {
			continue
		}
		if _, root := rootBodies[call.BodyID()]; !root {
			continue
		}
		if _, known := callable[target]; !known {
			t.Fatal("direct target is not a callable body")
		}
		callee = target
		break
	}
	var sibling identity.ContentID
	for body := range callable {
		if body != callee {
			sibling = body
			break
		}
	}
	return callee, sibling
}
