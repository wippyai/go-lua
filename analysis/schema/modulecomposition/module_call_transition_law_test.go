package modulecomposition_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	modulecomposition "github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

type moduleCallTransitionFixture struct {
	program    lawProgram
	link       identity.ContentID
	importRow  programschema.ModuleImport
	cache      modulecomposition.CacheIngress
	generation modulecomposition.InitGeneration
	transition executioncontext.Transition
	from, to   executioncontext.Context
	row        modulecomposition.ModuleCallTransition
}

func makeModuleCallTransitionFixture(t *testing.T, rootSuffix string) moduleCallTransitionFixture {
	t.Helper()
	program := makeLawProgram(t)
	linkID := lawID(t, "module-call-transition-link")
	resolved, ok := modulecomposition.NewResolvedImport(linkID, program.mount, program.request, program.targetMount.ModuleKey)
	if !ok {
		t.Fatal("resolved import")
	}
	fromContext, fromOK := executioncontext.NewContext(linkID, program.mount.ModuleKey, lawID(t, "module-call-transition-actor"), lawID(t, "module-call-transition-representative"))
	toContext, toOK := executioncontext.NewContext(linkID, program.targetMount.ModuleKey, lawID(t, "module-call-transition-actor"), lawID(t, "module-call-transition-target-representative"))
	if !fromOK || !toOK {
		t.Fatal("contexts")
	}
	cache, cacheOK := modulecomposition.NewCacheIngress(
		resolved,
		lawID(t, "module-call-transition-from-root-"+rootSuffix),
		lawID(t, "module-call-transition-to-root-"+rootSuffix),
		fromContext,
		toContext,
	)
	if !cacheOK {
		t.Fatal("cache ingress")
	}
	generation, generationOK := modulecomposition.NewInitGeneration(cache, program.targetMount, program.body)
	if !generationOK {
		t.Fatal("init generation")
	}
	transition, transitionOK := executioncontext.NewTransition(linkID, fromContext.ID(), toContext.ID())
	if !transitionOK {
		t.Fatal("transition")
	}
	importRow, importOK := program.mount.Program.ModuleImportAt(0)
	if !importOK {
		t.Fatal("module import")
	}
	row, rowOK := modulecomposition.NewModuleCallTransition(cache, generation, program.mount, importRow, transition)
	if !rowOK {
		t.Fatal("module-call transition")
	}
	return moduleCallTransitionFixture{program: program, link: linkID, importRow: importRow, cache: cache, generation: generation, transition: transition, from: fromContext, to: toContext, row: row}
}

func TestModuleCallTransitionRetainsCanonicalJoin(t *testing.T) {
	fixture := makeModuleCallTransitionFixture(t, "one")
	row := fixture.row
	if !row.Available() {
		t.Fatal("row unavailable")
	}
	if row.LinkID() != fixture.link || row.CacheIngressID() != fixture.cache.ID() ||
		row.GenerationID() != fixture.generation.ID() ||
		row.SourceModuleKey() != fixture.program.mount.ModuleKey ||
		row.SourcePointID() != lawID(t, "call-dispatch-point") ||
		row.ReturnPointID() != lawID(t, "call-effect-point") ||
		row.ArtifactID() != fixture.program.mount.ArtifactID || row.ProgramID() != fixture.program.mount.ProgramID ||
		row.ImportID() != fixture.importRow.ID() || row.CallID() != fixture.importRow.CallID() ||
		row.TransitionID() != fixture.transition.ID() || row.FromContextID() != fixture.transition.FromContextID() ||
		row.ToContextID() != fixture.transition.ToContextID() {
		t.Fatal("row lost one of its canonical joins")
	}
}

func TestModuleCallTransitionIdentityIsDeterministicAndCacheQualified(t *testing.T) {
	first := makeModuleCallTransitionFixture(t, "one")
	second := makeModuleCallTransitionFixture(t, "one")
	if first.row.ID() != second.row.ID() || first.cache.ID() != second.cache.ID() || first.transition.ID() != second.transition.ID() {
		t.Fatal("equal canonical witnesses produced different identities")
	}
	otherRoots := makeModuleCallTransitionFixture(t, "two")
	if otherRoots.transition.ID() != first.transition.ID() || otherRoots.cache.ID() == first.cache.ID() || otherRoots.row.ID() == first.row.ID() {
		t.Fatal("rows failed to remain distinct when one cache ingress changed")
	}
}

func TestModuleCallTransitionRejectsForeignRowsAndEndpoints(t *testing.T) {
	fixture := makeModuleCallTransitionFixture(t, "one")
	foreignLinkID := lawID(t, "module-call-transition-foreign-cache-link")
	foreignResolved, foreignResolvedOK := modulecomposition.NewResolvedImport(foreignLinkID, fixture.program.mount, fixture.program.request, fixture.program.targetMount.ModuleKey)
	if !foreignResolvedOK {
		t.Fatal("foreign resolved import")
	}
	foreignFrom, foreignFromOK := executioncontext.NewContext(foreignLinkID, fixture.program.mount.ModuleKey, lawID(t, "module-call-transition-actor"), lawID(t, "module-call-transition-representative"))
	foreignTo, foreignToOK := executioncontext.NewContext(foreignLinkID, fixture.program.targetMount.ModuleKey, lawID(t, "module-call-transition-actor"), lawID(t, "module-call-transition-target-representative"))
	if !foreignFromOK || !foreignToOK {
		t.Fatal("foreign cache contexts")
	}
	foreignCache, foreignCacheOK := modulecomposition.NewCacheIngress(foreignResolved, lawID(t, "module-call-transition-foreign-from-root"), lawID(t, "module-call-transition-foreign-to-root"), foreignFrom, foreignTo)
	if !foreignCacheOK {
		t.Fatal("foreign cache ingress")
	}
	if _, ok := modulecomposition.NewModuleCallTransition(foreignCache, fixture.generation, fixture.program.mount, fixture.importRow, fixture.transition); ok {
		t.Fatal("foreign cache ingress admitted")
	}

	foreignImport, importOK := programschema.NewModuleImport(
		fixture.importRow.ID(), lawID(t, "module-call-transition-foreign-call"), identity.ContentID{}, 0, 1, false,
	)
	if !importOK {
		t.Fatal("foreign import")
	}
	if _, ok := modulecomposition.NewModuleCallTransition(fixture.cache, fixture.generation, fixture.program.mount, foreignImport, fixture.transition); ok {
		t.Fatal("same-ID foreign ModuleImport admitted")
	}

	foreignMount := fixture.program.mount
	foreignMount.ModuleKey = lawID(t, "module-call-transition-foreign-source")
	if _, ok := modulecomposition.NewModuleCallTransition(fixture.cache, fixture.generation, foreignMount, fixture.importRow, fixture.transition); ok {
		t.Fatal("foreign mount admitted")
	}
	foreignProgramMount := fixture.program.mount
	foreignProgramMount.ProgramID = lawID(t, "module-call-transition-foreign-program")
	if _, ok := modulecomposition.NewModuleCallTransition(fixture.cache, fixture.generation, foreignProgramMount, fixture.importRow, fixture.transition); ok {
		t.Fatal("foreign Program identity admitted")
	}

	foreignLink, linkOK := executioncontext.NewContext(lawID(t, "module-call-transition-foreign-link"), fixture.program.targetMount.ModuleKey, lawID(t, "module-call-transition-actor"), lawID(t, "module-call-transition-target-representative"))
	if !linkOK {
		t.Fatal("foreign context")
	}
	foreignTransition, transitionOK := executioncontext.NewTransition(lawID(t, "module-call-transition-foreign-link"), fixture.transition.FromContextID(), foreignLink.ID())
	if !transitionOK {
		t.Fatal("foreign transition")
	}
	if _, ok := modulecomposition.NewModuleCallTransition(fixture.cache, fixture.generation, fixture.program.mount, fixture.importRow, foreignTransition); ok {
		t.Fatal("foreign transition admitted")
	}

	wrongEndpoint, wrongEndpointOK := executioncontext.NewTransition(fixture.link, fixture.transition.ToContextID(), fixture.transition.FromContextID())
	if !wrongEndpointOK {
		t.Fatal("wrong endpoint transition")
	}
	if _, ok := modulecomposition.NewModuleCallTransition(fixture.cache, fixture.generation, fixture.program.mount, fixture.importRow, wrongEndpoint); ok {
		t.Fatal("endpoint-inverted transition admitted")
	}

	foreignGeneration, foreignGenerationOK := modulecomposition.NewInitGeneration(
		foreignCache,
		fixture.program.targetMount,
		fixture.program.body,
	)
	if !foreignGenerationOK || foreignGeneration.ID() == fixture.generation.ID() {
		t.Fatal("foreign generation")
	}
	if _, ok := modulecomposition.NewModuleCallTransition(fixture.cache, foreignGeneration, fixture.program.mount, fixture.importRow, fixture.transition); ok {
		t.Fatal("foreign generation admitted")
	}
}
