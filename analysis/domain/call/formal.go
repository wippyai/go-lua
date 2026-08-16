package call

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	symbolicMagic   = "callsym1"
	symbolicVersion = uint64(1)
	symbolicHeader  = 8 + 8 + 32 + 1 + 1 + 4
	symbolicRole    = 1 + 32
)

// symbolicLawID identifies this closed, portable Call symbolic schema. Link
// identity is intentionally absent: a Link is hot authority supplied to
// BindSymbolic, while this versioned law ID is the only schema discriminator
// carried by the artifact.
var symbolicLawID = keyspace.ContentID(sha256.Sum256([]byte("wippy.analysis.call.symbolic.v1")))

// SymbolicValue is Call's portable, keyless dispatch relation. It contains
// only owner-issued TargetRoleID values and opaque status; no Link, Algebra,
// runtime, selector ordinal, or dense target table is retained.
type SymbolicValue struct {
	top   bool
	open  bool
	roles []TargetRoleID
}

func (value SymbolicValue) Valid() bool {
	if value.top && (value.open || len(value.roles) != 0) {
		return false
	}
	for index, role := range value.roles {
		if !role.Valid() || index > 0 && compareTargetRole(value.roles[index-1], role) >= 0 {
			return false
		}
	}
	return true
}

func (value SymbolicValue) IsTop() bool { return value.Valid() && value.top }
func (value SymbolicValue) IsOpen() bool {
	return value.Valid() && !value.top && value.open
}
func (value SymbolicValue) HasOpaqueAlternative() bool {
	return value.Valid() && (value.top || value.open)
}
func (value SymbolicValue) SchemaID() keyspace.ContentID { return symbolicLawID }
func (value SymbolicValue) RoleCount() int {
	if !value.Valid() || value.top {
		return 0
	}
	return len(value.roles)
}
func (value SymbolicValue) RoleAt(index int) (TargetRoleID, bool) {
	if !value.Valid() || index < 0 || index >= len(value.roles) {
		return TargetRoleID{}, false
	}
	return value.roles[index], true
}

// Symbolic projects a hot Value into portable role identity. Every known
// target must be issued by this exact Algebra; the resulting relation is
// detached and can be encoded or replayed against an equivalent Algebra.
func (algebra *Algebra) Symbolic(value Value) (SymbolicValue, bool) {
	if algebra == nil || !algebra.Valid() || !algebra.owns(value) {
		return SymbolicValue{}, false
	}
	result := SymbolicValue{top: value.top, open: value.open}
	if !value.top {
		result.roles = make([]TargetRoleID, len(value.selectors))
		for index, selector := range value.selectors {
			target := Target{owner: algebra, selector: selector}
			role, ok := target.RoleID()
			if !ok {
				return SymbolicValue{}, false
			}
			result.roles[index] = role
		}
		sort.Slice(result.roles, func(left, right int) bool { return compareTargetRole(result.roles[left], result.roles[right]) < 0 })
	}
	return result, result.Valid()
}

// BindSymbolic is the hot bind boundary. It requires the exact source Key,
// resolves each portable role through this Algebra's authenticated role
// index, and only then constructs the owner-fenced Value.
func (algebra *Algebra) BindSymbolic(key Key, value SymbolicValue) (Value, bool) {
	if algebra == nil || !algebra.Valid() || !algebra.validKey(key) || !value.Valid() {
		return Value{}, false
	}
	if value.top {
		return algebra.Top(), true
	}
	targets := make([]Target, len(value.roles))
	for index, roleID := range value.roles {
		role, ok := algebra.TargetForRole(roleID)
		if !ok {
			return Value{}, false
		}
		targets[index], ok = role.Target()
		if !ok {
			return Value{}, false
		}
	}
	return algebra.DispatchValue(key, targets, value.open)
}

// CanonicalBytes is a fixed-width, versioned encoding. It is suitable for
// replay but carries no decoder authority; BindSymbolic remains responsible
// for exact Algebra and Key authentication.
func (value SymbolicValue) CanonicalBytes() ([]byte, bool) {
	if !value.Valid() || uint64(len(value.roles)) > uint64(^uint32(0)) {
		return nil, false
	}
	encoded := make([]byte, symbolicHeader+symbolicRole*len(value.roles))
	copy(encoded[:8], symbolicMagic)
	binary.BigEndian.PutUint64(encoded[8:16], symbolicVersion)
	copy(encoded[16:48], symbolicLawID[:])
	if value.top {
		encoded[48] = 1
	}
	if value.open {
		encoded[49] = 1
	}
	binary.BigEndian.PutUint32(encoded[50:54], uint32(len(value.roles)))
	for index, role := range value.roles {
		offset := symbolicHeader + symbolicRole*index
		encoded[offset] = byte(role.Kind())
		id, ok := role.ContentID()
		if !ok {
			return nil, false
		}
		copy(encoded[offset+1:offset+33], id[:])
	}
	return encoded, true
}

// DecodeSymbolic validates framing, ordering, uniqueness, and role kinds but
// does not resolve the value. The caller must use Algebra.BindSymbolic.
func DecodeSymbolic(encoded []byte) (SymbolicValue, bool) {
	if len(encoded) < symbolicHeader || string(encoded[:8]) != symbolicMagic || binary.BigEndian.Uint64(encoded[8:16]) != symbolicVersion || encoded[48] > 1 || encoded[49] > 1 {
		return SymbolicValue{}, false
	}
	count := binary.BigEndian.Uint32(encoded[50:54])
	if uint64(len(encoded)) != uint64(symbolicHeader)+uint64(count)*symbolicRole {
		return SymbolicValue{}, false
	}
	var schemaID keyspace.ContentID
	copy(schemaID[:], encoded[16:48])
	if schemaID != symbolicLawID {
		return SymbolicValue{}, false
	}
	value := SymbolicValue{top: encoded[48] != 0, open: encoded[49] != 0, roles: make([]TargetRoleID, count)}
	for index := range value.roles {
		offset := symbolicHeader + symbolicRole*index
		var id keyspace.ContentID
		copy(id[:], encoded[offset+1:offset+33])
		role, ok := newTargetRoleID(TargetRoleKind(encoded[offset]), id)
		if !ok {
			return SymbolicValue{}, false
		}
		value.roles[index] = role
	}
	return value, value.Valid()
}

// ReplayCanonical decodes and hot-binds one exact portable artifact.
func (algebra *Algebra) ReplayCanonical(key Key, encoded []byte) (Value, bool) {
	value, ok := DecodeSymbolic(encoded)
	if !ok {
		return Value{}, false
	}
	return algebra.BindSymbolic(key, value)
}

// RoleRenaming is a kind-preserving monotone set renaming over stable target
// roles. It carries no provenance, hot target authority, or dense coordinate.
type RoleRenaming struct {
	from TargetRoleID
	to   TargetRoleID
}

func NewRoleRenaming(from, to TargetRoleID) (RoleRenaming, bool) {
	if !from.Valid() || !to.Valid() || from.Kind() != to.Kind() {
		return RoleRenaming{}, false
	}
	return RoleRenaming{from: from, to: to}, true
}

func (renaming RoleRenaming) Valid() bool {
	return renaming.from.Valid() && renaming.to.Valid() && renaming.from.Kind() == renaming.to.Kind()
}

func (renaming RoleRenaming) From() TargetRoleID { return renaming.from }
func (renaming RoleRenaming) To() TargetRoleID   { return renaming.to }

// Rename applies finite, duplicate-free role renamings. Missing roles remain
// unchanged, and renaming preserves opaque/top status.
func (value SymbolicValue) Rename(renamings []RoleRenaming) (SymbolicValue, bool) {
	if !value.Valid() {
		return SymbolicValue{}, false
	}
	lookup := make(map[TargetRoleID]TargetRoleID, len(renamings))
	for _, renaming := range renamings {
		if !renaming.Valid() || renaming.from == renaming.to {
			return SymbolicValue{}, false
		}
		if _, duplicate := lookup[renaming.from]; duplicate {
			return SymbolicValue{}, false
		}
		lookup[renaming.from] = renaming.to
	}
	result := SymbolicValue{top: value.top, open: value.open, roles: append([]TargetRoleID(nil), value.roles...)}
	for index, role := range result.roles {
		if replacement, ok := lookup[role]; ok {
			result.roles[index] = replacement
		}
	}
	sort.Slice(result.roles, func(left, right int) bool { return compareTargetRole(result.roles[left], result.roles[right]) < 0 })
	for index := 1; index < len(result.roles); index++ {
		if result.roles[index-1] == result.roles[index] {
			result.roles = append(result.roles[:index-1], result.roles[index:]...)
			index--
		}
	}
	return result, result.Valid()
}

func (value SymbolicValue) Equal(other SymbolicValue) bool {
	return value.Valid() && other.Valid() && value.top == other.top && value.open == other.open && equalTargetRoles(value.roles, other.roles)
}

func (value SymbolicValue) LessOrEq(other SymbolicValue) bool {
	if !value.Valid() || !other.Valid() {
		return false
	}
	if other.top || !value.top && !value.open && len(value.roles) == 0 {
		return true
	}
	if value.top || value.open && !other.open {
		return false
	}
	return targetRolesSubset(value.roles, other.roles)
}

func (value SymbolicValue) Join(other SymbolicValue) (SymbolicValue, bool) {
	if !value.Valid() || !other.Valid() {
		return SymbolicValue{}, false
	}
	if value.top || other.top {
		return SymbolicValue{top: true}, true
	}
	result := SymbolicValue{open: value.open || other.open, roles: unionTargetRoles(value.roles, other.roles)}
	return result, result.Valid()
}

func compareTargetRole(left, right TargetRoleID) int {
	if left.Kind() < right.Kind() {
		return -1
	}
	if left.Kind() > right.Kind() {
		return 1
	}
	return bytes.Compare(left.id[:], right.id[:])
}

func equalTargetRoles(left, right []TargetRoleID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func targetRolesSubset(left, right []TargetRoleID) bool {
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left) && rightIndex < len(right); {
		comparison := compareTargetRole(left[leftIndex], right[rightIndex])
		if comparison == 0 {
			leftIndex++
			rightIndex++
		} else if comparison > 0 {
			rightIndex++
		} else {
			return false
		}
		if leftIndex == len(left) {
			return true
		}
	}
	return len(left) == 0
}

func unionTargetRoles(left, right []TargetRoleID) []TargetRoleID {
	result := make([]TargetRoleID, 0, len(left)+len(right))
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left) || rightIndex < len(right); {
		switch {
		case rightIndex == len(right) || leftIndex < len(left) && compareTargetRole(left[leftIndex], right[rightIndex]) < 0:
			result = append(result, left[leftIndex])
			leftIndex++
		case leftIndex == len(left) || compareTargetRole(right[rightIndex], left[leftIndex]) < 0:
			result = append(result, right[rightIndex])
			rightIndex++
		default:
			result = append(result, left[leftIndex])
			leftIndex++
			rightIndex++
		}
	}
	return result
}
