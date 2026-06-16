package heapidentity

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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

func TestObjectJoinWidenRootStaticAndDynamic(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	objectDomain := ObjectDomain(reg)
	dynamicDomain := dynamicindex.MapDomain(reg)
	staticCommon := pathdom.PathKey("sym90@1.table.name")
	staticLeft := pathdom.PathKey("sym90@1.table.left")
	staticRight := pathdom.PathKey("sym90@1.table.right")
	dynCommon := dynamicindex.Key{Table: pathdom.PathKey("sym90@1.table"), Site: "dyn"}
	dynLeft := dynamicindex.Key{Table: pathdom.PathKey("sym90@1.table"), Site: "left"}
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
		StaticMembers: map[pathdom.PathKey]product.Value{
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
		StaticMembers: map[pathdom.PathKey]product.Value{
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

func TestCloneObjectAndMapIndependence(t *testing.T) {
	reg := standard.Registry()
	id := identity.ID{Kind: "table", Site: "clone", Index: 1}
	staticKey := pathdom.PathKey("sym91@1.table.name")
	dynKey := dynamicindex.Key{Table: pathdom.PathKey("sym91@1.table"), Site: "dyn"}
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
		StaticMembers: map[pathdom.PathKey]product.Value{staticKey: present},
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
}

func TestDeleteEntrySemantics(t *testing.T) {
	reg := standard.Registry()
	id := identity.ID{Kind: "table", Site: "delete", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "delete", Index: 2}
	staticKey := pathdom.PathKey("sym92@1.table.name")
	present := presentValue(reg)
	object := NewTableObject(TableObjectConfig{
		Root:          present,
		StaticMembers: map[pathdom.PathKey]product.Value{staticKey: present},
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
	got, ok := StaticMemberSuffixKey([]segment.Segment{
		{Kind: segment.SegmentField, Name: "id"},
		{Kind: segment.SegmentIndexString, Name: "name"},
		{Kind: segment.SegmentIndexInt, Index: 1},
	})
	if !ok || got != pathdom.PathKey(".id[\"name\"][1]") {
		t.Fatalf("StaticMemberSuffixKey = %q/%v, want .id[\"name\"][1]/true", got, ok)
	}

	if got, ok := StaticMemberSuffixKey(nil); ok || got != "" {
		t.Fatalf("StaticMemberSuffixKey(nil) = %q/%v, want empty/false", got, ok)
	}
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
