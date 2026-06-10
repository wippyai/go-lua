package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Union represents a type that can be any of its member types: T1 | T2 | ...
//
// Unions are normalized during construction:
//   - Nested unions are flattened
//   - Duplicate members are removed
//   - Never members are dropped (Never is identity for union)
//   - Any absorbs all other members (union with Any = Any)
//   - Literals are subsumed by their base types (string | "foo" = string)
//   - Single non-nil member with nil becomes Optional
//
// Members are sorted by hash for deterministic comparison and serialization.
type Union struct {
	Members               []Type
	memberHashes          []uint64
	hash                  uint64
	hasSoftMbr            bool // true if any member is a soft placeholder
	softPrunable          bool
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	containsCallableSurf  bool
	strCache              stringCache
}

// NewUnion creates a normalized union type from the given members.
// Returns Never for empty unions, the single type for one member,
// or a normalized Union for multiple distinct members.
func NewUnion(members ...Type) Type {
	if len(members) == 0 {
		return Never
	}

	if len(members) == 1 {
		return members[0]
	}

	// Flatten and collect
	flat := make([]Type, 0, len(members))
	hasNil := false
	hasAny := false
	hasUnknown := false

	var addMember func(Type)
	addMember = func(m Type) {
		if m == nil {
			return
		}

		// Unwrap Annotated to access structural type for flattening.
		// Annotations delegate Kind() to their inner type, so type
		// assertions on concrete wrappers (Union, Optional) require
		// operating on the unwrapped type.
		unwrapped := UnwrapAnnotated(m)

		switch unwrapped.Kind() {
		case kind.Never:
			return // Never is identity for union
		case kind.Unknown:
			hasUnknown = true
			return // Unknown doesn't contribute information to union
		case kind.Any:
			hasAny = true
		case kind.Nil:
			hasNil = true
		case kind.Union:
			for _, member := range unwrapped.(*Union).Members {
				addMember(member)
			}
		case kind.Optional:
			hasNil = true
			addMember(unwrapped.(*Optional).Inner)
		default:
			flat = append(flat, m)
		}
	}

	for _, m := range members {
		addMember(m)
	}

	if hasAny {
		return Any
	}

	// Deduplicate by hash + structural equality (collision-safe).
	unique, uniqueHashes := deduplicateTypesWithHashes(flat)

	// If nothing concrete remains, preserve Unknown (and Optional<Unknown> when nil present).
	if len(unique) == 0 && hasUnknown {
		if hasNil {
			return NewOptional(Unknown)
		}
		return Unknown
	}

	// Primitive subsumption:
	// number subsumes integer in the lattice, so keep number only.
	hasNumberType := false
	for _, m := range unique {
		if m.Kind() == kind.Number {
			hasNumberType = true
			break
		}
	}
	if hasNumberType {
		filtered := unique[:0]
		for _, m := range unique {
			if m.Kind() == kind.Integer {
				continue
			}
			filtered = append(filtered, m)
		}
		unique = filtered
	}

	// Subsume literals: if a base type is present, drop literal members with matching base.
	// e.g. string | "" => string, number | 42 => number
	var baseMask uint8
	for _, m := range unique {
		switch m.Kind() {
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
	}
	if baseMask != 0 {
		filtered := unique[:0]
		for _, m := range unique {
			if lit, ok := m.(*Literal); ok {
				drop := false
				switch lit.Base {
				case kind.String:
					drop = baseMask&1 != 0
				case kind.Number:
					drop = baseMask&2 != 0
				case kind.Integer:
					drop = baseMask&4 != 0 || baseMask&2 != 0
				case kind.Boolean:
					drop = baseMask&8 != 0
				}
				if drop {
					continue
				}
			}
			filtered = append(filtered, m)
		}
		unique = filtered
	}

	// Sort by hash then string for deterministic order. Keep the precomputed
	// hashes paired with their members because recursive products are expensive
	// to hash.
	slots := make([]hashedType, len(unique))
	for i, m := range unique {
		slots[i] = hashedType{typ: m, hash: uniqueHashes[i]}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].hash != slots[j].hash {
			return slots[i].hash < slots[j].hash
		}
		return slots[i].typ.String() < slots[j].typ.String()
	})
	for i, slot := range slots {
		unique[i] = slot.typ
		uniqueHashes[i] = slot.hash
	}

	// Add nil back if present
	if hasNil {
		// If single member + nil, return Optional
		if len(unique) == 1 {
			return NewOptional(unique[0])
		}

		unique = append([]Type{Nil}, unique...)
		uniqueHashes = append([]uint64{Nil.Hash()}, uniqueHashes...)
	}

	if len(unique) == 0 {
		if hasNil {
			return Nil
		}

		return Never
	}

	if len(unique) == 1 {
		return unique[0]
	}

	return newNormalizedUnion(unique, uniqueHashes)
}

func newNormalizedUnion(members []Type, memberHashes []uint64) Type {
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return members[0]
	}
	if len(memberHashes) != len(members) {
		memberHashes = make([]uint64, len(members))
		for i, m := range members {
			memberHashes[i] = unionMemberHash(m)
		}
	}

	membersCopy := make([]Type, len(members))
	copy(membersCopy, members)
	hashesCopy := make([]uint64, len(memberHashes))
	copy(hashesCopy, memberHashes)

	// Compute hash and check for soft members
	h := uint64(kind.Union)
	hasSoft := false
	hasNonSoft := false
	softPrunable := false
	containsAny := false
	containsNever := false
	containsTypeParam := false
	containsInstantiated := false
	containsRecursive := false
	containsOpenRecursive := false
	containsCallableSurf := false
	for i, m := range membersCopy {
		h = hash.HashCombine(h, hashesCopy[i])
		if softPruneMayRewrite(m) {
			softPrunable = true
		}
		if !containsAny && knownContainsAny(m) {
			containsAny = true
		}
		if !containsNever && knownContainsNever(m) {
			containsNever = true
		}
		if !containsTypeParam && knownContainsTypeParam(m) {
			containsTypeParam = true
		}
		if !containsInstantiated && knownContainsInstantiated(m) {
			containsInstantiated = true
		}
		if !containsRecursive && knownContainsRecursive(m) {
			containsRecursive = true
		}
		if !containsOpenRecursive && unionMemberContainsOpenRecursive(m) {
			containsOpenRecursive = true
		}
		if !containsCallableSurf && HasCallableSurface(m) {
			containsCallableSurf = true
		}
		if memberIsSoft(m) {
			hasSoft = true
		} else {
			hasNonSoft = true
		}
	}
	if hasSoft && hasNonSoft {
		softPrunable = true
	}

	return &Union{
		Members:               membersCopy,
		memberHashes:          hashesCopy,
		hash:                  h,
		hasSoftMbr:            hasSoft,
		softPrunable:          softPrunable,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
		containsCallableSurf:  containsCallableSurf,
	}
}

// HasSoftMember reports whether any union member is a soft placeholder type.
// Computed at construction time for O(1) access.
func (u *Union) HasSoftMember() bool { return u.hasSoftMbr }

func (u *Union) Kind() kind.Kind { return kind.Union }

// memberIsSoft checks if a type is a soft placeholder.
// Used at Union construction to set the hasSoftMbr flag.
// Mirrors isSoft logic but without recursion guard (unions are already flat).
func memberIsSoft(t Type) bool {
	if t == nil {
		return false
	}
	for {
		if ann, ok := t.(*Annotated); ok && ann.Inner != nil {
			t = ann.Inner
			continue
		}
		break
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	switch v := t.(type) {
	case *Optional:
		return memberIsSoft(v.Inner)
	case *Alias:
		return memberIsSoft(v.Target)
	case *Array:
		return memberIsSoft(v.Element)
	case *Map:
		return memberIsSoft(v.Value)
	case *Record:
		if len(v.Fields) == 0 && !v.HasMapComponent() {
			return true
		}
		if v.HasMapComponent() && len(v.Fields) == 0 {
			return memberIsSoft(v.MapValue)
		}
		return false
	case *Union:
		for _, m := range v.Members {
			if !memberIsSoft(m) {
				return false
			}
		}
		return len(v.Members) > 0
	}
	return false
}

func (u *Union) String() string {
	return u.strCache.get(func() string {
		parts := make([]string, len(u.Members))
		for i, m := range u.Members {
			if m == nil {
				parts[i] = "nil"
			} else {
				parts[i] = m.String()
			}
		}
		return strings.Join(parts, " | ")
	})
}

func (u *Union) Hash() uint64 { return u.hash }

func (u *Union) Equals(other Type) bool {
	return TypeEquals(u, other)
}

// ProjectUnionMembers applies a projection to the members of an already
// normalized union. If the projection only drops members, the existing member
// hash vector is reused so path-sensitive filters do not rehash recursive
// products. If any member is rewritten, the result is re-normalized through
// NewUnion because rewrites may introduce duplicates or subsumed literals.
func ProjectUnionMembers(u *Union, project func(Type) Type) Type {
	if u == nil {
		return Never
	}
	if project == nil {
		return u
	}
	kept := make([]Type, 0, len(u.Members))
	hashes := make([]uint64, 0, len(u.Members))
	changed := false
	filterOnly := true
	scalarRewriteOnly := true
	hasStoredHashes := len(u.memberHashes) == len(u.Members)
	for i, member := range u.Members {
		projected := project(member)
		if projected == nil || projected.Kind().IsNever() {
			changed = true
			continue
		}
		kept = append(kept, projected)
		if projected == member {
			if hasStoredHashes && !knownContainsOpenRecursive(member) {
				hashes = append(hashes, u.memberHashes[i])
			} else {
				hashes = append(hashes, unionMemberHash(member))
			}
			continue
		}
		changed = true
		filterOnly = false
		if !projectedUnionMemberUsesStructuralDedupe(projected) {
			scalarRewriteOnly = false
		}
		hashes = append(hashes, unionMemberHash(projected))
	}
	if !changed {
		return u
	}
	if len(kept) == 0 {
		return Never
	}
	if filterOnly {
		return newProjectedNormalizedUnion(kept, hashes)
	}
	coalesced := CoalesceProductUnionMembers(kept)
	if !sameTypeSlice(kept, coalesced) {
		return NewUnion(coalesced...)
	}
	if scalarRewriteOnly && projectedMembersStayFlatNormalized(kept) {
		return newRewrittenProjectedUnion(kept, hashes)
	}
	return NewUnion(kept...)
}

func newProjectedNormalizedUnion(members []Type, memberHashes []uint64) Type {
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return members[0]
	}
	if len(members) == 2 {
		if members[0] != nil && members[0].Kind() == kind.Nil {
			return NewOptional(members[1])
		}
		if members[1] != nil && members[1].Kind() == kind.Nil {
			return NewOptional(members[0])
		}
	}
	return newNormalizedUnion(members, memberHashes)
}

func newRewrittenProjectedUnion(members []Type, memberHashes []uint64) Type {
	unique, uniqueHashes := deduplicateProjectedTypesWithKnownHashes(members, memberHashes)
	sortHashedTypes(unique, uniqueHashes)
	return newProjectedNormalizedUnion(unique, uniqueHashes)
}

func unionMemberContainsOpenRecursive(t Type) bool {
	if rec, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return rec.Body == nil
	}
	return knownContainsOpenRecursive(t)
}

func deduplicateProjectedTypesWithKnownHashes(types []Type, hashes []uint64) ([]Type, []uint64) {
	if len(types) == 0 {
		return nil, nil
	}
	if len(hashes) != len(types) {
		return deduplicateTypesWithHashes(types)
	}

	seen := make(map[uint64][]Type)
	result := make([]Type, 0, len(types))
	resultHashes := make([]uint64, 0, len(types))
	for i, t := range types {
		if t == nil {
			continue
		}
		h := hashes[i]
		if !projectedUnionMemberUsesStructuralDedupe(t) {
			result = append(result, t)
			resultHashes = append(resultHashes, h)
			continue
		}
		bucket := seen[h]
		duplicate := false
		for _, existing := range bucket {
			if TypeEquals(existing, t) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[h] = append(bucket, t)
		result = append(result, t)
		resultHashes = append(resultHashes, h)
	}
	return result, resultHashes
}

func projectedUnionMemberUsesStructuralDedupe(t Type) bool {
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return true
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Literal, kind.Self:
		return true
	default:
		return false
	}
}

func sortHashedTypes(types []Type, hashes []uint64) {
	if len(types) != len(hashes) || len(types) < 2 {
		return
	}
	slots := make([]hashedType, len(types))
	for i, t := range types {
		slots[i] = hashedType{typ: t, hash: hashes[i]}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].hash != slots[j].hash {
			return slots[i].hash < slots[j].hash
		}
		return slots[i].typ.String() < slots[j].typ.String()
	})
	for i, slot := range slots {
		types[i] = slot.typ
		hashes[i] = slot.hash
	}
}

func projectedMembersStayFlatNormalized(members []Type) bool {
	var baseMask uint8
	var literalMask uint8
	for _, member := range members {
		if member == nil {
			return false
		}
		unwrapped := UnwrapAnnotated(member)
		switch unwrapped.Kind() {
		case kind.Never, kind.Unknown, kind.Any, kind.Nil, kind.Union, kind.Optional:
			return false
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
		if lit, ok := unwrapped.(*Literal); ok {
			switch lit.Base {
			case kind.String:
				literalMask |= 1
			case kind.Number:
				literalMask |= 2
			case kind.Integer:
				literalMask |= 4
			case kind.Boolean:
				literalMask |= 8
			}
		}
	}
	if baseMask&2 != 0 && baseMask&4 != 0 {
		return false
	}
	if baseMask&1 != 0 && literalMask&1 != 0 {
		return false
	}
	if baseMask&2 != 0 && literalMask&(2|4) != 0 {
		return false
	}
	if baseMask&4 != 0 && literalMask&4 != 0 {
		return false
	}
	if baseMask&8 != 0 && literalMask&8 != 0 {
		return false
	}
	return true
}

// UnionWithoutNil returns this normalized union without nil-capable members.
// It preserves the existing member hash vector for unchanged members so
// projection-style operations such as truthiness narrowing do not rehash large
// recursive union members just to remove nil from an already-normalized union.
func UnionWithoutNil(u *Union) Type {
	return ProjectUnionMembers(u, func(member Type) Type {
		if member == nil || member.Kind() == kind.Nil {
			return Never
		}
		if opt, ok := member.(*Optional); ok {
			if opt.Inner == nil || opt.Inner.Kind() == kind.Never || opt.Inner.Kind() == kind.Nil {
				return Never
			}
			return opt.Inner
		}
		return member
	})
}

// Contains checks if the union contains a specific type.
func (u *Union) Contains(t Type) bool {
	h := unionMemberHash(t)
	for _, m := range u.Members {
		if unionMemberHash(m) == h && unionMemberEquals(m, t) {
			return true
		}
	}

	return false
}
