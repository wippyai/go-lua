package link

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

func TestStaticOwnerAliasAndQualifiedProjectionReplayLaws(t *testing.T) {
	provider := source(t, `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	consumer := source(t, `
local API = require("dependency")
type Subject = API.Schema.User
`)
	contract := contract(t)
	actors, aliases, roots, entries := moduleCacheDeployment(t, consumer, provider)
	seal := func() *Link {
		sealed, err := Seal(&Spec{
			Target:  contract,
			Modules: []linkproject.Module{{Name: "main", Program: consumer}, {Name: "dependency", Program: provider}},
			Module:  linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries},
		})
		if err != nil {
			t.Fatal(err)
		}
		return sealed
	}
	sealed := seal()
	consumerShard := onlyProjectShardFor(t, sealed, consumer)

	consumerResolver, ok := sealed.Static().Namespaces().ResolverForShard(consumerShard)
	if !ok {
		t.Fatal("consumer Program resolver absent")
	}
	if shard, ok := sealed.Static().Namespaces().ResolverShard(consumerResolver); !ok || shard != consumerShard {
		t.Fatal("consumer Program resolver selected the wrong Shard")
	}

	importTerm, ok := consumer.Module().ImportAt(0)
	if !ok {
		t.Fatal("consumer Import absent")
	}
	importRow, ok := consumer.Module().Import(importTerm.Term)
	call, alias := importRow.Call, importRow.Alias
	if !ok || call == 0 || alias == 0 {
		t.Fatal("consumer Import alias malformed")
	}
	resolution, ok := sealed.Static().Resolutions().ForCall(consumerShard, call)
	if !ok {
		t.Fatal("static Call resolution absent")
	}
	importResolution, ok := sealed.Static().Resolutions().ForImport(consumerShard, importTerm.Term)
	if !ok || importResolution != resolution {
		t.Fatal("static Import lookup disagrees with owner Call lookup")
	}
	shardResolution, ok := sealed.Static().Resolutions().ForCallInShard(consumerResolver, call)
	if !ok || shardResolution != resolution {
		t.Fatal("static Shard Call lookup disagrees with owner Call lookup")
	}
	namespace, ok := sealed.Static().Namespaces().ForAlias(consumerShard, alias)
	if !ok {
		t.Fatal("static Import alias namespace absent")
	}
	if resolvedNamespace, ok := sealed.Static().Resolutions().Namespace(resolution); !ok || resolvedNamespace != namespace {
		t.Fatal("Call and alias projections disagree")
	}

	reference := qualifiedReferenceForAlias(t, consumer, alias)
	typedReference, ok := consumer.Static().StaticTypes().Ref(reference)
	if !ok {
		t.Fatal("qualified reference was not static")
	}
	consumerExpression, expressionOK := sealed.Static().Expressions().For(consumerResolver, typedReference)
	if !expressionOK {
		t.Fatal("qualified consumer expression absent")
	}
	expression, ok := sealed.Static().Expressions().Qualified(consumerExpression)
	resolvedReference, resolvedOK := sealed.Static().Expressions().Reference(expression)
	resolver, resolverOK := sealed.Static().Expressions().Resolver(expression)
	if !ok || !resolvedOK || !resolverOK {
		t.Fatalf("qualified expression = %v/%v/%v", expression, resolvedReference, ok)
	}
	wantResolver, ok := sealed.Static().Namespaces().Resolver(namespace)
	if !ok || resolver != wantResolver {
		t.Fatal("qualified type lost provider resolver")
	}
	priorShardIndex := -1
	var priorReference keyspace.Term
	for index := 0; index < sealed.Static().Expressions().QualifiedCount(); index++ {
		rowExpression, got, ok := sealed.Static().Expressions().QualifiedAt(index)
		rowReference, referenceOK := sealed.Static().Expressions().Reference(rowExpression)
		shard, shardOK := sealed.Static().Expressions().Shard(rowExpression)
		shardIndex, shardIndexOK := sealed.Project().Mounts().Index(shard)
		gotReference, gotReferenceOK := sealed.Static().Expressions().Reference(got)
		gotResolver, resolverOK := sealed.Static().Expressions().Resolver(got)
		if !ok || !referenceOK || !shardOK || !shardIndexOK || !gotReferenceOK || !resolverOK ||
			(index != 0 && (shardIndex < priorShardIndex || (shardIndex == priorShardIndex && rowReference.Term() <= priorReference))) {
			t.Fatalf("qualified row %d lookup/order is malformed", index)
		}
		if resolved, found := sealed.Static().Expressions().Qualified(rowExpression); !found || resolved != got {
			t.Fatalf("qualified row %d does not replay through its consumer expression", index)
		}
		_ = gotReference
		_ = gotResolver
		priorShardIndex, priorReference = shardIndex, rowReference.Term()
	}

	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = sealed.Static().Namespaces().ResolverForShard(consumerShard)
		_, _ = sealed.Static().Resolutions().ForImport(consumerShard, importTerm.Term)
		_, _ = sealed.Static().Resolutions().ForCall(consumerShard, call)
		_, _ = sealed.Static().Resolutions().ForCallInShard(consumerResolver, call)
		_, _ = sealed.Static().Namespaces().ForAlias(consumerShard, alias)
		_, _ = sealed.Static().Expressions().Qualified(consumerExpression)
	}); allocations != 0 {
		t.Fatalf("static owner/qualified projection allocates %v", allocations)
	}

	replayed := artifactAssertProjectionRoundTrip(t, sealed, contract, consumer, provider)
	twin := seal()
	for _, other := range []*Link{twin, replayed} {
		if other == sealed || other.ContentID() != sealed.ContentID() {
			t.Fatal("projection replay fixture is not equivalent")
		}
		if _, ok := other.Static().Namespaces().Shard(namespace); ok {
			t.Fatal("alias namespace crossed equivalent Link owner fence")
		}
		if _, ok := other.Static().Namespaces().Namespace(resolver); ok {
			t.Fatal("qualified resolver crossed equivalent Link owner fence")
		}
		if _, ok := other.Static().Namespaces().Namespace(consumerResolver); ok {
			t.Fatal("Program resolver crossed equivalent Link owner fence")
		}
		if _, _, _, _, ok := other.Static().Resolutions().Source(resolution); ok {
			t.Fatal("Call resolution crossed equivalent Link owner fence")
		}
		otherConsumerShard := onlyProjectShardFor(t, other, consumer)
		otherConsumerResolver, resolverOK := other.Static().Namespaces().ResolverForShard(otherConsumerShard)
		shardOK := otherConsumerShard != (linkproject.Shard{})
		otherImportResolution, importOK := other.Static().Resolutions().ForImport(otherConsumerShard, importTerm.Term)
		otherCallResolution, callOK := other.Static().Resolutions().ForCall(otherConsumerShard, call)
		otherShardResolution, shardResolutionOK := other.Static().Resolutions().ForCallInShard(otherConsumerResolver, call)
		otherAliasNamespace, aliasOK := other.Static().Namespaces().ForAlias(otherConsumerShard, alias)
		otherConsumerExpression, expressionOK := other.Static().Expressions().For(otherConsumerResolver, typedReference)
		otherExpression, ok := other.Static().Expressions().Qualified(otherConsumerExpression)
		otherReference, otherReferenceOK := other.Static().Expressions().Reference(otherExpression)
		otherResolver, otherResolverOK := other.Static().Expressions().Resolver(otherExpression)
		otherResolutionNamespace, resolutionNamespaceOK := other.Static().Resolutions().Namespace(otherCallResolution)
		if !ok || !resolverOK || !shardOK || !importOK || !callOK || !shardResolutionOK || !aliasOK ||
			otherImportResolution != otherCallResolution || otherCallResolution != otherShardResolution ||
			!resolutionNamespaceOK || otherResolutionNamespace != otherAliasNamespace ||
			!expressionOK || !otherReferenceOK || !otherResolverOK || otherReference != resolvedReference {
			t.Fatal("qualified replay changed stable typed reference")
		}
		if _, ok := sealed.Static().Namespaces().Namespace(otherResolver); ok {
			t.Fatal("replayed qualified resolver crossed back into original Link")
		}
	}

	if _, ok := sealed.Static().Namespaces().ResolverForShard(linkproject.Shard{}); ok {
		t.Fatal("zero Project Shard resolver accepted")
	}
	if _, ok := sealed.Static().Namespaces().ForAlias(linkproject.Shard{}, 0); ok {
		t.Fatal("zero Import alias accepted")
	}
	if _, ok := sealed.Static().Expressions().Qualified(linkstatic.Expression{}); ok {
		t.Fatal("zero qualified reference accepted")
	}
}

func TestStaticOwnerProjectionRejectsAmbiguousProgramIdentity(t *testing.T) {
	consumer := source(t, `local API = require("provider")`)
	provider := source(t, `return 1`)
	importTerm, ok := consumer.Module().ImportAt(0)
	if !ok {
		t.Fatal("consumer Import absent")
	}
	importRow, ok := consumer.Module().Import(importTerm.Term)
	call, alias := importRow.Call, importRow.Alias
	if !ok || call == 0 || alias == 0 {
		t.Fatal("consumer Import alias malformed")
	}
	sealed, err := Seal(&Spec{
		Target: contract(t),
		Modules: []linkproject.Module{
			{Name: "consumer-a", Program: consumer}, {Name: "consumer-b", Program: consumer}, {Name: "provider", Program: provider},
		},
		Module: linkmodule.Spec{Actors: []linkmodule.ActorSpec{{Name: "actor"}}, ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{
			{Actor: "actor", Instances: []string{"cache-a"}, Representative: "cache-a"},
			{Actor: "actor", Instances: []string{"cache-b"}, Representative: "cache-b"},
			{Actor: "actor", Instances: []string{"cache-provider"}, Representative: "cache-provider"},
		},
			AnalysisRoots: []linkmodule.AnalysisRootSpec{
				{Name: "root-a", Module: "consumer-a", Actor: "actor", Instance: "cache-a"},
				{Name: "root-b", Module: "consumer-b", Actor: "actor", Instance: "cache-b"},
				{Name: "root-provider", Module: "provider", Actor: "actor", Instance: "cache-provider"},
			},
			ModuleCacheEntries: []linkmodule.ModuleCacheEntrySpec{
				{Module: "consumer-a", Import: importTerm.Term, FromRoot: "root-a", ToRoot: "root-provider"},
				{Module: "consumer-b", Import: importTerm.Term, FromRoot: "root-b", ToRoot: "root-provider"},
			}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sealed.Static().Namespaces().ResolverForShard(linkproject.Shard{}); ok {
		t.Fatal("zero Project Shard selected one ResolverRef")
	}
	var scoped int
	for index := 0; index < sealed.Project().Mounts().Count(); index++ {
		shard, _ := sealed.Project().Mounts().At(index)
		p, _ := sealed.Project().Mounts().Program(shard)
		if p != consumer {
			continue
		}
		resolver, ok := sealed.Static().Namespaces().ResolverForShard(shard)
		resolution, found := sealed.Static().Resolutions().ForCallInShard(resolver, call)
		if !ok || !found {
			t.Fatalf("ambiguous consumer Shard %v lost its scoped static resolution", shard)
		}
		if _, ok := sealed.Static().Resolutions().Namespace(resolution); !ok {
			t.Fatalf("ambiguous consumer Shard %v lost its scoped namespace", shard)
		}
		scoped++
	}
	if scoped != 2 {
		t.Fatalf("scoped ambiguous consumers = %d, want 2", scoped)
	}
}

func qualifiedReferenceForAlias(t testing.TB, p *program.Program, alias keyspace.Term) keyspace.Term {
	t.Helper()
	references := p.Static().References()
	for index := 0; index < references.Count(); index++ {
		reference, ok := references.At(index)
		if !ok {
			continue
		}
		_, _, root, ok := references.Get(reference)
		if ok && root == alias {
			return reference
		}
	}
	t.Fatal("qualified TypeRef for Import alias absent")
	return 0
}
