package programschema_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

type identityOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
}

type identityOperations []identityOperation

func (operations *identityOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, identityOperation{kind: 'i', id: id})
	return true
}

func (operations *identityOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, identityOperation{kind: 'u', value: value})
	return true
}

func (operations *identityOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, identityOperation{kind: 'b', value: encoded})
	return true
}

func testIdentity(value byte) identity.ContentID { return identity.ContentID{value} }

func TestBodyIdentityFieldsCommitTheCanonicalCallableGeometry(t *testing.T) {
	bodyID, contextID, entryID := testIdentity(1), testIdentity(2), testIdentity(3)
	functionID, formalID := testIdentity(4), testIdentity(5)
	entryPointID, rootID := testIdentity(6), testIdentity(7)
	outcomeID, returnValuesID, outcomePointID := testIdentity(8), testIdentity(9), testIdentity(10)
	boundaryID := testIdentity(11)
	formalPortID, formalCellID, formalStorageID, declaredTypeID := testIdentity(12), testIdentity(13), testIdentity(14), testIdentity(15)
	varargID, varargCellID := testIdentity(16), testIdentity(17)
	captureID, innerCellID, outerCellID := testIdentity(18), testIdentity(19), testIdentity(20)
	innerStorageID, outerStorageID, outerBodyID := testIdentity(21), testIdentity(22), testIdentity(23)
	allocationID := testIdentity(24)

	body, bodyOK := programschema.NewBody(bodyID, contextID, entryID, functionID, formalID, 0, 1, 0, 1, 0, 1, true)
	entry, entryOK := programschema.NewBodyEntry(bodyID, entryPointID)
	root, rootOK := programschema.NewBodyRoot(bodyID, rootID, 1)
	outcome, outcomeOK := programschema.NewOutcome(outcomeID, bodyID, identity.ContentID{}, identity.ContentID{}, programschema.OutcomeReturn, 0, 1, 0, 1, false, false)
	returnValue, returnOK := programschema.NewOutcomeReturnValue(outcomeID, returnValuesID)
	outcomePoint, pointOK := programschema.NewOutcomePoint(outcomeID, outcomePointID)
	boundary, boundaryOK := programschema.NewFunctionBoundary(boundaryID, bodyID, contextID, entryID, formalID, 0, 1, 0, 1, 0, 1)
	formal, formalOK := programschema.NewFunctionFormal(formalPortID, formalCellID, formalStorageID, declaredTypeID, 0)
	vararg, varargOK := programschema.NewFunctionVararg(varargID, varargCellID)
	capture, captureOK := programschema.NewFunctionCapture(captureID, innerCellID, outerCellID, innerStorageID, outerStorageID, bodyID, outerBodyID, 0)
	target, targetOK := calltarget.NewTarget(allocationID, bodyID, contextID, functionID, formalID)
	if !bodyOK || !entryOK || !rootOK || !outcomeOK || !returnOK || !pointOK || !boundaryOK || !formalOK || !varargOK || !captureOK || !targetOK {
		t.Fatal("canonical callable geometry refused a complete row")
	}

	catalog, catalogOK := programcatalog.CatalogID(testIdentity(25))
	if !catalogOK {
		t.Fatal("catalog identity")
	}
	frozen, sealed := (programpublication.Publication{
		CallTargets:         []calltarget.Target{target},
		Bodies:              []programschema.Body{body},
		BodyEntries:         []programschema.BodyEntry{entry},
		BodyRoots:           []programschema.BodyRoot{root},
		Outcomes:            []programschema.Outcome{outcome},
		OutcomeReturnValues: []programschema.OutcomeReturnValue{returnValue},
		OutcomePoints:       []programschema.OutcomePoint{outcomePoint},
		FunctionBoundaries:  []programschema.FunctionBoundary{boundary},
		FunctionFormals:     []programschema.FunctionFormal{formal},
		FunctionVarargs:     []programschema.FunctionVararg{vararg},
		FunctionCaptures:    []programschema.FunctionCapture{capture},
	}).Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("seal canonical callable geometry")
	}

	var got identityOperations
	if !(programschema.Program{Frozen: frozen}).WriteBodyIdentityFields(&got) {
		t.Fatal("write body identity fields")
	}
	i := func(id identity.ContentID) identityOperation { return identityOperation{kind: 'i', id: id} }
	u := func(value uint64) identityOperation { return identityOperation{kind: 'u', value: value} }
	b := func(value bool) identityOperation {
		if value {
			return identityOperation{kind: 'b', value: 1}
		}
		return identityOperation{kind: 'b'}
	}
	want := identityOperations{
		u(1), i(bodyID), i(contextID), i(entryID), b(true), i(functionID), i(formalID), u(1), i(entryPointID), u(1), i(rootID), u(1), u(0), u(1),
		u(programschema.FunctionBoundaryLawVersion), u(1), i(boundaryID), i(bodyID), i(contextID), i(entryID), i(formalID), u(1),
		i(formalPortID), i(formalCellID), i(formalStorageID), i(declaredTypeID), u(0), b(true), i(varargID), i(varargCellID), u(1),
		i(captureID), i(innerCellID), i(outerCellID), i(innerStorageID), i(outerStorageID), i(bodyID), i(outerBodyID), u(0),
		u(1), i(allocationID), i(bodyID), i(contextID), i(functionID), i(formalID),
		u(1), i(outcomeID), i(bodyID), u(uint64(programschema.OutcomeReturn)), b(false), i(identity.ContentID{}), b(false), i(identity.ContentID{}), u(0), u(1), u(1), i(outcomePointID),
		u(1), i(returnValuesID),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity operations = %#v, want %#v", got, want)
	}
}
