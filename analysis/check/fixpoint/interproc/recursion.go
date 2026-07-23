package interproc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrRecursiveApproximationUnavailable means an evaluator attempted to read a
// key which is neither a member of the SCC it owns nor a closed dependency.
// In particular, an in-progress approximation is never available to an
// external evaluator.
var ErrRecursiveApproximationUnavailable = errors.New("interproc: recursive approximation is unavailable")

// DiscoveryLimitError is returned rather than broadening an exact projection
// when recursive demand discovers too many distinct instance keys.
type DiscoveryLimitError struct{ Limit uint64 }

func (e *DiscoveryLimitError) Error() string {
	return fmt.Sprintf("interproc: recursive instance discovery exceeded limit %d", e.Limit)
}

// RecursiveEvaluator supplies the body-specific part of a recursive solve.
// Discover must return exact callee instance keys only. Evaluate receives
// closed dependencies and current approximations for its own SCC through
// RecursiveValues; it cannot observe a partial result from another SCC.
type RecursiveEvaluator interface {
	Discover(context.Context, InstanceKey) ([]InstanceKey, error)
	Evaluate(context.Context, InstanceKey, RecursiveValues) (ClosedOutcome, error)
}

// FiniteOutcomeLattice is the sole termination authority for an instance SCC.
// Height is a bound on strict ascents of each member. Join must return changed
// only for a strict ascent and must never merge two different instance keys.
// A coordinator rejects a non-positive height instead of silently widening a
// cache key or publishing an unproven approximation.
type FiniteOutcomeLattice interface {
	Bottom(InstanceKey) (ClosedOutcome, error)
	Join(key InstanceKey, previous, candidate ClosedOutcome) (next ClosedOutcome, changed bool, err error)
	Height() uint64
}

// RecursiveValues is a read-only view passed to one body evaluation.
// It contains no caller context, entry binding, or mutable table cell.
type RecursiveValues struct {
	current map[string]ClosedOutcome
	closed  map[string]ClosedOutcome
}

// Read returns a current approximation only for the owning SCC, otherwise a
// completed dependency. A key absent from both sets is a protocol violation.
func (v RecursiveValues) Read(key InstanceKey) (ClosedOutcome, error) {
	if !key.valid() {
		return ClosedOutcome{}, fmt.Errorf("interproc: malformed recursive instance key")
	}
	if outcome, ok := v.current[instanceMapKey(key)]; ok {
		return outcome, nil
	}
	if outcome, ok := v.closed[instanceMapKey(key)]; ok {
		return outcome, nil
	}
	return ClosedOutcome{}, ErrRecursiveApproximationUnavailable
}

// RecursionMetrics describes completed coordinator work. Groups counts SCCs,
// not transient graph traversals.
type RecursionMetrics struct {
	Groups, DirectSCCs, MutualSCCs, AtomicCommits, Failures uint64
}

// RecursionCoordinator coordinates one exact projected table. The ownership
// gate intentionally serializes discovery: allowing racing traversals to form
// their own SCCs makes group ownership schedule-dependent. Within a group the
// owner is always the lexicographically least canonical instance key.
type RecursionCoordinator struct {
	table *ProjectedTable
	limit uint64

	gate    chan struct{}
	mu      sync.Mutex
	metrics RecursionMetrics
}

// NewRecursionCoordinator creates a coordinator for table. limit is the
// maximum number of freshly discovered exact keys in one resolve; zero is
// rejected at Resolve time as a fail-closed configuration.
func NewRecursionCoordinator(table *ProjectedTable, limit uint64) *RecursionCoordinator {
	return &RecursionCoordinator{table: table, limit: limit, gate: make(chan struct{}, 1)}
}

func (c *RecursionCoordinator) Metrics() RecursionMetrics {
	if c == nil {
		return RecursionMetrics{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metrics
}

// Resolve discovers and solves every needed exact instance SCC. Equal external
// callers continue to join the table cell; they never receive an approximation.
func (c *RecursionCoordinator) Resolve(ctx context.Context, root InstanceKey, evaluator RecursiveEvaluator, lattice FiniteOutcomeLattice) (ClosedOutcome, error) {
	if c == nil || c.table == nil || evaluator == nil || lattice == nil || ctx == nil || !root.valid() || c.limit == 0 || lattice.Height() == 0 {
		return ClosedOutcome{}, fmt.Errorf("interproc: invalid recursive resolution")
	}
	if err := ctx.Err(); err != nil {
		return ClosedOutcome{}, err
	}
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return ClosedOutcome{}, ctx.Err()
	}

	group := recursiveGroup{
		coordinator: c,
		ctx:         ctx,
		evaluator:   evaluator,
		lattice:     lattice,
		nodes:       make(map[string]*recursiveNode),
		closed:      make(map[string]ClosedOutcome),
	}
	if err := group.discover(root); err != nil {
		group.failUnclosed(err)
		c.noteFailure()
		return ClosedOutcome{}, err
	}
	if outcome, ok := group.closed[instanceMapKey(root)]; ok {
		return outcome, nil
	}
	if err := group.solve(); err != nil {
		group.failUnclosed(err)
		c.noteFailure()
		return ClosedOutcome{}, err
	}
	outcome, ok := group.closed[instanceMapKey(root)]
	if !ok || !outcome.Valid() {
		return ClosedOutcome{}, fmt.Errorf("interproc: recursive root did not close")
	}
	return outcome, nil
}

func (c *RecursionCoordinator) noteFailure() {
	c.mu.Lock()
	c.metrics.Failures++
	c.mu.Unlock()
}

type recursiveNode struct {
	key      InstanceKey
	cell     *tableCell
	bucketID ContentID
	edges    []string
}

type recursiveGroup struct {
	coordinator *RecursionCoordinator
	ctx         context.Context
	evaluator   RecursiveEvaluator
	lattice     FiniteOutcomeLattice
	nodes       map[string]*recursiveNode
	closed      map[string]ClosedOutcome
}

func instanceMapKey(key InstanceKey) string {
	// The content digest is an index only. Include both canonical witnesses so
	// even an artificial digest collision cannot merge recursive ownership.
	canonical := key.CanonicalBytes()
	return fmt.Sprintf("%d:%s%d:%s", len(canonical), canonical, len(key.artifactCanonical), key.artifactCanonical)
}

func instanceLess(left, right InstanceKey) bool {
	return instanceMapKey(left) < instanceMapKey(right)
}

func (g *recursiveGroup) discover(key InstanceKey) error {
	if err := g.ctx.Err(); err != nil {
		return err
	}
	id := instanceMapKey(key)
	if _, ok := g.nodes[id]; ok {
		return nil
	}
	if _, ok := g.closed[id]; ok {
		return nil
	}
	cell, bucketID, outcome, wait, err := g.reserve(key)
	if err != nil {
		return err
	}
	if wait != nil {
		select {
		case <-wait:
			if cell.err != nil {
				return cell.err
			}
			if !cell.closed || !cell.outcome.Valid() {
				return fmt.Errorf("interproc: recursive dependency completed without a closed outcome")
			}
			g.closed[id] = cell.outcome
			return nil
		case <-g.ctx.Done():
			return g.ctx.Err()
		}
	}
	if outcome.Valid() {
		g.closed[id] = outcome
		return nil
	}
	node := &recursiveNode{key: key, cell: cell, bucketID: bucketID}
	g.nodes[id] = node
	if uint64(len(g.nodes)) > g.coordinator.limit {
		return &DiscoveryLimitError{Limit: g.coordinator.limit}
	}
	callees, err := g.evaluator.Discover(g.ctx, key)
	if err != nil {
		return err
	}
	if err := canonicalizeCallees(callees); err != nil {
		return err
	}
	sort.Slice(callees, func(i, j int) bool { return instanceLess(callees[i], callees[j]) })
	for _, callee := range callees {
		calleeID := instanceMapKey(callee)
		node.edges = append(node.edges, calleeID)
		if err := g.discover(callee); err != nil {
			return err
		}
	}
	return nil
}

func canonicalizeCallees(callees []InstanceKey) error {
	seen := make(map[string]struct{}, len(callees))
	for _, callee := range callees {
		if !callee.valid() {
			return fmt.Errorf("interproc: malformed recursive callee key")
		}
		id := instanceMapKey(callee)
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("interproc: duplicate recursive callee key")
		}
		seen[id] = struct{}{}
	}
	return nil
}

// reserve returns a newly owned cell, a completed outcome, or a foreign cell
// to join. It never exposes a foreign in-progress approximation.
func (g *recursiveGroup) reserve(key InstanceKey) (cell *tableCell, bucketID ContentID, outcome ClosedOutcome, wait <-chan struct{}, err error) {
	table := g.coordinator.table
	bucketID = table.bucket(key.CanonicalBytes())
	table.mu.Lock()
	defer table.mu.Unlock()
	for _, existing := range table.buckets[bucketID] {
		if !existing.key.equal(key) {
			continue
		}
		if existing.closed {
			return existing, bucketID, existing.outcome, nil, nil
		}
		return existing, bucketID, ClosedOutcome{}, existing.done, nil
	}
	cell = &tableCell{key: key, done: make(chan struct{})}
	table.buckets[bucketID] = append(table.buckets[bucketID], cell)
	table.metrics.Misses++
	table.metrics.Executions++
	return cell, bucketID, ClosedOutcome{}, nil, nil
}

func (g *recursiveGroup) solve() error {
	components := g.components()
	componentFor := make(map[string]int, len(g.nodes))
	for index, component := range components {
		for _, id := range component {
			componentFor[id] = index
		}
	}
	solved := make([]bool, len(components))
	var solveComponent func(int) error
	solveComponent = func(index int) error {
		if solved[index] {
			return nil
		}
		dependencies := make(map[int]struct{})
		for _, id := range components[index] {
			for _, edge := range g.nodes[id].edges {
				if target, ok := componentFor[edge]; ok && target != index {
					dependencies[target] = struct{}{}
				}
			}
		}
		ordered := make([]int, 0, len(dependencies))
		for dependency := range dependencies {
			ordered = append(ordered, dependency)
		}
		sort.Slice(ordered, func(i, j int) bool { return components[ordered[i]][0] < components[ordered[j]][0] })
		for _, dependency := range ordered {
			if err := solveComponent(dependency); err != nil {
				return err
			}
		}
		if err := g.solveSCC(components[index]); err != nil {
			return err
		}
		solved[index] = true
		return nil
	}
	rootIDs := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		rootIDs = append(rootIDs, id)
	}
	sort.Strings(rootIDs)
	for _, id := range rootIDs {
		if err := solveComponent(componentFor[id]); err != nil {
			return err
		}
	}
	return nil
}

func (g *recursiveGroup) solveSCC(members []string) error {
	current := make(map[string]ClosedOutcome, len(members))
	for _, id := range members {
		bottom, err := g.lattice.Bottom(g.nodes[id].key)
		if err != nil || !bottom.Valid() {
			if err == nil {
				err = fmt.Errorf("interproc: lattice returned an invalid bottom")
			}
			return err
		}
		current[id] = bottom
	}
	for pass := uint64(0); pass <= g.lattice.Height(); pass++ {
		if err := g.ctx.Err(); err != nil {
			return err
		}
		next := make(map[string]ClosedOutcome, len(members))
		changed := false
		values := RecursiveValues{current: current, closed: g.closed}
		for _, id := range members {
			candidate, err := g.evaluator.Evaluate(g.ctx, g.nodes[id].key, values)
			if err != nil {
				return err
			}
			if !candidate.Valid() {
				return fmt.Errorf("interproc: recursive evaluator returned an invalid portable closed outcome")
			}
			joined, ascent, err := g.lattice.Join(g.nodes[id].key, current[id], candidate)
			if err != nil || !joined.Valid() {
				if err == nil {
					err = fmt.Errorf("interproc: lattice returned an invalid joined outcome")
				}
				return err
			}
			next[id] = joined
			changed = changed || ascent
		}
		if !changed {
			return g.commitSCC(members, current)
		}
		if pass == g.lattice.Height() {
			return fmt.Errorf("interproc: recursive lattice exceeded its declared height")
		}
		current = next
	}
	return fmt.Errorf("interproc: unreachable recursive lattice state")
}

func (g *recursiveGroup) commitSCC(members []string, outcomes map[string]ClosedOutcome) error {
	table := g.coordinator.table
	table.mu.Lock()
	defer table.mu.Unlock()
	// Validate every member before changing any cell: a group is observable only
	// as all closed or all solving/failed.
	for _, id := range members {
		node := g.nodes[id]
		if node == nil || !outcomes[id].Valid() || node.cell.closed {
			return fmt.Errorf("interproc: recursive SCC lost exclusive cell ownership")
		}
	}
	for _, id := range members {
		node := g.nodes[id]
		node.cell.outcome = outcomes[id]
		node.cell.closed = true
		g.closed[id] = outcomes[id]
	}
	for _, id := range members {
		close(g.nodes[id].cell.done)
	}
	g.coordinator.mu.Lock()
	g.coordinator.metrics.Groups++
	g.coordinator.metrics.AtomicCommits++
	if len(members) == 1 {
		for _, edge := range g.nodes[members[0]].edges {
			if edge == members[0] {
				g.coordinator.metrics.DirectSCCs++
				break
			}
		}
	} else {
		g.coordinator.metrics.MutualSCCs++
	}
	g.coordinator.mu.Unlock()
	return nil
}

func (g *recursiveGroup) failUnclosed(err error) {
	if err == nil {
		err = errors.New("interproc: recursive group failed")
	}
	table := g.coordinator.table
	table.mu.Lock()
	defer table.mu.Unlock()
	for _, node := range g.nodes {
		if node.cell.closed {
			continue
		}
		node.cell.err = err
		table.remove(node.bucketID, node.cell)
		close(node.cell.done)
		table.metrics.Failures++
	}
}

// components computes Tarjan SCCs from a canonically ordered graph. The
// resulting membership and owner ordering are independent of discovery timing.
func (g *recursiveGroup) components() [][]string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	index, next := make(map[string]int, len(ids)), 0
	low := make(map[string]int, len(ids))
	onStack := make(map[string]bool, len(ids))
	stack := make([]string, 0, len(ids))
	components := make([][]string, 0)
	var visit func(string)
	visit = func(id string) {
		index[id], low[id] = next, next
		next++
		stack, onStack[id] = append(stack, id), true
		edges := append([]string(nil), g.nodes[id].edges...)
		sort.Strings(edges)
		for _, edge := range edges {
			if _, internal := g.nodes[edge]; !internal {
				continue
			}
			if _, seen := index[edge]; !seen {
				visit(edge)
				if low[edge] < low[id] {
					low[id] = low[edge]
				}
			} else if onStack[edge] && index[edge] < low[id] {
				low[id] = index[edge]
			}
		}
		if low[id] != index[id] {
			return
		}
		component := make([]string, 0)
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == id {
				break
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}
	for _, id := range ids {
		if _, seen := index[id]; !seen {
			visit(id)
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i][0] < components[j][0] })
	return components
}
