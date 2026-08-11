package bootstrap

import (
	"context"
	"encoding/binary"
	"strconv"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type bootstrapObservation uint8

const (
	bootstrapObservationProjectFailed bootstrapObservation = 1 << iota
	bootstrapObservationWrongRowCount
	bootstrapObservationBadCells
	bootstrapObservationWrongWorld
	bootstrapObservationWrongValue
	bootstrapObservationExact bootstrapObservation = 0
)

// This law executes the production bootstrap Rule over one Link/Target-issued
// BootEntry. The query checks the complete public Heap world, rather than the
// retired independent-fact projection.
func TestBootstrapRuleLawHarnessStagesExactTargetEntry(t *testing.T) {
	schema := bootstrapLawSchema(t)
	entry, entryOK := schema.BootEntryAt(0)
	key, keyOK := entry.Key()
	_, slotOK := entry.Slot()
	raw, _, projectionOK := entry.Projection()
	_, childOK := entry.ValueChild()
	if !entryOK || !keyOK || !slotOK || !projectionOK || raw != heapdomain.RawPresent || !childOK || key.Kind() != heapdomain.RootBoot {
		t.Fatal("bootstrap law fixture did not issue one present rooted entry")
	}
	root, rootOK := NewRoot(schema, key)
	expected, expectedOK := bootstrapExpected(schema, key, false)
	if !rootOK || !expectedOK {
		t.Fatal("bootstrap semantic expectation")
	}

	composition := engine.NewComposition()
	owner, ownerOK := heapowner.Declare(composition, bootstrapKey(1), schema)
	rule, ruleOK := Declare(composition, bootstrapKey(3), bootstrapKey(4), bootstrapKey(5), owner)
	if !ownerOK || !ruleOK || owner == nil || rule == nil {
		t.Fatal("bootstrap law composition declaration")
	}

	var read engine.QueryRead[engine.OrderedCells[heapdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bootstrapObservation]{
		Semantic: bootstrapKey(6),
		Project: func(observation engine.Observation) bootstrapObservation {
			rows := 0
			var observed bootstrapObservation
			complete := engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				if !cellsOK || cells.Count() != 1 {
					observed |= bootstrapObservationBadCells
					return true
				}
				actual, present, cellOK := cells.At(0)
				if !cellOK || !present {
					observed |= bootstrapObservationBadCells
					return true
				}
				if actual.WorldCount() != 1 {
					observed |= bootstrapObservationWrongWorld
				} else if world, worldOK := actual.WorldAt(0); !worldOK || world.Kind() != heapdomain.WorldExact {
					observed |= bootstrapObservationWrongWorld
				}
				if !schema.Domain().Equal(actual, expected) {
					observed |= bootstrapObservationWrongValue
				}
				return true
			})
			if !complete {
				observed |= bootstrapObservationProjectFailed
			}
			if rows != 1 {
				observed |= bootstrapObservationWrongRowCount
			}
			return observed
		},
		Result: engine.FrozenResult[bootstrapObservation]{
			Semantic:    bootstrapKey(7),
			Freeze:      func(value bootstrapObservation) bootstrapObservation { return value },
			Clone:       func(value bootstrapObservation) bootstrapObservation { return value },
			Equal:       func(left, right bootstrapObservation) bool { return left == right },
			Fingerprint: func(value bootstrapObservation) uint64 { return uint64(value) },
		},
	}, func(query *engine.Query[bootstrapObservation]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("bootstrap law query/seal")
	}
	ref, refOK := owner.Locate(key)
	instance, instanceOK := rule.Instance(root)
	if !refOK || !instanceOK || instance == nil {
		t.Fatal("bootstrap law production instance")
	}

	result := testlaw.Run(context.Background(), testlaw.RuleFixture[heapdomain.Value, Root, bootstrapObservation]{
		Composition:        composition,
		Instance:           instance,
		Query:              query,
		SiteSemantic:       bootstrapKey(8),
		OccurrenceSemantic: bootstrapKey(9),
		BindQuery: func(binding *engine.QueryBinding[bootstrapObservation]) bool {
			return engine.InstanceQueryRead(binding, read, ref)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || result.Value != bootstrapObservationExact {
		t.Fatalf("bootstrap law execution = status:%v observed:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

// Target's initial-value kind is the sole containment authority for boot
// payloads. A callable is an exact Value-level callable but its Heap child is
// intentionally opaque: Heap has no structural root for it. This law checks
// the complete sealed kind set and then executes the one bootstrap Rule, so
// transfer and evidence use the same owner-issued classification.
func TestBootstrapClassifiesPresentContainmentByTargetKind(t *testing.T) {
	linked, schema := bootstrapContainmentSchema(t)
	contract, contractOK := linked.Boundary().Target()
	if !contractOK || contract == nil {
		t.Fatal("bootstrap containment target")
	}

	seen := make(map[target.InitialValueKind]bool)
	var rootKey heapdomain.Key
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		key, keyOK := entry.Key()
		raw, payload, projectionOK := entry.Projection()
		if !entryOK || !keyOK || !projectionOK || raw != heapdomain.RawPresent {
			t.Fatal("bootstrap containment present entry")
		}
		initial, initialOK := payload.InitialValue()
		kind, kindOK := contract.InitialValueKind(initial)
		containment, containmentOK := entry.ValueContainment()
		if !initialOK || !kindOK || !containmentOK {
			t.Fatal("bootstrap containment classification")
		}
		switch kind {
		case target.InitialValueRoot:
			child, childOK := entry.ValueChild()
			reference, referenceOK := containment.Reference()
			if !childOK || !referenceOK || reference != child || containment.Kind() != heapdomain.ContainmentExact {
				t.Fatal("root-valued bootstrap entry did not retain its exact child")
			}
			rootKey = key
		case target.InitialValueOperation, target.InitialValueDeniedOperation:
			if containment.Kind() != heapdomain.ContainmentUnknown {
				t.Fatal("callable bootstrap entry did not retain its opaque edge")
			}
		case target.InitialValueInteger:
			if containment.Kind() != heapdomain.ContainmentNone {
				t.Fatal("scalar bootstrap entry retained a reference edge")
			}
		default:
			t.Fatalf("unexpected containment fixture kind %v", kind)
		}
		seen[kind] = true
	}
	for _, kind := range []target.InitialValueKind{
		target.InitialValueRoot,
		target.InitialValueOperation,
		target.InitialValueDeniedOperation,
		target.InitialValueInteger,
	} {
		if !seen[kind] {
			t.Fatalf("bootstrap containment fixture omitted kind %v", kind)
		}
	}
	root, rootOK := NewRoot(schema, rootKey)
	expected, expectedOK := bootstrapExpected(schema, rootKey, false)
	if !rootOK || !expectedOK || !bootstrapRuleProduces(t, schema, root, expected, 90) {
		t.Fatal("bootstrap containment classification did not survive Rule evidence")
	}
}

// The whole-object header is a Target fact instantiated once for each actor.
// It is neither inferred from the entry set nor shared as one mutable Heap
// object across actor-local BootRoots.
func TestBootstrapPreservesWholeObjectHeaderForEveryActorCopy(t *testing.T) {
	for _, test := range []struct {
		name      string
		immutable bool
		want      heapdomain.Frozen
	}{
		{name: "mutable", immutable: false, want: heapdomain.FrozenMutable},
		{name: "frozen", immutable: true, want: heapdomain.FrozenFrozen},
	} {
		t.Run(test.name, func(t *testing.T) {
			linked, schema := bootstrapHeaderSchema(t, test.immutable, 2)
			if linked.Host().BootRoots().Count() != 2 {
				t.Fatalf("actor-local boot roots=%d, want 2", linked.Host().BootRoots().Count())
			}
			seen := make(map[keyspace.ContentID]bool)
			for index := 0; index < linked.Host().BootRoots().Count(); index++ {
				boot, bootOK := linked.Host().BootRoots().At(index)
				key, keyOK := schema.KeyForBootRoot(boot)
				frozen, frozenOK := schema.BootFrozen(key)
				root, rootOK := NewRoot(schema, key)
				id, idOK := root.ID()
				expected, expectedOK := bootstrapExpected(schema, key, false)
				world, worldOK := expected.WorldAt(0)
				object, objectOK := world.Exact()
				_, actualHeader, headerOK := object.Header()
				if !bootOK || !keyOK || !frozenOK || !rootOK || !idOK || !expectedOK || !worldOK || !objectOK || !headerOK ||
					frozen != test.want || actualHeader != test.want || seen[id] {
					t.Fatalf("boot copy %d did not preserve the exact header", index)
				}
				seen[id] = true
			}
		})
	}
}

func TestBootstrapRuleStagesWholeObjectHeader(t *testing.T) {
	for _, test := range []struct {
		name      string
		immutable bool
		want      heapdomain.Frozen
	}{
		{name: "mutable", immutable: false, want: heapdomain.FrozenMutable},
		{name: "frozen", immutable: true, want: heapdomain.FrozenFrozen},
	} {
		t.Run(test.name, func(t *testing.T) {
			linked, schema := bootstrapHeaderSchema(t, test.immutable, 1)
			boot, bootOK := linked.Host().BootRoots().At(0)
			key, keyOK := schema.KeyForBootRoot(boot)
			frozen, frozenOK := schema.BootFrozen(key)
			root, rootOK := NewRoot(schema, key)
			expected, expectedOK := bootstrapExpected(schema, key, false)
			if !bootOK || !keyOK || !frozenOK || !rootOK || !expectedOK || frozen != test.want ||
				!bootstrapRuleProduces(t, schema, root, expected, uint64(600)) {
				t.Fatal("bootstrap Rule did not stage the canonical whole-object header")
			}
		})
	}
}

// Target keeps Nil and Absent distinct for contract consumers. Heap has one
// raw-slot meaning for both: neither occupies a table key. This law checks the
// production bootstrap projection rather than a private schema row.
func TestBootstrapNilAndAbsentProjectToRawAbsence(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "heap_bootstrap_nil.lua", Text: []byte("return 1\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "BootstrapRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "BootstrapRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "nil"}, Value: target.InitialValueSpec{Kind: target.InitialValueNil}, Mutability: target.InitialMutable},
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "number"}, Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 1}, Mutability: target.InitialMutable},
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "boolean"}, Value: target.InitialValueSpec{Kind: target.InitialValueBoolean, Boolean: true}, Mutability: target.InitialMutable},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "bootstrap-nil", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, schemaOK := heapdomain.Seal(linked)
	if !schemaOK {
		t.Fatal("bootstrap nil schema")
	}

	want := make(map[linkproject.Key]heapdomain.RawPresence)
	presentCount := 0
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, exact, value, _, entryOK := contract.InitialEntryAt(index)
		kind, kindOK := contract.InitialValueKind(value)
		key, keyOK := linked.Project().Keys().ForTarget(contract, exact)
		if !entryOK || !kindOK || !keyOK {
			t.Fatal("target initial entry")
		}
		switch kind {
		case target.InitialValueNil, target.InitialValueAbsent:
			want[key] = heapdomain.RawAbsent
		default:
			want[key] = heapdomain.RawPresent
			presentCount++
		}
	}
	if presentCount < 2 {
		t.Fatal("bootstrap coexistence fixture requires two distinct present siblings")
	}
	seen := make(map[linkproject.Key]bool)
	seenRoots := make(map[heapdomain.Key]bool)
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		slot, slotOK := entry.Slot()
		kind, exact, _, _, originOK := slot.Origin()
		raw, payload, projectionOK := entry.Projection()
		if !entryOK || !slotOK || !originOK || kind != heapdomain.SlotExact || !projectionOK {
			t.Fatal("Heap bootstrap entry")
		}
		wantRaw, wantOK := want[exact]
		if !wantOK || raw != wantRaw {
			t.Fatal("Heap bootstrap projection changed Target raw-slot meaning")
		}
		if raw == heapdomain.RawAbsent {
			if _, _, _, source := payload.Source(); source {
				t.Fatal("Heap raw absence retained a Values payload")
			}
			if _, initial := payload.InitialValue(); initial {
				t.Fatal("Heap raw absence retained an initial payload")
			}
		}
		entryKey, entryKeyOK := entry.Key()
		if !entryKeyOK {
			t.Fatal("bootstrap entry root")
		}
		if !seenRoots[entryKey] {
			seenRoots[entryKey] = true
			expectedValue, expectedOK := bootstrapExpected(schema, entryKey, false)
			reversedValue, reversedOK := bootstrapExpected(schema, entryKey, true)
			root, rootOK := NewRoot(schema, entryKey)
			fingerprint, fingerprintOK := schema.Fingerprint(expectedValue)
			if !expectedOK || !reversedOK || !rootOK || len(root.entries) != len(want) || !fingerprintOK || !schema.Domain().Equal(expectedValue, reversedValue) {
				t.Fatal("bootstrap aggregate is not declaration-order independent")
			}
			if !bootstrapRuleProduces(t, schema, root, expectedValue, uint64(200+index*10)) {
				t.Fatal("bootstrap production relation disagreed with Target projection")
			}
			stableFingerprint, stableOK := schema.Fingerprint(expectedValue)
			if !stableOK || stableFingerprint != fingerprint || !schema.Domain().Equal(expectedValue, reversedValue) {
				t.Fatal("bootstrap execution mutated a published aggregate")
			}
		}
		seen[exact] = true
	}
	if len(seen) != len(want) {
		t.Fatal("Heap bootstrap omitted a Target entry")
	}
}

// bootstrapExpected independently folds every sealed entry for one BootRoot
// into one complete WorldExact relation. reverse exists only to state and
// check declaration-order independence of those coexisting table entries.
func bootstrapExpected(schema heapdomain.Schema, key heapdomain.Key, reverse bool) (heapdomain.Value, bool) {
	none, noneOK := schema.ContainmentNone()
	frozen, frozenOK := schema.BootFrozen(key)
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, frozen, none)
	if key.Kind() != heapdomain.RootBoot || !initializerOK || !noneOK || !frozenOK {
		return heapdomain.Value{}, false
	}
	entries := make([]heapdomain.BootEntry, 0)
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, keyOK := entry.Key()
		if !entryOK || !keyOK {
			return heapdomain.Value{}, false
		}
		if entryKey == key {
			entries = append(entries, entry)
		}
	}
	for left := 0; left < len(entries); left++ {
		for right := left + 1; right < len(entries); right++ {
			leftID, leftOK := entries[left].ID()
			rightID, rightOK := entries[right].ID()
			if !leftOK || !rightOK {
				return heapdomain.Value{}, false
			}
			if compareID(rightID, leftID) < 0 {
				entries[left], entries[right] = entries[right], entries[left]
			}
		}
	}
	if reverse {
		for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
			entries[left], entries[right] = entries[right], entries[left]
		}
	}
	for _, entry := range entries {
		slot, slotOK := entry.Slot()
		raw, payload, projectionOK := entry.Projection()
		selector, selectorOK := schema.SelectorForSlot(slot)
		if !slotOK || !projectionOK || !selectorOK {
			return heapdomain.Value{}, false
		}
		var state heapdomain.CellState
		var stateOK bool
		switch raw {
		case heapdomain.RawAbsent:
			state, stateOK = schema.CellAbsent()
		case heapdomain.RawPresent:
			valueChild, childOK := entry.ValueContainment()
			if !childOK {
				return heapdomain.Value{}, false
			}
			state, stateOK = schema.CellPresent(slot, payload, valueChild, none)
		default:
			return heapdomain.Value{}, false
		}
		if !stateOK || !initializer.Apply(selector, state) {
			return heapdomain.Value{}, false
		}
	}
	object, objectOK := initializer.Finish()
	world, worldOK := schema.Exact(key, object)
	value, relationOK := schema.Relation(key, world)
	if !objectOK || !worldOK || !relationOK {
		return heapdomain.Value{}, false
	}
	return value, schema.Admits(key, value)
}

// bootstrapRuleProduces runs the one production Rule and checks its completed
// result against a public complete-world expectation. It deliberately does
// not inspect engine/package composition.
func bootstrapRuleProduces(t testing.TB, schema heapdomain.Schema, root Root, expected heapdomain.Value, base uint64) bool {
	t.Helper()
	key := root.key
	_, keyOK := root.ID()
	if !keyOK {
		return false
	}
	composition := engine.NewComposition()
	owner, ownerOK := heapowner.Declare(composition, bootstrapKey(base+1), schema)
	rule, ruleOK := Declare(composition, bootstrapKey(base+2), bootstrapKey(base+3), bootstrapKey(base+4), owner)
	if !ownerOK || !ruleOK || owner == nil || rule == nil {
		return false
	}
	var read engine.QueryRead[engine.OrderedCells[heapdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: bootstrapKey(base + 5),
		Project: func(observation engine.Observation) bool {
			matched := false
			complete := engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, cellsOK := engine.QueryValue(row, read)
				actual, present, cellOK := cells.At(0)
				if !cellsOK || !cellOK || !present || cells.Count() != 1 || !schema.Domain().Equal(actual, expected) {
					return false
				}
				matched = true
				return true
			})
			return complete && matched
		},
		Result: engine.FrozenResult[bool]{
			Semantic: bootstrapKey(base + 6), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
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
		return false
	}
	ref, refOK := owner.Locate(key)
	instance, instanceOK := rule.Instance(root)
	if !refOK || !instanceOK || instance == nil {
		return false
	}
	result := testlaw.Run(context.Background(), testlaw.RuleFixture[heapdomain.Value, Root, bool]{
		Composition: composition, Instance: instance, Query: query, SiteSemantic: bootstrapKey(base + 7), OccurrenceSemantic: bootstrapKey(base + 8),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool { return engine.InstanceQueryRead(binding, read, ref) },
	})
	return result.Status == engine.SolveComplete && result.ValueAvailable && result.Value
}

func bootstrapLawSchema(t testing.TB) heapdomain.Schema {
	_, schema := bootstrapHeaderSchema(t, false, 1)
	return schema
}

func bootstrapContainmentSchema(t testing.TB) (*link.Link, heapdomain.Schema) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "heap_bootstrap_containment.lua", Text: []byte("return 1\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"admitted"}}},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{
			Identity: "BootstrapRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "BootstrapRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "self"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "BootstrapRoot"}, Mutability: target.InitialMutable},
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "admitted"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"admitted"}}}, Mutability: target.InitialMutable},
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "denied"}, Value: target.InitialValueSpec{Kind: target.InitialValueDeniedOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"denied"}}}, Mutability: target.InitialMutable},
			{Root: "BootstrapRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "count"}, Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 1}, Mutability: target.InitialMutable},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "bootstrap-containment", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := heapdomain.Seal(linked)
	if !ok {
		t.Fatal("bootstrap containment schema")
	}
	return linked, schema
}

func bootstrapHeaderSchema(t testing.TB, immutable bool, actors int) (*link.Link, heapdomain.Schema) {
	t.Helper()
	if actors < 1 {
		t.Fatal("bootstrap header actor denominator")
	}
	p, err := lower.Lower(lower.Source{Name: "heap_bootstrap_law.lua", Text: []byte("return 1\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "BootstrapRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Immutable: immutable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "BootstrapRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{{
			Root:       "BootstrapRoot",
			Key:        keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "self"},
			Value:      target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "BootstrapRoot"},
			Mutability: target.InitialMutable,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := &link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "bootstrap-law", Program: p}}}
	if actors > 1 {
		spec.Module.Actors = make([]linkmodule.ActorSpec, actors)
		spec.Module.ModuleCacheAliases = make([]linkmodule.ModuleCacheAliasClassSpec, actors)
		spec.Module.AnalysisRoots = make([]linkmodule.AnalysisRootSpec, actors)
		for index := 0; index < actors; index++ {
			ordinal := strconv.Itoa(index + 1)
			actor := "bootstrap-actor-" + ordinal
			instance := "bootstrap-cache-" + ordinal
			spec.Module.Actors[index] = linkmodule.ActorSpec{Name: actor}
			spec.Module.ModuleCacheAliases[index] = linkmodule.ModuleCacheAliasClassSpec{Actor: actor, Instances: []string{instance}, Representative: instance}
			spec.Module.AnalysisRoots[index] = linkmodule.AnalysisRootSpec{Name: "bootstrap-root-" + ordinal, Module: "bootstrap-law", Actor: actor, Instance: instance}
		}
	}
	linked, err := link.Seal(spec)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := heapdomain.Seal(linked)
	if !ok {
		t.Fatal("Heap schema")
	}
	return linked, schema
}

func bootstrapKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("bootstrap semantic key")
	}
	return key
}
