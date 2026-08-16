package link

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/relations"
)

func TestSourcePublicationsMatchTypedLinkProjectionsAndReplay(t *testing.T) {
	left, contract, main, dependency := semanticSourceFixture(t, false)
	leftRows := semanticSourcePublicationCounts(t, left)
	assertSemanticSourceTypedCounts(t, left, contract, leftRows)

	replayed := artifactAssertProjectionRoundTrip(t, left, contract, main, dependency)
	if got := semanticSourcePublicationCounts(t, replayed); !sameSemanticSourceCounts(leftRows, got) {
		t.Fatalf("artifact replay changed Link semantic-source rows: got %#v want %#v", got, leftRows)
	}

	right, _, _, _ := semanticSourceFixture(t, true)
	if right.ContentID() != left.ContentID() {
		t.Fatal("permuted Link input changed content identity")
	}
	if got := semanticSourcePublicationCounts(t, right); !sameSemanticSourceCounts(leftRows, got) {
		t.Fatalf("permuted Link input changed semantic-source rows: got %#v want %#v", got, leftRows)
	}
}

func TestSourcePublicationsRetainRequiredZeroRows(t *testing.T) {
	sealed := linked(t, contract(t), linkproject.Module{Name: "main", Program: source(t, ``)})
	rows := semanticSourcePublicationCounts(t, sealed)
	for _, definition := range []struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
	}{
		{semanticsource.OriginLinkProjectBaseApplication, 0},
		{semanticsource.OriginLinkBoundary, 0},
		{semanticsource.OriginLinkModule, 0},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal},
		{semanticsource.OriginLinkHost, 0},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget},
	} {
		key := semanticSourceDefinition(t, definition.origin, definition.facet).Token()
		if got, found := rows[key]; !found || got != 0 {
			t.Fatalf("zero Link definition %v/%v = %d/%t, want 0/true", definition.origin, definition.facet, got, found)
		}
	}
}

func semanticSourceFixture(t *testing.T, permuted bool) (*Link, *target.Contract, *program.Program, *program.Program) {
	t.Helper()
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	contract := contract(t, binding)
	main := source(t, `actor.send(1); require("dependency")`)
	dependency := source(t, `type User = { id: string }; return {}`)
	actors, aliases, roots, entries := moduleCacheDeployment(t, main, dependency)
	actor := hostGlobalRead(t, main, "actor")
	member := artifactMemberRead(t, main)
	spec := &Spec{
		Target:           contract,
		Modules:          []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module:           linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: binding}},
		Host: linkhost.Spec{
			ProviderCapabilities: []linkhost.ProviderCapabilitySpec{{Identity: "actor"}},
			ProviderCapabilitySeeds: []linkhost.ProviderCapabilitySeedSpec{{
				Capability: "actor", Source: linkhost.ProviderCapabilitySourceExposure, Module: "main", Access: actor,
			}},
			Exposures: []linkhost.HostExposureSpec{{Module: "main", Access: actor, Endpoint: "actor.send", Dispatch: linkhost.HostDispatchLookup}},
			Members:   []linkhost.HostMemberSpec{{Module: "main", Access: member, Capability: "actor", Endpoint: "actor.send", Dispatch: linkhost.HostDispatchLookup}},
		},
	}
	if permuted {
		spec.Modules[0], spec.Modules[1] = spec.Modules[1], spec.Modules[0]
		spec.Module.AnalysisRoots[0], spec.Module.AnalysisRoots[1] = spec.Module.AnalysisRoots[1], spec.Module.AnalysisRoots[0]
	}
	sealed, err := Seal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return sealed, contract, main, dependency
}

func semanticSourcePublicationCounts(t testing.TB, l *Link) map[semanticsource.Token]int {
	t.Helper()
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatalf("relation schema: %v", err)
	}
	publications, ok := l.sourcePublications(schema)
	if !ok || len(publications) != 17 {
		t.Fatalf("Link semantic-source publications = %d/%t, want 17/true", len(publications), ok)
	}
	counts := make(map[semanticsource.Token]int, len(publications))
	for _, publication := range publications {
		token := publication.Definition().Token()
		if _, duplicate := counts[token]; duplicate {
			t.Fatalf("duplicate Link publication %v", token)
		}
		counts[token] = publication.Count()
	}
	if len(counts) != 17 {
		t.Fatalf("Link semantic-source distinct definitions = %d, want 17", len(counts))
	}
	return counts
}

func assertSemanticSourceTypedCounts(t *testing.T, l *Link, contract *target.Contract, got map[semanticsource.Token]int) {
	t.Helper()
	if l == nil || contract == nil {
		t.Fatal("missing Link/Target fixture")
	}
	base, boundaries := 0, 0
	applications := l.Project().Applications()
	for applicationIndex := 0; applicationIndex < applications.Count(); applicationIndex++ {
		application, applicationOK := applications.At(applicationIndex)
		if !applicationOK {
			t.Fatalf("ApplicationAt(%d)", applicationIndex)
		}
		available := false
		for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
			operation, operationOK := contract.OperationAt(operationIndex)
			if !operationOK {
				t.Fatalf("OperationAt(%d)", operationIndex)
			}
			if l.Boundary().ApplicationOperationAvailable(contract, application, operation) {
				available = true
				boundaries++
			}
		}
		if available {
			base++
		}
	}
	for index := 0; index < l.Project().Mounts().Count(); index++ {
		if _, ok := l.Project().Mounts().At(index); !ok {
			t.Fatalf("ShardAt(%d)", index)
		}
	}
	for index := 0; index < l.Module().Cache().InstanceCount(); index++ {
		instance, ok := l.Module().Cache().InstanceAt(index)
		if !ok {
			t.Fatalf("ModuleCacheInstanceAt(%d)", index)
		}
		if representative, ok := l.Module().Cache().Representative(instance); !ok || false {
			t.Fatalf("ModuleCacheRepresentative(%v) = %v/%t", instance, representative, ok)
		}
	}
	for index := 0; index < l.Module().Roots().Count(); index++ {
		if _, ok := l.Module().Roots().At(index); !ok {
			t.Fatalf("AnalysisRootAt(%d)", index)
		}
	}
	for index := 0; index < l.Module().Coordinates().Count(); index++ {
		if _, ok := l.Module().Coordinates().At(index); !ok {
			t.Fatalf("ModuleCoordinateAt(%d)", index)
		}
	}
	for index := 0; index < l.Module().Cache().EntryCount(); index++ {
		entry, entryOK := l.Module().Cache().EntryAt(index)
		if !entryOK {
			t.Fatalf("ModuleCacheEntryAt(%d)", index)
		}
		if _, _, _, ok := l.Module().Cache().EntryMapping(entry); !ok {
			t.Fatalf("ModuleCacheEntryMapping(%d)", index)
		}
	}
	initOutcomes, initTerminals := 0, 0
	for index := 0; index < l.Module().Generations().Count(); index++ {
		generation, ok := l.Module().Generations().At(index)
		if !ok {
			t.Fatalf("ModuleInitGenerationAt(%d)", index)
		}
		for outcomeIndex := 0; outcomeIndex < l.Module().Outcomes().Count(generation); outcomeIndex++ {
			outcome, ok := l.Module().Outcomes().At(generation, outcomeIndex)
			if !ok {
				t.Fatalf("ModuleInitOutcomeAt(%d, %d)", index, outcomeIndex)
			}
			kind, ok := l.Module().Outcomes().Kind(outcome)
			if !ok {
				t.Fatalf("ModuleInitOutcomeKind(%d, %d)", index, outcomeIndex)
			}
			initOutcomes++
			if kind == flowkind.OutcomeThrow || kind == flowkind.OutcomeCancel {
				initTerminals++
			}
		}
	}
	endpoints := l.Boundary().Endpoints()
	for index := 0; index < endpoints.Count(); index++ {
		endpoint, ok := endpoints.At(index)
		if !ok {
			t.Fatalf("Boundary EndpointAt(%d)", index)
		}
		if operation, ok := endpoints.Operation(endpoint); !ok || operation == 0 {
			t.Fatalf("Boundary Endpoint Operation(%d) = %d/%t", index, operation, ok)
		}
	}
	for index := 0; index < l.Host().Exposures().Count(); index++ {
		if _, _, _, _, _, ok := l.Host().Exposures().At(index); !ok {
			t.Fatalf("HostExposureAt(%d)", index)
		}
	}
	for index := 0; index < l.Host().Globals().Count(); index++ {
		binding, ok := l.Host().Globals().At(index)
		if !ok {
			t.Fatalf("GlobalBindingAt(%d)", index)
		}
		if _, _, _, _, _, _, ok := l.Host().Globals().Mapping(binding); !ok {
			t.Fatalf("GlobalBindingMapping(%d)", index)
		}
	}
	for index := 0; index < l.Host().Members().Count(); index++ {
		if _, _, _, _, _, _, _, ok := l.Host().Members().At(index); !ok {
			t.Fatalf("HostMemberAt(%d)", index)
		}
	}

	want := map[semanticsource.Token]int{
		semanticSourceDefinition(t, semanticsource.OriginLinkProjectShardMount, 0).Token():                                 l.Project().Mounts().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkProjectBaseApplication, 0).Token():                            base,
		semanticSourceDefinition(t, semanticsource.OriginLinkBoundary, 0).Token():                                          boundaries,
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, 0).Token():                                            l.Module().Cache().EntryCount(),
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache).Token():          l.Module().Cache().InstanceCount(),
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative).Token(): l.Module().Cache().InstanceCount(),
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport).Token():      l.Module().Coordinates().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot).Token():   l.Module().Roots().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration).Token(): l.Module().Generations().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome).Token():    initOutcomes,
		semanticSourceDefinition(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal).Token():   initTerminals,
		semanticSourceDefinition(t, semanticsource.OriginLinkStatic, 0).Token():                                            l.static.Cold().SchemaContentCount(),
		semanticSourceDefinition(t, semanticsource.OriginLinkHost, 0).Token():                                              endpoints.Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure).Token():           l.Host().Exposures().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot).Token():               l.Host().Globals().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember).Token():             l.Host().Members().Count(),
		semanticSourceDefinition(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget).Token():     endpoints.Count(),
	}
	if !sameSemanticSourceCounts(want, got) {
		t.Fatalf("Link semantic-source measures = %#v, want %#v", got, want)
	}
}

func semanticSourceDefinition(t testing.TB, origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.RelationDef {
	t.Helper()
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatalf("relation schema: %v", err)
	}
	definition, ok := schema.Definition(origin, facet)
	if !ok {
		t.Fatalf("missing generated definition %v/%v", origin, facet)
	}
	return definition
}

func sameSemanticSourceCounts(left, right map[semanticsource.Token]int) bool {
	if len(left) != len(right) {
		return false
	}
	for token, count := range left {
		if right[token] != count {
			return false
		}
	}
	return true
}
