package flow_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

const functionIdentityLawSource = `
local outer = seed()
local function factory(a, b, ...)
  local inner = a
  local function nested(c)
    return outer, inner, c
  end
  return nested(b), ...
end
return factory(1, 2, 3)
`

func lowerFunctionIdentityLaw(t *testing.T) *program.Program {
	t.Helper()
	published, err := lualower.Lower(lualower.Source{Name: "function-identity-law.lua", Text: []byte(functionIdentityLawSource)})
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func expectedFunctionIdentity(t *testing.T, domain string, write func(*framing.Writer) bool) identity.ContentID {
	t.Helper()
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		t.Fatalf("failed to frame expected %s identity", domain)
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func expectedFunctionCell(t *testing.T, bodyPath identity.ContentID, role, index uint64) identity.ContentID {
	return expectedFunctionIdentity(t, "program/transformer/cell-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Uint(role) == nil && writer.Uint(index) == nil
	})
}

func expectedStorageCell(t *testing.T, programID identity.ContentID, term keyspace.Term) identity.ContentID {
	return expectedFunctionIdentity(t, "program/transformer/storage-cell", func(writer *framing.Writer) bool {
		return writer.Bytes(programID[:]) == nil && writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
	})
}

func TestFunctionIdentityIssuanceIsOwnerFencedAndReplayStable(t *testing.T) {
	left, right := lowerFunctionIdentityLaw(t), lowerFunctionIdentityLaw(t)
	leftFlow, rightFlow := left.Flow(), right.Flow()
	leftBoundaries, rightBoundaries := leftFlow.FunctionBoundaries(), rightFlow.FunctionBoundaries()
	if leftBoundaries.Count() == 0 || leftBoundaries.Count() != rightBoundaries.Count() {
		t.Fatalf("function boundary counts = %d/%d", leftBoundaries.Count(), rightBoundaries.Count())
	}
	for index := 0; index < leftBoundaries.Count(); index++ {
		leftBoundary, leftOK := leftBoundaries.At(index)
		rightBoundary, rightOK := rightBoundaries.At(index)
		if !leftOK || !rightOK {
			t.Fatalf("boundary[%d] unavailable", index)
		}
		leftFunctionID, leftFunctionOK := left.FunctionID(leftBoundary)
		rightFunctionID, rightFunctionOK := right.FunctionID(rightBoundary)
		if !leftFunctionOK || !rightFunctionOK || leftFunctionID != rightFunctionID {
			t.Fatalf("FunctionID[%d] replay mismatch: %v/%v %v/%v", index, leftFunctionID, leftFunctionOK, rightFunctionID, rightFunctionOK)
		}
		context := leftBoundary.ContextID()
		programID := left.ContentID()
		wantFunctionID := expectedFunctionIdentity(t, "program/transformer/function", func(writer *framing.Writer) bool {
			return writer.Bytes(programID[:]) == nil && writer.Bytes(context[:]) == nil
		})
		if leftFunctionID != wantFunctionID {
			t.Fatalf("FunctionID[%d] changed preimage: got %v want %v", index, leftFunctionID, wantFunctionID)
		}
		bodyTerm, bodyOK := leftBoundary.Body()
		bodyPath, bodyPathOK := leftFlow.BodyPath(bodyTerm)
		if !bodyOK || !bodyPathOK || !bodyPath.Available() {
			t.Fatalf("boundary[%d] Body path unavailable", index)
		}
		for position := 0; position < leftBoundary.FormalCount(); position++ {
			leftID, leftCell, leftFormalOK := leftFlow.FunctionFormalIDs(leftBoundary, position)
			rightID, rightCell, rightFormalOK := rightFlow.FunctionFormalIDs(rightBoundary, position)
			if !leftFormalOK || !rightFormalOK || leftID != rightID || leftCell != rightCell {
				t.Fatalf("formal[%d,%d] replay mismatch", index, position)
			}
			wantCell := expectedFunctionCell(t, bodyPath, 1, uint64(position))
			wantID := expectedFunctionIdentity(t, "program/transformer/formal", func(writer *framing.Writer) bool {
				return writer.Bytes(bodyPath[:]) == nil && writer.Uint(uint64(position)) == nil && writer.Bytes(wantCell[:]) == nil
			})
			if leftCell != wantCell || leftID != wantID {
				t.Fatalf("formal[%d,%d] changed preimage: id=%v/%v cell=%v/%v", index, position, leftID, wantID, leftCell, wantCell)
			}
		}
		if _, hasVararg := leftBoundary.Vararg(); hasVararg {
			leftID, leftCell, leftVarargOK := leftFlow.FunctionVarargIDs(leftBoundary)
			rightID, rightCell, rightVarargOK := rightFlow.FunctionVarargIDs(rightBoundary)
			if !leftVarargOK || !rightVarargOK || leftID != rightID || leftCell != rightCell {
				t.Fatalf("vararg[%d] replay mismatch", index)
			}
			wantCell := expectedFunctionCell(t, bodyPath, 2, 0)
			wantID := expectedFunctionIdentity(t, "program/transformer/vararg", func(writer *framing.Writer) bool {
				return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(wantCell[:]) == nil
			})
			if leftCell != wantCell || leftID != wantID {
				t.Fatalf("vararg[%d] changed preimage", index)
			}
		}
		for position := 0; position < leftBoundary.CaptureCount(); position++ {
			leftInner, leftOuter, leftInnerBody, leftOuterBody, leftCellsOK := leftFlow.FunctionCaptureCells(leftBoundary, position)
			rightInner, rightOuter, rightInnerBody, rightOuterBody, rightCellsOK := rightFlow.FunctionCaptureCells(rightBoundary, position)
			if !leftCellsOK || !rightCellsOK || leftInner != rightInner || leftOuter != rightOuter || leftInnerBody != rightInnerBody || leftOuterBody != rightOuterBody {
				t.Fatalf("capture cells[%d,%d] replay mismatch", index, position)
			}
			wantInner := expectedFunctionCell(t, leftInnerBody, 3, uint64(position))
			wantOuter := expectedFunctionCell(t, leftOuterBody, 4, uint64(position))
			if leftInner != wantInner || leftOuter != wantOuter {
				t.Fatalf("capture cells[%d,%d] changed preimage", index, position)
			}
			leftCapture, leftInnerID, leftOuterID, leftInnerStorage, leftOuterStorage, leftBody, leftOuterPath, leftCaptureOK := left.FunctionCaptureIDs(leftBoundary, position)
			rightCapture, rightInnerID, rightOuterID, rightInnerStorage, rightOuterStorage, rightBody, rightOuterPath, rightCaptureOK := right.FunctionCaptureIDs(rightBoundary, position)
			if !leftCaptureOK || !rightCaptureOK || leftCapture != rightCapture || leftInnerID != rightInnerID || leftOuterID != rightOuterID ||
				leftInnerStorage != rightInnerStorage || leftOuterStorage != rightOuterStorage || leftBody != rightBody || leftOuterPath != rightOuterPath {
				t.Fatalf("capture[%d,%d] replay mismatch", index, position)
			}
			pair, pairOK := leftBoundary.CaptureAt(position)
			if !pairOK {
				t.Fatalf("capture[%d,%d] source pair unavailable", index, position)
			}
			wantInnerStorage := expectedStorageCell(t, programID, pair.Inner)
			wantOuterStorage := expectedStorageCell(t, programID, pair.Outer)
			wantCapture := expectedFunctionIdentity(t, "program/transformer/capture", func(writer *framing.Writer) bool {
				return writer.Bytes(leftInnerBody[:]) == nil && writer.Bytes(leftOuterBody[:]) == nil && writer.Uint(uint64(position)) == nil &&
					writer.Bytes(leftInner[:]) == nil && writer.Bytes(leftOuter[:]) == nil && writer.Bytes(wantInnerStorage[:]) == nil && writer.Bytes(wantOuterStorage[:]) == nil
			})
			if leftCapture != wantCapture || leftInnerID != leftInner || leftOuterID != leftOuter || leftInnerStorage != wantInnerStorage || leftOuterStorage != wantOuterStorage || leftBody != leftInnerBody || leftOuterPath != leftOuterBody {
				t.Fatalf("capture[%d,%d] changed preimage", index, position)
			}
		}
	}
	if _, ok := left.FunctionID(functionboundary.Boundary{}); ok {
		t.Fatal("FunctionID accepted an unissued boundary")
	}
	if _, _, ok := leftFlow.FunctionFormalIDs(functionboundary.Boundary{}, 0); ok {
		t.Fatal("FunctionFormalIDs accepted an unissued boundary")
	}
}
