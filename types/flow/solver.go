package flow

import (
	"slices"
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Solution holds flow-narrowed types for all paths after solving.
//
// Solution is the result of running flow analysis on a function's CFG. It maps
// versioned path keys to their narrowed types at each program point, taking
// into account control flow constraints, assignments, and phi merges.
//
// The solver uses a worklist algorithm that propagates types through the CFG
// until reaching a fixed point. Edge conditions from conditionals narrow types
// along specific branches, while phi nodes merge types from different paths.
type Solution struct {
	inputs     *Inputs
	resolver   narrow.Resolver
	pkResolver *pathkey.Resolver
	// values, mutableIn, and mutableOut are the abstract-state carriers of the
	// worklist fixpoint. They hold product.AbstractValue so convergence/no-op
	// detection compares product identity (product.Equal) - the interner collapses
	// converged recursive families to one canonical node, so the worklist drains
	// where a typ.Type carrier with SameConvergedFact does not. typ.Type enters only
	// at admission (FromType in setValue/setMutableValue) and leaves only at egress
	// (ProjectValue in valueAtPoint/projectedValueAtPoint and the query API).
	values            map[string]product.AbstractValue // canonical key (sym<ID>@<ver><segs>) -> abstract value
	presence          map[string]pathPresence
	fieldOverlayIndex map[string]map[string]typ.Type
	// mutableIn is the joined predecessor state for each point (the Kildall IN).
	// mutableOut is the committed post-state each successor joins and every query
	// reads (the Kildall OUT). During a point's own transfer, mutableOut[p] holds
	// the working store the transfers read and write; the commit widens it against
	// the prior OUT so the iterate is monotone and bounded.
	mutableIn       map[cfg.Point]map[string]product.AbstractValue
	mutableOut      map[cfg.Point]map[string]product.AbstractValue
	mutablePresence map[cfg.Point]map[string]pathPresence
	edgeConditions  map[edgeKey]constraint.Condition
	declaredSyms    []cfg.SymbolID

	phisByPoint                  [][]cfg.PhiNode
	assignmentsByPoint           [][]UnifiedAssignment
	mapMutatorAssignmentsByPoint [][]MapMutatorAssignment
	tableMutatorByPoint          [][]TableMutatorAssignment
	containerMutatorByPoint      [][]ContainerMutatorAssignment
	arrayLiteralLengthByPoint    [][]ArrayLiteralLength
	loopInsertLengthByPoint      [][]LoopInsertLength
	edgeNumericConstraints       map[edgeKey][]constraint.NumericConstraint
	unsatEdges                   map[edgeKey]bool // edges proven unreachable by constraints
	pointConditions              map[cfg.Point]constraint.Condition
	reachabilityDeps             dependencyMap
	reachabilityDepsSeen         map[reachabilityPointDep]bool
	// numericAt is the relational numeric component of each point's worklist
	// state. It is a first-class slot of the unified value worklist fixpoint:
	// solve() computes it in the same iteration that processes the value env,
	// joining/widening predecessor contributions, and convergence requires both
	// the value and numeric components to be stable. Top is the absent slot.
	numericAt  map[cfg.Point]*numeric.State
	iterations int

	// Scratch buffers to reduce allocations in hot paths
	scratchTypes           []typ.Type
	scratchSuffix          map[string]struct{}
	scratchVersionIDs      map[cfg.SymbolID]int
	scratchMissingVersions map[cfg.SymbolID]struct{}
	scratchUnresolvedPaths map[constraint.PathKey]struct{}
	scratchValueMap        map[constraint.PathKey]typ.Type
	scratchResolvedPathMap map[constraint.PathKey]constraint.PathKey
	scratchParsedSuffixes  map[string][]constraint.Segment
	fieldOverlayCache      map[fieldOverlayCacheKey][]mergedField
	fieldMergeCache        map[fieldMergeCacheKey]typ.Type
	valueSuffixIndex       map[string][]string
	mutableSuffixIndex     map[pointRootKey][]string
	mutableSuffixIndexed   map[cfg.Point]bool
	phiJoinState           map[phiJoinKey]phiJoinValue
	pathAliases            map[string]string // canonical target path key -> canonical source path key
	narrowedTypeCache      map[narrowedTypeCacheKey]narrowedTypeCacheValue
	childTypesCache        map[childTypesCacheKey]childTypesCacheValue
	pointConditionCache    map[conditionPathCacheKey]constraint.Condition
	reachabilityCache      map[cfg.Point]reachabilityCacheValue
	stateEpoch             uint64
	queryCacheEnabled      bool

	// Worklist/dependency scratch to reduce per-iteration allocations.
	scratchChangedKeys  []string
	scratchPendingMarks []int
	scratchPendingPts   []cfg.Point
	pendingMarkEpoch    int
}

type narrowedTypeCacheKey struct {
	point cfg.Point
	path  constraint.PathKey
}

type narrowedTypeCacheValue struct {
	epoch uint64
	t     typ.Type
	ok    bool
}

type childTypesCacheKey struct {
	point    cfg.Point
	path     constraint.PathKey
	preState bool
}

type childTypesCacheValue struct {
	epoch uint64
	facts []PathFact
}

type conditionPathCacheKey struct {
	point         cfg.Point
	path          constraint.PathKey
	conditionHash uint64
}

type reachabilityCacheValue struct {
	epoch     uint64
	reachable bool
}

// edgeKey identifies a CFG edge for condition and constraint lookups.
type edgeKey struct {
	from cfg.Point
	to   cfg.Point
}

type mergedField struct {
	Name     string
	Type     typ.Type
	Optional bool
}

// Solve computes flow analysis and returns the solution.
//
// Solve is the main entry point for flow-sensitive type analysis. It takes
// the analysis inputs (CFG, declared types, assignments, constraints) and
// a type resolver, then computes narrowed types at each program point.
//
// The algorithm:
//  1. Builds edge conditions from input constraints
//  2. Propagates conditions through the CFG to compute point conditions
//  3. Runs worklist iteration to compute final narrowed types and the numeric
//     component, then marks unreachable edges from the converged numeric state
func Solve(inputs *Inputs, resolver narrow.Resolver) *Solution {
	s, size := newSolution(inputs, resolver)
	s.buildTransferPlan(size)
	s.buildEdgeConditions()
	s.buildEdgeNumericConstraints()

	// Propagate conditions using standalone propagate package
	s.runPropagation()
	s.reachabilityDeps = s.buildReachabilityDependencies()

	s.solve()
	s.queryCacheEnabled = true
	return s
}

// SolveConditionView computes the branch/numeric condition view of a function
// without running assignment or mutable-state transfer. It is the canonical
// pre-assignment analysis surface: queries are backed by declared/literal/
// sibling inputs, propagated path conditions, and numeric edge facts only.
func SolveConditionView(inputs *Inputs, resolver narrow.Resolver) *Solution {
	s, _ := newSolution(conditionViewInputs(inputs), resolver)
	s.buildEdgeConditions()
	s.buildEdgeNumericConstraints()
	s.runNumericWorklist()
	s.runPropagation()
	s.initializeDeclarations()
	s.queryCacheEnabled = true
	return s
}

func conditionViewInputs(inputs *Inputs) *Inputs {
	if inputs == nil {
		return nil
	}
	view := *inputs
	view.Assignments = nil
	view.MapMutatorAssignments = nil
	view.TableMutatorAssignments = nil
	view.ContainerMutatorAssignments = nil
	return &view
}

func newSolution(inputs *Inputs, resolver narrow.Resolver) (*Solution, int) {
	size := 0
	var pkRes *pathkey.Resolver
	if inputs != nil && inputs.Graph != nil {
		inputs.Normalize()
		size = inputs.Graph.Size()
		pkRes = pathkey.NewResolver(inputs.Graph)
	}

	s := &Solution{
		inputs:     inputs,
		resolver:   resolver,
		pkResolver: pkRes,
		values:     make(map[string]product.AbstractValue, estimateSolutionValueCapacity(inputs, size)),
	}
	if inputs != nil && len(inputs.DeclaredTypes) > 0 {
		s.declaredSyms = make([]cfg.SymbolID, 0, len(inputs.DeclaredTypes))
		for sym := range inputs.DeclaredTypes {
			s.declaredSyms = append(s.declaredSyms, sym)
		}
		slices.Sort(s.declaredSyms)
	}
	return s, size
}

func estimateSolutionValueCapacity(inputs *Inputs, graphSize int) int {
	if inputs == nil {
		return 0
	}
	capacity := len(inputs.DeclaredTypes) + len(inputs.Assignments)
	capacity += len(inputs.MapMutatorAssignments) + len(inputs.TableMutatorAssignments)
	capacity += len(inputs.ContainerMutatorAssignments)
	if capacity < len(inputs.ConstValues) {
		capacity += len(inputs.ConstValues)
	}
	if capacity < 8 && graphSize > 0 {
		capacity = 8
	}
	maxByGraph := graphSize * 2
	if maxByGraph > 0 && capacity > maxByGraph {
		return maxByGraph
	}
	return capacity
}

// runPropagation runs constraint propagation using the propagate package.
//
// Converts edge conditions to the propagate package format and runs
// forward propagation to compute conditions at each program point.
func (s *Solution) runPropagation() {
	if s.inputs == nil || s.inputs.Graph == nil {
		return
	}

	// Convert edge conditions to propagate format
	var edgeConds propagate.EdgeConditions
	if len(s.edgeConditions) > 0 {
		edgeConds = make(propagate.EdgeConditions, len(s.edgeConditions))
		for k, cond := range s.edgeConditions {
			edgeConds[propagate.EdgeKey{From: k.from, To: k.to}] = cond
		}
	}

	// Convert assignments to propagate format
	assigns := make([]propagate.Assignment, 0, len(s.inputs.Assignments))
	for _, a := range s.inputs.Assignments {
		if a.TargetPath.Symbol != 0 {
			assigns = append(assigns, propagate.Assignment{
				Point:      a.Point,
				TargetSym:  a.TargetPath.Symbol,
				TargetSegs: a.TargetPath.Segments,
			})
		}
	}

	demand := s.inputs.ConditionDemand
	if demand == nil {
		demand = buildConditionDemand(s.inputs)
	}

	propInputs := &propagate.Inputs{
		Graph:          s.inputs.Graph,
		EdgeConditions: edgeConds,
		DeadPoints:     s.inputs.DeadPoints,
		Assignments:    assigns,
		Demand:         demand,
	}

	result := propagate.Propagate(propInputs)
	s.pointConditions = result.PointConditions
}

// buildPointValueMap creates a type environment for constraint solving at a point.
//
// Builds a map from canonical path keys to their types, including:
//   - The target path with its base type
//   - Declared types for visible symbols
//   - Types for all paths referenced by constraints
//
// This environment is passed to the constraint solver for type narrowing.
func (s *Solution) buildPointValueMap(p cfg.Point, targetPath constraint.Path, baseType typ.Type, constraints []constraint.Constraint) map[constraint.PathKey]typ.Type {
	return newPointValueEnvBuilder(s, p, targetPath, baseType, constraints, pointValueEnvSolved).build()
}

func estimatePointValueMapCapacity(visibleCount, constraintCount int) int {
	capacity := 8
	if visibleCount > 0 {
		capacity += min(visibleCount, 32)
	}
	if constraintCount > 0 {
		capacity += min(constraintCount*2, 32)
	}
	return capacity
}

func estimateVersionCacheCapacity(visibleCount int) int {
	if visibleCount <= 0 {
		return 8
	}
	return min(visibleCount+1, 32)
}

func estimateUnresolvedPathCapacity(constraintCount int) int {
	if constraintCount <= 0 {
		return 8
	}
	return min(constraintCount, 16)
}

// isDescendantOf returns true if child is a strict descendant of parent.
//
// A path is a descendant if it has the same symbol and extends the parent's
// segments. For example, x.foo.bar is a descendant of x.foo but not of x.baz.
func isDescendantOf(child, parent constraint.Path) bool {
	if child.Symbol != parent.Symbol {
		return false
	}
	if len(child.Segments) <= len(parent.Segments) {
		return false
	}
	for i := 0; i < len(parent.Segments); i++ {
		if child.Segments[i] != parent.Segments[i] {
			return false
		}
	}
	return true
}

// constraintEnv creates a constraint environment for narrowing operations.
//
// The environment provides type resolution and field/index access needed
// by the constraint solver to apply narrowing operations.
func (s *Solution) constraintEnv() constraint.Env {
	return constraint.Env{
		ResolveType: s.resolveTypeKey,
		Resolver:    s.resolver,
	}
}

// DebugValueAt returns the raw value stored for a version key.
func (s *Solution) DebugValueAt(key string, p cfg.Point) typ.Type {
	if s == nil {
		return nil
	}
	return s.valueAtPoint(p, key)
}

// DebugVersionedKey returns the canonical key for root at point.
func (s *Solution) DebugVersionedKey(root string, p cfg.Point) string {
	if s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil {
		return ""
	}
	sym, ok := s.inputs.Graph.SymbolAt(p, root)
	if !ok {
		return ""
	}
	path := constraint.Path{Root: root, Symbol: sym}
	return string(s.pkResolver.KeyAt(p, path))
}

// DebugIterations returns worklist iterations used for convergence.
func (s *Solution) DebugIterations() int {
	return s.iterations
}

// DebugVersionValues returns the version values for debugging.
func (s *Solution) DebugVersionValues() map[string]typ.Type {
	out := make(map[string]typ.Type, len(s.values))
	for key, av := range s.values {
		out[key] = av.ProjectValue()
	}
	return out
}

// DebugEdgeValues returns nil (edge values removed in single narrowing system consolidation).
func (s *Solution) DebugEdgeValues(from, to cfg.Point) map[string]typ.Type {
	return nil
}

// DebugEdgeCondition returns the condition stored for a specific edge.
func (s *Solution) DebugEdgeCondition(from, to cfg.Point) constraint.Condition {
	if s == nil {
		return constraint.Condition{}
	}
	return s.edgeConditions[edgeKey{from: from, to: to}]
}

// UnreachableEdges returns all edges proven unreachable by constraint analysis.
//
// An edge is unreachable when its guard condition is unsatisfiable given the
// types flowing into it. This enables dead code detection and exhaustiveness
// checking for control flow.
func (s *Solution) UnreachableEdges() []cfg.Edge {
	if len(s.unsatEdges) == 0 {
		return nil
	}
	edges := make([]cfg.Edge, 0, len(s.unsatEdges))
	for key := range s.unsatEdges {
		edges = append(edges, cfg.Edge{From: key.from, To: key.to})
	}
	slices.SortFunc(edges, func(a, b cfg.Edge) int {
		if a.From != b.From {
			return int(a.From) - int(b.From)
		}
		return int(a.To) - int(b.To)
	})
	return edges
}

// solve runs the worklist algorithm to compute narrowed types.
//
// The algorithm iterates over CFG points in reverse postorder, processing
// assignments, phi nodes, and applying edge conditions. When a type changes,
// dependent points are added back to the worklist until convergence.
func (s *Solution) solve() {
	if s.inputs == nil {
		return
	}
	g := s.inputs.Graph
	if g == nil {
		return
	}

	s.queryCacheEnabled = false
	s.initializeDeclarations()

	// Build dependency maps for worklist propagation
	phiDeps := s.buildPhiDependencies()
	assignDeps := s.buildAssignmentDependencies()
	edgeDeps := s.buildEdgeConditionDependencies()

	// The numeric component is a first-class slot of this worklist's per-point
	// state. It carries its own seeding/widening bookkeeping and is computed in
	// the same iteration as the value env; convergence requires both components
	// to be stable, so a point is rescheduled when either changes.
	numericActive := len(s.edgeNumericConstraints) > 0 || s.hasNumericLengthEffects()
	var numericWS *numericWorklistState
	if numericActive {
		numericWS = newNumericWorklistState()
		if s.numericAt == nil {
			s.numericAt = make(map[cfg.Point]*numeric.State)
		}
	}

	worklist := g.RPO()
	inQueue := make([]bool, g.Size())
	for _, p := range worklist {
		if idx := int(p); idx >= 0 && idx < len(inQueue) {
			inQueue[idx] = true
		}
	}

	for len(worklist) > 0 {
		// FIFO queue: process in forward RPO order for correct dataflow
		p := worklist[0]
		worklist = worklist[1:]
		if idx := int(p); idx >= 0 && idx < len(inQueue) {
			inQueue[idx] = false
		}

		// Compute the numeric component first so the value transfer's
		// reachability and length-shape projection at p observe the current
		// numeric state in the same visit.
		numericChanged := numericActive && s.numericTransferAt(p, numericWS)
		changedKeys := s.processPointReturnChangedKeys(p)
		if len(changedKeys) > 0 || numericChanged {
			// Add direct successors
			for _, succ := range graphSuccessors(g, p) {
				if succIdx := int(succ); succIdx >= 0 && succIdx < len(inQueue) && !inQueue[succIdx] {
					worklist = append(worklist, succ)
					inQueue[succIdx] = true
				}
			}
			// Add dependent points from all dependency maps in one pass.
			worklist = s.addDependentPointsBatch(phiDeps, assignDeps, edgeDeps, changedKeys, worklist, inQueue)
		}

		s.iterations++
	}

	if numericActive {
		s.finalizeUnsatEdges()
	}
	if s.narrowedTypeCache != nil {
		clear(s.narrowedTypeCache)
	}
	if s.childTypesCache != nil {
		clear(s.childTypesCache)
	}
}

func (s *Solution) addDependentPointsBatch(
	phiDeps, assignDeps, edgeDeps dependencyMap,
	changedKeys []string,
	worklist []cfg.Point,
	inQueue []bool,
) []cfg.Point {
	if len(changedKeys) == 0 {
		return worklist
	}

	// Keep deterministic behavior while reusing backing storage.
	keys := s.scratchChangedKeys[:0]
	keys = append(keys, changedKeys...)
	sort.Strings(keys)
	s.scratchChangedKeys = keys

	if len(s.scratchPendingMarks) < len(inQueue) {
		s.scratchPendingMarks = make([]int, len(inQueue))
	}
	s.pendingMarkEpoch++
	if s.pendingMarkEpoch == 0 {
		clear(s.scratchPendingMarks)
		s.pendingMarkEpoch = 1
	}
	epoch := s.pendingMarkEpoch

	pendingPts := s.scratchPendingPts[:0]
	marks := s.scratchPendingMarks

	addPoint := func(point cfg.Point) {
		idx := int(point)
		if idx < 0 || idx >= len(inQueue) || inQueue[idx] || marks[idx] == epoch {
			return
		}
		marks[idx] = epoch
		pendingPts = append(pendingPts, point)
	}
	addByKey := func(dm dependencyMap, key string) {
		for _, point := range dm[key] {
			addPoint(point)
		}
	}
	// A key change can flip the constraint reachability of points whose path
	// condition reads it. Those points feed phi/transfer at their successors
	// (e.g. a now-reachable break edge contributes a back-edge operand to a loop
	// header phi). The reachability cache is cleared on write, but the consuming
	// points must also be re-queued or the worklist drains on the stale narrow
	// fixpoint. Re-queue the reachability dependents and their successors.
	addReachabilityDeps := func(key string) {
		for _, point := range s.reachabilityDeps[key] {
			addPoint(point)
			for _, succ := range graphSuccessors(s.inputs.Graph, point) {
				addPoint(succ)
			}
		}
	}

	for _, key := range keys {
		addByKey(phiDeps, key)
		addByKey(assignDeps, key)
		addByKey(edgeDeps, key)
		addReachabilityDeps(key)

		if sym := pathkey.KeySymbolUnchecked(constraint.PathKey(key)); sym != 0 {
			symKey := symbolDependencyKey(sym)
			addByKey(phiDeps, symKey)
			addByKey(assignDeps, symKey)
			addByKey(edgeDeps, symKey)
			addReachabilityDeps(symKey)
		}
	}

	slices.Sort(pendingPts)
	for _, point := range pendingPts {
		if idx := int(point); idx >= 0 && idx < len(inQueue) {
			worklist = append(worklist, point)
			inQueue[idx] = true
		}
	}
	s.scratchPendingPts = pendingPts

	return worklist
}

// initializeDeclarations seeds the solution with declared types.
//
// Initializes the values map with types from:
//   - DeclaredTypes: parameter and local variable declarations
//   - SiblingTypes: captured variables from parent scopes
//   - LiteralTypes: function literals defined in current scope
func (s *Solution) initializeDeclarations() {
	// DeclaredTypes: seed entry first, then declaration point when needed.
	s.initSymbolTypes(symbolTypeSource{
		types:        s.inputs.DeclaredTypes,
		tryDeclPoint: true,
		skipIfExists: false,
	})

	// SiblingTypes: entry only, skip if already set
	s.initSymbolTypes(symbolTypeSource{
		types:        s.inputs.SiblingTypes,
		tryDeclPoint: false,
		skipIfExists: true,
	})

	// LiteralTypes: seed entry first, then declaration point; keep existing slots.
	s.initSymbolTypes(symbolTypeSource{
		types:        s.inputs.LiteralTypes,
		tryDeclPoint: true,
		skipIfExists: true,
	})
}

// lookupDeclaredType returns the declared type for a symbol path.
//
// Lookup order prioritizes more specific type sources:
//  1. LiteralTypes - function literals with inferred types
//  2. SiblingTypes - captured variables from enclosing scopes
//  3. DeclaredTypes - explicit type annotations
func (s *Solution) lookupDeclaredType(path constraint.Path) typ.Type {
	if s.inputs == nil || path.Symbol == 0 {
		return nil
	}
	annotated := s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[path.Symbol]
	if annotated {
		if t := s.inputs.DeclaredTypes[path.Symbol]; t != nil {
			return t
		}
	}
	// Check literal types first (function literals defined in current scope),
	// but do not override explicit annotations.
	if !annotated {
		if t := s.inputs.LiteralTypes[path.Symbol]; t != nil {
			return t
		}
	}
	// Check sibling types (captured variables from parent scope)
	if t := s.inputs.SiblingTypes[path.Symbol]; t != nil {
		return t
	}
	return s.inputs.DeclaredTypes[path.Symbol]
}

func (s *Solution) resolveTypeKey(key narrow.TypeKey) typ.Type {
	if s.inputs == nil {
		return nil
	}
	switch key.Kind {
	case narrow.TypeKeyHash:
		if s.inputs.TypeKeys == nil {
			return nil
		}
		return s.inputs.TypeKeys[key.Hash]
	case narrow.TypeKeyBuiltin:
		if builtinKind, ok := key.BuiltinKind(); ok {
			return narrow.TypeForKind(builtinKind)
		}
		return nil
	default:
		return nil
	}
}

// mergeFieldAssignments merges sub-path field assignments into the base type.
//
// When assignments target fields of a base path (e.g., x.foo = 1), this method
// merges those field types into the base type. For example, if baseType is
// {count: number} and we have an assignment for baseKey + ".increment" = function,
// returns {count: number, increment: function}.
//
// This enables gradual type construction for tables built incrementally.
func (s *Solution) mergeFieldAssignments(baseType typ.Type, baseKey string) typ.Type {
	return s.mergeFieldAssignmentsAt(0, baseType, baseKey)
}

func (s *Solution) mergeFieldAssignmentsAt(p cfg.Point, baseType typ.Type, baseKey string) typ.Type {
	return newFieldOverlayMerger(s, p, baseType, baseKey).merge()
}

func mergeRecursiveUnionFieldOverlay(u *typ.Union, fields []mergedField) (typ.Type, bool) {
	if u == nil || len(fields) == 0 || !typ.ContainsRecursive(u) {
		return nil, false
	}

	builder := typ.NewRecord().SetOpen(true)
	for _, f := range fields {
		if f.Optional {
			builder.OptField(f.Name, f.Type)
		} else {
			builder.Field(f.Name, f.Type)
		}
	}
	overlay := builder.Build()

	residual := make([]typ.Type, 0, len(u.Members)+1)
	for _, member := range u.Members {
		if recursiveTableOverlayMember(member) {
			continue
		}
		residual = append(residual, member)
	}
	if len(residual) == 0 {
		return overlay, true
	}
	residual = append(residual, overlay)
	return join.Types(residual...), true
}

func recursiveTableOverlayMember(t typ.Type) bool {
	if t == nil || !typ.ContainsRecursive(t) {
		return false
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Recursive:
		switch unwrap.Alias(v.Body).(type) {
		case *typ.Record, *typ.Map:
			return true
		default:
			return false
		}
	case *typ.Record, *typ.Map:
		return true
	default:
		return false
	}
}

func (s *Solution) fieldAssignmentsForRoot(baseRoot string) []mergedField {
	return s.fieldAssignmentsForRootAt(0, baseRoot)
}

func (s *Solution) fieldAssignmentsForRootAt(p cfg.Point, baseRoot string) []mergedField {
	if s == nil || baseRoot == "" || (len(s.values) == 0 && len(s.mutableOut[p]) == 0) {
		return nil
	}
	if s.fieldOverlayCache == nil {
		s.fieldOverlayCache = make(map[fieldOverlayCacheKey][]mergedField)
	}
	cacheKey := fieldOverlayCacheKey{point: p, root: baseRoot}
	if fields, ok := s.fieldOverlayCache[cacheKey]; ok {
		return fields
	}
	fields := s.collectFieldAssignmentsForRootAt(p, baseRoot)
	s.fieldOverlayCache[cacheKey] = fields
	return fields
}

func (s *Solution) collectFieldAssignmentsForRootAt(p cfg.Point, baseRoot string) []mergedField {
	values := make(map[string]typ.Type, 8)
	if indexed := s.fieldOverlayValuesForRoot(baseRoot); len(indexed) > 0 {
		for suffix, value := range indexed {
			values[suffix] = value
		}
	}
	if state := s.mutableOut[p]; state != nil {
		prefixLen := len(baseRoot)
		for key, av := range state {
			if len(key) <= prefixLen || key[:prefixLen] != baseRoot {
				continue
			}
			suffix := key[prefixLen:]
			if _, ok := parseSingleOverlaySegment(suffix); !ok {
				continue
			}
			values[suffix] = av.ProjectValue()
		}
	}
	fields := make([]mergedField, 0, len(values))
	for suffix, value := range values {
		seg, ok := parseSingleOverlaySegment(suffix)
		if !ok {
			continue
		}
		fieldType, optional := typ.SplitNilableFieldType(value)
		switch s.presenceAtPoint(p, baseRoot+suffix) {
		case pathPresencePresent:
			if inner, nilable := typ.SplitNilableFieldType(value); nilable {
				fieldType = inner
			}
			optional = false
		case pathPresenceAbsent:
			fieldType = typ.Nil
			optional = true
		case pathPresenceMaybe:
			optional = true
		}
		fields = append(fields, mergedField{Name: seg.Name, Type: fieldType, Optional: optional})
	}
	sortMergedFields(fields)
	return fields
}

func (s *Solution) fieldOverlayValuesForRoot(baseRoot string) map[string]typ.Type {
	if s == nil || baseRoot == "" || len(s.values) == 0 {
		return nil
	}
	s.ensureFieldOverlayIndex()
	return s.fieldOverlayIndex[baseRoot]
}

func (s *Solution) ensureFieldOverlayIndex() {
	if s == nil || s.fieldOverlayIndex != nil {
		return
	}
	s.fieldOverlayIndex = make(map[string]map[string]typ.Type)
	for key, av := range s.values {
		s.indexFieldOverlayValue(key, av.ProjectValue())
	}
}

func (s *Solution) indexFieldOverlayValue(key string, value typ.Type) {
	if s == nil || key == "" {
		return
	}
	root, suffix, ok := pathkey.ParseRootAndSuffix(constraint.PathKey(key))
	if !ok || root == "" {
		return
	}
	if _, ok := parseSingleOverlaySegment(suffix); !ok {
		return
	}
	if s.fieldOverlayIndex == nil {
		s.fieldOverlayIndex = make(map[string]map[string]typ.Type)
	}
	if value == nil {
		if bySuffix := s.fieldOverlayIndex[root]; bySuffix != nil {
			delete(bySuffix, suffix)
			if len(bySuffix) == 0 {
				delete(s.fieldOverlayIndex, root)
			}
		}
		return
	}
	bySuffix := s.fieldOverlayIndex[root]
	if bySuffix == nil {
		bySuffix = make(map[string]typ.Type, 1)
		s.fieldOverlayIndex[root] = bySuffix
	}
	bySuffix[suffix] = value
}

func sortMergedFields(fields []mergedField) {
	if len(fields) <= 1 {
		return
	}
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Name != fields[j].Name {
			return fields[i].Name < fields[j].Name
		}
		if fields[i].Optional != fields[j].Optional {
			return !fields[i].Optional && fields[j].Optional
		}
		return fields[i].Type.Hash() < fields[j].Type.Hash()
	})
}

func parseSingleOverlaySegment(suffix string) (constraint.Segment, bool) {
	if suffix == "" {
		return constraint.Segment{}, false
	}
	switch suffix[0] {
	case '.':
		name := suffix[1:]
		if name == "" || !pathkey.IsIdentName(name) {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentField, Name: name}, true
	case '[':
		if len(suffix) < 3 || suffix[len(suffix)-1] != ']' {
			return constraint.Segment{}, false
		}
		inner := suffix[1 : len(suffix)-1]
		if inner == "" {
			return constraint.Segment{}, false
		}
		if inner[0] == '"' {
			if len(inner) < 2 || inner[len(inner)-1] != '"' {
				return constraint.Segment{}, false
			}
			name, ok := parseQuotedOverlayIndex(inner[1 : len(inner)-1])
			if !ok {
				return constraint.Segment{}, false
			}
			return constraint.Segment{Kind: constraint.SegmentIndexString, Name: name}, true
		}
		if idx, ok := pathkey.ParseIntLiteral(inner); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: inner}, true
	default:
		return constraint.Segment{}, false
	}
}

func parseQuotedOverlayIndex(inner string) (string, bool) {
	if inner == "" {
		return "", true
	}
	escaped := false
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' {
			escaped = true
			break
		}
	}
	if !escaped {
		return inner, true
	}

	out := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if ch != '\\' {
			out = append(out, ch)
			continue
		}
		if i+1 >= len(inner) {
			return "", false
		}
		next := inner[i+1]
		if next != '\\' && next != '"' {
			return "", false
		}
		out = append(out, next)
		i++
	}
	return string(out), true
}
