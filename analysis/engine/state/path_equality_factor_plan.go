package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// PathEqualityFactorPlan is the frozen carrier-neutral footprint of one exact
// path equality. The registered path family owns proof publication, path
// resolution, possible root-descendant invalidation and target writes in one
// dependency certificate.
type PathEqualityFactorPlan struct {
	seal           *productDomainSeal
	keys           *keyspace.KeySpace
	left, right    keyspace.Key
	leftRoot       keyspace.Key
	rightRoot      keyspace.Key
	leftValue      statekey.ValueDependency
	rightValue     statekey.ValueDependency
	proof          pathevidence.BranchProof
	coordinates    []CoordinateSlot
	reads          []CoordinateSlot
	writes         []CoordinateSlot
	readInventory  CoordinateFactorInventory
	writeInventory CoordinateFactorInventory
	regions        []CoordinateDependencyLocation
}

func (p PathEqualityFactorPlan) ValidFor(d ProductDomain) bool {
	return d.Valid() && p.seal == d.seal && p.keys != nil && p.keys.Valid() &&
		validPathEqualityProof(p.keys, p.proof) && p.leftValue.Valid() && p.rightValue.Valid()
}
func (p PathEqualityFactorPlan) KeySpace() *keyspace.KeySpace { return p.keys }
func (p PathEqualityFactorPlan) Left() keyspace.Key           { return p.left }
func (p PathEqualityFactorPlan) Right() keyspace.Key          { return p.right }
func (p PathEqualityFactorPlan) LeftRoot() keyspace.Key       { return p.leftRoot }
func (p PathEqualityFactorPlan) RightRoot() keyspace.Key      { return p.rightRoot }
func (p PathEqualityFactorPlan) LeftValue() statekey.ValueDependency {
	return p.leftValue
}
func (p PathEqualityFactorPlan) RightValue() statekey.ValueDependency {
	return p.rightValue
}
func (p PathEqualityFactorPlan) Proof() pathevidence.BranchProof { return p.proof }
func (p PathEqualityFactorPlan) Coordinates() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.coordinates...)
}
func (p PathEqualityFactorPlan) CoordinateReads() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.reads...)
}
func (p PathEqualityFactorPlan) CoordinateWrites() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.writes...)
}
func (p PathEqualityFactorPlan) CoordinateReadInventory() CoordinateFactorInventory {
	return p.readInventory
}
func (p PathEqualityFactorPlan) CoordinateWriteInventory() CoordinateFactorInventory {
	return p.writeInventory
}
func (p PathEqualityFactorPlan) MutationRegions() []CoordinateDependencyLocation {
	return append([]CoordinateDependencyLocation(nil), p.regions...)
}

// SealPathEqualityFactorPlan derives the complete equality cone once. Runtime
// Apply may neither discover another coordinate nor widen this certificate.
func (d ProductDomain) SealPathEqualityFactorPlan(
	keys *keyspace.KeySpace,
	left, right keyspace.Key,
	union []CoordinateSlot,
) (PathEqualityFactorPlan, error) {
	return d.sealPathEqualityFactorPlan(keys, left, right, union, false)
}

// SealPersistentPathEqualityFactorPlan seals the same equality cone plus the
// exact coordinate that retains the equality proof.
func (d ProductDomain) SealPersistentPathEqualityFactorPlan(
	keys *keyspace.KeySpace,
	left, right keyspace.Key,
	union []CoordinateSlot,
) (PathEqualityFactorPlan, error) {
	return d.sealPathEqualityFactorPlan(keys, left, right, union, true)
}

func (d ProductDomain) sealPathEqualityFactorPlan(
	keys *keyspace.KeySpace,
	left, right keyspace.Key,
	union []CoordinateSlot,
	_ bool,
) (PathEqualityFactorPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || left.Kind == keyspace.KindInvalid || right.Kind == keyspace.KindInvalid || left == right {
		return PathEqualityFactorPlan{}, fmt.Errorf("%w: invalid path-equality factor topology", ErrInvalidLaneFactor)
	}
	leftRoot, leftOK := keys.StructuralRoot(left)
	rightRoot, rightOK := keys.StructuralRoot(right)
	leftValue, leftValueOK := pathevidence.PathValueDependency(keys, leftRoot)
	rightValue, rightValueOK := pathevidence.PathValueDependency(keys, rightRoot)
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: left, Other: right}
	if !leftOK || !rightOK || !leftValueOK || !rightValueOK {
		return PathEqualityFactorPlan{}, fmt.Errorf("%w: unresolved path-equality roots", ErrInvalidLaneFactor)
	}
	seed := CoordinateDependencySeed{
		ID: 1, ResolvePaths: []keyspace.Key{left, right},
		DescendantMutationRoots: []keyspace.Key{leftRoot, rightRoot},
	}
	leftCongruence, rightCongruence := left, right
	if segments, segmentsOK := keys.SegmentsView(left); segmentsOK && len(segments) == 0 {
		leftCongruence = leftRoot
	}
	if segments, segmentsOK := keys.SegmentsView(right); segmentsOK && len(segments) == 0 {
		rightCongruence = rightRoot
	}
	seed.TransientEqualities = []CoordinateDependencyEquality{{Left: leftCongruence, Right: rightCongruence}}
	proofSlot, err := d.PathBranchProofCoordinateSlot(keys, proof)
	if err != nil {
		return PathEqualityFactorPlan{}, err
	}
	// Transient equality still needs the exact proof coordinate as scratch
	// authority for closure, although only persistent mode retains it.
	seed.AddCoordinates = append(seed.AddCoordinates, proofSlot)
	for _, target := range []struct{ path, root keyspace.Key }{{left, leftRoot}, {right, rightRoot}} {
		if target.path == target.root {
			continue
		}
		slot, err := d.PathRefinementCoordinateSlot(keys, target.path)
		if err != nil {
			return PathEqualityFactorPlan{}, err
		}
		seed.WritePaths = append(seed.WritePaths, target.path)
		seed.AddCoordinates = append(seed.AddCoordinates, slot)
	}
	dependencies, err := d.PlanPathCoordinateDependencies(keys, union, []CoordinateDependencySeed{seed})
	if err != nil {
		return PathEqualityFactorPlan{}, err
	}
	dependency, present := dependencies.Dependency(seed.ID)
	if !present {
		return PathEqualityFactorPlan{}, fmt.Errorf("%w: missing path-equality dependency certificate", ErrInvalidLaneFactor)
	}
	out := PathEqualityFactorPlan{
		seal: d.seal, keys: keys, left: left, right: right, leftRoot: leftRoot, rightRoot: rightRoot,
		leftValue: leftValue, rightValue: rightValue, proof: proof,
		coordinates: dependencies.Coordinates(), reads: dependency.CoordinateReads(), writes: dependency.CoordinateWrites(),
		regions: dependency.MutationRegions(),
	}
	out.readInventory, err = d.SealCoordinateFactorInventory(keys, out.reads)
	if err != nil {
		return PathEqualityFactorPlan{}, err
	}
	out.writeInventory, err = d.SealCoordinateFactorInventory(keys, out.writes)
	if err != nil {
		return PathEqualityFactorPlan{}, err
	}
	return out, nil
}
