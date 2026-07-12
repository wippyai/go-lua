// Package persistentcalls is an isolated proof of a generic alternative to
// symbolic per-feature transformers. It is not imported by the checker.
package persistentcalls

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type FunctionID string
type Revision uint64

type Summary[S any] struct {
	Value    S
	Revision Revision
}

type BodyResult[S any] struct {
	Summary     S
	CallEntries map[FunctionID]state.State
}

// SummaryReader records exact dependency revisions whenever a body consults a
// callee. A body need not know anything about invalidation or caching.
type SummaryReader[S any] interface {
	Read(FunctionID) (Summary[S], bool)
}

// Workspace is owned by exactly one lexical FunctionCell. ExtendEntry is only
// called for monotone entry growth while every observed callee revision is
// unchanged. A callee refinement discards the workspace and starts from the
// canonical aggregate entry.
type Workspace[S any] interface {
	ExtendEntry(state.State)
	Solve(context.Context, SummaryReader[S]) (BodyResult[S], error)
}

type WorkspaceFactory[S any] func(entry state.State) Workspace[S]

type Definition[S any] struct {
	ID      FunctionID
	Callees []FunctionID
	Factory WorkspaceFactory[S]
}

type cell[S any] struct {
	id FunctionID

	factory WorkspaceFactory[S]
	seed    state.State

	workspace      Workspace[S]
	workspaceEntry state.State
	workspaceDeps  map[FunctionID]Revision

	candidate BodyResult[S]
	published Summary[S]

	workspaceBuilds int
	entryExtensions int
}

type Engine[S any] struct {
	reg       *axis.Registry
	domain    lattice.Lattice[state.State]
	summaries lattice.Lattice[S]

	cells        map[FunctionID]*cell[S]
	definitions  map[FunctionID]Definition[S]
	predecessors map[FunctionID][]FunctionID
	influences   map[FunctionID][]FunctionID

	mu sync.Mutex
}

type Stats struct {
	WorkspaceBuilds map[FunctionID]int
	EntryExtensions map[FunctionID]int
	WTO             solve.Stats
}

func New[S any](reg *axis.Registry, summaries lattice.Lattice[S], definitions []Definition[S]) (*Engine[S], error) {
	if reg == nil {
		return nil, fmt.Errorf("persistentcalls: nil registry")
	}
	engine := &Engine[S]{
		reg:          reg,
		domain:       state.Domain(reg), // every registered State lane, unchanged
		summaries:    summaries,
		cells:        make(map[FunctionID]*cell[S], len(definitions)),
		definitions:  make(map[FunctionID]Definition[S], len(definitions)),
		predecessors: make(map[FunctionID][]FunctionID),
		influences:   make(map[FunctionID][]FunctionID),
	}
	for _, definition := range definitions {
		if definition.ID == "" || definition.Factory == nil {
			return nil, fmt.Errorf("persistentcalls: invalid definition")
		}
		if _, duplicate := engine.cells[definition.ID]; duplicate {
			return nil, fmt.Errorf("persistentcalls: duplicate function %q", definition.ID)
		}
		definition.Callees = canonicalIDs(definition.Callees)
		engine.definitions[definition.ID] = definition
		engine.cells[definition.ID] = &cell[S]{
			id:        definition.ID,
			factory:   definition.Factory,
			seed:      engine.domain.Bottom(),
			published: Summary[S]{Value: summaries.Bottom()},
		}
	}
	for _, definition := range engine.definitions {
		for _, callee := range definition.Callees {
			if _, ok := engine.cells[callee]; !ok {
				return nil, fmt.Errorf("persistentcalls: %q calls missing %q", definition.ID, callee)
			}
			engine.predecessors[callee] = append(engine.predecessors[callee], definition.ID)
			// Entry contributions flow caller -> callee; summaries flow callee ->
			// caller. Both are semantic equation dependencies.
			engine.influences[definition.ID] = append(engine.influences[definition.ID], callee)
			engine.influences[callee] = append(engine.influences[callee], definition.ID)
		}
	}
	for id := range engine.cells {
		engine.predecessors[id] = canonicalIDs(engine.predecessors[id])
		engine.influences[id] = canonicalIDs(engine.influences[id])
	}
	return engine, nil
}

// AddEntry monotonically grows a root/caller entry. A shrinking request is
// harmless because the canonical cell owns the join of every observed entry.
func (e *Engine[S]) AddEntry(id FunctionID, entry state.State) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cell, ok := e.cells[id]
	if !ok {
		return fmt.Errorf("persistentcalls: missing function %q", id)
	}
	cell.seed = e.domain.Join(cell.seed, entry)
	return nil
}

// Solve converges the complete interprocedural equation system through the
// production WTO scheduler. Published summaries remain unchanged on error or
// cancellation and are committed as one transaction only after convergence.
func (e *Engine[S]) Solve(ctx context.Context) (map[FunctionID]Summary[S], Stats, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ids := make([]FunctionID, 0, len(e.cells))
	for id := range e.cells {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	tracker := newRevisionTracker(e.summaries, e.snapshotPublished())
	approxDomain := approximationDomain(e.domain, e.summaries)
	var wtoStats solve.Stats
	var solveErr error
	system := solve.EquationSystem[FunctionID, approximation[S]]{
		Lattice: approxDomain,
		Cells:   ids,
		Transfer: func(id FunctionID, read func(FunctionID) approximation[S], emit func(FunctionID, approximation[S])) {
			if ctx.Err() != nil || solveErr != nil {
				return
			}
			entry := e.cells[id].seed
			for _, caller := range e.predecessors[id] {
				callerApprox := read(caller)
				if callerApprox.callsTop {
					entry = e.domain.Top()
					continue
				}
				if contribution, ok := callerApprox.callEntries[id]; ok {
					entry = e.domain.Join(entry, contribution)
				}
			}
			resolver := &trackingReader[S]{
				domain:  e.summaries,
				tracker: tracker,
				read: func(callee FunctionID) approximation[S] {
					return read(callee)
				},
			}
			result, err := e.cells[id].solve(ctx, e.domain, entry, resolver)
			if err != nil {
				solveErr = err
				return
			}
			if err := resolver.revalidate(); err != nil {
				solveErr = err
				return
			}
			emit(id, approximation[S]{valid: true, summary: result.Summary, callEntries: cloneCallEntries(result.CallEntries)})
		},
		WidenAt: func(id FunctionID) bool { return recursiveCell(id, e.influences) },
		WidenDelay: func(FunctionID) int {
			return 2
		},
		Stats: &wtoStats,
	}
	plan := solve.NewWTOPlan(ids, func(id FunctionID) []FunctionID {
		out := append([]FunctionID(nil), e.influences[id]...)
		return append(out, id)
	})
	result, _, err := solve.SolveWTOContextWithVersions(ctx, system, plan)
	if err != nil {
		e.abortWorkspaces()
		return e.snapshotPublished(), e.stats(wtoStats), err
	}
	if solveErr != nil {
		e.abortWorkspaces()
		return e.snapshotPublished(), e.stats(wtoStats), solveErr
	}
	if err := ctx.Err(); err != nil {
		e.abortWorkspaces()
		return e.snapshotPublished(), e.stats(wtoStats), err
	}

	// Transactional publication: no reader can observe a half-converged SCC.
	next := make(map[FunctionID]Summary[S], len(e.cells))
	for _, id := range ids {
		candidate := result[id]
		if !candidate.valid {
			e.abortWorkspaces()
			return e.snapshotPublished(), e.stats(wtoStats), fmt.Errorf("persistentcalls: %q did not converge", id)
		}
		// Commit the same canonical revision observed by dependent workspaces.
		// This keeps dependency vectors reusable across successful transactions.
		tracker.currentRevision(id, candidate.summary)
		published := tracker.summaries[id]
		next[id] = published
	}
	for id, summary := range next {
		e.cells[id].published = summary
	}
	return cloneSummaries(next), e.stats(wtoStats), nil
}

func (e *Engine[S]) abortWorkspaces() {
	for _, cell := range e.cells {
		cell.workspace = nil
		cell.workspaceDeps = nil
	}
}

func (c *cell[S]) solve(ctx context.Context, domain lattice.Lattice[state.State], entry state.State, reader *trackingReader[S]) (BodyResult[S], error) {
	reset := c.workspace == nil || !domain.LessOrEq(c.workspaceEntry, entry)
	if !reset {
		for dependency, revision := range c.workspaceDeps {
			approx := reader.read(dependency)
			current := reader.domain.Bottom()
			if approx.valid {
				current = approx.summary
			}
			if reader.tracker.currentRevision(dependency, current) != revision {
				reset = true
				break
			}
		}
	}
	if reset {
		c.workspace = c.factory(entry)
		if c.workspace == nil {
			return BodyResult[S]{}, fmt.Errorf("persistentcalls: %q returned nil workspace", c.id)
		}
		c.workspaceEntry = entry
		c.workspaceDeps = nil
		c.workspaceBuilds++
	} else if !domain.Equal(c.workspaceEntry, entry) {
		c.workspace.ExtendEntry(entry)
		c.workspaceEntry = entry
		c.entryExtensions++
	}
	reader.begin()
	result, err := c.workspace.Solve(ctx, reader)
	if err != nil {
		return BodyResult[S]{}, err
	}
	c.workspaceDeps = reader.dependencies()
	c.candidate = result
	return result, nil
}

func (e *Engine[S]) snapshotPublished() map[FunctionID]Summary[S] {
	out := make(map[FunctionID]Summary[S], len(e.cells))
	for id, cell := range e.cells {
		out[id] = cell.published
	}
	return out
}

func (e *Engine[S]) stats(wto solve.Stats) Stats {
	stats := Stats{WorkspaceBuilds: make(map[FunctionID]int), EntryExtensions: make(map[FunctionID]int), WTO: wto}
	for id, cell := range e.cells {
		stats.WorkspaceBuilds[id] = cell.workspaceBuilds
		stats.EntryExtensions[id] = cell.entryExtensions
	}
	return stats
}

func cloneSummaries[S any](in map[FunctionID]Summary[S]) map[FunctionID]Summary[S] {
	out := make(map[FunctionID]Summary[S], len(in))
	for id, summary := range in {
		out[id] = summary
	}
	return out
}

func canonicalIDs(in []FunctionID) []FunctionID {
	out := append([]FunctionID(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	dedup := out[:0]
	for _, id := range out {
		if len(dedup) == 0 || dedup[len(dedup)-1] != id {
			dedup = append(dedup, id)
		}
	}
	return dedup
}

func recursiveCell(start FunctionID, graph map[FunctionID][]FunctionID) bool {
	seen := map[FunctionID]bool{start: true}
	var visit func(FunctionID) bool
	visit = func(id FunctionID) bool {
		for _, next := range graph[id] {
			if next == start {
				return true
			}
			if !seen[next] {
				seen[next] = true
				if visit(next) {
					return true
				}
			}
		}
		return false
	}
	return visit(start)
}
