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
