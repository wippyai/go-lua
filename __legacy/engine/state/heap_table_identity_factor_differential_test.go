package state

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func TestHeapTableIdentityFactorwiseLatticeDifferential(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	key := func(suffix ...segment.Segment) keyspace.Key {
		t.Helper()
		out, ok := keys.FromRootlessSuffix(suffix)
		if !ok {
			t.Fatalf("invalid suffix %#v", suffix)
		}
		return out
	}
	field := func(name string) segment.Segment { return segment.Segment{Kind: segment.SegmentField, Name: name} }
	index := func(name string) segment.Segment {
		return segment.Segment{Kind: segment.SegmentIndexString, Name: name}
	}
	commonKey, leftKey, rightKey := key(field("common")), key(field("left")), key(field("right"))
	aliasIndex, aliasField := key(index("alias")), key(field("alias"))
	sharedID := identity.ID{Kind: "table", Site: "heap-factor-differential", Index: 1}
	leftOnlyID := identity.ID{Kind: "table", Site: "heap-factor-differential", Index: 2}

	leftShared := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{
			commonKey:  product.Bottom(reg),
			leftKey:    product.Absent(reg),
			aliasIndex: product.Bottom(reg),
			aliasField: product.Absent(reg),
		},
		StableShape: true,
	})
	rightShared := heapidentity.TopObject().WithRoot(product.Top()).WithPrefixStableShape()
	rightShared, _ = rightShared.WithStaticMember(reg, keys, []segment.Segment{field("common")}, product.Top())
	rightShared, _ = rightShared.WithStaticMember(reg, keys, []segment.Segment{field("right")}, product.Top())
	// Preserve independently stored mirror values: the string-index write creates
	// both spellings, then the field write changes only the canonical field.
	rightShared, _ = rightShared.WithStaticMember(reg, keys, []segment.Segment{index("alias")}, product.Top())
	rightShared, _ = rightShared.WithStaticMember(reg, keys, []segment.Segment{field("alias")}, product.Absent(reg))
	leftOnly := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{commonKey: product.Bottom(reg)}, PrefixStableShape: true,
	})

	left := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().
		WriteHeapTableObject(reg, sharedID, leftShared).
		WriteHeapTableObject(reg, leftOnlyID, leftOnly))
	right := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().
		WriteHeapTableObject(reg, sharedID, rightShared))

	type coordinate struct {
		id  identity.ID
		key keyspace.Key
	}
	type parts struct {
		skeleton HeapTableIdentitySkeletonFactor
		roots    map[identity.ID]HeapObjectRootFactor
		members  map[coordinate]HeapStaticMemberFactor
	}
	decompose := func(factor LaneFactor) parts {
		t.Helper()
		skeleton, roots, members, decomposeErr := domain.DecomposeHeapTableIdentity(factor, keys)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		out := parts{
			skeleton: skeleton,
			roots:    make(map[identity.ID]HeapObjectRootFactor, len(roots)),
			members:  make(map[coordinate]HeapStaticMemberFactor, len(members)),
		}
		for _, root := range roots {
			out.roots[root.Identity()] = root
		}
		for _, member := range members {
			out.members[coordinate{id: member.Identity(), key: member.Key()}] = member
		}
		return out
	}
	leftParts, rightParts := decompose(left), decompose(right)
	rootAt := func(value parts, id identity.ID) product.Value {
		t.Helper()
		if root, ok := value.roots[id]; ok {
			return root.Value()
		}
		fallback, explicit, defaultErr := domain.HeapTableIdentitySkeletonObjectRootDefault(value.skeleton, id)
		if defaultErr != nil || explicit {
			t.Fatalf("root default %v = explicit=%t err=%v", id, explicit, defaultErr)
		}
		return fallback
	}
	valueAt := func(value parts, at coordinate) product.Value {
		t.Helper()
		if member, ok := value.members[at]; ok {
			return member.Value()
		}
		fallback, explicit, defaultErr := domain.HeapTableIdentitySkeletonStaticMemberDefault(value.skeleton, at.id, at.key)
		if defaultErr != nil || explicit {
			t.Fatalf("default %v/%v = explicit=%t err=%v", at.id, keys.FormatReadOnly(at.key), explicit, defaultErr)
		}
		return fallback
	}
	recompose := func(name string, skeleton HeapTableIdentitySkeletonFactor, previous, next parts) LaneFactor {
		t.Helper()
		identities := make(map[identity.ID]struct{}, len(previous.roots)+len(next.roots))
		for id := range previous.roots {
			identities[id] = struct{}{}
		}
		for id := range next.roots {
			identities[id] = struct{}{}
		}
		orderedIdentities := make([]identity.ID, 0, len(identities))
		for id := range identities {
			orderedIdentities = append(orderedIdentities, id)
		}
		sort.Slice(orderedIdentities, func(i, j int) bool {
			return identityIDLess(orderedIdentities[i], orderedIdentities[j])
		})
		roots := make([]HeapObjectRootFactor, 0, len(orderedIdentities))
		for _, id := range orderedIdentities {
			_, explicit, defaultErr := domain.HeapTableIdentitySkeletonObjectRootDefault(skeleton, id)
			if defaultErr != nil {
				t.Fatal(defaultErr)
			}
			if !explicit {
				continue
			}
			template, ok := previous.roots[id]
			if !ok {
				template = next.roots[id]
			}
			var value product.Value
			switch name {
			case "join":
				value = product.Join(reg, rootAt(previous, id), rootAt(next, id))
			case "widen":
				value = product.Widen(reg, rootAt(previous, id), rootAt(next, id))
			case "meet":
				value = product.Meet(reg, rootAt(previous, id), rootAt(next, id))
			case "narrow":
				value = rootAt(previous, id)
			default:
				t.Fatalf("unknown operation %q", name)
			}
			root, bindErr := domain.WithHeapObjectRootValue(template, value)
			if bindErr != nil {
				t.Fatal(bindErr)
			}
			roots = append(roots, root)
		}

		coordinates := make(map[coordinate]struct{}, len(previous.members)+len(next.members))
		for at := range previous.members {
			coordinates[at] = struct{}{}
		}
		for at := range next.members {
			coordinates[at] = struct{}{}
		}
		ordered := make([]coordinate, 0, len(coordinates))
		for at := range coordinates {
			ordered = append(ordered, at)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].id != ordered[j].id {
				return identityIDLess(ordered[i].id, ordered[j].id)
			}
			return keys.Less(ordered[i].key, ordered[j].key)
		})
		members := make([]HeapStaticMemberFactor, 0, len(ordered))
		for _, at := range ordered {
			_, explicit, defaultErr := domain.HeapTableIdentitySkeletonStaticMemberDefault(skeleton, at.id, at.key)
			if defaultErr != nil {
				t.Fatal(defaultErr)
			}
			if !explicit {
				continue
			}
			template, ok := previous.members[at]
			if !ok {
				template = next.members[at]
			}
			var value product.Value
			switch name {
			case "join":
				value = product.Join(reg, valueAt(previous, at), valueAt(next, at))
			case "widen":
				value = product.Widen(reg, valueAt(previous, at), valueAt(next, at))
			case "meet":
				value = product.Meet(reg, valueAt(previous, at), valueAt(next, at))
			case "narrow":
				// The registered heap lane has no narrowing operator; both the
				// skeleton and every independent member retain previous exactly.
				value = valueAt(previous, at)
			default:
				t.Fatalf("unknown operation %q", name)
			}
			member, bindErr := domain.WithHeapStaticMemberValue(template, value)
			if bindErr != nil {
				t.Fatal(bindErr)
			}
			members = append(members, member)
		}
		factor, composeErr := domain.ComposeHeapTableIdentity(skeleton, roots, members, keys)
		if composeErr != nil {
			t.Fatalf("strict %s recompose: %v", name, composeErr)
		}
		return factor
	}
	assertEqual := func(name string, got, want LaneFactor) {
		t.Helper()
		equal, equalErr := domain.LaneEqual(got, want)
		if equalErr != nil || !equal {
			t.Fatalf("%s differential equality = %t, err=%v", name, equal, equalErr)
		}
	}

	joinedSkeleton, err := domain.HeapTableIdentitySkeletonJoin(leftParts.skeleton, rightParts.skeleton)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := domain.LaneJoin(left, right)
	if err != nil {
		t.Fatal(err)
	}
	assertEqual("join", recompose("join", joinedSkeleton, leftParts, rightParts), joined)

	widenedSkeleton, err := domain.HeapTableIdentitySkeletonWiden(leftParts.skeleton, rightParts.skeleton)
	if err != nil {
		t.Fatal(err)
	}
	widened, err := domain.LaneWiden(left, right)
	if err != nil {
		t.Fatal(err)
	}
	assertEqual("widen", recompose("widen", widenedSkeleton, leftParts, rightParts), widened)

	metSkeleton, err := domain.HeapTableIdentitySkeletonMeet(leftParts.skeleton, rightParts.skeleton)
	if err != nil {
		t.Fatal(err)
	}
	met := recompose("meet", metSkeleton, leftParts, rightParts)
	expectedMeetObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{
			commonKey:  product.Bottom(reg),
			leftKey:    product.Absent(reg),
			rightKey:   product.Top(),
			aliasIndex: product.Bottom(reg),
			aliasField: product.Absent(reg),
		},
		StableShape: true,
	})
	expectedMeet := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, sharedID, expectedMeetObject))
	assertEqual("meet", met, expectedMeet)

	narrowedSkeleton, err := domain.HeapTableIdentitySkeletonNarrow(leftParts.skeleton, rightParts.skeleton)
	if err != nil {
		t.Fatal(err)
	}
	narrowed, err := domain.LaneNarrow(left, right)
	if err != nil {
		t.Fatal(err)
	}
	assertEqual("narrow", recompose("narrow", narrowedSkeleton, leftParts, rightParts), narrowed)
}
