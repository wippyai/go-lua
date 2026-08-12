package value

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	programsource "github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/target"
)

func TestSourceLiteralAtomsProjectExistingNormalizedLinkKeys(t *testing.T) {
	// A Link Key exists only when Program/Target already issued this literal as
	// an exact Lua key. This table gives every storable literal such an existing
	// authority; Value must retain that dense Key rather than recreate payload.
	schema, linked := correlatedFixture(t, "local t = {[false] = false, [true] = true, [1] = 1, [2.5] = 2.5, ['x'] = 'x'}; return nil, false, true, 1, 2.5, 'x'", false)
	want := map[keyspace.LiteralValue]bool{
		{Kind: keyspace.LiteralBool, Bool: false}:                       true,
		{Kind: keyspace.LiteralBool, Bool: true}:                        true,
		{Kind: keyspace.LiteralInteger, Integer: 1}:                     true,
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(2.5)}: true,
		{Kind: keyspace.LiteralString, String: "x"}:                     true,
	}
	seen := make(map[keyspace.LiteralValue]bool, len(want))
	seenFamilies := make(map[keyspace.Family]bool, 5)
	valuesView := linked.Boundary().Values()
	for index := 0; index < valuesView.Count(); index++ {
		value, ok := valuesView.At(index)
		if !ok {
			t.Fatal("Value")
		}
		family, literal, ok := schema.sourceLiteral(value)
		if !ok {
			continue
		}
		seenFamilies[family] = true
		if family == keyspace.FamilyNil {
			atom, atomOK := schema.Source(value)
			if !atomOK {
				t.Fatal("nil source atom")
			}
			if _, keyOK := atom.ExactKey(); keyOK {
				t.Fatal("nil acquired a Lua table-key atom")
			}
			continue
		}
		expected, normalizeOK := programsource.NormalizeExactKey(literal)
		relevant := normalizeOK && want[expected]
		if !relevant {
			continue
		}
		atom, atomOK := schema.Source(value)
		key, keyOK := atom.ExactKey()
		actual, actualOK := linked.Project().Keys().Exact(key)
		if !atomOK || !keyOK || !actualOK || !normalizeOK || actual != expected {
			t.Fatalf("source %#v lost Link key: atom=%v key=%v/%t actual=%#v/%t want=%#v", literal, atom, key, keyOK, actual, actualOK, expected)
		}
		if family == keyspace.FamilyBool {
			wantTruth := TruthTrue
			if !literal.Bool {
				wantTruth = TruthFalse
			}
			filtered, filterOK := schema.FilterPresent(mustCorrelatedSingleton(t, schema, atom))
			if atom.Truthiness() != wantTruth || schema.Truthiness(filtered) != wantTruth || !filterOK || !schema.Equal(filtered, mustCorrelatedSingleton(t, schema, atom)) {
				t.Fatalf("exact-key %#v atom did not preserve its Lua truth/presence law", literal)
			}
		}
		seen[expected] = true
	}
	for literal := range want {
		if !seen[literal] {
			t.Fatalf("missing literal source %#v", literal)
		}
	}
	for _, family := range []keyspace.Family{
		keyspace.FamilyNil,
		keyspace.FamilyBool,
		keyspace.FamilyInteger,
		keyspace.FamilyFloat,
		keyspace.FamilyString,
	} {
		if !seenFamilies[family] {
			t.Fatalf("fixture did not exercise literal family %d", family)
		}
	}
}

func TestHeapProjectionReductionsAreExactAndAllocationFree(t *testing.T) {
	schema, _ := correlatedFixture(t, "local t = {}; return nil, false, true, 1, t", false)
	nilAtom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{})
	falseAtom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false})
	trueAtom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true})
	numberAtom := correlatedLiteralAtom(t, schema, keyspace.LiteralValue{Kind: keyspace.LiteralInteger})
	root := allocationKeyAt(t, schema, 0)
	rooted, rootedOK := schema.Allocation(root, materialization.Recent)
	if !rootedOK {
		t.Fatal("tracked allocation atom")
	}

	presentInput, ok := schema.Alternatives(nilAtom, falseAtom, trueAtom)
	if !ok {
		t.Fatal("present input")
	}
	present, ok := schema.FilterPresent(presentInput)
	if !ok || schema.Presence(present) != PresencePresent || schema.Truthiness(present) != (TruthFalse|TruthTrue) {
		t.Fatalf("FilterPresent lost false or retained nil: %#v/%t", present, ok)
	}
	if got, ok := schema.ForRuntimeKinds(runtimekind.Bit(runtimekind.Number)); !ok || !schema.RuntimeKinds(got).Contains(runtimekind.Number) || schema.RuntimeKinds(got).Contains(runtimekind.Nil) {
		t.Fatalf("ForRuntimeKinds(number)=%#v/%t", got, ok)
	}

	mixed, ok := schema.Alternatives(numberAtom, rooted)
	if !ok {
		t.Fatal("stored input")
	}
	rootedOnly, ok := schema.FilterStoredExact(mixed, rooted)
	if !ok || !schema.Equal(rootedOnly, mustCorrelatedSingleton(t, schema, rooted)) {
		t.Fatal("rooted payload filter regained another alternative")
	}
	none, ok := schema.FilterStoredNone(mixed)
	if !ok || !schema.Equal(none, mustCorrelatedSingleton(t, schema, numberAtom)) {
		t.Fatal("non-reference payload filter retained a tracked alternative")
	}

	if allocations := testing.AllocsPerRun(1_000, func() {
		if _, ok := schema.FilterPresent(presentInput); !ok {
			t.Fatal("FilterPresent")
		}
		if _, ok := schema.FilterStoredExact(mixed, rooted); !ok {
			t.Fatal("FilterStoredExact")
		}
		if _, ok := schema.FilterStoredNone(mixed); !ok {
			t.Fatal("FilterStoredNone")
		}
		if _, ok := schema.ForRuntimeKinds(runtimekind.Bit(runtimekind.Number)); !ok {
			t.Fatal("ForRuntimeKinds")
		}
	}); allocations != 0 {
		t.Fatalf("Heap projection reductions allocated %f times", allocations)
	}
}

func TestTargetInitialPreservesBootAndCallableIdentity(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "target_initial.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	operation := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"admitted"}}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{operation},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "self"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "admitted"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: operation}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "denied"}, Value: target.InitialValueSpec{Kind: target.InitialValueDeniedOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"denied"}}}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "false"}, Value: target.InitialValueSpec{Kind: target.InitialValueBoolean, Boolean: false}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "one"}, Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 1}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "one_float"}, Value: target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: math.Float64bits(1)}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "signed_zero"}, Value: target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: math.Float64bits(math.Copysign(0, -1))}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "text"}, Value: target.InitialValueSpec{Kind: target.InitialValueString, String: "x"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "nan"}, Value: target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: math.Float64bits(math.NaN())}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
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
		t.Fatal("Value schema")
	}
	boot, ok := linked.Host().BootRoots().At(0)
	if !ok {
		t.Fatal("boot root")
	}
	values := make(map[target.InitialValueKind]target.InitialValue)
	byName := make(map[string]target.InitialValue)
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, key, value, _, ok := contract.InitialEntryAt(index)
		kind, kindOK := contract.InitialValueKind(value)
		if !ok || !kindOK {
			t.Fatal("initial entry")
		}
		values[kind] = value
		literal, literalOK := contract.ExactKeyValue(key)
		if literalOK && literal.Kind == keyspace.LiteralString {
			byName[literal.String] = value
		}
	}
	rootValue, ok := contract.BootShapeValue(mustBootShape(t, contract))
	if !ok {
		t.Fatal("boot shape value")
	}
	rootFact, ok := schema.TargetInitial(boot, rootValue)
	rootAtoms, atomsOK := schema.Atoms(rootFact)
	if !ok || !atomsOK || len(rootAtoms) != 1 {
		t.Fatal("initial root did not retain one rooted Value atom")
	}
	ref, _, refOK := rootAtoms[0].Reference()
	if !refOK {
		t.Fatal("initial root did not retain one rooted Value atom")
	}
	if projected, ok := ref.BootRoot(); !ok || projected != boot {
		t.Fatal("initial root changed actor-local boot identity")
	}
	for _, kind := range []target.InitialValueKind{target.InitialValueOperation, target.InitialValueDeniedOperation} {
		fact, ok := schema.TargetInitial(boot, values[kind])
		atoms, atomsOK := schema.Atoms(fact)
		if !ok || !atomsOK || len(atoms) != 1 {
			t.Fatalf("target %v lost callable identity", kind)
		}
		ref, _, refOK := atoms[0].Reference()
		seed, callable := ref.Callable()
		if !refOK || !callable {
			t.Fatalf("target %v lost callable identity", kind)
		}
		disposition, _, denied, valid := linked.Boundary().Seeds().CallableDisposition(seed)
		if !valid || (kind == target.InitialValueOperation && disposition != linkboundary.CallableAdmittedOperation) || (kind == target.InitialValueDeniedOperation && (disposition != linkboundary.CallableDeniedTarget || denied != values[kind])) {
			t.Fatalf("target %v changed callable disposition", kind)
		}
	}
	falseFact, ok := schema.TargetInitial(boot, values[target.InitialValueBoolean])
	if !ok || schema.Presence(falseFact) != PresencePresent || schema.Truthiness(falseFact) != TruthFalse {
		t.Fatal("initial false projection")
	}
	falseAtoms, falseAtomsOK := schema.Atoms(falseFact)
	if !falseAtomsOK || len(falseAtoms) != 1 {
		t.Fatal("initial false atom")
	}
	falseKey, falseKeyOK := linked.Project().Keys().ForInitial(contract, byName["false"])
	projectedFalseKey, projectedFalseKeyOK := falseAtoms[0].ExactKey()
	filteredFalse, filteredFalseOK := schema.FilterPresent(falseFact)
	if !falseKeyOK || !projectedFalseKeyOK || falseKey != projectedFalseKey || !filteredFalseOK || !schema.Equal(falseFact, filteredFalse) {
		t.Fatal("initial false did not retain its exact dynamic-key identity")
	}
	projectLiteral := func(name string) Atom {
		t.Helper()
		initial := byName[name]
		fact, ok := schema.TargetInitial(boot, initial)
		atoms, atomsOK := schema.Atoms(fact)
		if !ok || !atomsOK || len(atoms) != 1 {
			t.Fatalf("initial %q did not project one Value atom", name)
		}
		want, wantOK := linked.Project().Keys().ForInitial(contract, initial)
		got, gotOK := atoms[0].ExactKey()
		if !wantOK || !gotOK || want != got {
			t.Fatalf("initial %q lost Link exact key", name)
		}
		return atoms[0]
	}
	one := projectLiteral("one")
	if oneFloat := projectLiteral("one_float"); oneFloat != one {
		t.Fatal("Target integer 1 and float 1.0 did not share the normalized key atom")
	}
	_ = projectLiteral("signed_zero")
	_ = projectLiteral("text")
	nanFact, nanOK := schema.TargetInitial(boot, byName["nan"])
	nanAtoms, nanAtomsOK := schema.Atoms(nanFact)
	if !nanOK || !nanAtomsOK || len(nanAtoms) != 1 || nanAtoms[0].RuntimeKinds() != runtimekind.Bit(runtimekind.Number) {
		t.Fatal("Target NaN did not retain Number fallback")
	}
	if _, exact := nanAtoms[0].ExactKey(); exact {
		t.Fatal("Target NaN acquired an exact table-key atom")
	}
	if validity := nanAtoms[0].TableKeyValidity(); validity.MayBeValid() || !validity.MayBeInvalid() {
		t.Fatalf("Target NaN table-key validity=%b, want invalid only", validity)
	}
	if _, ok := schema.TargetInitial(boot, values[target.InitialValueAbsent]); ok {
		t.Fatal("absent target entry manufactured a runtime Value")
	}
}

func TestTargetInitialRebindsAliasesWithinActorAndRejectsForeignBootRoots(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "target_initial_alias.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{
			{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}},
			{Identity: "TableRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "TableRoot"}}},
		},
		InitialEntries: []target.InitialEntrySpec{{
			Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "table"},
			Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "TableRoot"}, Mutability: target.InitialMutable,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	moduleSpec := linkmodule.Spec{
		Actors: []linkmodule.ActorSpec{{Name: "actor-a"}, {Name: "actor-b"}},
		ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{
			{Actor: "actor-a", Instances: []string{"cache-a"}, Representative: "cache-a"},
			{Actor: "actor-b", Instances: []string{"cache-b"}, Representative: "cache-b"},
		},
		AnalysisRoots: []linkmodule.AnalysisRootSpec{
			{Name: "root-a", Module: "main", Actor: "actor-a", Instance: "cache-a"},
			{Name: "root-b", Module: "main", Actor: "actor-b", Instance: "cache-b"},
		},
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}, Module: moduleSpec})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, schemaOK := Seal(linked, heaps)
	if !heapsOK || !schemaOK {
		t.Fatal("Value schema")
	}
	globalRoot, globalOK := contract.InitialRootAt(0)
	tableRoot, tableOK := contract.InitialRootAt(1)
	if !globalOK || !tableOK {
		t.Fatal("initial roots")
	}
	_ = tableRoot
	var tableInitial target.InitialValue
	for index := 0; index < contract.InitialEntryCount(); index++ {
		root, key, initial, _, entryOK := contract.InitialEntryAt(index)
		literal, literalOK := contract.ExactKeyValue(key)
		if entryOK && root == globalRoot && literalOK && literal.Kind == keyspace.LiteralString && literal.String == "table" {
			tableInitial = initial
		}
	}
	if tableInitial == 0 {
		t.Fatal("table alias initial")
	}
	boots := linked.Host().BootRoots()
	globalA, globalAOK := boots.At(0)
	tableA, tableAOK := boots.At(1)
	globalB, globalBOK := boots.At(2)
	tableB, tableBOK := boots.At(3)
	if !globalAOK || !tableAOK || !globalBOK || !tableBOK {
		t.Fatal("actor-local boot roots")
	}
	actorA, mappedA, mappedAOK := boots.Mapping(globalA)
	actorB, mappedB, mappedBOK := boots.Mapping(globalB)
	_, mappedTableA, mappedTableAOK := boots.Mapping(tableA)
	_, mappedTableB, mappedTableBOK := boots.Mapping(tableB)
	if !mappedAOK || !mappedBOK || !mappedTableAOK || !mappedTableBOK || mappedA != globalRoot || mappedB != globalRoot || mappedTableA != tableRoot || mappedTableB != tableRoot || actorA == actorB || tableA == tableB {
		t.Fatal("two-actor boot-root geometry")
	}
	projectedA, projectedAOK := schema.TargetInitial(globalA, tableInitial)
	projectedAtoms, projectedAtomsOK := schema.Atoms(projectedA)
	if !projectedAOK || !projectedAtomsOK || len(projectedAtoms) != 1 {
		t.Fatal("same-actor root alias did not project")
	}
	refA, _, refAOK := projectedAtoms[0].Reference()
	projectedTableA, projectedTableAOK := refA.BootRoot()
	if !refAOK || !projectedTableAOK || projectedTableA != tableA || projectedTableA == tableB {
		t.Fatal("root alias crossed actor-local boot identity")
	}
	projectedB, projectedBOK := schema.TargetInitial(globalB, tableInitial)
	projectedBAtoms, projectedBAtomsOK := schema.Atoms(projectedB)
	if !projectedBOK || !projectedBAtomsOK || len(projectedBAtoms) != 1 {
		t.Fatal("second actor root alias did not project")
	}
	refB, _, refBOK := projectedBAtoms[0].Reference()
	projectedTableB, projectedTableBOK := refB.BootRoot()
	if !refBOK || !projectedTableBOK || projectedTableB != tableB || projectedTableB == tableA {
		t.Fatal("second actor alias did not retain its child root")
	}
	foreign, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}, Module: moduleSpec})
	if err != nil {
		t.Fatal(err)
	}
	foreignTable, foreignTableOK := foreign.Host().BootRoots().At(1)
	if !foreignTableOK {
		t.Fatal("foreign boot root")
	}
	if _, ok := schema.TargetInitial(foreignTable, tableInitial); ok {
		t.Fatal("foreign Host boot root crossed Value fence")
	}
}

func TestTargetInitialScopesRequireAsNominalLoader(t *testing.T) {
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	closed := target.OperationSpec{
		Bindings: []target.BindingSpec{require},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}
	contract, err := target.Seal(&target.Spec{
		Operations:   []target.OperationSpec{closed},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: require}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
			{Name: "require", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(programlower.Source{Name: "target_initial_require.lua", Text: []byte("return require")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, schemaOK := Seal(linked, heaps)
	if !heapsOK || !schemaOK {
		t.Fatal("Value schema")
	}
	_, initial, _, _, initialOK := contract.InitialBinding("require")
	boot, bootOK := linked.Host().BootRoots().At(0)
	if !initialOK || !bootOK {
		t.Fatal("require bootstrap geometry")
	}
	fact, factOK := schema.TargetInitial(boot, initial)
	atoms, atomsOK := schema.Atoms(fact)
	if !factOK || !atomsOK || len(atoms) != 1 || !atoms[0].RuntimeKinds().Contains(runtimekind.Function) {
		t.Fatal("scoped require did not retain Function identity")
	}
	ref, _, refOK := atoms[0].Reference()
	operation, scoped := ref.ScopedLoader()
	if !refOK || !scoped || operation == 0 || operation != requireOperation(t, contract) {
		t.Fatal("scoped require did not retain nominal loader identity")
	}
	if _, callable := ref.Callable(); callable {
		t.Fatal("scoped require fabricated a global callable seed")
	}
	unknown, unknownOK := schema.FilterStoredUnknown(fact)
	if !unknownOK || !schema.Equal(unknown, fact) {
		t.Fatal("scoped loader marker was lost from stored unknown projection")
	}
	aliased, aliasedOK := schema.TargetInitial(boot, initial)
	if !aliasedOK || !schema.Equal(aliased, fact) {
		t.Fatal("scoped loader alias changed nominal identity")
	}
}

func requireOperation(t testing.TB, contract *target.Contract) target.Operation {
	t.Helper()
	operation, ok := contract.InitialValueOperation(mustInitialBinding(t, contract, "require"))
	if !ok {
		t.Fatal("require operation")
	}
	return operation
}

func mustInitialBinding(t testing.TB, contract *target.Contract, name string) target.InitialValue {
	t.Helper()
	_, value, _, _, ok := contract.InitialBinding(name)
	if !ok {
		t.Fatalf("initial binding %q", name)
	}
	return value
}

func mustBootShape(t testing.TB, contract *target.Contract) target.BootShape {
	t.Helper()
	root, ok := contract.InitialRootAt(0)
	if !ok {
		t.Fatal("initial root")
	}
	shape, ok := contract.InitialRootBootShape(root)
	if !ok {
		t.Fatal("boot shape")
	}
	return shape
}
