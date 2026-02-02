package flow

import (
	"slices"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
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
	inputs         *Inputs
	resolver       narrow.Resolver
	pkResolver     *pathkey.Resolver
	values         map[string]typ.Type // canonical key (sym<ID>@<ver><segs>) -> type
	edgeConditions map[edgeKey]constraint.Condition

	edgeNumericConstraints map[edgeKey][]constraint.NumericConstraint
	unsatEdges             map[edgeKey]bool // edges proven unreachable by constraints
	pointConditions        map[cfg.Point]constraint.Condition
	numericStates          map[cfg.Point]*numeric.State
	iterations             int

	// Scratch buffers to reduce allocations in hot paths
	scratchTypes  []typ.Type
	scratchSuffix map[string]struct{}
}

// edgeKey identifies a CFG edge for condition and constraint lookups.
type edgeKey struct {
	from cfg.Point
	to   cfg.Point
}

func sortedEdgeKeys(m map[edgeKey]constraint.Condition) []edgeKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]edgeKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})
	return keys
}

// Solve computes flow analysis and returns the solution.
//
// Solve is the main entry point for flow-sensitive type analysis. It takes
// the analysis inputs (CFG, declared types, assignments, constraints) and
// a type resolver, then computes narrowed types at each program point.
//
// The algorithm:
//  1. Builds edge conditions from input constraints
//  2. Checks numeric constraints to mark unreachable edges
//  3. Propagates conditions through the CFG to compute point conditions
//  4. Runs worklist iteration to compute final narrowed types
func Solve(inputs *Inputs, resolver narrow.Resolver) *Solution {
	size := 0
	var pkRes *pathkey.Resolver
	if inputs != nil && inputs.Graph != nil {
		inputs.Normalize()
		size = inputs.Graph.Size()
		pkRes = pathkey.NewResolver(inputs.Graph)
	}

	s := &Solution{
		inputs:                 inputs,
		resolver:               resolver,
		pkResolver:             pkRes,
		values:                 make(map[string]typ.Type, size*2),
		edgeConditions:         make(map[edgeKey]constraint.Condition),
		edgeNumericConstraints: make(map[edgeKey][]constraint.NumericConstraint),
		unsatEdges:             make(map[edgeKey]bool),
		pointConditions:        make(map[cfg.Point]constraint.Condition),
		numericStates:          make(map[cfg.Point]*numeric.State),
		scratchTypes:           make([]typ.Type, 0, 8),
		scratchSuffix:          make(map[string]struct{}, 16),
	}
	s.buildEdgeConditions()
	s.buildEdgeNumericConstraints()
	s.checkNumericConstraints()

	// Propagate conditions using standalone propagate package
	s.runPropagation()

	s.solve()
	return s
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
	edgeConds := make(propagate.EdgeConditions)
	for _, k := range sortedEdgeKeys(s.edgeConditions) {
		edgeConds[propagate.EdgeKey{From: k.from, To: k.to}] = s.edgeConditions[k]
	}

	// Convert assignments to propagate format
	var assigns []propagate.Assignment
	for _, a := range s.inputs.Assignments {
		if a.TargetPath.Symbol != 0 {
			assigns = append(assigns, propagate.Assignment{
				Point:      a.Point,
				TargetSym:  a.TargetPath.Symbol,
				TargetSegs: a.TargetPath.Segments,
			})
		}
	}

	propInputs := &propagate.Inputs{
		Graph:          s.inputs.Graph,
		EdgeConditions: edgeConds,
		DeadPoints:     s.inputs.DeadPoints,
		Assignments:    assigns,
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
	result := make(map[constraint.PathKey]typ.Type)

	// Add target path with its base type using canonical key
	targetKey := s.pkResolver.KeyAt(p, targetPath)
	if targetKey != "" {
		result[targetKey] = baseType
	}

	// Add declared types for symbols visible at this point
	if s.inputs != nil && s.inputs.DeclaredTypes != nil && s.inputs.Graph != nil {
		declSyms := make([]cfg.SymbolID, 0, len(s.inputs.DeclaredTypes))
		for sym := range s.inputs.DeclaredTypes {
			declSyms = append(declSyms, sym)
		}
		slices.Sort(declSyms)
		for _, sym := range declSyms {
			declPath := constraint.Path{Symbol: sym}
			canonicalKey := s.pkResolver.KeyAt(p, declPath)
			if canonicalKey == "" {
				continue
			}
			declType := s.inputs.DeclaredTypes[sym]
			if _, exists := result[canonicalKey]; !exists {
				result[canonicalKey] = declType
			}
		}
	}

	// Add types for all paths referenced by constraints
	if len(constraints) > 0 {
		seen := make(map[constraint.PathKey]bool)
		for key := range result {
			seen[key] = true
		}
		for _, c := range constraints {
			for _, cpath := range c.Paths() {
				if cpath.IsEmpty() || cpath.Symbol == 0 {
					continue
				}
				canonicalKey := s.pkResolver.KeyAt(p, cpath)
				if canonicalKey == "" || seen[canonicalKey] {
					continue
				}
				seen[canonicalKey] = true

				// Look up value using canonical key
				if t := s.values[string(canonicalKey)]; t != nil {
					result[canonicalKey] = t
					continue
				}

				// Derive child path type from parent's base type
				if isDescendantOf(cpath, targetPath) && baseType != nil {
					relativeSegs := cpath.Segments[len(targetPath.Segments):]
					if len(relativeSegs) > 0 {
						if derived, ok := s.deriveTypeFrom(baseType, relativeSegs); ok {
							result[canonicalKey] = derived
							continue
						}
					}
				}

				// Fall back to declared type
				if declType := s.inputs.DeclaredTypes[cpath.Symbol]; declType != nil {
					if len(cpath.Segments) > 0 {
						if derived, ok := s.deriveTypeFrom(declType, cpath.Segments); ok {
							result[canonicalKey] = derived
						}
					} else {
						result[canonicalKey] = declType
					}
				}
			}
		}
	}

	return result
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
	if s == nil || s.values == nil {
		return nil
	}
	return s.values[key]
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
	return s.values
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

	s.initializeDeclarations()

	// Build dependency maps for worklist propagation
	phiDeps := s.buildPhiDependencies()
	assignDeps := s.buildAssignmentDependencies()
	edgeDeps := s.buildEdgeConditionDependencies()

	worklist := g.RPO()
	inQueue := make(map[cfg.Point]bool, len(worklist))
	for _, p := range worklist {
		inQueue[p] = true
	}

	maxIterations := g.Size() * 100
	for len(worklist) > 0 {
		// FIFO queue: process in forward RPO order for correct dataflow
		p := worklist[0]
		worklist = worklist[1:]
		inQueue[p] = false

		changedKeys := s.processPointReturnChangedKeys(p)
		if len(changedKeys) > 0 {
			// Add direct successors
			for _, succ := range g.Successors(p) {
				if !inQueue[succ] {
					worklist = append(worklist, succ)
					inQueue[succ] = true
				}
			}
			// Add dependent points from all dependency maps
			worklist = addDependentPoints(phiDeps, changedKeys, worklist, inQueue)
			worklist = addDependentPoints(assignDeps, changedKeys, worklist, inQueue)
			worklist = addDependentPoints(edgeDeps, changedKeys, worklist, inQueue)
		}

		s.iterations++
		if s.iterations > maxIterations {
			break
		}
	}
}

// initializeDeclarations seeds the solution with declared types.
//
// Initializes the values map with types from:
//   - DeclaredTypes: parameter and local variable declarations
//   - SiblingTypes: captured variables from parent scopes
//   - LiteralTypes: function literals defined in current scope
func (s *Solution) initializeDeclarations() {
	// DeclaredTypes: try entry, fall back to declaration point
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

	// LiteralTypes: try entry, fall back to declaration point, skip if already set
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
	// Check literal types first (function literals defined in current scope)
	if t := s.inputs.LiteralTypes[path.Symbol]; t != nil {
		return t
	}
	// Check sibling types (captured variables from parent scope)
	if t := s.inputs.SiblingTypes[path.Symbol]; t != nil {
		return t
	}
	return s.inputs.DeclaredTypes[path.Symbol]
}

// deriveTypeFrom extracts a nested type from base following path segments.
//
// For example, deriving from {user: {name: string}} with segments [.user, .name]
// yields string. Handles both field access and index access segments.
func (s *Solution) deriveTypeFrom(base typ.Type, segs []constraint.Segment) (typ.Type, bool) {
	if base == nil || s.resolver == nil {
		return nil, false
	}

	current := base
	for _, seg := range segs {
		switch seg.Kind {
		case constraint.SegmentField:
			next, ok := s.resolver.Field(current, seg.Name)
			if ok && !isOpenRecordFallback(current, next) {
				current = next
				break
			}
			// In Lua t.x and t["x"] are equivalent; try index access for maps.
			key := typ.LiteralString(seg.Name)
			if idxNext, idxOk := s.resolver.Index(current, key); idxOk {
				current = idxNext
				break
			}
			if !ok {
				return nil, false
			}
			current = next
		case constraint.SegmentIndexString:
			if next, ok := s.resolver.Field(current, seg.Name); ok && !isOpenRecordFallback(current, next) {
				current = next
				break
			}
			key := typ.LiteralString(seg.Name)
			next, ok := s.resolver.Index(current, key)
			if !ok {
				return nil, false
			}
			current = next
		case constraint.SegmentIndexInt:
			key := typ.LiteralInt(int64(seg.Index))
			next, ok := s.resolver.Index(current, key)
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
		if current == nil {
			return nil, false
		}
	}
	return current, true
}

// isOpenRecordFallback detects when field lookup failed on an open record.
//
// Open records return unknown for missing fields. This function identifies
// that case so callers can try index-based access as a fallback, which may
// succeed if the record has a map component.
func isOpenRecordFallback(base typ.Type, result typ.Type) bool {
	if result == nil || result.Kind() != kind.Unknown {
		return false
	}
	rec, ok := base.(*typ.Record)
	return ok && rec.Open
}

func (s *Solution) resolveTypeKey(key narrow.TypeKey) typ.Type {
	if s.inputs == nil || s.inputs.TypeKeys == nil {
		return nil
	}
	switch key.Kind {
	case narrow.TypeKeyHash:
		return s.inputs.TypeKeys[key.Hash]
	case narrow.TypeKeyBuiltin:
		return narrow.TypeForKind(kind.FromString(key.Name))
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
	// Collect field assignments for this base key
	prefix := baseKey + "."
	var fields []typ.Field
	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(key) <= len(prefix) {
			continue
		}
		if key[:len(prefix)] != prefix {
			continue
		}
		// Extract field name (only direct children, not nested paths)
		suffix := key[len(prefix):]
		if idx := strings.IndexAny(suffix, ".["); idx != -1 {
			continue // Skip nested paths like ".foo.bar" or ".foo[0]"
		}
		fields = append(fields, typ.Field{Name: suffix, Type: s.values[key]})
	}

	if len(fields) == 0 {
		return baseType
	}

	if baseType == nil {
		baseType = typ.NewRecord().SetOpen(true).Build()
	}

	// Merge fields into base type
	return typ.Visit(baseType, typ.Visitor[typ.Type]{
		Map: func(m *typ.Map) typ.Type {
			// Map base: create Record(open) with MapComponent + merged fields
			builder := typ.NewRecord().SetOpen(true)
			builder.MapComponent(m.Key, m.Value)
			for _, f := range fields {
				builder.Field(f.Name, f.Type)
			}
			return builder.Build()
		},
		Record: func(r *typ.Record) typ.Type {
			// Build merged record: existing fields + new fields
			builder := typ.NewRecord()
			if r.Open {
				builder.SetOpen(true)
			}
			existing := make(map[string]bool)
			for _, f := range r.Fields {
				builder.Field(f.Name, f.Type)
				existing[f.Name] = true
			}
			for _, f := range fields {
				if !existing[f.Name] {
					builder.Field(f.Name, f.Type)
				}
			}
			if r.Metatable != nil {
				builder.Metatable(r.Metatable)
			}
			if r.HasMapComponent() {
				builder.MapComponent(r.MapKey, r.MapValue)
			}
			return builder.Build()
		},
		Default: func(t typ.Type) typ.Type {
			// Base is not a record or map; create one with just the field assignments
			builder := typ.NewRecord().SetOpen(true)
			for _, f := range fields {
				builder.Field(f.Name, f.Type)
			}
			return builder.Build()
		},
	})
}
