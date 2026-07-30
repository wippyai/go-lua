package sourcevalue

import (
	"testing"

	valuerefinement "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func resolveDynamicReadTest(t *testing.T, domain state.ProductDomain, keys *keyspace.KeySpace, cache *typevalue.Cache, request state.DynamicReadQuery, input state.State) (product.Value, bool, error) {
	t.Helper()
	request.KeySpace, request.TypeValues = keys, cache
	evidence, err := domain.ProjectDynamicReadEvidence(request, input)
	if err != nil {
		return product.Value{}, false, err
	}
	value, ok := ResolveDynamicRead(request, evidence)
	return value, ok, nil
}

func TestResolveDynamicReadPrefersFiniteFactAndUsesMembership(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	tableState := pathaddr.StateKey("sym930@1.table")
	keyState := pathaddr.StateKey("sym931@1.key")
	tableKey, _ := keys.InternStateKey(tableState)
	tableValue := typevalue.NewCache().FromTypeWithWitness(reg, typetable.NewMap(typ.String, typ.String))
	keyValue := typevalue.LiteralString(reg, "selected")
	factValue := product.WithPresence(reg, typevalue.LiteralString(reg, "fact"), presence.Present())
	input := state.Reachable(state.State{}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: tableKey, Site: "write"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: keyValue, HasKeyValue: true, Value: factValue, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddPathKeyMembership(keyState, tableState)
	cache := typevalue.NewCache()
	got, ok, err := resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{
		TableValue: tableValue, KeyValue: keyValue, TableKeys: []pathaddr.StateKey{tableState}, KeyKeys: []pathaddr.StateKey{keyState},
	}, input)
	contract, _ := cache.RuntimeIndex(reg, tableValue, keyValue)
	want := WithoutNilRuntimeKind(reg, product.WithPresence(reg, valuerefinement.MergeDeclaredContract(reg, factValue, contract), presence.Present()))
	if err != nil || !ok || !product.Equal(reg, got, want) || !presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("factor dynamic read = %#v/%v err=%v, want present joined contract %#v", got, ok, err, want)
	}
}

func TestResolveDynamicReadFiniteFactDoesNotNarrowOpenTable(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	tableState := pathaddr.StateKey("sym935@1.table")
	keyState := pathaddr.StateKey("sym936@1.key")
	tableKey, _ := keys.InternStateKey(tableState)
	keyValue := typevalue.LiteralString(reg, "selected")
	input := state.Reachable(state.State{}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: tableKey, Site: "write"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: keyValue, HasKeyValue: true, Value: typevalue.LiteralString(reg, "fact"), HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddPathKeyMembership(keyState, tableState)
	got, ok, err := resolveDynamicReadTest(t, domain, keys, typevalue.NewCache(), state.DynamicReadQuery{
		TableValue: product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		KeyValue:   keyValue,
		TableKeys:  []pathaddr.StateKey{tableState},
		KeyKeys:    []pathaddr.StateKey{keyState},
	}, input)
	if err != nil || !ok || !product.Equal(reg, got, product.Top()) {
		t.Fatalf("open-table finite-fact read = %#v/%v err=%v, want Top", got, ok, err)
	}
}

func TestResolveDynamicReadReadsHeapStaticMemberAndMatchesConcrete(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	cache := typevalue.NewCache()
	keyValue := typevalue.LiteralString(reg, "selected")
	id := identity.ID{Kind: "table", Site: "factor-read", Index: 1}
	tableValue := identityvalue.Present(reg, id)
	heapValue := typevalue.LiteralInt(reg, 42)
	suffix, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentIndexString, Name: "selected"}})
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: tableValue, StaticMembers: map[keyspace.Key]product.Value{suffix: heapValue}, StableShape: true,
	})
	input := state.Reachable(state.State{}).WriteHeapTableObject(reg, id, object)
	got, ok, err := resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{TableValue: tableValue, KeyValue: keyValue}, input)
	if err != nil || !ok || !product.Equal(reg, got, heapValue) {
		t.Fatalf("heap factor read = %#v/%v err=%v, want %#v", got, ok, err, heapValue)
	}
	concrete, concreteOK := ReadBoundDynamicTableValue(reg, cache, keys, nil, 0, pathdom.Path{}, pathdom.Path{}, tableValue, keyValue, input)
	if !concreteOK || !product.Equal(reg, got, concrete) {
		t.Fatalf("heap static factor/concrete = %#v/%#v (%v), want differential equality", got, concrete, concreteOK)
	}

	rootless, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "dynamic"}})
	dynamicValue := typevalue.LiteralString(reg, "dynamic")
	dynamicObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: tableValue, DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			{Table: rootless, Site: "write"}: dynamicindex.NewFact(reg, dynamicindex.FactConfig{
				KeyValue: keyValue, HasKeyValue: true, Value: dynamicValue, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
			}),
		}, StableShape: true,
	})
	dynamicInput := state.Reachable(state.State{}).WriteHeapTableObject(reg, id, dynamicObject)
	got, ok, err = resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{TableValue: tableValue, KeyValue: keyValue}, dynamicInput)
	concrete, concreteOK = ReadBoundDynamicTableValue(reg, cache, keys, nil, 0, pathdom.Path{}, pathdom.Path{}, tableValue, keyValue, dynamicInput)
	if err != nil || !ok || !concreteOK || !product.Equal(reg, got, concrete) || !product.Equal(reg, got, dynamicValue) {
		t.Fatalf("heap dynamic factor/concrete = %#v/%#v (%v/%v) err=%v", got, concrete, ok, concreteOK, err)
	}

	typedTable := cache.FromTypeWithWitness(reg, typetable.NewMap(typ.String, typ.Integer))
	want, _ := cache.RuntimeIndex(reg, typedTable, keyValue)
	got, ok, err = resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{TableValue: typedTable, KeyValue: keyValue}, state.Reachable(state.State{}))
	if err != nil || !ok || !product.Equal(reg, got, want) {
		t.Fatalf("runtime factor read = %#v/%v err=%v, want %#v", got, ok, err, want)
	}
}

func TestResolveDynamicReadMembershipOverridesStableHeapAbsenceWithContract(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	cache := typevalue.NewCache()
	id := identity.ID{Kind: "table", Site: "membership-contract", Index: 1}
	tableType := typetable.NewMap(typ.String, typ.Integer)
	tableValue := product.Set(reg, cache.FromTypeWithWitness(reg, tableType), identity.Key, identity.Singleton(id))
	keyValue := typevalue.LiteralString(reg, "selected")
	tableState := pathaddr.StateKey("sym940@1")
	keyState := pathaddr.StateKey("sym941@1")
	input := state.Reachable(state.State{}).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableValue, StableShape: true})).
		AddPathKeyMembership(keyState, tableState)
	got, ok, err := resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{
		TableValue: tableValue, KeyValue: keyValue,
		TableKeys: []pathaddr.StateKey{tableState}, KeyKeys: []pathaddr.StateKey{keyState},
	}, input)
	gotType, typed := cache.TypeOf(reg, got)
	if err != nil || !ok || !typed || !typ.TypeEquals(gotType, typ.Integer) ||
		!presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("membership-certified stable heap read = %v/%v presence=%s ok=%v err=%v, want present integer", gotType, typed, product.PresenceOf(got), ok, err)
	}
}

func TestResolveDynamicReadMembershipMakesSparseOperandsPresent(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	input := state.Reachable(state.State{}).AddPathKeyMembership("sym950@1", "sym951@1")
	got, ok, err := resolveDynamicReadTest(t, domain, keys, typevalue.NewCache(), state.DynamicReadQuery{
		TableValue: product.Bottom(reg), KeyValue: product.Bottom(reg),
		TableKeys: []pathaddr.StateKey{"sym951@1"}, KeyKeys: []pathaddr.StateKey{"sym950@1"},
	}, input)
	if err != nil || !ok || !presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("membership-certified sparse read presence=%s ok=%v err=%v, want present", product.PresenceOf(got), ok, err)
	}
}

func TestResolveDynamicReadBroadKeyDemandsOnlySelectedStableHeapObject(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	cache := typevalue.NewCache()
	selectedID := identity.ID{Kind: "table", Site: "broad-selected", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "broad-other", Index: 1}
	selectedTable := identityvalue.Present(reg, selectedID)
	otherTable := identityvalue.Present(reg, otherID)
	alpha := typevalue.LiteralInt(reg, 1)
	beta := typevalue.LiteralString(reg, "two")
	poison := typevalue.LiteralBool(reg, true)
	suffix := func(name string) keyspace.Key {
		value, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
		if !ok {
			t.Fatalf("suffix %q", name)
		}
		return value
	}
	input := state.Reachable(state.State{}).
		WriteHeapTableObject(reg, selectedID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: selectedTable, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{suffix("alpha"): alpha, suffix("beta"): beta},
		})).
		WriteHeapTableObject(reg, otherID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: otherTable, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{suffix("poison"): poison},
		}))
	broadKey := typevalue.String(reg)
	binderValue, binderOK, err := resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{TableValue: selectedTable, KeyValue: broadKey}, input)
	concreteValue, concreteOK := ReadBoundDynamicTableValue(reg, cache, keys, nil, 0, pathdom.Path{}, pathdom.Path{}, selectedTable, broadKey, input)
	want := product.Join(reg, product.Join(reg, alpha, beta), typevalue.Nil(reg))
	if err != nil || !binderOK || !concreteOK || !product.Equal(reg, binderValue, concreteValue) || !product.Equal(reg, binderValue, want) {
		t.Fatalf("broad binder/concrete = %#v/%#v (%v/%v) err=%v, want %#v", binderValue, concreteValue, binderOK, concreteOK, err, want)
	}
	if !product.Equal(reg, product.Meet(reg, binderValue, poison), product.Bottom(reg)) {
		t.Fatalf("broad read leaked unrelated heap object: value=%#v poison=%#v", binderValue, poison)
	}
}

func TestResolveDynamicReadProjectsNestedHeapOwnerLikeConcrete(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	cache := typevalue.NewCache()
	rootID := identity.ID{Kind: "table", Site: "nested-root", Index: 1}
	middleID := identity.ID{Kind: "table", Site: "nested-middle", Index: 1}
	tableID := identity.ID{Kind: "table", Site: "nested-table", Index: 1}
	rootValue := identityvalue.Present(reg, rootID)
	middleValue := identityvalue.Present(reg, middleID)
	tableValue := identityvalue.Present(reg, tableID)
	selected := typevalue.LiteralString(reg, "nested-value")
	keyValue := typevalue.LiteralString(reg, "selected")
	suffix := func(name string) keyspace.Key {
		value, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
		if !ok {
			t.Fatalf("suffix %q", name)
		}
		return value
	}
	input := state.Reachable(state.State{}).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue, StaticMembers: map[keyspace.Key]product.Value{suffix("a"): middleValue}, StableShape: true})).
		WriteHeapTableObject(reg, middleID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: middleValue, StaticMembers: map[keyspace.Key]product.Value{suffix("b"): tableValue}, StableShape: true})).
		WriteHeapTableObject(reg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableValue, StaticMembers: map[keyspace.Key]product.Value{suffix("selected"): selected}, StableShape: true}))
	path := pathdom.NewPath(symbol.ID(77), "root").Field("a").Field("b")
	pathKey := keys.FromPath(path)
	factorValue, factorOK, err := resolveDynamicReadTest(t, domain, keys, cache, state.DynamicReadQuery{
		TableValue: rootValue, KeyValue: keyValue, TablePath: pathKey, ProjectPath: true,
	}, input)
	concrete, concreteOK := ReadBoundDynamicIndexValue(reg, cache, keys, nil, 0, path, pathdom.Path{}, rootValue, keyValue, input)
	if err != nil || !factorOK || !concreteOK || !product.Equal(reg, factorValue, concrete) || !product.Equal(reg, factorValue, selected) {
		gotLiteral, gotLiteralOK := typevalue.StringLiteralOf(reg, factorValue)
		wantLiteral, wantLiteralOK := typevalue.StringLiteralOf(reg, selected)
		gotType, gotTypeOK := typevalue.TypeOf(reg, factorValue)
		t.Fatalf("nested factor/concrete = %#v/%#v (%v/%v) err=%v literal=%q/%v want=%q/%v type=%v/%v", factorValue, concrete, factorOK, concreteOK, err, gotLiteral, gotLiteralOK, wantLiteral, wantLiteralOK, gotType, gotTypeOK)
	}
}

func TestResolveDynamicReadFinalizesEveryResultThroughPairedRangeEvidence(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	point := cfg.Point(33)
	arraySymbol, indexSymbol := symbol.ID(3301), symbol.ID(3302)
	builder := visibility.NewBuilder()
	builder.Define(point, arraySymbol, "array")
	builder.Define(point, indexSymbol, "index")
	resolver := visibility.NewResolver(builder.Build())
	arrayPath := pathdom.NewPath(arraySymbol, "array")
	indexPath := pathdom.NewPath(indexSymbol, "index")
	arrayAddress := visibility.AddressAt(resolver, point, arrayPath)
	indexAddress := visibility.AddressAt(resolver, point, indexPath)
	arrayStateKey, arrayOK := arrayAddress.VisibleStateKey()
	indexStateKey, indexOK := indexAddress.VisibleStateKey()
	arrayKey, arrayInterned := resolver.KeySpace().InternStateKey(arrayStateKey)
	indexKey, indexInterned := resolver.KeySpace().InternStateKey(indexStateKey)
	if !arrayOK || !indexOK || !arrayInterned || !indexInterned {
		t.Fatal("failed to freeze exact array/index addresses")
	}
	input := state.Reachable(state.State{}).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofIndexInRange, Path: indexKey, Other: arrayKey})
	table := cache.FromTypeWithWitness(reg, typ.NewArray(typ.String))
	index := cache.FromTypeWithWitness(reg, typ.Integer)
	got, ok := ReadBoundDynamicTableValue(reg, cache, resolver.KeySpace(), resolver, point, arrayPath, indexPath, table, index, input)
	if !ok {
		t.Fatal("canonical dynamic read did not resolve")
	}
	gotType, gotTypeOK := typevalue.TypeOf(reg, got)
	if !presence.Equal(product.PresenceOf(got), presence.Present()) || !gotTypeOK || !typ.TypeEquals(gotType, typ.String) || typevalue.HasOnlyNilType(reg, got) {
		t.Fatalf("range-refined read = %#v presence=%s, want present non-nil string", got, product.PresenceOf(got))
	}

	withoutProof, ok := ReadBoundDynamicTableValue(reg, cache, resolver.KeySpace(), resolver, point, arrayPath, indexPath, table, index, state.Reachable(state.State{}))
	if !ok {
		t.Fatal("unrefined dynamic read did not resolve")
	}
	if presence.Equal(product.PresenceOf(withoutProof), presence.Present()) {
		t.Fatalf("read without registered range evidence became present: %#v", withoutProof)
	}
}
