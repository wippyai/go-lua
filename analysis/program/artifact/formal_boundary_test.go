package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/cold"
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program schema unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile function boundary artifact: %s", failure.Error())
	}
	return artifact
}

func TestProgramArtifactOwnsCanonicalFunctionBoundaryPorts(t *testing.T) {
	left, right := compileFunctionBoundaryArtifact(t), compileFunctionBoundaryArtifact(t)
	if left.ID() != right.ID() || left.FunctionBoundaryCount() != right.FunctionBoundaryCount() || left.FunctionBoundaryCount() < 2 {
		t.Fatalf("replayed function interfaces = id %v/%v count %d/%d", left.ID(), right.ID(), left.FunctionBoundaryCount(), right.FunctionBoundaryCount())
	}
	leftProgram, rightProgram := summaryProgram(t, left), summaryProgram(t, right)
	bodyCount, bodiesPublished := leftProgram.BodyCount()
	replayedBodyCount, replayedBodiesPublished := rightProgram.BodyCount()
	if !bodiesPublished || !replayedBodiesPublished || bodyCount != replayedBodyCount {
		t.Fatal("cold Body families unavailable")
	}
	bodyRows := make(map[identity.ContentID]cold.Body, bodyCount)
	for index := 0; index < bodyCount; index++ {
		row, ok := leftProgram.BodyAt(index)
		if !ok {
			t.Fatalf("BodyAt(%d)", index)
		}
		bodyRows[row.ID()] = row
	}
	varargRows, captureRows := 0, 0
	for index := 0; index < left.FunctionBoundaryCount(); index++ {
		got, gotOK := left.FunctionBoundaryAt(index)
		replayed, replayedOK := right.FunctionBoundaryAt(index)
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
		for position := 0; position < got.FormalCount(); position++ {
			port, portOK := got.FormalAt(position)
			replayedPort, replayedPortOK := replayed.FormalAt(position)
			gotPosition, positionOK := port.Position()
			declared, declaredOK := port.DeclaredStaticTypeID()
			replayedDeclared, replayedDeclaredOK := replayedPort.DeclaredStaticTypeID()
			if !portOK || !replayedPortOK || !positionOK || gotPosition != position ||
				port.ID() != replayedPort.ID() || port.CellID() != replayedPort.CellID() || port.StorageCellID() != replayedPort.StorageCellID() ||
				declaredOK != replayedDeclaredOK || declared != replayedDeclared || declaredOK {
				t.Fatalf("function boundary[%d] formal[%d] lost order", index, position)
			}
		}
		leftVararg, leftVarargOK := got.Vararg()
		rightVararg, rightVarargOK := replayed.Vararg()
		if leftVarargOK != rightVarargOK || leftVarargOK && (leftVararg.ID() != rightVararg.ID() || leftVararg.CellID() != rightVararg.CellID()) {
			t.Fatalf("function boundary[%d] vararg replay mismatch", index)
		}
		if leftVarargOK {
			varargRows++
		}
		for position := 0; position < got.CaptureCount(); position++ {
			capture, captureOK := got.CaptureAt(position)
			replayedCapture, replayedCaptureOK := replayed.CaptureAt(position)
			gotPosition, positionOK := capture.Position()
			if !captureOK || !replayedCaptureOK || !positionOK || gotPosition != position || capture.InnerBodyID() != got.BodyID() ||
				capture.ID() != replayedCapture.ID() || capture.InnerCellID() != replayedCapture.InnerCellID() ||
				capture.OuterCellID() != replayedCapture.OuterCellID() || capture.InnerBodyID() != replayedCapture.InnerBodyID() ||
				capture.OuterBodyID() != replayedCapture.OuterBodyID() {
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
	if _, ok := left.FunctionBoundaryAt(left.FunctionBoundaryCount()); ok {
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program schema unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile declared formal artifact: %s", failure.Error())
	}
	staticNodes := make(map[identity.ContentID]struct{}, artifact.StaticTypeNodeCount())
	for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
		node, nodeOK := artifact.StaticTypeNodeAt(index)
		if !nodeOK {
			t.Fatalf("StaticTypeNodeAt(%d)", index)
		}
		staticNodes[node.ID()] = struct{}{}
	}
	var boundary programartifact.FunctionBoundaryRow
	for index := 0; index < artifact.FunctionBoundaryCount(); index++ {
		candidate, candidateOK := artifact.FunctionBoundaryAt(index)
		if candidateOK && candidate.FormalCount() == 2 {
			if boundary.Available() {
				t.Fatal("fixture issued duplicate two-formal boundaries")
			}
			boundary = candidate
		}
	}
	left, leftOK := boundary.FormalAt(0)
	right, rightOK := boundary.FormalAt(1)
	leftType, leftTypeOK := left.DeclaredStaticTypeID()
	rightType, rightTypeOK := right.DeclaredStaticTypeID()
	_, leftOwned := staticNodes[leftType]
	_, rightOwned := staticNodes[rightType]
	if !boundary.Available() || !leftOK || !rightOK || !leftTypeOK || !rightTypeOK || !leftOwned || !rightOwned || leftType == rightType {
		t.Fatalf("declared formal types left=%s/%v/%v right=%s/%v/%v boundary=%+v", leftType, leftTypeOK, leftOwned, rightType, rightTypeOK, rightOwned, boundary)
	}
	program := summaryProgram(t, artifact)
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	if !arithmeticPublished || arithmeticCount != 1 {
		t.Fatalf("declared formal arithmetic summaries=%d/%v, want 1/true", arithmeticCount, arithmeticPublished)
	}
	summary, summaryOK := program.ArithmeticSummaryAt(0)
	leftRepresentation, rightRepresentation, resultRepresentation, representationsOK := summary.Representations()
	if !summaryOK || !representationsOK || leftRepresentation != cold.NumericRepresentationInteger ||
		rightRepresentation != cold.NumericRepresentationNumber || resultRepresentation != cold.NumericRepresentationNumber {
		t.Fatalf("declared formal arithmetic=%+v/%v representations=%d/%d/%d/%v", summary, summaryOK,
			leftRepresentation, rightRepresentation, resultRepresentation, representationsOK)
	}
}
