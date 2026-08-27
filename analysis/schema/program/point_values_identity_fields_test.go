package programschema_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

type pointValuesIdentityOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
}

type pointValuesIdentityOperations []pointValuesIdentityOperation

func (operations *pointValuesIdentityOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, pointValuesIdentityOperation{kind: 'i', id: id})
	return true
}

func (operations *pointValuesIdentityOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, pointValuesIdentityOperation{kind: 'u', value: value})
	return true
}

func (operations *pointValuesIdentityOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, pointValuesIdentityOperation{kind: 'b', value: encoded})
	return true
}

func pointValuesID(value byte) identity.ContentID { return identity.ContentID{value} }

func TestPointIdentityFieldsPreserveVersionedDecisionOrder(t *testing.T) {
	pointID, firstDecision, secondDecision := pointValuesID(1), pointValuesID(2), pointValuesID(3)
	firstAtom, firstAtomOK := region.NewAtom(firstDecision)
	secondAtom, secondAtomOK := region.NewAtom(secondDecision)
	point, pointOK := programschema.NewPoint(pointID, pointID, true, 0, 2)
	first, firstOK := programschema.NewPointDecision(firstDecision, firstAtom)
	second, secondOK := programschema.NewPointDecision(secondDecision, secondAtom)
	if !pointOK || !firstAtomOK || !secondAtomOK || !firstOK || !secondOK {
		t.Fatal("point rows")
	}
	catalog, ok := programcatalog.CatalogID(pointValuesID(200))
	if !ok {
		t.Fatal("catalog")
	}
	frozen, ok := (programpublication.Publication{Points: []programschema.Point{point}, PointDecisions: []programschema.PointDecision{first, second}}).Seal(catalog, identity.StoreID(1))
	if !ok {
		t.Fatal("seal publication")
	}
	var got pointValuesIdentityOperations
	if !(programschema.Program{Frozen: frozen}).WritePointIdentityFields(&got) {
		t.Fatal("write point identity fields")
	}
	i := func(id identity.ContentID) pointValuesIdentityOperation {
		return pointValuesIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) pointValuesIdentityOperation {
		return pointValuesIdentityOperation{kind: 'u', value: value}
	}
	b := func(value bool) pointValuesIdentityOperation {
		if value {
			return pointValuesIdentityOperation{kind: 'b', value: 1}
		}
		return pointValuesIdentityOperation{kind: 'b'}
	}
	want := pointValuesIdentityOperations{
		u(programschema.PointGeometryLawVersion), u(programschema.PointAttachmentLawVersion), u(1),
		i(pointID), i(pointID), b(true), u(2), i(firstDecision), i(secondDecision),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("point identity operations = %#v, want %#v", got, want)
	}
}

func TestPointIdentityFieldsIgnorePhysicalSharedSpanAndReplayLogicalRows(t *testing.T) {
	firstID, secondID := pointValuesID(21), pointValuesID(22)
	first, firstOK := programschema.NewPoint(firstID, firstID, true, 0, 2)
	second, secondOK := programschema.NewPoint(secondID, firstID, false, 0, 2)
	firstDecisionID, secondDecisionID := pointValuesID(23), pointValuesID(24)
	firstAtom, firstAtomOK := region.NewAtom(firstDecisionID)
	secondAtom, secondAtomOK := region.NewAtom(secondDecisionID)
	firstDecision, firstDecisionOK := programschema.NewPointDecision(firstDecisionID, firstAtom)
	secondDecision, secondDecisionOK := programschema.NewPointDecision(secondDecisionID, secondAtom)
	if !firstOK || !secondOK || !firstAtomOK || !secondAtomOK || !firstDecisionOK || !secondDecisionOK {
		t.Fatal("shared-span point rows")
	}
	catalog, ok := programcatalog.CatalogID(pointValuesID(202))
	if !ok {
		t.Fatal("catalog")
	}
	frozen, ok := (programpublication.Publication{
		Points:         []programschema.Point{first, second},
		PointDecisions: []programschema.PointDecision{firstDecision, secondDecision},
	}).Seal(catalog, identity.StoreID(1))
	if !ok {
		t.Fatal("seal publication")
	}
	var got pointValuesIdentityOperations
	if !(programschema.Program{Frozen: frozen}).WritePointIdentityFields(&got) {
		t.Fatal("write point identity fields")
	}
	i := func(id identity.ContentID) pointValuesIdentityOperation {
		return pointValuesIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) pointValuesIdentityOperation {
		return pointValuesIdentityOperation{kind: 'u', value: value}
	}
	b := func(value bool) pointValuesIdentityOperation {
		if value {
			return pointValuesIdentityOperation{kind: 'b', value: 1}
		}
		return pointValuesIdentityOperation{kind: 'b'}
	}
	want := pointValuesIdentityOperations{
		u(programschema.PointGeometryLawVersion), u(programschema.PointAttachmentLawVersion), u(2),
		i(firstID), i(firstID), b(true), u(2), i(pointValuesID(23)), i(pointValuesID(24)),
		i(secondID), i(firstID), b(false), u(2), i(pointValuesID(23)), i(pointValuesID(24)),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("shared-span point identity operations = %#v, want %#v", got, want)
	}
}

func TestValuesIdentityFieldsPreserveMemberAndClosedTailOrder(t *testing.T) {
	valuesID, bodyID, firstMember, secondMember := pointValuesID(11), pointValuesID(12), pointValuesID(13), pointValuesID(14)
	values, valuesOK := programschema.NewValues(valuesID, bodyID, identity.ContentID{}, 0, 2, programschema.ValuesTail{})
	first, firstOK := programschema.NewValuesMember(firstMember)
	second, secondOK := programschema.NewValuesMember(secondMember)
	if !valuesOK || !firstOK || !secondOK {
		t.Fatal("values rows")
	}
	catalog, ok := programcatalog.CatalogID(pointValuesID(201))
	if !ok {
		t.Fatal("catalog")
	}
	frozen, ok := (programpublication.Publication{Values: []programschema.Values{values}, ValuesMembers: []programschema.ValuesMember{first, second}}).Seal(catalog, identity.StoreID(1))
	if !ok {
		t.Fatal("seal publication")
	}
	var got pointValuesIdentityOperations
	if !(programschema.Program{Frozen: frozen}).WriteValuesIdentityFields(&got) {
		t.Fatal("write values identity fields")
	}
	i := func(id identity.ContentID) pointValuesIdentityOperation {
		return pointValuesIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) pointValuesIdentityOperation {
		return pointValuesIdentityOperation{kind: 'u', value: value}
	}
	b := func(value bool) pointValuesIdentityOperation {
		if value {
			return pointValuesIdentityOperation{kind: 'b', value: 1}
		}
		return pointValuesIdentityOperation{kind: 'b'}
	}
	want := pointValuesIdentityOperations{u(1), i(valuesID), i(bodyID), u(2), i(firstMember), i(secondMember), b(false), u(0), i(identity.ContentID{})}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values identity operations = %#v, want %#v", got, want)
	}
}
