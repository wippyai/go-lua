package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContainmentTracksTypedParentsFieldsAndDirectReturns(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyTypePrimitive: 1,
		keyspace.FamilyTypeOptional:  1,
		keyspace.FamilyTypeField:     1,
		keyspace.FamilyTypeAsserts:   1,
	}
	check := newContainment(counts, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	optional := keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	opaqueOwner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if !check.attach(opaqueOwner, primitive) || !check.attach(primitive, optional) {
		t.Fatal("containment rejected a valid typed parent chain")
	}
	if check.attach(opaqueOwner, primitive) {
		t.Fatal("containment accepted a duplicate concrete child")
	}
	if !check.claimField(opaqueOwner, field) {
		t.Fatal("containment rejected the first Field owner")
	}
	if check.claimField(opaqueOwner, field) {
		t.Fatal("containment accepted duplicate Field ownership")
	}
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	if !check.markDirectReturn(opaqueOwner, assertion) || check.markDirectReturn(opaqueOwner, assertion) {
		t.Fatal("direct assertion-return evidence did not enforce one claim")
	}
	if check.parentOf(optional) != primitive || check.parentOf(field) != opaqueOwner {
		t.Fatal("containment parent lookup lost typed ownership")
	}
	if !check.valid() {
		t.Fatal("valid typed containment chain was rejected")
	}

	cycle := newContainment(counts, 1)
	if !cycle.attach(primitive, optional) || !cycle.attach(optional, primitive) {
		t.Fatal("cycle fixture could not be constructed")
	}
	if cycle.valid() {
		t.Fatal("containment accepted a concrete cycle")
	}
}
