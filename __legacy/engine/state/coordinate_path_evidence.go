package state

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type coordinatePathEvidenceKind uint8

const (
	coordinatePathEvidenceInvalid coordinatePathEvidenceKind = iota
	coordinatePathEvidenceNone
	coordinatePathEvidenceUnique
)

// coordinatePathEvidenceOps is the single storage registration role for all
// coordinate path-evidence operations. Every family declares either no
// participation or unique ownership; ProductDomain rejects missing and
// duplicate owners before execution.
type coordinatePathEvidenceOps struct {
	kind  coordinatePathEvidenceKind
	apply func(coordinateSkeletonPayload, []coordinateEntry, *keyspace.KeySpace, []pathevidence.PathPresenceImplication) (coordinateSkeletonPayload, []coordinateEntry, bool)
	open  func(coordinateSkeletonPayload, []coordinateEntry, *keyspace.KeySpace) (coordinatePathEvidenceCarrier, bool)
}

// coordinatePathEvidenceCarrier is the registered storage protocol shared by
// presence consequences, branch/equality proof maintenance, and path subtree
// mutation. It contains no scheduling or decision-diagram operation.
type coordinatePathEvidenceCarrier interface {
	Clone() coordinatePathEvidenceCarrier
	MakeUnreachable()
	SnapshotImplications() (pathevidence.PathPresenceImplicationsSnapshot, bool)
	HasImplication(pathevidence.PathPresenceImplication) bool
	AddImplication(pathevidence.PathPresenceImplication) (bool, bool)
	ReadPath(keyspace.Key) (product.Value, bool)
	WritePath(keyspace.Key, product.Value) (bool, bool)
	ReadStaticMember(keyspace.Key) (product.Value, bool)
	WriteStaticMember(keyspace.Key, product.Value) (bool, bool)
	HasProof(pathevidence.BranchProof) bool
	AddProof(pathevidence.BranchProof) (bool, bool)
	CloseProofsAcrossKnownEqualities(*keyspace.KeySpace) (bool, bool)
	CloseProofsAcrossTransientEquality(keyspace.Key, keyspace.Key) (bool, bool)
	CloseRefinementsAcrossTransientEquality(*axis.Registry, keyspace.Key, keyspace.Key, bool, func(keyspace.Key) bool) (bool, bool)
	EquivalentKeys(keyspace.Key) ([]keyspace.Key, bool)
	HasEquivalentKey(keyspace.Key, keyspace.Key) (bool, bool)
	EqualityQuotient() (pathevidence.EqualityQuotient, bool)
	DescendantPrefixes(pathdom.PathKey) (pathevidence.PathKeyDescendantInvalidationPrefixes, bool)
	InvalidatePrefixes(pathevidence.PathKeyDescendantInvalidationPrefixes) (bool, bool)
	SubtreePrefixes(pathdom.PathKey) ([]pathdom.PathKey, bool)
	InvalidateSubtreePrefixes([]pathdom.PathKey) (bool, bool)
	ApplyStableRootMutation(StableRootPathEvidenceMutation) (bool, bool)
	Freeze() (coordinateSkeletonPayload, []coordinateEntry, bool)
}

// CoordinatePathEvidenceCarrier is an opaque, family-owned coordinate storage
// adapter. It exposes the shared path-evidence algebra without materializing a
// LaneFactor or a partial State.
type CoordinatePathEvidenceCarrier[K comparable] struct {
	domain           ProductDomain
	family           CoordinateFamily
	keys             *keyspace.KeySpace
	values           ValueFactor[K]
	authority        CoordinatePathEvidenceAuthority[K]
	baselineSkeleton coordinateSkeletonPayload
	baselineEntries  []coordinateEntry
	reachable        bool
	mutation         PathDescendantMutationFactors
	inner            coordinatePathEvidenceCarrier
}

type coordinatePathEvidenceAuthoritySeal struct{ _ byte }

// CoordinatePathEvidenceAuthority is the immutable, presealed access surface
// of one path-evidence operation. It is constructed at plan freeze, shared by
// every transactional carrier clone, and compared by seal identity at Apply.
// Coordinate inventories may contain any registered families; the unique path
// family consumes its own bucket without rediscovering topology at runtime.
type CoordinatePathEvidenceAuthority[K comparable] struct {
	seal             *coordinatePathEvidenceAuthoritySeal
	domain           *productDomainSeal
	keys             *keyspace.KeySpace
	valueReads       map[K]struct{}
	valueWrites      map[K]struct{}
	coordinateReads  CoordinateFactorInventory
	coordinateWrites CoordinateFactorInventory
	pathMutation     bool
	writeSkeleton    bool
}

// SealCoordinatePathEvidenceAuthority validates a complete access surface
// once. The supplied coordinate inventories are already immutable admission
// products; runtime Open and Clone retain them by identity without copying.
func SealCoordinatePathEvidenceAuthority[K comparable](
	d ProductDomain,
	keys *keyspace.KeySpace,
	valueReads, valueWrites []K,
	coordinateReads, coordinateWrites CoordinateFactorInventory,
	pathMutation, writeSkeleton bool,
	valid func(K) bool,
) (CoordinatePathEvidenceAuthority[K], error) {
	if !d.Valid() {
		return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf("%w: path-evidence authority has no product domain", ErrInvalidLaneFactor)
	}
	if keys == nil || !keys.Valid() {
		return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf("%w: path-evidence authority has no keyspace", ErrInvalidLaneFactor)
	}
	if valid == nil {
		return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf("%w: path-evidence authority has no root validator", ErrInvalidLaneFactor)
	}
	if !coordinateReads.ValidFor(d, keys) {
		return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf(
			"%w: path-evidence read inventory is foreign (sealed=%t domain=%t keys=%t set=%t)",
			ErrInvalidLaneFactor, coordinateReads.seal != nil, coordinateReads.seal == d.seal,
			coordinateReads.keys == keys, coordinateReads.set != nil,
		)
	}
	if !coordinateWrites.ValidFor(d, keys) {
		return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf(
			"%w: path-evidence write inventory is foreign (sealed=%t domain=%t keys=%t set=%t)",
			ErrInvalidLaneFactor, coordinateWrites.seal != nil, coordinateWrites.seal == d.seal,
			coordinateWrites.keys == keys, coordinateWrites.set != nil,
		)
	}
	reads := make(map[K]struct{}, len(valueReads)+len(valueWrites))
	writes := make(map[K]struct{}, len(valueWrites))
	for _, slot := range valueReads {
		if !valid(slot) {
			return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf("%w: invalid path-evidence read slot", ErrInvalidLaneFactor)
		}
		reads[slot] = struct{}{}
	}
	for _, slot := range valueWrites {
		if !valid(slot) {
			return CoordinatePathEvidenceAuthority[K]{}, fmt.Errorf("%w: invalid path-evidence write slot", ErrInvalidLaneFactor)
		}
		reads[slot] = struct{}{}
		writes[slot] = struct{}{}
	}
	return CoordinatePathEvidenceAuthority[K]{
		seal: new(coordinatePathEvidenceAuthoritySeal), domain: d.seal, keys: keys,
		valueReads: reads, valueWrites: writes, coordinateReads: coordinateReads, coordinateWrites: coordinateWrites,
		pathMutation: pathMutation, writeSkeleton: writeSkeleton,
	}, nil
}

func (a CoordinatePathEvidenceAuthority[K]) validFor(d ProductDomain, keys *keyspace.KeySpace) bool {
	return a.seal != nil && a.domain == d.seal && a.keys == keys &&
		a.coordinateReads.ValidFor(d, keys) && a.coordinateWrites.ValidFor(d, keys)
}

func (c *CoordinatePathEvidenceCarrier[K]) Valid() bool {
	return c != nil && c.domain.Valid() && c.inner != nil
}

func cloneCoordinateFamilyFactors(in []CoordinateFamilyFactor) []CoordinateFamilyFactor {
	out := make([]CoordinateFamilyFactor, len(in))
	for index, factor := range in {
		out[index] = CoordinateFamilyFactor{skeleton: factor.skeleton, scalars: append([]CoordinateScalarFactor(nil), factor.scalars...)}
	}
	return out
}

func coordinateScalarFactorsEqual(domain ProductDomain, left, right []CoordinateScalarFactor) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		equal, err := domain.CoordinateScalarEqual(left[index], right[index])
		if err != nil || !equal {
			return false
		}
	}
	return true
}

// Clone returns an isolated transactional carrier. Family lanes are
// persistent values, so their package-owned Clone decides the minimum copy;
// mutable adapter inventories are always detached here.
func (c *CoordinatePathEvidenceCarrier[K]) Clone() *CoordinatePathEvidenceCarrier[K] {
	if !c.Valid() {
		return nil
	}
	values := ValueFactor[K]{Top: c.values.Top, Values: make(map[K]product.Value, len(c.values.Values))}
	for slot, value := range c.values.Values {
		values.Values[slot] = value
	}
	return &CoordinatePathEvidenceCarrier[K]{
		domain: c.domain, family: c.family, keys: c.keys, values: values,
		authority: c.authority, reachable: c.reachable,
		baselineSkeleton: c.baselineSkeleton, baselineEntries: append([]coordinateEntry(nil), c.baselineEntries...),
		mutation: c.mutation.clone(), inner: c.inner.Clone(),
	}
}

// Commit atomically replaces this carrier with a fully validated staged
// result. A failed semantic law therefore cannot leak a partial cross-axis
// mutation.
func (c *CoordinatePathEvidenceCarrier[K]) Commit(staged *CoordinatePathEvidenceCarrier[K]) bool {
	if !c.Valid() || !staged.Valid() || c.domain.reg != staged.domain.reg || c.family != staged.family || c.keys != staged.keys {
		return false
	}
	*c = *staged
	return true
}

func (c *CoordinatePathEvidenceCarrier[K]) MakeUnreachable() bool {
	if !c.Valid() {
		return false
	}
	bottom := c.domain.lattice.Bottom()
	stagedFactors := make([]LaneFactor, len(c.mutation.lanes))
	for index, factor := range c.mutation.lanes {
		runtime, err := c.domain.validateFactor(factor)
		if err != nil {
			return false
		}
		stagedFactors[index] = LaneFactor{lane: runtime.lane, payload: runtime.ops.extract(bottom)}
	}
	stagedCoordinates := make([]CoordinateFamilyFactor, len(c.mutation.coordinates))
	for index, factor := range c.mutation.coordinates {
		laneFactors, err := c.domain.DecomposeLanes(bottom, []ProductLane{factor.Family().Lane()})
		if err != nil || len(laneFactors) != 1 {
			return false
		}
		skeleton, scalars, err := c.domain.DecomposeCoordinateFamily(laneFactors[0], factor.Family(), c.keys)
		if err != nil {
			return false
		}
		stagedCoordinates[index], err = c.domain.SealCoordinateFamilyFactor(skeleton, scalars)
		if err != nil {
			return false
		}
	}
	c.inner.MakeUnreachable()
	c.values = ValueFactor[K]{Values: make(map[K]product.Value)}
	c.mutation = PathDescendantMutationFactors{seal: c.domain.seal, lanes: stagedFactors, coordinates: stagedCoordinates}
	c.reachable = false
	return true
}

func (c *CoordinatePathEvidenceCarrier[K]) SnapshotImplications() (pathevidence.PathPresenceImplicationsSnapshot, bool) {
	if !c.Valid() {
		return pathevidence.PathPresenceImplicationsSnapshot{}, false
	}
	return c.inner.SnapshotImplications()
}

func (c *CoordinatePathEvidenceCarrier[K]) HasImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if !c.Valid() {
		return false, false
	}
	slot, err := c.domain.PresenceImplicationCoordinateSlot(c.keys, value)
	if err != nil || !c.coordinateReadAllowed(slot) {
		return false, false
	}
	return c.inner.HasImplication(value), true
}

func (c *CoordinatePathEvidenceCarrier[K]) AddImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if !c.Valid() || !c.authority.writeSkeleton {
		return false, false
	}
	slot, err := c.domain.PresenceImplicationCoordinateSlot(c.keys, value)
	if err != nil || !c.coordinateWriteAllowed(slot) {
		return false, false
	}
	changed, ok := c.inner.AddImplication(value)
	if changed && ok {
		c.reachable = true
	}
	return changed, ok
}

func (c *CoordinatePathEvidenceCarrier[K]) ReadPath(path keyspace.Key) (product.Value, bool) {
	if !c.Valid() || !c.pathAccessAllowed(path, false) {
		return product.Value{}, false
	}
	return c.inner.ReadPath(path)
}

func (c *CoordinatePathEvidenceCarrier[K]) WritePath(path keyspace.Key, value product.Value) (bool, bool) {
	if !c.Valid() || !c.pathAccessAllowed(path, true) || !product.BelongsToRegistry(c.domain.reg, value) {
		return false, false
	}
	changed, ok := c.inner.WritePath(path, value)
	if changed && ok {
		c.reachable = true
	}
	return changed, ok
}

func (c *CoordinatePathEvidenceCarrier[K]) ReadStaticMember(path keyspace.Key) (product.Value, bool) {
	if !c.Valid() || !c.staticMemberAccessAllowed(path, false) {
		return product.Value{}, false
	}
	return c.inner.ReadStaticMember(path)
}

// WriteStaticMember publishes persistent structural member evidence through
// the unique path carrier. Static-member evidence is a distinct coordinate
// from a flow-sensitive refinement even when both name the same path.
func (c *CoordinatePathEvidenceCarrier[K]) WriteStaticMember(path keyspace.Key, value product.Value) (bool, bool) {
	if !c.Valid() || !c.staticMemberAccessAllowed(path, true) || !product.BelongsToRegistry(c.domain.reg, value) {
		return false, false
	}
	changed, ok := c.inner.WriteStaticMember(path, value)
	if changed && ok {
		c.reachable = true
	}
	return changed, ok
}

func (c *CoordinatePathEvidenceCarrier[K]) HasProof(proof pathevidence.BranchProof) bool {
	return c.Valid() && c.inner.HasProof(proof)
}

// AddProof publishes one registered path-evidence coordinate. The frozen
// coordinate-write inventory, not the proof kind, authorizes topology growth.
func (c *CoordinatePathEvidenceCarrier[K]) AddProof(proof pathevidence.BranchProof) (bool, bool) {
	if !c.Valid() || !c.authority.writeSkeleton {
		return false, false
	}
	slot, err := c.domain.PathBranchProofCoordinateSlot(c.keys, proof)
	if err != nil || !c.coordinateWriteAllowed(slot) {
		return false, false
	}
	changed, ok := c.inner.AddProof(proof)
	if changed && ok {
		c.reachable = true
	}
	return changed, ok
}

// CloseProofsAcrossKnownEqualities publishes the exact finite closure owned by
// the registered path-evidence family. The carrier's frozen coordinate-write
// set remains the authority: a closure that would escape it is rejected when
// the staged image is frozen.
func (c *CoordinatePathEvidenceCarrier[K]) CloseProofsAcrossKnownEqualities() (bool, bool) {
	if !c.Valid() || !c.authority.writeSkeleton {
		return false, false
	}
	staged := c.Clone()
	if staged == nil {
		return false, false
	}
	changed, ok := staged.inner.CloseProofsAcrossKnownEqualities(staged.keys)
	if !ok {
		return false, false
	}
	if _, _, _, _, _, err := staged.freezePathEvidence(); err != nil {
		return false, false
	}
	if !c.Commit(staged) {
		return false, false
	}
	return changed, true
}

func (c *CoordinatePathEvidenceCarrier[K]) CloseProofsAcrossTransientEquality(left, right keyspace.Key) (bool, bool) {
	if !c.Valid() || !c.authority.writeSkeleton {
		return false, false
	}
	changed, ok := c.inner.CloseProofsAcrossTransientEquality(left, right)
	if changed && ok {
		c.reachable = true
	}
	return changed, ok
}

// CloseRefinementsAcrossTransientEquality mirrors the finite registered
// refinement image authorized by this carrier. The family law owns the
// structural rebase; the carrier supplies the frozen write certificate.
func (c *CoordinatePathEvidenceCarrier[K]) CloseRefinementsAcrossTransientEquality(left, right keyspace.Key, memberSafe bool) (bool, bool) {
	if !c.Valid() {
		return false, false
	}
	return c.inner.CloseRefinementsAcrossTransientEquality(c.domain.reg, left, right, memberSafe, func(path keyspace.Key) bool {
		return c.pathAccessAllowed(path, true)
	})
}

func (c *CoordinatePathEvidenceCarrier[K]) EquivalentKeys(path keyspace.Key) ([]keyspace.Key, bool) {
	if !c.Valid() {
		return nil, false
	}
	return c.inner.EquivalentKeys(path)
}

func (c *CoordinatePathEvidenceCarrier[K]) HasEquivalentKey(left, right keyspace.Key) (bool, bool) {
	if !c.Valid() {
		return false, false
	}
	return c.inner.HasEquivalentKey(left, right)
}

func (c *CoordinatePathEvidenceCarrier[K]) EqualityQuotient() (pathevidence.EqualityQuotient, bool) {
	if !c.Valid() {
		return pathevidence.EqualityQuotient{}, false
	}
	return c.inner.EqualityQuotient()
}

func (c *CoordinatePathEvidenceCarrier[K]) InvalidateDescendants(path pathdom.PathKey) (bool, bool) {
	if !c.Valid() {
		return false, false
	}
	staged := c.Clone()
	if staged == nil {
		return false, false
	}
	prefixes, ok := staged.inner.DescendantPrefixes(path)
	if !ok {
		return false, false
	}
	changed, ok := staged.inner.InvalidatePrefixes(prefixes)
	if !ok {
		return false, false
	}
	transaction := PathDescendantMutation{
		seal: staged.domain.seal, keys: c.keys, path: path,
		prefixes: pathevidence.PathKeyDescendantInvalidationPrefixes{
			Descendants: append([]pathdom.PathKey(nil), prefixes.Descendants...),
			Subtrees:    append([]pathdom.PathKey(nil), prefixes.Subtrees...),
		},
	}
	for index := range staged.mutation.lanes {
		next, err := staged.domain.ApplyPathDescendantMutationLane(transaction, staged.mutation.lanes[index])
		if err != nil {
			return false, false
		}
		equal, err := staged.domain.LaneCanonicalRepresentationEqual(staged.mutation.lanes[index], next)
		if err != nil {
			return false, false
		}
		if !equal {
			staged.mutation.lanes[index] = next
			changed = true
		}
	}
	for index := range staged.mutation.coordinates {
		factor := staged.mutation.coordinates[index]
		skeleton, scalars, err := staged.domain.ApplyCoordinatePathDescendantMutation(transaction, factor.skeleton, factor.scalars)
		if err != nil {
			return false, false
		}
		next, err := staged.domain.SealCoordinateFamilyFactor(skeleton, scalars)
		if err != nil {
			return false, false
		}
		equalSkeleton, err := staged.domain.CoordinateSkeletonEqual(factor.skeleton, next.skeleton)
		if err != nil {
			return false, false
		}
		if !equalSkeleton || !coordinateScalarFactorsEqual(staged.domain, factor.scalars, next.scalars) {
			staged.mutation.coordinates[index] = next
			changed = true
		}
	}
	if _, _, _, _, _, _, err := staged.Freeze(); err != nil {
		return false, false
	}
	if !c.Commit(staged) {
		return false, false
	}
	return changed, true
}

func (c *CoordinatePathEvidenceCarrier[K]) ReadValue(slot K) (product.Value, bool) {
	if !c.Valid() {
		return product.Value{}, false
	}
	if _, declared := c.authority.valueReads[slot]; !declared {
		return product.Value{}, false
	}
	if c.values.Top {
		return product.Top(), true
	}
	if value, ok := c.values.Values[slot]; ok {
		return value, true
	}
	return product.Bottom(c.domain.reg), true
}

func (c *CoordinatePathEvidenceCarrier[K]) WriteValue(slot K, value product.Value) (bool, bool) {
	if !c.Valid() || !product.BelongsToRegistry(c.domain.reg, value) {
		return false, false
	}
	if _, declared := c.authority.valueWrites[slot]; !declared {
		return false, false
	}
	if c.values.Top {
		return false, true
	}
	current := product.Bottom(c.domain.reg)
	if prior, ok := c.values.Values[slot]; ok {
		current = prior
	}
	if product.Equal(c.domain.reg, current, value) {
		return false, true
	}
	if c.values.Values == nil {
		c.values.Values = make(map[K]product.Value)
	}
	if product.Equal(c.domain.reg, value, product.Bottom(c.domain.reg)) {
		delete(c.values.Values, slot)
	} else {
		c.values.Values[slot] = value
	}
	c.reachable = true
	return true, true
}

func (c *CoordinatePathEvidenceCarrier[K]) Reachable() bool { return c.Valid() && c.reachable }

// MatchesAuthority is the O(1) execution-time ownership proof. Authorities
// have no mutable surface, so exact seal identity proves the complete access
// contract without rebuilding or comparing coordinate/value inventories.
func (c *CoordinatePathEvidenceCarrier[K]) MatchesAuthority(authority CoordinatePathEvidenceAuthority[K]) bool {
	return c.Valid() && authority.validFor(c.domain, c.keys) && c.authority.seal == authority.seal
}

func (c *CoordinatePathEvidenceCarrier[K]) coordinateWriteAllowed(slot CoordinateSlot) bool {
	allowed, err := c.authority.coordinateWrites.Contains(c.domain, slot)
	return err == nil && allowed
}

func (c *CoordinatePathEvidenceCarrier[K]) coordinateReadAllowed(slot CoordinateSlot) bool {
	allowed, err := c.authority.coordinateReads.Contains(c.domain, slot)
	if err == nil && allowed {
		return true
	}
	return c.coordinateWriteAllowed(slot)
}

type coordinateWriteIndex map[uint64][]coordinateKeyPayload

func (c *CoordinatePathEvidenceCarrier[K]) coordinateWriteIndex() (coordinateWriteIndex, bool) {
	coordinate, err := c.domain.validateCoordinateFamily(c.family)
	if err != nil {
		return nil, false
	}
	slots, err := c.authority.coordinateWrites.familySlots(c.family)
	if err != nil {
		return nil, false
	}
	index := make(coordinateWriteIndex, len(slots))
	for _, slot := range slots {
		if slot.family != c.family || slot.keys != c.keys || slot.key == nil {
			return nil, false
		}
		hash := coordinate.ops.keyHash(slot.key, c.keys)
		index[hash] = append(index[hash], slot.key)
	}
	return index, true
}

func (c *CoordinatePathEvidenceCarrier[K]) coordinateWriteIndexContains(index coordinateWriteIndex, key coordinateKeyPayload) bool {
	coordinate, err := c.domain.validateCoordinateFamily(c.family)
	if err != nil || key == nil {
		return false
	}
	for _, candidate := range index[coordinate.ops.keyHash(key, c.keys)] {
		if coordinate.ops.keyEqual(candidate, key) {
			return true
		}
	}
	return false
}

func (c *CoordinatePathEvidenceCarrier[K]) pathAccessAllowed(path keyspace.Key, write bool) bool {
	slot, err := c.domain.PresenceImplicationRefinementCoordinateSlot(c.keys, path)
	if err != nil {
		return false
	}
	if write {
		return c.coordinateWriteAllowed(slot)
	}
	return c.coordinateReadAllowed(slot)
}

func (c *CoordinatePathEvidenceCarrier[K]) staticMemberAccessAllowed(path keyspace.Key, write bool) bool {
	slot, err := c.domain.PathStaticMemberCoordinateSlot(c.keys, path)
	if err != nil {
		return false
	}
	if write {
		return c.coordinateWriteAllowed(slot)
	}
	return c.coordinateReadAllowed(slot)
}

func (c *CoordinatePathEvidenceCarrier[K]) freezePathEvidence() (CoordinateFamilySkeleton, []CoordinateScalarFactor, ValueFactor[K], []LaneFactor, bool, error) {
	if !c.Valid() {
		return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, fmt.Errorf("%w: invalid presence carrier", ErrInvalidLaneFactor)
	}
	skeleton, entries, ok := c.inner.Freeze()
	if !ok {
		return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, fmt.Errorf("%w: invalid presence carrier result", ErrInvalidLaneFactor)
	}
	coordinate, err := c.domain.validateCoordinateFamily(c.family)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, err
	}
	if !c.authority.writeSkeleton && !coordinate.ops.skeletonEqual(c.baselineSkeleton, skeleton) {
		return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, fmt.Errorf("%w: undeclared presence skeleton write", ErrInvalidLaneFactor)
	}
	writeIndex, validWrites := c.coordinateWriteIndex()
	if !validWrites {
		return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, fmt.Errorf("%w: invalid presence coordinate writes", ErrInvalidLaneFactor)
	}
	// Both inventories are in the family-owned strict key order. A merge
	// validates changed coordinates once instead of repeatedly rescanning the
	// complete before/after vectors.
	for beforeIndex, afterIndex := 0, 0; beforeIndex < len(c.baselineEntries) || afterIndex < len(entries); {
		var key coordinateKeyPayload
		changed := false
		switch {
		case beforeIndex == len(c.baselineEntries):
			key, changed = entries[afterIndex].key, true
			afterIndex++
		case afterIndex == len(entries):
			key, changed = c.baselineEntries[beforeIndex].key, true
			beforeIndex++
		case coordinate.ops.keyEqual(c.baselineEntries[beforeIndex].key, entries[afterIndex].key):
			key = c.baselineEntries[beforeIndex].key
			changed = !coordinate.ops.scalarEqual(c.baselineEntries[beforeIndex].scalar, entries[afterIndex].scalar)
			beforeIndex++
			afterIndex++
		case coordinate.ops.keyLess(c.baselineEntries[beforeIndex].key, entries[afterIndex].key, c.keys):
			key, changed = c.baselineEntries[beforeIndex].key, true
			beforeIndex++
		default:
			key, changed = entries[afterIndex].key, true
			afterIndex++
		}
		if changed && !c.coordinateWriteIndexContains(writeIndex, key) {
			return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, fmt.Errorf("%w: undeclared presence coordinate write", ErrInvalidLaneFactor)
		}
	}
	out := make([]CoordinateScalarFactor, len(entries))
	for index, entry := range entries {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, c.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) ||
			index != 0 && !coordinate.ops.keyLess(entries[index-1].key, entry.key, c.keys) {
			return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, false, fmt.Errorf("%w: noncanonical presence carrier result", ErrInvalidLaneFactor)
		}
		out[index] = CoordinateScalarFactor{
			slot:    CoordinateSlot{family: c.family, keys: c.keys, key: entry.key},
			payload: entry.scalar,
		}
	}
	values := ValueFactor[K]{Top: c.values.Top, Values: make(map[K]product.Value, len(c.values.Values))}
	for slot, value := range c.values.Values {
		values.Values[slot] = value
	}
	return CoordinateFamilySkeleton{family: c.family, keys: c.keys, payload: skeleton}, out, values, append([]LaneFactor(nil), c.mutation.lanes...), c.reachable, nil
}

// Freeze publishes the one exclusive factor tuple: the path-evidence owner,
// opaque mutation lanes, and every other registered coordinate participant.
// No coordinate-backed lane is also returned as a whole LaneFactor.
func (c *CoordinatePathEvidenceCarrier[K]) Freeze() (CoordinateFamilySkeleton, []CoordinateScalarFactor, ValueFactor[K], []LaneFactor, []CoordinateFamilyFactor, bool, error) {
	skeleton, scalars, values, lanes, reachable, err := c.freezePathEvidence()
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, ValueFactor[K]{}, nil, nil, false, err
	}
	return skeleton, scalars, values, lanes, cloneCoordinateFamilyFactors(c.mutation.coordinates), reachable, nil
}

func noCoordinatePathEvidence() coordinatePathEvidenceOps {
	return coordinatePathEvidenceOps{kind: coordinatePathEvidenceNone}
}

func uniqueCoordinatePathEvidence(
	apply func(coordinateSkeletonPayload, []coordinateEntry, *keyspace.KeySpace, []pathevidence.PathPresenceImplication) (coordinateSkeletonPayload, []coordinateEntry, bool),
	open func(coordinateSkeletonPayload, []coordinateEntry, *keyspace.KeySpace) (coordinatePathEvidenceCarrier, bool),
) coordinatePathEvidenceOps {
	return coordinatePathEvidenceOps{kind: coordinatePathEvidenceUnique, apply: apply, open: open}
}

func coordinatePathEvidenceOpsComplete(ops coordinatePathEvidenceOps) bool {
	switch ops.kind {
	case coordinatePathEvidenceNone:
		return ops.apply == nil && ops.open == nil
	case coordinatePathEvidenceUnique:
		return ops.apply != nil && ops.open != nil
	default:
		return false
	}
}

// OpenCoordinatePathEvidenceCarrier validates and opens the unique registered
// path-evidence family as an opaque native carrier. Dynamic inventories select
// coordinates, but the family and its algebra were frozen at registration.
func (d ProductDomain) OpenCoordinatePathEvidenceCarrier(
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	values ValueLaneFactor,
	reachable bool,
	authority CoordinatePathEvidenceAuthority[statekey.Value],
	mutation PathDescendantMutationFactors,
) (*CoordinatePathEvidenceCarrier[statekey.Value], error) {
	return OpenCoordinatePathEvidenceCarrier(
		d, skeleton, scalars, values, reachable, authority, mutation,
	)
}

// OpenCoordinatePathEvidenceCarrier opens the single canonical path carrier
// over a caller-owned, sealed scalar-root vocabulary. valid rejects empty or
// foreign roots before execution; closure never discovers root bindings.
func OpenCoordinatePathEvidenceCarrier[K comparable](
	d ProductDomain,
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	values ValueFactor[K],
	reachable bool,
	authority CoordinatePathEvidenceAuthority[K],
	mutation PathDescendantMutationFactors,
) (*CoordinatePathEvidenceCarrier[K], error) {
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok || skeleton.family != owner || !authority.validFor(d, skeleton.keys) {
		return nil, fmt.Errorf("%w: coordinate family does not own presence implications", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || coordinate.ops.pathEvidence.kind != coordinatePathEvidenceUnique || coordinate.ops.pathEvidence.open == nil {
		return nil, fmt.Errorf("%w: invalid path-evidence carrier", ErrInvalidLaneFactor)
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return nil, err
	}
	inner, ok := coordinate.ops.pathEvidence.open(skeleton.payload, entries, skeleton.keys)
	if !ok || inner == nil {
		return nil, fmt.Errorf("%w: presence carrier open failed", ErrInvalidLaneFactor)
	}
	ownedValues := ValueFactor[K]{Top: values.Top, Values: make(map[K]product.Value, len(values.Values))}
	for slot, value := range values.Values {
		if _, declared := authority.valueReads[slot]; !declared || !product.BelongsToRegistry(d.reg, value) {
			return nil, fmt.Errorf("%w: undeclared presence carrier value", ErrInvalidLaneFactor)
		}
		ownedValues.Values[slot] = value
	}
	if authority.pathMutation {
		if !mutation.validFor(d) {
			return nil, fmt.Errorf("%w: incomplete descendant mutation tuple", ErrIncompleteLaneFactors)
		}
	} else if mutation.seal != nil {
		return nil, fmt.Errorf("%w: unexpected descendant mutation tuple", ErrInvalidLaneFactor)
	}
	return &CoordinatePathEvidenceCarrier[K]{
		domain: d, family: owner, keys: skeleton.keys, values: ownedValues, authority: authority,
		baselineSkeleton: skeleton.payload, baselineEntries: append([]coordinateEntry(nil), entries...),
		reachable: reachable, mutation: mutation, inner: inner,
	}, nil
}

// PathEvidenceCoordinateFamily returns the optional unique coordinate family
// that owns all persistent path-evidence storage operations.
func (d ProductDomain) PathEvidenceCoordinateFamily() (CoordinateFamily, bool) {
	if !d.Valid() || !d.hasPathEvidenceFamily {
		return CoordinateFamily{}, false
	}
	if _, err := d.validateCoordinateFamily(d.pathEvidenceFamily); err != nil {
		return CoordinateFamily{}, false
	}
	return d.pathEvidenceFamily, true
}

// PresenceImplicationShapes projects the fixed structural addresses of
// persistent implications from an opaque coordinate inventory. Product-valued
// clauses deliberately do not appear here: they are scalar terminal payload
// and are reconstructed only from a key+scalar snapshot.
func (d ProductDomain) PresenceImplicationShapes(
	keys *keyspace.KeySpace,
	slots []CoordinateSlot,
) ([]pathevidence.PathPresenceImplication, error) {
	if keys == nil || !keys.Valid() {
		return nil, fmt.Errorf("%w: invalid presence implication keyspace", ErrInvalidLaneFactor)
	}
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok {
		return nil, fmt.Errorf("%w: no presence implication family", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	if err != nil {
		return nil, err
	}
	rows := make([]pathevidence.PathPresenceImplication, 0)
	for _, slot := range slots {
		if slot.family != owner || slot.keys != keys || d.validateCoordinateSlotFor(coordinate, slot, keys) != nil {
			return nil, fmt.Errorf("%w: invalid presence coordinate inventory", ErrInvalidLaneFactor)
		}
		descriptor, valid := pathevidence.DescribeCoordinate(pathEvidenceCoordinateKey(slot.key))
		if !valid {
			return nil, fmt.Errorf("%w: invalid presence coordinate descriptor", ErrInvalidLaneFactor)
		}
		if descriptor.Kind == pathevidence.CoordinateDescriptorPresenceImplication {
			rows = append(rows, descriptor.Implication)
		}
	}
	return rows, nil
}

// PresenceImplicationCoordinateSlot seals the scalar coordinate created by a
// publication before guarded evaluation. This augments the frozen inventory;
// leaf execution never invents an output identity.
func (d ProductDomain) PresenceImplicationCoordinateSlot(keys *keyspace.KeySpace, implication pathevidence.PathPresenceImplication) (CoordinateSlot, error) {
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok || keys == nil || !keys.Valid() {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid presence coordinate publication", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	key := pathevidence.PresenceImplicationCoordinate(implication)
	payload := typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: key}
	if err != nil || !coordinate.ops.keyValid(payload, keys) {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid presence implication coordinate", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: owner, keys: keys, key: payload}, nil
}

// PresenceImplicationRefinementCoordinateSlot seals a local path target that
// the implication kernel may create at runtime.
func (d ProductDomain) PresenceImplicationRefinementCoordinateSlot(keys *keyspace.KeySpace, path keyspace.Key) (CoordinateSlot, error) {
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok || keys == nil || !keys.Valid() {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid presence refinement coordinate", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	key := pathevidence.RefinementCoordinate(path)
	payload := typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: key}
	if err != nil || !coordinate.ops.keyValid(payload, keys) {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid presence refinement coordinate", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: owner, keys: keys, key: payload}, nil
}

// ApplyCoordinatePresenceImplications publishes a canonical implication plan
// directly into one family quotient and scalar inventory. It never materializes
// a LaneFactor or whole Product State.
func (d ProductDomain) ApplyCoordinatePresenceImplications(
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	implications []pathevidence.PathPresenceImplication,
) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok || skeleton.family != owner {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: coordinate family does not own presence implications", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || coordinate.ops.pathEvidence.kind != coordinatePathEvidenceUnique {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: invalid presence-implication family", ErrInvalidLaneFactor)
	}

	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, err
	}

	payload, published, applied := coordinate.ops.pathEvidence.apply(skeleton.payload, entries, skeleton.keys, implications)
	if !applied || payload == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: presence-implication publication", ErrInvalidLaneFactor)
	}
	out := make([]CoordinateScalarFactor, len(published))
	for index, entry := range published {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, skeleton.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: invalid published presence-implication coordinate", ErrInvalidLaneFactor)
		}
		if index != 0 && !coordinate.ops.keyLess(published[index-1].key, entry.key, skeleton.keys) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: noncanonical published presence-implication coordinates", ErrInvalidLaneFactor)
		}
		out[index] = CoordinateScalarFactor{
			slot:    CoordinateSlot{family: coordinate.family, keys: skeleton.keys, key: entry.key},
			payload: entry.scalar,
		}
	}
	return CoordinateFamilySkeleton{family: coordinate.family, keys: skeleton.keys, payload: payload}, out, nil
}

// explicitCoordinateEntries quotients a guarded union-coordinate inventory to
// the explicit finite entries consumed by a registered family adapter. The
// omitted/default law is family-owned and therefore shared by publication and
// transactional carrier opening; neither consumer may reinterpret scalar
// representation.
func (d ProductDomain) explicitCoordinateEntries(
	coordinate *coordinateFamilyRuntime,
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
) ([]coordinateEntry, error) {
	entries := make([]coordinateEntry, 0, len(scalars))
	for index, scalar := range scalars {
		if err := d.validateCoordinateFactorFor(coordinate, scalar, skeleton.keys); err != nil {
			return nil, err
		}
		if index != 0 && !coordinate.ops.keyLess(scalars[index-1].slot.key, scalar.slot.key, skeleton.keys) {
			return nil, fmt.Errorf("%w: noncanonical coordinate inventory", ErrInvalidLaneFactor)
		}
		omitted, omitErr := d.CoordinateScalarIsOmitted(skeleton, scalar)
		if omitErr != nil {
			return nil, omitErr
		}
		if omitted {
			continue
		}
		entries = append(entries, coordinateEntry{key: scalar.slot.key, scalar: scalar.payload})
	}
	return entries, nil
}
