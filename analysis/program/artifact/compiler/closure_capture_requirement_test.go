package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func closureRequirementID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func closureRequirementCompiler(t *testing.T, source string, want heapallocation.Role, wantCapture bool) (*compiler, identity.ContentID) {
	t.Helper()
	input, err := lower.Lower(lower.Source{Name: "closure-requirement.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	state := &compiler{input: input, pointIDsBySite: make(map[identity.ContentID][]identity.ContentID)}
	if failure := state.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("point fixture failed: %v", failure)
	}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("value fixture failed: %v", failure)
	}
	bundle, fault := allocation.Build(allocation.Input{Program: input, Values: state.publication.Values})
	if fault.Failed() || bundle == nil {
		t.Fatalf("allocation fixture fault=%#v bundle=%v", fault, bundle)
	}
	boundaryBundle, boundaryFault := bodyboundary.Build(bodyboundary.Input{
		Program: input, ProgramID: input.ContentID(), Values: state.publication.Values, PointIDsBySite: state.pointIDsBySite,
	})
	if boundaryFault.Failed() || boundaryBundle == nil {
		t.Fatalf("boundary fixture fault=%#v bundle=%v", boundaryFault, boundaryBundle)
	}
	var body programschema.Body
	var callableCaptureCounts []int
	for _, candidate := range boundaryBundle.Bodies() {
		if candidate.Callable() {
			boundary, ok := boundaryBundle.FunctionBoundaryForBody(candidate.ID())
			if ok {
				callableCaptureCounts = append(callableCaptureCounts, boundary.CaptureCount())
			}
		}
		if candidate.Callable() != (want == heapallocation.RoleClosure) {
			continue
		}
		if want == heapallocation.RoleClosure {
			boundary, ok := boundaryBundle.FunctionBoundaryForBody(candidate.ID())
			if !ok || (boundary.CaptureCount() != 0) != wantCapture {
				continue
			}
		}
		body = candidate
		break
	}
	if !body.Available() {
		t.Fatalf("fixture omitted callable body with capture=%t; callable capture counts=%v", wantCapture, callableCaptureCounts)
	}
	boundary, boundaryOK := boundaryBundle.FunctionBoundaryForBody(body.ID())
	functionID, formalID := closureRequirementID(15), closureRequirementID(16)
	if want == heapallocation.RoleClosure {
		if !boundaryOK || (boundary.CaptureCount() != 0) != wantCapture {
			t.Fatalf("fixture callable capture mismatch: got=%d want-positive=%t", boundary.CaptureCount(), wantCapture)
		}
		var functionOK, formalOK bool
		functionID, functionOK = body.FunctionContextID()
		formalID, formalOK = body.CallFormalID()
		if !functionOK || !formalOK || functionID != boundary.ID() {
			t.Fatalf("fixture callable identity mismatch")
		}
	}
	for index := 0; index < bundle.Count(); index++ {
		row, rowOK := bundle.RowAt(index)
		role, roleOK := row.Role()
		template, templateOK := row.Template()
		if rowOK && roleOK && templateOK && role == want {
			target, targetOK := calltarget.NewTarget(template, body.ID(), body.ContextID(), functionID, formalID)
			if !targetOK {
				t.Fatalf("fixture call target")
			}
			return &compiler{allocations: bundle, publication: programpublication.Publication{CallTargets: []calltarget.Target{target}}, bodyBoundary: boundaryBundle}, template
		}
	}
	t.Fatalf("fixture omitted allocation role %v", want)
	return nil, identity.ContentID{}
}

func closureRequirementOccurrence(id identity.ContentID) programschema.Occurrence {
	row, ok := programschema.NewOccurrence(programschema.OccurrenceAllocation, id, identity.ContentID{}, 0, 0, 0, 0, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	if !ok {
		panic("invalid closure requirement test occurrence")
	}
	return row
}

func TestClosureCaptureRequirementAdmitsOnlyPositiveCanonicalBoundary(t *testing.T) {
	compiler, allocationID := closureRequirementCompiler(t, "local captured = 1\nlocal function nested() return captured end\nreturn nested\n", heapallocation.RoleClosure, true)
	row := closureRequirementOccurrence(allocationID)
	if admitted, decided := compiler.closureCaptureAdmits(row); !admitted || !decided {
		t.Fatalf("positive closure boundary admitted=%t decided=%t", admitted, decided)
	}

	compiler, allocationID = closureRequirementCompiler(t, "return function() end\n", heapallocation.RoleClosure, false)
	row = closureRequirementOccurrence(allocationID)
	if admitted, decided := compiler.closureCaptureAdmits(row); admitted || !decided {
		t.Fatalf("capture-free closure admitted=%t decided=%t", admitted, decided)
	}

	compiler, tableID := closureRequirementCompiler(t, "return {}\n", heapallocation.RoleTable, false)
	if admitted, decided := compiler.closureCaptureAdmits(closureRequirementOccurrence(tableID)); admitted || !decided {
		t.Fatalf("table allocation admitted=%t decided=%t", admitted, decided)
	}
}
