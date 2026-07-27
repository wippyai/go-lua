package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/__legacy/analysis/type/indexproof"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func readBoundDynamicIndexValueForTest(
	reg *axis.Registry, typeValues *typevalue.Cache, keys *keyspace.KeySpace, resolver *visibility.Resolver,
	point cfg.Point, tablePath, keyPath pathdom.Path, tableValue, keyValue product.Value, input state.State, projectPath bool,
) (product.Value, bool) {
	form := indexform.IndexForm{}
	if constant, exact := typevalue.IntegerLiteralValue(reg, keyValue); exact {
		form = indexform.NewConstantIndex(constant)
	} else if !keyPath.IsEmpty() {
		form, _ = indexform.NewAffineIndex(keyPath, 1, 0)
	}
	return ReadBoundDynamicValue(BoundDynamicRead{
		Registry: reg, TypeValues: typeValues, KeySpace: keys, Visibility: resolver, Point: point,
		TablePath: tablePath, KeyPath: keyPath, TableValue: tableValue, KeyValue: keyValue,
		ValueInput: input, ProjectPath: projectPath, IndexForm: form,
	})
}

func ReadBoundDynamicIndexValue(
	reg *axis.Registry, typeValues *typevalue.Cache, keys *keyspace.KeySpace, resolver *visibility.Resolver,
	point cfg.Point, tablePath, keyPath pathdom.Path, tableValue, keyValue product.Value, input state.State,
) (product.Value, bool) {
	return readBoundDynamicIndexValueForTest(reg, typeValues, keys, resolver, point, tablePath, keyPath, tableValue, keyValue, input, true)
}

func ReadBoundDynamicTableValue(
	reg *axis.Registry, typeValues *typevalue.Cache, keys *keyspace.KeySpace, resolver *visibility.Resolver,
	point cfg.Point, tablePath, keyPath pathdom.Path, tableValue, keyValue product.Value, input state.State,
) (product.Value, bool) {
	return readBoundDynamicIndexValueForTest(reg, typeValues, keys, resolver, point, tablePath, keyPath, tableValue, keyValue, input, false)
}

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
		got, ok := ReadBoundDynamicIndexValue(reg, typevalue.NewCache(), ks, nil, 0, pathdom.Path{Root: "references"}, pathdom.Path{}, table, abstractName, in)
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

	got, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, pathdom.Path{}, table, typevalue.LiteralString(reg, "false-node"), in)
	if !ok || !product.Equal(reg, got, falseValue) || !presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("false member = %#v/%v, want present false", got, ok)
	}
	missing, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, pathdom.Path{}, table, typevalue.LiteralString(reg, "missing"), in)
	if !ok || !presence.Equal(product.PresenceOf(missing), presence.Absent()) {
		t.Fatalf("stable missing member = %#v/%v, want absent", missing, ok)
	}
	mutable := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: table})
	if _, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, pathdom.Path{}, table, typevalue.LiteralString(reg, "missing"), state.State{}.WriteHeapTableObject(reg, id, mutable)); ok {
		t.Fatal("mutable missing member did not fail closed")
	}
	if unknown, ok := ReadBoundDynamicIndexValue(reg, nil, ks, nil, 0, basePath, pathdom.Path{}, product.Top(), typevalue.LiteralString(reg, "missing"), in); !ok || !product.Equal(reg, unknown, product.Top()) {
		t.Fatalf("abstract owner read = %#v/%v, want conservative Top", unknown, ok)
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

	got, ok := ReadBoundDynamicIndexValue(reg, nil, ks, resolver, point, referencesPath, pathdom.Path{}, selfValue, typevalue.LiteralString(reg, "node"), in)
	if !ok || !product.Equal(reg, got, nodeValue) {
		t.Fatalf("self.references[node] = %#v/%v, want %#v", got, ok, nodeValue)
	}
	if _, ok := ReadBoundDynamicIndexValue(reg, nil, ks, resolver, point, referencesPath, pathdom.Path{}, referencesValue, typevalue.LiteralString(reg, "node"), in); ok {
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
	got, ok := ReadBoundDynamicIndexValue(reg, cache, ks, nil, 0, pathdom.Path{Root: "references"}, pathdom.Path{}, tableValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("typed identity-free dynamic read = %#v/%v, want canonical RuntimeIndex %#v", got, ok, want)
	}
	if presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("typed map lookup incorrectly proved membership/presence: %#v", got)
	}
}

func TestBoundConstantReadUsesLengthFloorToExcludeShortUnionArm(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	sym := symbol.ID(27)
	resolver := testResolver(point, sym, "parts")
	parent := pathdom.NewPath(sym, "parts")
	containerType := &typ.Union{Members: []typ.Type{
		typetable.NewRecord().Build(),
		typ.NewArray(typ.String),
	}}
	table := typevalue.WithWitness(reg, product.Top(), containerType)
	parentKey, ok := resolver.StateKeyAt(point, parent)
	if !ok {
		t.Fatal("parent state key")
	}
	input := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), table).
		WriteLenFloor(resolver.KeySpace(), parentKey, 2)
	request := BoundDynamicRead{
		Registry: reg, KeySpace: resolver.KeySpace(), Visibility: resolver, Point: point,
		TablePath: parent, TableValue: table, KeyValue: typevalue.LiteralInt(reg, 2),
		ValueInput: input, IndexForm: indexform.NewConstantIndex(2),
	}
	observedType, typeOK := typevalue.TypeOf(reg, table)
	if !typeOK || !indexproof.StaticIndexExcludesNilUnderLengthFloor(observedType, 2, 2) {
		t.Fatalf("selected type proof = %v/%t, want non-nil", observedType, typeOK)
	}
	if inRange, projected := BoundDynamicReadInRange(request); !projected || !inRange {
		t.Fatalf("range evidence = %t/%t, want exact proof", inRange, projected)
	}
	value, projected := ReadBoundDynamicValue(request)
	if !projected || !presence.Equal(product.PresenceOf(value), presence.Present()) {
		projectedType, hasProjectedType := typevalue.TypeOf(reg, value)
		runtimeValue, runtimeOK := typevalue.RuntimeIndex(reg, table, request.KeyValue)
		runtimeType, hasRuntimeType := typevalue.TypeOf(reg, runtimeValue)
		t.Fatalf("read presence = %v/%t type=%v/%t; runtime=%t type=%v/%t", product.PresenceOf(value), projected, projectedType, hasProjectedType, runtimeOK, runtimeType, hasRuntimeType)
	}
}

func TestReadBoundDynamicOwnerValueProjectsDescendantWithoutStateRoot(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	ks := keyspace.New()
	tableType := typetable.NewMap(typ.String, typ.String)
	tableValue := cache.FromTypeWithWitness(reg, tableType)
	ownerValue := cache.FromTypeWithWitness(reg, typetable.NewMap(typ.String, tableType))
	keyValue := typevalue.LiteralString(reg, "node")
	path := pathdom.NewPath(symbol.ID(9), "graph").Field("references")
	want, ok := cache.RuntimeIndex(reg, tableValue, keyValue)
	if !ok {
		t.Fatal("test setup RuntimeIndex failed")
	}
	want = InheritTopOriginEvidence(reg, want, tableValue)
	got, ok := ReadBoundDynamicTableValue(reg, cache, ks, nil, 0, path, pathdom.Path{}, tableValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("direct table read = %#v/%v, want %#v", got, ok, want)
	}
	ownerRead, ok := ReadBoundDynamicIndexValue(reg, cache, ks, nil, 0, path, pathdom.Path{}, ownerValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, ownerRead, want) {
		t.Fatalf("owner-path read with omitted State root = %#v/%v, want %#v", ownerRead, ok, want)
	}
}

func TestReadBoundDynamicTableValueOfSparseAxisTopIsExactlyTop(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	// Omitted registered axes are Top in product.Value. Presence-only input is
	// therefore an unconstrained present Lua value, not a missing type producer.
	tableValue := product.WithPresence(reg, product.Top(), presence.Present())
	keyValue := typevalue.LiteralString(reg, "field")
	got, ok := ReadBoundDynamicTableValue(reg, typevalue.NewCache(), ks, nil, 0, pathdom.NewPath(symbol.ID(9), "table"), pathdom.Path{}, tableValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, product.Top()) {
		t.Fatalf("sparse-axis Top table read = %#v/%v, want exact Top", got, ok)
	}
}

func TestReadBoundDynamicTableValueOfNonIndexableOperandsIsExactlyBottom(t *testing.T) {
	reg := standard.Registry()
	got, ok := ReadBoundDynamicTableValue(
		reg, typevalue.NewCache(), keyspace.New(), nil, 0, pathdom.Path{}, pathdom.Path{},
		typevalue.LiteralNumber(reg, 42), typevalue.LiteralString(reg, "field"), state.State{},
	)
	if !ok || !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("non-indexable normal result = %#v/%v, want exact Bottom", got, ok)
	}
}

func TestReadBoundDynamicIndexValueOfVisibleNestedSparseAxisTopIsExactlyTop(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	payload := symbol.ID(19)
	resolver := testResolver(point, payload, "payload")
	ks := resolver.KeySpace()
	root := product.WithPresence(reg, product.Top(), presence.Present())
	tablePath := pathdom.NewPath(payload, "payload").Field("user").Field("profile")
	in := state.State{}.WriteValue(reg, key.SymbolValue(payload), root)
	got, ok := ReadBoundDynamicIndexValue(reg, typevalue.NewCache(), ks, resolver, point, tablePath, pathdom.Path{}, root, typevalue.LiteralString(reg, "name"), in)
	if !ok || !product.Equal(reg, got, product.Top()) {
		t.Fatalf("visible nested sparse-axis Top read = %#v/%v, want exact Top", got, ok)
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
	got, ok := ReadBoundDynamicIndexValue(reg, cache, ks, nil, 0, path, pathdom.Path{}, ownerValue, keyValue, state.State{})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("typed owner-path dynamic read = %#v/%v, want %#v", got, ok, want)
	}
	if presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("typed owner-path lookup incorrectly proved membership/presence: %#v", got)
	}
}
