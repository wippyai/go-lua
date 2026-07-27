package state

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestHeapTableIdentityFactorExactInversePreservesObjectMetadata(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	fieldSuffix := []segment.Segment{{Kind: segment.SegmentField, Name: "name"}}
	indexSuffix := []segment.Segment{{Kind: segment.SegmentIndexString, Name: "alias"}}
	fieldKey, ok := keys.FromRootlessSuffix(fieldSuffix)
	if !ok {
		t.Fatal("failed to intern field suffix")
	}
	indexKey, ok := keys.FromRootlessSuffix(indexSuffix)
	if !ok {
		t.Fatal("failed to intern string-index suffix")
	}
	aliasKey, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "alias"}})
	if !ok {
		t.Fatal("failed to intern alias mirror")
	}
	firstID := identity.ID{Kind: "table", Site: "heap-factor", Index: 1}
	secondID := identity.ID{Kind: "table", Site: "heap-factor", Index: 2}
	root := product.Top()
	dynamicFact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue: root, HasKeyValue: true, Value: product.Absent(reg), HasValue: true,
		Admission: dynamicindex.AdmissionAdmitted,
	})
	first := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: root,
		StaticMembers: map[keyspace.Key]product.Value{
			fieldKey: product.Absent(reg),
			indexKey: root,
			aliasKey: root,
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			{Table: fieldKey, Site: dynamicindex.Site("heap-factor")}: dynamicFact,
		},
		StableShape: true,
	})
	second := heapidentity.TopObject().WithRoot(product.Absent(reg))
	second, ok = second.WithStaticMember(reg, keys, indexSuffix, root)
	if !ok {
		t.Fatal("failed to construct dynamic-top object")
	}
	second = second.WithPrefixStableShape()
	stateValue := domain.Lattice().Bottom().
		WriteHeapTableObject(reg, secondID, second).
		WriteHeapTableObject(reg, firstID, first)
	factor := onlyHeapTableIdentityFactor(t, domain, stateValue)

	skeleton, roots, members, err := domain.DecomposeHeapTableIdentity(factor, keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 || roots[0].Identity() != firstID || roots[1].Identity() != secondID {
		t.Fatalf("roots = %#v, want identity-sorted roots", roots)
	}
	if len(members) != 5 {
		t.Fatalf("member count = %d, want 5", len(members))
	}
	for index := 1; index < len(members); index++ {
		prior, current := members[index-1], members[index]
		if prior.Identity() == current.Identity() && !keys.Less(prior.Key(), current.Key()) {
			t.Fatal("members are not deterministically key ordered")
		}
	}
	recomposed, err := domain.ComposeHeapTableIdentity(skeleton, roots, members, keys)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.LaneEqual(factor, recomposed)
	if err != nil || !equal {
		t.Fatalf("decompose/compose inverse equality = %t, err=%v", equal, err)
	}
	if _, err := domain.ComposeHeapTableIdentity(skeleton, roots, append(members, members[0]), keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("duplicate compose error = %v, want ErrInvalidLaneFactor", err)
	}
	if _, err := domain.ComposeHeapTableIdentity(skeleton, append(roots, roots[0]), members, keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("duplicate root compose error = %v, want ErrInvalidLaneFactor", err)
	}
	if _, err := domain.ComposeHeapTableIdentity(skeleton, roots[:1], members, keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("missing root compose error = %v, want ErrInvalidLaneFactor", err)
	}
	if _, _, _, err := domain.DecomposeHeapTableIdentity(factor, keyspace.New()); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign-keyspace decomposition error = %v, want ErrInvalidLaneFactor", err)
	}
}

func TestHeapTableIdentityFactorPreservesFormalVocabularyCoordinates(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	skeleton, err := domain.HeapTableIdentitySkeletonBottom(keys)
	if err != nil {
		t.Fatal(err)
	}
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 0x72
	schema := identity.NewFormalSchemaID(owner, 3)
	input := identity.FormalTerm(identity.NewFormalVar(schema, formal.Input))
	output := identity.FormalTerm(identity.NewFormalVar(schema, formal.Output))
	roots := make([]HeapObjectRootFactor, 0, 2)
	for _, term := range []identity.Term{input, output} {
		var slot HeapObjectRootSlot
		skeleton, slot, _, err = domain.installHeapTableTermConstructor(skeleton, term, HeapTableConstructorConfig{})
		if err != nil {
			t.Fatal(err)
		}
		factor, bindErr := domain.BindHeapObjectRootValue(slot, product.Top())
		if bindErr != nil {
			t.Fatal(bindErr)
		}
		roots = append(roots, factor)
	}
	composed, err := domain.ComposeHeapTableIdentity(skeleton, roots, nil, keys)
	if err != nil {
		t.Fatal(err)
	}
	_, got, members, err := domain.DecomposeHeapTableIdentity(composed, keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(members) != 0 {
		t.Fatalf("formal heap coordinates roots=%d members=%d", len(got), len(members))
	}
	seen := map[identity.Term]bool{}
	for _, root := range got {
		seen[root.IdentityTerm()] = true
		if root.Identity() != (identity.ID{}) {
			t.Fatalf("formal heap coordinate escaped as concrete identity: %#v", root.Identity())
		}
	}
	if !seen[input] || !seen[output] {
		t.Fatalf("formal vocabulary coordinates not preserved: %#v", seen)
	}
	recomposed, err := domain.ComposeHeapTableIdentity(skeleton, got, nil, keys)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.LaneEqual(composed, recomposed)
	if err != nil || !equal {
		t.Fatalf("formal heap inverse equal=%t err=%v", equal, err)
	}
}

func TestHeapTableIdentitySkeletonMustMapPresenceLaws(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	id := identity.ID{Kind: "table", Site: "heap-presence-law", Index: 1}
	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	key, ok := keys.FromRootlessSuffix(suffix)
	if !ok {
		t.Fatal("failed to intern suffix")
	}
	bottom, err := domain.HeapTableIdentitySkeletonBottom(keys)
	if err != nil {
		t.Fatal(err)
	}
	present, presentRootSlot, slots, err := domain.InstallHeapTableConstructor(bottom, HeapTableConstructorConfig{
		Identity: id, MemberSuffixes: [][]segment.Segment{suffix}, StableShape: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	absent, absentRootSlot, absentTemplates, err := domain.InstallHeapTableConstructor(bottom, HeapTableConstructorConfig{
		Identity: id, StableShape: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || len(absentTemplates) != 0 {
		t.Fatalf("constructor slots = %d/%d, want 1/0", len(slots), len(absentTemplates))
	}
	if _, explicit, err := domain.HeapTableIdentitySkeletonStaticMemberDefault(present, id, key); err != nil || !explicit {
		t.Fatalf("present key query = %t, err=%v", explicit, err)
	}
	if defaultValue, explicit, err := domain.HeapTableIdentitySkeletonStaticMemberDefault(absent, id, key); err != nil || explicit || !product.Equal(reg, defaultValue, product.Top()) {
		t.Fatalf("present-object absent-key query explicit=%t err=%v", explicit, err)
	}
	if defaultValue, explicit, err := domain.HeapTableIdentitySkeletonStaticMemberDefault(bottom, id, key); err != nil || explicit || !product.Equal(reg, defaultValue, product.Bottom(reg)) {
		t.Fatalf("absent-object query explicit=%t err=%v", explicit, err)
	}

	// A missing MustMap coordinate is Top: present <= absent, while the reverse
	// is rejected by key presence independently of any product value.
	if !product.Domain(reg).LessOrEq(product.Absent(reg), product.Top()) {
		t.Fatal("member value does not satisfy v <= absent-coordinate Top")
	}
	if le, err := domain.HeapTableIdentitySkeletonLessOrEq(present, absent); err != nil || !le {
		t.Fatalf("present <= absent = %t, err=%v", le, err)
	}
	if le, err := domain.HeapTableIdentitySkeletonLessOrEq(absent, present); err != nil || le {
		t.Fatalf("absent <= present = %t, err=%v", le, err)
	}

	joined, err := domain.HeapTableIdentitySkeletonJoin(present, absent)
	if err != nil {
		t.Fatal(err)
	}
	if _, explicit, _ := domain.HeapTableIdentitySkeletonStaticMemberDefault(joined, id, key); explicit {
		t.Fatal("MustMap Join retained a key absent from one branch")
	}
	met, err := domain.HeapTableIdentitySkeletonMeet(present, absent)
	if err != nil {
		t.Fatal(err)
	}
	if _, explicit, _ := domain.HeapTableIdentitySkeletonStaticMemberDefault(met, id, key); !explicit {
		t.Fatal("MustMap Meet lost the present branch key")
	}
	narrowed, err := domain.HeapTableIdentitySkeletonNarrow(present, absent)
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.HeapTableIdentitySkeletonEqual(narrowed, present); err != nil || !equal {
		t.Fatalf("heap skeleton Narrow did not preserve previous: equal=%t err=%v", equal, err)
	}
	joinedWithAbsentObject, err := domain.HeapTableIdentitySkeletonJoin(bottom, present)
	if err != nil {
		t.Fatal(err)
	}
	if _, explicit, _ := domain.HeapTableIdentitySkeletonStaticMemberDefault(joinedWithAbsentObject, id, key); !explicit {
		t.Fatal("outer Map Join treated absent object like present-object absent key")
	}

	// Strict composition accepts present Top but rejects the same coordinate
	// when the skeleton says it is absent; callers must omit absent roots.
	topMember, err := domain.BindHeapStaticMemberValue(slots[0], product.Top())
	if err != nil {
		t.Fatal(err)
	}
	presentRoot, err := domain.BindHeapObjectRootValue(presentRootSlot, product.Absent(reg))
	if err != nil {
		t.Fatal(err)
	}
	absentRoot, err := domain.BindHeapObjectRootValue(absentRootSlot, product.Absent(reg))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ComposeHeapTableIdentity(present, []HeapObjectRootFactor{presentRoot}, []HeapStaticMemberFactor{topMember}, keys); err != nil {
		t.Fatalf("present Top compose: %v", err)
	}
	if _, err := domain.ComposeHeapTableIdentity(absent, []HeapObjectRootFactor{absentRoot}, []HeapStaticMemberFactor{topMember}, keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("absent coordinate compose error = %v, want ErrInvalidLaneFactor", err)
	}
}

func TestHeapTableIdentityObjectRootIsIndependentFromSkeleton(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	id := identity.ID{Kind: "table", Site: "heap-root-law", Index: 1}
	bottom, err := domain.HeapTableIdentitySkeletonBottom(keys)
	if err != nil {
		t.Fatal(err)
	}
	if defaultRoot, explicit, err := domain.HeapTableIdentitySkeletonObjectRootDefault(bottom, id); err != nil || explicit || !product.Equal(reg, defaultRoot, product.Bottom(reg)) {
		t.Fatalf("absent root default explicit=%t err=%v", explicit, err)
	}
	skeleton, rootSlot, _, err := domain.InstallHeapTableConstructor(bottom, HeapTableConstructorConfig{Identity: id})
	if err != nil {
		t.Fatal(err)
	}
	leftRoot, err := domain.BindHeapObjectRootValue(rootSlot, product.Absent(reg))
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := domain.WithHeapObjectRootValue(leftRoot, product.Top())
	if err != nil {
		t.Fatal(err)
	}
	if product.Equal(reg, leftRoot.Value(), rightRoot.Value()) {
		t.Fatal("independent root factors did not retain different values")
	}
	hashBefore, err := domain.HeapTableIdentitySkeletonFingerprint(FingerprintConfig{Registry: reg, KeySpace: keys}, skeleton)
	if err != nil {
		t.Fatal(err)
	}
	hashAfter, err := domain.HeapTableIdentitySkeletonFingerprint(FingerprintConfig{Registry: reg, KeySpace: keys}, skeleton)
	if err != nil {
		t.Fatal(err)
	}
	if hashBefore != hashAfter {
		t.Fatal("skeleton fingerprint changed with no skeleton-coordinate change")
	}
	visited := 0
	if err := domain.VisitHeapTableIdentitySkeletonValueDependencies(skeleton, func(product.Value) { visited++ }); err != nil {
		t.Fatal(err)
	}
	if visited != 0 {
		t.Fatalf("skeleton value dependencies = %d, want no embedded root", visited)
	}
	if _, explicit, err := domain.HeapTableIdentitySkeletonObjectRootDefault(skeleton, id); err != nil || !explicit {
		t.Fatalf("present root query explicit=%t err=%v", explicit, err)
	}
	left, err := domain.ComposeHeapTableIdentity(skeleton, []HeapObjectRootFactor{leftRoot}, nil, keys)
	if err != nil {
		t.Fatal(err)
	}
	right, err := domain.ComposeHeapTableIdentity(skeleton, []HeapObjectRootFactor{rightRoot}, nil, keys)
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.LaneEqual(left, right); err != nil || equal {
		t.Fatalf("recomposed roots lane equality=%t err=%v, want distinct", equal, err)
	}
}

func TestHeapObjectRootAuthorityAndInventoryAreExact(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	firstID := identity.ID{Kind: "table", Site: "heap-root-authority", Index: 1}
	secondID := identity.ID{Kind: "table", Site: "heap-root-authority", Index: 2}
	memberSuffix := []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}
	bottom, err := domain.HeapTableIdentitySkeletonBottom(keys)
	if err != nil {
		t.Fatal(err)
	}
	firstSkeleton, firstSlot, firstMemberSlots, err := domain.InstallHeapTableConstructor(bottom, HeapTableConstructorConfig{
		Identity: firstID, MemberSuffixes: [][]segment.Segment{memberSuffix},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRoot, err := domain.BindHeapObjectRootValue(firstSlot, product.Absent(reg))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := peer.BindHeapObjectRootValue(firstSlot, product.Absent(reg)); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign-domain bind error = %v, want ErrInvalidLaneFactor", err)
	}
	if _, err := peer.ImportHeapObjectRootSlot(firstSlot, keyspace.New()); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign-keyspace import error = %v, want ErrInvalidLaneFactor", err)
	}
	importedSlot, err := peer.ImportHeapObjectRootSlot(firstSlot, keys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := peer.BindHeapObjectRootValue(importedSlot, product.Absent(reg)); err != nil {
		t.Fatalf("imported root-slot bind: %v", err)
	}
	foreignReg, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.BindHeapObjectRootValue(firstSlot, product.Absent(foreignReg)); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign-value bind error = %v, want ErrInvalidLaneFactor", err)
	}
	if len(firstMemberSlots) != 1 {
		t.Fatalf("member slots = %d, want 1", len(firstMemberSlots))
	}
	if _, err := domain.BindHeapStaticMemberValue(firstMemberSlots[0], product.Absent(foreignReg)); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign-member bind error = %v, want ErrInvalidLaneFactor", err)
	}

	secondSkeleton, secondSlot, _, err := domain.InstallHeapTableConstructor(bottom, HeapTableConstructorConfig{Identity: secondID})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := domain.BindHeapObjectRootValue(secondSlot, product.Top())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ComposeHeapTableIdentity(firstSkeleton, []HeapObjectRootFactor{firstRoot, secondRoot}, nil, keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("extra root compose error = %v, want ErrInvalidLaneFactor", err)
	}
	if _, err := domain.ComposeHeapTableIdentity(secondSkeleton, []HeapObjectRootFactor{firstRoot}, nil, keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("wrong root compose error = %v, want ErrInvalidLaneFactor", err)
	}

	lane, ok := domain.ProductLane(LaneHeapTableIdentity)
	if !ok {
		t.Fatal("heap lane is not enabled")
	}
	topFactor, err := domain.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	topSkeleton, roots, members, err := domain.DecomposeHeapTableIdentity(topFactor, keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 0 || len(members) != 0 {
		t.Fatalf("top decomposition roots/members = %d/%d, want 0/0", len(roots), len(members))
	}
	defaultRoot, explicit, err := domain.HeapTableIdentitySkeletonObjectRootDefault(topSkeleton, firstID)
	if err != nil || explicit || !product.Equal(reg, defaultRoot, product.Top()) {
		t.Fatalf("top root default explicit=%t err=%v", explicit, err)
	}
	if _, err := domain.ComposeHeapTableIdentity(topSkeleton, []HeapObjectRootFactor{firstRoot}, nil, keys); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("top with finite root error = %v, want ErrInvalidLaneFactor", err)
	}
	if _, err := domain.ComposeHeapTableIdentity(topSkeleton, nil, nil, keys); err != nil {
		t.Fatalf("top exact compose: %v", err)
	}
}

func onlyHeapTableIdentityFactor(t *testing.T, domain ProductDomain, value State) LaneFactor {
	t.Helper()
	lane, ok := domain.ProductLane(LaneHeapTableIdentity)
	if !ok {
		t.Fatal("heap lane is not enabled")
	}
	factors, err := domain.DecomposeLanes(value, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	return factors[0]
}
