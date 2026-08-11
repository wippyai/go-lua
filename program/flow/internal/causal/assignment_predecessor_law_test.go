package causal

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestAssignmentPredecessorProjectsExactCommitOrder(t *testing.T) {
	spec := assignmentPredecessorSpec("causal-assignment-predecessor.lua")
	fixture := openCausalFixture(t, spec)
	successors := fixture.result.Successors()
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	write1 := keyspace.MakeTerm(keyspace.FamilyWrite, 1)
	write2 := keyspace.MakeTerm(keyspace.FamilyWrite, 2)

	first, firstOK := successors.AssignmentPredecessor(write1)
	second, secondOK := successors.AssignmentPredecessor(write2)
	if !firstOK || !secondOK {
		t.Fatalf("AssignmentPredecessor availability = %v/%v", firstOK, secondOK)
	}
	if !first.IsLocal() || first.From != write2 || first.To != write1 {
		t.Fatalf("Write 1 commit predecessor = %#v, want %v -> %v", first, write2, write1)
	}
	if !second.IsLocal() || second.From != values || second.To != write2 {
		t.Fatalf("Write 2 commit predecessor = %#v, want %v -> %v", second, values, write2)
	}
	firstID, firstIDOK := first.Identity()
	secondID, secondIDOK := second.Identity()
	if !firstIDOK || !secondIDOK || !firstID.Available() || !secondID.Available() {
		t.Fatal("assignment predecessor returned an unauthenticated route identity")
	}
	resolved, resolvedOK := successors.Resolve(firstID)
	resolvedID, resolvedIDOK := resolved.Identity()
	if !resolvedOK || !resolvedIDOK || resolvedID != firstID || resolved.From != first.From || resolved.To != first.To {
		t.Fatal("assignment predecessor identity did not resolve to its existing route")
	}

	replay := openCausalFixture(t, spec)
	replayFirst, replayFirstOK := replay.result.Successors().AssignmentPredecessor(write1)
	replaySecond, replaySecondOK := replay.result.Successors().AssignmentPredecessor(write2)
	replayFirstID, replayFirstIDOK := replayFirst.Identity()
	replaySecondID, replaySecondIDOK := replaySecond.Identity()
	if !replayFirstOK || !replaySecondOK || !replayFirstIDOK || !replaySecondIDOK || firstID != replayFirstID || secondID != replaySecondID {
		t.Fatal("assignment predecessor route identity changed across equivalent replay")
	}

	if _, ok := successors.AssignmentPredecessor(0); ok {
		t.Fatal("zero Write resolved an assignment predecessor")
	}
	if _, ok := successors.AssignmentPredecessor(keyspace.MakeTerm(keyspace.FamilyWrite, 3)); ok {
		t.Fatal("foreign Write ordinal resolved an assignment predecessor")
	}
	if _, ok := successors.AssignmentPredecessor(values); ok {
		t.Fatal("foreign family term resolved an assignment predecessor")
	}
	if _, ok := (Successors{}).AssignmentPredecessor(write1); ok {
		t.Fatal("unavailable Causal Successors resolved an assignment predecessor")
	}

	foreignSpec := spec
	foreignSpec.name = "causal-assignment-predecessor-foreign.lua"
	foreign := openCausalFixture(t, foreignSpec)
	foreignFirst, foreignOK := foreign.result.Successors().AssignmentPredecessor(write1)
	foreignID, foreignIDOK := foreignFirst.Identity()
	if !foreignOK || !foreignIDOK || firstID == foreignID {
		t.Fatal("foreign Causal owner crossed assignment predecessor identity fence")
	}
	if _, ok := foreign.result.Successors().Resolve(firstID); ok {
		t.Fatal("foreign Causal owner resolved local assignment predecessor identity")
	}
}

func assignmentPredecessorSpec(name string) causalSpec {
	body := causalTerm(keyspace.FamilyBody, 1)
	values := causalTerm(keyspace.FamilyValues, 1)
	valuesValue := causalTerm(keyspace.FamilyNil, 1)
	lens1Base := causalTerm(keyspace.FamilyNil, 2)
	lens1Key := causalTerm(keyspace.FamilyNil, 3)
	lens2Base := causalTerm(keyspace.FamilyNil, 4)
	lens2Key := causalTerm(keyspace.FamilyNil, 5)
	assign := causalTerm(keyspace.FamilyAssign, 1)
	lens1 := causalTerm(keyspace.FamilyLensKey, 1)
	lens2 := causalTerm(keyspace.FamilyLensKey, 2)
	return causalSpec{
		name: name,
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 5},
			causalFamilyCount{keyspace.FamilyLensKey, 2},
			causalFamilyCount{keyspace.FamilyAssign, 1},
			causalFamilyCount{keyspace.FamilyWrite, 2},
		),
		rows:      [][]keyspace.Term{{assign}},
		nilOwners: []keyspace.Term{body, body, body, body, body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{valuesValue}},
			Access: authored.AccessInput{Dynamic: []authored.DynamicLens{
				{Owner: body, Base: lens1Base, Key: lens1Key},
				{Owner: body, Base: lens2Base, Key: lens2Key},
			}},
			Storage: authored.StorageInput{
				Assigns: []authored.Assign{{Owner: body, Values: values}},
				Writes:  []authored.Write{{Assign: assign, Target: lens1}, {Assign: assign, Target: lens2}},
			},
		},
	}
}
