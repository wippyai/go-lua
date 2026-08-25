package bodyroute

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// endpointFixture seals the two authorities BeyondTargets is fenced to and one
// mounted call site to ask it about. The endpoint is a judgment over
// owner-issued values, so nothing short of the owners can state a law about
// it.
//
// It compiles a direct artifact grammar and a synthetic structural vocabulary
// rather than reaching the composition, for the reason the sibling package
// already records: composite wires this package into its rule inventory, so a
// test here that reached composite would close a cycle on the code under test.
type endpointFixture struct {
	effects *effectfactor.Algebra
	calls   *calldomain.Algebra
	mounted effectfactor.MountedCall
	key     calldomain.Key
}

func newEndpointFixture(t testing.TB) endpointFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "bodyroute_endpoint_law.lua", Text: []byte("local function sink(value) return value end\nsink(1)")})
	if err != nil {
		t.Fatal(err)
	}
	// Call's algebra reads the boundary's require operation, so the target
	// carries the scoped one beside this fixture's single call operation.
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{
		requireOperation,
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{endpointAnyType(t)}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "bodyroute_endpoint_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("artifact grammar")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if !shardOK || !moduleOK {
		t.Fatal("endpoint mount")
	}
	snapshot, lowered := ingress.Lower(artifact, syntheticEndpointVocabulary(t))
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !lowered || !mountOK {
		t.Fatal("endpoint artifact mount")
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()}, nil)
	if err != nil || types == nil {
		t.Fatalf("seal types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedPrograms(
		staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types,
		[]staticdomain.MountedProgram{{Program: artifact.Program(), ModuleID: module, NamespaceID: module}},
	)
	if err != nil || statics == nil {
		t.Fatalf("seal statics: %v", err)
	}
	packs, packsOK := pack.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{mount})
	effects, effectsOK := effectfactor.NewWithMountedArtifacts(linked, packs, contract,
		[]effectfactor.MountedArtifact{{ModuleKey: module, Snapshot: snapshot}})
	if !packsOK || packs == nil || !effectsOK || effects == nil {
		t.Fatal("endpoint effect authority")
	}
	calls := calltest.MustSeal(t, linked, []programmount.MountedArtifact{mount})
	site, siteOK := effects.MountedCallAt(0)
	if !siteOK {
		t.Fatal("endpoint mounted call")
	}
	// Any key this algebra issued serves: the endpoint asks what a call VALUE
	// admits, and the key is only what makes one an owner-issued value.
	key, keyOK := calls.KeyAt(0)
	if !keyOK {
		t.Fatal("endpoint call key")
	}
	return endpointFixture{effects: effects, calls: calls, mounted: site, key: key}
}

// endpointAnyType is the portable operand type the fixture's one operation
// takes. It is a real primitive because the target compiler seals the
// operation's value row and refuses a nil element.
func endpointAnyType(t testing.TB) schematype.Type {
	t.Helper()
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("portable any type")
	}
	return value
}

// TestACallThatAdmitsAnUnnamedTargetIsBeyondEnumeration is the endpoint's own
// half of the member set, and the one no emitted construction can state for
// this package.
//
// The generated set enumerates whatever the endpoint says is enumerable. Call
// publishes TWO ways of naming no closed list and answers for both with one
// predicate: a TOP value named no alternative at all, and an OPEN one named
// some and admitted there are others it cannot name. A site whose call is open
// reaches bodies its written-down targets do not include, so enumerating only
// those would answer a member set as though it were complete - a false
// negative in the reachability direction, which is the unsound one.
func TestACallThatAdmitsAnUnnamedTargetIsBeyondEnumeration(t *testing.T) {
	fixture := newEndpointFixture(t)
	open, openOK := fixture.calls.DispatchValue(fixture.key, nil, true)
	closed, closedOK := fixture.calls.DispatchValue(fixture.key, nil, false)
	if !openOK || !closedOK {
		t.Fatal("endpoint call values")
	}
	if !open.IsOpen() || open.IsTop() {
		t.Fatalf("the open specimen is not an open call value: %+v", open)
	}
	if closed.HasOpaqueAlternative() {
		t.Fatalf("the closed specimen admits an unnamed alternative: %+v", closed)
	}
	for _, item := range []struct {
		name   string
		fact   calldomain.Value
		beyond bool
	}{
		{"top", fixture.calls.Top(), true},
		{"open", open, true},
		{"closed", closed, false},
	} {
		beyond, admissible := BeyondTargets(fixture.effects, fixture.calls, fixture.mounted, item.fact)
		if !admissible {
			t.Fatalf("the %s call value was not admitted by the endpoint", item.name)
		}
		if beyond != item.beyond {
			t.Fatalf("the %s call value is beyond=%t, want %t", item.name, beyond, item.beyond)
		}
	}
}

// TestTheEndpointRefusesUnauthenticatedAuthorities keeps the fence the
// endpoint answers behind. It is the one judgment of this derivation that runs
// whether or not the call names a target, so authorities that do not belong
// together are refused here or nowhere.
func TestTheEndpointRefusesUnauthenticatedAuthorities(t *testing.T) {
	fixture := newEndpointFixture(t)
	foreign := newEndpointFixture(t)
	for _, item := range []struct {
		name    string
		effects *effectfactor.Algebra
		calls   *calldomain.Algebra
	}{
		{"no effect authority", nil, fixture.calls},
		{"no call authority", fixture.effects, nil},
		{"authorities sealed from two links", fixture.effects, foreign.calls},
	} {
		if _, admissible := BeyondTargets(item.effects, item.calls, fixture.mounted, fixture.calls.Top()); admissible {
			t.Fatalf("the endpoint admitted %s", item.name)
		}
	}
}

// syntheticEndpointVocabulary is the neutral structural projection the ingress
// lowering needs, supplied directly for the same reason the artifact grammar
// is: this package cannot reach the composition that would otherwise publish
// it.
func syntheticEndpointVocabulary(t testing.TB) structure.Table {
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
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("bodyroute/%d/%d", category, ordinal)
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
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("synthetic structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(endpointEmptySurface{kind: kind}) {
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

type endpointEmptySurface struct{ kind schema.SurfaceKind }

func (surface endpointEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface endpointEmptySurface) Entries() []schema.Entry  { return nil }
func (surface endpointEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
