package flow

import (
	"slices"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

type dependencyKeyKind uint8

const (
	dependencyKeyPath dependencyKeyKind = iota + 1
	dependencyKeySym
)

// dependencyKey is the scheduler dependency key. It is deliberately a typed sum:
// path dependencies use canonical PathKey identity, while symbol dependencies
// cover all versions of a symbol when the exact version key is not enough.
type dependencyKey struct {
	kind dependencyKeyKind
	path constraint.PathKey
	sym  cfg.SymbolID
}

func dependencyPathKey(key constraint.PathKey) dependencyKey {
	if key == "" {
		return dependencyKey{}
	}
	return dependencyKey{kind: dependencyKeyPath, path: key}
}

func dependencySym(sym cfg.SymbolID) dependencyKey {
	if sym == 0 {
		return dependencyKey{}
	}
	return dependencyKey{kind: dependencyKeySym, sym: sym}
}

// symbolTypeSource configures how to initialize symbol types in the solution's values map.
//
// This struct encapsulates the common pattern of iterating over a symbol-to-type map,
// resolving canonical keys for each symbol, and storing the types in the solution.
// Different type sources (DeclaredTypes, SiblingTypes, LiteralTypes) have slightly
// different behaviors controlled by the configuration fields.
//
// Fields:
//   - types: The source map from SymbolID to Type to initialize from.
//   - tryDeclPoint: If true and the entry point key is empty, try the symbol's
//     declaration point as declared evidence. Used for local variables that aren't
//     visible at function entry.
//   - skipIfExists: If true, don't overwrite existing entries in the values map.
//     Used to establish priority ordering (DeclaredTypes > SiblingTypes > LiteralTypes).
type symbolTypeSource struct {
	types        map[cfg.SymbolID]typ.Type
	tryDeclPoint bool
	skipIfExists bool
}

// initSymbolTypes initializes the solution's values map from a symbol-to-type mapping.
//
// This method extracts the common initialization pattern used for DeclaredTypes,
// SiblingTypes, and LiteralTypes. For each symbol in the source map:
//
//  1. Resolves the canonical key at the function entry point
//  2. If tryDeclPoint is set and entry key is empty, tries the declaration point
//  3. If skipIfExists is set, skips symbols that already have values
//  4. Stores the type in the values map keyed by the canonical key
//
// Symbols are processed in sorted order to ensure deterministic initialization,
// which is important for reproducible analysis results and debugging.
//
// This method is safe to call with nil or empty types maps.
func (s *Solution) initSymbolTypes(src symbolTypeSource) {
	if len(src.types) == 0 {
		return
	}

	g := s.inputs.Graph
	if g == nil {
		return
	}

	entry := g.Entry()

	// Sort symbols for deterministic processing order
	syms := make([]cfg.SymbolID, 0, len(src.types))
	for sym := range src.types {
		syms = append(syms, sym)
	}
	slices.Sort(syms)

	for _, sym := range syms {
		t := src.types[sym]
		if t == nil {
			continue
		}

		// Try to resolve key at entry point first
		path := constraint.Path{Symbol: sym}
		key := s.pkResolver.KeyAt(entry, path)

		// Fall back to declaration point for local variables
		if key == "" && src.tryDeclPoint {
			if declPoint, ok := g.DeclarationPoint(sym); ok && declPoint != 0 {
				key = s.pkResolver.KeyAt(declPoint, path)
			}
		}

		if key == "" {
			continue
		}

		// Respect priority ordering if skipIfExists is set
		keyStr := string(key)
		if src.skipIfExists {
			if _, exists := s.values[key]; exists {
				continue
			}
		}

		s.setValue(keyStr, t)
	}
}

func (s *Solution) setValue(key string, t typ.Type) {
	if s == nil || key == "" {
		return
	}
	// Admission boundary: a typ.Type fact enters the abstract-state carrier as a
	// product.AbstractValue. Bookkeeping uses the caller's typ.Type directly.
	s.storeValue(key, liftFlowValue(t), t)
}

// setValueAV stores an already-built carrier in the stable values store. It is
// the native ingress for the value-domain fixpoint merge: a product.Join/Widen
// result is stored without a project-then-relift round trip, which would mint a
// different interned node and break the product-identity convergence check.
func (s *Solution) setValueAV(key string, av product.AbstractValue) {
	if s == nil || key == "" || av.IsZero() {
		return
	}
	s.storeValue(key, av, projectFlowValue(av))
}

func (s *Solution) storeValue(key string, av product.AbstractValue, t typ.Type) {
	if s.values == nil {
		s.values = make(pathValueMap, 1)
	}
	s.values[constraint.PathKey(key)] = av
	s.setValuePresence(key, t)
	s.indexFieldOverlayValue(key, av)
	s.indexValueSuffix(constraint.PathKey(key))
	s.invalidateQueryCachesForWrite(key)
}

// liftFlowValue is the admission boundary that lifts a typ.Type transfer fact
// into the product.AbstractValue carrier. A nil fact lifts to Bottom so the
// stored handle is always a valid interned value.
func liftFlowValue(t typ.Type) product.AbstractValue {
	if t == nil {
		return product.Bottom()
	}
	return product.FromType(t)
}

func (s *Solution) invalidateQueryCachesForWrite(key string) {
	if s == nil {
		return
	}
	s.bumpStateEpoch()
	s.invalidateReachabilityForWrite(key)
	if s.narrowedTypeCache != nil {
		clear(s.narrowedTypeCache)
	}
	if s.childTypesCache != nil {
		clear(s.childTypesCache)
	}
	if s.fieldOverlayCache != nil {
		clear(s.fieldOverlayCache)
	}
	if s.fieldMergeCache != nil {
		clear(s.fieldMergeCache)
	}
}

func (s *Solution) invalidateReachabilityForWrite(key string) {
	if s == nil || key == "" || len(s.reachabilityCache) == 0 {
		return
	}
	invalidate := func(depKey dependencyKey) {
		for _, point := range s.reachabilityDeps[depKey] {
			delete(s.reachabilityCache, point)
		}
	}
	invalidate(dependencyPathKey(constraint.PathKey(key)))
	if sym := pathkey.KeySymbolUnchecked(constraint.PathKey(key)); sym != 0 {
		invalidate(dependencySym(sym))
	}
}

func (s *Solution) bumpStateEpoch() {
	s.stateEpoch++
	if s.stateEpoch == 0 {
		s.stateEpoch = 1
		if s.reachabilityCache != nil {
			clear(s.reachabilityCache)
		}
	}
}

// dependencyMap tracks which CFG points depend on a canonical path or symbol.
//
// During worklist iteration, when a key's type value changes, all dependent
// points must be re-processed. This map enables efficient lookup of those
// dependencies without re-scanning the entire input on each iteration.
//
// The map is built once before the worklist loop and consulted whenever
// a key's value changes. Values are slices of CFG points that read from that
// key. Path dependencies are canonical PathKeys (e.g., "sym1@1.field");
// symbol dependencies are explicit SymbolID keys, not string sentinels.
type dependencyMap map[dependencyKey][]cfg.Point

type reachabilityPointDep struct {
	key   dependencyKey
	point cfg.Point
}

// register adds a dependency from a canonical key to a CFG point.
//
// If the key is empty, the registration is silently ignored. This allows
// callers to pass potentially-empty keys without checking first.
//
// The same point may be registered multiple times for different keys,
// and the same (key, point) pair may be registered multiple times
// (duplicates are handled during worklist iteration via the inQueue map).
func (dm dependencyMap) register(key constraint.PathKey, point cfg.Point) {
	depKey := dependencyPathKey(key)
	if depKey != (dependencyKey{}) {
		dm[depKey] = append(dm[depKey], point)
	}
}

func (dm dependencyMap) registerSymbol(sym cfg.SymbolID, point cfg.Point) {
	depKey := dependencySym(sym)
	if depKey == (dependencyKey{}) {
		return
	}
	dm[depKey] = append(dm[depKey], point)
}

// buildPhiDependencies constructs a map from version keys to phi points.
//
// Phi nodes merge values from multiple control flow paths. When any operand's
// value changes, the phi must be re-evaluated to compute the new merged type.
// This method builds the reverse mapping: for each operand version key, which
// phi points depend on it.
//
// Example: If phi at point 5 merges x@1 and x@2, this returns:
//
//	{"sym(x)@1": [5], "sym(x)@2": [5]}
//
// Returns nil if the graph is nil.
func (s *Solution) buildPhiDependencies() dependencyMap {
	g := s.inputs.Graph
	if g == nil {
		return nil
	}

	deps := make(dependencyMap)
	for _, phi := range g.PhiNodes() {
		for _, op := range phi.Operands {
			opKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
			deps.register(opKey, phi.Point)
			deps.registerSymbol(op.Version.Symbol, phi.Point)
		}
	}
	return deps
}

// buildAssignmentDependencies constructs a map from source keys to assignment points.
//
// When the type of a source path changes, assignments that read from that source
// must be re-processed. This includes:
//   - Regular assignments with a source path (e.g., y = x)
//   - Iterator source paths (e.g., for k, v in pairs(t))
//   - Table mutator value paths (e.g., table.insert(t, value))
//   - Map element sources (e.g., v = t[k] with dynamic key)
//   - Container element sources (e.g., v = ch:receive())
//   - Length-index sources (e.g., v = t[#t])
//
// This enables incremental re-evaluation during worklist iteration.
func (s *Solution) buildAssignmentDependencies() dependencyMap {
	deps := make(dependencyMap)

	// Regular assignments
	for _, assign := range s.inputs.Assignments {
		switch assign.Source.Kind {
		case AssignmentSourcePath:
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.Path)
			deps.register(srcKey, assign.Point)
		case AssignmentSourceIterator:
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.Path)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.Source.Path.Symbol, assign.Point)
		case AssignmentSourceMapElement:
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.MapPath)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.Source.MapPath.Symbol, assign.Point)
			deps.registerSymbol(assign.Source.KeySymbol, assign.Point)
		case AssignmentSourceContainerElement:
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.ContainerPath)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.Source.ContainerPath.Symbol, assign.Point)
		case AssignmentSourceLengthIndex:
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.ContainerPath)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.Source.ContainerPath.Symbol, assign.Point)
		case AssignmentSourceCallReturn:
			if assign.Source.ReceiverPath.Symbol != 0 {
				srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.ReceiverPath)
				deps.register(srcKey, assign.Point)
				deps.registerSymbol(assign.Source.ReceiverPath.Symbol, assign.Point)
			}
			if assign.Source.CalleePath.Symbol != 0 {
				srcKey := s.pkResolver.KeyAt(assign.Point, assign.Source.CalleePath)
				deps.register(srcKey, assign.Point)
				deps.registerSymbol(assign.Source.CalleePath.Symbol, assign.Point)
			}
		}
	}

	// Mutator value paths and embedded value-template source slots.
	for _, mm := range s.inputs.MapMutatorAssignments {
		if mm.ValuePath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(mm.Point, mm.ValuePath)
			deps.register(srcKey, mm.Point)
		}
		s.registerValueTemplateDependencies(deps, mm.Point, mm.Value)
	}
	for _, tm := range s.inputs.TableMutatorAssignments {
		if tm.ValuePath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(tm.Point, tm.ValuePath)
			deps.register(srcKey, tm.Point)
		}
		s.registerValueTemplateDependencies(deps, tm.Point, tm.Value)
	}
	return deps
}

func (s *Solution) registerValueTemplateDependencies(deps dependencyMap, point cfg.Point, template ValueTemplate) {
	if len(template.Slots) == 0 {
		return
	}
	for _, slot := range template.Slots {
		s.registerAssignmentSourceDependencies(deps, point, slot.Source)
	}
}

func (s *Solution) registerAssignmentSourceDependencies(deps dependencyMap, point cfg.Point, source AssignmentSource) {
	switch source.Kind {
	case AssignmentSourcePath:
		srcKey := s.pkResolver.KeyAt(point, source.Path)
		deps.register(srcKey, point)
		deps.registerSymbol(source.Path.Symbol, point)
	case AssignmentSourceIterator:
		srcKey := s.pkResolver.KeyAt(point, source.Path)
		deps.register(srcKey, point)
		deps.registerSymbol(source.Path.Symbol, point)
	case AssignmentSourceMapElement:
		srcKey := s.pkResolver.KeyAt(point, source.MapPath)
		deps.register(srcKey, point)
		deps.registerSymbol(source.MapPath.Symbol, point)
		deps.registerSymbol(source.KeySymbol, point)
	case AssignmentSourceContainerElement:
		srcKey := s.pkResolver.KeyAt(point, source.ContainerPath)
		deps.register(srcKey, point)
		deps.registerSymbol(source.ContainerPath.Symbol, point)
	case AssignmentSourceLengthIndex:
		srcKey := s.pkResolver.KeyAt(point, source.ContainerPath)
		deps.register(srcKey, point)
		deps.registerSymbol(source.ContainerPath.Symbol, point)
	case AssignmentSourceCallReturn:
		if source.ReceiverPath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(point, source.ReceiverPath)
			deps.register(srcKey, point)
			deps.registerSymbol(source.ReceiverPath.Symbol, point)
		}
		if source.CalleePath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(point, source.CalleePath)
			deps.register(srcKey, point)
			deps.registerSymbol(source.CalleePath.Symbol, point)
		}
	}
}

// buildEdgeConditionDependencies constructs a map from keys to edge condition source points.
//
// Edge conditions contain type constraints that depend on variable types at the
// source point. When a variable's type changes, edge conditions that reference
// it may produce different narrowing results.
//
// This method extracts all path keys referenced in edge conditions and maps them
// to the source points. A deduplication set prevents the same (key, point) pair
// from being registered multiple times, which would cause redundant work.
func (s *Solution) buildEdgeConditionDependencies() dependencyMap {
	deps := make(dependencyMap)

	// Deduplication set for (key, point) pairs
	type ecDep struct {
		key   string
		point cfg.Point
	}
	seen := make(map[ecDep]bool)

	for _, ec := range s.inputs.EdgeConditions {
		// Iterate through all disjuncts and their constraints
		for i := 0; i < ec.Condition.NumDisjuncts(); i++ {
			for _, c := range ec.Condition.DisjunctConstraints(i) {
				for _, path := range c.Paths() {
					if path.Symbol == 0 {
						continue
					}
					verKey := s.pkResolver.KeyAt(ec.From, path)
					if verKey == "" {
						continue
					}
					dep := ecDep{key: string(verKey), point: ec.From}
					if seen[dep] {
						continue
					}
					seen[dep] = true
					deps.register(verKey, ec.From)
				}
			}
		}
	}

	return deps
}

func (s *Solution) buildReachabilityDependencies() dependencyMap {
	deps := make(dependencyMap)
	if s == nil || s.pkResolver == nil {
		return deps
	}

	seen := make(map[reachabilityPointDep]bool)
	for point, cond := range s.pointConditions {
		if !cond.HasConstraints() {
			continue
		}
		for i := 0; i < cond.NumDisjuncts(); i++ {
			for _, c := range cond.DisjunctConstraints(i) {
				for _, path := range c.Paths() {
					if path.Symbol == 0 {
						continue
					}
					key := s.pkResolver.KeyAt(point, path)
					if key == "" {
						continue
					}
					dep := reachabilityPointDep{key: dependencyPathKey(key), point: point}
					if seen[dep] {
						continue
					}
					seen[dep] = true
					deps.register(key, point)
					s.registerReachabilityAncestorDeps(deps, seen, point, path)
				}
			}
		}
	}
	s.reachabilityDepsSeen = seen
	return deps
}

// registerPointNumericShapeReachabilityDeps registers the reachability
// dependencies induced by point p's converged numeric length bounds. A length
// lower-bound on an array key makes p's shape-reachability depend on that array's
// value type, so the value worklist must re-evaluate p when the array key (or its
// ancestors) changes. It is called from the unified worklist's numeric transfer
// as numeric states materialize, since the numeric component is computed inside
// solve() rather than in a prior pass.
func (s *Solution) registerPointNumericShapeReachabilityDeps(point cfg.Point) {
	if s == nil || s.reachabilityDeps == nil {
		return
	}
	state := s.numericAt[point]
	if state == nil {
		return
	}
	seen := s.reachabilityDepsSeen
	if seen == nil {
		seen = make(map[reachabilityPointDep]bool)
		s.reachabilityDepsSeen = seen
	}
	state.ForEachLenBound(func(key constraint.PathKey, lower, _ int64) bool {
		if lower <= 0 || key == "" {
			return true
		}
		dep := reachabilityPointDep{key: dependencyPathKey(key), point: point}
		if !seen[dep] {
			seen[dep] = true
			s.reachabilityDeps.register(key, point)
		}
		if path, ok := s.pathFromCanonicalKeyAtPoint(point, key); ok {
			s.registerReachabilityAncestorDeps(s.reachabilityDeps, seen, point, path)
		}
		return true
	})
}

func (s *Solution) registerReachabilityAncestorDeps(
	deps dependencyMap,
	seen map[reachabilityPointDep]bool,
	point cfg.Point,
	path constraint.Path,
) {
	if len(path.Segments) == 0 {
		return
	}
	for cut := len(path.Segments) - 1; cut >= 0; cut-- {
		ancestor := path
		if cut == 0 {
			ancestor.Segments = nil
		} else {
			ancestor.Segments = path.Segments[:cut]
		}
		key := s.pkResolver.KeyAt(point, ancestor)
		if key == "" {
			continue
		}
		dep := reachabilityPointDep{key: dependencyPathKey(key), point: point}
		if seen[dep] {
			continue
		}
		seen[dep] = true
		deps.register(key, point)
	}
}

// addDependentPoints adds CFG points that depend on changed keys to the worklist.
//
// This function is the core of incremental worklist propagation. When a key's
// value changes, all points that depend on that key (via phi nodes, assignments,
// or edge conditions) must be added to the worklist for re-processing.
//
// The inQueue map prevents duplicate additions - if a point is already queued,
// it's skipped. This is critical for termination and efficiency.
//
// Parameters:
//   - deps: The dependency map to consult
//   - changedKeys: Keys whose values changed in the current iteration
//   - worklist: The current worklist to append to
//   - inQueue: Set of points already in the worklist
//
// Returns the updated worklist (may be the same slice if no growth occurred).
func addDependentPoints(deps dependencyMap, changedKeys []constraint.PathKey, worklist []cfg.Point, inQueue map[cfg.Point]bool) []cfg.Point {
	if len(changedKeys) == 0 {
		return worklist
	}
	keys := append([]constraint.PathKey(nil), changedKeys...)
	sortPathKeys(keys)

	pending := make(map[cfg.Point]bool)
	points := make([]cfg.Point, 0)
	for _, key := range keys {
		pathDep := dependencyPathKey(key)
		for _, point := range deps[pathDep] {
			if inQueue[point] || pending[point] {
				continue
			}
			pending[point] = true
			points = append(points, point)
		}
		if sym := pathkey.KeySymbolUnchecked(key); sym != 0 {
			for _, point := range deps[dependencySym(sym)] {
				if inQueue[point] || pending[point] {
					continue
				}
				pending[point] = true
				points = append(points, point)
			}
		}
	}
	slices.Sort(points)
	for _, point := range points {
		worklist = append(worklist, point)
		inQueue[point] = true
	}
	return worklist
}
