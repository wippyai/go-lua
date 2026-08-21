package causal

import (
	"bytes"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSitesEquivalentReplayAndPermutationPreserveContext(t *testing.T) {
	left := openCausalFixture(t, wideCallSpec(8))
	right := openCausalFixture(t, wideCallSpec(8))
	wantTerms := make(map[keyspace.Term]struct{})
	for refIndex := range left.result.index.refs {
		route, ok := left.result.successorForRef(&left.result.index.refs[refIndex])
		if !ok {
			t.Fatal("existing route disappeared while checking endpoint denominator")
		}
		wantTerms[route.From] = struct{}{}
		wantTerms[route.To] = struct{}{}
	}
	if got, want := left.result.SiteCount(), len(wantTerms); got != want {
		t.Fatalf("site denominator = %d, want distinct route endpoints %d", got, want)
	}
	if got, want := right.result.SiteCount(), left.result.SiteCount(); got != want {
		t.Fatalf("replay endpoint denominator = %d, want %d", got, want)
	}
	callOne, callOneOK := left.result.SiteForTerm(keyspace.MakeTerm(keyspace.FamilyCall, 1))
	callTwo, callTwoOK := left.result.SiteForTerm(keyspace.MakeTerm(keyspace.FamilyCall, 2))
	if !callOneOK || !callTwoOK || callOne.Equal(callTwo) {
		t.Fatal("distinct nested Call endpoints collapsed into one Site")
	}
	for index := 0; index < left.result.SiteCount(); index++ {
		leftSite, leftOK := left.result.SiteAt(index)
		rightSite, rightOK := right.result.SiteAt(index)
		if !leftOK || !rightOK {
			t.Fatalf("replay SiteAt(%d) = %v/%v", index, leftOK, rightOK)
		}
		leftTerm, leftTermOK := leftSite.Term()
		rightTerm, rightTermOK := rightSite.Term()
		if !leftSite.Equal(rightSite) || leftSite.ContextID() != rightSite.ContextID() ||
			!leftTermOK || !rightTermOK || leftTerm != rightTerm {
			t.Fatalf("replay site %d changed identity: left=%x right=%x", index,
				leftSite.ContextID(), rightSite.ContextID())
		}
		if _, ok := left.result.ResolveContextID(leftSite.ContextID()); !ok {
			t.Fatalf("local ContextID(%d) did not resolve", index)
		}
	}
}

func TestSitesRetainNestedEndpointTermsAndOutcomeLookup(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	fixture := openCausalFixture(t, selectLeftCallSpec(kind.SelectOr))
	callSite, callOK := fixture.result.SiteForTerm(call)
	selectSite, selectOK := fixture.result.SiteForTerm(selectTerm)
	if !callOK || !selectOK || callSite.Equal(selectSite) {
		t.Fatal("nested Call and Select endpoints collapsed into one Site")
	}
	callTerm, callTermOK := callSite.Term()
	selectTermGot, selectTermOK := selectSite.Term()
	if !callTermOK || !selectTermOK || callTerm != call || selectTermGot != selectTerm {
		t.Fatalf("nested endpoint Terms = %v/%v and %v/%v", callTerm, callTermOK, selectTermGot, selectTermOK)
	}
	normal, normalOK := fixture.outcomes.BodyExit(body, kind.OutcomeNormal)
	if normalOK {
		exit, exitOK := fixture.result.SiteForTerm(normal)
		term, termOK := exit.Term()
		if exitOK && (!termOK || term != normal) {
			t.Fatalf("BodyExit site = %v/%v, want Outcome %v", term, termOK, normal)
		}
	}
}

func TestSitesIncludeEveryBodyTerminalOutcome(t *testing.T) {
	parent := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returnedBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	fallthroughBody := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	condition := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	fixture := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 3},
			causalFamilyCount{keyspace.FamilyBranch, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{branch}, {returned}, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: returnedBody}}},
			Control: authored.ControlInput{
				Branches: []authored.Branch{{Owner: parent, Condition: condition, WhenTrue: returnedBody, WhenFalse: fallthroughBody}},
				Returns:  []authored.Return{{Owner: returnedBody, Values: values}},
			},
		},
	})

	terminalKinds := map[kind.OutcomeKind]bool{
		kind.OutcomeNormal: true,
		kind.OutcomeReturn: true,
		kind.OutcomeThrow:  true,
		kind.OutcomeYield:  true,
		kind.OutcomeCancel: true,
	}
	for index := 0; index < fixture.outcomes.Count(); index++ {
		term, ok := fixture.outcomes.At(index)
		if !ok {
			t.Fatalf("Outcomes.At(%d) failed", index)
		}
		_, outcomeKind, target, ok := fixture.outcomes.Get(term)
		if !ok || target != 0 || !terminalKinds[outcomeKind] {
			continue
		}
		if site, ok := fixture.result.SiteForTerm(term); !ok || !site.Available() {
			t.Fatalf("body terminal outcome %v (%v) did not resolve as a Causal site", term, outcomeKind)
		}
	}
	parentReturn, ok := fixture.outcomes.Find(parent, kind.OutcomeReturn, 0)
	if !ok || fixture.result.Successors().Count(parentReturn) != 0 {
		t.Fatalf("terminal parent Return outcome successors = %d, want 0", fixture.result.Successors().Count(parentReturn))
	}
}

func TestSitesRejectForeignContextAndDuplicateLookup(t *testing.T) {
	local := openCausalFixture(t, wideCallSpec(2))
	foreignSpec := wideCallSpec(2)
	foreignSpec.name = "causal-foreign.lua"
	foreign := openCausalFixture(t, foreignSpec)
	localSite, ok := local.result.SiteAt(0)
	if !ok {
		t.Fatal("local SiteAt(0) failed")
	}
	if _, ok := foreign.result.ResolveContextID(localSite.ContextID()); ok {
		t.Fatal("foreign Causal resolved local contextual site")
	}
	if (Site{}).Available() {
		t.Fatal("zero Site is available")
	}
	foreignSite, ok := foreign.result.SiteAt(0)
	if !ok || localSite.Equal(foreignSite) {
		t.Fatal("foreign Site crossed exact owner fence")
	}

	// A hostile duplicate lookup must fail closed instead of selecting an
	// arbitrary physical row. This mutates only a test-owned copy of the
	// immutable index to exercise the collision guard in the query contract.
	foreign.result.sites.lookups = append(foreign.result.sites.lookups,
		siteLookup{context: foreignSite.ContextID(), index: 0})
	sort.Slice(foreign.result.sites.lookups, func(left, right int) bool {
		return bytes.Compare(foreign.result.sites.lookups[left].context[:], foreign.result.sites.lookups[right].context[:]) < 0
	})
	if _, ok := foreign.result.ResolveContextID(foreignSite.ContextID()); ok {
		t.Fatal("duplicate contextual lookup resolved ambiguously")
	}
}

func TestSitesHotQueriesAreAllocationFree(t *testing.T) {
	fixture := openCausalFixture(t, wideCallSpec(16))
	site, ok := fixture.result.SiteAt(0)
	if !ok {
		t.Fatal("SiteAt(0) failed")
	}
	id := site.ContextID()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = site.Available()
		_ = site.ContextID()
		term, termOK := site.Term()
		if termOK {
			_, _ = fixture.result.SiteForTerm(term)
		}
		_, _ = fixture.result.ResolveContextID(id)
	})
	if allocs != 0 {
		t.Fatalf("hot Site queries allocate %v times", allocs)
	}
}

func TestSitesForTermQueriesRetainNestedEndpointRows(t *testing.T) {
	fixture := openCausalFixture(t, wideCallSpec(16))
	for index := 0; index < fixture.result.SiteCount(); index++ {
		site, ok := fixture.result.SiteAt(index)
		if !ok {
			t.Fatalf("SiteAt(%d) failed", index)
		}
		term, termOK := site.Term()
		if !termOK {
			t.Fatalf("SiteAt(%d) has no endpoint Term", index)
		}
		resolved, resolvedOK := fixture.result.SiteForTerm(term)
		if !resolvedOK || !resolved.Equal(site) {
			t.Fatalf("ForTerm(%v) did not round-trip SiteAt(%d)", term, index)
		}
	}
}
