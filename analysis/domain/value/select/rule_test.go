package selectrule

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestSelectRulesSplitSelectedLeftFromSelectedRight(t *testing.T) {
	schema, source := selectSchema(t)
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, selectKey(1), selectKey(900_001), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	left, ok := DeclareLeft(composition, selectKey(2), selectKey(3), selectKey(4), owner)
	if !ok || left == nil {
		t.Fatal("selected-left Rule")
	}
	right, ok := DeclareRight(composition, selectKey(5), selectKey(6), selectKey(7), owner)
	if !ok || right == nil {
		t.Fatal("selected-right Rule")
	}
	if evidence, admitted := left.check(engine.RuleDerivation[value.Value, value.SelectBranch]{}); admitted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged selected-left derivation produced evidence")
	}
	if evidence, admitted := right.check(engine.RuleDerivation[value.Value, value.SelectBranch]{}); admitted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged selected-right derivation produced evidence")
	}
	if !declareSelectQuery(composition, owner) || !composition.Seal() {
		t.Fatal("select composition seal")
	}
	leftCount, rightCount := 0, 0
	for index, raw := range selectTerms(source) {
		for branch := 0; branch < 2; branch++ {
			operand, ok := schema.SelectBranch(raw.shard, raw.term, branch)
			if !ok {
				t.Fatalf("SelectBranch(%d,%d)", index, branch)
			}
			_, _, _, _, chosenIsLeft, ok := operand.Endpoints()
			if !ok {
				t.Fatal("Select endpoints")
			}
			if chosenIsLeft {
				if instance, ok := left.Instance(operand); !ok || instance == nil {
					t.Fatalf("selected-left instance %d/%d", index, branch)
				}
				leftCount++
			} else {
				if instance, ok := right.Instance(operand); !ok || instance == nil {
					t.Fatalf("selected-right instance %d/%d", index, branch)
				}
				rightCount++
			}
		}
	}
	if leftCount == 0 || rightCount == 0 {
		t.Fatalf("selected branch split = left:%d right:%d", leftCount, rightCount)
	}
	andLeft, andRight, orLeft, orRight := selectTruthOperands(t, schema, source)
	if _, accepted := left.Instance(andRight); accepted {
		t.Fatal("selected-right branch replayed through selected-left Rule")
	}
	if _, accepted := right.Instance(andLeft); accepted {
		t.Fatal("selected-left branch replayed through selected-right Rule")
	}
	if _, accepted := left.Instance(orRight); accepted {
		t.Fatal("or selected-right branch replayed through selected-left Rule")
	}
	if _, accepted := right.Instance(orLeft); accepted {
		t.Fatal("or selected-left branch replayed through selected-right Rule")
	}
}

// TestSelectRuleEvaluatorLaws covers the domain-owned semantic evaluator
// used by both transfer and its local derivation checker.  Access and
// RuleDerivation are correctly opaque outside engine; this test therefore
// checks the exact Value judgment rather than fabricating an engine frame.
func TestSelectRuleEvaluatorLaws(t *testing.T) {
	schema, source := selectSchema(t)
	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, selectKey(100), selectKey(900_100), schema)
	left, leftOK := DeclareLeft(composition, selectKey(101), selectKey(102), selectKey(103), owner)
	right, rightOK := DeclareRight(composition, selectKey(104), selectKey(105), selectKey(106), owner)
	if !ownerOK || !leftOK || !rightOK || !declareSelectQuery(composition, owner) || !composition.Seal() {
		t.Fatal("select evaluator declaration")
	}
	andLeft, andRight, orLeft, orRight := selectTruthOperands(t, schema, source)
	falseFact := selectSourceFact(t, schema, source, runtimekind.Boolean, value.TruthFalse)
	trueFact := selectSourceFact(t, schema, source, runtimekind.Boolean, value.TruthTrue)
	mixed, mixedOK := schema.Join(falseFact, trueFact)
	if !mixedOK {
		t.Fatal("mixed truth")
	}

	assertResult := func(label string, rule *LeftRule, operand value.SelectBranch, input, want value.Value) {
		t.Helper()
		got, ok := rule.result(operand, input)
		if !ok || !schema.Equal(got, want) {
			t.Fatalf("%s = %x/%t, want %x", label, schema.Fingerprint(got), ok, schema.Fingerprint(want))
		}
	}
	assertEnabled := func(label string, rule *RightRule, operand value.SelectBranch, input value.Value, want bool) {
		t.Helper()
		if got := rule.enabled(operand, input); got != want {
			t.Fatalf("%s enabled=%t, want %t", label, got, want)
		}
	}

	assertResult("false and left", left, andLeft, falseFact, falseFact)
	assertEnabled("false and right", right, andRight, falseFact, false)
	assertResult("false or left", left, orLeft, falseFact, schema.Bottom())
	assertEnabled("false or right", right, orRight, falseFact, true)
	assertResult("mixed and left", left, andLeft, mixed, falseFact)
	assertEnabled("mixed and right", right, andRight, mixed, true)
	assertResult("mixed or left", left, orLeft, mixed, trueFact)
	assertEnabled("mixed or right", right, orRight, mixed, true)
	assertResult("bottom and left", left, andLeft, schema.Bottom(), schema.Bottom())
	assertResult("bottom or left", left, orLeft, schema.Bottom(), schema.Bottom())
	assertEnabled("bottom and right", right, andRight, schema.Bottom(), false)
	assertEnabled("bottom or right", right, orRight, schema.Bottom(), false)
	for _, test := range []struct {
		name    string
		rule    *LeftRule
		operand value.SelectBranch
		truthy  bool
	}{
		{name: "top and left", rule: left, operand: andLeft, truthy: false},
		{name: "top or left", rule: left, operand: orLeft, truthy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := test.rule.result(test.operand, schema.Top())
			if !ok || schema.Equal(got, schema.Bottom()) {
				t.Fatal("Top lost selected truth arm")
			}
			for _, atom := range mustAtoms(t, schema, got) {
				truth := atom.Truthiness()
				if (test.truthy && !truth.MayBeTrue()) || (!test.truthy && !truth.MayBeFalse()) {
					t.Fatal("Top retained wrong truth alternative")
				}
			}
		})
	}
}

// TestLeftRuleAssembledExecutionFiltersTheLuaFalseArm runs the authored
// `false and "right"` left branch through the production source and Select
// Rules. The result can reach the Query only after the runtime-issued
// derivations have been accepted. Reading the left coordinate at the target
// point also proves the Value carry remains intact beside the derived result.
func TestLeftRuleAssembledExecutionFiltersTheLuaFalseArm(t *testing.T) {
	schema, linked := selectSchemaText(t, "return false and 'right'")
	var operand value.SelectBranch
	for index, raw := range selectTerms(linked) {
		for branch := 0; branch < 2; branch++ {
			candidate, candidateOK := schema.SelectBranch(raw.shard, raw.term, branch)
			if !candidateOK {
				t.Fatalf("left execution SelectBranch(%d,%d)", index, branch)
			}
			_, _, _, truthy, chosenLeft, endpointsOK := candidate.Endpoints()
			if endpointsOK && chosenLeft && !truthy {
				operand = candidate
				break
			}
		}
		if operand != (value.SelectBranch{}) {
			break
		}
	}
	resultCoordinate, leftCoordinate, _, truthy, chosenLeft, endpointsOK := operand.Endpoints()
	if operand == (value.SelectBranch{}) || !endpointsOK || !chosenLeft || truthy || resultCoordinate == leftCoordinate {
		t.Fatal("left execution fixture did not issue a distinct false-and left branch")
	}

	var seed value.SourceSeed
	var falseFact value.Value
	for index := 0; index < linked.Boundary().Values().Count(); index++ {
		candidate, candidateOK := schema.SourceSeedAt(index)
		coordinate, fact, resultOK := candidate.Result()
		if candidateOK && resultOK && schema.RuntimeKinds(fact) == runtimekind.Bit(runtimekind.Boolean) && schema.Truthiness(fact) == value.TruthFalse && coordinate == leftCoordinate {
			seed, falseFact = candidate, fact
			break
		}
	}
	if seed == (value.SourceSeed{}) || schema.Equal(falseFact, schema.Bottom()) {
		t.Fatal("left execution fixture did not issue its exact false source")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, selectKey(1_000_001), selectKey(1_000_002), schema)
	seedRule, seedRuleOK := valuesource.Declare(composition, selectKey(1_000_003), selectKey(1_000_004), selectKey(1_000_005), owner)
	leftRule, leftRuleOK := DeclareLeft(composition, selectKey(1_000_006), selectKey(1_000_007), selectKey(1_000_008), owner)
	if !ownerOK || !seedRuleOK || !leftRuleOK || owner == nil || seedRule == nil || leftRule == nil {
		t.Fatal("left execution production Rule declaration")
	}

	var resultRead, carriedRead engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: selectKey(1_000_009),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				resultCells, resultCellsOK := engine.QueryValue(row, resultRead)
				carriedCells, carriedCellsOK := engine.QueryValue(row, carriedRead)
				if !resultCellsOK || !carriedCellsOK || resultCells.Count() != 1 || carriedCells.Count() != 1 {
					return false
				}
				actualResult, resultPresent, resultAvailable := resultCells.At(0)
				actualCarried, carriedPresent, carriedAvailable := carriedCells.At(0)
				return rows == 1 && resultAvailable && resultPresent && carriedAvailable && carriedPresent &&
					schema.Equal(actualResult, falseFact) && schema.Equal(actualCarried, falseFact) &&
					schema.RuntimeKinds(actualResult).Contains(runtimekind.Boolean) && schema.Truthiness(actualResult) == value.TruthFalse
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: selectKey(1_000_010), Freeze: func(result bool) bool { return result }, Clone: func(result bool) bool { return result }, Equal: func(left, right bool) bool { return left == right },
			Fingerprint: func(result bool) uint64 {
				if result {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var resultOK, carriedOK bool
		resultRead, resultOK = engine.QueryReadFrom(query, owner.ExactRead())
		carriedRead, carriedOK = engine.QueryReadFrom(query, owner.ExactRead())
		return resultOK && carriedOK
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("left execution query/seal")
	}
	resultRef, resultRefOK := owner.Locate(resultCoordinate)
	leftRef, leftRefOK := owner.Locate(leftCoordinate)
	seedInstance, seedInstanceOK := seedRule.Instance(seed)
	leftInstance, leftInstanceOK := leftRule.Instance(operand)
	if !resultRefOK || !leftRefOK || !seedInstanceOK || !leftInstanceOK || seedInstance == nil || leftInstance == nil {
		t.Fatal("left execution production instances")
	}

	execution := testlaw.RunOne(context.Background(), testlaw.OneFixture[value.Value, value.SourceSeed, value.Value, value.SelectBranch, bool]{
		Composition:           composition,
		Predecessor:           seedInstance,
		Target:                leftInstance,
		Query:                 query,
		PredecessorSite:       selectKey(1_000_011),
		PredecessorOccurrence: selectKey(1_000_012),
		TargetSite:            selectKey(1_000_013),
		TargetOccurrence:      selectKey(1_000_014),
		BoundarySemantic:      selectKey(1_000_015),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, resultRead, resultRef) && engine.InstanceQueryRead(binding, carriedRead, leftRef)
		},
	})
	if execution.Status != engine.SolveComplete || !execution.ValueAvailable || !execution.Value {
		t.Fatalf("left execution = status:%v observed:%v value:%v", execution.Status, execution.ValueAvailable, execution.Value)
	}
}

func selectTruthOperands(t testing.TB, schema *value.Schema, source *link.Link) (andLeft, andRight, orLeft, orRight value.SelectBranch) {
	t.Helper()
	for index, raw := range selectTerms(source) {
		for branch := 0; branch < 2; branch++ {
			operand, ok := schema.SelectBranch(raw.shard, raw.term, branch)
			if !ok {
				t.Fatalf("SelectBranch(%d,%d)", index, branch)
			}
			_, _, _, truthy, chosenLeft, ok := operand.Endpoints()
			if !ok {
				t.Fatal("select endpoints")
			}
			switch {
			case chosenLeft && !truthy:
				andLeft = operand
			case !chosenLeft && truthy:
				andRight = operand
			case chosenLeft && truthy:
				orLeft = operand
			case !chosenLeft && !truthy:
				orRight = operand
			}
		}
	}
	for _, operand := range []value.SelectBranch{andLeft, andRight, orLeft, orRight} {
		if _, _, _, _, _, ok := operand.Endpoints(); !ok {
			t.Fatal("incomplete and/or branch denominator")
		}
	}
	return andLeft, andRight, orLeft, orRight
}

type selectTerm struct {
	shard linkproject.Shard
	term  keyspace.Term
}

func selectTerms(source *link.Link) []selectTerm {
	var terms []selectTerm
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, _ := source.Project().Mounts().At(index)
		p, _ := source.Project().Mounts().Program(shard)
		selects := p.Flow().Authored().Operators().Selects()
		for at := 0; at < selects.Count(); at++ {
			term, _ := selects.At(at)
			if p.Flow().Executable().Contains(term) {
				terms = append(terms, selectTerm{shard: shard, term: term})
			}
		}
	}
	return terms
}

func selectSourceFact(t testing.TB, schema *value.Schema, source *link.Link, kind runtimekind.Kind, truth value.Truth) value.Value {
	t.Helper()
	values := source.Boundary().Values()
	for index := 0; index < values.Count(); index++ {
		raw, ok := values.At(index)
		if !ok {
			continue
		}
		fact, ok := schema.SourceValue(raw)
		if ok && schema.RuntimeKinds(fact).Contains(kind) && schema.Truthiness(fact) == truth {
			return fact
		}
	}
	t.Fatalf("source value kind=%d truth=%d", kind, truth)
	return value.Value{}
}

func mustAtoms(t testing.TB, schema *value.Schema, fact value.Value) []value.Atom {
	t.Helper()
	atoms, ok := schema.Atoms(fact)
	if !ok {
		t.Fatal("Value atoms")
	}
	return atoms
}

func selectSchema(t testing.TB) (*value.Schema, *link.Link) {
	return selectSchemaText(t, "local left = false\nlocal right = 'right'\nlocal truth = true\nreturn left and right, left or right, truth\n")
}

func selectSchemaText(t testing.TB, text string) (*value.Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "select_value.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(source)
	schema, ok := value.Seal(source, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema")
	}
	return schema, source
}

func declareSelectQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: selectKey(90), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{Semantic: selectKey(91), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v }, Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
			if v {
				return 1
			}
			return 0
		}},
	}, func(query *engine.Query[bool]) bool {
		_, ok := engine.QueryReadFrom(query, owner.ExactRead())
		return ok
	})
	return ok && query != nil
}

func selectKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("select semantic key")
	}
	return key
}
