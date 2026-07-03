package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type deadAssignments producerContext

type deadAssignmentWrite struct {
	point  cfg.Point
	stmt   ast.Stmt
	symbol symbol.ID
	name   string
	write  diagnostic.Span
}

type deadAssignmentExit struct {
	point cfg.Point
	span  diagnostic.Span
}

type deadAssignmentView struct {
	graph         cfg.Graph
	reachable     map[cfg.Point]bool
	writes        []deadAssignmentWrite
	writesByPoint map[cfg.Point][]deadAssignmentWrite
	readsByPoint  map[cfg.Point]map[symbol.ID]struct{}
	exitsByPoint  map[cfg.Point]deadAssignmentExit
}

func (p deadAssignments) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	view := newDeadAssignmentView(result, graph, producerContext(p).guardEnvironments(result))
	if len(view.writes) == 0 {
		return nil
	}
	bySymbol := make(map[symbol.ID][]deadAssignmentWrite)
	for _, write := range view.writes {
		bySymbol[write.symbol] = append(bySymbol[write.symbol], write)
	}
	var out []diagnostic.Diagnostic
	for _, writes := range bySymbol {
		out = append(out, view.diagnosticsForSymbol(writes)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Span.StartLine != out[j].Span.StartLine {
			return out[i].Span.StartLine < out[j].Span.StartLine
		}
		if out[i].Span.StartCol != out[j].Span.StartCol {
			return out[i].Span.StartCol < out[j].Span.StartCol
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func newDeadAssignmentView(result *body.Result, graph cfg.Graph, envs map[cfg.Point]guardEnv) deadAssignmentView {
	reachable := collectDiagnosticReachability(result, graph, envs)
	writes := collectDeadAssignmentWrites(result, graph, reachable)
	view := deadAssignmentView{
		graph:         graph,
		reachable:     reachable,
		writes:        writes,
		writesByPoint: make(map[cfg.Point][]deadAssignmentWrite),
		readsByPoint:  collectReachableSymbolReads(result, graph, reachable),
		exitsByPoint:  collectDeadAssignmentExits(result, graph, reachable),
	}
	for _, write := range writes {
		view.writesByPoint[write.point] = append(view.writesByPoint[write.point], write)
	}
	return view
}

func collectDeadAssignmentWrites(result *body.Result, graph cfg.Graph, reachable map[cfg.Point]bool) []deadAssignmentWrite {
	var writes []deadAssignmentWrite
	for _, point := range cfg.RPOReadOnly(graph) {
		if !diagnosticPointReachable(reachable, point) {
			continue
		}
		if fact, ok := result.LocalAssignment(point); ok {
			write, ok := localDeadAssignmentWrite(result, point, fact)
			if ok {
				writes = append(writes, write)
			}
			continue
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			write, ok := ordinaryDeadAssignmentWrite(result, point, fact)
			if ok {
				writes = append(writes, write)
			}
		}
	}
	return writes
}

func localDeadAssignmentWrite(result *body.Result, point cfg.Point, fact semantics.LocalAssignmentFact) (deadAssignmentWrite, bool) {
	if !fact.HasSymbol || fact.Expr == nil || ignoredUnusedLocalName(fact.Name) {
		return deadAssignmentWrite{}, false
	}
	if !deadAssignmentSymbolKind(result, fact.Symbol) {
		return deadAssignmentWrite{}, false
	}
	write := localNameSpan(fact.Stmt, fact.Index, fact.Name)
	if !write.Valid() {
		return deadAssignmentWrite{}, false
	}
	return deadAssignmentWrite{
		point:  point,
		stmt:   fact.Stmt,
		symbol: fact.Symbol,
		name:   fact.Name,
		write:  write,
	}, true
}

func ordinaryDeadAssignmentWrite(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (deadAssignmentWrite, bool) {
	if !fact.HasSymbol || fact.Value == nil {
		return deadAssignmentWrite{}, false
	}
	ident, ok := fact.Target.(*ast.IdentExpr)
	if !ok || ignoredUnusedLocalName(ident.Value) {
		return deadAssignmentWrite{}, false
	}
	if !deadAssignmentSymbolKind(result, fact.Symbol) {
		return deadAssignmentWrite{}, false
	}
	write := ast.SpanOf(ident)
	if !write.Valid() {
		write = ast.SpanOf(fact.Target)
	}
	if !write.Valid() {
		return deadAssignmentWrite{}, false
	}
	return deadAssignmentWrite{
		point:  point,
		stmt:   fact.Stmt,
		symbol: fact.Symbol,
		name:   ident.Value,
		write:  write,
	}, true
}

func deadAssignmentSymbolKind(result *body.Result, id symbol.ID) bool {
	kind, ok := result.SymbolKind(id)
	return ok && (kind == symbol.Local || kind == symbol.Param)
}

func collectDeadAssignmentExits(result *body.Result, graph cfg.Graph, reachable map[cfg.Point]bool) map[cfg.Point]deadAssignmentExit {
	out := make(map[cfg.Point]deadAssignmentExit)
	if graph == nil {
		return out
	}
	exit := graph.Exit()
	for _, point := range graph.RPO() {
		if !diagnosticPointReachable(reachable, point) {
			continue
		}
		if point == exit {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, point)
		if len(successors) == 0 {
			continue
		}
		allExit := true
		for _, succ := range successors {
			if succ != exit {
				allExit = false
				break
			}
		}
		if !allExit {
			continue
		}
		var span diagnostic.Span
		if fact, ok := result.ReturnFact(point); ok {
			span = ast.SpanOf(fact.Stmt)
		}
		out[point] = deadAssignmentExit{point: point, span: span}
	}
	return out
}

func (v deadAssignmentView) diagnosticsForSymbol(writes []deadAssignmentWrite) []diagnostic.Diagnostic {
	var out []diagnostic.Diagnostic
	for _, previous := range writes {
		if ambiguousSameStatementWrite(previous, writes) {
			continue
		}
		overwrites, exits, ok := v.firstOverwritesBeforeRead(previous, writes)
		if ok {
			out = append(out, deadAssignmentDiagnostic(previous, overwrites, exits))
		}
	}
	return out
}

func ambiguousSameStatementWrite(write deadAssignmentWrite, writes []deadAssignmentWrite) bool {
	if write.stmt == nil {
		return false
	}
	for _, other := range writes {
		if other.point != write.point && other.stmt == write.stmt {
			return true
		}
	}
	return false
}

type deadAssignmentProof struct {
	ok         bool
	frontier   map[cfg.Point]deadAssignmentWrite
	exitPoints map[cfg.Point]deadAssignmentExit
}

func (v deadAssignmentView) firstOverwritesBeforeRead(previous deadAssignmentWrite, writes []deadAssignmentWrite) ([]deadAssignmentWrite, []deadAssignmentExit, bool) {
	if v.graph == nil {
		return nil, nil, false
	}
	successors := cfg.SuccessorsReadOnly(v.graph, previous.point)
	if len(successors) == 0 {
		return nil, nil, false
	}
	memo := make(map[cfg.Point]deadAssignmentProof)
	visiting := make(map[cfg.Point]bool)
	var walk func(cfg.Point) deadAssignmentProof
	walk = func(point cfg.Point) deadAssignmentProof {
		if !v.pointReachable(point) {
			return deadAssignmentProof{ok: true}
		}
		if v.pointReadsSymbol(point, previous.symbol) {
			return deadAssignmentProof{}
		}
		if overwrite, ok := v.pointOverwrite(point, previous.symbol, previous.point, writes); ok {
			return deadAssignmentProof{
				ok:       true,
				frontier: map[cfg.Point]deadAssignmentWrite{overwrite.point: overwrite},
			}
		}
		if v.pointWritesSymbol(point, previous.symbol) {
			return deadAssignmentProof{}
		}
		if exit, ok := v.pointExit(point); ok {
			return deadAssignmentProof{
				ok:         true,
				exitPoints: map[cfg.Point]deadAssignmentExit{exit.point: exit},
			}
		}
		if point == v.graph.Exit() {
			return deadAssignmentProof{
				ok:         true,
				exitPoints: map[cfg.Point]deadAssignmentExit{point: {point: point}},
			}
		}
		if cached, ok := memo[point]; ok {
			return cached
		}
		if visiting[point] {
			return deadAssignmentProof{}
		}
		visiting[point] = true
		successors := cfg.SuccessorsReadOnly(v.graph, point)
		proof := deadAssignmentProof{
			ok:         len(successors) > 0,
			frontier:   make(map[cfg.Point]deadAssignmentWrite),
			exitPoints: make(map[cfg.Point]deadAssignmentExit),
		}
		for _, succ := range successors {
			child := walk(succ)
			if !child.ok {
				proof = deadAssignmentProof{}
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
			proof = deadAssignmentProof{}
		}
		memo[point] = proof
		return proof
	}

	frontier := make(map[cfg.Point]deadAssignmentWrite)
	exitPoints := make(map[cfg.Point]deadAssignmentExit)
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

func (v deadAssignmentView) pointReachable(point cfg.Point) bool {
	return diagnosticPointReachable(v.reachable, point)
}

func sortedDeadAssignmentWrites(frontier map[cfg.Point]deadAssignmentWrite) []deadAssignmentWrite {
	out := make([]deadAssignmentWrite, 0, len(frontier))
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

func sortedDeadAssignmentExits(exits map[cfg.Point]deadAssignmentExit) []deadAssignmentExit {
	out := make([]deadAssignmentExit, 0, len(exits))
	for _, exit := range exits {
		out = append(out, exit)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].span, out[j].span
		if left.Valid() != right.Valid() {
			return left.Valid()
		}
		if left.Valid() {
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

func (v deadAssignmentView) pointReadsSymbol(point cfg.Point, id symbol.ID) bool {
	reads := v.readsByPoint[point]
	if len(reads) == 0 {
		return false
	}
	_, ok := reads[id]
	return ok
}

func (v deadAssignmentView) pointExit(point cfg.Point) (deadAssignmentExit, bool) {
	exit, ok := v.exitsByPoint[point]
	return exit, ok
}

func (v deadAssignmentView) pointWritesSymbol(point cfg.Point, id symbol.ID) bool {
	for _, write := range v.writesByPoint[point] {
		if write.symbol == id {
			return true
		}
	}
	return false
}

func (v deadAssignmentView) pointOverwrite(point cfg.Point, id symbol.ID, ignoredPoint cfg.Point, writes []deadAssignmentWrite) (deadAssignmentWrite, bool) {
	for _, write := range v.writesByPoint[point] {
		if write.point == ignoredPoint {
			continue
		}
		if write.symbol == id && !ambiguousSameStatementWrite(write, writes) {
			return write, true
		}
	}
	return deadAssignmentWrite{}, false
}

func localNameSpan(stmt *ast.LocalAssignStmt, index int, name string) diagnostic.Span {
	if stmt != nil && index >= 0 && index < len(stmt.NamePositions) {
		pos := stmt.NamePositions[index]
		if pos.Valid() {
			endLine, endCol := pos.EndLine, pos.EndColumn
			if endLine == 0 {
				endLine = pos.Line
			}
			if endCol == 0 {
				endCol = pos.Column + len(name)
			}
			return diagnostic.Span{
				StartLine: pos.Line,
				StartCol:  pos.Column,
				EndLine:   endLine,
				EndCol:    endCol,
			}
		}
	}
	return ast.SpanOf(stmt)
}

func deadAssignmentDiagnostic(previous deadAssignmentWrite, overwrites []deadAssignmentWrite, exits []deadAssignmentExit) diagnostic.Diagnostic {
	hasExit := len(exits) > 0
	message := deadAssignmentMessage(previous.name, hasExit)
	help := deadAssignmentHelp(previous.name, hasExit)
	var evidence []diagnostic.Evidence
	labels := []diagnostic.Label{sourceLabel(previous.write, labelDeadAssignment)}
	for _, overwrite := range overwrites {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    overwrite.write,
			Message: deadAssignmentOverwriteEvidence(previous.name),
		})
		labels = append(labels, sourceLabel(overwrite.write, labelOverwrite))
	}
	for _, exit := range exits {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    exit.span,
			Message: deadAssignmentExitEvidence(previous.name),
		})
		labels = append(labels, sourceLabel(exit.span, labelExitBeforeRead))
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		return diagnosticEvidenceSpanLess(evidence[i].Span, evidence[j].Span)
	})
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        previous.write,
		Code:        CodeDeadAssignment,
		Severity:    diagnostic.SeverityWarning,
		Message:     message,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        help,
		Labels:      labels,
	})
}

func diagnosticEvidenceSpanLess(left, right diagnostic.Span) bool {
	if left.Valid() != right.Valid() {
		return left.Valid()
	}
	if !left.Valid() {
		return false
	}
	if left.StartLine != right.StartLine {
		return left.StartLine < right.StartLine
	}
	if left.StartCol != right.StartCol {
		return left.StartCol < right.StartCol
	}
	if left.EndLine != right.EndLine {
		return left.EndLine < right.EndLine
	}
	return left.EndCol < right.EndCol
}
