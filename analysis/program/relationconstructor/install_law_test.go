package relationconstructor

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

func typeDeclaration(t *testing.T, key carrier.Key) TypeDeclaration {
	t.Helper()
	token, ok := identity.DeriveContentID("relation-install-law/v1", []byte("type"), []byte(key))
	if !ok {
		t.Fatalf("derive type token for %s", key)
	}
	return TypeDeclaration{Carrier: key, Name: schema.Key(key) + "#type", Token: token}
}

// heapAxis is one axis stated the way production states one: its declared
// members, its owner fence, and the types its columns carry. The relational
// attachment is produced from it by the same producer a real axis goes
// through, so only this input changes when real axes arrive.
func heapAxis(t *testing.T) Axis {
	t.Helper()
	return Axis{
		Entry:   axisRef("heap"),
		Owner:   testOwner(t),
		Catalog: sharedCatalog(t, providerOf("heap/directory"), member.RelationRef{}),
		Types: []TypeDeclaration{
			typeDeclaration(t, "carrier/heap/route"),
			typeDeclaration(t, "carrier/heap/source"),
		},
	}
}

func lawSite() relcompile.Site { return relcompile.Site{Path: "install-law"} }

// TestAnInstalledAxisResolvesEveryDeclarationItStated is the installer's whole
// obligation: after installation the one cross-owner registry answers for the
// relations, columns and keys the axis declared, and for the scope its rows are
// produced under. Until this existed those answers lived only in test code.
func TestAnInstalledAxisResolvesEveryDeclarationItStated(t *testing.T) {
	registry, ok := Install([]Axis{heapAxis(t)}, nil)
	if !ok || registry == nil {
		t.Fatal("the axis did not install")
	}
	routes := relcompile.NewName(axisRef("heap"), "heap/routes")
	relation, err := registry.Relation(lawSite(), routes)
	if err != nil || !relation.Available() {
		t.Fatalf("the registry does not resolve a declared relation: %v", err)
	}
	column, err := registry.Column(lawSite(), relcompile.NewName(axisRef("heap"), "heap/route-key"))
	if err != nil || !column.Available() {
		t.Fatalf("the registry does not resolve a declared column: %v", err)
	}
	key, err := registry.Key(lawSite(), relcompile.NewName(axisRef("heap"), "heap/routes/publication"))
	if err != nil || !key.Available() {
		t.Fatalf("the registry does not resolve a declared key vector: %v", err)
	}
	scope, err := registry.Scope(lawSite(), relcompile.NewName(axisRef("heap"), "scope/heap/heap/directory"))
	if err != nil || !scope.Available() {
		t.Fatalf("the registry does not resolve the scope rows are produced under: %v", err)
	}
	// The column's type resolves to the one the axis declared, so a column's
	// identity and the name it installs under are two readings of one
	// statement rather than two facts that must be kept in step.
	columnType, err := registry.Type(lawSite(), relcompile.NewName(axisRef("heap"), "carrier/heap/route#type"))
	if err != nil || !columnType.Available() {
		t.Fatalf("the registry does not resolve a declared column's type: %v", err)
	}
}

// TestDecisionScopesInstallUnderTheEntryThatNamesThem states that a rule's
// placement is installable: the scopes the declaration determines become
// registry scopes, and one scope named by two rules is installed once rather
// than refusing as a duplicate.
func TestDecisionScopesInstallUnderTheEntryThatNamesThem(t *testing.T) {
	first, ok := DecisionScopes(readingSpec("heap/allocation/closed", "heap/route-key"))
	if !ok {
		t.Fatal("the first rule was not placed")
	}
	second, ok := DecisionScopes(readingSpec("heap/allocation/empty", "heap/route-key"))
	if !ok {
		t.Fatal("the second rule was not placed")
	}
	registry, ok := Install([]Axis{heapAxis(t)}, []relcompile.Placement{first, second, first})
	if !ok || registry == nil {
		t.Fatal("the placed rules did not install")
	}
	for _, name := range []relcompile.Name{first.Candidate, first.Ports[0], second.Candidate, second.Ports[0]} {
		scope, err := registry.Scope(lawSite(), name)
		if err != nil || !scope.Available() {
			t.Fatalf("the registry does not resolve placed scope %v: %v", name, err)
		}
	}
	if first.Candidate == second.Candidate {
		t.Fatal("two rules were placed at one candidate scope")
	}
}

// TestOneInputInstallsOneRegistry states that installation mints nothing of its
// own. Two installations of one construction issue the same identities, so two
// mounts of a program resolve in one address space rather than in two that
// happen to agree.
func TestOneInputInstallsOneRegistry(t *testing.T) {
	placement, ok := DecisionScopes(readingSpec("heap/allocation/closed", "heap/route-key"))
	if !ok {
		t.Fatal("the rule was not placed")
	}
	first, ok := Install([]Axis{heapAxis(t)}, []relcompile.Placement{placement})
	if !ok {
		t.Fatal("the first installation refused")
	}
	second, ok := Install([]Axis{heapAxis(t)}, []relcompile.Placement{placement})
	if !ok {
		t.Fatal("the second installation refused")
	}
	routes := relcompile.NewName(axisRef("heap"), "heap/routes")
	firstRelation, firstErr := first.Relation(lawSite(), routes)
	secondRelation, secondErr := second.Relation(lawSite(), routes)
	if firstErr != nil || secondErr != nil {
		t.Fatal("a declared relation did not resolve")
	}
	if firstRelation != secondRelation {
		t.Fatal("two installations issued different relation identities")
	}
	firstScope, firstErr := first.Scope(lawSite(), placement.Candidate)
	secondScope, secondErr := second.Scope(lawSite(), placement.Candidate)
	if firstErr != nil || secondErr != nil {
		t.Fatal("a placed scope did not resolve")
	}
	if firstScope != secondScope {
		t.Fatal("two installations issued different scope identities")
	}
}

// TestAMalformedInstallationRefusesWholly states that a registry is filled or
// absent. A half-filled registry would resolve some of a program's names and
// silently fail the rest, which is the state this refusal exists to prevent.
func TestAMalformedInstallationRefusesWholly(t *testing.T) {
	if registry, ok := Install(nil, nil); ok || registry != nil {
		t.Fatal("an empty construction installed")
	}
	unfenced := heapAxis(t)
	unfenced.Entry = axisRef("value")
	if registry, ok := Install([]Axis{unfenced}, nil); ok || registry != nil {
		t.Fatal("an axis whose owner fence names another entry installed")
	}
	untyped := heapAxis(t)
	untyped.Types = nil
	if registry, ok := Install([]Axis{untyped}, nil); ok || registry != nil {
		t.Fatal("an axis installed columns whose type nobody declared")
	}
	twice := heapAxis(t)
	if registry, ok := Install([]Axis{heapAxis(t), twice}, nil); ok || registry != nil {
		t.Fatal("one axis installed twice")
	}
}

