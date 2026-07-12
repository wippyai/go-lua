// Package densewto is an isolated proof that a prepared body can execute its
// WTO directly over dense point storage while reusing the production semantic
// transactions. It is not wired into the analyzer.
package densewto

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// NaturalLoop is the representative reducible WTO component. Prefix and
// Suffix are singleton WTO vertices; Head+Body are the cyclic component. The
// POC deliberately fails closed instead of pretending this is a general WTO
// compiler.
type NaturalLoop struct {
	Prefix []cfg.Point
	Head   cfg.Point
	Body   []cfg.Point
	Suffix []cfg.Point
}

type Config struct {
	Graph      cfg.Graph
	Registry   *axis.Registry
	Operations *operationplan.Plan
	Node       transfer.NodeTransfer
	Edge       transfer.EdgeTransfer
	EntryState state.State
	StateLanes []state.LaneID
	WidenDelay int
	Loop       NaturalLoop
}

type Result struct {
	Points    []state.State
	Transfers int
}

type Executor struct {
	graph      cfg.Graph
	reg        *axis.Registry
	ops        *operationplan.Plan
	node       transfer.NodeTransfer
	edge       transfer.EdgeTransfer
	domain     lattice.Lattice[state.State]
	normalize  func(state.State) state.State
	nodeWork   []bool
	edgeWork   []bool
	indegree   []uint32
	entry      state.State
	loop       NaturalLoop
	widenDelay int
}

func Compile(c Config) (*Executor, error) {
	if c.Graph == nil || c.Registry == nil || c.Operations == nil {
		return nil, fmt.Errorf("densewto: graph, registry, and operation plan are required")
	}
	if c.Operations.PointCount() != c.Graph.Size() {
		return nil, fmt.Errorf("densewto: operation rows=%d graph points=%d", c.Operations.PointCount(), c.Graph.Size())
	}
	seen := make([]bool, c.Graph.Size())
	nodeWork := make([]bool, c.Graph.Size())
	edgeWork := make([]bool, c.Graph.Size())
	indegree := make([]uint32, c.Graph.Size())
	for p := cfg.Point(0); int(p) < c.Graph.Size(); p++ {
		for _, succ := range cfg.SuccessorsReadOnly(c.Graph, p) {
			indegree[succ]++
		}
	}
	mark := func(p cfg.Point) error {
		if uint64(p) >= uint64(len(seen)) || seen[p] {
			return fmt.Errorf("densewto: invalid or duplicate point %d", p)
		}
		seen[p] = true
		// Force every admitted row through the canonical barrier catalog. The
		// executor invokes the shared point transaction once, not each sidecar.
		cursor := c.Operations.Cursor(p)
		var last operationplan.Barrier
		for {
			cell, ok := cursor.Next()
			if !ok {
				break
			}
			meta, exists := operationplan.Describe(cell.Kind())
			if !exists || meta.Barrier < last {
				return fmt.Errorf("densewto: non-canonical operation row at %d", p)
			}
			if meta.Phase == operationplan.Node {
				nodeWork[p] = true
			}
			if meta.Phase == operationplan.Edge || meta.Stages.Has(operationplan.E5CallEffects) {
				edgeWork[p] = true
			}
			last = meta.Barrier
		}
		return nil
	}
	for _, p := range c.Loop.Prefix {
		if err := mark(p); err != nil {
			return nil, err
		}
	}
	if err := mark(c.Loop.Head); err != nil {
		return nil, err
	}
	for _, p := range c.Loop.Body {
		if err := mark(p); err != nil {
			return nil, err
		}
	}
	for _, p := range c.Loop.Suffix {
		if err := mark(p); err != nil {
			return nil, err
		}
	}
	for p, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("densewto: point %d is outside representative WTO", p)
		}
	}
	domain, err := state.TryDomainWithOptionalLanes(c.Registry, c.StateLanes)
	if err != nil {
		return nil, err
	}
	node := c.Node
	if node == nil {
		node = func(_ transfer.NodeContext, in state.State) state.State { return in }
	}
	edge := c.Edge
	if edge == nil {
		edge = func(_ transfer.EdgeContext, out state.State) state.State { return out }
	}
	normalize := func(st state.State) state.State { return st }
	if c.StateLanes != nil {
		normalize = func(st state.State) state.State { return state.NormalizeForDomain(domain, st) }
	}
	return &Executor{graph: c.Graph, reg: c.Registry, ops: c.Operations, node: node, edge: edge,
		domain: domain, normalize: normalize, nodeWork: nodeWork, edgeWork: edgeWork,
		indegree: indegree, entry: c.EntryState, loop: c.Loop, widenDelay: c.WidenDelay}, nil
}

// Run owns dense scratch. No equation maps, contribution maps, dependency
// maps, closures per equation, or nested callee body solve are constructed.
func (e *Executor) Run() Result {
	n := e.graph.Size()
	cur := make([]state.State, n)
	versions := make([]uint64, n)
	visits := make([]uint32, n)
	widenChanges := make([]uint32, n)
	propagated := make([]uint64, n)
	bottom := e.domain.Bottom()
	for i := range cur {
		cur[i] = bottom
	}
	entry := e.graph.Entry()
	cur[entry] = e.normalize(state.Reachable(e.entry))
	versions[entry] = 1
	var revision uint64 = 1
	transfers := 0
	emit := func(dst cfg.Point, value state.State) {
		prev := cur[dst]
		next := e.domain.Join(prev, e.normalize(value))
		if dst == e.loop.Head && visits[dst] != 0 {
			if int(widenChanges[dst]) >= e.widenDelay {
				next = e.domain.Widen(prev, next)
			} else if !e.domain.Equal(next, prev) {
				widenChanges[dst]++
			}
		}
		if !e.domain.Equal(next, prev) {
			cur[dst] = next
			revision++
			versions[dst] = revision
		}
	}
	runPoint := func(point cfg.Point) {
		transfers++
		in := cur[point]
		if e.domain.Equal(in, bottom) {
			if point == e.loop.Head {
				visits[point]++
			}
			return
		}
		read := func(other cfg.Point) state.State { return cur[other] }
		successors := cfg.SuccessorsReadOnly(e.graph, point)
		if !e.nodeWork[point] && !e.edgeWork[point] && len(successors) == 1 && e.indegree[successors[0]] == 1 {
			// A unique-predecessor identity row has exactly one contribution.
			// Its monotone accumulated value is its predecessor value, so the
			// dense executor can publish the persistent snapshot directly.
			if propagated[point] != versions[point] {
				cur[successors[0]] = in
				revision++
				versions[successors[0]] = revision
				propagated[point] = versions[point]
			}
			return
		}
		out := in
		if e.nodeWork[point] {
			out = e.normalize(e.node(transfer.NodeContext{Graph: e.graph, Registry: e.reg, Point: point, Node: e.graph.Node(point), Read: read}, in))
		}
		for _, succ := range successors {
			cond, has := e.graph.EdgeCond(point, succ)
			has = has && e.graph.IsBranch(point)
			value := out
			if e.edgeWork[point] {
				value = e.edge(transfer.EdgeContext{Graph: e.graph, Registry: e.reg, Edge: cfg.Edge{From: point, To: succ, Cond: cond}, HasCond: has, Read: read}, out)
			}
			emit(succ, value)
		}
		if point == e.loop.Head {
			visits[point]++
		}
	}
	for _, p := range e.loop.Prefix {
		runPoint(p)
	}
	for {
		before := versions[e.loop.Head]
		runPoint(e.loop.Head)
		for _, p := range e.loop.Body {
			runPoint(p)
		}
		if versions[e.loop.Head] == before {
			break
		}
	}
	for _, p := range e.loop.Suffix {
		runPoint(p)
	}

	// Match the production solver's two bounded decreasing passes. Candidate
	// equations read the converged state but accumulate from the original entry.
	for pass := 0; pass < 2; pass++ {
		candidate := make([]state.State, n)
		for i := range candidate {
			candidate[i] = bottom
		}
		candidate[entry] = e.normalize(state.Reachable(e.entry))
		candidateEmit := func(dst cfg.Point, value state.State) {
			candidate[dst] = e.domain.Join(candidate[dst], e.normalize(value))
		}
		for _, point := range append(append(append(append([]cfg.Point{}, e.loop.Prefix...), e.loop.Head), e.loop.Body...), e.loop.Suffix...) {
			in := cur[point]
			if e.domain.Equal(in, bottom) {
				continue
			}
			read := func(other cfg.Point) state.State { return cur[other] }
			successors := cfg.SuccessorsReadOnly(e.graph, point)
			if !e.nodeWork[point] && !e.edgeWork[point] && len(successors) == 1 && e.indegree[successors[0]] == 1 {
				candidate[successors[0]] = cur[point]
				continue
			}
			out := in
			if e.nodeWork[point] {
				out = e.normalize(e.node(transfer.NodeContext{Graph: e.graph, Registry: e.reg, Point: point, Node: e.graph.Node(point), Read: read}, in))
			}
			for _, succ := range successors {
				cond, has := e.graph.EdgeCond(point, succ)
				has = has && e.graph.IsBranch(point)
				value := out
				if e.edgeWork[point] {
					value = e.edge(transfer.EdgeContext{Graph: e.graph, Registry: e.reg, Edge: cfg.Edge{From: point, To: succ, Cond: cond}, HasCond: has, Read: read}, out)
				}
				candidateEmit(succ, value)
			}
		}
		changed := false
		for i := range cur {
			next := e.domain.Narrow(cur[i], candidate[i])
			if e.domain.LessOrEq(next, cur[i]) && !e.domain.Equal(next, cur[i]) {
				cur[i] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return Result{Points: cur, Transfers: transfers}
}
