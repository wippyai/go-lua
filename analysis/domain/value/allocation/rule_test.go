package allocation

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuetransfer "github.com/wippyai/go-lua/analysis/domain/value/transfer"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclarePublishesOneTransformedValueCarry(t *testing.T) {
	schema, heaps, _, _, _ := allocationFixture(t)
	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(1), allocationKey(900_001), schema)
	rule, ruleOK := Declare(composition, allocationKey(2), allocationKey(3), allocationKey(4), allocationKey(5), owner)
	if !ownerOK || !ruleOK || rule == nil {
		t.Fatal("allocation declaration")
	}
	if !declareAllocationQuery(composition, owner, allocationKey(6), allocationKey(7)) || !composition.Seal() {
		t.Fatal("allocation composition seal")
	}
	inventory, inventoryOK := composition.RuleAdmissionInventory()
	if !inventoryOK || len(inventory.Rules) != 1 || inventory.Rules[0] != (engine.RuleAdmissionRecord{
		Rule: allocationKey(2), Basis: engine.RuleAdmissionBasisDerivation, Identity: allocationKey(5),
	}) {
		t.Fatal("allocation Rule omitted derivation evidence admission")
	}
	report, reportOK := composition.SemanticReport()
	if !reportOK || len(report.Incidences) != 1 || report.Incidences[0] != (engine.FactorIncidence{Read: allocationKey(1), Write: allocationKey(1)}) ||
		len(report.Components) != 1 || len(report.Components[0].Factors) != 1 || report.Components[0].Factors[0] != allocationKey(1) {
		t.Fatal("allocation Rule omitted its self transformed-carry incidence")
	}
	if key, ok := heapAllocationKey(t, heaps); !ok {
		t.Fatal("allocation fixture Key")
	} else if _, instanceOK := rule.Instance(key); !instanceOK {
		t.Fatal("allocation Key did not derive one Rule instance")
	}
}

func TestInstanceAcceptsOnlyItsHeapKeyAndReplayIdentity(t *testing.T) {
	schema, heaps, linked, programValue, contract := allocationFixture(t)
	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(10), allocationKey(900_010), schema)
	rule, ruleOK := Declare(composition, allocationKey(11), allocationKey(12), allocationKey(13), allocationKey(14), owner)
	if !ownerOK || !ruleOK || rule == nil || !declareAllocationQuery(composition, owner, allocationKey(15), allocationKey(16)) || !composition.Seal() {
		t.Fatal("allocation instance declaration")
	}
	key, keyOK := heapAllocationKey(t, heaps)
	if !keyOK {
		t.Fatal("local allocation Key")
	}
	if instance, ok := rule.Instance(key); !ok || instance == nil {
		t.Fatal("local allocation Key did not bind an instance")
	}
	if instance, ok := rule.Instance(heap.Key{}); ok || instance != nil {
		t.Fatal("zero Heap Key entered allocation Rule")
	}

	artifact, err := link.EncodeArtifact(linked)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := link.DecodeArtifact(artifact, contract, map[keyspace.ContentID]*program.Program{programValue.ContentID(): programValue})
	if err != nil {
		t.Fatal(err)
	}
	replayedHeap, replayedHeapOK := heap.Seal(replayed)
	replayedKey, replayedKeyOK := heapAllocationKey(t, replayedHeap)
	if !replayedHeapOK || !replayedKeyOK {
		t.Fatal("replayed allocation Key")
	}
	if instance, ok := rule.Instance(replayedKey); ok || instance != nil {
		t.Fatal("foreign replayed Heap Key crossed Value owner fence")
	}
	localOperand, localOperandOK := allocationOperandFor(owner, key)
	_, localDigest, localOK := allocationOperandContent(owner, localOperand)
	replayedOwner := replayAllocationOwner(t, replayed, replayedHeap)
	replayedOperand, replayedOperandOK := allocationOperandFor(replayedOwner, replayedKey)
	_, replayedDigest, replayedOK := allocationOperandContent(replayedOwner, replayedOperand)
	if !localOperandOK || !localOK || !replayedOperandOK || !replayedOK || localDigest == [32]byte{} || localDigest != replayedDigest {
		t.Fatal("allocation Key identity changed across artifact replay")
	}
}

// The predecessor is a genuine zero-input engine Rule, but its only fact is a
// Value-local alias of the selected allocation root.  RunOne then gives the
// allocation Rule its sole real identity predecessor through Batch, Assembly,
// and Solve.  Observing both coordinates proves that carry aging and the
// exact Fresh result arrived in the same accepted contribution.
func TestAllocationRuleExecutionAgesAliasAndWritesFreshRecent(t *testing.T) {
	schema, heaps, linked, _, _ := allocationFixture(t)
	key, keyOK := heapAllocationKey(t, heaps)
	if !keyOK {
		t.Fatal("allocation execution Key")
	}
	root, resultCoordinate, fresh, resultOK := allocationResultForTest(schema, key)
	aliasCoordinate := distinctCoordinate(t, schema, linked, resultCoordinate)
	recentAtom, recentOK := schema.Allocation(root, materialization.Recent)
	summaryAtom, summaryOK := schema.Allocation(root, materialization.Summary)
	aliasRecent, aliasRecentOK := schema.Singleton(recentAtom)
	aliasSummary, aliasSummaryOK := schema.Singleton(summaryAtom)
	if !resultOK || !recentOK || !summaryOK || !aliasRecentOK || !aliasSummaryOK || !schema.Equal(fresh, aliasRecent) {
		t.Fatal("allocation execution facts")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(100), allocationKey(900_100), schema)
	predecessor, predecessorOK := declareAliasPredecessor(composition, allocationKey(101), allocationKey(102), allocationKey(103), owner, key, aliasCoordinate, aliasRecent)
	rule, ruleOK := Declare(composition, allocationKey(104), allocationKey(105), allocationKey(106), allocationKey(107), owner)
	if !ownerOK || !predecessorOK || !ruleOK || predecessor == nil || rule == nil {
		t.Fatal("allocation execution declaration")
	}

	var aliasRead, resultRead engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: allocationKey(108),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				aliasCells, aliasCellsOK := engine.QueryValue(row, aliasRead)
				resultCells, resultCellsOK := engine.QueryValue(row, resultRead)
				alias, aliasPresent, aliasOK := oneValue(aliasCells, aliasCellsOK)
				result, resultPresent, resultOK := oneValue(resultCells, resultCellsOK)
				return rows == 1 && aliasOK && resultOK && aliasPresent && resultPresent &&
					schema.Equal(alias, aliasSummary) && schema.Equal(result, fresh)
			}) && rows == 1
		},
		Result: allocationBoolResult(allocationKey(109)),
	}, func(query *engine.Query[bool]) bool {
		var aliasDeclared, resultDeclared bool
		aliasRead, aliasDeclared = engine.QueryReadFrom(query, owner.ExactRead())
		resultRead, resultDeclared = engine.QueryReadFrom(query, owner.ExactRead())
		return aliasDeclared && resultDeclared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("allocation execution query/seal")
	}
	aliasRef, aliasRefOK := owner.Locate(aliasCoordinate)
	resultRef, resultRefOK := owner.Locate(resultCoordinate)
	predecessorInstance, predecessorInstanceOK := predecessor.Instance()
	targetInstance, targetInstanceOK := rule.Instance(key)
	if !aliasRefOK || !resultRefOK || !predecessorInstanceOK || !targetInstanceOK || predecessorInstance == nil || targetInstance == nil {
		t.Fatal("allocation execution bindings")
	}
	if evidence, admitted := allocationChecker(owner, allocationKey(104), allocationKey(106))(engine.RuleDerivation[value.Value, operand]{}); admitted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged allocation derivation minted evidence")
	}

	result := testlaw.RunOne(context.Background(), testlaw.OneFixture[value.Value, allocationAliasOperand, value.Value, operand, bool]{
		Composition:           composition,
		Predecessor:           predecessorInstance,
		Target:                targetInstance,
		Query:                 query,
		PredecessorSite:       allocationKey(110),
		PredecessorOccurrence: allocationKey(111),
		TargetSite:            allocationKey(112),
		TargetOccurrence:      allocationKey(113),
		BoundarySemantic:      allocationKey(114),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, aliasRead, aliasRef) && engine.InstanceQueryRead(binding, resultRead, resultRef)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("allocation execution = status:%v observed:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

// TestAllocationRuleZeroRowAtAbsentSourceCompletesWithoutFreshValue exercises
// the production SourceAssembly/Solver boundary with an unreachable input.
// The allocation member is still assembled at the absent target, but its
// predecessor support is empty, so Product must be a successful zero-row
// structural no-op: no checker admission and no Fresh write.
func TestAllocationRuleZeroRowAtAbsentSourceCompletesWithoutFreshValue(t *testing.T) {
	schema, heaps, _, _, _ := allocationFixture(t)
	key, keyOK := heapAllocationKey(t, heaps)
	if !keyOK {
		t.Fatal("allocation zero-row Key")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(180), allocationKey(900_180), schema)
	rule, ruleOK := Declare(composition, allocationKey(181), allocationKey(182), allocationKey(183), allocationKey(184), owner)
	if !ownerOK || !ruleOK || rule == nil {
		t.Fatal("allocation zero-row declaration")
	}
	coordinate, fresh, resultOK := allocationResult(owner, key)
	if !resultOK {
		t.Fatal("allocation zero-row result")
	}

	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: allocationKey(185),
		Project: func(observation engine.Observation) bool {
			freshPresent := false
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, readable := engine.QueryValue(row, read)
				if !readable || cells.Count() != 1 {
					return false
				}
				actual, present, cellOK := cells.At(0)
				if !cellOK {
					return false
				}
				freshPresent = present && schema.Equal(actual, fresh)
				return true
			}) {
				return false
			}
			return !freshPresent
		},
		Result: allocationBoolResult(allocationKey(186)),
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("allocation zero-row composition seal")
	}
	ref, refOK := owner.Locate(coordinate)
	instance, instanceOK := rule.Instance(key)
	if !refOK || !instanceOK || instance == nil {
		t.Fatal("allocation zero-row binding")
	}

	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("allocation zero-row source")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	inputSite, inputSiteOK := source.Site(allocationKey(187), scope, falsity, false)
	targetSite, targetSiteOK := source.Site(allocationKey(188), scope, falsity, false)
	occurrence, occurrenceOK := source.Relation(targetSite, allocationKey(189))
	prepared, preparedOK := source.PrepareInstance(occurrence, instance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(inputSite, targetSite, allocationKey(190), truth, reindex, truth)
	if !scopeOK || !truthOK || !falseOK || !inputSiteOK || !targetSiteOK || !occurrenceOK || !preparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("allocation zero-row source assembly")
	}

	var queryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		_, inputPointOK := assembly.Point(inputSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		member, memberOK := assembly.Member(targetPoint, prepared)
		group, groupOK := assembly.Group(targetPoint, member)
		boundaryOK := assembly.Boundary(group, boundary)
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, ref)
		})
		_, observationOK := assembly.Query(targetPoint, queryInstance)
		return inputPointOK && targetPointOK && memberOK && groupOK && boundaryOK && queryOK && observationOK
	})
	if !assembled || solver == nil || queryInstance == nil {
		t.Fatal("allocation zero-row solver assembly")
	}

	state, status, report := solver.SolveWithReport(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	absent, absentOK := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || report.Available() || !receiptOK || !absentOK || !absent {
		t.Fatalf("allocation zero-row solve status=%v state=%v report=%v receipt=%t result=%t/%t", status, state, report.Available(), receiptOK, absentOK, absent)
	}
}

// TestAllocationRuleSelfCarrySeedsFreshRecent exercises the same recurrence
// shape used by allocation occurrences in a source topology: one init-present
// site, the allocation member alone, and an identity boundary from that site
// back to itself. No predecessor Rule or fixture-supplied Value fact exists.
func TestAllocationRuleSelfCarrySeedsFreshRecent(t *testing.T) {
	schema, heaps, _, _, _ := allocationFixture(t)
	key, keyOK := heapAllocationKey(t, heaps)
	_, coordinate, fresh, resultOK := allocationResultForTest(schema, key)
	if !keyOK || !resultOK {
		t.Fatal("allocation self-carry fixture")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(120), allocationKey(900_120), schema)
	rule, ruleOK := Declare(composition, allocationKey(121), allocationKey(122), allocationKey(123), allocationKey(124), owner)
	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: allocationKey(125),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				actual, present, available := cells.At(0)
				return rows == 1 && cellsOK && available && present && schema.Equal(actual, fresh)
			}) && rows == 1
		},
		Result: allocationBoolResult(allocationKey(126)),
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !ownerOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("allocation self-carry declaration")
	}
	ref, refOK := owner.Locate(coordinate)
	instance, instanceOK := rule.Instance(key)
	if !refOK || !instanceOK || instance == nil {
		t.Fatal("allocation self-carry binding")
	}

	result := testlaw.RunSelf(context.Background(), testlaw.SelfFixture[value.Value, operand, bool]{
		Composition: composition, Instance: instance, Query: query,
		SiteSemantic: allocationKey(127), OccurrenceSemantic: allocationKey(128), BoundarySemantic: allocationKey(128),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, ref)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("allocation self-carry = status:%v observed:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

// TestAllocationSelfCarryTraversesCanonicalStorageTransfers keeps the
// allocation recurrence owner-correct while proving that its fresh fact can
// traverse two real fixed Value storage relations to an absent terminal site.
// The test supplies neither an intermediate nor terminal Value fact.
func TestAllocationSelfCarryTraversesCanonicalStorageTransfers(t *testing.T) {
	schema, heaps, _, _, _ := allocationFixture(t)
	key, keyOK := heapAllocationKey(t, heaps)
	_, start, fresh, resultOK := allocationResultForTest(schema, key)
	if !keyOK || !resultOK {
		t.Fatal("allocation storage-chain fixture")
	}
	transfers := allocationStoragePath(t, schema, start)
	if len(transfers) < 2 {
		t.Fatalf("allocation storage chain length=%d, want at least 2", len(transfers))
	}
	_, terminal, terminalOK := transfers[1].Endpoints()
	if !terminalOK {
		t.Fatal("allocation terminal storage coordinate")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(140), allocationKey(900_140), schema)
	allocationRule, allocationRuleOK := Declare(composition, allocationKey(141), allocationKey(142), allocationKey(143), allocationKey(144), owner)
	storageRule, storageRuleOK := valuetransfer.Declare(composition, allocationKey(145), allocationKey(146), allocationKey(147), owner)
	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: allocationKey(148),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				actual, present, available := cells.At(0)
				return rows == 1 && cellsOK && available && present && schema.Equal(actual, fresh)
			}) && rows == 1
		},
		Result: allocationBoolResult(allocationKey(149)),
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !ownerOK || !allocationRuleOK || allocationRule == nil || !storageRuleOK || storageRule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("allocation storage-chain declaration")
	}
	terminalRef, terminalRefOK := owner.Locate(terminal)
	allocationInstance, allocationInstanceOK := allocationRule.Instance(key)
	steps := make([]*engine.RuleInstance[value.Value, value.StorageTransfer], 2)
	stepsOK := true
	for index := range steps {
		steps[index], stepsOK = storageRule.Instance(transfers[index])
		if !stepsOK || steps[index] == nil {
			break
		}
	}
	if !terminalRefOK || !allocationInstanceOK || allocationInstance == nil || !stepsOK {
		t.Fatal("allocation storage-chain bindings")
	}

	result := testlaw.RunSelfLinear(context.Background(), testlaw.SelfLinearFixture[value.Value, operand, value.StorageTransfer, bool]{
		Composition: composition, Source: allocationInstance, Steps: steps, Query: query,
		SourceSite: allocationKey(150), SourceOccurrence: allocationKey(151), SourceBoundary: allocationKey(151),
		StepSites:       []engine.SemanticKey{allocationKey(152), allocationKey(155)},
		StepOccurrences: []engine.SemanticKey{allocationKey(153), allocationKey(156)},
		BoundaryKeys:    []engine.SemanticKey{allocationKey(154), allocationKey(157)},
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, terminalRef)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("allocation storage-chain = status:%v observed:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

func allocationStoragePath(t testing.TB, schema *value.Schema, start value.Coordinate) []value.StorageTransfer {
	t.Helper()
	current := start
	path := make([]value.StorageTransfer, 0, 2)
	seen := make(map[value.Coordinate]struct{})
	for {
		if _, duplicate := seen[current]; duplicate {
			t.Fatal("allocation storage cycle")
		}
		seen[current] = struct{}{}
		var next value.StorageTransfer
		found := false
		for index := 0; index < schema.StorageTransferCount(); index++ {
			candidate, candidateOK := schema.StorageTransferAt(index)
			from, _, endpointsOK := candidate.Endpoints()
			if !candidateOK || !endpointsOK || from != current {
				continue
			}
			if found {
				t.Fatal("allocation storage fanout")
			}
			next, found = candidate, true
		}
		if !found {
			return path
		}
		_, current, _ = next.Endpoints()
		path = append(path, next)
	}
}

type allocationAliasOperand struct{ key heap.Key }

type allocationAliasPredecessor struct {
	rule  *engine.Rule[value.Value, allocationAliasOperand]
	write engine.Write[value.Value]
	owner *valueowner.Owner
	op    allocationAliasOperand
	to    value.Coordinate
}

func declareAliasPredecessor(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *valueowner.Owner, key heap.Key, to value.Coordinate, fact value.Value) (*allocationAliasPredecessor, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil {
		return nil, false
	}
	operand := allocationAliasOperand{key: key}
	predecessor := &allocationAliasPredecessor{owner: owner, op: operand, to: to}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, allocationAliasOperand]{
		Semantic:      semantic,
		OperandFamily: family,
		OperandContent: func(candidate allocationAliasOperand) (allocationAliasOperand, [32]byte, bool) {
			allocation, allocationOK := allocationOperandFor(owner, candidate.key)
			if !allocationOK {
				return allocationAliasOperand{}, [32]byte{}, false
			}
			return candidate, allocation.digest, true
		},
		Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, func(derivation engine.RuleDerivation[value.Value, allocationAliasOperand]) (engine.RuleEvidence, bool) {
			if derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			candidate, candidateOK := derivation.Operand()
			allocation, contentOK := allocationOperandFor(owner, candidate.key)
			digest := allocation.digest
			disposition, dispositionOK := derivation.DispositionAt(0)
			actual, actualOK := disposition.Value()
			target, targetOK := disposition.TargetAt(0)
			ref, refOK := owner.Locate(to)
			if !candidateOK || !contentOK || !derivation.OperandContentMatches(digest) || !dispositionOK || disposition.Kind() != engine.RuleDispositionStaged ||
				disposition.TransformOnly() || disposition.TargetCount() != 1 || disposition.Guard().Empty() || !actualOK || !owner.Schema().Equal(actual, fact) || !targetOK || !refOK || !engine.TargetMatchesRef(target, ref) {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access engine.Access[value.Value, allocationAliasOperand]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, fact) })
		},
	}, func(rule *engine.Rule[value.Value, allocationAliasOperand]) bool {
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if writeOK {
			predecessor.write = write
		}
		return writeOK
	})
	if !ok || declared == nil {
		return nil, false
	}
	predecessor.rule = declared
	return predecessor, true
}

func (predecessor *allocationAliasPredecessor) Instance() (*engine.RuleInstance[value.Value, allocationAliasOperand], bool) {
	if predecessor == nil || predecessor.rule == nil || predecessor.owner == nil {
		return nil, false
	}
	ref, ok := predecessor.owner.Locate(predecessor.to)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(predecessor.rule, predecessor.op, func(binding *engine.RuleBinding[value.Value, allocationAliasOperand]) bool {
		return engine.InstanceWrite(binding, predecessor.write, ref)
	})
}

func allocationFixture(t testing.TB) (*value.Schema, heap.Schema, *link.Link, *program.Program, *target.Contract) {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: "allocation_rule.lua", Text: []byte("local root = {}\nlocal alias = root\nlocal witness = 1\nreturn alias, witness\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, schemaOK := value.Seal(linked, heaps)
	if !heapsOK || !schemaOK {
		t.Fatal("allocation Value schema")
	}
	return schema, heaps, linked, programValue, contract
}

func heapAllocationKey(t testing.TB, heaps heap.Schema) (heap.Key, bool) {
	t.Helper()
	for index := 0; index < heaps.KeyCount(); index++ {
		key, keyOK := heaps.KeyAt(index)
		if _, _, _, programAllocation := key.ProgramAllocation(); keyOK && programAllocation {
			return key, true
		}
	}
	return heap.Key{}, false
}

func allocationResultForTest(schema *value.Schema, key heap.Key) (heap.Key, value.Coordinate, value.Value, bool) {
	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(700), allocationKey(900_700), schema)
	if !ownerOK {
		return heap.Key{}, value.Coordinate{}, value.Value{}, false
	}
	coordinate, fresh, ok := allocationResult(owner, key)
	return key, coordinate, fresh, ok
}

func distinctCoordinate(t testing.TB, schema *value.Schema, linked *link.Link, excluded value.Coordinate) value.Coordinate {
	t.Helper()
	values := linked.Boundary().Values()
	for index := 0; index < values.Count(); index++ {
		subject, subjectOK := values.At(index)
		coordinate, coordinateOK := schema.CoordinateFor(subject)
		if subjectOK && coordinateOK && coordinate != excluded {
			return coordinate
		}
	}
	t.Fatal("allocation fixture lacked a distinct alias coordinate")
	return value.Coordinate{}
}

func replayAllocationOwner(t testing.TB, schemaSource *link.Link, heaps heap.Schema) *valueowner.Owner {
	t.Helper()
	schema, schemaOK := value.Seal(schemaSource, heaps)
	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, allocationKey(710), allocationKey(900_710), schema)
	if !schemaOK || !ownerOK {
		t.Fatal("replayed allocation owner")
	}
	return owner
}

func oneValue(cells engine.OrderedCells[value.Value], readable bool) (value.Value, bool, bool) {
	if !readable || cells.Count() != 1 {
		return value.Value{}, false, false
	}
	return cells.At(0)
}

func allocationBoolResult(semantic engine.SemanticKey) engine.FrozenResult[bool] {
	return engine.FrozenResult[bool]{
		Semantic: semantic, Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value }, Equal: func(left, right bool) bool { return left == right },
		Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
	}
}

func declareAllocationQuery(composition *engine.Composition, owner *valueowner.Owner, semantic, resultSemantic engine.SemanticKey) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: semantic,
		Project:  func(engine.Observation) bool { return true },
		Result:   allocationBoolResult(resultSemantic),
	}, func(query *engine.Query[bool]) bool {
		_, declared := engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	return ok && query != nil
}

func allocationKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("allocation semantic key")
	}
	return key
}
