package pathevidence

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CoordinateDependencyID is a caller-owned stable identity for one semantic
// dependency seed. It is never interpreted as a position in a coordinate
// inventory, so augmenting or sorting that inventory cannot retarget a plan.
type CoordinateDependencyID uint64

// CoordinateDependencySeed describes path-family dependency shape without
// naming an operation. Fact application owns the meaning of the operation;
// this package owns path congruence, coordinate observations and invalidation.
type CoordinateDependencySeed struct {
	ID        CoordinateDependencyID
	ReadPaths []keyspace.Key
	// ResolvePaths are value-resolving observations. In addition to the exact
	// structural path coordinate, a segmented path reads its normalized root
	// Values location because canonical projection falls back through that root.
	ResolvePaths            []keyspace.Key
	WritePaths              []keyspace.Key
	DescendantMutationRoots []keyspace.Key
	// SubtreeMutationRoots are inclusive destructive replacements: evidence at
	// each root and below it, including equality-rebased aliases, is part of the
	// exact registered read/write certificate.
	SubtreeMutationRoots []keyspace.Key
	// StableRootMutations name lexical roots whose stable/unversioned evidence
	// may be removed or retained by one atomic mutation. The coordinate family
	// owns the exact affected inventory; callers do not classify coordinates.
	StableRootMutations []symbol.ID
	// TransientEqualities close existing data proofs and path refinements across
	// an operation-local equality without publishing an equality proof.
	TransientEqualities []CoordinateDependencyEquality
	// ReadCoordinates are exact representation-owned scalar observations not
	// derivable from path reads (for example Horn-clause membership).
	ReadCoordinates []CoordinateKey
	// AddCoordinates are new operation-owned scalar publications. Each is both
	// inserted into Coordinates and returned in this seed's CoordinateWrites.
	AddCoordinates []CoordinateKey
}

type CoordinateDependencyEquality struct {
	Left, Right keyspace.Key
}

// CoordinateDependencyLocation is one normalized semantic storage location.
// Segment-free resolver/stable/unversioned spellings share Root. Structural
// paths remain exact KeySpace identities.
type CoordinateDependencyLocation struct {
	Root statekey.ValueDependency
	Path keyspace.Key
}

func (l CoordinateDependencyLocation) IsRoot() bool { return l.Root.Valid() }

// CoordinateDependency is an immutable snapshot returned by Dependency.
// Its slices are detached from the owning plan.
type CoordinateDependency struct {
	ID               CoordinateDependencyID
	CoordinateReads  []CoordinateKey
	CoordinateWrites []CoordinateKey
	LocationReads    []CoordinateDependencyLocation
	LocationWrites   []CoordinateDependencyLocation
	MutationRegions  []CoordinateDependencyLocation
	mutations        []coordinateDependencyMutation
}

// coordinateDependencyMutation retains the distinction between strict
// descendant invalidation and inclusive subtree invalidation. The public
// normalized region inventory deliberately omits this execution detail, while
// the sealed dependency graph must preserve it to avoid coupling a root-only
// read to a strict-descendant mutation.
type coordinateDependencyMutation struct {
	path          keyspace.Key
	includePrefix bool
}

type coordinateDependencyEdge struct {
	writer CoordinateDependencyID
	reader CoordinateDependencyID
}

// CoordinateDependencyPlan is the family-owned, frozen union-coordinate
// certificate. Internal maps and slices are never exposed directly.
type CoordinateDependencyPlan struct {
	keys        *keyspace.KeySpace
	coordinates []CoordinateKey
	order       []CoordinateDependencyID
	byID        map[CoordinateDependencyID]CoordinateDependency
	depends     map[coordinateDependencyEdge]struct{}
	feeds       map[coordinateDependencyEdge]struct{}
}

func (p CoordinateDependencyPlan) Coordinates() []CoordinateKey {
	return append([]CoordinateKey(nil), p.coordinates...)
}

func (p CoordinateDependencyPlan) IDs() []CoordinateDependencyID {
	return append([]CoordinateDependencyID(nil), p.order...)
}

func (p CoordinateDependencyPlan) Dependency(id CoordinateDependencyID) (CoordinateDependency, bool) {
	value, ok := p.byID[id]
	if !ok {
		return CoordinateDependency{}, false
	}
	return cloneCoordinateDependency(value), true
}

// Depends reports whether writer may change an observation read or written by
// reader. It is directional: shared read-only support is independent, W->R is
// an edge, and W/W is represented by both directional queries. All path alias,
// semantic-root and descendant reasoning is sealed here by the owning family.
func (p CoordinateDependencyPlan) Depends(writer, reader CoordinateDependencyID) bool {
	_, ok := p.depends[coordinateDependencyEdge{writer: writer, reader: reader}]
	return ok
}

// Feeds reports the directional RAW relation from writer output to reader's
// explicit observations. Reads used only to accumulate reader's own target are
// excluded, as are WAW-only conflicts; neither is evidence that another
// consequence round can enable the reader.
func (p CoordinateDependencyPlan) Feeds(writer, reader CoordinateDependencyID) bool {
	_, ok := p.feeds[coordinateDependencyEdge{writer: writer, reader: reader}]
	return ok
}

func (p CoordinateDependencyPlan) AllowsLocationWrite(id CoordinateDependencyID, path keyspace.Key) bool {
	value, ok := p.byID[id]
	if !ok {
		return false
	}
	location := coordinateDependencyLocation(p.keys, path)
	return containsCoordinateDependencyLocation(value.LocationWrites, location)
}

func (p CoordinateDependencyPlan) AllowsCoordinateWrite(id CoordinateDependencyID, key CoordinateKey) bool {
	value, ok := p.byID[id]
	return ok && containsCoordinateDependencyKey(value.CoordinateWrites, key)
}

// PlanCoordinateDependencies computes the finite least union-coordinate
// closure under the canonical path congruence and descendant invalidation
// laws. It has no semantic work/depth cap.
func PlanCoordinateDependencies(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	union []CoordinateKey,
	seeds []CoordinateDependencySeed,
) (CoordinateDependencyPlan, bool) {
	if reg == nil || ks == nil || !ks.Valid() || len(seeds) == 0 {
		return CoordinateDependencyPlan{}, false
	}
	coordinates := append([]CoordinateKey(nil), union...)
	seenIDs := make(map[CoordinateDependencyID]struct{}, len(seeds))
	for _, seed := range seeds {
		if seed.ID == 0 {
			return CoordinateDependencyPlan{}, false
		}
		if _, duplicate := seenIDs[seed.ID]; duplicate {
			return CoordinateDependencyPlan{}, false
		}
		seenIDs[seed.ID] = struct{}{}
		for _, coordinate := range seed.AddCoordinates {
			coordinates = appendCoordinateDependencyKey(coordinates, coordinate)
		}
		if !coordinateDependencySeedValid(ks, seed) {
			return CoordinateDependencyPlan{}, false
		}
	}
	for _, coordinate := range coordinates {
		if !CoordinateKeyValid(coordinate, ks, reg) {
			return CoordinateDependencyPlan{}, false
		}
	}
	for _, seed := range seeds {
		for _, coordinate := range seed.ReadCoordinates {
			if !containsCoordinateDependencyKey(coordinates, coordinate) {
				return CoordinateDependencyPlan{}, false
			}
		}
	}
	coordinates = closeCoordinateBranchProofUniverse(coordinates, ks)
	coordinates = closeCoordinateTransientEqualityUniverse(coordinates, seeds, ks)
	coordinates = canonicalCoordinateDependencyKeys(coordinates, ks)

	// Adding a refinement output makes an already-known path an explicit finite
	// observation. Repeat until alias expansion adds no output identity.
	for {
		lane, ok := coordinateDependencyMayLane(reg, ks, coordinates)
		if !ok {
			return CoordinateDependencyPlan{}, false
		}
		changed := false
		for _, seed := range seeds {
			for _, path := range seed.WritePaths {
				for _, target := range coordinateDependencyAliases(lane, ks, path) {
					if coordinateDependencyLocation(ks, target).IsRoot() {
						continue
					}
					before := len(coordinates)
					coordinates = appendCoordinateDependencyKey(coordinates, RefinementCoordinate(target))
					changed = changed || len(coordinates) != before
				}
			}
		}
		if !changed {
			break
		}
		coordinates = canonicalCoordinateDependencyKeys(coordinates, ks)
	}

	lane, ok := coordinateDependencyMayLane(reg, ks, coordinates)
	if !ok {
		return CoordinateDependencyPlan{}, false
	}
	congruence := newPathCongruence(ks, lane)
	plan := CoordinateDependencyPlan{
		keys:        ks,
		coordinates: append([]CoordinateKey(nil), coordinates...),
		order:       make([]CoordinateDependencyID, 0, len(seeds)),
		byID:        make(map[CoordinateDependencyID]CoordinateDependency, len(seeds)),
		depends:     make(map[coordinateDependencyEdge]struct{}),
		feeds:       make(map[coordinateDependencyEdge]struct{}),
	}
	feedReaders := make(map[CoordinateDependencyID]CoordinateDependency, len(seeds))
	for _, seed := range seeds {
		dependency, valid := buildCoordinateDependency(reg, ks, coordinates, lane, congruence, seed)
		if !valid {
			return CoordinateDependencyPlan{}, false
		}
		feedSeed := seed
		feedSeed.WritePaths = nil
		feedSeed.DescendantMutationRoots = nil
		feedSeed.SubtreeMutationRoots = nil
		feedSeed.StableRootMutations = nil
		feedSeed.AddCoordinates = nil
		feedReader, valid := buildCoordinateDependency(reg, ks, coordinates, lane, congruence, feedSeed)
		if !valid {
			return CoordinateDependencyPlan{}, false
		}
		plan.order = append(plan.order, seed.ID)
		plan.byID[seed.ID] = dependency
		feedReaders[seed.ID] = feedReader
	}
	for _, writer := range plan.order {
		for _, reader := range plan.order {
			if coordinateDependencyAffects(plan.byID[writer], plan.byID[reader], ks) {
				plan.depends[coordinateDependencyEdge{writer: writer, reader: reader}] = struct{}{}
			}
			if coordinateDependencyAffects(plan.byID[writer], feedReaders[reader], ks) {
				plan.feeds[coordinateDependencyEdge{writer: writer, reader: reader}] = struct{}{}
			}
		}
	}
	return plan, true
}

func buildCoordinateDependency(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	coordinates []CoordinateKey,
	lane Lane,
	congruence *pathCongruence,
	seed CoordinateDependencySeed,
) (CoordinateDependency, bool) {
	out := CoordinateDependency{ID: seed.ID}
	writePaths := make([]keyspace.Key, 0, len(seed.WritePaths))
	for _, path := range seed.ReadPaths {
		out.LocationReads = appendCoordinateDependencyLocation(out.LocationReads, coordinateDependencyLocation(ks, path))
		key := RefinementCoordinate(path)
		if containsCoordinateDependencyKey(coordinates, key) {
			out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, key)
		}
	}
	for _, path := range seed.ResolvePaths {
		location := coordinateDependencyLocation(ks, path)
		out.LocationReads = appendCoordinateDependencyLocation(out.LocationReads, location)
		key := RefinementCoordinate(path)
		if containsCoordinateDependencyKey(coordinates, key) {
			out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, key)
		}
		if path.Segs != 0 {
			if root, ok := PathValueDependency(ks, path); ok {
				out.LocationReads = appendCoordinateDependencyLocation(out.LocationReads, CoordinateDependencyLocation{Root: root})
			}
		}
	}
	for _, path := range seed.WritePaths {
		for _, target := range coordinateDependencyAliases(lane, ks, path) {
			writePaths = appendUniqueDependencyPath(writePaths, target)
			location := coordinateDependencyLocation(ks, target)
			out.LocationReads = appendCoordinateDependencyLocation(out.LocationReads, location)
			out.LocationWrites = appendCoordinateDependencyLocation(out.LocationWrites, location)
			if !location.IsRoot() {
				key := RefinementCoordinate(target)
				out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, key)
				out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, key)
			}
		}
	}

	supportPaths := append([]keyspace.Key(nil), seed.ReadPaths...)
	supportPaths = append(supportPaths, seed.ResolvePaths...)
	supportPaths = append(supportPaths, writePaths...)
	supportPaths = append(supportPaths, seed.DescendantMutationRoots...)
	supportPaths = append(supportPaths, seed.SubtreeMutationRoots...)
	// A published Presence or IndexInRange proof is evaluated by the same
	// equality closure as an ordinary path observation.  Make its structural
	// paths explicit support here so the finite dependency certificate carries
	// the equality proof scalars which can enable mirrored publications.  The
	// publication coordinates themselves remain exact writes below.
	supportPaths = append(supportPaths, coordinateDependencyPublicationSupportPaths(seed.AddCoordinates)...)
	dependencyClasses := coordinateDependencyClasses(congruence, supportPaths)
	for _, coordinate := range coordinates {
		if coordinate.kind == coordinateBranchProof && coordinate.proof.Kind == BranchProofPathEqual {
			left, leftOK := congruence.term(coordinate.proof.Path)
			right, rightOK := congruence.term(coordinate.proof.Other)
			if leftOK && rightOK {
				_, leftRelevant := dependencyClasses[congruence.find(left)]
				_, rightRelevant := dependencyClasses[congruence.find(right)]
				if leftRelevant || rightRelevant {
					out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
				}
			}
		}
		if coordinateDependencyObservesAnyAlias(congruence, coordinate, supportPaths) {
			out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
		}
	}

	for _, root := range seed.DescendantMutationRoots {
		prefixes, valid := lane.PathKeyDescendantInvalidationPrefixes(ks, ks.Format(root))
		if !valid {
			return CoordinateDependency{}, false
		}
		for _, prefix := range prefixes.Descendants {
			key, valid := ks.FromStateKey(prefix)
			if valid {
				out.MutationRegions = appendCoordinateDependencyLocation(out.MutationRegions, coordinateDependencyLocation(ks, key))
				out.mutations = appendCoordinateDependencyMutation(out.mutations, coordinateDependencyMutation{path: key})
			}
		}
		for _, prefix := range prefixes.Subtrees {
			key, valid := ks.FromStateKey(prefix)
			if valid {
				out.MutationRegions = appendCoordinateDependencyLocation(out.MutationRegions, coordinateDependencyLocation(ks, key))
				out.mutations = appendCoordinateDependencyMutation(out.mutations, coordinateDependencyMutation{path: key, includePrefix: true})
			}
		}
		mutated := lane.InvalidatePathKeyDescendantPrefixes(ks, prefixes)
		for _, coordinate := range coordinates {
			if coordinateDependencyLaneHas(lane, coordinate) && !coordinateDependencyLaneHas(mutated, coordinate) {
				out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
				out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
			}
		}
	}
	for _, root := range seed.SubtreeMutationRoots {
		prefixes, valid := lane.PathKeySubtreeInvalidationPrefixes(ks, ks.Format(root))
		if !valid {
			return CoordinateDependency{}, false
		}
		for _, prefix := range prefixes {
			key, valid := ks.FromStateKey(prefix)
			if valid {
				out.MutationRegions = appendCoordinateDependencyLocation(out.MutationRegions, coordinateDependencyLocation(ks, key))
				out.mutations = appendCoordinateDependencyMutation(out.mutations, coordinateDependencyMutation{path: key, includePrefix: true})
			}
		}
		mutated := lane.InvalidatePathKeySubtreePrefixes(ks, prefixes)
		for _, coordinate := range coordinates {
			if coordinateDependencyLaneHas(lane, coordinate) && !coordinateDependencyLaneHas(mutated, coordinate) {
				out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
				out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
			}
		}
	}
	for _, root := range seed.StableRootMutations {
		location := CoordinateDependencyLocation{Root: statekey.ConcreteDependency(statekey.SymbolValue(root))}
		out.LocationReads = appendCoordinateDependencyLocation(out.LocationReads, location)
		out.LocationWrites = appendCoordinateDependencyLocation(out.LocationWrites, location)
		out.MutationRegions = appendCoordinateDependencyLocation(out.MutationRegions, location)
		for _, coordinate := range coordinates {
			if !StableRootMutationRemovesCoordinate(coordinate, root, false, nil) {
				continue
			}
			out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
			out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
		}
	}
	for _, coordinate := range seed.AddCoordinates {
		out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
	}
	for _, coordinate := range seed.ReadCoordinates {
		out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
	}
	if len(seed.TransientEqualities) != 0 {
		for _, coordinate := range coordinates {
			if coordinate.kind == coordinateBranchProof {
				out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
			}
		}
		refinementReads, refinementWrites := coordinateDependencyTransientEqualityRefinementAccess(
			coordinates, seed.TransientEqualities, ks,
		)
		for _, coordinate := range refinementReads {
			out.CoordinateReads = appendCoordinateDependencyKey(out.CoordinateReads, coordinate)
		}
		for _, coordinate := range refinementWrites {
			out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
		}
		for _, coordinate := range coordinateDependencyTransientEqualityClosure(coordinates, seed.TransientEqualities, ks) {
			out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
		}
	}
	for _, coordinate := range coordinateDependencyPublicationClosure(coordinates, seed.AddCoordinates, ks) {
		out.CoordinateWrites = appendCoordinateDependencyKey(out.CoordinateWrites, coordinate)
	}
	canonicalizeCoordinateDependency(&out, ks)
	return out, true
}

func coordinateDependencyPublicationSupportPaths(publications []CoordinateKey) []keyspace.Key {
	var out []keyspace.Key
	for _, coordinate := range publications {
		if coordinate.kind != coordinateBranchProof {
			continue
		}
		switch coordinate.proof.Kind {
		case BranchProofPathPresence:
			out = appendUniqueDependencyPath(out, coordinate.proof.Path)
		case BranchProofIndexInRange:
			out = appendUniqueDependencyPath(out, coordinate.proof.Path)
			if coordinate.proof.Other.Kind != keyspace.KindInvalid {
				out = appendUniqueDependencyPath(out, coordinate.proof.Other)
			}
		}
	}
	return out
}

// coordinateDependencyTransientEqualityRefinementAccess is the finite
// registered-coordinate image of closeCongruenceAcrossEquality.  Concrete
// execution snapshots existing path refinements and rebases those structurally
// beneath either equality endpoint in both directions.  The coordinate family
// therefore reads exactly those registered sources and authorizes exactly the
// registered mirrored destinations; unrelated siblings never enter the
// factor.  Member-table eligibility is a runtime value question, so this
// certificate conservatively carries both possible directions while the
// canonical kernel decides whether a write occurs.
func coordinateDependencyTransientEqualityRefinementAccess(
	coordinates []CoordinateKey,
	equalities []CoordinateDependencyEquality,
	ks *keyspace.KeySpace,
) (reads, writes []CoordinateKey) {
	if ks == nil || len(equalities) == 0 {
		return nil, nil
	}
	registered := make(map[CoordinateKey]struct{}, len(coordinates))
	for _, coordinate := range coordinates {
		registered[coordinate] = struct{}{}
	}
	for _, source := range coordinates {
		if source.kind != coordinateRefinement {
			continue
		}
		for _, equality := range equalities {
			for _, direction := range [][2]keyspace.Key{
				{equality.Left, equality.Right},
				{equality.Right, equality.Left},
			} {
				rebased, ok := rebaseCoordinateRefinementAcrossEquality(ks, source.path, direction[0], direction[1])
				if !ok {
					continue
				}
				target := RefinementCoordinate(rebased)
				if _, present := registered[target]; !present {
					continue
				}
				reads = appendCoordinateDependencyKey(reads, source)
				writes = appendCoordinateDependencyKey(writes, target)
			}
		}
	}
	return reads, writes
}

func closeCoordinateBranchProofUniverse(coordinates []CoordinateKey, ks *keyspace.KeySpace) []CoordinateKey {
	proofs := make([]BranchProof, 0)
	for _, coordinate := range coordinates {
		if coordinate.kind == coordinateBranchProof {
			proofs = append(proofs, coordinate.proof)
		}
	}
	for _, proof := range CloseBranchProofsAcrossKnownEqualities(ks, proofs) {
		coordinates = appendCoordinateDependencyKey(coordinates, BranchProofCoordinate(proof))
	}
	return coordinates
}

func closeCoordinateTransientEqualityUniverse(coordinates []CoordinateKey, seeds []CoordinateDependencySeed, ks *keyspace.KeySpace) []CoordinateKey {
	// One concrete equality kernel snapshots its input refinements and rebases
	// each source once in both directions.  Freeze that exact finite image into
	// the coordinate universe before proof closure.  Multiple transaction atoms
	// retain separate seeds, so each atom's image is derived from the same
	// body-wide registered universe without inventing a private convergence
	// loop over path spellings.
	registered := append([]CoordinateKey(nil), coordinates...)
	for _, seed := range seeds {
		for _, coordinate := range coordinateDependencyTransientEqualityRefinementUniverse(registered, seed.TransientEqualities, ks) {
			coordinates = appendCoordinateDependencyKey(coordinates, coordinate)
		}
	}
	for changed := true; changed; {
		changed = false
		for _, seed := range seeds {
			for _, coordinate := range coordinateDependencyTransientEqualityClosure(coordinates, seed.TransientEqualities, ks) {
				before := len(coordinates)
				coordinates = appendCoordinateDependencyKey(coordinates, coordinate)
				changed = changed || len(coordinates) != before
			}
		}
	}
	return coordinates
}

func coordinateDependencyTransientEqualityRefinementUniverse(
	coordinates []CoordinateKey,
	equalities []CoordinateDependencyEquality,
	ks *keyspace.KeySpace,
) []CoordinateKey {
	if ks == nil || len(equalities) == 0 {
		return nil
	}
	var out []CoordinateKey
	for _, source := range coordinates {
		if source.kind != coordinateRefinement {
			continue
		}
		for _, equality := range equalities {
			for _, direction := range [][2]keyspace.Key{
				{equality.Left, equality.Right},
				{equality.Right, equality.Left},
			} {
				rebased, ok := rebaseCoordinateRefinementAcrossEquality(ks, source.path, direction[0], direction[1])
				if ok {
					out = appendCoordinateDependencyKey(out, RefinementCoordinate(rebased))
				}
			}
		}
	}
	return out
}

func rebaseCoordinateRefinementAcrossEquality(
	ks *keyspace.KeySpace,
	path, from, to keyspace.Key,
) (keyspace.Key, bool) {
	if ks == nil || !ks.HasPrefix(path, from) {
		return keyspace.Key{}, false
	}
	rebased, ok := ks.Rebase(path, from, to)
	if !ok || rebased == path || !ks.HasPrefix(rebased, to) {
		return keyspace.Key{}, false
	}
	return rebased, true
}

func coordinateDependencyTransientEqualityClosure(coordinates []CoordinateKey, equalities []CoordinateDependencyEquality, ks *keyspace.KeySpace) []CoordinateKey {
	if len(equalities) == 0 {
		return nil
	}
	persistentEqualities := make([]BranchProof, 0)
	data := make([]BranchProof, 0)
	for _, coordinate := range coordinates {
		if coordinate.kind != coordinateBranchProof {
			continue
		}
		switch coordinate.proof.Kind {
		case BranchProofPathEqual:
			persistentEqualities = append(persistentEqualities, coordinate.proof)
		case BranchProofPathPresence, BranchProofIndexInRange:
			data = append(data, coordinate.proof)
		}
	}
	transient := make([]BranchProof, 0, len(equalities))
	for _, equality := range equalities {
		transient = append(transient, BranchProof{Kind: BranchProofPathEqual, Path: equality.Left, Other: equality.Right})
	}
	out := make([]CoordinateKey, 0)
	for _, source := range data {
		baselineProofs := append(append([]BranchProof(nil), persistentEqualities...), source)
		baseline := CloseBranchProofsAcrossKnownEqualities(ks, baselineProofs)
		baselineSet := make(map[BranchProof]struct{}, len(baseline))
		for _, proof := range baseline {
			baselineSet[proof] = struct{}{}
		}
		extendedProofs := append(append([]BranchProof(nil), persistentEqualities...), transient...)
		extendedProofs = append(extendedProofs, source)
		for _, proof := range CloseBranchProofsAcrossKnownEqualities(ks, extendedProofs) {
			if proof.Kind != BranchProofPathPresence && proof.Kind != BranchProofIndexInRange {
				continue
			}
			if _, already := baselineSet[proof]; !already {
				out = appendCoordinateDependencyKey(out, BranchProofCoordinate(proof))
			}
		}
	}
	return out
}

// coordinateDependencyPublicationClosure returns every proof scalar one
// operation may publish through the canonical equality closure. Coordinate
// identities represent optional union members, so a newly published equality
// may consume any present data proof while a new data proof propagates through
// every present equality. The pure family closure supplies the shared math.
func coordinateDependencyPublicationClosure(coordinates, publications []CoordinateKey, ks *keyspace.KeySpace) []CoordinateKey {
	if len(publications) == 0 {
		return nil
	}
	equalities := make([]BranchProof, 0)
	data := make([]BranchProof, 0)
	publicationData := make(map[BranchProof]struct{})
	addedEquality := false
	for _, coordinate := range coordinates {
		if coordinate.kind != coordinateBranchProof {
			continue
		}
		if coordinate.proof.Kind == BranchProofPathEqual {
			equalities = append(equalities, coordinate.proof)
		} else if coordinate.proof.Kind == BranchProofPathPresence || coordinate.proof.Kind == BranchProofIndexInRange {
			data = append(data, coordinate.proof)
		}
	}
	for _, coordinate := range publications {
		if coordinate.kind != coordinateBranchProof {
			continue
		}
		switch coordinate.proof.Kind {
		case BranchProofPathEqual:
			addedEquality = true
			equalities = append(equalities, coordinate.proof)
		case BranchProofPathPresence, BranchProofIndexInRange:
			publicationData[coordinate.proof] = struct{}{}
		}
	}
	seeds := make([]BranchProof, 0, len(equalities)+len(data)+len(publicationData))
	seeds = append(seeds, equalities...)
	if addedEquality {
		seeds = append(seeds, data...)
	}
	for proof := range publicationData {
		seeds = append(seeds, proof)
	}
	closed := CloseBranchProofsAcrossKnownEqualities(ks, seeds)
	out := make([]CoordinateKey, 0, len(closed))
	for _, proof := range closed {
		if proof.Kind == BranchProofPathPresence || proof.Kind == BranchProofIndexInRange {
			out = appendCoordinateDependencyKey(out, BranchProofCoordinate(proof))
		}
	}
	return out
}

func coordinateDependencySeedValid(ks *keyspace.KeySpace, seed CoordinateDependencySeed) bool {
	for _, root := range seed.StableRootMutations {
		if root == 0 {
			return false
		}
	}
	for _, paths := range [][]keyspace.Key{seed.ReadPaths, seed.ResolvePaths, seed.WritePaths, seed.DescendantMutationRoots, seed.SubtreeMutationRoots} {
		for _, path := range paths {
			if path.Kind == keyspace.KindInvalid {
				return false
			}
			if _, ok := ks.SegmentsView(path); !ok {
				return false
			}
		}
	}
	for _, equality := range seed.TransientEqualities {
		if equality.Left.Kind == keyspace.KindInvalid || equality.Right.Kind == keyspace.KindInvalid || equality.Left == equality.Right {
			return false
		}
		if _, ok := ks.SegmentsView(equality.Left); !ok {
			return false
		}
		if _, ok := ks.SegmentsView(equality.Right); !ok {
			return false
		}
	}
	return true
}

func coordinateDependencyMayLane(reg *axis.Registry, ks *keyspace.KeySpace, coordinates []CoordinateKey) (Lane, bool) {
	lane := Lane{}
	proofs := make([]BranchProof, 0, len(coordinates))
	implications := make([]PathPresenceImplication, 0, len(coordinates))
	for _, coordinate := range coordinates {
		if !CoordinateKeyValid(coordinate, ks, reg) {
			return Lane{}, false
		}
		switch coordinate.kind {
		case coordinateRefinement:
			lane, _ = lane.WritePathKey(reg, coordinate.path, product.Top())
		case coordinateStaticMember:
			lane, _ = lane.WritePathStaticMember(coordinate.path, product.Top())
		case coordinateBranchProof:
			proofs = append(proofs, coordinate.proof)
		case coordinatePathPresenceImplication:
			implications = append(implications, coordinate.implication)
		default:
			return Lane{}, false
		}
	}
	lane, _ = lane.AddBranchProofs(proofs)
	lane, _ = lane.AddPathPresenceImplications(implications)
	return lane, true
}

func coordinateDependencyAliases(lane Lane, ks *keyspace.KeySpace, path keyspace.Key) []keyspace.Key {
	out := []keyspace.Key{path}
	for _, alias := range lane.EquivalentKeyspaceKeys(ks, path) {
		out = appendUniqueDependencyPath(out, alias)
	}
	sort.Slice(out, func(i, j int) bool { return ks.Less(out[i], out[j]) })
	return out
}

func coordinateDependencyClasses(congruence *pathCongruence, paths []keyspace.Key) map[int]struct{} {
	out := make(map[int]struct{})
	for _, path := range paths {
		root := pathCongruenceRoot{Kind: path.Kind, Sym: uint64(path.Sym), Ver: path.Ver, Root: path.Root}
		term, ok := congruence.roots[root]
		if !ok {
			continue
		}
		class := congruence.find(term)
		out[class] = struct{}{}
		segments, ok := congruence.ks.SegmentsView(path)
		if !ok {
			continue
		}
		for _, part := range segments {
			next, found := congruence.classes[pathCongruenceChild{parent: class, constructor: pathConstructor(path.Kind, part)}]
			if !found {
				break
			}
			class = next
			out[class] = struct{}{}
		}
	}
	return out
}

func coordinateDependencyObservesAnyAlias(congruence *pathCongruence, coordinate CoordinateKey, aliases []keyspace.Key) bool {
	observes := false
	visitCoordinateDependencyPaths(coordinate, func(path keyspace.Key) {
		if observes {
			return
		}
		left, leftOK := congruence.normal(path)
		if !leftOK {
			return
		}
		for _, alias := range aliases {
			right, rightOK := congruence.normal(alias)
			if rightOK && pathCongruenceNormalsEqual(left, right) {
				observes = true
				return
			}
		}
	})
	return observes
}

func visitCoordinateDependencyPaths(coordinate CoordinateKey, visit func(keyspace.Key)) {
	switch coordinate.kind {
	case coordinateRefinement, coordinateStaticMember:
		visit(coordinate.path)
	case coordinateBranchProof:
		visit(coordinate.proof.Path)
		if coordinate.proof.Other.Kind != keyspace.KindInvalid {
			visit(coordinate.proof.Other)
		}
	case coordinatePathPresenceImplication:
		visit(coordinate.implication.Trigger)
		if coordinate.implication.TriggerOther.Kind != keyspace.KindInvalid {
			visit(coordinate.implication.TriggerOther)
		}
		visit(coordinate.implication.Target)
	}
}

func coordinateDependencyLaneHas(lane Lane, coordinate CoordinateKey) bool {
	switch coordinate.kind {
	case coordinateRefinement:
		_, ok := lane.refinements[coordinate.path.Handle()]
		return ok
	case coordinateStaticMember:
		_, ok := lane.staticMembers[coordinate.path]
		return ok
	case coordinateBranchProof:
		_, ok := lane.proofs[coordinate.proof]
		return ok
	case coordinatePathPresenceImplication:
		return lane.HasPathPresenceImplication(coordinate.implication)
	default:
		return false
	}
}

func coordinateDependencyLocation(keys *keyspace.KeySpace, path keyspace.Key) CoordinateDependencyLocation {
	if path.Segs == 0 {
		if dependency, ok := PathValueDependency(keys, path); ok {
			return CoordinateDependencyLocation{Root: dependency}
		}
	}
	return CoordinateDependencyLocation{Path: path}
}

func canonicalCoordinateDependencyKeys(in []CoordinateKey, ks *keyspace.KeySpace) []CoordinateKey {
	out := make([]CoordinateKey, 0, len(in))
	for _, key := range in {
		out = appendCoordinateDependencyKey(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return CoordinateKeyLess(out[i], out[j], ks) })
	return out
}

func canonicalizeCoordinateDependency(value *CoordinateDependency, ks *keyspace.KeySpace) {
	value.CoordinateReads = canonicalCoordinateDependencyKeys(value.CoordinateReads, ks)
	value.CoordinateWrites = canonicalCoordinateDependencyKeys(value.CoordinateWrites, ks)
	sort.Slice(value.LocationReads, func(i, j int) bool {
		return coordinateDependencyLocationLess(value.LocationReads[i], value.LocationReads[j], ks)
	})
	sort.Slice(value.LocationWrites, func(i, j int) bool {
		return coordinateDependencyLocationLess(value.LocationWrites[i], value.LocationWrites[j], ks)
	})
	sort.Slice(value.MutationRegions, func(i, j int) bool {
		return coordinateDependencyLocationLess(value.MutationRegions[i], value.MutationRegions[j], ks)
	})
	sort.Slice(value.mutations, func(i, j int) bool {
		if value.mutations[i].path != value.mutations[j].path {
			return ks.Less(value.mutations[i].path, value.mutations[j].path)
		}
		return !value.mutations[i].includePrefix && value.mutations[j].includePrefix
	})
}

func coordinateDependencyAffects(writer, reader CoordinateDependency, ks *keyspace.KeySpace) bool {
	for _, write := range writer.CoordinateWrites {
		if containsCoordinateDependencyKey(reader.CoordinateReads, write) ||
			containsCoordinateDependencyKey(reader.CoordinateWrites, write) {
			return true
		}
	}
	for _, write := range writer.LocationWrites {
		for _, read := range reader.LocationReads {
			if coordinateDependencyLocationsEqual(write, read, ks) {
				return true
			}
		}
		for _, otherWrite := range reader.LocationWrites {
			if coordinateDependencyLocationsEqual(write, otherWrite, ks) {
				return true
			}
		}
		for _, mutation := range reader.mutations {
			if coordinateDependencyMutationContains(mutation, write, ks) {
				return true
			}
		}
	}
	for _, mutation := range writer.mutations {
		for _, read := range reader.LocationReads {
			if coordinateDependencyMutationContains(mutation, read, ks) {
				return true
			}
		}
		for _, write := range reader.LocationWrites {
			if coordinateDependencyMutationContains(mutation, write, ks) {
				return true
			}
		}
		for _, other := range reader.mutations {
			if coordinateDependencyMutationsOverlap(mutation, other, ks) {
				return true
			}
		}
	}
	return false
}

func coordinateDependencyLocationsEqual(left, right CoordinateDependencyLocation, ks *keyspace.KeySpace) bool {
	if left.IsRoot() || right.IsRoot() {
		return left.IsRoot() && right.IsRoot() && left.Root == right.Root
	}
	return coordinateDependencyPathsEqual(left.Path, right.Path, ks)
}

func coordinateDependencyMutationContains(mutation coordinateDependencyMutation, candidate CoordinateDependencyLocation, ks *keyspace.KeySpace) bool {
	if candidate.IsRoot() {
		root, ok := PathValueDependency(ks, mutation.path)
		segments, valid := ks.SegmentsView(mutation.path)
		return ok && valid && len(segments) == 0 && root == candidate.Root && mutation.includePrefix
	}
	if !coordinateDependencyPathHasPrefix(candidate.Path, mutation.path, ks) {
		return false
	}
	return mutation.includePrefix || !coordinateDependencyPathsEqual(candidate.Path, mutation.path, ks)
}

func coordinateDependencyMutationsOverlap(left, right coordinateDependencyMutation, ks *keyspace.KeySpace) bool {
	// Structural path universes have descendants at every valid prefix. Equal
	// strict-descendant regions therefore overlap even though neither contains
	// its own prefix. Unequal regions overlap exactly when one prefix contains
	// the other under the canonical KeySpace/root-normalized prefix law.
	return coordinateDependencyPathHasPrefix(left.path, right.path, ks) ||
		coordinateDependencyPathHasPrefix(right.path, left.path, ks)
}

func coordinateDependencyPathsEqual(left, right keyspace.Key, ks *keyspace.KeySpace) bool {
	return coordinateDependencyPathHasPrefix(left, right, ks) && coordinateDependencyPathHasPrefix(right, left, ks)
}

func coordinateDependencyPathHasPrefix(candidate, prefix keyspace.Key, ks *keyspace.KeySpace) bool {
	if ks.HasPrefix(candidate, prefix) {
		return true
	}
	leftRoot, leftOK := coordinateDependencySemanticRoot(candidate)
	rightRoot, rightOK := coordinateDependencySemanticRoot(prefix)
	if !leftOK || !rightOK || leftRoot != rightRoot {
		return false
	}
	left, leftValid := ks.SegmentsView(candidate)
	right, rightValid := ks.SegmentsView(prefix)
	if !leftValid || !rightValid || len(right) > len(left) {
		return false
	}
	for index := range right {
		if !coordinateDependencySegmentsEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func coordinateDependencySemanticRoot(path keyspace.Key) (statekey.Value, bool) {
	switch path.Kind {
	case keyspace.KindResolverSym, keyspace.KindUnversionedSym, keyspace.KindStableSym:
		return statekey.SymbolValue(path.Sym), true
	case keyspace.KindRetSlot:
		return statekey.ReturnSlot(int(path.Root)), true
	default:
		return 0, false
	}
}

func coordinateDependencySegmentsEqual(left, right segment.Segment) bool {
	if left == right {
		return true
	}
	return (left.Kind == segment.SegmentField || left.Kind == segment.SegmentIndexString) &&
		(right.Kind == segment.SegmentField || right.Kind == segment.SegmentIndexString) &&
		left.Name == right.Name
}

func coordinateDependencyLocationLess(left, right CoordinateDependencyLocation, ks *keyspace.KeySpace) bool {
	if left.IsRoot() != right.IsRoot() {
		return left.IsRoot()
	}
	if left.IsRoot() {
		leftConcrete, lc := left.Root.Concrete()
		rightConcrete, rc := right.Root.Concrete()
		if lc || rc {
			return lc && (!rc || leftConcrete < rightConcrete)
		}
		leftFormal, lf := left.Root.Formal()
		rightFormal, rf := right.Root.Formal()
		if !lf || !rf {
			return lf
		}
		leftOwner, rightOwner := leftFormal.Owner(), rightFormal.Owner()
		if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
			return compared < 0
		}
		if leftFormal.Vocabulary() != rightFormal.Vocabulary() {
			return leftFormal.Vocabulary() < rightFormal.Vocabulary()
		}
		return leftFormal.Ordinal() < rightFormal.Ordinal()
	}
	return ks.Less(left.Path, right.Path)
}

func appendCoordinateDependencyKey(out []CoordinateKey, value CoordinateKey) []CoordinateKey {
	if containsCoordinateDependencyKey(out, value) {
		return out
	}
	return append(out, value)
}

func containsCoordinateDependencyKey(in []CoordinateKey, value CoordinateKey) bool {
	for _, prior := range in {
		if prior == value {
			return true
		}
	}
	return false
}

func appendCoordinateDependencyLocation(out []CoordinateDependencyLocation, value CoordinateDependencyLocation) []CoordinateDependencyLocation {
	if containsCoordinateDependencyLocation(out, value) {
		return out
	}
	return append(out, value)
}

func containsCoordinateDependencyLocation(in []CoordinateDependencyLocation, value CoordinateDependencyLocation) bool {
	for _, prior := range in {
		if prior == value {
			return true
		}
	}
	return false
}

func appendUniqueDependencyPath(out []keyspace.Key, value keyspace.Key) []keyspace.Key {
	for _, prior := range out {
		if prior == value {
			return out
		}
	}
	return append(out, value)
}

func appendCoordinateDependencyMutation(out []coordinateDependencyMutation, value coordinateDependencyMutation) []coordinateDependencyMutation {
	for index, prior := range out {
		if prior.path != value.path {
			continue
		}
		if value.includePrefix && !prior.includePrefix {
			out[index].includePrefix = true
		}
		return out
	}
	return append(out, value)
}

func cloneCoordinateDependency(value CoordinateDependency) CoordinateDependency {
	value.CoordinateReads = append([]CoordinateKey(nil), value.CoordinateReads...)
	value.CoordinateWrites = append([]CoordinateKey(nil), value.CoordinateWrites...)
	value.LocationReads = append([]CoordinateDependencyLocation(nil), value.LocationReads...)
	value.LocationWrites = append([]CoordinateDependencyLocation(nil), value.LocationWrites...)
	value.MutationRegions = append([]CoordinateDependencyLocation(nil), value.MutationRegions...)
	value.mutations = append([]coordinateDependencyMutation(nil), value.mutations...)
	return value
}
