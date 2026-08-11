package unarynot

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
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestUnaryNotRuleOwnsExactCorrelatedTransform(t *testing.T) {
	schema, source := unaryNotSchema(t)
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, unaryKey(1), unaryKey(900_001), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	rule, ok := Declare(composition, unaryKey(2), unaryKey(3), unaryKey(4), owner)
	if !ok || rule == nil {
		t.Fatal("UnaryNot Rule")
	}
	if evidence, admitted := rule.check(engine.RuleDerivation[value.Value, value.UnaryNot]{}); admitted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged UnaryNot derivation produced evidence")
	}
	if !declareUnaryQuery(composition, owner) || !composition.Seal() {
		t.Fatal("UnaryNot composition seal")
	}
	instances := 0
	for index, raw := range unaryNotTerms(source) {
		operand, ok := schema.UnaryNot(raw.shard, raw.term)
		if !ok {
			t.Fatalf("Value UnaryNot(%d)", index)
		}
		if instance, ok := rule.Instance(operand); !ok || instance == nil {
			t.Fatalf("UnaryNot Rule instance %d", index)
		}
		instances++
	}
	if instances == 0 {
		t.Fatal("empty UnaryNot denominator")
	}
}

// The source and unary-not instances are both production Rules. testlaw.RunOne
// joins them only through the engine's true identity boundary, so this law
// exercises the actual one-input Rule path without an invented input fact.
func TestUnaryNotRuleLawHarnessCarriesOneRealSourceAcrossIdentityBoundary(t *testing.T) {
	schema, source := unaryNotSchemaText(t, "return not false")
	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, unaryKey(1_000_001), unaryKey(1_900_001), schema)
	seedRule, seedRuleOK := valuesource.Declare(composition, unaryKey(1_000_002), unaryKey(1_000_003), unaryKey(1_000_004), owner)
	targetRule, targetRuleOK := Declare(composition, unaryKey(1_000_005), unaryKey(1_000_006), unaryKey(1_000_007), owner)
	if !ownerOK || !seedRuleOK || !targetRuleOK || owner == nil || seedRule == nil || targetRule == nil {
		t.Fatal("unary law composition declaration")
	}

	var seed value.SourceSeed
	var target value.UnaryNot
	var input, output value.Coordinate
	for index, raw := range unaryNotTerms(source) {
		candidate, candidateOK := schema.UnaryNot(raw.shard, raw.term)
		if !candidateOK {
			t.Fatalf("unary law operand %d", index)
		}
		candidateOutput, candidateInput, endpointsOK := candidate.Endpoints()
		if !endpointsOK {
			t.Fatalf("unary law endpoints %d", index)
		}
		for sourceIndex := 0; sourceIndex < source.Boundary().Values().Count(); sourceIndex++ {
			candidateSeed, seedOK := schema.SourceSeedAt(sourceIndex)
			coordinate, fact, coordinateOK := candidateSeed.Result()
			if seedOK && coordinateOK && schema.RuntimeKinds(fact) == runtimekind.Bit(runtimekind.Boolean) && schema.Truthiness(fact) == value.TruthFalse && coordinate == candidateInput {
				seed, target, input, output = candidateSeed, candidate, candidateInput, candidateOutput
				break
			}
		}
		if target != (value.UnaryNot{}) {
			break
		}
	}
	if seed == (value.SourceSeed{}) || target == (value.UnaryNot{}) || input == (value.Coordinate{}) || output == (value.Coordinate{}) {
		t.Fatal("unary law did not find canonical false-source/unary relation")
	}

	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: unaryKey(1_000_008),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				if !cellsOK || cells.Count() != 1 {
					return false
				}
				actual, present, cellOK := cells.At(0)
				atoms, atomsOK := schema.Atoms(actual)
				return rows == 1 && cellOK && present && atomsOK && len(atoms) == 1 &&
					schema.Presence(actual) == value.PresencePresent && schema.RuntimeKinds(actual).Contains(runtimekind.Boolean) &&
					schema.Truthiness(actual) == value.TruthTrue
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: unaryKey(1_000_009),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("unary law query/seal")
	}
	outputRef, outputRefOK := owner.Locate(output)
	if !outputRefOK {
		t.Fatal("unary law output ref")
	}
	seedInstance, seedInstanceOK := seedRule.Instance(seed)
	targetInstance, targetInstanceOK := targetRule.Instance(target)
	if !seedInstanceOK || !targetInstanceOK || seedInstance == nil || targetInstance == nil {
		t.Fatal("unary law production instances")
	}

	result := testlaw.RunOne(context.Background(), testlaw.OneFixture[value.Value, value.SourceSeed, value.Value, value.UnaryNot, bool]{
		Composition:           composition,
		Predecessor:           seedInstance,
		Target:                targetInstance,
		Query:                 query,
		PredecessorSite:       unaryKey(1_000_010),
		PredecessorOccurrence: unaryKey(1_000_011),
		TargetSite:            unaryKey(1_000_012),
		TargetOccurrence:      unaryKey(1_000_013),
		BoundarySemantic:      unaryKey(1_000_014),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, outputRef)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("unary law execution = status:%v observed:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

type unaryNotTerm struct {
	shard linkproject.Shard
	term  keyspace.Term
}

func unaryNotTerms(source *link.Link) []unaryNotTerm {
	var terms []unaryNotTerm
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, _ := source.Project().Mounts().At(index)
		p, _ := source.Project().Mounts().Program(shard)
		unaries := p.Flow().Authored().Operators().Unaries()
		for at := 0; at < unaries.Count(); at++ {
			term, _ := unaries.At(at)
			_, op, _, ok := unaries.Get(term)
			if ok && p.Flow().Executable().Contains(term) && op == flowkind.UnaryNot {
				terms = append(terms, unaryNotTerm{shard: shard, term: term})
			}
		}
	}
	return terms
}

func unaryNotSchema(t testing.TB) (*value.Schema, *link.Link) {
	return unaryNotSchemaText(t, "local x = false\nreturn not x\n")
}

func unaryNotSchemaText(t testing.TB, text string) (*value.Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "unary_not_value.lua", Text: []byte(text)})
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

func declareUnaryQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: unaryKey(90), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{Semantic: unaryKey(91), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v }, Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
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

func unaryKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("unary semantic key")
	}
	return key
}
