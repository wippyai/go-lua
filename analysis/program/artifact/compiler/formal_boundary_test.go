package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/domain/composite"
)

const functionBoundaryArtifactSource = `
local outer = seed()
local function factory(a, b, ...)
  local inner = a
  local function nested(c)
    if c then
      return outer, inner, c
    end
    return nil
  end
  return nested(b), ...
end
return factory(1, 2, 3)
`

func compileFunctionBoundaryArtifact(t testing.TB) *programartifact.Artifact {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "function-boundary-artifact.lua", Text: []byte(functionBoundaryArtifactSource)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program schema unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile function boundary artifact: %s", failure.Error())
	}
	return artifact
}

func TestProgramArtifactOwnsCanonicalFunctionBoundaryPorts(t *testing.T) {
	left, right := compileFunctionBoundaryArtifact(t), compileFunctionBoundaryArtifact(t)
	leftProgram, rightProgram := summaryProgram(t, left), summaryProgram(t, right)
	leftBoundaryCount, leftBoundariesPublished := leftProgram.FunctionBoundaryCount()
	rightBoundaryCount, rightBoundariesPublished := rightProgram.FunctionBoundaryCount()
	if left.ID() != right.ID() || !leftBoundariesPublished || !rightBoundariesPublished || leftBoundaryCount != rightBoundaryCount || leftBoundaryCount < 2 {
		t.Fatalf("replayed function interfaces = id %v/%v count %d/%d", left.ID(), right.ID(), leftBoundaryCount, rightBoundaryCount)
	}
	bodyCount, bodiesPublished := leftProgram.BodyCount()
	replayedBodyCount, replayedBodiesPublished := rightProgram.BodyCount()
	if !bodiesPublished || !replayedBodiesPublished || bodyCount != replayedBodyCount {
		t.Fatal("cold Body families unavailable")
	}
	leftState, leftStateOK := leftProgram.ColdState()
	targets, targetsOK := calltarget.NewView(leftState)
	allocations, allocationsOK := heapallocation.NewView(leftState)
	if !leftStateOK || !targetsOK || !allocationsOK {
		t.Fatal("cold closure relation views unavailable")
	}
	bodyRows := make(map[identity.ContentID]programschema.Body, bodyCount)
	for index := 0; index < bodyCount; index++ {
		row, ok := leftProgram.BodyAt(index)
		if !ok {
			t.Fatalf("BodyAt(%d)", index)
		}
		bodyRows[row.ID()] = row
	}
	varargRows, captureRows := 0, 0
	for index := 0; index < leftBoundaryCount; index++ {
		got, gotOK := leftProgram.FunctionBoundaryAt(index)
		replayed, replayedOK := rightProgram.FunctionBoundaryAt(index)
		if !gotOK || !replayedOK || got.ID() != replayed.ID() || got.BodyID() != replayed.BodyID() ||
			got.BodyContextID() != replayed.BodyContextID() || got.EntryID() != replayed.EntryID() ||
			got.CallFormalID() != replayed.CallFormalID() || got.FormalCount() != replayed.FormalCount() ||
			got.CaptureCount() != replayed.CaptureCount() {
			t.Fatalf("function boundary replay[%d] diverged", index)
		}
		body, bodyOK := bodyRows[got.BodyID()]
		if !bodyOK || !body.Callable() || body.ContextID() != got.BodyContextID() || body.EntryID() != got.EntryID() {
			t.Fatalf("function boundary[%d] did not refine its exact Body row", index)
		}
		target, targetOK := targets.ForBody(got.BodyID())
		allocation, allocationOK := allocations.AllocationForID(target.AllocationID())
		inverseTarget, inverseTargetOK := targets.ForAllocation(allocation.ID())
		inverse, inverseOK := leftProgram.FunctionBoundaryForBody(inverseTarget.BodyID())
		if !targetOK || !allocationOK || !inverseTargetOK || !inverseOK || inverse.ID() != got.ID() || target.AllocationID() != allocation.ID() || allocation.Role() != heapallocation.RoleClosure || !allocation.RootSpan().Available() {
			t.Fatalf("function boundary[%d] did not join its closure allocation target=%v/%v allocation=%v/%v", index, target.AllocationID(), targetOK, allocation.ID(), allocationOK)
		}
		formalOffset, formalWidth, formalSpanOK := got.FormalSpan()
		replayedFormalOffset, replayedFormalWidth, replayedFormalSpanOK := replayed.FormalSpan()
		for position := 0; position < got.FormalCount(); position++ {
			port, portOK := leftProgram.FunctionFormalAt(int(formalOffset) + position)
			replayedPort, replayedPortOK := rightProgram.FunctionFormalAt(int(replayedFormalOffset) + position)
			gotPosition, positionOK := port.Position()
			declared, declaredOK := port.DeclaredStaticTypeID()
			replayedDeclared, replayedDeclaredOK := replayedPort.DeclaredStaticTypeID()
			if !formalSpanOK || !replayedFormalSpanOK || formalWidth != replayedFormalWidth || !portOK || !replayedPortOK || !positionOK || gotPosition != position ||
				port.ID() != replayedPort.ID() || port.CellID() != replayedPort.CellID() || port.StorageCellID() != replayedPort.StorageCellID() ||
				declaredOK != replayedDeclaredOK || declared != replayedDeclared || declaredOK {
				t.Fatalf("function boundary[%d] formal[%d] lost order", index, position)
			}
		}
		leftVarargOK, rightVarargOK := got.HasVararg(), replayed.HasVararg()
		var leftVararg, rightVararg programschema.FunctionVararg
		if leftVarargOK {
			offset, count, spanOK := got.VarargSpan()
			leftVararg, leftVarargOK = leftProgram.FunctionVarargAt(int(offset))
			leftVarargOK = leftVarargOK && count == 1 && spanOK
		}
		if rightVarargOK {
			offset, count, spanOK := replayed.VarargSpan()
			rightVararg, rightVarargOK = rightProgram.FunctionVarargAt(int(offset))
			rightVarargOK = rightVarargOK && count == 1 && spanOK
		}
		if got.HasVararg() != replayed.HasVararg() || leftVarargOK != rightVarargOK || leftVarargOK && (leftVararg.ID() != rightVararg.ID() || leftVararg.CellID() != rightVararg.CellID()) {
			t.Fatalf("function boundary[%d] vararg replay mismatch", index)
		}
		if leftVarargOK {
			varargRows++
		}
		captureOffset, captureWidth, captureSpanOK := got.CaptureSpan()
		replayedCaptureOffset, replayedCaptureWidth, replayedCaptureSpanOK := replayed.CaptureSpan()
		for position := 0; position < got.CaptureCount(); position++ {
			capture, captureOK := leftProgram.FunctionCaptureAt(int(captureOffset) + position)
			replayedCapture, replayedCaptureOK := rightProgram.FunctionCaptureAt(int(replayedCaptureOffset) + position)
			gotPosition, positionOK := capture.Position()
			if !captureSpanOK || !replayedCaptureSpanOK || captureWidth != replayedCaptureWidth || !captureOK || !replayedCaptureOK || !positionOK || gotPosition != position || capture.InnerBodyID() != got.BodyID() ||
				capture.ID() != replayedCapture.ID() || capture.InnerCellID() != replayedCapture.InnerCellID() ||
				capture.OuterCellID() != replayedCapture.OuterCellID() || capture.InnerBodyID() != replayedCapture.InnerBodyID() ||
				capture.OuterBodyID() != replayedCapture.OuterBodyID() ||
				capture.InnerStorageCellID() != replayedCapture.InnerStorageCellID() ||
				capture.OuterStorageCellID() != replayedCapture.OuterStorageCellID() ||
				!capture.InnerStorageCellID().Available() || !capture.OuterStorageCellID().Available() {
				t.Fatalf("function boundary[%d] capture[%d] lost direction/order", index, position)
			}
			if _, outerOK := bodyRows[capture.OuterBodyID()]; !outerOK {
				t.Fatalf("function boundary[%d] capture[%d] points outside the artifact Body plane", index, position)
			}
			captureRows++
		}
		bodyIndex := -1
		for candidate := 0; candidate < bodyCount; candidate++ {
			row, _ := leftProgram.BodyAt(candidate)
			if row.ID() == body.ID() {
				bodyIndex = candidate
				break
			}
		}
		if bodyIndex < 0 {
			t.Fatalf("function boundary[%d] Body ordinal unavailable", index)
		}
		for position := 0; position < body.OutcomeCount(); position++ {
			outcome, bodyOutcomeOK := leftProgram.BodyOutcomeFor(bodyIndex, position)
			replayedOutcome, replayedOutcomeOK := rightProgram.BodyOutcomeFor(bodyIndex, position)
			if !bodyOutcomeOK || !replayedOutcomeOK || outcome.ID() != replayedOutcome.ID() {
				t.Fatalf("function boundary[%d] outcome[%d] lost Body order", index, position)
			}
		}
	}
	if varargRows == 0 || captureRows == 0 {
		t.Fatalf("fixture did not exercise vararg/capture ports: %d/%d", varargRows, captureRows)
	}
	if _, ok := leftProgram.FunctionBoundaryAt(leftBoundaryCount); ok {
		t.Fatal("out-of-range function boundary was available")
	}
}

func TestProgramArtifactFunctionBoundaryRetainsExactDeclaredStaticTypes(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "function-boundary-declared-types.lua", Text: []byte(`
local function add(a: integer, b: number): number
  return a + b
end
return add
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program schema unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile declared formal artifact: %s", failure.Error())
	}
	program := summaryProgram(t, artifact)
	staticView := staticNodeTestView(t, program)
	staticCount, staticPublished := staticView.StaticTypeNodeCount()
	if !staticPublished {
		t.Fatal("StaticTypeNode family unavailable")
	}
	staticNodes := make(map[identity.ContentID]struct{}, staticCount)
	for index := 0; index < staticCount; index++ {
		node, nodeOK := staticView.StaticTypeNodeAt(index)
		if !nodeOK {
			t.Fatalf("StaticTypeNodeAt(%d)", index)
		}
		staticNodes[node.ID()] = struct{}{}
	}
	boundaryCount, boundaryPublished := program.FunctionBoundaryCount()
	var boundary programschema.FunctionBoundary
	for index := 0; boundaryPublished && index < boundaryCount; index++ {
		candidate, candidateOK := program.FunctionBoundaryAt(index)
		if candidateOK && candidate.FormalCount() == 2 {
			if boundary.Available() {
				t.Fatal("fixture issued duplicate two-formal boundaries")
			}
			boundary = candidate
		}
	}
	formalOffset, _, formalSpanOK := boundary.FormalSpan()
	left, leftOK := program.FunctionFormalAt(int(formalOffset))
	right, rightOK := program.FunctionFormalAt(int(formalOffset) + 1)
	leftType, leftTypeOK := left.DeclaredStaticTypeID()
	rightType, rightTypeOK := right.DeclaredStaticTypeID()
	_, leftOwned := staticNodes[leftType]
	_, rightOwned := staticNodes[rightType]
	if !boundaryPublished || !formalSpanOK || !boundary.Available() || !leftOK || !rightOK || !leftTypeOK || !rightTypeOK || !leftOwned || !rightOwned || leftType == rightType {
		t.Fatalf("declared formal types left=%s/%v/%v right=%s/%v/%v boundary=%+v", leftType, leftTypeOK, leftOwned, rightType, rightTypeOK, rightOwned, boundary)
	}
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	if !arithmeticPublished || arithmeticCount != 1 {
		t.Fatalf("declared formal arithmetic summaries=%d/%v, want 1/true", arithmeticCount, arithmeticPublished)
	}
	summary, summaryOK := program.ArithmeticSummaryAt(0)
	leftRepresentation, rightRepresentation, resultRepresentation, representationsOK := summary.Representations()
	if !summaryOK || !representationsOK || leftRepresentation != programschema.NumericRepresentationInteger ||
		rightRepresentation != programschema.NumericRepresentationNumber || resultRepresentation != programschema.NumericRepresentationNumber {
		t.Fatalf("declared formal arithmetic=%+v/%v representations=%d/%d/%d/%v", summary, summaryOK,
			leftRepresentation, rightRepresentation, resultRepresentation, representationsOK)
	}
}
