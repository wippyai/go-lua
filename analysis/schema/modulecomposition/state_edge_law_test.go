package modulecomposition_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	modulecomposition "github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

type moduleReturnStateEdgeFixture struct {
	call       moduleCallTransitionFixture
	directory  executioncontext.Directory
	outcome    modulecomposition.InitOutcome
	outcomeRow programschema.Outcome
	point      programschema.OutcomePoint
}

func makeModuleReturnStateEdgeFixture(t *testing.T) moduleReturnStateEdgeFixture {
	t.Helper()
	call := makeModuleCallTransitionFixture(t, "return")
	fromRoot, fromRootOK := executioncontext.NewRootContext(call.link, lawID(t, "module-return-caller-root"), call.from.ID())
	toRoot, toRootOK := executioncontext.NewRootContext(call.link, lawID(t, "module-return-target-root"), call.to.ID())
	if !fromRootOK || !toRootOK {
		t.Fatal("return contexts")
	}
	directory, directoryOK := executioncontext.Seal(call.link, []executioncontext.Context{call.from, call.to}, []executioncontext.RootContext{fromRoot, toRoot}, []executioncontext.Transition{call.transition})
	if !directoryOK {
		t.Fatal("return directory")
	}
	outcome, outcomeOK := modulecomposition.NewInitOutcomeFromModuleEntry(call.generation, call.program.targetMount, call.program.entry)
	if !outcomeOK {
		t.Fatal("return outcome")
	}
	count, published := call.program.targetMount.Program.OutcomeCount()
	if !published {
		t.Fatal("outcome publication")
	}
	outcomeIndex := -1
	for index := 0; index < count; index++ {
		candidate, held := call.program.targetMount.Program.OutcomeAt(index)
		if held && candidate.ID() == outcome.OutcomeID() {
			if outcomeIndex >= 0 {
				t.Fatal("duplicate return outcome")
			}
			outcomeIndex = index
		}
	}
	outcomeRow, outcomeRowOK := call.program.targetMount.Program.OutcomeAt(outcomeIndex)
	point, pointOK := call.program.targetMount.Program.OutcomePointFor(outcomeIndex, 0)
	if outcomeIndex < 0 || !outcomeRowOK || !pointOK || outcomeRow.ID() != outcome.OutcomeID() || point.OutcomeID() != outcome.OutcomeID() {
		t.Fatalf("return outcome point index=%d outcome=%t point=%t count=%d point-count=%d", outcomeIndex, outcomeRowOK, pointOK, count, outcomeRow.PointCount())
	}
	return moduleReturnStateEdgeFixture{call: call, directory: directory, outcome: outcome, outcomeRow: outcomeRow, point: point}
}

func TestModuleReturnStateEdgeRetainsExactReverseGeometry(t *testing.T) {
	fixture := makeModuleReturnStateEdgeFixture(t)
	row, ok := modulecomposition.NewModuleReturnStateEdge(fixture.call.row, fixture.call.generation, fixture.outcome, fixture.call.program.targetMount, fixture.point, fixture.directory)
	if !ok || !row.Available() {
		t.Fatal("construct return state edge")
	}
	returnEdge, returnOK := fixture.directory.ActivationEdge(fixture.call.row.ToContextID(), fixture.call.row.FromContextID())
	if !returnOK || row.LinkID() != fixture.call.link || row.CallTransitionID() != fixture.call.row.ID() ||
		row.GenerationID() != fixture.call.generation.ID() || row.OutcomeID() != fixture.outcome.OutcomeID() ||
		row.OutcomePointID() != fixture.point.PointID() || row.CallerReturnPointID() != fixture.call.row.ReturnPointID() ||
		row.ReturnTransitionID() != returnEdge.ID() || row.FromContextID() != fixture.call.row.ToContextID() ||
		row.ToContextID() != fixture.call.row.FromContextID() || row.ReturnModuleKey() != fixture.call.program.targetMount.ModuleKey ||
		row.CallerModuleKey() != fixture.call.program.mount.ModuleKey {
		t.Fatal("return edge lost canonical geometry")
	}
}

func TestModuleReturnStateEdgeRejectsNonReturnAndForeignPoint(t *testing.T) {
	fixture := makeModuleReturnStateEdgeFixture(t)
	nonReturn, nonReturnOK := modulecomposition.NewInitOutcome(fixture.call.generation, fixture.call.program.targetMount, fixture.call.program.normal)
	if !nonReturnOK {
		t.Fatal("normal outcome")
	}
	if _, ok := modulecomposition.NewModuleReturnStateEdge(fixture.call.row, fixture.call.generation, nonReturn, fixture.call.program.targetMount, fixture.point, fixture.directory); ok {
		t.Fatal("non-return outcome admitted")
	}
	foreignPoint, foreignPointOK := programschema.NewOutcomePoint(fixture.outcome.ID(), identity.ContentID{})
	if foreignPointOK || foreignPoint.Available() {
		t.Fatal("invalid foreign-point fixture")
	}
	foreignPoint, foreignPointOK = programschema.NewOutcomePoint(fixture.outcome.ID(), lawID(t, "foreign-return-point"))
	if !foreignPointOK {
		t.Fatal("foreign point")
	}
	if _, ok := modulecomposition.NewModuleReturnStateEdge(fixture.call.row, fixture.call.generation, fixture.outcome, fixture.call.program.targetMount, foreignPoint, fixture.directory); ok {
		t.Fatal("foreign outcome point admitted")
	}
}
