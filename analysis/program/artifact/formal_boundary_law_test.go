package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
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
	receipt, receiptOK := grammar.Global()
	if !receiptOK {
		t.Fatal("Program schema unavailable")
	}
	artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
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
	pack, packOK := left.PackReceipt()
	if !packOK {
		t.Fatal("Pack refinement unavailable")
	}
	packBodies := make(map[identity.ContentID]programartifact.PackBodyReceiptRow, pack.BodyCount())
	for index := 0; index < pack.BodyCount(); index++ {
		row, ok := pack.BodyAt(index)
		if !ok {
			t.Fatalf("Pack BodyAt(%d)", index)
		}
		packBodies[row.ID()] = row
	}
	bodyRows := make(map[identity.ContentID]programartifact.BodyRow, left.BodyCount())
	for index := 0; index < left.BodyCount(); index++ {
		row, ok := left.BodyAt(index)
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
			got.CaptureCount() != replayed.CaptureCount() || got.OutcomeCount() != replayed.OutcomeCount() {
			t.Fatalf("function boundary replay[%d] diverged", index)
		}
		body, bodyOK := bodyRows[got.BodyID()]
		packBody, packBodyOK := packBodies[got.BodyID()]
		if !bodyOK || !packBodyOK || !body.Callable() || !packBody.Callable() || body.ContextID() != got.BodyContextID() || body.EntryID() != got.EntryID() {
			t.Fatalf("function boundary[%d] did not refine its exact Body/Pack rows", index)
		}
		if got.FormalCount() != packBody.FormalCount() {
			t.Fatalf("function boundary[%d] formal count %d, Pack %d", index, got.FormalCount(), packBody.FormalCount())
		}
		for position := 0; position < got.FormalCount(); position++ {
			port, portOK := got.FormalAt(position)
			replayedPort, replayedPortOK := replayed.FormalAt(position)
			packPort, packPortOK := packBody.FormalAt(position)
			gotPosition, positionOK := port.Position()
			declared, declaredOK := port.DeclaredStaticTypeID()
			replayedDeclared, replayedDeclaredOK := replayedPort.DeclaredStaticTypeID()
			if !portOK || !replayedPortOK || !packPortOK || !positionOK || gotPosition != position ||
				port.ID() != replayedPort.ID() || port.CellID() != replayedPort.CellID() || port.StorageCellID() != replayedPort.StorageCellID() ||
				port.ID() != packPort.FormalID() || port.CellID() != packPort.CellID() || port.StorageCellID() != packPort.StorageCellID() ||
				declaredOK != replayedDeclaredOK || declared != replayedDeclared || declaredOK {
				t.Fatalf("function boundary[%d] formal[%d] lost order or Pack refinement", index, position)
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
		if got.OutcomeCount() != body.OutcomeCount() {
			t.Fatalf("function boundary[%d] outcomes %d, body %d", index, got.OutcomeCount(), body.OutcomeCount())
		}
		bodyIndex := -1
		for candidate := 0; candidate < left.BodyCount(); candidate++ {
			row, _ := left.BodyAt(candidate)
			if row.ID() == body.ID() {
				bodyIndex = candidate
				break
			}
		}
		if bodyIndex < 0 {
			t.Fatalf("function boundary[%d] Body ordinal unavailable", index)
		}
		for position := 0; position < got.OutcomeCount(); position++ {
			outcomeID, outcomeOK := got.OutcomeAt(position)
			replayedID, replayedOK := replayed.OutcomeAt(position)
			outcome, bodyOutcomeOK := left.BodyOutcomeAt(bodyIndex, position)
			if !outcomeOK || !replayedOK || !bodyOutcomeOK || outcomeID != replayedID || outcomeID != outcome.ID() {
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
	receipt, receiptOK := grammar.Global()
	if !receiptOK {
		t.Fatal("Program schema unavailable")
	}
	artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
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
	if artifact.ArithmeticSummaryCount() != 1 {
		t.Fatalf("declared formal arithmetic summaries=%d, want 1", artifact.ArithmeticSummaryCount())
	}
	summary, summaryOK := artifact.ArithmeticSummaryAt(0)
	leftRepresentation, rightRepresentation, resultRepresentation, representationsOK := summary.Representations()
	if !summaryOK || !representationsOK || leftRepresentation != programartifact.NumericRepresentationInteger ||
		rightRepresentation != programartifact.NumericRepresentationNumber || resultRepresentation != programartifact.NumericRepresentationNumber {
		t.Fatalf("declared formal arithmetic=%+v/%v representations=%d/%d/%d/%v", summary, summaryOK,
			leftRepresentation, rightRepresentation, resultRepresentation, representationsOK)
	}
}
