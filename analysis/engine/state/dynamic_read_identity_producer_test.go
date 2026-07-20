package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDynamicReadIdentityTopologyPlannerExactWildcardAndConditional(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	table := mustDynamicIdentityStateKey(t, keys, "sym8801@1")
	exactA, err := domain.PrepareDynamicReadIdentityKeyClass(typevalue.NewCache(), typevalue.LiteralString(reg, "a"))
	if err != nil || !exactA.exact {
		t.Fatalf("exact key class = %#v, %v", exactA, err)
	}
	exactB, _ := domain.PrepareDynamicReadIdentityKeyClass(typevalue.NewCache(), typevalue.LiteralString(reg, "b"))
	wildcard := DynamicReadIdentityWildcardKeyClass()
	ownerTerm := identity.ConcreteTerm(identity.ID{Kind: "table", Site: t.Name(), Index: 1})
	var atoms []DynamicReadIdentityProducer
	for _, item := range []struct {
		site dynamicindex.Site
		key  DynamicReadIdentityKeyClass
	}{{"a", exactA}, {"b", exactB}, {"wild", wildcard}} {
		declared, declareErr := domain.DynamicIndexDynamicReadIdentityProducerDeclarations(keys,
			[]dynamicindex.Key{{Table: table, Site: item.site}}, item.key, []identity.Term{ownerTerm})
		if declareErr != nil {
			t.Fatal(declareErr)
		}
		atoms = append(atoms, declared...)
	}
	index, err := domain.SealDynamicReadIdentityProducerIndex(keys, atoms)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := domain.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	query := DynamicReadQuery{KeySpace: keys, TableKeys: []pathaddr.StateKey{"sym8801@1"}, TableValue: product.Top(),
		KeyValue: typevalue.LiteralString(reg, "a"), TypeValues: typevalue.NewCache()}
	selection, err := domain.PrepareDynamicReadSelection(query)
	if err != nil || selection.MembershipMode() != DynamicReadMembershipImpossible {
		t.Fatalf("selection = %#v, %v", selection, err)
	}
	selected, err := domain.PlanDynamicReadIdentityTopologyProducers(selection, []identity.Term{ownerTerm}, inventory, &index)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[dynamicindex.Site]bool)
	for _, atom := range selected {
		seen[atom.fact.Site] = true
	}
	if !seen["a"] || !seen["wild"] || seen["b"] {
		t.Fatalf("selected atoms = %#v", seen)
	}
	query.KeyKeys = []pathaddr.StateKey{"sym8802@1"}
	conditional, err := domain.PrepareDynamicReadSelection(query)
	if err != nil || conditional.MembershipMode() != DynamicReadMembershipConditional {
		t.Fatalf("conditional = %#v, %v", conditional, err)
	}
	conditionalAtoms, err := domain.PlanDynamicReadIdentityTopologyProducers(conditional, []identity.Term{ownerTerm}, inventory, &index)
	if err != nil {
		t.Fatal(err)
	}
	conditionalSeen := make(map[dynamicindex.Site]bool)
	for _, atom := range conditionalAtoms {
		conditionalSeen[atom.fact.Site] = true
	}
	if !conditionalSeen["a"] || !conditionalSeen["b"] || !conditionalSeen["wild"] {
		t.Fatalf("conditional topology = %#v", conditionalSeen)
	}
}

func TestDynamicReadIdentityStaticAndObjectPublicationsReuseCanonicalPlans(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	target := mustDynamicIdentityStateKey(t, keys, "sym8810@1.field")
	staticPlan, err := domain.PrepareStaticMemberFactorPlan(keys, target, product.Bottom(reg))
	if err != nil {
		t.Fatal(err)
	}
	static, err := domain.StaticMemberIdentityPublications(staticPlan)
	if err != nil || len(static) == 0 {
		t.Fatalf("static = %#v, %v", static, err)
	}
	term := identity.ConcreteTerm(identity.ID{Kind: "table", Site: t.Name(), Index: 1})
	constructor, err := domain.PrepareObjectConstructorPlan(keys, []ObjectConstructorShape{{
		Identity: term, MemberSuffixes: [][]segment.Segment{{{Kind: segment.SegmentField, Name: "field"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	object, err := domain.ObjectConstructorIdentityPublications(constructor)
	if err != nil || len(object) == 0 {
		t.Fatalf("object = %#v, %v", object, err)
	}
	for _, publication := range object {
		if publication.Source != 0 || publication.Producer.term != term {
			t.Fatalf("object publication = %#v", publication)
		}
	}
}

func TestDynamicReadIdentityProducerFormalForwardPullbackAndTerms(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	root := from.FromPath(pathdom.NewPath(symbol.ID(8840), "table"))
	formalRoot := formal.NewRoot(owner, 1, formal.Input)
	wire, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: root, Target: formalRoot}})
	if err != nil {
		t.Fatal(err)
	}
	formalTerm := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(owner, 2), formal.Input))
	suffix, ok := from.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "field"}})
	if !ok {
		t.Fatal("suffix")
	}
	producer := DynamicReadIdentityProducer{seal: domain.seal, keys: from, kind: dynamicReadIdentityProducerHeapMember, path: suffix, term: formalTerm}
	one := identity.ConcreteTerm(identity.ID{Kind: "table", Site: t.Name(), Index: 1})
	image, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: formalTerm, Images: []identity.Term{one}}})
	if !ok {
		t.Fatal("image")
	}
	forward, err := domain.TransportDynamicReadIdentityProducersFormal(domain, []CoordinateFormalRootRekey{wire}, image, []DynamicReadIdentityProducer{producer})
	if err != nil || len(forward) != 1 || forward[0].term != one {
		t.Fatalf("forward = %#v, %v", forward, err)
	}
	pulled, err := domain.PullbackDynamicReadIdentityProducersFormal(domain, []CoordinateFormalRootRekey{wire}, image, forward)
	if err != nil || len(pulled) != 1 || pulled[0] != producer {
		t.Fatalf("pullback = %#v, %v", pulled, err)
	}
	terms, err := domain.DynamicReadIdentityProducerIdentityTerms([]DynamicReadIdentityProducer{producer})
	if err != nil || len(terms) != 1 || terms[0] != formalTerm {
		t.Fatalf("terms = %#v, %v", terms, err)
	}
}

func TestDynamicReadHeapDynamicProducerFormalPullbackMappedUnmappedAndMalformed(t *testing.T) {
	reg := standard.Registry()
	sourceDomain := RegisteredProductDomain(reg)
	callerDomain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	table := from.FromPath(pathdom.NewPath(symbol.ID(8841), "table"))
	formalRoot := formal.NewRoot(owner, 1, formal.Input)
	wire, err := sourceDomain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: table, Target: formalRoot}})
	if err != nil {
		t.Fatal(err)
	}
	formalTerm := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(owner, 2), formal.Input))
	concreteTerm := identity.ConcreteTerm(identity.ID{Kind: "table", Site: t.Name(), Index: 1})
	image, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: formalTerm, Images: []identity.Term{concreteTerm}}})
	if !ok {
		t.Fatal("image")
	}
	declared, err := sourceDomain.DynamicIndexDynamicReadIdentityProducerDeclarations(from,
		[]dynamicindex.Key{{Table: table, Site: "write"}}, DynamicReadIdentityWildcardKeyClass(), []identity.Term{formalTerm})
	if err != nil {
		t.Fatal(err)
	}
	var heap DynamicReadIdentityProducer
	for _, producer := range declared {
		if producer.kind == dynamicReadIdentityProducerHeapDynamic {
			heap = producer
		}
	}
	if !heap.ValidFor(sourceDomain, from) {
		t.Fatal("heap-dynamic declaration missing")
	}
	forward, err := sourceDomain.TransportDynamicReadIdentityProducersFormal(callerDomain, []CoordinateFormalRootRekey{wire}, image, []DynamicReadIdentityProducer{heap})
	if err != nil || len(forward) != 1 || forward[0].term != concreteTerm {
		t.Fatalf("forward = %#v, %v", forward, err)
	}
	pulled, err := sourceDomain.PullbackDynamicReadIdentityProducersFormal(callerDomain, []CoordinateFormalRootRekey{wire}, image, forward)
	if err != nil || len(pulled) != 1 || pulled[0] != heap {
		t.Fatalf("mapped pullback = %#v, %v", pulled, err)
	}

	unmappedTable := to.FromPath(pathdom.NewPath(symbol.ID(8842), "other"))
	unmappedDeclared, err := callerDomain.DynamicIndexDynamicReadIdentityProducerDeclarations(to,
		[]dynamicindex.Key{{Table: unmappedTable, Site: "other-write"}}, DynamicReadIdentityWildcardKeyClass(), []identity.Term{concreteTerm})
	if err != nil {
		t.Fatal(err)
	}
	var unmapped DynamicReadIdentityProducer
	for _, producer := range unmappedDeclared {
		if producer.kind == dynamicReadIdentityProducerHeapDynamic {
			unmapped = producer
		}
	}
	pulled, err = sourceDomain.PullbackDynamicReadIdentityProducersFormal(callerDomain, []CoordinateFormalRootRekey{wire}, image, []DynamicReadIdentityProducer{unmapped})
	if err != nil || len(pulled) != 0 {
		t.Fatalf("unmapped pullback = %#v, %v", pulled, err)
	}
	if _, err := sourceDomain.PullbackDynamicReadIdentityProducersFormal(callerDomain, []CoordinateFormalRootRekey{wire}, image, []DynamicReadIdentityProducer{{}}); err == nil {
		t.Fatal("malformed pullback accepted")
	}
}

func mustDynamicIdentityStateKey(t *testing.T, keys *keyspace.KeySpace, raw pathaddr.StateKey) keyspace.Key {
	t.Helper()
	key, ok := keys.InternStateKey(raw)
	if !ok {
		t.Fatalf("InternStateKey(%q)", raw)
	}
	return key
}
