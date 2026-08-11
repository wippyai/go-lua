package link

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/target"
)

// A literal require inside static containment belongs exclusively to the
// sealed namespace resolver. It must not force the runtime loader operation
// into a contract that has no executable require ingress; the paired live
// source proves that the same exact spelling remains rejected in that case.
func TestStaticRequireDoesNotDemandRuntimeAuthority(t *testing.T) {
	provider := source(t, `
type User = { id: string }
`)
	staticOnly := source(t, `
type Snapshot = typeof(require("dependency"))
`)
	withoutRequire := testBootContract(t, []target.OperationSpec(nil), staticOnly, provider)
	sealed, err := Seal(&Spec{Target: withoutRequire, Modules: []linkproject.Module{
		{Name: "main", Program: staticOnly},
		{Name: "dependency", Program: provider},
	}})
	if err != nil {
		t.Fatalf("static-only require rejected without runtime authority: %v", err)
	}
	shard := onlyProjectShardFor(t, sealed, staticOnly)
	if sealed.Project().Applications().Count() != 0 {
		t.Fatalf("static require emitted runtime applications: %d", sealed.Project().Applications().Count())
	}
	importTerm, ok := staticOnly.Module().ImportAt(0)
	if !ok {
		t.Fatal("static require lost Import occurrence")
	}
	if _, ok := sealed.Static().Resolutions().ForImport(shard, importTerm.Term); !ok {
		t.Fatal("static require lost namespace resolution")
	}
	live := source(t, `require("dependency")`)
	liveContract := testBootContract(t, []target.OperationSpec(nil), live, provider)
	_, err = Seal(&Spec{Target: liveContract, Modules: []linkproject.Module{
		{Name: "main", Program: live},
		{Name: "dependency", Program: provider},
	}})
	if err == nil || !strings.Contains(err.Error(), "require has no target authority") {
		t.Fatalf("live require error = %v, want missing runtime authority", err)
	}
}

func TestLinkModuleAuthorityIsDrivenOnlyByCanonicalProgramImports(t *testing.T) {
	provider := source(t, `return {}`)
	cases := []struct {
		name       string
		text       string
		wantImport int
	}{
		{name: "aliased loader", text: `local loader = require; loader("dependency")`},
		{name: "conditional read", text: `if require then end`},
		{name: "global write", text: `require = function() end`},
		{name: "static import", text: `type Snapshot = typeof(require("dependency"))`},
		{name: "dead import", text: `goto done; require("dependency"); ::done::`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			main := source(t, test.text)
			withoutRequire := testBootContract(t, []target.OperationSpec(nil), main, provider)
			modules := []linkproject.Module{
				{Name: "main", Program: main},
				{Name: "dependency", Program: provider},
			}
			sealed, err := Seal(&Spec{Target: withoutRequire, Modules: modules})
			if err != nil {
				t.Fatalf("ordinary/non-executable require entered Link authority: %v", err)
			}
			if got := sealed.Project().Applications().Imports().Count(); got != test.wantImport {
				t.Fatalf("executable Program Import count = %d, want %d", got, test.wantImport)
			}
			permutedModules := append([]linkproject.Module(nil), modules...)
			reverseModules(permutedModules)
			permuted, err := Seal(&Spec{Target: withoutRequire, Modules: permutedModules})
			if err != nil || permuted.ContentID() != sealed.ContentID() {
				t.Fatalf("module permutation changed Link authority: %v/%v", err, permuted != nil)
			}
			replayed := artifactAssertProjectionRoundTrip(t, sealed, withoutRequire, main, provider)
			if got := replayed.Project().Applications().Imports().Count(); got != test.wantImport {
				t.Fatalf("replayed executable Program Import count = %d, want %d", got, test.wantImport)
			}
		})
	}

	executable := source(t, `require("dependency")`)
	withoutRequire := testBootContract(t, []target.OperationSpec(nil), executable, provider)
	if _, err := Seal(&Spec{Target: withoutRequire, Modules: []linkproject.Module{{Name: "main", Program: executable}, {Name: "dependency", Program: provider}}}); err == nil || !strings.Contains(err.Error(), "require has no target authority") {
		t.Fatalf("executable Import without Target authority = %v, want missing runtime require authority", err)
	}

	withRequire := contract(t)
	actors, aliases, roots, entries := moduleCacheDeployment(t, executable, provider)
	runtimeModules := []linkproject.Module{{Name: "main", Program: executable}, {Name: "dependency", Program: provider}}
	runtime, err := Seal(&Spec{Target: withRequire, Modules: runtimeModules, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}})
	if err != nil {
		t.Fatal(err)
	}
	imports := runtime.Project().Applications().Imports()
	application, ok := imports.At(0)
	shard, occurrence, _, importOK := runtime.Project().Applications().Import(application)
	importTerm, termOK := executable.Module().ImportAt(0)
	if !ok || !importOK || !termOK || shard == (linkproject.Shard{}) || occurrence != importTerm.Term {
		t.Fatal("executable canonical Program Import did not enter Link authority")
	}
	runtimePermutedModules := append([]linkproject.Module(nil), runtimeModules...)
	reverseModules(runtimePermutedModules)
	runtimePermuted, err := Seal(&Spec{Target: withRequire, Modules: runtimePermutedModules, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}})
	if err != nil || runtimePermuted.ContentID() != runtime.ContentID() {
		t.Fatalf("runtime Import permutation changed Link authority: %v/%v", err, runtimePermuted != nil)
	}
	replayed := artifactAssertProjectionRoundTrip(t, runtime, withRequire, executable, provider)
	if got := replayed.Project().Applications().Imports().Count(); got != 1 {
		t.Fatalf("replayed executable Program Import count = %d, want one", got)
	}
}

func TestSourceDeadRequireDoesNotDemandRuntimeAuthority(t *testing.T) {
	provider := source(t, `return {}`)
	main := source(t, `goto done; require("dependency"); ::done::`)
	withoutRequire := testBootContract(t, []target.OperationSpec(nil), main, provider)
	if _, err := Seal(&Spec{Target: withoutRequire, Modules: []linkproject.Module{
		{Name: "main", Program: main},
		{Name: "dependency", Program: provider},
	}}); err != nil {
		t.Fatalf("source-dead require rejected without runtime authority: %v", err)
	}
}

func TestStaticLiteralRequireResolvesSealedExportNamespace(t *testing.T) {
	provider := source(t, `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	consumer := source(t, `
local API = require("dependency")
type Subject = API.Schema.User
require("missing")
`)
	contract := contract(t)
	actors, aliases, roots, entries := moduleCacheDeployment(t, consumer, provider)
	sealed, err := Seal(&Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: consumer}, {Name: "dependency", Program: provider}}, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}})
	if err != nil {
		t.Fatal(err)
	}
	consumerShard := onlyProjectShardFor(t, sealed, consumer)
	providerShard := onlyProjectShardFor(t, sealed, provider)

	if got := sealed.Static().Namespaces().Count(); got != 2 {
		t.Fatalf("namespace count = %d, want two module namespaces", got)
	}
	providerNamespace := staticNamespaceForShard(t, sealed, providerShard)
	if content, ok := sealed.Static().Namespaces().ContentID(providerNamespace); !ok || !content.Available() {
		t.Fatal("provider namespace has no sealed content identity")
	}
	resolver, ok := sealed.Static().Namespaces().Resolver(providerNamespace)
	if !ok {
		t.Fatal("provider namespace has no resolver authority")
	}
	if namespace, ok := sealed.Static().Namespaces().Namespace(resolver); !ok || namespace != providerNamespace {
		t.Fatalf("resolver namespace = %v/%v, want %v", namespace, ok, providerNamespace)
	}
	if content, ok := sealed.Static().Namespaces().ResolverContentID(resolver); !ok || !content.Available() {
		t.Fatal("resolver has no content-addressed authority")
	}
	if got := sealed.Static().Namespaces().ExportCount(providerNamespace); got != 1 {
		t.Fatalf("provider export count = %d, want one type publication", got)
	}
	expression, ok := sealed.Static().Namespaces().ExportExpression(providerNamespace, 0)
	reference, referenceOK := sealed.Static().Expressions().Reference(expression)
	if !ok || !referenceOK || reference.Term() == 0 {
		t.Fatalf("provider export expression = %v/%v", expression, ok)
	}
	path, ok := sealed.Static().Namespaces().ExportPath(providerNamespace, 0, nil)
	if !ok || len(path) != 2 || staticKeyText(t, provider, path[0]) != "Schema" || staticKeyText(t, provider, path[1]) != "User" {
		t.Fatalf("provider export path = %v/%v", path, ok)
	}

	resolvedImport, unresolvedImport := staticImportsByLiteral(t, consumer, "dependency", "missing")
	resolved := staticResolutionForImport(t, sealed, consumerShard, resolvedImport)
	if namespace, ok := sealed.Static().Resolutions().Namespace(resolved); !ok || namespace != providerNamespace {
		t.Fatalf("dependency namespace = %v/%v, want %v", namespace, ok, providerNamespace)
	}
	importRow, ok := consumer.Module().Import(resolvedImport)
	importAlias := importRow.Alias
	if !ok || importAlias == 0 {
		t.Fatal("dependency Import lost its direct local alias")
	}
	if alias, ok := sealed.Static().Resolutions().Alias(resolved); !ok || alias != importAlias {
		t.Fatalf("dependency alias = %v/%v, want %v", alias, ok, importAlias)
	}
	if shard, item, call, literal, ok := sealed.Static().Resolutions().Source(resolved); !ok || shard != consumerShard || item != resolvedImport || call == 0 || literal == 0 {
		t.Fatalf("dependency source = %v/%v/%v/%v/%v", shard, item, call, literal, ok)
	}

	unresolved := staticResolutionForImport(t, sealed, consumerShard, unresolvedImport)
	if disposition, ok := sealed.Static().Resolutions().Disposition(unresolved); !ok || disposition != linkstatic.ResolutionUnresolved {
		t.Fatalf("unresolved import disposition = %v/%v", disposition, ok)
	}
	if _, ok := sealed.Static().Resolutions().Namespace(unresolved); ok {
		t.Fatal("unresolved import acquired a static namespace")
	}
	if shard, item, call, literal, ok := sealed.Static().Resolutions().Source(unresolved); !ok || shard != consumerShard || item != unresolvedImport || call == 0 || literal == 0 {
		t.Fatalf("unresolved source = %v/%v/%v/%v/%v", shard, item, call, literal, ok)
	}

	replayed := artifactAssertProjectionRoundTrip(t, sealed, contract, consumer, provider)
	replayedConsumer := onlyProjectShardFor(t, replayed, consumer)
	replayedProvider := staticNamespaceForShard(t, replayed, onlyProjectShardFor(t, replayed, provider))
	replayedResolved := staticResolutionForImport(t, replayed, replayedConsumer, resolvedImport)
	if namespace, ok := replayed.Static().Resolutions().Namespace(replayedResolved); !ok || namespace != replayedProvider {
		t.Fatalf("artifact replay lost static resolution = %v/%v", namespace, ok)
	}
	if alias, ok := replayed.Static().Resolutions().Alias(replayedResolved); !ok || alias != importAlias {
		t.Fatalf("artifact replay lost static Import alias = %v/%v", alias, ok)
	}
	replayedUnresolved := staticResolutionForImport(t, replayed, replayedConsumer, unresolvedImport)
	if disposition, ok := replayed.Static().Resolutions().Disposition(replayedUnresolved); !ok || disposition != linkstatic.ResolutionUnresolved {
		t.Fatalf("artifact replay unresolved disposition = %v/%v", disposition, ok)
	}
	if _, ok := replayed.Static().Resolutions().Namespace(replayedUnresolved); ok {
		t.Fatal("artifact replay unresolved import acquired a static namespace")
	}
}

// Static containment owns the exact Import and its namespace resolution, but
// it never creates executable control.  The paired runtime require remains an
// ordinary Import Application, which proves Link does not collapse the two
// source occurrences or invent a Call for the static one.
func TestStaticContainedImportDoesNotRequireRuntimeApplication(t *testing.T) {
	provider := source(t, `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	consumer := source(t, `
type Snapshot = typeof(require("dependency"))
local API = require("dependency")
type Subject = API.Schema.User
`)
	contract := contract(t)
	if consumer.Module().Count() != 2 {
		t.Fatalf("ImportCount = %d, want two exact source Imports", consumer.Module().Count())
	}
	staticImport, _ := consumer.Module().ImportAt(0)
	runtimeImport, _ := consumer.Module().ImportAt(1)
	// The module-cache declaration is a runtime ingress declaration.  Point
	// it at the live Import, rather than treating the static source occurrence
	// as an executable module entry.
	actors, aliases, roots, entries := moduleCacheDeployment(t, consumer, provider)
	entries[0].Import = runtimeImport.Term
	sealed, err := Seal(&Spec{Target: contract, Modules: []linkproject.Module{
		{Name: "main", Program: consumer},
		{Name: "dependency", Program: provider},
	}, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}})
	if err != nil {
		t.Fatal(err)
	}
	shard := onlyProjectShardFor(t, sealed, consumer)

	staticRow, ok := consumer.Module().Import(staticImport.Term)
	staticCall := staticRow.Call
	if !ok || staticCall == 0 || !consumer.Flow().Containment().Static(staticCall) || consumer.Flow().Executable().Contains(staticCall) {
		t.Fatalf("static Import Call = %v/%v; want static, non-live source occurrence", staticCall, ok)
	}
	runtimeRow, ok := consumer.Module().Import(runtimeImport.Term)
	runtimeCall := runtimeRow.Call
	if !ok || runtimeCall == 0 || !consumer.Flow().Executable().Contains(runtimeCall) {
		t.Fatalf("runtime Import Call = %v/%v; want live source occurrence", runtimeCall, ok)
	}
	if _, ok := sealed.Static().Resolutions().ForImport(shard, staticImport.Term); !ok {
		t.Fatal("static-contained Import lost StaticResolution")
	}
	if _, ok := sealed.Static().Resolutions().ForImport(shard, runtimeImport.Term); !ok {
		t.Fatal("runtime Import lost StaticResolution")
	}

	var importApplications int
	applications := sealed.Project().Applications()
	imports := applications.Imports()
	for index := 0; index < imports.Count(); index++ {
		application, _ := imports.At(index)
		_, occurrence, _, ok := applications.Import(application)
		if !ok {
			t.Fatal("sealed Application lost Program occurrence")
		}
		if occurrence != staticImport.Term && occurrence != runtimeImport.Term {
			continue
		}
		if occurrence == staticImport.Term {
			t.Fatal("static-contained Import acquired a runtime Application")
		}
		importApplications++
	}
	if importApplications != 1 {
		t.Fatalf("runtime Import Applications = %d, want one", importApplications)
	}
}

func staticNamespaceForShard(t testing.TB, link *Link, shard linkproject.Shard) linkstatic.Namespace {
	t.Helper()
	for index := 0; index < link.Static().Namespaces().Count(); index++ {
		namespace, _ := link.Static().Namespaces().At(index)
		owner, ok := link.Static().Namespaces().Shard(namespace)
		if ok && owner == shard {
			return namespace
		}
	}
	t.Fatalf("missing static namespace for shard %v", shard)
	return linkstatic.Namespace{}
}

func staticImportsByLiteral(t testing.TB, p *program.Program, first, second string) (keyspace.Term, keyspace.Term) {
	t.Helper()
	var firstImport, secondImport keyspace.Term
	for index := 0; index < p.Module().Count(); index++ {
		item, ok := p.Module().ImportAt(index)
		if !ok {
			continue
		}
		row, ok := p.Module().Import(item.Term)
		literal := row.Request
		if !ok || literal == 0 {
			continue
		}
		text, ok := sourceStringValue(p, literal)
		if !ok {
			continue
		}
		switch text {
		case first:
			firstImport = item.Term
		case second:
			secondImport = item.Term
		}
	}
	if firstImport == 0 || secondImport == 0 {
		t.Fatalf("literal Imports %q/%q = %v/%v", first, second, firstImport, secondImport)
	}
	return firstImport, secondImport
}

func staticResolutionForImport(t testing.TB, link *Link, shard linkproject.Shard, item keyspace.Term) linkstatic.Resolution {
	t.Helper()
	resolution, ok := link.Static().Resolutions().ForImport(shard, item)
	if !ok {
		t.Fatalf("missing static resolution for %v/%v", shard, item)
	}
	return resolution
}

func staticKeyText(t testing.TB, p *program.Program, key keyspace.Key) string {
	t.Helper()
	value, ok := p.Source().Keys().Exact(key)
	if !ok || value.Kind != keyspace.LiteralString {
		t.Fatalf("static export key = %#v/%v", value, ok)
	}
	return value.String
}

func sourceStringValue(p *program.Program, term keyspace.Term) (string, bool) {
	if p == nil {
		return "", false
	}
	strings := p.Source().Literals().Strings()
	for index := 0; index < strings.Count(); index++ {
		candidate, _, value, ok := strings.At(index)
		if ok && candidate == term {
			return value, true
		}
	}
	return "", false
}
