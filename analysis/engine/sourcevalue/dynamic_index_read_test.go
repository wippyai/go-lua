package sourcevalue

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestReadBoundDynamicIndexValueJoinsEveryMayMatchingFiniteFactDeterministically(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	id := identity.ID{Kind: "table", Site: "resolve-reference", Index: 1}
	table := identityvalue.Present(reg, id)
	abstractName := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	firstKey := typevalue.LiteralString(reg, "first")
	secondKey := typevalue.LiteralString(reg, "second")
	firstValue := typevalue.LiteralBool(reg, false)
	secondValue := typevalue.LiteralString(reg, "node")
	staticValue := typevalue.LiteralInt(reg, 7)
	rootless, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "references"}})
	if !ok {
		t.Fatal("rootless dynamic-index key")
	}
	factA := dynamicindex.NewFact(reg, dynamicindex.FactConfig{KeyValue: firstKey, HasKeyValue: true, Value: firstValue, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})
	factB := dynamicindex.NewFact(reg, dynamicindex.FactConfig{KeyValue: secondKey, HasKeyValue: true, Value: secondValue, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})
	staticKey, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "static"}})
	if !ok {
		t.Fatal("static member key")
	}
	want := product.Join(reg, product.Join(reg, product.Join(reg, firstValue, secondValue), staticValue), typevalue.Nil(reg))

	read := func(reverse bool) product.Value {
		facts := make(map[dynamicindex.Key]dynamicindex.Fact, 2)
		if reverse {
			facts[dynamicindex.Key{Table: rootless, Site: "b"}] = factB
			facts[dynamicindex.Key{Table: rootless, Site: "a"}] = factA
		} else {
			facts[dynamicindex.Key{Table: rootless, Site: "a"}] = factA
			facts[dynamicindex.Key{Table: rootless, Site: "b"}] = factB
		}
		object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: table, DynamicIndexFacts: facts, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{staticKey: staticValue},
		})
		in := state.State{}.WriteHeapTableObject(reg, id, object)
		got, ok := ReadBoundDynamicIndexValue(reg, typevalue.NewCache(), ks, nil, 0, pathdom.Path{Root: "references"}, table, abstractName, in)
		if !ok {
			t.Fatal("abstract-string dynamic read failed")
		}
		return got
	}
	forward, reverse := read(false), read(true)
	if !product.Equal(reg, forward, want) || !product.Equal(reg, reverse, want) || product.Hash(reg, forward) != product.Hash(reg, reverse) {
		t.Fatalf("dynamic read = %#v / %#v, want deterministic join %#v", forward, reverse, want)
	}
}

func TestReadBoundDynamicIndexValueDistinguishesFalseFromAbsentAndFailsClosed(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	id := identity.ID{Kind: "table", Site: "resolve-reference", Index: 2}
	table := identityvalue.Present(reg, id)
	falseValue := typevalue.LiteralBool(reg, false)
	memberKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "false-node"}})
	basePath := pathdom.Path{Root: "references"}
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: table, StaticMembers: map[keyspace.Key]product.Value{memberKey: falseValue}, StableShape: true})
	in := state.State{}.WriteHeapTableObject(reg, id, object)

	got, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, table, typevalue.LiteralString(reg, "false-node"), in)
	if !ok || !product.Equal(reg, got, falseValue) || !presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("false member = %#v/%v, want present false", got, ok)
	}
	missing, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, table, typevalue.LiteralString(reg, "missing"), in)
	if !ok || !presence.Equal(product.PresenceOf(missing), presence.Absent()) {
		t.Fatalf("stable missing member = %#v/%v, want absent", missing, ok)
	}
	mutable := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: table})
	if _, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, table, typevalue.LiteralString(reg, "missing"), state.State{}.WriteHeapTableObject(reg, id, mutable)); ok {
		t.Fatal("mutable missing member did not fail closed")
	}
	if _, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, product.Top(), typevalue.LiteralString(reg, "missing"), in); ok {
		t.Fatal("read without exact caller heap binding did not fail closed")
	}
}

func TestReadBoundDynamicIndexValueProjectsReferencesIdentityFromSelfPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	selfSym := symbol.ID(44)
	resolver := testResolver(point, selfSym, "self")
	ks := resolver.KeySpace()
	selfID := identity.ID{Kind: "table", Site: "self", Index: 1}
	referencesID := identity.ID{Kind: "table", Site: "references", Index: 1}
	selfValue := identityvalue.Present(reg, selfID)
	referencesValue := identityvalue.Present(reg, referencesID)
	referencesPath := pathdom.NewPath(selfSym, "self").Field("references")
	nodeValue := typevalue.LiteralString(reg, "node-id")
	memberKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "node"}})
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: referencesValue, StaticMembers: map[keyspace.Key]product.Value{memberKey: nodeValue}, StableShape: true,
	})
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(selfSym), selfValue).
		WritePathKey(reg, ks, resolver.KeyAt(point, referencesPath), referencesValue).
		WriteHeapTableObject(reg, referencesID, object)

	got, ok := ReadBoundDynamicIndexValue(reg, nil, ks, resolver, point, referencesPath, selfValue, typevalue.LiteralString(reg, "node"), in)
	if !ok || !product.Equal(reg, got, nodeValue) {
		t.Fatalf("self.references[node] = %#v/%v, want %#v", got, ok, nodeValue)
	}
	if _, ok := ReadBoundDynamicIndexValue(reg, nil, ks, resolver, point, referencesPath, referencesValue, typevalue.LiteralString(reg, "node"), in); ok {
		t.Fatal("receiver operand incompatible with tablePath owner did not fail closed")
	}
}

func TestReadBoundDynamicIndexValueFallsBackToCanonicalTypedIndexWithoutIdentity(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	ks := keyspace.New()
	tableType := typetable.NewMap(typ.String, typ.String)
	tableValue := cache.FromTypeWithWitness(reg, tableType)
	keyValue := typevalue.LiteralString(reg, "node")
	want, ok := cache.RuntimeIndex(reg, tableValue, keyValue)
	if !ok {
		t.Fatal("test setup RuntimeIndex failed")
	}
	want = InheritTopOriginEvidence(reg, want, tableValue)
	got, ok := ReadBoundDynamicIndexValue(reg, cache, ks, nil, 0, pathdom.Path{Root: "references"}, tableValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("typed identity-free dynamic read = %#v/%v, want canonical RuntimeIndex %#v", got, ok, want)
	}
	if presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("typed map lookup incorrectly proved membership/presence: %#v", got)
	}
}

func TestReadBoundDynamicTableValueDoesNotReprojectRealTablePath(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	ks := keyspace.New()
	tableValue := cache.FromTypeWithWitness(reg, typetable.NewMap(typ.String, typ.String))
	keyValue := typevalue.LiteralString(reg, "node")
	path := pathdom.NewPath(symbol.ID(9), "graph").Field("references")
	want, ok := cache.RuntimeIndex(reg, tableValue, keyValue)
	if !ok {
		t.Fatal("test setup RuntimeIndex failed")
	}
	want = InheritTopOriginEvidence(reg, want, tableValue)
	got, ok := ReadBoundDynamicTableValue(reg, cache, ks, nil, 0, path, tableValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("direct table read = %#v/%v, want %#v", got, ok, want)
	}
	if _, ok := ReadBoundDynamicIndexValue(reg, cache, ks, nil, 0, path, tableValue, keyValue, state.State{}); ok {
		t.Fatal("owner-path form accepted an already-derived table value")
	}
}

func TestReadBoundDynamicIndexValueProjectsTypedOwnerPathBeforeFallbackIndex(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	ks := keyspace.New()
	referencesType := typetable.NewMap(typ.String, typ.String)
	ownerType := typetable.NewRecord().Field("references", referencesType).Build()
	ownerValue := cache.FromTypeWithWitness(reg, ownerType)
	keyValue := typevalue.LiteralString(reg, "node")
	references, ok := cache.RuntimeIndex(reg, ownerValue, typevalue.LiteralString(reg, "references"))
	if !ok {
		t.Fatal("test setup owner.references RuntimeIndex failed")
	}
	want, ok := cache.RuntimeIndex(reg, references, keyValue)
	if !ok {
		t.Fatal("test setup references[node] RuntimeIndex failed")
	}
	want = InheritTopOriginEvidence(reg, want, references)
	path := pathdom.NewPath(symbol.ID(9), "self").Field("references")
	got, ok := ReadBoundDynamicIndexValue(reg, cache, ks, nil, 0, path, ownerValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("typed owner-path dynamic read = %#v/%v, want %#v", got, ok, want)
	}
	if presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("typed owner-path lookup incorrectly proved membership/presence: %#v", got)
	}
}
