package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DeadAssignmentProof proves that Write is overwritten before any reachable
// read on every path. Overwrites is the first overwrite frontier; Exits records
// paths that leave the function without a read.
type DeadAssignmentProof struct {
	Write      DeadAssignmentWriteOccurrence
	Overwrites []DeadAssignmentWriteOccurrence
	Exits      []DeadAssignmentExitOccurrence
}

type deadAssignmentProofState struct {
	ok         bool
	frontier   map[cfg.Point]DeadAssignmentWriteOccurrence
	exitPoints map[cfg.Point]DeadAssignmentExitOccurrence
}

type deadAssignmentProofView struct {
	result        *Result
	graph         cfg.Graph
	writes        []DeadAssignmentWriteOccurrence
	writesByPoint map[cfg.Point][]DeadAssignmentWriteOccurrence
	readsByPoint  SymbolReadSets
	exitsByPoint  map[cfg.Point]DeadAssignmentExitOccurrence
}

// DeadAssignmentProofs returns deterministic all-path overwrite proofs for
// local/param writes whose assigned value is discarded before any reachable
// read. It is the body-owned proof engine behind the dead-assignment judgment.
func (r *Result) DeadAssignmentProofs() []DeadAssignmentProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	view := r.newDeadAssignmentProofView(r.Graph())
	if len(view.writes) == 0 {
		return nil
	}
	bySymbol := make(map[symbol.ID][]DeadAssignmentWriteOccurrence)
	for _, write := range view.writes {
		bySymbol[write.Symbol] = append(bySymbol[write.Symbol], write)
	}
	var out []DeadAssignmentProof
	for _, writes := range bySymbol {
		out = append(out, view.deadAssignmentProofsForSymbol(writes)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Write.Span.StartLine != out[j].Write.Span.StartLine {
			return out[i].Write.Span.StartLine < out[j].Write.Span.StartLine
		}
		if out[i].Write.Span.StartCol != out[j].Write.Span.StartCol {
			return out[i].Write.Span.StartCol < out[j].Write.Span.StartCol
		}
		return out[i].Write.Name < out[j].Write.Name
	})
	return out
}

func (r *Result) newDeadAssignmentProofView(graph cfg.Graph) deadAssignmentProofView {
	writes := r.deadAssignmentProofWrites()
	view := deadAssignmentProofView{
		result:        r,
		graph:         graph,
		writes:        writes,
		writesByPoint: make(map[cfg.Point][]DeadAssignmentWriteOccurrence),
		readsByPoint:  r.ReachableSymbolReads(),
		exitsByPoint:  r.DeadAssignmentExitOccurrences(),
	}
	for _, write := range writes {
		view.writesByPoint[write.Point] = append(view.writesByPoint[write.Point], write)
	}
	return view
}

func (r *Result) deadAssignmentProofWrites() []DeadAssignmentWriteOccurrence {
	var writes []DeadAssignmentWriteOccurrence
	for _, occ := range r.DeadAssignmentWriteOccurrences() {
		if r.deadAssignmentSymbolKind(occ.Symbol) {
			writes = append(writes, occ)
		}
	}
	return writes
}

func (r *Result) deadAssignmentSymbolKind(id symbol.ID) bool {
	kind, ok := r.SymbolKind(id)
	return ok && (kind == symbol.Local || kind == symbol.Param)
}

func (v deadAssignmentProofView) deadAssignmentProofsForSymbol(writes []DeadAssignmentWriteOccurrence) []DeadAssignmentProof {
	var out []DeadAssignmentProof
	for _, previous := range writes {
		if ambiguousSameStatementWrite(previous, writes) {
			continue
		}
		overwrites, exits, ok := v.firstOverwritesBeforeRead(previous, writes)
		if !ok {
			continue
		}
		out = append(out, DeadAssignmentProof{
			Write:      previous,
			Overwrites: overwrites,
			Exits:      exits,
		})
	}
	return out
}

func (v deadAssignmentProofView) firstOverwritesBeforeRead(previous DeadAssignmentWriteOccurrence, writes []DeadAssignmentWriteOccurrence) ([]DeadAssignmentWriteOccurrence, []DeadAssignmentExitOccurrence, bool) {
	if v.graph == nil {
		return nil, nil, false
	}
	successors := cfg.SuccessorsReadOnly(v.graph, previous.Point)
	if len(successors) == 0 {
		return nil, nil, false
	}
	memo := make(map[cfg.Point]deadAssignmentProofState)
	visiting := make(map[cfg.Point]bool)
	var walk func(cfg.Point) deadAssignmentProofState
	walk = func(point cfg.Point) deadAssignmentProofState {
		if !v.pointReachable(point) {
			return deadAssignmentProofState{ok: true}
		}
		if v.pointReadsSymbol(point, previous.Symbol) {
			return deadAssignmentProofState{}
		}
		if overwrite, ok := v.pointOverwrite(point, previous.Symbol, previous.Point, writes); ok {
			return deadAssignmentProofState{
				ok:       true,
				frontier: map[cfg.Point]DeadAssignmentWriteOccurrence{overwrite.Point: overwrite},
			}
		}
		if v.pointWritesSymbol(point, previous.Symbol) {
			return deadAssignmentProofState{}
		}
		if exit, ok := v.pointExit(point); ok {
			return deadAssignmentProofState{
				ok:         true,
				exitPoints: map[cfg.Point]DeadAssignmentExitOccurrence{exit.Point: exit},
			}
		}
		if point == v.graph.Exit() {
			return deadAssignmentProofState{
				ok:         true,
				exitPoints: map[cfg.Point]DeadAssignmentExitOccurrence{point: {Point: point}},
			}
		}
		if cached, ok := memo[point]; ok {
			return cached
		}
		if visiting[point] {
			return deadAssignmentProofState{}
		}
		visiting[point] = true
		successors := cfg.SuccessorsReadOnly(v.graph, point)
		proof := deadAssignmentProofState{
			ok:         len(successors) > 0,
			frontier:   make(map[cfg.Point]DeadAssignmentWriteOccurrence),
			exitPoints: make(map[cfg.Point]DeadAssignmentExitOccurrence),
		}
		for _, succ := range successors {
			child := walk(succ)
			if !child.ok {
				proof = deadAssignmentProofState{}
				break
			}
			for point, overwrite := range child.frontier {
				proof.frontier[point] = overwrite
			}
			for point, exit := range child.exitPoints {
				proof.exitPoints[point] = exit
			}
		}
		delete(visiting, point)
		if proof.ok && len(proof.frontier) == 0 && len(proof.exitPoints) == 0 {
			proof = deadAssignmentProofState{}
		}
		memo[point] = proof
		return proof
	}

	frontier := make(map[cfg.Point]DeadAssignmentWriteOccurrence)
	exitPoints := make(map[cfg.Point]DeadAssignmentExitOccurrence)
	for _, succ := range successors {
		child := walk(succ)
		if !child.ok {
			return nil, nil, false
		}
		for point, overwrite := range child.frontier {
			frontier[point] = overwrite
		}
		for point, exit := range child.exitPoints {
			exitPoints[point] = exit
		}
	}
	overwrites := sortedDeadAssignmentWrites(frontier)
	if len(overwrites) == 0 {
		return nil, nil, false
	}
	return overwrites, sortedDeadAssignmentExits(exitPoints), true
}

func (v deadAssignmentProofView) pointReachable(point cfg.Point) bool {
	return v.result.PointNormallyReachable(point)
}

func (v deadAssignmentProofView) pointReadsSymbol(point cfg.Point, id symbol.ID) bool {
	reads := v.readsByPoint[point]
	if len(reads) == 0 {
		return false
	}
	_, ok := reads[id]
	return ok
}

func (v deadAssignmentProofView) pointExit(point cfg.Point) (DeadAssignmentExitOccurrence, bool) {
	exit, ok := v.exitsByPoint[point]
	return exit, ok
}

func (v deadAssignmentProofView) pointWritesSymbol(point cfg.Point, id symbol.ID) bool {
	for _, write := range v.writesByPoint[point] {
		if write.Symbol == id {
			return true
		}
	}
	return false
}

func (v deadAssignmentProofView) pointOverwrite(point cfg.Point, id symbol.ID, ignoredPoint cfg.Point, writes []DeadAssignmentWriteOccurrence) (DeadAssignmentWriteOccurrence, bool) {
	for _, write := range v.writesByPoint[point] {
		if write.Point == ignoredPoint {
			continue
		}
		if write.Symbol == id && !ambiguousSameStatementWrite(write, writes) {
			return write, true
		}
	}
	return DeadAssignmentWriteOccurrence{}, false
}

func ambiguousSameStatementWrite(write DeadAssignmentWriteOccurrence, writes []DeadAssignmentWriteOccurrence) bool {
	if write.Statement == 0 {
		return false
	}
	for _, other := range writes {
		if other.Point != write.Point && other.Statement == write.Statement {
			return true
		}
	}
	return false
}

func sortedDeadAssignmentWrites(frontier map[cfg.Point]DeadAssignmentWriteOccurrence) []DeadAssignmentWriteOccurrence {
	out := make([]DeadAssignmentWriteOccurrence, 0, len(frontier))
	for _, write := range frontier {
		out = append(out, write)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Span.StartLine != out[j].Span.StartLine {
			return out[i].Span.StartLine < out[j].Span.StartLine
		}
		if out[i].Span.StartCol != out[j].Span.StartCol {
			return out[i].Span.StartCol < out[j].Span.StartCol
		}
		return out[i].Point < out[j].Point
	})
	return out
}

func sortedDeadAssignmentExits(exits map[cfg.Point]DeadAssignmentExitOccurrence) []DeadAssignmentExitOccurrence {
	out := make([]DeadAssignmentExitOccurrence, 0, len(exits))
	for _, exit := range exits {
		out = append(out, exit)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].Span, out[j].Span
		if sourceSpanValid(left) != sourceSpanValid(right) {
			return sourceSpanValid(left)
		}
		if sourceSpanValid(left) {
			if left.StartLine != right.StartLine {
				return left.StartLine < right.StartLine
			}
			if left.StartCol != right.StartCol {
				return left.StartCol < right.StartCol
			}
		}
		return out[i].Point < out[j].Point
	})
	return out
}
