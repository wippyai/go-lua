package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	semanticreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// semanticQuerySnapshot contains only immutable projection DTOs and source
// bytes. It is built after the solve, before publication; query reads never
// touch mutable checker worklists or run another analysis.
type semanticQuerySnapshot struct {
	entryFile string
	sources   map[string][]byte
	binders   []BinderInfo
	bodies    []queryBody
	symbols   []DocumentSymbol
	calls     []BodyCallRelations
	anchors   []anchoredSubject
	repairs   []RepairAction
	exprs     []expressionAt
}

type queryBody struct {
	id       BodyID
	location SourceLocation
}

type expressionAt struct {
	body     BodyID
	location SourceLocation
	display  string
}

type anchoredSubject struct {
	location SourceLocation
	anchor   judgment.SubjectAnchor
}

type semanticProjectionBuilder struct {
	file      string
	source    []byte
	root      *body.Result
	bodies    []projectionBody
	byFunc    map[*ast.FunctionExpr]projectionBody
	binders   map[uint64]*BinderInfo
	anchors   []anchoredSubject
	exprs     []expressionAt
	seenExprs map[ast.Expr]map[cfg.Point]struct{}
}

type projectionBody struct {
	id       BodyID
	result   *body.Result
	function *ast.FunctionExpr
	location SourceLocation
}

func projectSemanticQueries(input UnitInput, stmts []ast.Stmt, root *body.Result, judgments []judgment.Judgment) *semanticQuerySnapshot {
	entryFile := documentLabel(input, input.EntryDocument)
	entrySource := input.Sources[input.EntryDocument]
	b := semanticProjectionBuilder{
		file:      entryFile,
		source:    append([]byte(nil), entrySource.Content...),
		root:      root,
		byFunc:    make(map[*ast.FunctionExpr]projectionBody),
		binders:   make(map[uint64]*BinderInfo),
		seenExprs: make(map[ast.Expr]map[cfg.Point]struct{}),
	}
	b.collectBodies(root, BodyID("root"), SourceLocation{File: entryFile, Span: wholeSourceSpan(b.source)})
	b.collectBinderDefinitionsAndOccurrences(stmts)
	b.collectExpressions()

	result := &semanticQuerySnapshot{
		entryFile: entryFile,
		sources: map[string][]byte{
			entryFile: append([]byte(nil), entrySource.Content...),
		},
		bodies:  make([]queryBody, 0, len(b.bodies)),
		anchors: anchorsFromJudgments(entryFile, judgments),
		repairs: repairActionsFromJudgments(entryFile, judgments),
	}
	for document, snapshot := range input.Sources {
		result.sources[documentLabel(input, document)] = append([]byte(nil), snapshot.Content...)
	}
	for _, item := range b.bodies {
		result.bodies = append(result.bodies, queryBody{id: item.id, location: item.location})
	}
	result.binders = b.sortedBinders()
	result.symbols = b.documentSymbols(stmts)
	result.calls = b.callRelations()
	result.anchors = append(result.anchors, b.anchors...)
	// Expressions are held in body order and source order, which makes position
	// tie-breaking independent of map iteration.
	result.exprs = append([]expressionAt(nil), b.exprs...)
	return result
}

func (b *semanticProjectionBuilder) collectBodies(result *body.Result, id BodyID, location SourceLocation) {
	if result == nil {
		return
	}
	item := projectionBody{id: id, result: result, function: result.Function(), location: location}
	if item.function != nil {
		item.location = b.locationForSpan(ast.SpanOf(item.function))
		b.byFunc[item.function] = item
	}
	b.bodies = append(b.bodies, item)
	for index, child := range result.FunctionResults() {
		b.collectBodies(child, BodyID(fmt.Sprintf("%s/%d", id, index)), SourceLocation{})
	}
}

func (b *semanticProjectionBuilder) collectBinderDefinitionsAndOccurrences(stmts []ast.Stmt) {
	captures := make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
	for _, item := range b.bodies {
		if item.function == nil {
			continue
		}
		set := make(map[symbol.ID]struct{})
		for _, capture := range b.root.DirectCaptures(item.function) {
			if capture.Captured != 0 {
				set[capture.Captured] = struct{}{}
			}
		}
		captures[item.function] = set
	}

	var walkStmts func([]ast.Stmt, *ast.FunctionExpr)
	var walkExpr func(ast.Expr, *ast.FunctionExpr, BinderOccurrenceRole)
	addOccurrence := func(ident *ast.IdentExpr, fn *ast.FunctionExpr, role BinderOccurrenceRole) {
		if ident == nil || b.root == nil {
			return
		}
		id, ok := b.root.SymbolOfIdent(ident)
		if !ok || id == 0 {
			return
		}
		if fn != nil {
			if _, captured := captures[fn][id]; captured {
				role = BinderCapture
			}
		}
		info := b.ensureBinder(id)
		loc := b.locationForSpan(ast.SpanOf(ident))
		if !loc.Valid() || occurrenceAlreadyPresent(info.Occurrences, role, loc) {
			return
		}
		info.Occurrences = append(info.Occurrences, BinderOccurrence{Role: role, Location: loc})
	}

	walkExpr = func(expr ast.Expr, fn *ast.FunctionExpr, role BinderOccurrenceRole) {
		switch expr := expr.(type) {
		case nil:
			return
		case *ast.IdentExpr:
			addOccurrence(expr, fn, role)
		case *ast.AttrGetExpr:
			walkExpr(expr.Object, fn, BinderRead)
			walkExpr(expr.Key, fn, BinderRead)
		case *ast.TableExpr:
			for _, field := range expr.Fields {
				if field == nil {
					continue
				}
				walkExpr(field.Key, fn, BinderRead)
				walkExpr(field.Value, fn, BinderRead)
			}
		case *ast.FuncCallExpr:
			walkExpr(expr.Func, fn, BinderRead)
			walkExpr(expr.Receiver, fn, BinderRead)
			for _, arg := range expr.Args {
				walkExpr(arg, fn, BinderRead)
			}
		case *ast.LogicalOpExpr:
			walkExpr(expr.Lhs, fn, BinderRead)
			walkExpr(expr.Rhs, fn, BinderRead)
		case *ast.RelationalOpExpr:
			walkExpr(expr.Lhs, fn, BinderRead)
			walkExpr(expr.Rhs, fn, BinderRead)
		case *ast.StringConcatOpExpr:
			walkExpr(expr.Lhs, fn, BinderRead)
			walkExpr(expr.Rhs, fn, BinderRead)
		case *ast.ArithmeticOpExpr:
			walkExpr(expr.Lhs, fn, BinderRead)
			walkExpr(expr.Rhs, fn, BinderRead)
		case *ast.UnaryMinusOpExpr:
			walkExpr(expr.Expr, fn, BinderRead)
		case *ast.UnaryNotOpExpr:
			walkExpr(expr.Expr, fn, BinderRead)
		case *ast.UnaryLenOpExpr:
			walkExpr(expr.Expr, fn, BinderRead)
		case *ast.UnaryBNotOpExpr:
			walkExpr(expr.Expr, fn, BinderRead)
		case *ast.CastExpr:
			walkExpr(expr.Expr, fn, BinderRead)
		case *ast.NonNilAssertExpr:
			walkExpr(expr.Expr, fn, BinderRead)
		case *ast.FunctionExpr:
			b.defineFunction(expr)
			walkStmts(expr.Stmts, expr)
		}
	}

	walkStmts = func(items []ast.Stmt, fn *ast.FunctionExpr) {
		for _, stmt := range items {
			switch stmt := stmt.(type) {
			case *ast.AssignStmt:
				for _, lhs := range stmt.Lhs {
					walkExpr(lhs, fn, BinderWrite)
				}
				for _, rhs := range stmt.Rhs {
					walkExpr(rhs, fn, BinderRead)
				}
			case *ast.LocalAssignStmt:
				for index, id := range b.root.LocalSymbols(stmt) {
					b.defineBinder(id, stmt.Names[index], b.locationForPosition(namePosition(stmt.NamePositions, index), stmt.Names[index]))
				}
				for _, expr := range stmt.Exprs {
					walkExpr(expr, fn, BinderRead)
				}
			case *ast.FuncCallStmt:
				walkExpr(stmt.Expr, fn, BinderRead)
			case *ast.ReturnStmt:
				for _, expr := range stmt.Exprs {
					walkExpr(expr, fn, BinderRead)
				}
			case *ast.DoBlockStmt:
				walkStmts(stmt.Stmts, fn)
			case *ast.IfStmt:
				walkExpr(stmt.Condition, fn, BinderRead)
				walkStmts(stmt.Then, fn)
				walkStmts(stmt.Else, fn)
			case *ast.WhileStmt:
				walkExpr(stmt.Condition, fn, BinderRead)
				walkStmts(stmt.Stmts, fn)
			case *ast.RepeatStmt:
				walkStmts(stmt.Stmts, fn)
				walkExpr(stmt.Condition, fn, BinderRead)
			case *ast.NumberForStmt:
				if id, ok := b.root.NumForSymbol(stmt); ok {
					b.defineBinder(id, stmt.Name, b.locationForPosition(stmt.NamePosition, stmt.Name))
				}
				walkExpr(stmt.Init, fn, BinderRead)
				walkExpr(stmt.Limit, fn, BinderRead)
				walkExpr(stmt.Step, fn, BinderRead)
				walkStmts(stmt.Stmts, fn)
			case *ast.GenericForStmt:
				for index, id := range b.root.GenericForSymbols(stmt) {
					b.defineBinder(id, stmt.Names[index], b.locationForPosition(namePosition(stmt.NamePositions, index), stmt.Names[index]))
				}
				for _, expr := range stmt.Exprs {
					walkExpr(expr, fn, BinderRead)
				}
				walkStmts(stmt.Stmts, fn)
			case *ast.FuncDefStmt:
				if ident, ok := stmt.Name.Func.(*ast.IdentExpr); ok {
					if id, bound := b.root.SymbolOfIdent(ident); bound {
						b.defineBinder(id, ident.Value, b.locationForSpan(ast.SpanOf(ident)))
					}
				} else if stmt.Name != nil {
					walkExpr(stmt.Name.Func, fn, BinderWrite)
				}
				if stmt.Name != nil {
					walkExpr(stmt.Name.Receiver, fn, BinderRead)
				}
				if stmt.Func != nil {
					b.defineFunction(stmt.Func)
					walkStmts(stmt.Func.Stmts, stmt.Func)
				}
			}
		}
	}
	walkStmts(stmts, nil)
}

func (b *semanticProjectionBuilder) defineFunction(fn *ast.FunctionExpr) {
	if fn == nil || b.root == nil {
		return
	}
	id, ok := b.root.FunctionSymbol(fn)
	if !ok || id == 0 {
		return
	}
	name := "function"
	if origin, found := b.root.FunctionOrigin(fn); found && origin.HasTargetSymbol {
		if targetName := b.root.SymbolName(origin.TargetSymbol); targetName != "" {
			name = targetName
		}
	}
	b.defineBinder(id, name, b.functionDefinitionLocation(fn))
	for index, slot := range b.root.FunctionParamSlots(fn) {
		b.defineBinder(slot.Symbol, slot.Name, b.parameterLocation(fn, index, slot.Name))
	}
	if id, ok := b.root.VarargSymbol(fn); ok {
		b.defineBinder(id, "...", b.parameterLocation(fn, -1, "..."))
	}
}

func (b *semanticProjectionBuilder) defineBinder(id symbol.ID, name string, location SourceLocation) {
	if id == 0 {
		return
	}
	item := b.ensureBinder(id)
	if item.Name == "" && name != "" {
		item.Name = name
	}
	if !item.Definition.Valid() && location.Valid() {
		item.Definition = location
	}
}

func (b *semanticProjectionBuilder) ensureBinder(id symbol.ID) *BinderInfo {
	key := uint64(id)
	if item := b.binders[key]; item != nil {
		return item
	}
	item := &BinderInfo{SymbolID: key, Kind: BinderUnknown}
	for _, candidate := range b.bodies {
		if info, ok := candidate.result.WIRSymbolInfo(id); ok {
			item.Name = candidate.result.SymbolName(id)
			item.Kind = binderKindFromWIR(info.Kind)
			break
		}
	}
	if item.Name == "" {
		item.Name = b.root.SymbolName(id)
	}
	if item.Kind == BinderUnknown {
		if kind, ok := b.root.SymbolKind(id); ok {
			item.Kind = binderKindFromBind(kind)
		}
	}
	b.binders[key] = item
	return item
}

func (b *semanticProjectionBuilder) collectExpressions() {
	for _, item := range b.bodies {
		reader := semanticreadmodel.New(item.result)
		item.result.ForEachReachableExpressionUse(func(use body.ExpressionUse) bool {
			var walk func(ast.Expr)
			walk = func(expr ast.Expr) {
				if expr == nil {
					return
				}
				seen := b.seenExprs[expr]
				if seen == nil {
					seen = make(map[cfg.Point]struct{})
					b.seenExprs[expr] = seen
				}
				if _, exists := seen[use.Point]; !exists {
					seen[use.Point] = struct{}{}
					display := ""
					if t, ok := reader.ExpressionEvaluationType(body.ExpressionEvaluationFact{Point: use.Point, Expr: expr}); ok && t != nil {
						display = typeformat.Short(t)
					}
					b.exprs = append(b.exprs, expressionAt{body: item.id, location: b.locationForSpan(ast.SpanOf(expr)), display: display})
				}
				switch expr := expr.(type) {
				case *ast.AttrGetExpr:
					walk(expr.Object)
					walk(expr.Key)
				case *ast.TableExpr:
					for _, field := range expr.Fields {
						if field != nil {
							walk(field.Key)
							walk(field.Value)
						}
					}
				case *ast.FuncCallExpr:
					walk(expr.Func)
					walk(expr.Receiver)
					for _, arg := range expr.Args {
						walk(arg)
					}
				case *ast.LogicalOpExpr:
					walk(expr.Lhs)
					walk(expr.Rhs)
				case *ast.RelationalOpExpr:
					walk(expr.Lhs)
					walk(expr.Rhs)
				case *ast.StringConcatOpExpr:
					walk(expr.Lhs)
					walk(expr.Rhs)
				case *ast.ArithmeticOpExpr:
					walk(expr.Lhs)
					walk(expr.Rhs)
				case *ast.UnaryMinusOpExpr:
					walk(expr.Expr)
				case *ast.UnaryNotOpExpr:
					walk(expr.Expr)
				case *ast.UnaryLenOpExpr:
					walk(expr.Expr)
				case *ast.UnaryBNotOpExpr:
					walk(expr.Expr)
				case *ast.CastExpr:
					walk(expr.Expr)
				case *ast.NonNilAssertExpr:
					walk(expr.Expr)
				}
			}
			walk(use.Expr)
			return true
		})
	}
}

func (b *semanticProjectionBuilder) sortedBinders() []BinderInfo {
	out := make([]BinderInfo, 0, len(b.binders))
	for _, item := range b.binders {
		copy := *item
		copy.Occurrences = append([]BinderOccurrence(nil), item.Occurrences...)
		sort.SliceStable(copy.Occurrences, func(i, j int) bool { return locationLess(copy.Occurrences[i].Location, copy.Occurrences[j].Location) })
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SymbolID < out[j].SymbolID })
	return out
}

func (b *semanticProjectionBuilder) documentSymbols(stmts []ast.Stmt) []DocumentSymbol {
	byID := make(map[BodyID]*DocumentSymbol)
	for _, item := range b.bodies {
		if item.id == "root" || item.function == nil {
			continue
		}
		id, ok := b.root.FunctionSymbol(item.function)
		if !ok || id == 0 {
			continue
		}
		name := b.root.SymbolName(id)
		if origin, found := b.root.FunctionOrigin(item.function); found && origin.HasTargetSymbol && b.root.SymbolName(origin.TargetSymbol) != "" {
			name = b.root.SymbolName(origin.TargetSymbol)
		}
		if name == "" {
			name = "function"
		}
		byID[item.id] = &DocumentSymbol{Name: name, Kind: DocumentSymbolFunction, Anchor: fmt.Sprintf("function:%d", id), Location: item.location}
	}
	functionRoots := make([]DocumentSymbol, 0, len(byID))
	ids := make([]BodyID, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] }) // children before parents
	for _, id := range ids {
		node := byID[id]
		parent := parentBodyID(id)
		if parentNode := byID[parent]; parentNode != nil {
			parentNode.Children = append(parentNode.Children, *node)
			continue
		}
		functionRoots = append(functionRoots, *node)
	}
	sort.SliceStable(functionRoots, func(i, j int) bool { return locationLess(functionRoots[i].Location, functionRoots[j].Location) })
	var out []DocumentSymbol
	out = append(out, functionRoots...)
	fieldOrdinal := 0
	for _, stmt := range stmts {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Exprs) == 0 {
			continue
		}
		table, ok := ret.Exprs[0].(*ast.TableExpr)
		if !ok {
			continue
		}
		for _, field := range table.Fields {
			if field == nil {
				continue
			}
			name := ast.KeyName(field.Key)
			if name == "" {
				continue
			}
			fieldOrdinal++
			location := b.locationForSpan(ast.SpanOf(field.Key))
			if !location.Valid() {
				location = b.locationForSpan(ast.SpanOf(field.Value))
			}
			out = append(out, DocumentSymbol{Name: name, Kind: DocumentSymbolModuleField, Anchor: fmt.Sprintf("module-field:%s:%d", name, fieldOrdinal), Location: location})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return locationLess(out[i].Location, out[j].Location) })
	return out
}

func parentBodyID(id BodyID) BodyID {
	text := string(id)
	index := strings.LastIndexByte(text, '/')
	if index < 0 {
		return ""
	}
	return BodyID(text[:index])
}

func (b *semanticProjectionBuilder) callRelations() []BodyCallRelations {
	out := make([]BodyCallRelations, 0, len(b.bodies))
	for _, item := range b.bodies {
		calls := BodyCallRelations{Body: item.id}
		graph := item.result.Graph()
		if graph == nil {
			out = append(out, calls)
			continue
		}
		for _, point := range graph.RPO() {
			site, ok := item.result.CallSiteView(point)
			if !ok {
				continue
			}
			location := b.locationForFactSpan(site.CallSpan())
			if !location.Valid() {
				location = b.locationForFactSpan(site.CalleeSpan())
			}
			call := CallRelation{Location: location, MaySuspend: item.result.PointMaySuspend(point)}
			if symbolID := site.CalleeSymbol(); symbolID != 0 {
				if _, function := item.result.FunctionBySymbol(symbolID); function {
					call.Callee = &CalleeIdentity{Kind: "function", SymbolID: uint64(symbolID), Name: item.result.SymbolName(symbolID)}
				} else if target, found := b.functionTarget(symbolID); found && !item.result.SymbolHasWrite(symbolID) {
					call.Callee = &CalleeIdentity{Kind: "function", SymbolID: uint64(target), Name: item.result.SymbolName(symbolID)}
				}
			}
			if call.Callee == nil {
				if name, known := item.result.CallSignatureNameAtPoint(point); known {
					call.Callee = &CalleeIdentity{Kind: "signature", Name: name}
				}
			}
			calls.Calls = append(calls.Calls, call)
		}
		out = append(out, calls)
	}
	return out
}

func (b *semanticProjectionBuilder) functionTarget(target symbol.ID) (symbol.ID, bool) {
	for _, item := range b.bodies {
		if item.function == nil {
			continue
		}
		origin, ok := b.root.FunctionOrigin(item.function)
		if ok && origin.HasTargetSymbol && origin.TargetSymbol == target && origin.Symbol != 0 {
			return origin.Symbol, true
		}
	}
	return 0, false
}

func anchorsFromJudgments(defaultFile string, items []judgment.Judgment) []anchoredSubject {
	var out []anchoredSubject
	for _, item := range items {
		if item.Subject.Anchor.IsZero() {
			continue
		}
		for _, span := range item.Spans {
			file := span.File
			if file == "" {
				file = defaultFile
			}
			location := SourceLocation{File: file, Span: SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}}
			if location.Valid() {
				out = append(out, anchoredSubject{location: location, anchor: item.Subject.Anchor})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return locationLess(out[i].location, out[j].location) })
	return out
}

func repairActionsFromJudgments(defaultFile string, items []judgment.Judgment) []RepairAction {
	registry := judgment.DefaultRegistry()
	var out []RepairAction
	for _, item := range items {
		spec, ok := registry.Lookup(item.Code)
		if !ok || len(spec.Repairs) == 0 {
			continue
		}
		target, ok := judgmentTarget(defaultFile, item)
		if !ok {
			continue
		}
		for _, descriptor := range spec.Repairs {
			switch descriptor.Kind {
			case judgment.RepairAddNilGuard:
				if !judgmentHasNilCause(item) {
					continue
				}
			case judgment.RepairAddAnnotation:
				if item.Expected.Type == nil {
					continue
				}
			}
			action := RepairAction{Kind: descriptor.Kind, Target: target}
			if descriptor.Kind == judgment.RepairAddAnnotation {
				action.Payload.Type = typeformat.Short(item.Expected.Type)
			}
			out = append(out, action)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return locationLess(out[i].Target, out[j].Target)
	})
	return out
}

func judgmentTarget(defaultFile string, item judgment.Judgment) (SourceLocation, bool) {
	for _, span := range item.Spans {
		file := span.File
		if file == "" {
			file = defaultFile
		}
		location := SourceLocation{File: file, Span: SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}}
		if location.Valid() {
			return location, true
		}
	}
	return SourceLocation{}, false
}

func judgmentHasNilCause(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		switch evidence.Detail.Kind {
		case judgment.EvidenceDetailMayBeNil, judgment.EvidenceDetailCalleeMayBeNil:
			return true
		}
	}
	return false
}

func (b *semanticProjectionBuilder) locationForSpan(span source.Span) SourceLocation {
	return SourceLocation{File: b.file, Span: SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}}
}

func (b *semanticProjectionBuilder) locationForFactSpan(span factflow.SourceSpan) SourceLocation {
	return SourceLocation{File: b.file, Span: SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}}
}

func (b *semanticProjectionBuilder) locationForPosition(position ast.Position, name string) SourceLocation {
	endLine, endCol := position.EndLine, position.EndColumn
	if endLine == 0 {
		endLine = position.Line
	}
	if endCol < position.Column || (endLine == position.Line && endCol == position.Column) {
		endCol = position.Column + len(name) - 1
	}
	return SourceLocation{File: b.file, Span: SourceSpan{StartLine: position.Line, StartCol: position.Column, EndLine: endLine, EndCol: endCol}}
}

func (b *semanticProjectionBuilder) parameterLocation(fn *ast.FunctionExpr, index int, name string) SourceLocation {
	if fn == nil || name == "" {
		return SourceLocation{}
	}
	start, end, ok := offsetsForSpan(b.source, ast.SpanOf(fn))
	if !ok {
		return b.locationForSpan(ast.SpanOf(fn))
	}
	open := bytesIndexByte(b.source[start:end], '(')
	if open < 0 {
		return b.locationForSpan(ast.SpanOf(fn))
	}
	segment := b.source[start+open : end]
	needle := []byte(name)
	seen := 0
	for at := 0; ; {
		found := bytesIndex(segment[at:], needle)
		if found < 0 {
			break
		}
		found += at
		leftOK := found == 0 || !identifierByte(segment[found-1])
		right := found + len(needle)
		rightOK := right >= len(segment) || !identifierByte(segment[right])
		if leftOK && rightOK {
			if index < 0 || seen == index {
				line, col := lineColumnAt(b.source, start+open+found)
				return SourceLocation{File: b.file, Span: SourceSpan{StartLine: line, StartCol: col, EndLine: line, EndCol: col + len(name) - 1}}
			}
			seen++
		}
		at = found + len(needle)
	}
	return b.locationForSpan(ast.SpanOf(fn))
}

func (b *semanticProjectionBuilder) functionDefinitionLocation(fn *ast.FunctionExpr) SourceLocation {
	if fn == nil {
		return SourceLocation{}
	}
	span := ast.SpanOf(fn)
	if !span.Valid() {
		return SourceLocation{}
	}
	return SourceLocation{File: b.file, Span: SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.StartLine, EndCol: span.StartCol + len("function") - 1}}
}

func binderKindFromWIR(kind wir.SymbolKind) BinderKind {
	switch kind {
	case wir.SymbolParam:
		return BinderParam
	case wir.SymbolLocal:
		return BinderLocal
	case wir.SymbolGlobal:
		return BinderGlobal
	case wir.SymbolUpvalue:
		return BinderUpvalue
	case wir.SymbolFunction:
		return BinderFunction
	default:
		return BinderUnknown
	}
}

func binderKindFromBind(kind symbol.Kind) BinderKind {
	switch kind {
	case symbol.Param:
		return BinderParam
	case symbol.Local:
		return BinderLocal
	case symbol.Global:
		return BinderGlobal
	case symbol.Upvalue:
		return BinderUpvalue
	case symbol.Function:
		return BinderFunction
	default:
		return BinderUnknown
	}
}

func namePosition(items []ast.Position, index int) ast.Position {
	if index >= 0 && index < len(items) {
		return items[index]
	}
	return ast.Position{}
}
func occurrenceAlreadyPresent(items []BinderOccurrence, role BinderOccurrenceRole, location SourceLocation) bool {
	for _, item := range items {
		if item.Role == role && item.Location == location {
			return true
		}
	}
	return false
}
func locationLess(left, right SourceLocation) bool {
	if left.File != right.File {
		return left.File < right.File
	}
	if left.Span.StartLine != right.Span.StartLine {
		return left.Span.StartLine < right.Span.StartLine
	}
	if left.Span.StartCol != right.Span.StartCol {
		return left.Span.StartCol < right.Span.StartCol
	}
	if left.Span.EndLine != right.Span.EndLine {
		return left.Span.EndLine < right.Span.EndLine
	}
	return left.Span.EndCol < right.Span.EndCol
}
func wholeSourceSpan(data []byte) SourceSpan {
	line, col := lineColumnAt(data, len(data))
	return SourceSpan{StartLine: 1, StartCol: 1, EndLine: line, EndCol: col}
}

func offsetsForSpan(data []byte, span source.Span) (int, int, bool) {
	if !span.Valid() {
		return 0, 0, false
	}
	start, ok := offsetAt(data, span.StartLine, span.StartCol)
	if !ok {
		return 0, 0, false
	}
	endLine, endCol := span.EndLine, span.EndCol
	if endLine == 0 {
		endLine = span.StartLine
	}
	if endCol < span.StartCol && endLine == span.StartLine {
		endCol = span.StartCol
	}
	end, ok := offsetAt(data, endLine, endCol)
	if !ok {
		return 0, 0, false
	}
	if end < len(data) {
		end++ // parser token end coordinates are inclusive
	}
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

func offsetAt(data []byte, line, column int) (int, bool) {
	if line < 1 || column < 1 {
		return 0, false
	}
	currentLine, currentColumn := 1, 1
	for offset := 0; offset < len(data); offset++ {
		if currentLine == line && currentColumn == column {
			return offset, true
		}
		if data[offset] == '\n' {
			currentLine++
			currentColumn = 1
			continue
		}
		currentColumn++
	}
	return len(data), currentLine == line && currentColumn == column
}

func lineColumnAt(data []byte, target int) (int, int) {
	if target < 0 {
		target = 0
	}
	if target > len(data) {
		target = len(data)
	}
	line, column := 1, 1
	for offset := 0; offset < target; offset++ {
		if data[offset] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}
func identifierByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}
func bytesIndex(data, needle []byte) int         { return strings.Index(string(data), string(needle)) }
func bytesIndexByte(data []byte, value byte) int { return strings.IndexByte(string(data), value) }
