package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

func compileStorageLifetimeLawProgram(t *testing.T, text string) (programschema.Program, lifecycle.View) {
	t.Helper()
	input, err := lower.Lower(lower.Source{Name: "storage-lifetime-law.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("execution schema")
	}
	artifact, failure := CompileDetailed(input, grammar, testIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile storage lifetime fixture: %s", failure.Error())
	}
	program := artifact.Program()
	state, stateOK := program.ColdState()
	if !stateOK {
		t.Fatal("cold Program state")
	}
	view, viewOK := lifecycle.NewView(state)
	if !viewOK {
		t.Fatal("lifecycle view")
	}
	return program, view
}

func TestStorageLifetimeCaptureUsesClosureClass(t *testing.T) {
	program, view := compileStorageLifetimeLawProgram(t, `
local captured = { value = 1 }
local function read()
  return captured.value
end
return read()
`)
	count, countOK := view.StorageCellLifetimeCount()
	if !countOK {
		t.Fatal("storage lifetime denominator")
	}
	// FunctionCapture carries the exact storage-cell identities. The producer
	// must classify every captured local as closure-retained. Merely belonging
	// to the entry body proves no module owner: a chunk local otherwise dies
	// with that activation just like a local in any nested function.
	boundaryCount, boundaryCountOK := program.FunctionBoundaryCount()
	if !boundaryCountOK {
		t.Fatal("function boundary denominator")
	}
	captures := 0
	for boundaryIndex := 0; boundaryIndex < boundaryCount; boundaryIndex++ {
		boundary, boundaryOK := program.FunctionBoundaryAt(boundaryIndex)
		if !boundaryOK || !boundary.Available() {
			t.Fatalf("function boundary %d unavailable", boundaryIndex)
		}
		captureOffset, captureCount, captureSpanOK := boundary.CaptureSpan()
		if !captureSpanOK || int(captureCount) != boundary.CaptureCount() {
			t.Fatalf("function boundary %d capture span unavailable", boundaryIndex)
		}
		for captureIndex := 0; captureIndex < int(captureCount); captureIndex++ {
			capture, captureOK := program.FunctionCaptureAt(int(captureOffset) + captureIndex)
			if !captureOK || !capture.Available() {
				t.Fatalf("capture %d unavailable", captureIndex)
			}
			captured := []struct {
				storageID identity.ContentID
				bodyID    identity.ContentID
			}{
				{capture.InnerStorageCellID(), capture.InnerBodyID()},
				{capture.OuterStorageCellID(), capture.OuterBodyID()},
			}
			for _, endpoint := range captured {
				row, rowOK := view.StorageCellLifetimeForID(endpoint.storageID)
				if !rowOK {
					t.Fatalf("captured storage lifetime unavailable for %s", endpoint.storageID)
				}
				want := lifecycle.StorageLifetimeClosure
				if row.Lifetime() != want {
					t.Fatalf("captured storage body=%s lifetime=%v, want %v", endpoint.bodyID, row.Lifetime(), want)
				}
			}
			captures++
		}
	}
	if captures == 0 {
		t.Fatal("capture fixture emitted no FunctionCapture rows")
	}
	closureRows := 0
	for index := 0; index < count; index++ {
		row, rowOK := view.StorageCellLifetimeAt(index)
		if !rowOK || !row.Available() {
			t.Fatalf("storage lifetime row %d unavailable", index)
		}
		if row.Lifetime() == lifecycle.StorageLifetimeClosure {
			closureRows++
		}
	}
	if closureRows == 0 {
		t.Fatal("captured non-entry storage emitted no closure rows")
	}
}

func TestStorageLifetimeKeepsEntryLocalsFrameAndGlobalsExplicit(t *testing.T) {
	_, view := compileStorageLifetimeLawProgram(t, `
local moduleState = {}
local function make()
  local scratch = { value = 1 }
  return scratch.value
end
local output = make()
return print, moduleState, output
`)
	count, countOK := view.StorageCellLifetimeCount()
	if !countOK {
		t.Fatal("storage lifetime denominator")
	}
	frames, modules, unknowns := 0, 0, 0
	for index := 0; index < count; index++ {
		row, rowOK := view.StorageCellLifetimeAt(index)
		if !rowOK || !row.Available() {
			t.Fatalf("storage lifetime row %d unavailable", index)
		}
		switch row.Lifetime() {
		case lifecycle.StorageLifetimeFrame:
			frames++
		case lifecycle.StorageLifetimeModule:
			modules++
		case lifecycle.StorageLifetimeUnknown:
			unknowns++
		default:
			t.Fatalf("storage lifetime row %d has invalid semantic class %v", index, row.Lifetime())
		}
	}
	if frames == 0 || unknowns == 0 || modules != 0 {
		t.Fatalf("storage lifetime classes frame=%d module=%d unknown=%d; entry locals must not fabricate module ownership", frames, modules, unknowns)
	}
}
