package readmodel

import (
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ForEachDeadAssignment visits writes whose assigned value is discarded before
// any reachable read on every path.
func (r Reader) ForEachDeadAssignment(visit func(DeadAssignment) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	graph := r.result.Graph()
	view := r.newDeadAssignmentView(graph)
	if len(view.writes) == 0 {
		return false
	}
	bySymbol := make(map[symbol.ID][]readmodelDeadAssignmentWrite)
	for _, write := range view.writes {
		bySymbol[write.symbol] = append(bySymbol[write.symbol], write)
	}
	var items []DeadAssignment
	for _, writes := range bySymbol {
		items = append(items, view.deadAssignmentsForSymbol(writes)...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].WriteSpan.StartLine != items[j].WriteSpan.StartLine {
			return items[i].WriteSpan.StartLine < items[j].WriteSpan.StartLine
		}
		if items[i].WriteSpan.StartCol != items[j].WriteSpan.StartCol {
			return items[i].WriteSpan.StartCol < items[j].WriteSpan.StartCol
		}
		return items[i].Name < items[j].Name
	})
	for _, item := range items {
		if !visit(item) {
			return true
		}
	}
	return len(items) > 0
}

type readmodelDeadAssignmentWrite struct {
	point     cfg.Point
	statement body.StatementIdentity
	symbol    symbol.ID
	name      string
	write     SourceSpan
}

type readmodelDeadAssignmentExit struct {
	point cfg.Point
	span  SourceSpan
}

type readmodelDeadAssignmentView struct {
	reader        Reader
	graph         cfg.Graph
	writes        []readmodelDeadAssignmentWrite
	writesByPoint map[cfg.Point][]readmodelDeadAssignmentWrite
	readsByPoint  map[cfg.Point]map[symbol.ID]struct{}
	exitsByPoint  map[cfg.Point]readmodelDeadAssignmentExit
}

func (r Reader) newDeadAssignmentView(graph cfg.Graph) readmodelDeadAssignmentView {
	writes := r.deadAssignmentWrites(graph)
	view := readmodelDeadAssignmentView{
		reader:        r,
		graph:         graph,
		writes:        writes,
		writesByPoint: make(map[cfg.Point][]readmodelDeadAssignmentWrite),
		readsByPoint:  r.result.ReachableSymbolReads(),
		exitsByPoint:  r.deadAssignmentExits(),
	}
	for _, write := range writes {
		view.writesByPoint[write.point] = append(view.writesByPoint[write.point], write)
	}
	return view
}

func (r Reader) deadAssignmentWrites(graph cfg.Graph) []readmodelDeadAssignmentWrite {
	var writes []readmodelDeadAssignmentWrite
	for _, occ := range r.result.DeadAssignmentWriteOccurrences() {
		if !r.deadAssignmentSymbolKind(occ.Symbol) {
			continue
		}
		writes = append(writes, readmodelDeadAssignmentWrite{
			point:     occ.Point,
			statement: occ.Statement,
			symbol:    occ.Symbol,
			name:      occ.Name,
			write:     sourceSpanFromBody(occ.Span),
		})
	}
	return writes
}

func (r Reader) deadAssignmentSymbolKind(id symbol.ID) bool {
	kind, ok := r.result.SymbolKind(id)
	return ok && (kind == symbol.Local || kind == symbol.Param)
}

func (r Reader) deadAssignmentExits() map[cfg.Point]readmodelDeadAssignmentExit {
	out := make(map[cfg.Point]readmodelDeadAssignmentExit)
	for point, occ := range r.result.DeadAssignmentExitOccurrences() {
		out[point] = readmodelDeadAssignmentExit{point: occ.Point, span: sourceSpanFromBody(occ.Span)}
	}
	return out
}

func (v readmodelDeadAssignmentView) deadAssignmentsForSymbol(writes []readmodelDeadAssignmentWrite) []DeadAssignment {
	var out []DeadAssignment
	for _, previous := range writes {
		if readmodelAmbiguousSameStatementWrite(previous, writes) {
			continue
		}
		overwrites, exits, ok := v.firstOverwritesBeforeRead(previous, writes)
		if !ok {
			continue
		}
		item := DeadAssignment{
			Point:     previous.point,
			Name:      previous.name,
			Key:       strconv.Itoa(int(previous.symbol)),
			WriteSpan: previous.write,
		}
		for _, overwrite := range overwrites {
			item.Overwrites = append(item.Overwrites, DeadAssignmentOverwrite{Point: overwrite.point, Span: overwrite.write})
		}
		for _, exit := range exits {
			item.Exits = append(item.Exits, DeadAssignmentExit{Point: exit.point, Span: exit.span})
		}
		out = append(out, item)
	}
	return out
}

func readmodelAmbiguousSameStatementWrite(write readmodelDeadAssignmentWrite, writes []readmodelDeadAssignmentWrite) bool {
	if write.statement == 0 {
		return false
	}
	for _, other := range writes {
		if other.point != write.point && other.statement == write.statement {
			return true
		}
	}
	return false
}

type readmodelDeadAssignmentProof struct {
	ok         bool
	frontier   map[cfg.Point]readmodelDeadAssignmentWrite
	exitPoints map[cfg.Point]readmodelDeadAssignmentExit
}

func (v readmodelDeadAssignmentView) firstOverwritesBeforeRead(previous readmodelDeadAssignmentWrite, writes []readmodelDeadAssignmentWrite) ([]readmodelDeadAssignmentWrite, []readmodelDeadAssignmentExit, bool) {
	if v.graph == nil {
		return nil, nil, false
	}
	successors := cfg.SuccessorsReadOnly(v.graph, previous.point)
	if len(successors) == 0 {
		return nil, nil, false
	}
	memo := make(map[cfg.Point]readmodelDeadAssignmentProof)
	visiting := make(map[cfg.Point]bool)
	var walk func(cfg.Point) readmodelDeadAssignmentProof
	walk = func(point cfg.Point) readmodelDeadAssignmentProof {
		if !v.pointReachable(point) {
			return readmodelDeadAssignmentProof{ok: true}
		}
		if v.pointReadsSymbol(point, previous.symbol) {
			return readmodelDeadAssignmentProof{}
		}
		if overwrite, ok := v.pointOverwrite(point, previous.symbol, previous.point, writes); ok {
			return readmodelDeadAssignmentProof{
				ok:       true,
				frontier: map[cfg.Point]readmodelDeadAssignmentWrite{overwrite.point: overwrite},
			}
		}
		if v.pointWritesSymbol(point, previous.symbol) {
			return readmodelDeadAssignmentProof{}
		}
		if exit, ok := v.pointExit(point); ok {
			return readmodelDeadAssignmentProof{
				ok:         true,
				exitPoints: map[cfg.Point]readmodelDeadAssignmentExit{exit.point: exit},
			}
		}
		if point == v.graph.Exit() {
			return readmodelDeadAssignmentProof{
				ok:         true,
				exitPoints: map[cfg.Point]readmodelDeadAssignmentExit{point: {point: point}},
			}
		}
		if cached, ok := memo[point]; ok {
			return cached
		}
		if visiting[point] {
			return readmodelDeadAssignmentProof{}
		}
		visiting[point] = true
		successors := cfg.SuccessorsReadOnly(v.graph, point)
		proof := readmodelDeadAssignmentProof{
			ok:         len(successors) > 0,
			frontier:   make(map[cfg.Point]readmodelDeadAssignmentWrite),
			exitPoints: make(map[cfg.Point]readmodelDeadAssignmentExit),
		}
		for _, succ := range successors {
			child := walk(succ)
			if !child.ok {
				proof = readmodelDeadAssignmentProof{}
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
			proof = readmodelDeadAssignmentProof{}
		}
		memo[point] = proof
		return proof
	}

	frontier := make(map[cfg.Point]readmodelDeadAssignmentWrite)
	exitPoints := make(map[cfg.Point]readmodelDeadAssignmentExit)
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
	overwrites := sortedReadmodelDeadAssignmentWrites(frontier)
	if len(overwrites) == 0 {
		return nil, nil, false
	}
	return overwrites, sortedReadmodelDeadAssignmentExits(exitPoints), true
}

func (v readmodelDeadAssignmentView) pointReachable(point cfg.Point) bool {
	return v.reader.result.PointNormallyReachable(point)
}

func (v readmodelDeadAssignmentView) pointReadsSymbol(point cfg.Point, id symbol.ID) bool {
	reads := v.readsByPoint[point]
	if len(reads) == 0 {
		return false
	}
	_, ok := reads[id]
	return ok
}

func (v readmodelDeadAssignmentView) pointExit(point cfg.Point) (readmodelDeadAssignmentExit, bool) {
	exit, ok := v.exitsByPoint[point]
	return exit, ok
}

func (v readmodelDeadAssignmentView) pointWritesSymbol(point cfg.Point, id symbol.ID) bool {
	for _, write := range v.writesByPoint[point] {
		if write.symbol == id {
			return true
		}
	}
	return false
}

func (v readmodelDeadAssignmentView) pointOverwrite(point cfg.Point, id symbol.ID, ignoredPoint cfg.Point, writes []readmodelDeadAssignmentWrite) (readmodelDeadAssignmentWrite, bool) {
	for _, write := range v.writesByPoint[point] {
		if write.point == ignoredPoint {
			continue
		}
		if write.symbol == id && !readmodelAmbiguousSameStatementWrite(write, writes) {
			return write, true
		}
	}
	return readmodelDeadAssignmentWrite{}, false
}

func sortedReadmodelDeadAssignmentWrites(frontier map[cfg.Point]readmodelDeadAssignmentWrite) []readmodelDeadAssignmentWrite {
	out := make([]readmodelDeadAssignmentWrite, 0, len(frontier))
	for _, write := range frontier {
		out = append(out, write)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].write.StartLine != out[j].write.StartLine {
			return out[i].write.StartLine < out[j].write.StartLine
		}
		if out[i].write.StartCol != out[j].write.StartCol {
			return out[i].write.StartCol < out[j].write.StartCol
		}
		return out[i].point < out[j].point
	})
	return out
}

func sortedReadmodelDeadAssignmentExits(exits map[cfg.Point]readmodelDeadAssignmentExit) []readmodelDeadAssignmentExit {
	out := make([]readmodelDeadAssignmentExit, 0, len(exits))
	for _, exit := range exits {
		out = append(out, exit)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].span, out[j].span
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
		return out[i].point < out[j].point
	})
	return out
}
