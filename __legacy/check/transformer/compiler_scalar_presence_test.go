package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func TestExactScalarPredicatePresenceConsequences(t *testing.T) {
	tests := []struct {
		name            string
		predicate       exactScalarPredicate
		truthy          bool
		want            presence.Value
		wantConsequence bool
	}{
		{name: "equal nonnil", predicate: exactScalarPredicate{equal: true, literalPresent: true}, truthy: true, want: presence.Present(), wantConsequence: true},
		{name: "unequal nonnil", predicate: exactScalarPredicate{equal: true, literalPresent: true}, truthy: false},
		{name: "equal nil", predicate: exactScalarPredicate{equal: true}, truthy: true, want: presence.Absent(), wantConsequence: true},
		{name: "unequal nil", predicate: exactScalarPredicate{equal: true}, truthy: false, want: presence.Present(), wantConsequence: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := test.predicate.entailedPresence(test.truthy)
			if ok != test.wantConsequence || ok && !presence.Equal(got, test.want) {
				t.Fatalf("entailed presence = %s/%v, want %s/%v", got, ok, test.want, test.wantConsequence)
			}
		})
	}
}
