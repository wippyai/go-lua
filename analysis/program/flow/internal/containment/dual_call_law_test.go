package containment

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// TestProveRejectsCallWithoutDirectOrExpressionParent exercises the complete
// production boundary. Body deliberately accepts a Call absent from Source
// direct order because Calls may be nested expressions; containment must then
// reject the Call when no expression relation actually supplies its parent.
func TestProveRejectsCallWithoutDirectOrExpressionParent(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyInteger, 2),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyCall, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	callee := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	actual := keyspace.MakeTerm(keyspace.FamilyInteger, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)

	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{nil},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{actual},
			},
			Calls: []authored.Call{{Owner: body, Callee: callee, Actuals: values}},
		},
		static: static.Input{Contracts: static.ContractsInput{Call: []static.CallContract{{}}}},
		module: emptyModule(t),
	})
	if _, err := fixture.prove(); err == nil || !strings.Contains(err.Error(), "is missing a parent") {
		t.Fatalf("Prove orphan Call error = %v, want missing-parent rejection", err)
	}
}

// TestProveRejectsCallClaimedBySourceAndValues prevents the two legal Call
// modes from overlapping. A direct Source Call is Body-owned; the same Call
// cannot simultaneously be a Values child with a second structural parent.
func TestProveRejectsCallClaimedBySourceAndValues(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyInteger, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyCall, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	callee := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)

	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{call}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{call},
			},
			Calls: []authored.Call{{Owner: body, Callee: callee, Actuals: values}},
		},
		static: static.Input{Contracts: static.ContractsInput{Call: []static.CallContract{{}}}},
		module: emptyModule(t),
	})
	if _, err := fixture.prove(); err == nil || !strings.Contains(err.Error(), "conflicting containment parents") {
		t.Fatalf("Prove dual-owned Call error = %v, want conflicting-parent rejection", err)
	}
}
