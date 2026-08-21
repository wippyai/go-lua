package fresh_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	programschema "github.com/wippyai/go-lua/analysis/schema/typecontract"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	freshdomain "github.com/wippyai/go-lua/domain/placement/fresh"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestFreshRuleUsesTargetRootsAndSealedStackSeed(t *testing.T) {
	heap, placement := freshOwnerFixture(t, "placement-fresh-owner", `
local root = {}
local result = fresh(1)
local second = fresh(2)
root.result = result
root.second = second
return root
`)
	cold, placementFragment, freshFragment := freshRuleSchema(t)
	_, _, rule := bindFresh(t, cold, placementFragment, freshFragment, placement)
	linkInventory, linkOK := freshdomain.LinkCatalog(rule)
	if !linkOK || linkInventory == nil {
		t.Fatal("fresh Link issuer")
	}
	if got, want := linkInventory.Count(), heap.FreshCount(); got != want || rule.Count() != want {
		t.Fatalf("fresh denominator=%d/%d/%d", got, rule.Count(), want)
	}

	seen := make([]identity.ContentID, 0, heap.FreshCount())
	for index := 0; index < heap.FreshCount(); index++ {
		ownerID, key, ownerOK := heap.FreshAt(index)
		linkID, linkIDOK := linkInventory.IDAt(index)
		if !ownerOK || !linkIDOK || ownerID != linkID || !ownerID.Available() {
			t.Fatalf("fresh identity %d=%v/%v/%t/%t", index, ownerID, linkID, ownerOK, linkIDOK)
		}
		if key.Kind() != heapdomain.RootAllocation {
			t.Fatalf("fresh identity %d is not a Target allocation: %v", index, key.Kind())
		}
		_, _, _, fresh := key.FreshResultID()
		if !fresh {
			t.Fatalf("fresh identity %d is not a Target fresh result", index)
		}
		resolved, resolvedOK := heap.KeyForID(ownerID)
		if !resolvedOK || resolved != key {
			t.Fatalf("fresh identity %d inverse=%v/%t, want %v", index, resolved, resolvedOK, key)
		}
		for prior, priorID := range seen {
			if priorID == ownerID {
				t.Fatalf("fresh identity %d repeats Target identity %d", index, prior)
			}
		}
		seen = append(seen, ownerID)
	}

	// The Link issuer must never widen its denominator to Boot or ordinary
	// Program allocation roots. Those roots remain present in the same Heap
	// schema, but no fresh rule row can be issued for either one.
	sawBoot, sawProgram := false, false
	for index := 0; index < heap.KeyCount(); index++ {
		key, keyOK := heap.KeyAt(index)
		if !keyOK {
			t.Fatalf("Heap key %d", index)
		}
		id, idOK := key.ContentID()
		if !idOK || !id.Available() {
			t.Fatalf("Heap key %d identity", index)
		}
		switch key.Kind() {
		case heapdomain.RootBoot:
			sawBoot = true
		case heapdomain.RootAllocation:
			_, _, _, fresh := key.FreshResultID()
			if !fresh {
				sawProgram = true
			}
		default:
			continue
		}
		for _, freshID := range seen {
			if freshID == id && key.Kind() != heapdomain.RootAllocation {
				t.Fatalf("non-Target root received a fresh identity: %v", key.Kind())
			}
		}
	}
	if !sawBoot || !sawProgram {
		t.Fatalf("fixture roots Boot=%t Program=%t", sawBoot, sawProgram)
	}
	if _, ok := linkInventory.IDAt(-1); ok {
		t.Fatal("fresh issuer accepted a negative index")
	}
	if _, ok := linkInventory.IDAt(linkInventory.Count()); ok {
		t.Fatal("fresh issuer accepted its count boundary")
	}

	issuer, issued := rule.Implementation()
	capability, capabilityOK := issuer.LinkCapability()
	if !issued || issuer == nil || !capabilityOK || !capability.Link() || capability.Activation() {
		t.Fatal("fresh Stack seed did not publish a canonical Link capability")
	}
	if resolved, resolvedOK := placementowner.ResolveRuleImplementation(issuer); !resolvedOK || resolved == nil {
		t.Fatal("fresh Stack seed issuer lost its owner proof")
	}
}

func TestFreshRuleOwnerFenceRejectsForeignAuthority(t *testing.T) {
	localHeap, localPlacement := freshOwnerFixture(t, "placement-fresh-local", `local a = fresh(1); return a`)
	foreignHeap, foreignPlacement := freshOwnerFixture(t, "placement-fresh-foreign", `local a = fresh(1); return a`)
	cold, placementFragment, freshFragment := freshRuleSchema(t)
	localBinding, localOwner, localRule := bindFresh(t, cold, placementFragment, freshFragment, localPlacement)
	_, foreignOwner, foreignRule := bindFresh(t, cold, placementFragment, freshFragment, foreignPlacement)

	if _, ok := freshdomain.BindHot(localBinding, freshFragment, foreignOwner, localPlacement); ok {
		t.Fatal("foreign Placement owner crossed the binding fence")
	}
	if _, ok := freshdomain.BindHot(localBinding, freshFragment, localOwner, foreignPlacement); ok {
		t.Fatal("foreign Placement schema crossed the owner fence")
	}
	localIssuer, localIssued := localRule.Implementation()
	foreignIssuer, foreignIssued := foreignRule.Implementation()
	if !localIssued || !foreignIssued || localIssuer == nil || foreignIssuer == nil {
		t.Fatal("owner-fenced fresh issuer")
	}
	if implementation, accepted := placementowner.ResolveRuleImplementationFor(foreignOwner, localIssuer); accepted || implementation != nil {
		t.Fatal("foreign Placement owner accepted the local fresh issuer")
	}
	if implementation, accepted := placementowner.ResolveRuleImplementationFor(foreignOwner, foreignIssuer); !accepted || implementation == nil {
		t.Fatal("foreign Placement owner rejected its own fresh issuer")
	}

	foreignID, foreignKey, foreignOK := foreignHeap.FreshAt(0)
	if !foreignOK || !foreignID.Available() || !foreignKey.Valid() {
		t.Fatal("foreign fresh owner row")
	}
	if _, accepted := localHeap.KeyForID(foreignID); accepted {
		t.Fatal("local Heap accepted a foreign Target identity")
	}
	for index := 0; index < localRule.Count(); index++ {
		id, idOK := localRule.IDAt(index)
		if !idOK || id == foreignID {
			t.Fatalf("local fresh issuer crossed foreign identity at %d", index)
		}
	}
}

func TestFreshRuleEmptyTargetDenominatorRemainsSealed(t *testing.T) {
	heap, placement := freshOwnerFixture(t, "placement-fresh-empty", `return 1`)
	if heap.FreshCount() != 0 {
		t.Fatalf("empty Target fixture FreshCount=%d", heap.FreshCount())
	}
	cold, placementFragment, freshFragment := freshRuleSchema(t)
	_, _, rule := bindFresh(t, cold, placementFragment, freshFragment, placement)
	linkInventory, linkOK := freshdomain.LinkCatalog(rule)
	if !linkOK || linkInventory == nil || rule.Count() != 0 || linkInventory.Count() != 0 {
		t.Fatal("empty fresh denominator was not retained")
	}
	if _, ok := linkInventory.IDAt(0); ok {
		t.Fatal("empty fresh issuer produced a row")
	}
	issuer, issued := rule.Implementation()
	capability, capabilityOK := issuer.LinkCapability()
	if !issued || issuer == nil || !capabilityOK || !capability.Link() || capability.Activation() {
		t.Fatal("empty fresh Stack seed did not publish a canonical Link capability")
	}
}

func bindFresh(t testing.TB, cold *engine.Schema, placementFragment *placementowner.SchemaFragment, freshFragment *freshdomain.SchemaFragment, placement placementdomain.Schema) (*engine.SchemaBinding, *placementowner.HotOwner, *freshdomain.HotRule) {
	t.Helper()
	binding := engine.NewSchemaBinding(cold)
	owner, ownerOK := placementowner.BindHot(binding, placementFragment, placement)
	rule, ruleOK := freshdomain.BindHot(binding, freshFragment, owner, placement)
	_, linkOK := engine.RegisterLinkSlot(binding, freshFragment.RuleSlot())
	if !ownerOK || !ruleOK || owner == nil || rule == nil || !linkOK || !binding.Seal() {
		t.Fatalf("fresh owner bind owner=%t rule=%t", ownerOK, ruleOK)
	}
	return binding, owner, rule
}

func freshRuleSchema(t testing.TB) (*engine.Schema, *placementowner.SchemaFragment, *freshdomain.SchemaFragment) {
	t.Helper()
	builder := engine.NewSchema()
	placementFragment, placementOK := placementowner.DeclareSchema(builder, freshSemanticKey(1), freshSemanticKey(2))
	freshFragment, freshOK := freshdomain.DeclareSchema(builder, freshSemanticKey(3), freshSemanticKey(4), placementFragment)
	cold, coldOK := builder.Seal()
	if !placementOK || !freshOK || !coldOK || cold == nil {
		t.Fatal("fresh owner cold schema")
	}
	return cold, placementFragment, freshFragment
}

func freshOwnerFixture(t testing.TB, name, source string) (heapdomain.Schema, placementdomain.Schema) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	freshOperation := freshOperationSpec()
	target, err := compiler.Seal(&declaration.Spec{
		Semantics:  domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{freshOperation},
		InitialRoots: []vocabulary.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: vocabulary.BootShapeSpec{
				Aggregate: vocabulary.BootAggregateTable,
				Value:     vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__heap_absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: freshOperation.Bindings[0]}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__heap_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__heap_absent"}},
			{Name: "fresh", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance.Directory{})
	if failure.Available() || artifact == nil {
		t.Fatalf("artifact compile: %v", failure)
	}
	structural := syntheticStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	mount, mountOK := heapdomain.NewArtifactMount(snapshot, module, programID)
	if !grammarOK || !lowered || !shardOK || !moduleOK || !programIDOK || !mountOK {
		t.Fatal("artifact mount")
	}
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{mount})
	placement, placementOK := placementdomain.NewSchema(heap)
	if heapFailure != heapdomain.SealFailureNone || !placementOK {
		t.Fatalf("Heap/Placement seal=%v/%t", heapFailure, placementOK)
	}
	return heap, placement
}

func freshSemanticKey(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xf5, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("fresh law semantic key")
	}
	return key
}

func syntheticStructuralVocabulary(t testing.TB) structure.Table {
	t.Helper()
	counts := func(category structure.Category) int {
		switch category {
		case structure.CategoryArm:
			return 8
		case structure.CategoryEvent:
			return 3
		case structure.CategoryOutcome:
			return 7
		case structure.CategoryRuntimeKind:
			return int(runtimekind.Count) - 1
		case structure.CategoryOccurrenceKind:
			return 32
		case structure.CategoryIssuanceForm:
			return 5
		case structure.CategoryIssuanceInput:
			return 4
		case structure.CategoryIssuanceStage:
			return 5
		case structure.CategoryIssuanceRequirement:
			return 2
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("placement-fresh/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("synthetic structural declarations")
	}
	builder := schema.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("synthetic structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(emptySurface{kind: kind}) {
			t.Fatalf("synthetic surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("synthetic schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("synthetic structure view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("synthetic structure table")
	}
	return table
}

type emptySurface struct{ kind schema.SurfaceKind }

func (surface emptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface emptySurface) Entries() []schema.Entry  { return nil }
func (surface emptySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func freshOperationSpec() vocabulary.OperationSpec {
	anyType, ok := programschema.NewPrimitive(programschema.PrimitiveAny)
	if !ok {
		panic("fresh law any type")
	}
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []programschema.Type{anyType}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind:         flowkind.OutcomeNormal,
			Values:       vocabulary.ValuesSpec{Fixed: []programschema.Type{anyType}, Tail: vocabulary.ValuesClosed},
			FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: programschema.FreshClassTable}},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}
