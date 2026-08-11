package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func correlatedFixture(t testing.TB, text string, capability bool) (*Schema, *link.Link) {
	t.Helper()
	capabilityCount := 0
	if capability {
		capabilityCount = 1
	}
	return correlatedFixtureWithCapabilityCount(t, text, capabilityCount)
}

func correlatedFixtureWithCapabilityCount(t testing.TB, text string, capabilityCount int) (*Schema, *link.Link) {
	t.Helper()
	if capabilityCount < 0 || capabilityCount > 2 {
		t.Fatalf("unsupported capability count %d", capabilityCount)
	}
	p, err := programlower.Lower(programlower.Source{Name: "correlated_value.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contractSpec := &target.Spec{}
	if capabilityCount != 0 {
		contractSpec.InitialRoots = []target.InitialRootSpec{{
			Identity: "ValueCapabilityRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "ValueCapabilityRoot"},
			},
		}}
	}
	contract, err := target.Seal(contractSpec)
	if err != nil {
		t.Fatal(err)
	}
	spec := &link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}, Host: linkhost.Spec{}}
	if capabilityCount != 0 {
		spec.Host.ProviderCapabilities = []linkhost.ProviderCapabilitySpec{{Identity: "test-capability"}}
		spec.Host.ProviderCapabilitySeeds = []linkhost.ProviderCapabilitySeedSpec{{
			Capability:  "test-capability",
			Source:      linkhost.ProviderCapabilitySourceInitialRoot,
			InitialRoot: "ValueCapabilityRoot",
		}}
		if capabilityCount == 2 {
			spec.Host.ProviderCapabilities = append(spec.Host.ProviderCapabilities, linkhost.ProviderCapabilitySpec{Identity: "test-capability-two"})
			spec.Host.ProviderCapabilitySeeds = append(spec.Host.ProviderCapabilitySeeds, linkhost.ProviderCapabilitySeedSpec{
				Capability:  "test-capability-two",
				Source:      linkhost.ProviderCapabilitySourceInitialRoot,
				InitialRoot: "ValueCapabilityRoot",
			})
		}
	}
	linked, err := link.Seal(spec)
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema seal")
	}
	return schema, linked
}

func freshAllocationFixture(t testing.TB) (*Schema, *link.Link) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "value_fresh_roots.lua", Text: []byte("return fresh(1)")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"fresh"}}},
			Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any, typ.Any}, Tail: target.ValuesClosed},
				FreshResults: []target.FreshResultSpec{
					{Result: 0, Kind: target.FreshTable},
					{Result: 1, Kind: target.FreshFunction},
					{Result: 2, Kind: target.FreshThread},
				},
			}},
			Effects: target.RowSpec{Tail: target.RowClosed},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"fresh"}}}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
			{Name: "fresh", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema seal")
	}
	return schema, linked
}

// allocationKeys is the complete sealed Heap allocation denominator.  Link
// deliberately has no allocation projection: Program and Target creation
// provenance enter Value only through these Heap-issued coordinates.
func allocationKeys(t testing.TB, schema *Schema) []heap.Key {
	t.Helper()
	if schema == nil {
		t.Fatal("nil Value schema")
	}
	keys := make([]heap.Key, 0, schema.heap.KeyCount())
	for index := 0; index < schema.heap.KeyCount(); index++ {
		key, ok := schema.heap.KeyAt(index)
		if !ok {
			t.Fatalf("Heap KeyAt(%d)", index)
		}
		if key.Kind() == heap.RootAllocation {
			keys = append(keys, key)
		}
	}
	return keys
}

func allocationKeyAt(t testing.TB, schema *Schema, index int) heap.Key {
	t.Helper()
	keys := allocationKeys(t, schema)
	if index < 0 || index >= len(keys) {
		t.Fatalf("Heap allocation key %d", index)
	}
	return keys[index]
}

func correlatedLiteralAtom(t testing.TB, schema *Schema, want keyspace.LiteralValue) Atom {
	t.Helper()
	values := schema.source.Boundary().Values()
	for index := 0; index < values.Count(); index++ {
		value, ok := values.At(index)
		if !ok {
			continue
		}
		family, literal, ok := schema.sourceLiteral(value)
		if want.Kind == 0 {
			if !ok || family != keyspace.FamilyNil {
				continue
			}
		} else if !ok || literal.Kind != want.Kind || (want.Kind == keyspace.LiteralBool && literal.Bool != want.Bool) {
			continue
		}
		fact, ok := schema.SourceValue(value)
		if !ok {
			t.Fatal("literal source result")
		}
		atoms, ok := schema.Atoms(fact)
		if !ok || len(atoms) != 1 {
			t.Fatalf("literal source atom=%v/%t", atoms, ok)
		}
		return atoms[0]
	}
	t.Fatalf("literal Source value %v", want)
	return Atom{}
}

func mustCorrelatedSingleton(t testing.TB, schema *Schema, atom Atom) Value {
	t.Helper()
	value, ok := schema.Singleton(atom)
	if !ok {
		t.Fatal("singleton")
	}
	return value
}

func TestValueCoordinatesFenceSameContentSchemas(t *testing.T) {
	localSchema, localLink := correlatedFixture(t, "local answer = 1\nreturn answer", false)
	foreignSchema, foreignLink := correlatedFixture(t, "local answer = 1\nreturn answer", false)
	if localLink.ContentID() != foreignLink.ContentID() {
		t.Fatal("independently sealed identical Value inputs changed semantic content")
	}
	localRaw, ok := localLink.Boundary().Values().At(0)
	if !ok {
		t.Fatal("local raw Value coordinate")
	}
	foreignRaw, ok := foreignLink.Boundary().Values().At(0)
	if !ok || localRaw == foreignRaw {
		t.Fatal("same-content Links shared one ownerful Value handle")
	}
	localCoordinate, ok := localSchema.CoordinateAt(0)
	if !ok {
		t.Fatal("local Value coordinate")
	}
	foreignCoordinate, ok := foreignSchema.CoordinateAt(0)
	if !ok {
		t.Fatal("foreign Value coordinate")
	}
	if localCoordinate == foreignCoordinate {
		t.Fatal("same-content schemas issued one shared Value coordinate")
	}
	index, ok := localSchema.CoordinateIndex(localCoordinate)
	if !ok || index != 0 {
		t.Fatalf("local Value coordinate index = %d/%t, want 0/true", index, ok)
	}
	if _, ok := localSchema.CoordinateIndex(foreignCoordinate); ok {
		t.Fatal("foreign same-content Value coordinate resolved in local schema")
	}
	if !localSchema.AdmitsCoordinate(localCoordinate, localSchema.Bottom()) {
		t.Fatal("local Value coordinate did not admit an owned fact")
	}
	if localSchema.AdmitsCoordinate(foreignCoordinate, localSchema.Bottom()) {
		t.Fatal("foreign same-content Value coordinate admitted an owned fact")
	}
	if projected, available := localSchema.CoordinateFor(localRaw); !available || projected != localCoordinate {
		t.Fatal("local Link Value did not project to its existing Value coordinate")
	}
	if _, available := localSchema.CoordinateFor(foreignRaw); available {
		t.Fatal("foreign same-content Link Value crossed the local Schema fence")
	}
	if _, available := localSchema.CoordinateFor(linkboundary.Value{}); available {
		t.Fatal("zero Link Value acquired a coordinate")
	}
	values := localLink.Boundary().Values()
	for position := 0; position < values.Count(); position++ {
		raw, available := values.At(position)
		if !available {
			t.Fatalf("ValueAt(%d)", position)
		}
		projected, available := localSchema.CoordinateFor(raw)
		if !available {
			t.Fatalf("CoordinateFor(ValueAt(%d))", position)
		}
		direct, available := localSchema.CoordinateAt(position)
		if !available || projected != direct {
			t.Fatalf("coordinate projection %d changed the canonical coordinate", position)
		}
		projectedIndex, available := localSchema.CoordinateIndex(projected)
		if !available || projectedIndex != uint32(position) {
			t.Fatalf("coordinate projection index = %d/%t, want %d/true", projectedIndex, available, position)
		}
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if _, available := localSchema.CoordinateFor(localRaw); !available {
			t.Fatal("CoordinateFor stopped accepting a local Link Value")
		}
	}); allocations != 0 {
		t.Fatalf("CoordinateFor allocations = %v, want 0", allocations)
	}
}

func TestValueCoordinateProjectionIsCanonicalAcrossPermutationAndReplay(t *testing.T) {
	firstProgram, err := programlower.Lower(programlower.Source{Name: "coordinate_a.lua", Text: []byte("local a = {}; return a, 1")})
	if err != nil {
		t.Fatal(err)
	}
	secondProgram, err := programlower.Lower(programlower.Source{Name: "coordinate_b.lua", Text: []byte("local b = false; return b")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	seal := func(modules []linkproject.Module) (*Schema, *link.Link) {
		linked, sealErr := link.Seal(&link.Spec{Target: contract, Modules: modules})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		heaps, heapsOK := heap.Seal(linked)
		schema, ok := Seal(linked, heaps)
		if !heapsOK || !ok {
			t.Fatal("Value schema seal")
		}
		return schema, linked
	}
	forwardSchema, forward := seal([]linkproject.Module{{Name: "a", Program: firstProgram}, {Name: "b", Program: secondProgram}})
	permutedSchema, permuted := seal([]linkproject.Module{{Name: "b", Program: secondProgram}, {Name: "a", Program: firstProgram}})
	if forward.ContentID() != permuted.ContentID() || forward.Boundary().Values().Count() != permuted.Boundary().Values().Count() {
		t.Fatal("module authoring permutation changed the canonical Link Value universe")
	}
	assertSameCoordinates := func(leftSchema *Schema, left *link.Link, rightSchema *Schema, right *link.Link) {
		t.Helper()
		leftValues, rightValues := left.Boundary().Values(), right.Boundary().Values()
		if leftValues.Count() != rightValues.Count() {
			t.Fatal("coordinate universes have different sizes")
		}
		for position := 0; position < leftValues.Count(); position++ {
			leftValue, leftOK := leftValues.At(position)
			rightValue, rightOK := rightValues.At(position)
			if !leftOK || !rightOK {
				t.Fatalf("ValueAt(%d)", position)
			}
			leftID, leftIDOK := leftValues.ID(leftValue)
			rightID, rightIDOK := rightValues.ID(rightValue)
			if !leftIDOK || !rightIDOK || leftID != rightID {
				t.Fatalf("ValueID(%d) changed across canonical reconstruction", position)
			}
			leftCoordinate, leftCoordinateOK := leftSchema.CoordinateFor(leftValue)
			rightCoordinate, rightCoordinateOK := rightSchema.CoordinateFor(rightValue)
			leftIndex, leftIndexOK := leftSchema.CoordinateIndex(leftCoordinate)
			rightIndex, rightIndexOK := rightSchema.CoordinateIndex(rightCoordinate)
			if !leftCoordinateOK || !rightCoordinateOK || !leftIndexOK || !rightIndexOK || leftIndex != uint32(position) || rightIndex != uint32(position) {
				t.Fatalf("coordinate %d changed across canonical reconstruction", position)
			}
			if leftCoordinate == rightCoordinate {
				t.Fatalf("coordinate %d lost its Schema owner fence", position)
			}
			if _, accepted := leftSchema.CoordinateFor(rightValue); accepted {
				t.Fatalf("coordinate %d accepted a foreign Link Value", position)
			}
		}
	}
	assertSameCoordinates(forwardSchema, forward, permutedSchema, permuted)

	artifact, err := link.EncodeArtifact(forward)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := link.DecodeArtifact(artifact, contract, map[keyspace.ContentID]*program.Program{
		firstProgram.ContentID():  firstProgram,
		secondProgram.ContentID(): secondProgram,
	})
	if err != nil {
		t.Fatal(err)
	}
	replayedHeap, replayedHeapOK := heap.Seal(replayed)
	replayedSchema, ok := Seal(replayed, replayedHeap)
	if !replayedHeapOK || !ok {
		t.Fatal("replayed Value schema")
	}
	assertSameCoordinates(forwardSchema, forward, replayedSchema, replayed)
}

func TestCorrelatedValueLatticeAndFalseTableTruthiness(t *testing.T) {
	schema, _ := correlatedFixture(t, "local f = false; local a = {}; return f, a", false)
	falseAtom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false})
	root := allocationKeyAt(t, schema, 0)
	tableAtom, ok := schema.Allocation(root, materialization.Recent)
	if !ok {
		t.Fatal("recent table allocation atom")
	}
	falseValue := mustCorrelatedSingleton(t, schema, falseAtom)
	tableValue := mustCorrelatedSingleton(t, schema, tableAtom)
	joined, ok := schema.Join(falseValue, tableValue)
	if !ok {
		t.Fatal("false/table join")
	}
	if got := schema.Truthiness(tableValue); got != TruthTrue {
		t.Fatalf("table truth projection=%b, want true", got)
	}
	if got := schema.Truthiness(joined); !got.MayBeFalse() || !got.MayBeTrue() {
		t.Fatalf("false|table truth projection=%b", got)
	}
	if got := schema.Presence(joined); got != PresencePresent {
		t.Fatalf("false|table presence=%b, want present", got)
	}
	latticelaws.LawSuite[Value]{
		Name:   "correlated-value",
		Domain: schema.Domain(),
		Sample: []Value{schema.Bottom(), falseValue, tableValue, joined, schema.Top()},
	}.Run(t)
}

func TestCorrelatedValuePreservesExactReferenceAliases(t *testing.T) {
	schema, _ := correlatedFixture(t, "local a = {}; local b = {}; return a, b", false)
	leftRoot := allocationKeyAt(t, schema, 0)
	rightRoot := allocationKeyAt(t, schema, 1)
	leftAtom, ok := schema.Allocation(leftRoot, materialization.Recent)
	if !ok {
		t.Fatal("first recent table allocation atom")
	}
	rightAtom, ok := schema.Allocation(rightRoot, materialization.Recent)
	if !ok {
		t.Fatal("second recent table allocation atom")
	}
	refs := []Atom{leftAtom, rightAtom}
	left, _, ok := refs[0].Reference()
	if !ok {
		t.Fatal("first rooted atom")
	}
	right, _, ok := refs[1].Reference()
	if !ok {
		t.Fatal("second rooted atom")
	}
	leftReferenceRoot, leftOK := left.AllocationKey()
	rightReferenceRoot, rightOK := right.AllocationKey()
	if !leftOK || !rightOK || leftReferenceRoot != leftRoot || rightReferenceRoot != rightRoot || leftReferenceRoot == rightReferenceRoot {
		t.Fatal("distinct recent allocation roots collapsed")
	}
	joined, ok := schema.Join(mustCorrelatedSingleton(t, schema, refs[0]), mustCorrelatedSingleton(t, schema, refs[1]))
	if !ok {
		t.Fatal("alias join")
	}
	atoms, ok := schema.Atoms(joined)
	if !ok || len(atoms) != 2 {
		t.Fatalf("alias join alternatives=%d/%v", len(atoms), ok)
	}
}

func TestCorrelatedValuePresealsCanonicalFreshRoots(t *testing.T) {
	schema, _ := freshAllocationFixture(t)
	var fresh heap.Key
	freshCount := 0
	for index, root := range allocationKeys(t, schema) {
		if _, _, _, _, _, _, ok := root.FreshResult(); ok {
			fresh = root
			freshCount++
		}
		for _, role := range []materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary} {
			atom, ok := schema.Allocation(root, role)
			if !ok {
				t.Fatalf("presealed root %d role %d", index, role)
			}
			again, ok := schema.Allocation(root, role)
			if !ok || again != atom {
				t.Fatalf("unstable root %d role %d", index, role)
			}
			reference, gotRole, ok := atom.Reference()
			if !ok || gotRole != role {
				t.Fatalf("root %d role %d reference=%v/%d/%v", index, role, reference, gotRole, ok)
			}
			stored, ok := reference.AllocationKey()
			if !ok || stored != root {
				t.Fatalf("root %d role %d lost canonical identity", index, role)
			}
		}
	}
	if freshCount == 0 {
		t.Fatal("fixture omitted canonical fresh roots")
	}

	// The selected operation supplies a fresh root's nominal kind under its
	// guard. The candidate-independent Value universe retains only the exact
	// root and therefore makes no unguarded kind claim.
	exact, ok := schema.Allocation(fresh, materialization.Exact)
	if !ok || exact.RuntimeKinds() != referenceKinds(ReferenceOpaque) {
		t.Fatalf("fresh root kind=%b/%v, want rooted opaque reference", exact.RuntimeKinds(), ok)
	}
	recent, ok := schema.Allocation(fresh, materialization.Recent)
	if !ok {
		t.Fatal("presealed recent fresh root")
	}
	summary, ok := schema.Allocation(fresh, materialization.Summary)
	if !ok {
		t.Fatal("presealed summary fresh root")
	}
	aged, ok := schema.Age(mustCorrelatedSingleton(t, schema, recent), fresh)
	if !ok || !schema.Equal(aged, mustCorrelatedSingleton(t, schema, summary)) {
		t.Fatal("fresh root recency age")
	}

	foreignSchema, _ := freshAllocationFixture(t)
	var foreignFresh heap.Key
	foreignFound := false
	for _, root := range allocationKeys(t, foreignSchema) {
		if _, _, _, _, _, _, ok := root.FreshResult(); ok {
			foreignFresh = root
			foreignFound = true
			break
		}
	}
	if !foreignFound {
		t.Fatal("foreign fixture omitted canonical fresh root")
	}
	if _, ok := schema.Allocation(foreignFresh, materialization.Exact); ok {
		t.Fatal("foreign canonical root admitted")
	}
}

func TestCorrelatedValueCapabilityDoesNotSmearAndAgesExactly(t *testing.T) {
	schema, linked := correlatedFixture(t, "local a = {}; local f = false; return a, f", true)
	capability, ok := linked.Host().Capabilities().At(0)
	if !ok {
		t.Fatal("capability")
	}
	if declared, ok := schema.CapabilityAt(0); !ok || declared != capability || schema.CapabilityCount() != 1 {
		t.Fatal("Value capability range")
	}
	seed, ok := linked.Host().CapabilitySeeds().At(0)
	if !ok {
		t.Fatal("capability seed")
	}
	seedView, ok := schema.CapabilitySeed(seed)
	if !ok {
		t.Fatal("Value capability seed")
	}
	if sealedSeed, ok := schema.CapabilitySeedAt(0); !ok || !sealedSeed.valid() || schema.CapabilitySeedCount() != 1 {
		t.Fatal("Value capability seed range")
	}
	if seeded, ok := seedView.Capability(); !ok || seeded != capability {
		t.Fatal("capability seed lost Link handle")
	}
	if source, ok := seedView.Source(); !ok || source != linkhost.ProviderCapabilitySourceInitialRoot {
		t.Fatal("capability seed lost exact source")
	}
	root := allocationKeyAt(t, schema, 0)
	tableAtom, ok := schema.Allocation(root, materialization.Recent)
	if !ok {
		t.Fatal("recent table allocation atom")
	}
	falseAtom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false})
	withCapability, ok := schema.WithCapability(mustCorrelatedSingleton(t, schema, tableAtom), tableAtom, capability)
	if !ok {
		t.Fatal("attach recent capability")
	}
	joined, ok := schema.Join(withCapability, mustCorrelatedSingleton(t, schema, falseAtom))
	if !ok || !schema.HasCapability(joined, tableAtom, capability) || schema.HasCapability(joined, falseAtom, capability) {
		t.Fatal("capability smeared across alternatives")
	}

	recent := tableAtom
	summary, ok := schema.Allocation(root, materialization.Summary)
	if !ok {
		t.Fatal("summary allocation atom")
	}
	recurrent, ok := schema.WithCapability(mustCorrelatedSingleton(t, schema, recent), recent, capability)
	if !ok {
		t.Fatal("recent capability")
	}
	aged, ok := schema.Age(recurrent, root)
	if !ok {
		t.Fatal("age allocation recurrence")
	}
	if !schema.HasCapability(aged, summary, capability) || schema.HasCapability(aged, recent, capability) {
		t.Fatal("age did not move the exact atom/capability pair to summary")
	}
	atoms, ok := schema.Atoms(aged)
	if !ok || len(atoms) != 1 || atoms[0] != summary {
		t.Fatalf("aged image=%v/%v", atoms, ok)
	}
}

func TestCorrelatedValueSchemaFenceAndFiniteTermination(t *testing.T) {
	schema, linked := correlatedFixture(t, "local a = {}; local f = false; return a, f", true)
	other, _ := correlatedFixture(t, "local a = {}; local f = false; return a, f", true)
	atom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false})
	foreign := correlatedLiteralAtom(t, other, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false})
	local := mustCorrelatedSingleton(t, schema, atom)
	foreignValue := mustCorrelatedSingleton(t, other, foreign)
	if schema.Equal(local, foreignValue) || schema.LessOrEq(local, foreignValue) {
		t.Fatal("cross-Link correlated images were admitted")
	}
	if _, ok := schema.Join(local, foreignValue); ok {
		t.Fatal("cross-Link join was admitted")
	}

	previous := schema.Bottom()
	previousRank, ok := schema.WidenRank(previous)
	if !ok {
		t.Fatal("bottom rank")
	}
	for id := 1; id <= schema.AtomCount(); id++ {
		next, ok := schema.Singleton(Atom{schema: schema, id: uint32(id)})
		if !ok {
			t.Fatal("sealed atom")
		}
		previous, ok = schema.Widen(previous, next)
		if !ok {
			t.Fatal("finite widen")
		}
		rank, rankOK := schema.WidenRank(previous)
		if rankOK && rank >= previousRank {
			t.Fatalf("widen rank did not descend: %d -> %d", previousRank, rank)
		}
		if rankOK {
			previousRank = rank
		}
	}
	capability, ok := linked.Host().Capabilities().At(0)
	if !ok {
		t.Fatal("capability")
	}
	for id := 1; id <= schema.AtomCount(); id++ {
		atom := Atom{schema: schema, id: uint32(id)}
		previous, ok = schema.WithCapability(previous, atom, capability)
		if !ok {
			t.Fatal("finite capability expansion")
		}
	}
	if !schema.Equal(previous, schema.Top()) {
		t.Fatal("complete finite relation did not canonicalize to top")
	}
	if _, ok := schema.WidenRank(schema.Top()); ok {
		t.Fatal("top has no strict widening successor")
	}
}

func TestVisitSupportIsTotalOverTopAndEmptyOverBottom(t *testing.T) {
	schema, _ := correlatedFixture(t, "local object = {}; return nil, 1, object", false)
	other, _ := correlatedFixture(t, "local object = {}; return nil, 1, object", false)
	var bottom []Atom
	if !schema.VisitSupport(schema.Bottom(), func(atom Atom) { bottom = append(bottom, atom) }) || len(bottom) != 0 {
		t.Fatal("Bottom support must be empty")
	}
	first := make([]Atom, 0, schema.AtomCount())
	second := make([]Atom, 0, schema.AtomCount())
	if !schema.VisitSupport(schema.Top(), func(atom Atom) { first = append(first, atom) }) ||
		!schema.VisitSupport(schema.Top(), func(atom Atom) { second = append(second, atom) }) {
		t.Fatal("Top support")
	}
	if len(first) != schema.AtomCount() || len(second) != len(first) {
		t.Fatalf("Top support length=%d/%d, want %d", len(first), len(second), schema.AtomCount())
	}
	for index := range first {
		if first[index] != second[index] || !first[index].valid() {
			t.Fatal("Top support was not canonical and deterministic")
		}
	}
	if other.VisitSupport(schema.Top(), func(Atom) {}) {
		t.Fatal("foreign Value entered total support iterator")
	}
}

func TestCorrelatedValueCoordinatesRemainLinkValues(t *testing.T) {
	schema, linked := correlatedFixture(t, "local n = 1; return n", false)
	if schema.Link() != linked || schema.Link().ContentID() == (keyspace.ContentID{}) {
		t.Fatal("missing Link authority")
	}
	values := linked.Boundary().Values()
	for index := 0; index < values.Count(); index++ {
		value, ok := values.At(index)
		if !ok {
			t.Fatalf("Link coordinate %d absent", index)
		}
		if _, _, owned := values.Origin(value); !owned {
			t.Fatalf("Link coordinate %d is not ownerful", index)
		}
	}
}
