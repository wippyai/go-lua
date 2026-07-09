package heapidentity

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestObjectDomainBottomTopLaws(t *testing.T) {
	reg := standard.Registry()
	domain := ObjectDomain(reg)
	bottom := BottomObject(reg)
	top := TopObject()
	object := NewTableObject(TableObjectConfig{Root: presentValue(reg)})

	if !domain.Equal(bottom, domain.Bottom()) {
		t.Fatalf("BottomObject and domain.Bottom differ: %#v vs %#v", bottom, domain.Bottom())
	}
	if domain.Equal(bottom, top) {
		t.Fatalf("bottom and top should differ")
	}
	if !domain.LessOrEq(bottom, object) {
		t.Fatalf("bottom should be below finite object")
	}
	if domain.LessOrEq(object, bottom) {
		t.Fatalf("finite object should not be below bottom")
	}
	if !domain.LessOrEq(object, top) {
		t.Fatalf("finite object should be below top")
	}
	if !domain.Equal(domain.Join(bottom, object), object) {
		t.Fatalf("join(bottom, object) should return object")
	}
	if !domain.Equal(domain.Join(top, object), top) {
		t.Fatalf("top should absorb join")
	}
}

func TestObjectDomainBottomIdentityIsStackLocal(t *testing.T) {
	reg := standard.Registry()
	domain := ObjectDomain(reg)
	bottom := BottomObject(reg)
	ks := keyspace.New()
	staticKey := fieldSuffixKey(t, ks, "name")
	dynKey := dynamicindex.Key{Table: stateKey(t, ks, "sym90@1.table"), Site: "dyn"}
	present := presentValue(reg)
	object := NewTableObject(TableObjectConfig{
		Root:          present,
		StaticMembers: map[keyspace.Key]product.Value{staticKey: present},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynKey: {
				KeyPresence: presence.Present(),
				KeyValue:    present,
				Value:       present,
				Admission:   dynamicindex.AdmissionAdmitted,
			},
		},
	})

	if got := domain.Join(bottom, object); !domain.Equal(got, object) {
		t.Fatalf("join(bottom, object) = %#v, want object", got)
	}
	if got := domain.Widen(bottom, object); !domain.Equal(got, object) {
		t.Fatalf("widen(bottom, object) = %#v, want object", got)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = domain.Join(bottom, object)
		_ = domain.Join(object, bottom)
		_ = domain.Widen(bottom, object)
		_ = domain.Widen(object, bottom)
	}); allocs != 0 {
		t.Fatalf("bottom identity object operations allocated %.1f times, want immutable operand reuse", allocs)
	}
}

func TestObjectJoinWidenRootStaticAndDynamic(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	objectDomain := ObjectDomain(reg)
	dynamicDomain := dynamicindex.MapDomain(reg)
	ks := keyspace.New()
	staticCommon := stateKey(t, ks, "sym90@1.table.name")
	staticLeft := stateKey(t, ks, "sym90@1.table.left")
	staticRight := stateKey(t, ks, "sym90@1.table.right")
	dynTable := stateKey(t, ks, "sym90@1.table")
	dynCommon := dynamicindex.Key{Table: dynTable, Site: "dyn"}
	dynLeft := dynamicindex.Key{Table: dynTable, Site: "left"}
	present := presentValue(reg)
	absent := absentValue(reg)
	presentFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	absentFact := dynamicindex.Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absent,
		Value:       absent,
		Admission:   dynamicindex.AdmissionRejected,
	}

	left := NewTableObject(TableObjectConfig{
		Root: present,
		StaticMembers: map[keyspace.Key]product.Value{
			staticCommon: present,
			staticLeft:   present,
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynCommon: presentFact,
			dynLeft:   presentFact,
		},
	})
	right := NewTableObject(TableObjectConfig{
		Root: absent,
		StaticMembers: map[keyspace.Key]product.Value{
			staticCommon: absent,
			staticRight:  absent,
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynCommon: absentFact,
		},
	})

	joined := objectDomain.Join(left, right)
	if !valueDomain.Equal(joined.Root(), product.Top()) {
		t.Fatalf("joined root = %s, want top", formatValue(reg, joined.Root()))
	}
	if got, ok := joined.StaticMember(staticCommon); !ok || !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined static common = %s ok=%v, want top", formatValue(reg, got), ok)
	}
	if _, ok := joined.StaticMember(staticLeft); ok {
		t.Fatalf("left-only static member survived must-map join")
	}
	if _, ok := joined.StaticMember(staticRight); ok {
		t.Fatalf("right-only static member survived must-map join")
	}

	wantDynamic := dynamicDomain.Join(left.DynamicIndexFacts(), right.DynamicIndexFacts())
	if !dynamicDomain.Equal(joined.DynamicIndexFacts(), wantDynamic) {
		t.Fatalf("joined dynamic facts = %#v, want dynamicindex map-domain join %#v", joined.DynamicIndexFacts(), wantDynamic)
	}
	gotDynamic, _ := joined.DynamicIndexFact(dynCommon)
	if !presence.Equal(gotDynamic.KeyPresence, presence.Maybe()) || gotDynamic.Admission != dynamicindex.AdmissionUnknown {
		t.Fatalf("joined dynamic common = %#v, want joined dynamicindex fact", gotDynamic)
	}
	if got, _ := joined.DynamicIndexFact(dynLeft); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("left-only dynamic fact = %#v, want preserved fact", got)
	}
	if widened := objectDomain.Widen(left, right); !objectDomain.Equal(widened, joined) {
		t.Fatalf("widen differs from join")
	}
	if !objectDomain.LessOrEq(left, joined) || objectDomain.LessOrEq(joined, left) {
		t.Fatalf("joined object should be an upper bound")
	}
}

func TestObjectDomainPrefixStableMustIntersectAndKillsOnInvalidation(t *testing.T) {
	reg := standard.Registry()
	objectDomain := ObjectDomain(reg)
	ks := keyspace.New()
	common := fieldSuffixKey(t, ks, "host")
	leftOnly := fieldSuffixKey(t, ks, "left")
	rightOnly := fieldSuffixKey(t, ks, "right")
	present := presentValue(reg)

	left := NewTableObject(TableObjectConfig{
		Root:              present,
		PrefixStableShape: true,
		StaticMembers: map[keyspace.Key]product.Value{
			common:   present,
			leftOnly: present,
		},
	})
	right := NewTableObject(TableObjectConfig{
		Root:              present,
		PrefixStableShape: true,
		StaticMembers: map[keyspace.Key]product.Value{
			common:    present,
			rightOnly: present,
		},
	})

	joined := objectDomain.Join(left, right)
	if !joined.PrefixStableShape() {
		t.Fatalf("prefix-stable marker did not survive agreeing join")
	}
	if _, ok := joined.StaticMember(common); !ok {
		t.Fatalf("common prefix member missing after join")
	}
	if _, ok := joined.StaticMember(leftOnly); ok {
		t.Fatalf("left-only member survived prefix must-intersection")
	}
	if _, ok := joined.StaticMember(rightOnly); ok {
		t.Fatalf("right-only member survived prefix must-intersection")
	}
	if widened := objectDomain.Widen(left, right); !objectDomain.Equal(widened, joined) {
		t.Fatalf("prefix widen differs from join: %#v vs %#v", widened, joined)
	}
	if generic := right.WithoutPrefixStableShape(); objectDomain.Join(left, generic).PrefixStableShape() {
		t.Fatalf("prefix marker survived join with generic object")
	}
	if extended, ok := left.WithStaticMember(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "port"}}, present); !ok || !extended.PrefixStableShape() {
		t.Fatalf("monotone static addition did not preserve prefix marker")
	}
	if removed, ok := left.WithoutStaticMemberSubtree(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "host"}}); !ok || removed.PrefixStableShape() {
		t.Fatalf("static member invalidation did not kill prefix marker")
	}
}

func TestTableObjectInvalidatesDynamicIndexFactsBySuffix(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	present := presentValue(reg)
	keptKey := dynamicindex.Key{Table: stateKey(t, ks, "sym90@1.other"), Site: "kept"}
	exactKey := dynamicindex.Key{Table: stateKey(t, ks, "sym90@1.active"), Site: "exact"}
	childKey := dynamicindex.Key{Table: stateKey(t, ks, "sym90@1.active.value"), Site: "child"}
	fact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	object := NewTableObject(TableObjectConfig{
		Root: present,
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			keptKey:  fact,
			exactKey: fact,
			childKey: fact,
		},
	})

	activeSuffix := []segment.Segment{{Kind: segment.SegmentField, Name: "active"}}
	desc, changed := object.WithoutDynamicIndexFactDescendants(ks, activeSuffix)
	if !changed {
		t.Fatalf("WithoutDynamicIndexFactDescendants reported unchanged")
	}
	if _, ok := desc.DynamicIndexFact(childKey); ok {
		t.Fatalf("child dynamic-index fact survived descendant invalidation")
	}
	if _, ok := desc.DynamicIndexFact(exactKey); !ok {
		t.Fatalf("exact dynamic-index fact was removed by descendant invalidation")
	}
	if _, ok := desc.DynamicIndexFact(keptKey); !ok {
		t.Fatalf("sibling dynamic-index fact was removed by descendant invalidation")
	}

	subtree, changed := object.WithoutDynamicIndexFactSubtree(ks, activeSuffix)
	if !changed {
		t.Fatalf("WithoutDynamicIndexFactSubtree reported unchanged")
	}
	if _, ok := subtree.DynamicIndexFact(exactKey); ok {
		t.Fatalf("exact dynamic-index fact survived subtree invalidation")
	}
	if _, ok := subtree.DynamicIndexFact(childKey); ok {
		t.Fatalf("child dynamic-index fact survived subtree invalidation")
	}
	if _, ok := subtree.DynamicIndexFact(keptKey); !ok {
		t.Fatalf("sibling dynamic-index fact was removed by subtree invalidation")
	}

	suffix, changed := object.WithoutDynamicIndexFactSubtree(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "value"}})
	if !changed {
		t.Fatalf("WithoutDynamicIndexFactSubtree by suffix reported unchanged")
	}
	if _, ok := suffix.DynamicIndexFact(childKey); ok {
		t.Fatalf("child dynamic-index fact survived suffix subtree invalidation")
	}
	if _, ok := suffix.DynamicIndexFact(exactKey); !ok {
		t.Fatalf("exact dynamic-index fact was removed by suffix subtree invalidation")
	}
}

func TestMapDomainJoinsPointwiseByIdentity(t *testing.T) {
	reg := standard.Registry()
	domain := MapDomain(reg)
	valueDomain := product.Domain(reg)
	id := identity.ID{Kind: "table", Site: "alloc", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "alloc", Index: 2}
	present := presentValue(reg)
	absent := absentValue(reg)

	left := map[identity.ID]TableObject{
		id:      NewTableObject(TableObjectConfig{Root: present}),
		otherID: NewTableObject(TableObjectConfig{Root: present}),
	}
	right := map[identity.ID]TableObject{
		id: NewTableObject(TableObjectConfig{Root: absent}),
	}

	joined := domain.Join(left, right)
	if got := joined[id].Root(); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared identity root = %s, want top", formatValue(reg, got))
	}
	if got := joined[otherID].Root(); !valueDomain.Equal(got, present) {
		t.Fatalf("joined disjoint identity root = %s, want present", formatValue(reg, got))
	}
	if !domain.LessOrEq(left, joined) || !domain.LessOrEq(right, joined) {
		t.Fatalf("map join should be an upper bound")
	}
	if !domain.Equal(domain.Join(domain.Bottom(), left), left) {
		t.Fatalf("map bottom should be join identity")
	}
	if !domain.Equal(domain.Join(domain.Top(), left), domain.Top()) {
		t.Fatalf("map top should absorb join")
	}
}

func TestMapDomainTopStableAcrossRepeatedConstruction(t *testing.T) {
	reg := standard.Registry()
	top := MapDomain(reg).Top()
	domain := MapDomain(reg)
	if !domain.Equal(top, domain.Top()) {
		t.Fatalf("reconstructed map domain did not recognize prior top sentinel")
	}
}

func TestNewTableObjectDefensivelyCopiesInputMaps(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	staticKey := fieldSuffixKey(t, ks, "name")
	dynKey := dynamicindex.Key{Table: stateKey(t, ks, "sym91@1.table"), Site: "dyn"}
	present := presentValue(reg)
	absent := absentValue(reg)
	presentFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	absentFact := dynamicindex.Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absent,
		Value:       absent,
		Admission:   dynamicindex.AdmissionRejected,
	}
	staticMembers := map[keyspace.Key]product.Value{staticKey: present}
	dynamicFacts := map[dynamicindex.Key]dynamicindex.Fact{dynKey: presentFact}

	object := NewTableObject(TableObjectConfig{
		Root:              present,
		StaticMembers:     staticMembers,
		DynamicIndexFacts: dynamicFacts,
	})
	staticMembers[staticKey] = absent
	dynamicFacts[dynKey] = absentFact

	if got, _ := object.StaticMember(staticKey); !product.Domain(reg).Equal(got, present) {
		t.Fatalf("NewTableObject shared input static map")
	}
	if got, _ := object.DynamicIndexFact(dynKey); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("NewTableObject shared input dynamic map")
	}
}

func TestNewOwnedStaticTableObjectKeepsAccessorsDefensive(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	staticKey := fieldSuffixKey(t, ks, "name")
	present := presentValue(reg)
	absent := absentValue(reg)
	object := NewOwnedStaticTableObject(present, map[keyspace.Key]product.Value{staticKey: present})

	members := object.StaticMembers()
	members[staticKey] = absent
	if got, _ := object.StaticMember(staticKey); !product.Domain(reg).Equal(got, present) {
		t.Fatalf("owned static table object exposed mutable static members")
	}
	if len(NewOwnedStaticTableObject(present, map[keyspace.Key]product.Value{}).staticMembers) != 0 {
		t.Fatalf("owned static table object should normalize empty static maps")
	}
}

func TestCloneObjectAndMapIndependence(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	id := identity.ID{Kind: "table", Site: "clone", Index: 1}
	staticKey := fieldSuffixKey(t, ks, "name")
	dynKey := dynamicindex.Key{Table: stateKey(t, ks, "sym91@1.table"), Site: "dyn"}
	present := presentValue(reg)
	absent := absentValue(reg)
	presentFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	absentFact := dynamicindex.Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absent,
		Value:       absent,
		Admission:   dynamicindex.AdmissionRejected,
	}
	object := NewTableObject(TableObjectConfig{
		Root:          present,
		StaticMembers: map[keyspace.Key]product.Value{staticKey: present},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynKey: presentFact,
		},
	})

	clone := CloneObject(object)
	cloneStatic := clone.StaticMembers()
	cloneDynamic := clone.DynamicIndexFacts()
	cloneStatic[staticKey] = absent
	cloneDynamic[dynKey] = absentFact
	if got, _ := object.StaticMember(staticKey); !product.Domain(reg).Equal(got, present) {
		t.Fatalf("object clone mutation changed static member")
	}
	if got, _ := object.DynamicIndexFact(dynKey); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("object clone mutation changed dynamic fact")
	}

	objects := map[identity.ID]TableObject{id: object}
	mapClone := CloneMap(objects)
	mapCloneStatic := mapClone[id].StaticMembers()
	mapCloneDynamic := mapClone[id].DynamicIndexFacts()
	mapCloneStatic[staticKey] = absent
	mapCloneDynamic[dynKey] = absentFact
	if got, _ := objects[id].StaticMember(staticKey); !product.Domain(reg).Equal(got, present) {
		t.Fatalf("map clone mutation changed static member")
	}
	if got, _ := objects[id].DynamicIndexFact(dynKey); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("map clone mutation changed dynamic fact")
	}

	withoutStatic, changed := mapClone[id].WithoutStaticMemberSubtree(ks, []segment.Segment{
		{Kind: segment.SegmentField, Name: "name"},
	})
	if !changed {
		t.Fatalf("WithoutStaticMemberSubtree on cloned map object reported no change")
	}
	mapClone[id] = withoutStatic
	if got, ok := objects[id].StaticMember(staticKey); !ok || !product.Domain(reg).Equal(got, present) {
		t.Fatalf("copy-on-write object update through cloned map changed original static member: %v/%v", got, ok)
	}
}

func TestDeleteEntrySemantics(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	id := identity.ID{Kind: "table", Site: "delete", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "delete", Index: 2}
	staticKey := stateKey(t, ks, "sym92@1.table.name")
	present := presentValue(reg)
	object := NewTableObject(TableObjectConfig{
		Root:          present,
		StaticMembers: map[keyspace.Key]product.Value{staticKey: present},
	})

	out, removed := DeleteEntry(map[identity.ID]TableObject{id: object, otherID: object}, id)
	if !removed {
		t.Fatalf("delete should report removal")
	}
	if _, ok := out[id]; ok {
		t.Fatalf("delete retained removed identity")
	}
	outStatic := out[otherID].StaticMembers()
	outStatic[staticKey] = absentValue(reg)
	if got, _ := object.StaticMember(staticKey); !product.Domain(reg).Equal(got, present) {
		t.Fatalf("delete output shares object maps with input")
	}
	if _, removed := DeleteEntry(out, id); removed {
		t.Fatalf("deleting a missing identity should report false")
	}
	if out, removed := DeleteEntry(map[identity.ID]TableObject{id: object}, id); !removed || out != nil {
		t.Fatalf("deleting last entry should return nil/true, got %#v/%v", out, removed)
	}
}

func TestStaticMemberSuffixKeyUsesCanonicalRelativeSegments(t *testing.T) {
	ks := keyspace.New()
	got, ok := StaticMemberSuffixKey(ks, []segment.Segment{
		{Kind: segment.SegmentField, Name: "id"},
		{Kind: segment.SegmentIndexString, Name: "name"},
		{Kind: segment.SegmentIndexInt, Index: 1},
	})
	if !ok || ks.Format(got) != pathdom.PathKey(".id[\"name\"][1]") {
		t.Fatalf("StaticMemberSuffixKey = %q/%v, want .id[\"name\"][1]/true", ks.Format(got), ok)
	}

	if got, ok := StaticMemberSuffixKey(ks, nil); ok || (got != keyspace.Key{}) {
		t.Fatalf("StaticMemberSuffixKey(nil) = %q/%v, want empty/false", ks.Format(got), ok)
	}
}

func stateKey(t *testing.T, ks *keyspace.KeySpace, name string) keyspace.Key {
	t.Helper()
	k, ok := ks.FromStateKey(pathdom.PathKey(name))
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", name)
	}
	return k
}

func fieldSuffixKey(t *testing.T, ks *keyspace.KeySpace, name string) keyspace.Key {
	t.Helper()
	k, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
	if !ok {
		t.Fatalf("FromRootlessSuffix(%q) failed", name)
	}
	return k
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	case presence.Equal(product.PresenceOf(v), presence.Present()):
		return "present"
	case presence.Equal(product.PresenceOf(v), presence.Absent()):
		return "absent"
	default:
		return product.PresenceOf(v).String()
	}
}
