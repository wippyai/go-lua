package transformer

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// SymbolicExactWTOOptions temporarily bounds semi-naive component passes while
// recursive tuple-mu publication is being installed. Row cardinality is not a
// budget: every distinct finite correlated alternative is retained exactly.
type SymbolicExactWTOOptions struct {
	SymbolicCFGOptions
	MaxIterations int
	rowHash       func(SymbolicCFGRow) uint64
}

func (o SymbolicExactWTOOptions) normalized() SymbolicExactWTOOptions {
	o.SymbolicCFGOptions = o.SymbolicCFGOptions.normalized()
	if o.MaxIterations <= 0 {
		o.MaxIterations = 1024
	}
	return o
}

const (
	exactWTOPhaseOutside uint8 = iota
	exactWTOPhaseZero
	exactWTOPhaseOnePlus
)

type exactWTORow struct {
	row    SymbolicCFGRow
	phases []uint8
}

type exactWTOBucket struct {
	rows   []exactWTORow
	hashes []uint64
	cursor int
}

// SolveExactWTOCFGExpandedRows computes the least finite set of correlated
// rows over the reachable WTO tape. It is transactional: every error,
// cancellation, or budget failure returns a nil result. Recurrent effects are
// rejected because this first exact slice has no Kleene-star effect algebra.
func SolveExactWTOCFGExpandedRows(ctx context.Context, graph cfg.Graph, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGExpandTransfer, branch SymbolicCFGBranch, options SymbolicExactWTOOptions) (map[cfg.Point][]SymbolicCFGRow, error) {
	if ctx == nil {
		return nil, fmt.Errorf("transformer: exact WTO requires a context")
	}
	if graph == nil || arena == nil {
		return nil, fmt.Errorf("transformer: exact WTO requires graph and arena")
	}
	options = options.normalized()
	if !validCFGRow(arena, options.Shape, initial) {
		return nil, fmt.Errorf("transformer: exact WTO initial row is invalid for boundary shape")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("transformer: exact WTO canceled: %w", err)
	}
	tape, err := compileSymbolicWTOTape(graph)
	if err != nil {
		return nil, err
	}
	return solveExactWTOCFGExpandedRowsWithTape(ctx, graph, tape, arena, initial, transfer, branch, options)
}

// solveExactWTOCFGExpandedRowsWithTape executes against an immutable topology
// compiled by the function owner. Prepared compilers use this entry so
// repeated equation reads allocate only row scratch, never WTO structure.
func solveExactWTOCFGExpandedRowsWithTape(ctx context.Context, graph cfg.Graph, tape *symbolicWTOTape, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGExpandTransfer, branch SymbolicCFGBranch, options SymbolicExactWTOOptions) (map[cfg.Point][]SymbolicCFGRow, error) {
	solver, err := solveExactWTOCFGExpandedWithTape(ctx, graph, tape, arena, initial, transfer, branch, options, true, nil)
	if err != nil {
		return nil, err
	}
	return solver.publish()
}

// solveExactWTOCFGExpandedExitRowsWithTape is the prepared-compiler result
// seam. Relation construction needs only rows entering the CFG exit, so it
// avoids cloning and publishing every intermediate bucket. Public solver
// compatibility remains map-shaped through solveExactWTOCFGExpandedRowsWithTape.
func solveExactWTOCFGExpandedExitRowsWithTape(ctx context.Context, graph cfg.Graph, tape *symbolicWTOTape, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGExpandTransfer, branch SymbolicCFGBranch, options SymbolicExactWTOOptions) ([]SymbolicCFGRow, error) {
	return solveExactWTOCFGExpandedExitRowsWithTrace(ctx, graph, tape, arena, initial, transfer, branch, options, nil)
}

func solveExactWTOCFGExpandedExitRowsWithTrace(ctx context.Context, graph cfg.Graph, tape *symbolicWTOTape, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGExpandTransfer, branch SymbolicCFGBranch, options SymbolicExactWTOOptions, trace *sparseProjectionTraceBuilder) ([]SymbolicCFGRow, error) {
	solver, err := solveExactWTOCFGExpandedWithTape(ctx, graph, tape, arena, initial, transfer, branch, options, false, trace)
	if err != nil {
		return nil, err
	}
	exit := tape.denseIndex(graph.Exit())
	if exit < 0 {
		return nil, fmt.Errorf("transformer: exact WTO exit is unreachable")
	}
	return solver.publishBucket(uint32(exit))
}

func solveExactWTOCFGExpandedWithTape(ctx context.Context, graph cfg.Graph, tape *symbolicWTOTape, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGExpandTransfer, branch SymbolicCFGBranch, options SymbolicExactWTOOptions, retainIntermediates bool, trace *sparseProjectionTraceBuilder) (*exactWTOSolver, error) {
	if ctx == nil {
		return nil, fmt.Errorf("transformer: exact WTO requires a context")
	}
	if graph == nil || tape == nil || arena == nil {
		return nil, fmt.Errorf("transformer: exact WTO requires graph, tape, and arena")
	}
	options = options.normalized()
	if !validCFGRow(arena, options.Shape, initial) {
		return nil, fmt.Errorf("transformer: exact WTO initial row is invalid for boundary shape")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("transformer: exact WTO canceled: %w", err)
	}
	if err := certifyExactWTOComponentHeads(graph, tape); err != nil {
		return nil, err
	}
	if len(tape.points) == 0 {
		return nil, fmt.Errorf("transformer: exact WTO tape has no reachable points")
	}
	hashRow := options.rowHash
	if hashRow == nil {
		hashRow = exactWTOCFGRowHash
	}
	solver := exactWTOSolver{
		ctx: ctx, graph: graph, arena: arena, tape: tape,
		transfer: transfer, branch: branch, options: options,
		hashRow: hashRow, buckets: make([]exactWTOBucket, len(tape.points)), trace: trace,
		// With no components, a row can never return to a drained bucket.
		// Prepared relation evaluation publishes only the exit, so intermediate
		// bucket rows can transfer ownership instead of being defensively cloned.
		transientDAG: !retainIntermediates && len(tape.components) == 0,
	}
	entry := tape.denseIndex(graph.Entry())
	if entry < 0 {
		return nil, fmt.Errorf("transformer: exact WTO entry is unreachable")
	}
	// Retained execution gets the bucket's owned copy from insert. A transient
	// DAG transfers its seed directly, so it still needs one copy to preserve
	// the caller-owned initial row.
	seedRow := initial
	if solver.transientDAG {
		seedRow = cloneCFGRow(initial)
	}
	seed := exactWTORow{row: seedRow, phases: make([]uint8, len(tape.components))}
	if _, err := solver.insert(uint32(entry), seed); err != nil {
		return nil, err
	}
	if err := solver.solveSequence(0, uint32(len(tape.points)), -1); err != nil {
		return nil, err
	}
	return &solver, nil
}

// certifyExactWTOComponentHeads gives phase zero/one-plus an unambiguous
// structural meaning. Broader SCC shapes remain on the contextual fallback
// until they have an exact iteration algebra of their own.
func certifyExactWTOComponentHeads(graph cfg.Graph, tape *symbolicWTOTape) error {
	for component, item := range tape.components {
		head := tape.points[item.head].point
		edges := tape.edges[tape.points[item.head].edgeBegin:tape.points[item.head].edgeEnd]
		if !graph.IsBranch(head) || len(edges) != 2 {
			return fmt.Errorf("transformer: exact WTO component head %d is not a two-way zero-vs-1+ branch", head)
		}
		inside, outside := 0, 0
		for _, edge := range edges {
			if tape.componentContains(int32(component), edge.to) && edge.to != item.head && edge.kind != symbolicWTOEdgeBackedge {
				inside++
			} else if !tape.componentContains(int32(component), edge.to) {
				outside++
			}
		}
		if inside != 1 || outside != 1 {
			return fmt.Errorf("transformer: exact WTO component head %d has %d body and %d exit successors, want one each", head, inside, outside)
		}
	}
	return nil
}

type exactWTOSolver struct {
	ctx          context.Context
	graph        cfg.Graph
	arena        *Arena
	tape         *symbolicWTOTape
	transfer     SymbolicCFGExpandTransfer
	branch       SymbolicCFGBranch
	options      SymbolicExactWTOOptions
	hashRow      func(SymbolicCFGRow) uint64
	buckets      []exactWTOBucket
	passes       int
	transientDAG bool
	trace        *sparseProjectionTraceBuilder
}

// solveSequence recursively follows the WTO component nesting. Each component
// is drained in semi-naive passes: bucket cursors ensure already transferred
// row/phase states are never evaluated again.
func (s *exactWTOSolver) solveSequence(begin, end uint32, parent int32) error {
	for point := begin; point < end; {
		headed := s.tape.points[point].headComponent
		if headed >= 0 && s.tape.components[headed].parent == parent {
			if err := s.solveComponent(headed); err != nil {
				return err
			}
			point = s.tape.components[headed].end
			continue
		}
		if err := s.processDelta(point); err != nil {
			return err
		}
		point++
	}
	return nil
}

func (s *exactWTOSolver) solveComponent(component int32) error {
	c := s.tape.components[component]
	for s.hasPending(c.begin, c.end) {
		if err := s.checkContext(); err != nil {
			return err
		}
		s.passes++
		if s.passes > s.options.MaxIterations {
			return fmt.Errorf("transformer: exact WTO iteration budget at component head %d", s.tape.points[c.head].point)
		}
		if err := s.processDelta(c.head); err != nil {
			return err
		}
		if err := s.solveSequence(c.head+1, c.end, component); err != nil {
			return err
		}
	}
	return nil
}

func (s *exactWTOSolver) hasPending(begin, end uint32) bool {
	for i := begin; i < end; i++ {
		if s.buckets[i].cursor < len(s.buckets[i].rows) {
			return true
		}
	}
	return false
}

func (s *exactWTOSolver) processDelta(dense uint32) error {
	bucket := &s.buckets[dense]
	limit := len(bucket.rows)
	for bucket.cursor < limit {
		if err := s.checkContext(); err != nil {
			return err
		}
		candidate := bucket.rows[bucket.cursor]
		// Bucket rows are immutable after insertion. Transfer receives its own
		// mutable row below; phases are copied only when an outgoing state is
		// formed, so borrowing the candidate cannot expose bucket storage.
		bucket.cursor++
		point := s.tape.points[dense].point
		if s.trace != nil {
			s.trace.pointInput(point, candidate.row)
		}
		input := candidate.row
		if !s.transientDAG {
			input = cloneCFGRow(candidate.row)
		}
		produced := []SymbolicCFGRow{input}
		var err error
		if s.transfer != nil {
			produced, err = s.transfer(point, input)
			if err != nil {
				return fmt.Errorf("transformer: exact WTO point %d: %w", point, err)
			}
		}
		produced = dedupCFGRows(s.arena, produced)
		for _, row := range produced {
			if !validCFGRow(s.arena, s.options.Shape, row) {
				return fmt.Errorf("transformer: exact WTO point %d produced an invalid row", point)
			}
			if s.trace != nil {
				s.trace.pointOutput(point, row)
			}
			if err := s.emitSuccessors(dense, exactWTORow{row: row, phases: append([]uint8(nil), candidate.phases...)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *exactWTOSolver) emitSuccessors(dense uint32, out exactWTORow) error {
	point := s.tape.points[dense].point
	edges := s.tape.edges[s.tape.points[dense].edgeBegin:s.tape.points[dense].edgeEnd]
	if len(edges) == 0 {
		return nil
	}
	if s.graph.IsBranch(point) {
		if s.branch == nil || len(edges) != 2 {
			return fmt.Errorf("transformer: exact WTO branch %d has no exact branch algebra", point)
		}
		for _, edge := range edges {
			to := s.tape.points[edge.to].point
			cond, ok := s.graph.EdgeCond(point, to)
			if !ok {
				return fmt.Errorf("transformer: exact WTO branch %d edge polarity missing", point)
			}
			// Each branch callback gets one independent mutable row. insert owns
			// the returned state, so cloning next.row before the callback as well
			// would be a redundant second deep copy.
			next := exactWTORow{phases: append([]uint8(nil), out.phases...)}
			row, edgeGuard, err := s.branch(point, cloneCFGRow(out.row), cond)
			if err != nil {
				return fmt.Errorf("transformer: exact WTO branch %d edge %t: %w", point, cond, err)
			}
			if row.Guard != out.row.Guard || !validCFGRow(s.arena, s.options.Shape, row) || !s.arena.validGuard(edgeGuard, s.options.Shape) {
				return fmt.Errorf("transformer: exact WTO branch %d edge %t produced an invalid row", point, cond)
			}
			row.Guard = s.arena.And(out.row.Guard, edgeGuard)
			if row.Guard == s.arena.False() {
				continue
			}
			if s.trace != nil {
				s.trace.normalEdge(point, to, row.Guard)
			}
			next.row = row
			if err := s.emitEdge(edge, next); err != nil {
				return err
			}
		}
		return nil
	}
	if len(edges) != 1 {
		return fmt.Errorf("transformer: exact WTO non-branch point %d has %d successors", point, len(edges))
	}
	// There is only one consumer. emitEdge/insert establishes bucket ownership
	// for retained cyclic execution; the transient DAG transfers ownership.
	return s.emitEdge(edges[0], out)
}

func (s *exactWTOSolver) emitEdge(edge symbolicWTOTapeEdge, row exactWTORow) error {
	// Ordered effects cannot be represented after an unbounded number of laps.
	// Reject at the recurrence boundary before the row becomes visible.
	if edge.kind == symbolicWTOEdgeBackedge && len(row.row.Effects) != 0 {
		return fmt.Errorf("transformer: exact WTO recurrent effects require an exact closure algebra")
	}
	s.applyPhaseBoundary(edge, row.phases)
	_, err := s.insert(edge.to, row)
	return err
}

func (s *exactWTOSolver) applyPhaseBoundary(edge symbolicWTOTapeEdge, phases []uint8) {
	fromComponent := s.tape.points[edge.from].component
	for count, component := edge.exitCount, fromComponent; count > 0 && component >= 0; count-- {
		phases[component] = exactWTOPhaseOutside
		component = s.tape.components[component].parent
	}
	// Enter outer-to-inner so a transition cannot retain a stale nested phase.
	if edge.enterCount > 0 {
		chain := make([]int32, 0, edge.enterCount)
		for count, component := edge.enterCount, s.tape.points[edge.to].component; count > 0 && component >= 0; count-- {
			chain = append(chain, component)
			component = s.tape.components[component].parent
		}
		for i := len(chain) - 1; i >= 0; i-- {
			phases[chain[i]] = exactWTOPhaseZero
		}
	}
	// A loop becomes 1+ on its certified head-to-body edge, including an
	// edge whose destination is the head of a nested component.
	if headed := s.tape.points[edge.from].headComponent; headed >= 0 &&
		edge.kind != symbolicWTOEdgeBackedge && s.tape.componentContains(headed, edge.to) && edge.to != s.tape.components[headed].head {
		phases[headed] = exactWTOPhaseOnePlus
	}
}

func (s *exactWTOSolver) insert(point uint32, row exactWTORow) (bool, error) {
	if err := s.checkContext(); err != nil {
		return false, err
	}
	bucket := &s.buckets[point]
	hash := s.hashRow(row.row) ^ exactWTOPhaseHash(row.phases)
	for i, candidateHash := range bucket.hashes {
		if candidateHash == hash && equalExactWTORow(s.arena, bucket.rows[i], row) {
			bucket.rows[i].row.Observations = unionObservationTerms(s.arena, bucket.rows[i].row.Observations, row.row.Observations)
			bucket.rows[i].row.observationObligations = unionobservationObligations(bucket.rows[i].row.observationObligations, row.row.observationObligations)
			return false, nil
		}
	}
	if !s.transientDAG {
		row = cloneExactWTORow(row)
	}
	bucket.rows = append(bucket.rows, row)
	bucket.hashes = append(bucket.hashes, hash)
	return true, nil
}

func (s *exactWTOSolver) checkContext() error {
	if err := s.ctx.Err(); err != nil {
		return fmt.Errorf("transformer: exact WTO canceled: %w", err)
	}
	return nil
}

func (s *exactWTOSolver) publish() (map[cfg.Point][]SymbolicCFGRow, error) {
	out := make(map[cfg.Point][]SymbolicCFGRow, len(s.buckets))
	for dense := range s.buckets {
		if err := s.checkContext(); err != nil {
			return nil, err
		}
		rows, err := s.publishBucket(uint32(dense))
		if err != nil {
			return nil, err
		}
		if len(rows) != 0 {
			out[s.tape.points[dense].point] = rows
		}
	}
	return out, nil
}

func (s *exactWTOSolver) publishBucket(dense uint32) ([]SymbolicCFGRow, error) {
	if dense >= uint32(len(s.buckets)) {
		return nil, fmt.Errorf("transformer: exact WTO result bucket %d is out of range", dense)
	}
	if err := s.checkContext(); err != nil {
		return nil, err
	}
	rows := make([]SymbolicCFGRow, 0, len(s.buckets[dense].rows))
	for _, candidate := range s.buckets[dense].rows {
		duplicate := false
		for i := range rows {
			if equalCFGRow(s.arena, rows[i], candidate.row) {
				rows[i].Observations = unionObservationTerms(s.arena, rows[i].Observations, candidate.row.Observations)
				rows[i].observationObligations = unionobservationObligations(rows[i].observationObligations, candidate.row.observationObligations)
				duplicate = true
				break
			}
		}
		if !duplicate {
			rows = append(rows, cloneCFGRow(candidate.row))
		}
	}
	return rows, nil
}

func cloneExactWTORow(row exactWTORow) exactWTORow {
	return exactWTORow{row: cloneCFGRow(row.row), phases: append([]uint8(nil), row.phases...)}
}

func equalExactWTORow(arena *Arena, left, right exactWTORow) bool {
	if len(left.phases) != len(right.phases) || !equalCFGRow(arena, left.row, right.row) {
		return false
	}
	for i := range left.phases {
		if left.phases[i] != right.phases[i] {
			return false
		}
	}
	return true
}

// exactWTOCFGRowHash is deliberately only a fingerprint. It may omit fields;
// exact equality is always checked on collisions. Equal rows must hash alike.
func exactWTOCFGRowHash(row SymbolicCFGRow) uint64 {
	h := uint64(1469598103934665603)
	mix := func(value uint64) { h = (h ^ value) * 1099511628211 }
	mix(uint64(row.Guard))
	keys := make([]int, 0, len(row.Values))
	for key := range row.Values {
		keys = append(keys, int(key))
	}
	sort.Ints(keys)
	for _, key := range keys {
		mix(uint64(key))
		mix(uint64(row.Values[symbol.ID(key)]))
	}
	mix(uint64(len(row.Operations)))
	mix(uint64(len(row.Effects)))
	mix(uint64(len(row.Proofs)))
	mix(uint64(len(row.genericBindings)))
	if row.paramPreserved.tracked {
		mix(1)
	}
	mix(uint64(row.paramPreserved.boundaryParams))
	for _, word := range row.paramPreserved.words {
		mix(word)
	}
	return h
}

func exactWTOPhaseHash(phases []uint8) uint64 {
	h := uint64(1099511628211)
	for _, phase := range phases {
		h = (h ^ uint64(phase)) * 1469598103934665603
	}
	return h
}
