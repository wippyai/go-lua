package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	programartifact "github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestAuthoritySealsCanonicalRootsAndBinderSeeds(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := fixture.authority
	if authority.RootCount() < authority.SeedCount() {
		t.Fatalf("root count = %d, want at least seed count %d", authority.RootCount(), authority.SeedCount())
	}
	if authority.SeedCount() != 1 {
		t.Fatalf("seed count = %d, want one binder-authorized call base", authority.SeedCount())
	}
	seed, ok := authority.SeedAt(0)
	if !ok {
		t.Fatal("missing seed")
	}
	seedID, ok := authority.SeedID(seed)
	if !ok {
		t.Fatal("seed lost exact source identity")
	}
	root, ok := authority.SeedRoot(seed)
	if !ok {
		t.Fatal("seed lost exact root")
	}
	if indexed, ok := authority.RootIndex(root); !ok || indexed >= uint32(authority.RootCount()) {
		t.Fatal("seed root is not an authority-owned coordinate")
	}
	if rebound, ok := authority.RootForValueIdentity(seedID); !ok || rebound != root {
		t.Fatal("seed root did not retain its canonical source identity")
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

func TestAuthorityRetainsTotalOtherRuntimeSummaryAndRejectsStructuralOnlyRuntime(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := fixture.authority
	summary, ok := authority.DescriptorAt(0)
	if !ok {
		t.Fatal("missing canonical Other Runtime summary")
	}
	if inner, ok := authority.DescriptorInner(summary); ok || inner != (typeauthority.RuntimeInner{}) {
		t.Fatal("Other Runtime summary fabricated exact structure")
	}
	if name, disposition, ok := authority.DescriptorName(summary); !ok || disposition != NameOther || name != "" {
		t.Fatalf("Other Runtime summary name = %q/%v/%v", name, disposition, ok)
	}
	if incomplete, ok := New(nil, fixture.heaps); ok || incomplete != nil {
		t.Fatal("TypeValue accepted no Static occurrence authority")
	}
}

func TestCanonicalFreshRootsNeedNoAdmission(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := fixture.authority
	rootIDs := make(map[keyspace.ContentID]struct{})
	for index := 0; index < authority.RootCount(); index++ {
		root, ok := authority.RootAt(index)
		if !ok {
			t.Fatalf("RootAt(%d)", index)
		}
		if id, ok := authority.FreshRootID(root); ok {
			rootIDs[id] = struct{}{}
		}
	}
	wantAllocationRoots := 0
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		allocation, ok := fixture.heaps.KeyAt(index)
		if !ok {
			t.Fatal("malformed Heap key range")
		}
		if allocation.Kind() != heap.RootAllocation {
			continue
		}
		wantAllocationRoots++
		keyID, ok := fixture.heaps.KeyID(allocation)
		if !ok {
			t.Fatal("allocation key lost receipt identity")
		}
		if _, ok := rootIDs[keyID]; !ok {
			t.Fatal("canonical allocation root absent from immutable authority")
		}
		if _, _, _, _, fresh := allocation.FreshResultID(); fresh {
			// Fresh target results are virtual keys, not caller-admitted roots.
			continue
		}
		if _, ok := allocation.AllocationReceipt(); !ok {
			t.Fatal("non-fresh allocation root lost its receipt")
		}
	}
	if wantAllocationRoots == 0 {
		t.Fatal("fixture did not produce a canonical allocation root")
	}
}

func TestSeedValueUsesOnlyTheBinderAuthorizedRootAndDescriptor(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := fixture.authority
	seed, ok := authority.SeedAt(0)
	if !ok {
		t.Fatal("missing canonical seed")
	}
	seedID, ok := authority.SeedID(seed)
	if !ok {
		t.Fatal("seed lost canonical Value identity")
	}
	root, value, ok := authority.SeedValue(seed)
	if !ok {
		t.Fatal("seed source interpretation")
	}
	if valueRoot, ok := authority.RootForValueIdentity(seedID); !ok || valueRoot != root {
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
	other := typeValueFixtureSource(t).authority
	foreignSeed, _ := other.SeedAt(0)
	if _, _, ok := authority.SeedValue(foreignSeed); ok {
		t.Fatal("foreign seed crossed authority fence")
	}
}

func TestAuthorityReplayAndForeignFences(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	first := fixture.authority
	firstID, _ := first.SchemaID()
	firstRoot, _ := first.RootAt(0)
	firstDescriptor, _ := first.DescriptorAt(0)

	replayed := replayTypeValueLink(t, fixture)
	second := typeValueAuthority(t, replayed, fixture.contract)
	secondID, _ := second.SchemaID()
	if secondID != firstID {
		t.Fatal("artifact replay changed TypeValue authority identity")
	}
	if _, ok := second.RootValueIdentity(firstRoot); ok {
		t.Fatal("replayed authority accepted original root handle")
	}
	if _, ok := second.DescriptorInner(firstDescriptor); ok {
		t.Fatal("replayed authority accepted original descriptor handle")
	}
}

func TestAtomPowersetIsExactHomogeneousLattice(t *testing.T) {
	fixture := typeValueFixtureSource(t)
	authority := fixture.authority
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

func TestDescriptorsPreserveNameDispositionAndOwnerFences(t *testing.T) {
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
	if !stringOK {
		t.Fatalf("runtime seed names = %#v", byName)
	}
	if equal, decided := authority.StructuralEqual(stringDescriptor, stringDescriptor); !decided || !equal {
		t.Fatal("canonical primitive descriptor did not compare equal to itself")
	}
	foreignAuthority := typeValueFixtureSource(t).authority
	foreignSeed, _ := foreignAuthority.SeedAt(0)
	foreignArgument, _ := foreignAuthority.SeedDescriptor(foreignSeed)
	if _, _, ok := authority.DescriptorName(foreignArgument); ok {
		t.Fatal("foreign descriptor crossed authority fence")
	}
}

type typeValueFixture struct {
	source    *link.Link
	heaps     heap.Schema
	contract  *target.Contract
	program   *program.Program
	authority *Authority
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
	statics, heaps := sealTypeValueAuthorities(t, linked, contract)
	authority, ok := New(statics, heaps)
	if !ok {
		t.Fatal("TypeValue New rejected canonical mounted artifacts")
	}
	return &typeValueFixture{source: linked, heaps: heaps, contract: contract, program: p, authority: authority}
}

func typeValueAuthority(t testing.TB, source *link.Link, contract *target.Contract) *Authority {
	t.Helper()
	statics, heaps := sealTypeValueAuthorities(t, source, contract)
	authority, ok := New(statics, heaps)
	if !ok {
		t.Fatal("TypeValue New rejected canonical mounted artifacts")
	}
	return authority
}

func sealTypeValueAuthorities(t testing.TB, source *link.Link, contract *target.Contract) (*staticdomain.Authority, heap.Schema) {
	t.Helper()
	if source == nil || contract == nil || source.Project() == nil {
		t.Fatal("typevalue source authority")
	}
	receipt, ok := programschema.Global()
	if !ok {
		t.Fatal("program schema receipt")
	}
	projectMounts := source.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, projectMounts.Count())
	artifactProgramIDs := make([]keyspace.ContentID, projectMounts.Count())
	staticMounts := make([]staticdomain.MountedArtifact, projectMounts.Count())
	heapMounts := make([]heap.ArtifactMount, projectMounts.Count())
	valueIDs := make([]staticdomain.MountedValueID, 0)
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		published, programOK := projectMounts.Program(shard)
		module, moduleOK := source.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || published == nil || !moduleOK || !programIDOK {
			t.Fatalf("typevalue artifact mount %d", index)
		}
		artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile typevalue artifact %d: %s", index, failure.Error())
		}
		artifacts[index] = artifact
		artifactProgramIDs[index] = programID
		var heapOK bool
		heapMounts[index], heapOK = heap.NewArtifactMount(artifact, module, programID)
		if !heapOK {
			t.Fatalf("heap artifact mount %d", index)
		}
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
		for rowIndex := 0; rowIndex < artifact.StaticTypeValueCount(); rowIndex++ {
			row, rowOK := artifact.StaticTypeValueAt(rowIndex)
			if !rowOK {
				t.Fatalf("typevalue artifact row %d/%d", index, rowIndex)
			}
			value, valueOK := source.Boundary().Values().ForMountedSemantic(module, row.ID())
			valueID, valueIDOK := source.Boundary().Values().ID(value)
			if !valueOK || !valueIDOK {
				t.Fatalf("mounted TypeValue value %d/%d", index, rowIndex)
			}
			valueIDs = append(valueIDs, staticdomain.MountedValueID{ModuleID: module, SemanticID: row.ID(), ValueID: valueID})
		}
	}
	artifactRows := make([]*programartifact.Artifact, 0, len(artifacts))
	seenPrograms := make(map[keyspace.ContentID]struct{}, len(artifacts))
	for index, artifact := range artifacts {
		programID := artifactProgramIDs[index]
		if _, seen := seenPrograms[programID]; seen {
			continue
		}
		seenPrograms[programID] = struct{}{}
		artifactRows = append(artifactRows, artifact)
	}
	types, err := typeauthority.SealArtifactRows(source.ContentID(), artifactRows)
	if err != nil || types == nil {
		t.Fatalf("seal type authority: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: source.ContentID(), Target: contract, ValueIDs: valueIDs}, types, staticMounts)
	if err != nil || statics == nil {
		t.Fatalf("seal static mounts: %v", err)
	}
	heaps, failure := heap.SealWithArtifacts(source, heapMounts)
	if failure != heap.SealFailureNone || !heaps.Valid() {
		t.Fatalf("seal heap mounts: %v", failure)
	}
	return statics, heaps
}

func reflectionTypeValueAuthority(t testing.TB) *Authority {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{
		Name: "typevalue_reflection",
		Text: []byte(`
type Shape = {name: string, next: string?}
string("probe")
Shape({})
return Shape({})
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
	return typeValueAuthority(t, linked, contract)
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
