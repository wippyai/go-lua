package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSeedEntryHeapObjectsForValueCopiesReachableObjectSlice(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()

	rootID := identity.ID{Kind: "test.table", Site: "root", Index: 1}
	childID := identity.ID{Kind: "test.table", Site: "child", Index: 2}
	unrelatedID := identity.ID{Kind: "test.table", Site: "unrelated", Index: 3}
	root := testIdentityValue(reg, rootID)
	child := testIdentityValue(reg, childID)
	unrelated := testIdentityValue(reg, unrelatedID)
	childKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatalf("failed to build child suffix key")
	}

	caller := state.State{}.
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          root,
			StaticMembers: map[keyspace.Key]product.Value{childKey: child},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: child,
		})).
		WriteHeapTableObject(reg, unrelatedID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: unrelated,
		}))

	entry, ok := seedEntryHeapObjectsForValue(reg, caller, state.State{}, root)
	if !ok {
		t.Fatalf("seedEntryHeapObjectsForValue reported no copied object")
	}
	if got := entry.ReadHeapTableObject(reg, rootID); !product.Equal(reg, got.Root(), root) {
		t.Fatalf("root object = %#v, want copied root", got)
	}
	if got := entry.ReadHeapTableObject(reg, childID); !product.Equal(reg, got.Root(), child) {
		t.Fatalf("child object = %#v, want reachable child copied", got)
	}
	if got := entry.ReadHeapTableObject(reg, unrelatedID); !heapidentity.ObjectDomain(reg).Equal(got, heapidentity.BottomObject(reg)) {
		t.Fatalf("unrelated object = %#v, want not copied", got)
	}
}

func TestApplyParamSeedsPreservesExistingValuesAndSeededHeapObjects(t *testing.T) {
	reg := standard.Registry()

	existingSlot := statekey.SymbolValue(101)
	seededSlot := statekey.SymbolValue(102)
	existingValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	rootID := identity.ID{Kind: "test.table", Site: "seed-param-root", Index: 1}
	root := testIdentityValue(reg, rootID)

	caller := state.State{}.WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: root,
	}))
	base := state.State{}.WriteValue(reg, existingSlot, existingValue)

	got := applyParamSeeds(reg, base, caller, []paramSeed{
		{slot: existingSlot, value: root},
		{slot: seededSlot, value: root},
	})
	if value := got.ReadValue(reg, existingSlot); !product.Equal(reg, value, existingValue) {
		t.Fatalf("existing slot = %#v, want caller-supplied value preserved", value)
	}
	if value := got.ReadValue(reg, seededSlot); !product.Equal(reg, value, root) {
		t.Fatalf("seeded slot = %#v, want root seed", value)
	}
	if object := got.ReadHeapTableObject(reg, rootID); !product.Equal(reg, object.Root(), root) {
		t.Fatalf("seeded heap object = %#v, want copied root object", object)
	}
}

func TestCallContextParamEntryValueDoesNotTrustDataShapeForExplicitAnyContract(t *testing.T) {
	reg := standard.Registry()
	actualID := identity.ID{Kind: "test.table", Site: "actual-param", Index: 1}
	actualType := typetable.NewRecord().Field("run", typ.Func().Build()).Build()
	actual := typevalue.WithWitness(reg, typevalue.FromType(reg, actualType), actualType)
	actual = product.Set(reg, actual, identity.Key, identity.Singleton(actualID))
	contract, ok := paramContractEntryValue(reg, typ.Any)
	if !ok {
		t.Fatal("paramContractEntryValue(any) returned !ok")
	}

	got, ok := callContextParamEntryValue(reg, actual, true, contract, true)
	if !ok {
		t.Fatal("callContextParamEntryValue returned !ok")
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); ok {
		t.Fatalf("context param identity = %v, want explicit any contract to hide data-shape identity", gotID)
	}
}

func TestSeedEntryCallableHeapObjectsForValueKeepsCallbacksDropsDataMembers(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	rootID := identity.ID{Kind: "test.table", Site: "root", Index: 1}
	fnID := identity.ID{Kind: "test.function", Site: "callback", Index: 2}
	root := testIdentityValue(reg, rootID)
	callbackType := typ.Func().Build()
	callback := typevalue.WithWitness(reg, typevalue.FromType(reg, callbackType), callbackType)
	callback = product.Set(reg, callback, identity.Key, identity.Singleton(fnID))
	label := typevalue.FromType(reg, typ.String)
	callbackKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "callback"}})
	if !ok {
		t.Fatal("failed to build callback suffix key")
	}
	labelKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "label"}})
	if !ok {
		t.Fatal("failed to build label suffix key")
	}

	caller := state.State{}.WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: root,
		StaticMembers: map[keyspace.Key]product.Value{
			callbackKey: callback,
			labelKey:    label,
		},
	}))

	entry, contextRoot, ok := seedEntryCallableHeapObjectsForValue(reg, caller, state.State{}, root)
	if !ok {
		t.Fatal("seedEntryCallableHeapObjectsForValue returned !ok")
	}
	if gotID, ok := product.Get(reg, contextRoot, identity.Key).ID(); !ok || gotID != rootID {
		t.Fatalf("context root identity = %v/%v, want %v", gotID, ok, rootID)
	}
	object := entry.ReadHeapTableObject(reg, rootID)
	if got, ok := object.StaticMember(callbackKey); !ok || !product.Equal(reg, got, callback) {
		t.Fatalf("callback member = %#v/%v, want copied callback", got, ok)
	}
	if got, ok := object.StaticMember(labelKey); ok {
		t.Fatalf("label member = %#v, want data member omitted across explicit top boundary", got)
	}
}

func testIdentityValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}
