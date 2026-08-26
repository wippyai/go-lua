package relationconstructor

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func axisRef(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func testOwner(t *testing.T) authority.Owner {
	t.Helper()
	token, ok := identity.DeriveContentID("relation-authority-law/v1", []byte("heap-owner"))
	if !ok {
		t.Fatal("derive owner token")
	}
	owner, ok := authority.NewOwner(axisRef("heap"), token)
	if !ok {
		t.Fatal("seal the owner fence")
	}
	return owner
}

// testTypes resolves every carrier to one owner-issued type. Column types are
// the carrier surface's authority, so the producer is handed them.
func testTypes(t *testing.T, owner authority.Owner) TypeResolver {
	t.Helper()
	return func(key carrier.Key) (model.TypeID, bool) {
		content, ok := identity.DeriveContentID("relation-authority-law/v1", []byte("type"), []byte(key))
		if !ok {
			return model.TypeID{}, false
		}
		return model.IssueTypeID(owner.ID(), content)
	}
}

func providerOf(member_ schema.Key) member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{Axis: axisRef("heap"), Member: member_})
}

// sharedCatalog declares two relations produced under one candidate provider,
// each with its own declared column, and gives the first a key vector.
func sharedCatalog(t *testing.T, secondProvider member.CandidateRef, secondNested member.RelationRef) member.Catalog {
	t.Helper()
	routes := member.Relation{
		Key: "heap/routes", Subject: "carrier/heap/route",
		CandidateProvider: providerOf("heap/directory"),
		Keys: []member.KeyVector{{
			Name: "heap/routes/publication", Columns: []schema.Key{"heap/route-key"},
		}},
	}
	sources := member.Relation{
		Key: "heap/sources", Subject: "carrier/heap/source",
		CandidateProvider: secondProvider,
		Parent:            secondNested,
	}
	if secondNested.Available() {
		sources.Ordinal = "carrier/heap/ordinal"
	}
	catalog, ok := member.NewCatalog(
		[]member.Relation{routes, sources},
		[]member.Projection{
			{Key: "heap/route-key", Relation: "heap/routes", Role: member.Key,
				Result: "carrier/heap/route", CandidateProvider: providerOf("heap/directory")},
			{Key: "heap/source-key", Relation: "heap/sources", Role: member.Key,
				Result: "carrier/heap/source", CandidateProvider: providerOf("heap/directory")},
		}, nil, nil)
	if !ok {
		t.Fatal("the member catalog was refused")
	}
	return catalog
}

// TestRelationsSharingACandidateProviderShareAScope is the ruling this
// producer rests on. A relation's scope is the decision context its rows are
// produced under, and rows exist exactly per candidate the provider admits, so
// two relations produced under one provider are decided in one scope. Nothing
// declares this separately; the provider already does.
func TestRelationsSharingACandidateProviderShareAScope(t *testing.T) {
	owner := testOwner(t)
	catalog, ok := Authority(axisRef("heap"), sharedCatalog(t, providerOf("heap/directory"), member.RelationRef{}), owner, testTypes(t, owner))
	if !ok || !catalog.Available() {
		t.Fatal("the axis attachment was refused")
	}
	if catalog.ScopeCount() != 1 {
		t.Fatalf("two relations under one provider declared %d scopes, want 1", catalog.ScopeCount())
	}
	relations := catalog.Relations()
	if len(relations) != 2 {
		t.Fatalf("attached %d relations, want 2", len(relations))
	}
	if relations[0].Scope() != relations[1].Scope() {
		t.Fatal("two relations produced under one provider were scoped apart")
	}
}

// TestDifferentProvidersAreDifferentScopes states the other half: rows
// produced under different candidates are decided in different contexts, so a
// scope is not an axis-wide constant.
func TestDifferentProvidersAreDifferentScopes(t *testing.T) {
	owner := testOwner(t)
	catalog, ok := Authority(axisRef("heap"), sharedCatalog(t, providerOf("heap/other-directory"), member.RelationRef{}), owner, testTypes(t, owner))
	if !ok || !catalog.Available() {
		t.Fatal("the axis attachment was refused")
	}
	if catalog.ScopeCount() != 2 {
		t.Fatalf("two providers declared %d scopes, want 2", catalog.ScopeCount())
	}
}

// TestOneProviderWithDisagreeingNestingRefuses states the composition defect
// the producer will not paper over. One relation decides rows per candidate
// and the other per parent row, so a single decision context cannot hold both,
// and silently sharing one would give the pair a scope neither produces under.
func TestOneProviderWithDisagreeingNestingRefuses(t *testing.T) {
	owner := testOwner(t)
	nested := member.RelationRef{Axis: axisRef("heap"), Member: "heap/routes"}
	catalog := sharedCatalog(t, providerOf("heap/directory"), nested)
	if produced, ok := Authority(axisRef("heap"), catalog, owner, testTypes(t, owner)); ok || produced.Available() {
		t.Fatal("one provider held relations that disagree about nesting")
	}
}

// TestARelationWithNoProviderTakesTheAxisScope states the base-population arm.
// A relation no candidate gates produces its rows in the axis's own structural
// context. A complete member catalog always names a provider, so this arm is
// the floor the derivation stands on rather than a path through it.
func TestARelationWithNoProviderTakesTheAxisScope(t *testing.T) {
	base := member.Relation{Key: "heap/base", Subject: "carrier/heap/base"}
	if got := scopeKey(axisRef("heap"), base); got != "scope/heap" {
		t.Fatalf("a base population took scope %q, want the axis's own", got)
	}
	gated := member.Relation{
		Key: "heap/routes", Subject: "carrier/heap/route",
		CandidateProvider: providerOf("heap/directory"),
	}
	if scopeKey(axisRef("heap"), gated) == scopeKey(axisRef("heap"), base) {
		t.Fatal("a gated relation shared the axis's base scope")
	}
}

// TestOneDeclarationProducesOneAttachment states that the producer mints
// nothing of its own: tokens come from the owner's fence and the declaration's
// key, so two productions of one catalog are the same sealed attachment.
func TestOneDeclarationProducesOneAttachment(t *testing.T) {
	owner := testOwner(t)
	first, ok := Authority(axisRef("heap"), sharedCatalog(t, providerOf("heap/directory"), member.RelationRef{}), owner, testTypes(t, owner))
	if !ok {
		t.Fatal("the first production was refused")
	}
	second, ok := Authority(axisRef("heap"), sharedCatalog(t, providerOf("heap/directory"), member.RelationRef{}), owner, testTypes(t, owner))
	if !ok {
		t.Fatal("the second production was refused")
	}
	if first.Digest() != second.Digest() {
		t.Fatal("two productions of one declaration sealed different attachments")
	}
}

// TestADeclaredKeyVectorBecomesAKeyAndItsDenominator closes the chain the key
// primitive was landed for: the vector a relation declares is the key the
// registry installs, and the universe its rows are addressed over is that
// exact pair rather than a third named thing.
func TestADeclaredKeyVectorBecomesAKeyAndItsDenominator(t *testing.T) {
	owner := testOwner(t)
	catalog, ok := Authority(axisRef("heap"), sharedCatalog(t, providerOf("heap/directory"), member.RelationRef{}), owner, testTypes(t, owner))
	if !ok {
		t.Fatal("the axis attachment was refused")
	}
	if catalog.KeyCount() != 1 || catalog.DenominatorCount() != 1 {
		t.Fatalf("attached %d keys and %d denominators, want 1 and 1", catalog.KeyCount(), catalog.DenominatorCount())
	}
	keys := catalog.Keys()
	if keys[0].Name() != "heap/routes/publication" {
		t.Fatalf("attached key %q", keys[0].Name())
	}
	denominators := catalog.Denominators()
	if denominators[0].Relation() != "heap/routes" || denominators[0].Key() != "heap/routes/publication" {
		t.Fatal("the denominator does not pair the relation with its declared key")
	}
}
