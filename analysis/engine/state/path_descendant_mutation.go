package state

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// PathDescendantMutation is the ProductDomain-sealed destructive descendant
// plan used by index mutation. The unique path-evidence owner computes alias
// expansion once; every registered lane and coordinate family consumes the
// same immutable prefix partition.
type PathDescendantMutation struct {
	seal     *productDomainSeal
	keys     *keyspace.KeySpace
	path     pathdom.PathKey
	prefixes pathevidence.PathKeyDescendantInvalidationPrefixes
}

// PathDescendantMutationFactorTopology is the unique factor-native
// representation of N4. A semantic participant is present exactly once:
// registered coordinate families as family factors, and only lanes without a
// coordinate implementation as opaque lane factors. The path-evidence owner
// is a coordinate family even though its concrete State lane also declares
// the same law.
type PathDescendantMutationFactorTopology struct {
	seal     *productDomainSeal
	lanes    []ProductLane
	families []CoordinateFamily
}

// SealPathDescendantMutationFactorTopology derives the representation solely
// from ProductDomain registration. Adding a participating coordinate family
// therefore changes every factor consumer without a hand-maintained axis
// list.
func (d ProductDomain) SealPathDescendantMutationFactorTopology() (PathDescendantMutationFactorTopology, error) {
	if !d.Valid() {
		return PathDescendantMutationFactorTopology{}, fmt.Errorf("%w: invalid path-descendant topology domain", ErrInvalidProductLane)
	}
	owner, hasOwner := d.PathEvidenceCoordinateFamily()
	out := PathDescendantMutationFactorTopology{seal: d.seal}
	for laneIndex := range d.factorLanes {
		runtime := &d.factorLanes[laneIndex]
		factored := false
		for familyIndex := range runtime.coordinates {
			family := &runtime.coordinates[familyIndex]
			participates := family.ops.pathMutation.participates || hasOwner && family.family == owner
			if !participates {
				continue
			}
			out.families = append(out.families, family.family)
			factored = true
		}
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathDescendantMutation)
		if declared && law.participates && !factored {
			out.lanes = append(out.lanes, runtime.lane)
		}
	}
	if !hasOwner {
		return PathDescendantMutationFactorTopology{}, fmt.Errorf("%w: path-descendant topology has no path-evidence owner", ErrInvalidLaneFactor)
	}
	return out, nil
}

func (t PathDescendantMutationFactorTopology) ValidFor(d ProductDomain) bool {
	return d.Valid() && t.seal != nil && t.seal == d.seal
}

func (t PathDescendantMutationFactorTopology) Lanes() []ProductLane {
	return append([]ProductLane(nil), t.lanes...)
}

func (t PathDescendantMutationFactorTopology) Families() []CoordinateFamily {
	return append([]CoordinateFamily(nil), t.families...)
}

// CoordinateFamilyFactor is one family-exclusive factor spelling. It cannot
// also be published as a whole LaneFactor in the same mutation topology.
type CoordinateFamilyFactor struct {
	skeleton CoordinateFamilySkeleton
	scalars  []CoordinateScalarFactor
}

func (d ProductDomain) SealCoordinateFamilyFactor(skeleton CoordinateFamilySkeleton, scalars []CoordinateScalarFactor) (CoordinateFamilyFactor, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil {
		return CoordinateFamilyFactor{}, err
	}
	if _, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars); err != nil {
		return CoordinateFamilyFactor{}, err
	}
	return CoordinateFamilyFactor{skeleton: skeleton, scalars: append([]CoordinateScalarFactor(nil), scalars...)}, nil
}

func (f CoordinateFamilyFactor) Family() CoordinateFamily           { return f.skeleton.family }
func (f CoordinateFamilyFactor) Skeleton() CoordinateFamilySkeleton { return f.skeleton }
func (f CoordinateFamilyFactor) Scalars() []CoordinateScalarFactor {
	return append([]CoordinateScalarFactor(nil), f.scalars...)
}

func (d ProductDomain) CoordinateFamilyFactorIsBottom(f CoordinateFamilyFactor) (bool, error) {
	if _, err := d.SealCoordinateFamilyFactor(f.skeleton, f.scalars); err != nil {
		return false, err
	}
	bottom, err := d.LaneBottom(f.Family().Lane())
	if err != nil {
		return false, err
	}
	skeleton, scalars, err := d.DecomposeCoordinateFamily(bottom, f.Family(), f.skeleton.keys)
	if err != nil {
		return false, err
	}
	equal, err := d.CoordinateSkeletonEqual(f.skeleton, skeleton)
	return equal && coordinateScalarFactorsEqual(d, f.scalars, scalars), err
}

// PathDescendantMutationFactors is the sealed factor-native companion tuple
// for the path-evidence owner. Its lane and coordinate components cannot be
// reordered or supplied independently after sealing.
type PathDescendantMutationFactors struct {
	seal        *productDomainSeal
	lanes       []LaneFactor
	coordinates []CoordinateFamilyFactor
}

// SealPathDescendantMutationFactors binds the exact topology-derived
// companion inventory. The unique path-evidence family is carried by the
// CoordinatePathEvidenceCarrier itself and is therefore excluded here.
func (d ProductDomain) SealPathDescendantMutationFactors(lanes []LaneFactor, coordinates []CoordinateFamilyFactor) (PathDescendantMutationFactors, error) {
	topology, err := d.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return PathDescendantMutationFactors{}, err
	}
	requiredLanes := topology.Lanes()
	requiredFamilies := topology.Families()
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok || len(requiredFamilies) == 0 || requiredFamilies[0] != owner {
		return PathDescendantMutationFactors{}, fmt.Errorf("%w: invalid descendant mutation owner", ErrIncompleteLaneFactors)
	}
	requiredFamilies = requiredFamilies[1:]
	if len(lanes) != len(requiredLanes) || len(coordinates) != len(requiredFamilies) {
		return PathDescendantMutationFactors{}, fmt.Errorf("%w: incomplete descendant mutation tuple", ErrIncompleteLaneFactors)
	}
	out := PathDescendantMutationFactors{seal: d.seal, lanes: make([]LaneFactor, len(lanes)), coordinates: make([]CoordinateFamilyFactor, len(coordinates))}
	for index, lane := range requiredLanes {
		factor := lanes[index]
		runtime, factorErr := d.validateFactor(factor)
		if factorErr != nil || factor.lane != lane || runtime.lane != lane {
			return PathDescendantMutationFactors{}, fmt.Errorf("%w: descendant mutation lane %d", ErrInvalidLaneFactor, index)
		}
		out.lanes[index] = factor
	}
	for index, family := range requiredFamilies {
		factor := coordinates[index]
		if factor.Family() != family {
			return PathDescendantMutationFactors{}, fmt.Errorf("%w: descendant mutation coordinate %d", ErrInvalidLaneFactor, index)
		}
		sealed, factorErr := d.SealCoordinateFamilyFactor(factor.skeleton, factor.scalars)
		if factorErr != nil {
			return PathDescendantMutationFactors{}, factorErr
		}
		out.coordinates[index] = sealed
	}
	return out, nil
}

func (f PathDescendantMutationFactors) validFor(d ProductDomain) bool {
	return d.Valid() && f.seal != nil && f.seal == d.seal
}

func (f PathDescendantMutationFactors) clone() PathDescendantMutationFactors {
	return PathDescendantMutationFactors{seal: f.seal, lanes: append([]LaneFactor(nil), f.lanes...), coordinates: cloneCoordinateFamilyFactors(f.coordinates)}
}

// LaneFactors returns the topology-ordered opaque mutation participants.
func (f PathDescendantMutationFactors) LaneFactors() []LaneFactor {
	return append([]LaneFactor(nil), f.lanes...)
}

// CoordinateFactors returns the topology-ordered coordinate mutation
// participants, excluding the path-evidence owner carried separately.
func (f PathDescendantMutationFactors) CoordinateFactors() []CoordinateFamilyFactor {
	return cloneCoordinateFamilyFactors(f.coordinates)
}

// DecomposePathDescendantMutationFactors transposes one concrete product into
// the exclusive factor topology. It is an edge adapter only; factor-native
// execution consumes the sealed result and never reconstructs State.
func (d ProductDomain) DecomposePathDescendantMutationFactors(input State, keys *keyspace.KeySpace) (PathDescendantMutationFactors, error) {
	return d.BindPathDescendantMutationFactors(keys, func(lane ProductLane) (LaneFactor, bool) {
		factors, err := d.DecomposeLanes(input, []ProductLane{lane})
		return firstLaneFactor(factors, err)
	})
}

func firstLaneFactor(factors []LaneFactor, err error) (LaneFactor, bool) {
	returnFactor := LaneFactor{}
	if err == nil && len(factors) == 1 {
		returnFactor = factors[0]
		return returnFactor, true
	}
	return LaneFactor{}, false
}

// BindPathDescendantMutationFactors seals the exclusive tuple from any
// caller-owned lane inventory. lookup is preparation-only and is not retained.
func (d ProductDomain) BindPathDescendantMutationFactors(keys *keyspace.KeySpace, lookup func(ProductLane) (LaneFactor, bool)) (PathDescendantMutationFactors, error) {
	if keys == nil || !keys.Valid() || lookup == nil {
		return PathDescendantMutationFactors{}, fmt.Errorf("%w: invalid descendant mutation binding", ErrInvalidLaneFactor)
	}
	topology, err := d.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return PathDescendantMutationFactors{}, err
	}
	lanes := topology.Lanes()
	laneFactors := make([]LaneFactor, len(lanes))
	for index, lane := range lanes {
		factor, present := lookup(lane)
		if !present {
			return PathDescendantMutationFactors{}, fmt.Errorf("%w: descendant lane %s", ErrIncompleteLaneFactors, lane.ID())
		}
		laneFactors[index] = factor
	}
	families := topology.Families()
	if len(families) == 0 {
		return PathDescendantMutationFactors{}, fmt.Errorf("%w: descendant mutation owner missing", ErrIncompleteLaneFactors)
	}
	coordinates := make([]CoordinateFamilyFactor, 0, len(families)-1)
	for _, family := range families[1:] {
		factor, present := lookup(family.Lane())
		if !present {
			return PathDescendantMutationFactors{}, fmt.Errorf("%w: descendant coordinate lane", ErrIncompleteLaneFactors)
		}
		skeleton, scalars, factorErr := d.DecomposeCoordinateFamily(factor, family, keys)
		if factorErr != nil {
			return PathDescendantMutationFactors{}, factorErr
		}
		coordinateFactor, factorErr := d.SealCoordinateFamilyFactor(skeleton, scalars)
		if factorErr != nil {
			return PathDescendantMutationFactors{}, factorErr
		}
		coordinates = append(coordinates, coordinateFactor)
	}
	return d.SealPathDescendantMutationFactors(laneFactors, coordinates)
}

// PrepareCoordinatePathDescendantMutation derives the exact descendant and
// subtree partition without composing State or inspecting another axis.
func (d ProductDomain) PrepareCoordinatePathDescendantMutation(
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	path pathdom.PathKey,
) (PathDescendantMutation, error) {
	mutation, present, err := d.PrepareCoordinatePathDescendantMutationIfPresent(skeleton, scalars, path)
	if err != nil {
		return PathDescendantMutation{}, err
	}
	if !present {
		return PathDescendantMutation{}, fmt.Errorf("state: unresolved coordinate path-descendant mutation")
	}
	return mutation, nil
}

// PrepareCoordinatePathDescendantMutationIfPresent separates a legitimate
// empty affected cone from malformed authority. Concrete invalidation treats
// an empty cone as identity; factor callers can now do the same without
// matching error text or inventing a sentinel transaction.
func (d ProductDomain) PrepareCoordinatePathDescendantMutationIfPresent(
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	path pathdom.PathKey,
) (PathDescendantMutation, bool, error) {
	owner, ok := d.PathValueFamily()
	if !ok || skeleton.family != owner || skeleton.keys == nil || !skeleton.keys.Valid() || path == "" {
		return PathDescendantMutation{}, false, fmt.Errorf("state: invalid coordinate path-descendant mutation")
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || coordinate.ops.pathEvidence.open == nil {
		return PathDescendantMutation{}, false, fmt.Errorf("state: path coordinate carrier cannot derive descendant mutation")
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return PathDescendantMutation{}, false, err
	}
	carrier, opened := coordinate.ops.pathEvidence.open(skeleton.payload, entries, skeleton.keys)
	if !opened || carrier == nil {
		return PathDescendantMutation{}, false, fmt.Errorf("state: path coordinate carrier open failed")
	}
	prefixes, ok := carrier.DescendantPrefixes(path)
	if !ok || len(prefixes.Descendants) == 0 && len(prefixes.Subtrees) == 0 {
		return PathDescendantMutation{}, false, nil
	}
	return PathDescendantMutation{
		seal: d.seal, keys: skeleton.keys, path: path,
		prefixes: pathevidence.PathKeyDescendantInvalidationPrefixes{
			Descendants: append([]pathdom.PathKey(nil), prefixes.Descendants...),
			Subtrees:    append([]pathdom.PathKey(nil), prefixes.Subtrees...),
		},
	}, true, nil
}

func (d ProductDomain) ownsPathDescendantMutation(transaction PathDescendantMutation) bool {
	return d.Valid() && transaction.seal == d.seal && transaction.keys != nil && transaction.keys.Valid() &&
		transaction.path != "" && (len(transaction.prefixes.Descendants) != 0 || len(transaction.prefixes.Subtrees) != 0)
}

// PathDescendantMutationParticipantLanes is the exact registered whole-lane
// write closure. Coordinate-factored lanes appear once regardless of how many
// participating families they own.
func (d ProductDomain) PathDescendantMutationParticipantLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathDescendantMutation)
		participates := declared && law.participates
		for familyIndex := range runtime.coordinates {
			family := &runtime.coordinates[familyIndex]
			participates = participates || family.ops.pathMutation.participates
		}
		if participates {
			out = append(out, runtime.lane)
		}
	}
	return out
}

// ApplyPathDescendantMutationLane applies the complete registered law for one
// whole lane. A coordinate-factored lane is patched family-by-family inside
// ProductDomain, so callers never reconstruct that composition themselves.
func (d ProductDomain) ApplyPathDescendantMutationLane(transaction PathDescendantMutation, current LaneFactor) (LaneFactor, error) {
	if !d.ownsPathDescendantMutation(transaction) {
		return LaneFactor{}, fmt.Errorf("state: foreign path-descendant mutation")
	}
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	hasCoordinate := false
	next := current
	for familyIndex := range runtime.coordinates {
		family := &runtime.coordinates[familyIndex]
		if !family.ops.pathMutation.participates {
			continue
		}
		hasCoordinate = true
		skeleton, scalars, decomposeErr := d.DecomposeCoordinateFamily(next, family.family, transaction.keys)
		if decomposeErr != nil {
			return LaneFactor{}, decomposeErr
		}
		skeleton, scalars, err = d.ApplyCoordinatePathDescendantMutation(transaction, skeleton, scalars)
		if err != nil {
			return LaneFactor{}, err
		}
		next, err = d.ReplaceCoordinateFamily(next, skeleton, scalars)
		if err != nil {
			return LaneFactor{}, err
		}
	}
	if hasCoordinate {
		return next, nil
	}
	return d.ApplyPathDescendantMutationFactor(transaction, current)
}

// ApplyPathDescendantMutationFactor applies one registered opaque-lane law.
// Unchanged factors retain their exact representation identity.
func (d ProductDomain) ApplyPathDescendantMutationFactor(transaction PathDescendantMutation, current LaneFactor) (LaneFactor, error) {
	if !d.ownsPathDescendantMutation(transaction) {
		return LaneFactor{}, fmt.Errorf("state: foreign path-descendant mutation")
	}
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathDescendantMutation)
	if !declared || !law.participates || law.applyFactor == nil {
		return LaneFactor{}, fmt.Errorf("%w: lane %q does not own path-descendant mutation", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed, valid := law.applyFactor(current.payload, pathDescendantMutationRequest{
		keys: transaction.keys, prefixes: transaction.prefixes, path: transaction.path,
	})
	if !valid {
		return LaneFactor{}, fmt.Errorf("state: lane %q rejected path-descendant mutation", runtime.lane.id)
	}
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

// ApplyCoordinatePathDescendantMutation applies the same sealed partition to
// either the unique path-evidence owner or another registered coordinate
// participant.
func (d ProductDomain) ApplyCoordinatePathDescendantMutation(
	transaction PathDescendantMutation,
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	if !d.ownsPathDescendantMutation(transaction) || skeleton.keys != transaction.keys {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: foreign coordinate path-descendant mutation")
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err == nil && coordinate.ops.pathMutation.participates {
		return d.applyCoordinatePathDescendantMutation(transaction, skeleton, scalars)
	}
	owner, ok := d.PathValueFamily()
	if !ok || skeleton.family != owner || err != nil || coordinate.ops.pathEvidence.open == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: coordinate family does not own path-descendant mutation")
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, err
	}
	carrier, opened := coordinate.ops.pathEvidence.open(skeleton.payload, entries, skeleton.keys)
	if !opened || carrier == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: path coordinate carrier open failed")
	}
	if _, valid := carrier.InvalidatePrefixes(transaction.prefixes); !valid {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: path coordinate carrier rejected descendant mutation")
	}
	nextSkeleton, nextEntries, frozen := carrier.Freeze()
	if !frozen {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: path coordinate carrier freeze failed")
	}
	nextScalars := make([]CoordinateScalarFactor, len(nextEntries))
	for index, entry := range nextEntries {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, skeleton.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) || index != 0 && !coordinate.ops.keyLess(nextEntries[index-1].key, entry.key, skeleton.keys) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: invalid path coordinate descendant result")
		}
		nextScalars[index] = CoordinateScalarFactor{slot: CoordinateSlot{family: owner, keys: skeleton.keys, key: entry.key}, payload: entry.scalar}
	}
	return CoordinateFamilySkeleton{family: owner, keys: skeleton.keys, payload: nextSkeleton}, nextScalars, nil
}
