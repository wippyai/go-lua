package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

// TestEveryDeclaredQueryOpensItsColdSlot states that the cold query pass is
// total over the sealed inventory: every declared family opens its own slot
// against the axis fragments its subjects produced.
//
// The law exists because the refusal is otherwise unattributable. A slot
// declaration that is refused poisons the schema builder, so the first family
// that cannot open its slot makes every later family report the same refusal
// and Build reports only that declareQueries said no. Replaying the pass here
// names the stage and the family, so a missing declaration is read off the
// failure instead of being attributed to whichever line touched the query
// table last.
func TestEveryDeclaredQueryOpensItsColdSlot(t *testing.T) {
	if _, built := Build(); built {
		return
	}
	state, failure := newCatalog()
	if state == nil || failure.Available() {
		t.Fatalf("the declaration table did not seal: %+v", failure)
	}
	roles := state.roles
	if !roles.Available() {
		t.Fatal("the sealed table resolved no roles")
	}
	builder := engine.NewSchema()
	axisFragments, _, axesOK := declareAxes(state, builder, roles)
	if !axesOK {
		t.Fatal("Build refuses at the axis pass, before any query is declared")
	}
	owners, ownersOK := axisFragments.coldPrincipals(state)
	if !ownersOK {
		t.Fatal("Build refuses at the cold principal pass, before any query is declared")
	}
	if _, _, rulesOK := declareRules(state, builder, roles, owners); !rulesOK {
		t.Fatal("Build refuses at the rule pass, before any query is declared")
	}
	subjects, subjectsOK := axisFragments.subjects(state, state.axes)
	if !subjectsOK {
		t.Fatal("the axis pass produced no subject view for the query pass")
	}
	for position, contributor := range state.queryContributors {
		registration := state.queries[position]
		if registration == nil {
			t.Fatalf("sealed query row %d is absent", position)
		}
		if !contributor.producerComplete() {
			t.Fatalf("query family %q carries no complete typed producer", registration.Key())
		}
		if _, opened := contributor.declare(builder, subjects); !opened {
			t.Fatalf("query family %q (population %v) does not open its cold slot; every family declared after it inherits the poisoned builder",
				registration.Key(), registration.PopulationKind())
		}
	}
	t.Fatal("every declared query opened its slot, so Build refuses after the query pass")
}
