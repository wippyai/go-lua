package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestAuthoritySealsCanonicalRootsAndBinderSeeds(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := typeValueAuthorityWithHeap(t, fixture.source, fixture.heaps)
	wantRoots := fixture.source.Boundary().Values().Count()
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		root, ok := fixture.heaps.KeyAt(index)
		if !ok {
			t.Fatal("malformed Heap Key range")
		}
		if root.Kind() == heap.RootAllocation {
			if _, _, _, _, _, _, fresh := root.FreshResult(); fresh {
				wantRoots++
			}
		}
	}
	if authority.RootCount() != wantRoots {
		t.Fatalf("root count = %d, want %d", authority.RootCount(), wantRoots)
	}
	if authority.SeedCount() != 1 {
		t.Fatalf("seed count = %d, want one binder-authorized call base", authority.SeedCount())
	}
	seed, ok := authority.SeedAt(0)
	if !ok {
		t.Fatal("missing seed")
	}
	root, ok := authority.SeedRoot(seed)
	if !ok {
		t.Fatal("seed lost exact root")
	}
	runtime, ok := authority.RootValue(root)
	if !ok {
		t.Fatal("seed root became a fresh root")
	}
	want, _ := authority.SeedSource(seed)
	if runtime != want {
		t.Fatal("seed root changed source identity")
	}
	descriptor, ok := authority.SeedDescriptor(seed)
	if !ok {
		t.Fatal("seed descriptor unavailable")
	}
	if name, disposition, ok := authority.DescriptorName(descriptor); !ok || disposition != NameExact || name != "string" {
		t.Fatalf("descriptor name = %q/%v/%v", name, disposition, ok)
	}
	if form, ok := authority.Form(descriptor); !ok || form != typeauthority.FormString {
		t.Fatalf("descriptor form = %v/%v", form, ok)
	}
	if _, ok := authority.SchemaID(); !ok {
		t.Fatal("authority lacks cold identity")
	}
}

func TestAuthorityConsumesTotalOtherRuntimeDispositionAndRejectsStructuralOnlyRuntime(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "typevalue_other.lua", Text: []byte(`
local subject = 1
type Dynamic = typeof(subject)
return Dynamic(subject)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "typevalue_other", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, ok := heap.Seal(source)
	if !ok {
		t.Fatal("Heap seal")
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	authority, ok := New(statics, heaps)
	if !ok || authority == nil || authority.SeedCount() != 1 {
		t.Fatal("total Other Runtime disposition was treated as a missing binding")
	}
	seed, _ := authority.SeedAt(0)
	descriptor, ok := authority.SeedDescriptor(seed)
	if !ok {
		t.Fatal("Other Runtime seed descriptor")
	}
	if inner, ok := authority.DescriptorInner(descriptor); ok || inner != (typeauthority.RuntimeInner{}) {
		t.Fatal("Other Runtime seed fabricated exact structure")
	}
	if name, disposition, ok := authority.DescriptorName(descriptor); !ok || disposition != NameExact || name != "Dynamic" {
		t.Fatalf("Other Runtime seed name = %q/%v/%v", name, disposition, ok)
	}

	if incomplete, ok := New(nil, heaps); ok || incomplete != nil {
		t.Fatal("TypeValue accepted no Static occurrence authority")
	}
}

func TestCanonicalFreshRootsNeedNoAdmission(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := typeValueAuthorityWithHeap(t, fixture.source, fixture.heaps)
	seenFresh := make(map[uint32]struct{})
	wantFresh := 0
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		allocation, ok := fixture.heaps.KeyAt(index)
		if !ok {
			t.Fatal("malformed Heap Key range")
		}
		if allocation.Kind() != heap.RootAllocation {
			continue
		}
		if _, _, _, _, _, _, fresh := allocation.FreshResult(); !fresh {
			continue
		}
		wantFresh++
		root, ok := authority.RootForHeapKey(allocation)
		if !ok {
			t.Fatal("canonical fresh root absent from immutable authority")
		}
		ordinal, _ := authority.RootIndex(root)
		seenFresh[ordinal] = struct{}{}
		if rebound, ok := authority.FreshRoot(root); !ok {
			t.Fatal("fresh root lost Heap authority")
		} else if _, ok := fixture.heaps.KeyID(rebound); !ok {
			t.Fatal("fresh root crossed Heap fence")
		}
	}
	if len(seenFresh) != wantFresh || wantFresh < 2 {
		t.Fatalf("fresh roots = %d, want complete canonical range %d", len(seenFresh), wantFresh)
	}
}

func TestSeedValueUsesOnlyTheBinderAuthorizedRootAndDescriptor(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := typeValueAuthorityWithHeap(t, fixture.source, fixture.heaps)
	seed, ok := authority.SeedAt(0)
	if !ok {
		t.Fatal("missing canonical seed")
	}
	seedID, ok := authority.SeedID(seed)
	if !ok {
		t.Fatal("seed lost canonical Value identity")
	}
	linkValue, ok := authority.SeedSource(seed)
	if !ok {
		t.Fatal("Link seed Value")
	}
	wantID, ok := fixture.source.Boundary().Values().ID(linkValue)
	if !ok || seedID != wantID {
		t.Fatal("seed identity did not remain the exact Link Value identity")
	}
	root, value, ok := authority.SeedValue(seed)
	if !ok {
		t.Fatal("seed source interpretation")
	}
	wantRoot, _ := authority.SeedRoot(seed)
	if root != wantRoot {
		t.Fatal("seed interpretation changed Link root")
	}
	if count, exact := authority.AtomCountIn(value); !exact || count != 1 {
		t.Fatalf("seed value = %d/%v atoms, want singleton", count, exact)
	}
	atom, _ := authority.AtomAt(value, 0)
	descriptor, ok := authority.ObjectDescriptor(atom)
	if !ok {
		t.Fatal("seed did not produce an Object atom")
	}
	wantDescriptor, _ := authority.SeedDescriptor(seed)
	if descriptor != wantDescriptor {
		t.Fatal("seed interpretation changed sealed descriptor")
	}
	other := typeValueAuthority(t, typeValueFixtureSource(t).source)
	foreignSeed, _ := other.SeedAt(0)
	if _, _, ok := authority.SeedValue(foreignSeed); ok {
		t.Fatal("foreign seed crossed authority fence")
	}
}

func TestAuthorityReplayAndForeignFences(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	first := typeValueAuthorityWithHeap(t, fixture.source, fixture.heaps)
	firstID, _ := first.SchemaID()
	firstRoot, _ := first.RootAt(0)
	firstDescriptor, _ := first.DescriptorAt(0)

	replayed := replayTypeValueLink(t, fixture)
	second := typeValueAuthority(t, replayed)
	secondID, _ := second.SchemaID()
	if secondID != firstID {
		t.Fatal("artifact replay changed TypeValue authority identity")
	}
	if _, ok := second.RootValue(firstRoot); ok {
		t.Fatal("replayed authority accepted original root handle")
	}
	if _, ok := second.DescriptorInner(firstDescriptor); ok {
		t.Fatal("replayed authority accepted original descriptor handle")
	}
	foreignValue, _ := replayed.Boundary().Values().At(0)
	if _, ok := first.RootForValue(foreignValue); ok {
		t.Fatal("foreign Link Value crossed authority fence")
	}
}

func TestAtomPowersetIsExactHomogeneousLattice(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := typeValueAuthorityWithHeap(t, fixture.source, fixture.heaps)
	seed, _ := authority.SeedAt(0)
	descriptor, _ := authority.SeedDescriptor(seed)
	root, _ := authority.SeedRoot(seed)
	object, ok := authority.Object(descriptor)
	if !ok {
		t.Fatal("Object atom")
	}
	method, ok := authority.Method(SelectorKind, root)
	if !ok {
		t.Fatal("Method atom")
	}
	unknown, _ := authority.UnknownCursor()
	iterator, ok := authority.Iterator(SelectorFields, root, unknown)
	if !ok {
		t.Fatal("Iter atom")
	}
	objectValue, _ := authority.Singleton(object)
	methodValue, _ := authority.Singleton(method)
	iteratorValue, _ := authority.Singleton(iterator)
	joined := authority.Join(objectValue, methodValue)
	joined = authority.Join(joined, iteratorValue)
	if count, ok := authority.AtomCountIn(joined); !ok || count != 3 {
		t.Fatalf("joined atom count = %d/%v", count, ok)
	}
	if !authority.LessOrEq(objectValue, joined) || !authority.LessOrEq(methodValue, joined) || !authority.LessOrEq(iteratorValue, joined) {
		t.Fatal("union lost a singleton")
	}
	if !authority.Equal(joined, authority.Join(iteratorValue, authority.Join(methodValue, objectValue))) {
		t.Fatal("Join is not associative/commutative")
	}
	if !authority.Equal(joined, authority.Join(joined, joined)) {
		t.Fatal("Join is not idempotent")
	}
	if authority.WidenRank(authority.Bottom()) <= authority.WidenRank(joined) || authority.WidenRank(joined) <= authority.WidenRank(authority.Top()) {
		t.Fatal("powerset widening rank does not strictly descend")
	}
	if !authority.LessOrEq(joined, authority.Top()) || !authority.LessOrEq(authority.Bottom(), joined) {
		t.Fatal("Bottom/Top order")
	}
}

func TestDescriptorsPreserveReflectionAndExistingGenericConstruction(t *testing.T) {
	authority := reflectionTypeValueAuthority(t)
	byName := make(map[string]Descriptor)
	for index := 0; index < authority.SeedCount(); index++ {
		seed, _ := authority.SeedAt(index)
		descriptor, _ := authority.SeedDescriptor(seed)
		name, disposition, ok := authority.DescriptorName(descriptor)
		if ok && disposition == NameExact {
			byName[name] = descriptor
		}
	}
	stringDescriptor, stringOK := byName["string"]
	shape, shapeOK := byName["Shape"]
	box, boxOK := byName["Box"]
	pair, pairOK := byName["Pair"]
	if !stringOK || !shapeOK || !boxOK || !pairOK {
		t.Fatalf("runtime seed names = %#v", byName)
	}
	if length, compatible, decided := authority.IteratorLength(shape, SelectorFields); length != 2 || !compatible || !decided {
		t.Fatalf("Shape fields = %d/%v/%v", length, compatible, decided)
	}
	for index, want := range []string{"name", "next"} {
		name, named, child, present, ok := authority.IteratorEntry(shape, SelectorFields, index)
		if !ok || !named || !present || name != want {
			t.Fatalf("Shape field %d = %q/%v/%v/%v", index, name, named, present, ok)
		}
		if _, ok := authority.DescriptorInner(child); !ok {
			t.Fatalf("Shape field %d lost exact Runtime child", index)
		}
	}
	if child, present, found, decided := authority.RecordField(shape, "next"); !decided || !found || !present {
		t.Fatalf("exact Shape.next = %v/%v/%v", present, found, decided)
	} else if form, ok := authority.Form(child); !ok || form != typeauthority.FormOptional {
		t.Fatalf("exact Shape.next form = %v/%v", form, ok)
	}
	if _, present, found, decided := authority.RecordField(shape, "missing"); !decided || found || present {
		t.Fatalf("missing Shape field = %v/%v/%v", present, found, decided)
	}
	if length, compatible, decided := authority.IteratorLength(box, SelectorTparams); length != 1 || !compatible || !decided {
		t.Fatalf("Box tparams = %d/%v/%v", length, compatible, decided)
	}
	name, named, constraint, present, ok := authority.IteratorEntry(box, SelectorTparams, 0)
	if !ok || !named || !present || name != "T" {
		t.Fatalf("Box tparam = %q/%v/%v/%v", name, named, present, ok)
	}
	if equal, decided := authority.StructuralEqual(constraint, stringDescriptor); !decided || !equal {
		t.Fatal("Box constraint did not reuse canonical string authority")
	}
	arguments := []Descriptor{stringDescriptor}
	instantiated, exact, ok := authority.Instantiate(box, arguments)
	if !ok || !exact {
		t.Fatalf("Box<string> = exact:%v ok:%v", exact, ok)
	}
	if form, ok := authority.Form(instantiated); !ok || form != typeauthority.FormInstantiated {
		t.Fatalf("Box<string> form = %v/%v", form, ok)
	}
	if name, disposition, ok := authority.DescriptorName(instantiated); !ok || disposition != NameExact || name != "Box" {
		t.Fatalf("Box<string> name = %q/%v/%v", name, disposition, ok)
	}
	if resolver, exactResolver, otherResolver := authority.DescriptorResolver(instantiated); resolver != (linkstatic.Resolver{}) || exactResolver || otherResolver {
		t.Fatal("generic construction retained source resolver")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _, _ = authority.Instantiate(box, arguments)
	}); allocations != 0 {
		t.Fatalf("hot Instantiate allocations = %v", allocations)
	}
	foreignAuthority := typeValueAuthority(t, typeValueFixtureSource(t).source)
	foreignSeed, _ := foreignAuthority.SeedAt(0)
	foreignArgument, _ := foreignAuthority.SeedDescriptor(foreignSeed)
	if _, _, ok := authority.Instantiate(box, []Descriptor{foreignArgument}); ok {
		t.Fatal("generic construction accepted a foreign descriptor")
	}
	if _, _, ok := authority.Instantiate(pair, []Descriptor{shape, foreignArgument}); ok {
		t.Fatal("early local trie miss concealed a foreign tail argument")
	}
	if _, _, ok := authority.Instantiate(shape, []Descriptor{foreignArgument}); ok {
		t.Fatal("nongeneric rejection concealed a foreign argument")
	}
	if _, _, ok := authority.Instantiate(box, []Descriptor{stringDescriptor, foreignArgument}); ok {
		t.Fatal("wrong-arity rejection concealed a foreign argument")
	}
}

type typeValueFixture struct {
	source   *link.Link
	heaps    heap.Schema
	contract *target.Contract
	program  *program.Program
}

func typeValueFixtureSource(t testing.TB) *typeValueFixture {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{
		{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"op"}}},
			Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{
				Kind:   kind.OutcomeNormal,
				Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesClosed},
				FreshResults: []target.FreshResultSpec{
					{Result: 0, Kind: target.FreshReflection},
					{Result: 1, Kind: target.FreshFunction},
				},
				Produced: []target.ProducedSpec{{
					Result: 1, Operation: target.SpecRef(2),
					Captures: []target.CaptureSpec{{Kind: target.CaptureTypeValueFormal, Ordinal: 0}},
				}},
			}},
			Effects: target.RowSpec{Tail: target.RowClosed},
		},
		{
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(programlower.Source{
		Name: "typevalue_source",
		Text: []byte(`local invoke; string("value"); return invoke("captured")`),
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "typevalue_source", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	if !heapsOK {
		t.Fatal("Heap seal")
	}
	return &typeValueFixture{source: linked, heaps: heaps, contract: contract, program: p}
}

func typeValueAuthority(t testing.TB, source *link.Link) *Authority {
	t.Helper()
	heaps, heapsOK := heap.Seal(source)
	if !heapsOK {
		t.Fatal("Heap seal")
	}
	return typeValueAuthorityWithHeap(t, source, heaps)
}

func typeValueAuthorityWithHeap(t testing.TB, source *link.Link, heaps heap.Schema) *Authority {
	t.Helper()
	authority, ok := New(staticAuthority(t, source), heaps)
	if !ok {
		t.Fatal("TypeValue New rejected canonical Link/Runtime")
	}
	return authority
}

func staticAuthority(t testing.TB, source *link.Link) *staticdomain.Authority {
	t.Helper()
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("typeauthority Seal")
	}
	statics, _, err := staticdomain.Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	return statics
}

func reflectionTypeValueAuthority(t testing.TB) *Authority {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{
		Name: "typevalue_reflection",
		Text: []byte(`
type Shape = {name: string, next: string?}
type Box<T: string> = {value: T}
type Boxed = Box<string>
type Pair<L: string, R: string> = {left: L, right: R}
type Paired = Pair<string, string>
string("probe")
Shape({})
return Box(string), Pair(string, string)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__typevalue_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "typevalue_reflection", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return typeValueAuthority(t, linked)
}

func replayTypeValueLink(t testing.TB, fixture *typeValueFixture) *link.Link {
	t.Helper()
	data, err := link.EncodeArtifact(fixture.source)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := link.DecodeArtifact(data, fixture.contract, map[keyspace.ContentID]*program.Program{
		fixture.program.ContentID(): fixture.program,
	})
	if err != nil {
		t.Fatal(err)
	}
	return replayed
}
