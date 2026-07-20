package state

import (
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
)

type numericConsistencyStatus uint8

const (
	numericConsistencyUnknown numericConsistencyStatus = iota
	numericConsistencyDirty
	numericConsistencyCertified
)

type numericConsistencyMutation struct {
	keys  [3]pathdom.PathKey
	count uint8
}

type laneNumericConsistencyKind uint8

const (
	laneNumericConsistencyInvalid laneNumericConsistencyKind = iota
	laneNumericConsistencyIndependent
	laneNumericConsistencyContributor
)

// laneNumericConsistencyPolicy is the exhaustive cross-axis arithmetic law of
// one State lane. Every lane must explicitly declare independence or append
// its exact affine assertions. The invalid zero value makes catalog growth
// fail closed instead of silently omitting a new numeric axis.
type laneNumericConsistencyPolicy struct {
	kind       laneNumericConsistencyKind
	contribute func(State, *numericConsistencyBuilder)
}

func numericConsistencyIndependent() laneNumericConsistencyPolicy {
	return laneNumericConsistencyPolicy{kind: laneNumericConsistencyIndependent}
}

func numericConsistencyContributor(contribute func(State, *numericConsistencyBuilder)) laneNumericConsistencyPolicy {
	return laneNumericConsistencyPolicy{kind: laneNumericConsistencyContributor, contribute: contribute}
}

type numericConsistencyBuilder struct {
	assertions []numeric.NumericConstraint
	valid      bool
}

func newNumericConsistencyBuilder() numericConsistencyBuilder {
	return numericConsistencyBuilder{valid: true}
}

func (b *numericConsistencyBuilder) add(constraint numeric.NumericConstraint) {
	if b == nil || constraint == nil {
		if b != nil {
			b.valid = false
		}
		return
	}
	switch constraint.(type) {
	case numeric.SumLe, numeric.GeConst, numeric.LeConst:
	default:
		b.valid = false
		return
	}
	b.assertions = append(b.assertions, constraint)
}

func (b *numericConsistencyBuilder) addNumBound(keyPath pathdom.PathKey, value int64, lower bool) {
	if keyPath == "" {
		b.valid = false
		return
	}
	if lower {
		b.add(numeric.GeConst{X: keyPath, C: value})
		return
	}
	b.add(numeric.LeConst{X: keyPath, C: value})
}

func (s *State) invalidateNumericConsistency() {
	if s != nil {
		s.numericConsistency = numericConsistencyUnknown
		s.numericConsistencyMutation = nil
	}
}

// markNumericConsistencyDirty records the exact support of one assertion
// added to a previously certified state. This lets publication solve only the
// affected connected component while arbitrary/batched changes remain on the
// exact whole-conjunction path.
func (s *State) markNumericConsistencyDirty(previous numericConsistencyStatus, keys ...pathdom.PathKey) {
	if s == nil || previous != numericConsistencyCertified {
		return
	}
	mutation := &numericConsistencyMutation{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		duplicate := false
		for index := 0; index < int(mutation.count); index++ {
			if mutation.keys[index] == key {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if int(mutation.count) == len(mutation.keys) {
			// Unreachable for the registered affine assertion vocabulary. Keep
			// the exact full-check spelling if that vocabulary grows without
			// updating this representation.
			s.invalidateNumericConsistency()
			return
		}
		mutation.keys[mutation.count] = key
		mutation.count++
	}
	if mutation.count == 0 {
		return
	}
	s.numericConsistency = numericConsistencyDirty
	s.numericConsistencyMutation = mutation
}

func setStateNumFloors(out *State, lane numBoundLane) {
	changedAssertions := numericBoundLaneHasAssertions(out.numFloors) || numericBoundLaneHasAssertions(lane)
	out.numFloors = lane
	if changedAssertions {
		out.invalidateNumericConsistency()
	}
}

func setStateNumCeils(out *State, lane numBoundLane) {
	changedAssertions := numericBoundLaneHasAssertions(out.numCeils) || numericBoundLaneHasAssertions(lane)
	out.numCeils = lane
	if changedAssertions {
		out.invalidateNumericConsistency()
	}
}

func setStateLenFloors(out *State, lane lenFloorLane) {
	changedAssertions := lenFloorLaneHasAssertions(out.lenFloors) || lenFloorLaneHasAssertions(lane)
	out.lenFloors = lane
	if changedAssertions {
		out.invalidateNumericConsistency()
	}
}

func setStateDiffRelations(out *State, lane diffRelationLane) {
	changedAssertions := diffRelationLaneHasAssertions(out.diffRelations) || diffRelationLaneHasAssertions(lane)
	out.diffRelations = lane
	if changedAssertions {
		out.invalidateNumericConsistency()
	}
}

func numericBoundLaneHasAssertions(lane numBoundLane) bool { return len(lane.lane.Values()) != 0 }
func lenFloorLaneHasAssertions(lane lenFloorLane) bool     { return len(lane.lane.Values()) != 0 }
func diffRelationLaneHasAssertions(lane diffRelationLane) bool {
	return len(lane.values) != 0
}

func contributeNumFloors(s State, out *numericConsistencyBuilder) {
	contributeNumBounds(s.numFloors, true, out)
}

func contributeNumCeils(s State, out *numericConsistencyBuilder) {
	contributeNumBounds(s.numCeils, false, out)
}

func contributeNumBounds(lane numBoundLane, lower bool, out *numericConsistencyBuilder) {
	if lane.lane.Bottom() {
		return
	}
	for key, bound := range lane.lane.Values() {
		path, ok := key.PathKey()
		if !ok {
			out.valid = false
			return
		}
		out.addNumBound(path, bound, lower)
	}
}

func contributeLenFloors(s State, out *numericConsistencyBuilder) {
	if s.lenFloors.lane.Bottom() {
		return
	}
	for key, floor := range s.lenFloors.lane.Values() {
		path, ok := key.PathKey()
		stateKey, stateOK := pathaddr.StateKeyFromPathKey(path)
		if !ok || !stateOK {
			out.valid = false
			return
		}
		out.addNumBound(RelLengthOperand(stateKey).NumericKey(), floor.Lo, true)
	}
}

func contributeDiffRelations(s State, out *numericConsistencyBuilder) {
	if s.diffRelations.bottom {
		return
	}
	for relation := range s.diffRelations.values {
		canonical, ok := canonicalRelConstraint(relation)
		if !ok || canonical != relation {
			out.valid = false
			return
		}
		out.add(relation.NumericConstraint())
	}
}

// certifyNumericConsistency applies the single product-level arithmetic
// invariant. Each connected variable component is checked independently by
// the exact, unbudgeted affine solver. Disconnected satisfiable components
// cannot affect one another, while any inconsistent component makes the whole
// abstract State unreachable.
func (d ProductDomain) certifyNumericConsistency(value State) State {
	if !d.Valid() || value.numericConsistency == numericConsistencyCertified {
		return value
	}
	if d.lattice.Equal(value, d.lattice.Bottom()) {
		return d.lattice.Bottom()
	}
	builder := newNumericConsistencyBuilder()
	for index := range d.factorLanes {
		policy := d.factorLanes[index].numericConsistency
		if policy.kind == laneNumericConsistencyContributor {
			policy.contribute(value, &builder)
		}
	}
	satisfiable := builder.valid
	if satisfiable && value.numericConsistency == numericConsistencyDirty && value.numericConsistencyMutation != nil {
		mutation := value.numericConsistencyMutation
		satisfiable = numericAssertionsComponentSatisfiable(builder.assertions, mutation.keys[:mutation.count])
	} else if satisfiable {
		satisfiable = numericAssertionsSatisfiable(builder.assertions)
	}
	if !satisfiable {
		return d.lattice.Bottom()
	}
	value.numericConsistency = numericConsistencyCertified
	value.numericConsistencyMutation = nil
	return value
}

func certifyNumericConsistencyForLattice(value, bottom State, lanes []laneOps, specs []laneSpec) State {
	if value.numericConsistency == numericConsistencyCertified {
		return value
	}
	if lanesEqual(lanes, value, bottom) {
		return bottom
	}
	builder := newNumericConsistencyBuilder()
	for index := range specs {
		policy := specs[index].numericConsistency
		if policy.kind == laneNumericConsistencyContributor {
			policy.contribute(value, &builder)
		}
	}
	satisfiable := builder.valid
	if satisfiable && value.numericConsistency == numericConsistencyDirty && value.numericConsistencyMutation != nil {
		mutation := value.numericConsistencyMutation
		satisfiable = numericAssertionsComponentSatisfiable(builder.assertions, mutation.keys[:mutation.count])
	} else if satisfiable {
		satisfiable = numericAssertionsSatisfiable(builder.assertions)
	}
	if !satisfiable {
		return bottom
	}
	value.numericConsistency = numericConsistencyCertified
	value.numericConsistencyMutation = nil
	return value
}

// numericAssertionsComponentSatisfiable decides only the connected components
// incident to seeds. The input state was already certified before one new
// assertion over seeds was added, so disconnected components remain
// satisfiable by construction. Connectivity is still rebuilt exactly from the
// current conjunction; no cached graph, budget, or stale component identity is
// trusted.
func numericAssertionsComponentSatisfiable(assertions []numeric.NumericConstraint, seeds []pathdom.PathKey) bool {
	if len(assertions) == 0 || len(seeds) == 0 {
		return true
	}
	variables := make(map[pathdom.PathKey]int)
	parents := make([]int, 0)
	var root func(int) int
	root = func(index int) int {
		for parents[index] != index {
			parents[index] = parents[parents[index]]
			index = parents[index]
		}
		return index
	}
	variable := func(key pathdom.PathKey) int {
		if index, ok := variables[key]; ok {
			return index
		}
		index := len(parents)
		variables[key] = index
		parents = append(parents, index)
		return index
	}
	union := func(left, right int) {
		left, right = root(left), root(right)
		if left != right {
			parents[right] = left
		}
	}
	rowVariable := make([]int, len(assertions))
	for index := range rowVariable {
		rowVariable[index] = -1
	}
	for row, assertion := range assertions {
		var storage [3]pathdom.PathKey
		keys := numericConstraintKeys(storage[:0], assertion)
		for _, key := range keys {
			if key == "" {
				continue
			}
			index := variable(key)
			if rowVariable[row] < 0 {
				rowVariable[row] = index
			} else {
				union(rowVariable[row], index)
			}
		}
	}
	dirtyRoots := make(map[int]struct{}, len(seeds))
	for _, seed := range seeds {
		if index, ok := variables[seed]; ok {
			dirtyRoots[root(index)] = struct{}{}
		}
	}
	affected := make([]numeric.NumericConstraint, 0, len(seeds)+1)
	for row, assertion := range assertions {
		if rowVariable[row] < 0 {
			affected = append(affected, assertion)
			continue
		}
		if _, dirty := dirtyRoots[root(rowVariable[row])]; dirty {
			affected = append(affected, assertion)
		}
	}
	return solver.AffineSatisfiable(affected)
}

// numericAssertionsSatisfiable partitions the affine conjunction by variable
// connectivity before invoking simplex. This is exact because components have
// disjoint supports; it also makes thousands of unrelated facts linear rather
// than one large dense tableau.
func numericAssertionsSatisfiable(assertions []numeric.NumericConstraint) bool {
	if len(assertions) == 0 {
		return true
	}
	variables := make(map[pathdom.PathKey]int)
	parents := make([]int, 0)
	var root func(int) int
	root = func(index int) int {
		for parents[index] != index {
			parents[index] = parents[parents[index]]
			index = parents[index]
		}
		return index
	}
	variable := func(key pathdom.PathKey) int {
		if index, ok := variables[key]; ok {
			return index
		}
		index := len(parents)
		variables[key] = index
		parents = append(parents, index)
		return index
	}
	union := func(left, right int) {
		left, right = root(left), root(right)
		if left != right {
			parents[right] = left
		}
	}

	rowVariable := make([]int, len(assertions))
	for index := range rowVariable {
		rowVariable[index] = -1
	}
	for row, assertion := range assertions {
		var storage [3]pathdom.PathKey
		keys := numericConstraintKeys(storage[:0], assertion)
		for _, key := range keys {
			if key == "" {
				continue
			}
			index := variable(key)
			if rowVariable[row] < 0 {
				rowVariable[row] = index
			} else {
				union(rowVariable[row], index)
			}
		}
	}
	type componentAssertions struct {
		first numeric.NumericConstraint
		rest  []numeric.NumericConstraint
	}
	components := make(map[int]*componentAssertions)
	constant := make([]numeric.NumericConstraint, 0)
	for row, assertion := range assertions {
		if rowVariable[row] < 0 {
			constant = append(constant, assertion)
			continue
		}
		component := root(rowVariable[row])
		group := components[component]
		if group == nil {
			components[component] = &componentAssertions{first: assertion}
		} else {
			group.rest = append(group.rest, assertion)
		}
	}
	if len(constant) != 0 && !solver.AffineSatisfiable(constant) {
		return false
	}
	for _, component := range components {
		if len(component.rest) == 0 && singleAffineConstraintSatisfiable(component.first) {
			continue
		}
		asserted := make([]numeric.NumericConstraint, 1, len(component.rest)+1)
		asserted[0] = component.first
		asserted = append(asserted, component.rest...)
		if !solver.AffineSatisfiable(asserted) {
			return false
		}
	}
	return true
}

func numericConstraintKeys(out []pathdom.PathKey, constraint numeric.NumericConstraint) []pathdom.PathKey {
	switch value := constraint.(type) {
	case numeric.Le:
		return append(out, value.X, value.Y)
	case numeric.SumLe:
		if value.CoX != 0 {
			out = append(out, value.X)
		}
		if value.Y != "" && value.CoY != 0 {
			out = append(out, value.Y)
		}
		return append(out, value.Z)
	case numeric.GeConst:
		return append(out, value.X)
	case numeric.LeConst:
		return append(out, value.X)
	default:
		return out
	}
}

// singleAffineConstraintSatisfiable recognizes the exact one-row case without
// constructing a simplex. Any non-constant affine half-space has a model;
// only a row whose coefficients cancel completely can be contradictory.
func singleAffineConstraintSatisfiable(constraint numeric.NumericConstraint) bool {
	switch value := constraint.(type) {
	case numeric.GeConst, numeric.LeConst:
		return true
	case numeric.Le:
		return value.X != value.Y || value.C >= 0
	case numeric.SumLe:
		if value.Y == "" && value.X != value.Z {
			return true
		}
		if value.Y != "" && value.X != value.Y && value.X != value.Z && value.Y != value.Z {
			return true
		}
		// Repeated variables are rare; let the exact solver decide coefficient
		// cancellation without duplicating its arbitrary-precision arithmetic.
		return false
	default:
		return false
	}
}
