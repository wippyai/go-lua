package state

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// PathSubtreeMutation is the ProductDomain-sealed N4 destructive replacement
// plan. Prefix expansion is derived once by the registered path-evidence
// authority; every other participating axis consumes that same immutable
// prefix set through its registered factor law.
type PathSubtreeMutation struct {
	seal     *productDomainSeal
	keys     *keyspace.KeySpace
	path     pathdom.PathKey
	prefixes []pathdom.PathKey
}

// PrepareCoordinatePathSubtreeMutation derives the transaction directly
// from the registered path coordinate carrier, without composing a State or
// opaque LaneFactor.
func (d ProductDomain) PrepareCoordinatePathSubtreeMutation(
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	path pathdom.PathKey,
) (PathSubtreeMutation, error) {
	mutation, present, err := d.PrepareCoordinatePathSubtreeMutationIfPresent(skeleton, scalars, path)
	if err != nil {
		return PathSubtreeMutation{}, err
	}
	if !present {
		return PathSubtreeMutation{}, fmt.Errorf("state: unresolved coordinate path-subtree mutation")
	}
	return mutation, nil
}

// PrepareCoordinatePathSubtreeMutationIfPresent returns present=false for the
// exact empty visible-path sum arm. Malformed or foreign carriers still fail.
func (d ProductDomain) PrepareCoordinatePathSubtreeMutationIfPresent(
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
	path pathdom.PathKey,
) (PathSubtreeMutation, bool, error) {
	owner, ok := d.PathValueFamily()
	if !ok || skeleton.family != owner || skeleton.keys == nil || !skeleton.keys.Valid() || path == "" {
		return PathSubtreeMutation{}, false, fmt.Errorf("state: invalid coordinate path-subtree mutation")
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || coordinate.ops.pathEvidence.open == nil {
		return PathSubtreeMutation{}, false, fmt.Errorf("state: path coordinate carrier cannot derive subtree mutation")
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return PathSubtreeMutation{}, false, err
	}
	carrier, opened := coordinate.ops.pathEvidence.open(skeleton.payload, entries, skeleton.keys)
	if !opened || carrier == nil {
		return PathSubtreeMutation{}, false, fmt.Errorf("state: path coordinate carrier open failed")
	}
	prefixes, ok := carrier.SubtreePrefixes(path)
	if !ok || len(prefixes) == 0 {
		return PathSubtreeMutation{}, false, nil
	}
	return PathSubtreeMutation{
		seal: d.seal, keys: skeleton.keys, path: path,
		prefixes: append([]pathdom.PathKey(nil), prefixes...),
	}, true, nil
}

func (d ProductDomain) ownsPathSubtreeMutation(transaction PathSubtreeMutation) bool {
	return d.Valid() && transaction.seal == d.seal && transaction.keys != nil && transaction.keys.Valid() &&
		transaction.path != "" && len(transaction.prefixes) != 0
}

// applyPathSubtreeMutationFactor applies one registered opaque-lane law.
// Unchanged factors retain identity.
func (d ProductDomain) applyPathSubtreeMutationFactor(transaction PathSubtreeMutation, current LaneFactor) (LaneFactor, error) {
	if !d.ownsPathSubtreeMutation(transaction) {
		return LaneFactor{}, fmt.Errorf("state: foreign path-subtree mutation")
	}
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathSubtreeMutation)
	if !declared || !law.participates {
		return LaneFactor{}, fmt.Errorf("%w: lane %q does not own path-subtree mutation", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed, valid := law.applyFactor(current.payload, pathSubtreeMutationRequest{
		keys: transaction.keys, prefixes: transaction.prefixes, path: transaction.path,
	})
	if !valid {
		return LaneFactor{}, fmt.Errorf("state: lane %q rejected path-subtree mutation", runtime.lane.id)
	}
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

// applyCoordinatePathSubtreeMutationFactor applies one registered coordinate
// family law, including the path-value authority.
func (d ProductDomain) applyCoordinatePathSubtreeMutationFactor(
	transaction PathSubtreeMutation,
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	if !d.ownsPathSubtreeMutation(transaction) || skeleton.keys != transaction.keys {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: foreign coordinate path-subtree mutation")
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err == nil && coordinate.ops.pathMutation.participates {
		return d.applyCoordinatePathSubtreeMutation(transaction, skeleton, scalars)
	}
	owner, ok := d.PathValueFamily()
	if !ok || skeleton.family != owner {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: coordinate family does not own path-subtree mutation")
	}
	if err != nil || coordinate.ops.pathEvidence.open == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: invalid path coordinate carrier")
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, err
	}
	carrier, opened := coordinate.ops.pathEvidence.open(skeleton.payload, entries, skeleton.keys)
	if !opened || carrier == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: path coordinate carrier open failed")
	}
	if _, valid := carrier.InvalidateSubtreePrefixes(transaction.prefixes); !valid {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: path coordinate carrier rejected subtree mutation")
	}
	nextSkeleton, nextEntries, frozen := carrier.Freeze()
	if !frozen {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: path coordinate carrier freeze failed")
	}
	nextScalars := make([]CoordinateScalarFactor, len(nextEntries))
	for index, entry := range nextEntries {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, skeleton.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) ||
			index != 0 && !coordinate.ops.keyLess(nextEntries[index-1].key, entry.key, skeleton.keys) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: invalid path coordinate subtree result")
		}
		nextScalars[index] = CoordinateScalarFactor{
			slot: CoordinateSlot{family: owner, keys: skeleton.keys, key: entry.key}, payload: entry.scalar,
		}
	}
	return CoordinateFamilySkeleton{family: owner, keys: skeleton.keys, payload: nextSkeleton}, nextScalars, nil
}
