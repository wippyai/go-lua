package transfer

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
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclarePublishesOneCarriedExactValueTransfer(t *testing.T) {
	schema, _ := transferSchema(t)
	composition, owner, rule := transferComposition(t, schema)
	if owner.Schema() != schema || rule == nil {
		t.Fatal("transfer declaration lost Value owner")
	}
	inventory, ok := composition.RuleAdmissionInventory()
	if !ok || len(inventory.Rules) != 1 || inventory.Rules[0] != (engine.RuleAdmissionRecord{
		Rule: transferKey(2), Basis: engine.RuleAdmissionBasisDerivation, Identity: transferKey(4),
	}) {
		t.Fatal("Value transfer derivation admission")
	}
	report, ok := composition.SemanticReport()
	if !ok || len(report.Incidences) != 1 || report.Incidences[0] != (engine.FactorIncidence{Read: transferKey(1), Write: transferKey(1)}) ||
		len(report.Components) != 1 || len(report.Components[0].Factors) != 1 || report.Components[0].Factors[0] != transferKey(1) {
		t.Fatal("Value transfer semantic component")
	}

	instances := 0
	for index := 0; index < schema.StorageTransferCount(); index++ {
		operand, ok := schema.StorageTransferAt(index)
		if !ok {
			t.Fatalf("Value StorageTransferAt(%d)", index)
		}
		frozen, digest, frozenOK := storageTransferContent(operand)
		replayed, replayDigest, replayOK := storageTransferContent(frozen)
		if !frozenOK || !replayOK || digest == [32]byte{} || digest != replayDigest || replayed != operand {
			t.Fatalf("StorageTransfer(%d) OperandContent is not pure and idempotent", index)
		}
		if instance, ok := rule.Instance(operand); !ok || instance == nil {
			t.Fatalf("StorageTransfer(%d) did not derive atomic RuleInstance", index)
		}
		instances++
	}
	if instances == 0 {
		t.Fatal("empty fixed storage transfer denominator")
	}
}

func TestTransferRejectsInvalidSemanticsAndForeignOwnerOrOperand(t *testing.T) {
	schema, linked := transferSchema(t)
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, transferKey(10), transferKey(900_010), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	if rule, ok := Declare(composition, transferKey(11), transferKey(11), transferKey(13), owner); ok || rule != nil {
		t.Fatal("colliding semantic keys declared transfer Rule")
	}
	foreignComposition := engine.NewComposition()
	foreignOwner, ok := valueowner.Declare(foreignComposition, transferKey(14), transferKey(900_014), schema)
	if !ok {
		t.Fatal("foreign Value owner")
	}
	if rule, ok := Declare(composition, transferKey(15), transferKey(16), transferKey(17), foreignOwner); ok || rule != nil {
		t.Fatal("foreign owner declared transfer Rule")
	}

	sealed, localOwner, rule := transferComposition(t, schema)
	operand, ok := schema.StorageTransferAt(0)
	if !ok {
		t.Fatal("local Value storage transfer")
	}
	if _, _, ok := transferEndpoints(localOwner, operand); !ok {
		t.Fatal("local transfer endpoints")
	}
	if instance, ok := rule.Instance(value.StorageTransfer{}); ok || instance != nil {
		t.Fatal("zero storage transfer entered Rule")
	}

	foreignSchema, foreignLink := transferSchema(t)
	if foreignLink.ContentID() != linked.ContentID() || foreignLink == linked {
		t.Fatal("same-content foreign Link")
	}
	foreignOperand, ok := foreignSchema.StorageTransferAt(0)
	if !ok {
		t.Fatal("foreign Value storage transfer")
	}
	if _, _, ok := transferEndpoints(localOwner, foreignOperand); ok {
		t.Fatal("foreign transfer crossed Value owner fence")
	}
	if instance, ok := rule.Instance(foreignOperand); ok || instance != nil {
		t.Fatal("foreign transfer derived local RuleInstance")
	}
	if !sealed.Sealed() {
		t.Fatal("transfer composition seal")
	}
}

// A RuleDerivation is an engine-issued, opaque capability.  This adversarial
// zero-shaped input is the only shape a domain test can forge; it must not
// manufacture evidence for either a different Rule or a detached transfer.
func TestTransferCheckerRejectsForgedDerivation(t *testing.T) {
	schema, _ := transferSchema(t)
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, transferKey(80), transferKey(900_080), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	var read engine.Read[engine.OrderedCells[value.Value]]
	checker := transferChecker(owner, transferKey(81), &read)
	if evidence, accepted := checker(engine.RuleDerivation[value.Value, value.StorageTransfer]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged derivation minted Value transfer evidence")
	}
}

// TestStorageTransferRuleCarriesOneLiteralThroughCanonicalLocalCells executes
// the only source and storage-copy Rules over Link's exact Read/Bind/Write
// relation for a real local-variable program. No test-side value is staged:
// observing the literal at the final read requires every runtime-issued
// derivation in the chain to be accepted by its owning production Rule.
func TestStorageTransferRuleCarriesOneLiteralThroughCanonicalLocalCells(t *testing.T) {
	schema, linked := transferExecutionSchema(t)
	var seed value.SourceSeed
	var literalCoordinate value.Coordinate
	var literal value.Value
	seedFound := false
	for index := 0; index < linked.Boundary().Values().Count(); index++ {
		candidate, candidateOK := schema.SourceSeedAt(index)
		coordinate, fact, resultOK := candidate.Result()
		if candidateOK && resultOK && schema.RuntimeKinds(fact) == runtimekind.Bit(runtimekind.Number) {
			seed, literalCoordinate, literal, seedFound = candidate, coordinate, fact, true
			break
		}
	}
	if !seedFound || schema.Equal(literal, schema.Bottom()) {
		t.Fatal("local-cell fixture did not issue its literal Value seed")
	}

	transfers := storageTransferChain(t, schema, literalCoordinate)
	if len(transfers) != 4 {
		t.Fatalf("local-cell storage chain length=%d, want Read/Bind/Read/Bind/Read path of four transfers", len(transfers))
	}
	_, finalCoordinate, finalOK := transfers[len(transfers)-1].Endpoints()
	if !finalOK {
		t.Fatal("final local-cell transfer endpoints")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, transferKey(1_000_001), transferKey(1_000_002), schema)
	sourceRule, sourceOK := valuesource.Declare(composition, transferKey(1_000_003), transferKey(1_000_004), transferKey(1_000_005), owner)
	transferRule, transferOK := Declare(composition, transferKey(1_000_006), transferKey(1_000_007), transferKey(1_000_008), owner)
	if !ownerOK || !sourceOK || !transferOK || owner == nil || sourceRule == nil || transferRule == nil {
		t.Fatal("local-cell production Rule declaration")
	}

	var literalRead, finalRead engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transferKey(1_000_009),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				literalCells, literalCellsOK := engine.QueryValue(row, literalRead)
				finalCells, finalCellsOK := engine.QueryValue(row, finalRead)
				if !literalCellsOK || !finalCellsOK || literalCells.Count() != 1 || finalCells.Count() != 1 {
					return false
				}
				actualLiteral, literalPresent, literalAvailable := literalCells.At(0)
				actualFinal, finalPresent, finalAvailable := finalCells.At(0)
				return rows == 1 && literalAvailable && literalPresent && finalAvailable && finalPresent &&
					schema.Equal(actualLiteral, literal) && schema.Equal(actualFinal, literal) && schema.Equal(actualFinal, actualLiteral)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: transferKey(1_000_010),
			Freeze:   func(result bool) bool { return result }, Clone: func(result bool) bool { return result }, Equal: func(left, right bool) bool { return left == right },
			Fingerprint: func(result bool) uint64 {
				if result {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var literalDeclared, finalDeclared bool
		literalRead, literalDeclared = engine.QueryReadFrom(query, owner.ExactRead())
		finalRead, finalDeclared = engine.QueryReadFrom(query, owner.ExactRead())
		return literalDeclared && finalDeclared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("local-cell query/seal")
	}

	literalRef, literalRefOK := owner.Locate(literalCoordinate)
	finalRef, finalRefOK := owner.Locate(finalCoordinate)
	sourceInstance, sourceInstanceOK := sourceRule.Instance(seed)
	instances := make([]*engine.RuleInstance[value.Value, value.StorageTransfer], len(transfers))
	instancesOK := true
	for index, transfer := range transfers {
		instances[index], instancesOK = transferRule.Instance(transfer)
		if !instancesOK {
			break
		}
	}
	if !literalRefOK || !finalRefOK || !sourceInstanceOK || !instancesOK || sourceInstance == nil {
		t.Fatal("local-cell production instances")
	}

	result := testlaw.RunLinear(context.Background(), testlaw.LinearFixture[value.Value, value.SourceSeed, value.Value, value.StorageTransfer, bool]{
		Composition: composition, Source: sourceInstance, Steps: instances, Query: query,
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, literalRead, literalRef) && engine.InstanceQueryRead(binding, finalRead, finalRef)
		},
		SourceSite: transferKey(1_000_011), SourceOccurrence: transferKey(1_000_012),
		StepSites:         []engine.SemanticKey{transferKey(1_000_013), transferKey(1_000_016), transferKey(1_000_019), transferKey(1_000_022)},
		StepOccurrences:   []engine.SemanticKey{transferKey(1_000_014), transferKey(1_000_017), transferKey(1_000_020), transferKey(1_000_023)},
		BoundarySemantics: []engine.SemanticKey{transferKey(1_000_015), transferKey(1_000_018), transferKey(1_000_021), transferKey(1_000_024)},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("local-cell transfer execution = status:%v available:%t value:%t", result.Status, result.ValueAvailable, result.Value)
	}
}

func transferExecutionSchema(t testing.TB) (*value.Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "value_transfer_execution.lua", Text: []byte("local n = 1\nlocal m = n\nreturn m\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "value_transfer_execution", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := value.Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("local-cell Value schema")
	}
	return schema, linked
}

func storageTransferChain(t testing.TB, schema *value.Schema, start value.Coordinate) []value.StorageTransfer {
	t.Helper()
	current := start
	chain := make([]value.StorageTransfer, 0, 4)
	visited := make(map[value.Coordinate]struct{})
	for {
		if _, seen := visited[current]; seen {
			t.Fatal("local-cell fixture formed a fixed storage-transfer cycle")
		}
		visited[current] = struct{}{}
		var next value.StorageTransfer
		found := false
		for index := 0; index < schema.StorageTransferCount(); index++ {
			transfer, transferOK := schema.StorageTransferAt(index)
			from, _, endpointsOK := transfer.Endpoints()
			if !transferOK || !endpointsOK || from != current {
				continue
			}
			if found {
				t.Fatalf("local-cell fixture has multiple fixed transfers from one coordinate")
			}
			next, found = transfer, true
		}
		if !found {
			return chain
		}
		_, current, _ = next.Endpoints()
		chain = append(chain, next)
	}
}

func transferComposition(t testing.TB, schema *value.Schema) (*engine.Composition, *valueowner.Owner, *Rule) {
	t.Helper()
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, transferKey(1), transferKey(900_101), schema)
	if !ok {
		t.Fatal("Value owner declaration")
	}
	rule, ok := Declare(composition, transferKey(2), transferKey(3), transferKey(4), owner)
	if !ok || rule == nil {
		t.Fatal("Value transfer declaration")
	}
	if !declareTransferQuery(composition, owner) {
		t.Fatal("Value transfer query declaration")
	}
	if !composition.Seal() {
		t.Fatal("transfer composition seal")
	}
	return composition, owner, rule
}

func declareTransferQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transferKey(900_001),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: transferKey(900_002),
			Freeze:   func(result bool) bool { return result },
			Clone:    func(result bool) bool { return result },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(result bool) uint64 {
				if result {
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

func transferSchema(t testing.TB) (*value.Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "value_transfer_rule.lua", Text: []byte("local n = 1\nlocal m = n\nn = m\nreturn n\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "value_transfer_rule", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := value.Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema")
	}
	return schema, linked
}

func transferKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("transfer test semantic key")
	}
	return key
}
