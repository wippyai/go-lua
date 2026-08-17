package functionboundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestValidateResultRejectsMalformedInverseAndOutcomeStorage(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Result)
	}{
		{"synchronized hostile outcome swap", func(result *Result) {
			// Swap two rows in the Function Body, then synchronize every
			// inverse plane and both context maps. Only canonical dense term
			// order remains invalid.
			result.outcomes[4], result.outcomes[5] = result.outcomes[5], result.outcomes[4]
			result.outcomeAt[5], result.outcomeAt[6] = result.outcomeAt[6], result.outcomeAt[5]
			result.bodyByOutcome[5], result.bodyByOutcome[6] = 2, 2
			result.byOutcome[5], result.byOutcome[6] = 1, 1
			recomputeBoundaryContextsForLaw(result)
		}},
		{"aligned outcome inverse", func(result *Result) {
			result.outcomeAt[1], result.outcomeAt[2] = result.outcomeAt[2], result.outcomeAt[1]
		}},
		{"body outcome inverse", func(result *Result) { result.bodyByOutcome[1] = 2 }},
		{"function outcome inverse", func(result *Result) { result.byOutcome[5] = 0 }},
		{"context inverse", func(result *Result) {
			for context := range result.contexts {
				result.contexts[context] = 9
				break
			}
		}},
		{"body context inverse", func(result *Result) {
			for context := range result.bodyContexts {
				result.bodyContexts[context] = 9
				break
			}
		}},
		{"formal context", func(result *Result) { result.formals[0] = keyspace.MakeTerm(keyspace.FamilyCell, 9) }},
		{"capture context", func(result *Result) { result.captures[0].outer = keyspace.MakeTerm(keyspace.FamilyCell, 9) }},
		{"vararg context", func(result *Result) { result.functions[0].vararg = keyspace.MakeTerm(keyspace.FamilyCell, 9) }},
		{"outcome context", func(result *Result) { result.outcomes[4].kind = kind.OutcomeReturn }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := validBoundaryResultForLaw(t)
			test.mutate(result)
			if result.validateResult() {
				t.Fatal("validateResult accepted malformed boundary storage")
			}
		})
	}
}
