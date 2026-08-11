package source

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclarePublishesOneDerivedZeroReadValueWriter(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_rule.lua", "return nil, false, true, 1, 2.5, 'x', {}")
	factorSemantic := sourceKey(1)
	ruleSemantic := sourceKey(2)
	operandFamily := sourceKey(3)
	evidenceSemantic := sourceKey(4)
	composition, owner, rule := sourceComposition(t, schema, factorSemantic, sourceKey(5), ruleSemantic, operandFamily, evidenceSemantic)
	if owner.Schema() != schema || rule == nil {
		t.Fatal("source declaration lost its Value owner")
	}
	inventory, ok := composition.RuleAdmissionInventory()
	if !ok || inventory.ID != composition.ID() || len(inventory.Rules) != 1 || inventory.Rules[0] != (engine.RuleAdmissionRecord{
		Rule: ruleSemantic, Basis: engine.RuleAdmissionBasisDerivation, Identity: evidenceSemantic,
	}) {
		t.Fatal("source Rule did not publish its exact derivation admission")
	}
	report, ok := composition.SemanticReport()
	if !ok || report.ID != composition.ID() || len(report.Incidences) != 0 || len(report.Components) != 1 ||
		len(report.Components[0].Factors) != 1 || report.Components[0].Factors[0] != factorSemantic || len(report.Components[0].Successors) != 0 {
		t.Fatal("zero-read source Rule introduced a semantic predecessor")
	}
}

func TestDeclareKeysAreExplicitAndSemantic(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_keys.lua", "return 1")
	base := sourceCompositionID(t, schema, sourceKey(10), sourceKey(14), sourceKey(11), sourceKey(12), sourceKey(13))
	for _, scenario := range []struct {
		name                                     string
		factor, summary, rule, operand, evidence engine.SemanticKey
	}{
		{name: "factor", factor: sourceKey(20), summary: sourceKey(14), rule: sourceKey(11), operand: sourceKey(12), evidence: sourceKey(13)},
		{name: "summary", factor: sourceKey(10), summary: sourceKey(24), rule: sourceKey(11), operand: sourceKey(12), evidence: sourceKey(13)},
		{name: "rule", factor: sourceKey(10), summary: sourceKey(14), rule: sourceKey(21), operand: sourceKey(12), evidence: sourceKey(13)},
		{name: "operand", factor: sourceKey(10), summary: sourceKey(14), rule: sourceKey(11), operand: sourceKey(22), evidence: sourceKey(13)},
		{name: "evidence", factor: sourceKey(10), summary: sourceKey(14), rule: sourceKey(11), operand: sourceKey(12), evidence: sourceKey(23)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if got := sourceCompositionID(t, schema, scenario.factor, scenario.summary, scenario.rule, scenario.operand, scenario.evidence); got == base {
				t.Fatal("changed semantic identity did not change CompositionID")
			}
		})
	}

	for _, scenario := range []struct {
		name                    string
		rule, operand, evidence engine.SemanticKey
	}{
		{name: "missing-rule", operand: sourceKey(32), evidence: sourceKey(33)},
		{name: "missing-operand", rule: sourceKey(31), evidence: sourceKey(33)},
		{name: "missing-evidence", rule: sourceKey(31), operand: sourceKey(32)},
		{name: "rule-operand-collision", rule: sourceKey(31), operand: sourceKey(31), evidence: sourceKey(33)},
		{name: "rule-evidence-collision", rule: sourceKey(31), operand: sourceKey(32), evidence: sourceKey(31)},
		{name: "operand-evidence-collision", rule: sourceKey(31), operand: sourceKey(32), evidence: sourceKey(32)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			composition := engine.NewComposition()
			owner, ok := valueowner.Declare(composition, sourceKey(30), sourceKey(34), schema)
			if !ok {
				t.Fatal("Value owner")
			}
			if rule, ok := Declare(composition, scenario.rule, scenario.operand, scenario.evidence, owner); ok || rule != nil {
				t.Fatal("invalid source semantic keys declared a Rule")
			}
		})
	}
}

func TestSourceResultIsConstantMonotoneNonDefaultAndCannotMintFacts(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_result.lua", "return false, 1, 'x'")
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, sourceKey(40), sourceKey(49), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	seeds := sourceSeeds(t, schema)
	if len(seeds) < 3 {
		t.Fatalf("source seeds=%d, want at least 3", len(seeds))
	}
	for index, seed := range seeds {
		coordinate, first, firstOK := sourceResult(owner, seed)
		againCoordinate, again, againOK := sourceResult(owner, seed)
		if !firstOK || !againOK || coordinate != againCoordinate || !schema.Same(first, again) || schema.Equal(first, schema.Default()) {
			t.Fatalf("source %d was not a stable non-default result", index)
		}
		joined, joinedOK := schema.Join(schema.Default(), first)
		widened, widenedOK := schema.Widen(first, first)
		if !joinedOK || !widenedOK || !schema.Same(joined, first) || !schema.Same(widened, first) ||
			!schema.LessOrEq(schema.Default(), first) || !schema.LessOrEq(first, again) {
			t.Fatalf("source %d violated constant-transfer monotonicity", index)
		}
		if !sourceFactMatches(owner, seed, first) || sourceFactMatches(owner, seed, schema.Default()) || sourceFactMatches(owner, seed, schema.Top()) {
			t.Fatalf("source %d admitted a default or minted fact", index)
		}
	}
	_, other, _ := sourceResult(owner, seeds[1])
	if sourceFactMatches(owner, seeds[0], other) {
		t.Fatal("one source seed admitted another seed's fact")
	}
	checker := sourceChecker(owner, sourceKey(41))
	if evidence, admitted := checker(engine.RuleDerivation[value.Value, value.SourceSeed]{}); admitted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("zero derivation minted source evidence")
	}
}

// The source Rule is Value's only unconditional runtime observation inlet.
// This law deliberately checks the correlated projections together: a later
// Rule may consume presence, kind, truth, or identity, but none may be seeded
// independently of the one exact authored literal alternative.
func TestSourceResultPreservesLiteralPresenceKindTruthAndIdentity(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_literal_product.lua", "return nil, false, true, 1, 2.5, 'x'")
	_, owner, _ := sourceComposition(t, schema, sourceKey(44), sourceKey(48), sourceKey(45), sourceKey(46), sourceKey(47))

	type observation struct {
		presence value.Presence
		kind     runtimekind.Kind
		truth    value.Truth
	}
	want := map[observation]int{
		{value.PresenceAbsent, runtimekind.Nil, value.TruthFalse}:      1,
		{value.PresencePresent, runtimekind.Boolean, value.TruthFalse}: 1,
		{value.PresencePresent, runtimekind.Boolean, value.TruthTrue}:  1,
		{value.PresencePresent, runtimekind.Number, value.TruthTrue}:   2,
		{value.PresencePresent, runtimekind.String, value.TruthTrue}:   1,
	}
	seen := make(map[observation]int, len(want))
	ids := make(map[[32]byte]struct{}, 6)
	for _, seed := range sourceSeeds(t, schema) {
		coordinate, fact, resultOK := sourceResult(owner, seed)
		id, identityOK := seed.ID()
		actual := observation{schema.Presence(fact), runtimekind.Invalid, schema.Truthiness(fact)}
		kinds := schema.RuntimeKinds(fact)
		for kind := runtimekind.Nil; kind < runtimekind.Count; kind++ {
			if kinds == runtimekind.Bit(kind) {
				actual.kind = kind
				break
			}
		}
		if !resultOK || !identityOK || coordinate == (value.Coordinate{}) || want[actual] == 0 {
			t.Fatalf("source lost its exact correlated runtime observation: %+v", actual)
		}
		atoms, atomsOK := schema.Atoms(fact)
		if !atomsOK || len(atoms) != 1 || schema.RuntimeKinds(fact) != atoms[0].RuntimeKinds() || schema.Truthiness(fact) != atoms[0].Truthiness() {
			t.Fatalf("source %+v widened one authored alternative", actual)
		}
		if _, duplicate := ids[[32]byte(id)]; duplicate {
			t.Fatalf("source %+v shared a semantic identity", actual)
		}
		ids[[32]byte(id)] = struct{}{}
		seen[actual]++
	}
	for actual, count := range want {
		if seen[actual] != count {
			t.Fatalf("source observation %+v count=%d, want %d", actual, seen[actual], count)
		}
	}
}

// This is the first domain use of engine/testlaw. It deliberately runs the
// actual declared source Rule through Batch -> Assembly -> Solve -> Query;
// it does not stage or evaluate through sourceResult. The Query checks the
// canonical Link source family and the Value axes it semantically promises.
// It can observe that fact only after the runtime-issued derivation checker
// accepted the staged disposition.
func TestSourceRuleLawHarnessRequiresAcceptedRuntimeDerivation(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_rule_law_harness.lua", "return 1")
	seed := sourceSeeds(t, schema)[0]
	coordinate, _, coordinateOK := seed.Result()
	if !coordinateOK {
		t.Fatal("source law fixture did not issue one integer seed")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, sourceKey(1_000_001), sourceKey(1_000_007), schema)
	rule, ruleOK := Declare(composition, sourceKey(1_000_002), sourceKey(1_000_003), sourceKey(1_000_004), owner)
	if !ownerOK || !ruleOK || owner == nil || rule == nil {
		t.Fatal("source law composition declaration")
	}

	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: sourceKey(1_000_005),
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
					schema.Presence(actual) == value.PresencePresent && schema.RuntimeKinds(actual).Contains(runtimekind.Number) &&
					schema.Truthiness(actual) == value.TruthTrue
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: sourceKey(1_000_006),
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
		t.Fatal("source law query/seal")
	}
	ref, refOK := owner.Locate(coordinate)
	if !refOK {
		t.Fatal("source law result ref")
	}
	instance, instanceOK := rule.Instance(seed)
	if !instanceOK || instance == nil {
		t.Fatal("source law instance")
	}

	result := testlaw.Run(context.Background(), testlaw.RuleFixture[value.Value, value.SourceSeed, bool]{
		Composition:        composition,
		Instance:           instance,
		Query:              query,
		SiteSemantic:       sourceKey(1_000_007),
		OccurrenceSemantic: sourceKey(1_000_008),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, ref)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("source law execution = status:%v observed:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

func TestSourceResultFencesForeignSeed(t *testing.T) {
	leftSchema, _, _, _ := sourceSchema(t, "source_replay.lua", "return 1, 2.5, 'x'")
	rightSchema, _, _, _ := sourceSchema(t, "source_replay.lua", "return 1, 2.5, 'x'")
	leftComposition := engine.NewComposition()
	leftOwner, ok := valueowner.Declare(leftComposition, sourceKey(50), sourceKey(59), leftSchema)
	if !ok {
		t.Fatal("left Value owner")
	}
	rightComposition := engine.NewComposition()
	rightOwner, ok := valueowner.Declare(rightComposition, sourceKey(50), sourceKey(59), rightSchema)
	if !ok {
		t.Fatal("right Value owner")
	}
	leftSeed := sourceSeeds(t, leftSchema)[0]
	rightSeed := sourceSeeds(t, rightSchema)[0]
	if _, _, ok := sourceResult(leftOwner, rightSeed); ok {
		t.Fatal("foreign SourceSeed crossed the Value owner fence")
	}
	if _, _, ok := sourceResult(rightOwner, leftSeed); ok {
		t.Fatal("reverse foreign SourceSeed crossed the Value owner fence")
	}
}

func TestRuleInstanceDerivesIdentityPayloadAndTargetFromOneSeed(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_instance.lua", "return 1, 2")
	_, owner, rule := sourceComposition(t, schema, sourceKey(54), sourceKey(58), sourceKey(55), sourceKey(56), sourceKey(57))
	seeds := sourceSeeds(t, schema)
	if len(seeds) < 2 {
		t.Fatal("source instance denominator")
	}
	if instance, ok := rule.Instance(seeds[0]); !ok || instance == nil {
		t.Fatal("canonical SourceSeed did not produce one atomic Rule instance")
	}
	frozen, digest, frozenOK := sourceSeedContent(seeds[0])
	replayed, replayDigest, replayOK := sourceSeedContent(frozen)
	frozenID, frozenIDOK := frozen.ID()
	replayedID, replayedIDOK := replayed.ID()
	if !frozenOK || !replayOK || !frozenIDOK || !replayedIDOK || digest == [32]byte{} || digest != replayDigest || frozenID != replayedID {
		t.Fatal("SourceSeed OperandContent is not pure and idempotent")
	}

	foreignSchema, _, _, _ := sourceSchema(t, "source_instance_foreign.lua", "return 1, 2")
	foreign := sourceSeeds(t, foreignSchema)[0]
	if _, _, ok := sourceResult(owner, foreign); ok {
		t.Fatal("test foreign seed crossed owner fence")
	}
	if instance, ok := rule.Instance(foreign); ok || instance != nil {
		t.Fatal("foreign seed produced a Rule instance")
	}
}

func TestSourceDeclarationIsIndependentOfCanonicalModuleOrder(t *testing.T) {
	first := sourceProgram(t, "source_order_a.lua", "return 1, {}")
	second := sourceProgram(t, "source_order_b.lua", "return false, 'x'")
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	seal := func(modules []linkproject.Module) *value.Schema {
		linked, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
		if err != nil {
			t.Fatal(err)
		}
		heaps, heapsOK := heap.Seal(linked)
		schema, ok := value.Seal(linked, heaps)
		if !heapsOK || !ok {
			t.Fatal("Value schema")
		}
		return schema
	}
	forward := seal([]linkproject.Module{{Name: "a", Program: first}, {Name: "b", Program: second}})
	reverse := seal([]linkproject.Module{{Name: "b", Program: second}, {Name: "a", Program: first}})
	forwardID := sourceCompositionID(t, forward, sourceKey(60), sourceKey(64), sourceKey(61), sourceKey(62), sourceKey(63))
	reverseID := sourceCompositionID(t, reverse, sourceKey(60), sourceKey(64), sourceKey(61), sourceKey(62), sourceKey(63))
	if forward.Link().ContentID() != reverse.Link().ContentID() || forwardID != reverseID {
		t.Fatal("canonical module order changed source Rule semantics")
	}
}

func TestDeclareRejectsNilAndForeignOwnerCapabilities(t *testing.T) {
	schema, _, _, _ := sourceSchema(t, "source_owner.lua", "return 1")
	composition := engine.NewComposition()
	if rule, ok := Declare(composition, sourceKey(71), sourceKey(72), sourceKey(73), nil); ok || rule != nil {
		t.Fatal("nil Value owner declared a source Rule")
	}
	foreignComposition := engine.NewComposition()
	foreignOwner, ok := valueowner.Declare(foreignComposition, sourceKey(70), sourceKey(79), schema)
	if !ok {
		t.Fatal("foreign Value owner")
	}
	if rule, ok := Declare(composition, sourceKey(71), sourceKey(72), sourceKey(73), foreignOwner); ok || rule != nil {
		t.Fatal("foreign-composition Value owner declared a source Rule")
	}
	if rule, ok := Declare(nil, sourceKey(71), sourceKey(72), sourceKey(73), foreignOwner); ok || rule != nil {
		t.Fatal("nil Composition declared a source Rule")
	}
}

func sourceComposition(
	t testing.TB,
	schema *value.Schema,
	factorSemantic, summarySemantic, ruleSemantic, operandFamily, evidenceSemantic engine.SemanticKey,
) (*engine.Composition, *valueowner.Owner, *Rule) {
	t.Helper()
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, factorSemantic, summarySemantic, schema)
	if !ok {
		t.Fatal("Value owner declaration")
	}
	rule, ok := Declare(composition, ruleSemantic, operandFamily, evidenceSemantic, owner)
	if !ok || rule == nil {
		t.Fatal("source Rule declaration")
	}
	if !declareSourceTestQuery(composition, owner) {
		t.Fatal("source test Query declaration")
	}
	if !composition.Seal() {
		t.Fatal("source composition seal")
	}
	return composition, owner, rule
}

func declareSourceTestQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: sourceKey(900_001),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: sourceKey(900_002),
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
		_, declared := engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	return ok && query != nil
}

func sourceCompositionID(
	t testing.TB,
	schema *value.Schema,
	factorSemantic, summarySemantic, ruleSemantic, operandFamily, evidenceSemantic engine.SemanticKey,
) engine.CompositionID {
	t.Helper()
	composition, _, _ := sourceComposition(t, schema, factorSemantic, summarySemantic, ruleSemantic, operandFamily, evidenceSemantic)
	return composition.ID()
}

func sourceSchema(t testing.TB, name, text string) (*value.Schema, *link.Link, *target.Contract, *program.Program) {
	t.Helper()
	p := sourceProgram(t, name, text)
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := value.Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema")
	}
	return schema, linked, contract, p
}

func sourceProgram(t testing.TB, name, text string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func sourceSeeds(t testing.TB, schema *value.Schema) []value.SourceSeed {
	t.Helper()
	result := make([]value.SourceSeed, 0)
	for index := 0; index < schema.Link().Boundary().Values().Count(); index++ {
		if seed, ok := schema.SourceSeedAt(index); ok {
			result = append(result, seed)
		}
	}
	if len(result) == 0 {
		t.Fatal("empty source seed denominator")
	}
	return result
}

func mustSeedCoordinate(t testing.TB, seed value.SourceSeed) value.Coordinate {
	t.Helper()
	coordinate, _, ok := seed.Result()
	if !ok {
		t.Fatal("source seed result")
	}
	return coordinate
}

func sourceKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("source test semantic key")
	}
	return key
}
