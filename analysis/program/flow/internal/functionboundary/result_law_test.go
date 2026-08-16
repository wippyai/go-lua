package functionboundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func testBoundaryID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[31] = seed, seed+1
	return id
}

func validBoundaryResultForLaw(t *testing.T) *Result {
	t.Helper()
	bodyOne := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bodyTwo := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	result := &Result{
		sourceID: testBoundaryID(1), flowID: testBoundaryID(2), staticID: testBoundaryID(3), moduleID: testBoundaryID(4), entry: bodyOne,
		bodies: []bodyRow{
			{},
			{body: bodyOne, entry: bodyOne, outcomes: range32{start: 0, end: 4}},
			{body: bodyTwo, entry: bodyTwo, outcomes: range32{start: 4, end: 8}, function: 1},
		},
		byBody:        []uint32{0, 0, 1},
		byOutcome:     []uint32{0, 0, 0, 0, 0, 1, 1, 1, 1},
		bodyByOutcome: []uint32{0, 1, 1, 1, 1, 2, 2, 2, 2},
		outcomeAt:     []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8},
		formals:       []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)},
		captures: []captureRow{{
			inner: keyspace.MakeTerm(keyspace.FamilyCell, 2),
			outer: keyspace.MakeTerm(keyspace.FamilyCell, 3),
		}},
		functions: []functionRow{{
			function: keyspace.MakeTerm(keyspace.FamilyFunction, 1), owner: bodyOne, body: bodyTwo, entry: bodyTwo,
			vararg: keyspace.MakeTerm(keyspace.FamilyCell, 4), formals: range32{start: 0, end: 1},
			captures: range32{start: 0, end: 1}, outcomes: range32{start: 4, end: 8},
		}},
		contexts:     make(map[identity.ContentID]uint32),
		bodyContexts: make(map[identity.ContentID]uint32),
	}
	for index := 0; index < 8; index++ {
		outcomeKind := [...]kind.OutcomeKind{kind.OutcomeNormal, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel}[index%4]
		result.outcomes = append(result.outcomes, outcomeRow{
			term: keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(index+1)),
			body: map[bool]keyspace.Term{true: bodyOne, false: bodyTwo}[index < 4], kind: outcomeKind,
		})
	}
	for index := 1; index < len(result.bodies); index++ {
		row := result.bodies[index]
		row.context = hashBodyContext(result, row)
		result.bodies[index] = row
		result.bodyContexts[row.context] = uint32(index)
	}
	row := result.functions[0]
	row.context = hashContext(result, row)
	result.functions[0] = row
	result.contexts[row.context] = 1
	if !result.validateResult() {
		t.Fatal("hand-built valid boundary result failed validation")
	}
	result.sealed = true
	return result
}

func recomputeBoundaryContextsForLaw(result *Result) {
	result.bodyContexts = make(map[identity.ContentID]uint32, len(result.bodies)-1)
	for index := 1; index < len(result.bodies); index++ {
		row := result.bodies[index]
		row.context = hashBodyContext(result, row)
		result.bodies[index] = row
		result.bodyContexts[row.context] = uint32(index)
	}
	result.contexts = make(map[identity.ContentID]uint32, len(result.functions))
	for index := range result.functions {
		row := result.functions[index]
		row.context = hashContext(result, row)
		result.functions[index] = row
		result.contexts[row.context] = uint32(index + 1)
	}
}

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

func TestFunctionBoundaryMatchesRequiresForeignQuartetAndSealedFence(t *testing.T) {
	result := validBoundaryResultForLaw(t)
	if !Matches(result, result.sourceID, result.flowID, result.staticID, result.moduleID) {
		t.Fatal("valid sealed result did not match exact quartet")
	}
	foreign := result.sourceID
	foreign[0]++
	if Matches(result, foreign, result.flowID, result.staticID, result.moduleID) {
		t.Fatal("foreign Source identity crossed FunctionBoundary fence")
	}
	result.sealed = false
	if Matches(result, result.sourceID, result.flowID, result.staticID, result.moduleID) || result.Count() != 0 {
		t.Fatal("unsealed FunctionBoundary result remained queryable")
	}
}
