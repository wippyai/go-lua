package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	semanticreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
	"github.com/wippyai/go-lua/analysis/embedding"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	typeformat "github.com/wippyai/go-lua/analysis/domain/type/format"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// semanticQuerySnapshot contains only immutable projection DTOs and source
// bytes. It is built after the solve, before publication; query reads never
// touch mutable checker worklists or run another analysis.
type semanticQuerySnapshot struct {
	entryDocument embedding.DocumentID
	sources       map[embedding.DocumentID]embedding.SourceSnapshot
	binders       []BinderInfo
	bodies        []queryBody
	symbols       []DocumentSymbol
	calls         []BodyCallRelations
	anchors       []anchoredSubject
	repairs       []RepairAction
	exprs         []expressionAt
	tokens        []SemanticToken
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
	document  embedding.DocumentID
	digest    embedding.Digest
	file      string // Display only; semantic joins use document and digest.
	source    []byte
	lineIndex sourceLineIndex // Built once for this exact source digest.
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

func projectSemanticQueries(input UnitInput, stmts []ast.Stmt, root *body.Result, judgments []judgment.Judgment, placement placementplan.Plan) *semanticQuerySnapshot {
	entryFile := documentLabel(input, input.EntryDocument)
	entrySource := input.Sources[input.EntryDocument]
	b := semanticProjectionBuilder{
		document:  input.EntryDocument,
		digest:    entrySource.ContentDigest,
		file:      entryFile,
		source:    append([]byte(nil), entrySource.Content...),
		root:      root,
		byFunc:    make(map[*ast.FunctionExpr]projectionBody),
		binders:   make(map[uint64]*BinderInfo),
		seenExprs: make(map[ast.Expr]map[cfg.Point]struct{}),
	}
	b.lineIndex = newSourceLineIndex(b.source)
	b.collectBodies(root, BodyID("root"), b.locationForSpan(b.lineIndex.wholeSourceSpan(len(b.source))))
	b.collectBinderDefinitionsAndOccurrences(stmts)
	b.collectExpressions()

	result := &semanticQuerySnapshot{
		entryDocument: input.EntryDocument,
		sources: map[embedding.DocumentID]embedding.SourceSnapshot{
			input.EntryDocument: entrySource.Clone(),
		},
		bodies:  make([]queryBody, 0, len(b.bodies)),
		anchors: anchorsFromJudgments(input.EntryDocument, entrySource.ContentDigest, entryFile, judgments),
		repairs: repairActionsFromJudgmentsWithSource(input.EntryDocument, entrySource.ContentDigest, entryFile, b.source, judgments),
	}
	for document, snapshot := range input.Sources {
		result.sources[document] = snapshot.Clone()
	}
	for _, item := range b.bodies {
		result.bodies = append(result.bodies, queryBody{id: item.id, location: item.location})
	}
	result.binders = b.sortedBinders()
	result.tokens = b.semanticTokens(result.binders, placement)
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
	var walkStmts func([]ast.Stmt, *ast.FunctionExpr)
	var walkExpr func(ast.Expr, *ast.FunctionExpr, BinderOccurrenceRole)
	addOccurrence := func(ident *ast.IdentExpr, _ *ast.FunctionExpr, _ BinderOccurrenceRole) {
		if ident == nil || b.root == nil {
			return
		}
		id, ok := b.root.SymbolOfIdent(ident)
		if !ok || id == 0 {
			return
		}
		// The binder owns the definitive occurrence set and roles. The AST walk
		// below only discovers which binder declarations need projecting.
		b.ensureBinder(id)
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
	b.projectBinderOccurrences()
}

func (b *semanticProjectionBuilder) projectBinderOccurrences() {
	for symbolID, info := range b.binders {
		for _, occurrence := range b.root.BinderOccurrences(symbol.ID(symbolID)) {
			location := b.locationForSpan(occurrence.Span)
			role, ok := binderOccurrenceRole(occurrence.Role)
			if !ok || !location.Valid() || occurrenceAlreadyPresent(info.Occurrences, role, location) {
				continue
			}
			info.Occurrences = append(info.Occurrences, BinderOccurrence{Role: role, Location: location, Scope: b.scopeForLocation(location)})
		}
	}
}

func binderOccurrenceRole(role bind.OccurrenceRole) (BinderOccurrenceRole, bool) {
	switch role {
	case bind.OccurrenceRead:
		return BinderRead, true
	case bind.OccurrenceWrite:
		return BinderWrite, true
	case bind.OccurrenceCapture:
		return BinderCapture, true
	default:
		return "", false
	}
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
	for _, slot := range b.root.FunctionParamSlots(fn) {
		b.defineBinder(slot.Symbol, slot.Name, SourceLocation{})
	}
	if id, ok := b.root.VarargSymbol(fn); ok {
		b.defineBinder(id, "...", SourceLocation{})
	}
}

func (b *semanticProjectionBuilder) defineBinder(id symbol.ID, name string, location SourceLocation) {
	if id == 0 {
		return
	}
	item := b.ensureBinder(id)
	if declaration, ok := b.root.BinderDeclaration(id); ok && declaration.Valid() {
		location = b.locationForSpan(declaration.Span)
	}
	if item.Name == "" && name != "" {
		item.Name = name
	}
	if !item.Definition.Valid() && location.Valid() {
		item.Definition = location
		item.Scope = b.scopeForLocation(location)
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
	item.ModuleLocal = item.Kind == BinderParam || item.Kind == BinderLocal || item.Kind == BinderUpvalue || item.Kind == BinderFunction
	b.binders[key] = item
	return item
}

func (b *semanticProjectionBuilder) scopeForLocation(location SourceLocation) SourceLocation {
	if !location.Valid() {
		return SourceLocation{}
	}
	start := location.ByteSpan.StartByte
	best := b.locationForSpan(b.lineIndex.wholeSourceSpan(len(b.source)))
	bestWidth := len(b.source) + 1
	for _, body := range b.bodies {
		if !body.location.Valid() || !locationContains(body.location, b.document, b.digest, start) {
			continue
		}
		if width := locationWidth(body.location); width < bestWidth {
			best, bestWidth = body.location, width
		}
	}
	return best
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
		b.certifyBinder(item)
		copy := *item
		copy.Occurrences = append([]BinderOccurrence(nil), item.Occurrences...)
		sort.SliceStable(copy.Occurrences, func(i, j int) bool { return locationLess(copy.Occurrences[i].Location, copy.Occurrences[j].Location) })
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SymbolID < out[j].SymbolID })
	return out
}

func (b *semanticProjectionBuilder) certifyBinder(item *BinderInfo) {
	if item == nil {
		return
	}
	expected := b.root.BinderOccurrences(symbol.ID(item.SymbolID))
	complete := len(expected) == len(item.Occurrences)
	seen := make(map[embedding.ByteSpan]struct{}, len(item.Occurrences))
	for _, occurrence := range item.Occurrences {
		if !occurrence.Location.Valid() || occurrence.Location.Document != b.document || occurrence.Location.ContentDigest != b.digest {
			complete = false
			continue
		}
		span := occurrence.Location.ByteSpan
		if _, exists := seen[span]; exists {
			complete = false
			continue
		}
		seen[span] = struct{}{}
		if item.Definition.Valid() && sourceLocationsOverlap(b.source, item.Definition, occurrence.Location) {
			complete = false
		}
	}
	item.OccurrencesComplete = complete
	declaration, hasDeclaration := b.root.BinderDeclaration(symbol.ID(item.SymbolID))
	item.Renameable = complete && item.ModuleLocal && hasDeclaration && declaration.Valid() && !declaration.Synthetic && item.Definition.Valid()
}

// semanticTokens joins already-solved binder, typestate, and placement facts
// into one immutable query projection. It deliberately makes no type or
// lifecycle decision: binder kinds come from the bind/WIR projection,
// typestate membership from solved lifecycle sites, and placement licensing
// from the completed placement plan.
func (b *semanticProjectionBuilder) semanticTokens(binders []BinderInfo, plan placementplan.Plan) []SemanticToken {
	byLocation := make(map[SourceLocation]SemanticToken)
	add := func(location SourceLocation, kind SemanticTokenKind, modifiers ...SemanticTokenModifier) {
		if !location.Valid() {
			return
		}
		item, exists := byLocation[location]
		if !exists {
			item = SemanticToken{Kind: kind, Location: location}
		}
		for _, modifier := range modifiers {
			if modifier == "" || semanticTokenHasModifier(item.Modifiers, modifier) {
				continue
			}
			item.Modifiers = append(item.Modifiers, modifier)
		}
		sort.Slice(item.Modifiers, func(i, j int) bool { return item.Modifiers[i] < item.Modifiers[j] })
		byLocation[location] = item
	}

	for _, binder := range binders {
		kind := semanticTokenKindForBinder(binder.Kind)
		add(binder.Definition, kind)
		for _, occurrence := range binder.Occurrences {
			add(occurrence.Location, kind)
		}
	}

	tracked := b.typestateTrackedBinders(binders)
	for _, binder := range binders {
		if _, ok := tracked[binder.SymbolID]; !ok {
			continue
		}
		kind := semanticTokenKindForBinder(binder.Kind)
		add(binder.Definition, kind, SemanticTokenTypestateTracked)
		for _, occurrence := range binder.Occurrences {
			add(occurrence.Location, kind, SemanticTokenTypestateTracked)
		}
	}

	placementByIdentity := make(map[string]placementplan.Entry, len(plan.Entries))
	for _, entry := range plan.Entries {
		placementByIdentity[entry.ID.String()] = entry
	}
	for _, item := range b.bodies {
		item.result.ForEachAllocationSiteFact(func(fact body.AllocationSiteFact) bool {
			entry, ok := placementByIdentity[fact.Identity.String()]
			if !ok || (!entry.FrameLocal && !entry.Decomposable) || !fact.HasBirthSpan {
				return true
			}
			// A table literal is not itself a binder. Mark only its opening
			// source character so the protocol stream remains non-overlapping
			// with binder tokens nested inside the literal.
			add(semanticTokenSiteLocation(b.locationForBodySpan(fact.BirthSpan)), SemanticTokenVariable, SemanticTokenPlacement)
			return true
		})
	}

	out := make([]SemanticToken, 0, len(byLocation))
	for _, item := range byLocation {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Location != out[j].Location {
			return locationLess(out[i].Location, out[j].Location)
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func semanticTokenKindForBinder(kind BinderKind) SemanticTokenKind {
	switch kind {
	case BinderParam:
		return SemanticTokenParameter
	case BinderFunction:
		return SemanticTokenFunction
	default:
		return SemanticTokenVariable
	}
}

func semanticTokenHasModifier(items []SemanticTokenModifier, wanted SemanticTokenModifier) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func (b *semanticProjectionBuilder) typestateTrackedBinders(binders []BinderInfo) map[uint64]struct{} {
	tracked := make(map[uint64]struct{})
	for _, item := range b.bodies {
		for _, site := range item.result.LifecycleSites() {
			if site.TargetLabel == "" {
				continue
			}
			siteLocation := b.locationForBodySpan(site.Span)
			if !siteLocation.Valid() {
				continue
			}
			for _, binder := range binders {
				if !semanticTokenTargetMatchesBinder(site.TargetLabel, binder.Name) || !binderTouchesLocation(b.source, binder, siteLocation) {
					continue
				}
				tracked[binder.SymbolID] = struct{}{}
			}
		}
	}
	return tracked
}

func semanticTokenTargetMatchesBinder(target, name string) bool {
	if name == "" || target == "" {
		return false
	}
	return target == name || strings.HasPrefix(target, name+".") || strings.HasPrefix(target, name+"[")
}

func binderTouchesLocation(data []byte, binder BinderInfo, location SourceLocation) bool {
	for _, candidate := range binderSourceLocations(binder) {
		if sourceLocationsOverlap(data, candidate, location) {
			return true
		}
	}
	return false
}

func binderSourceLocations(binder BinderInfo) []SourceLocation {
	locations := make([]SourceLocation, 0, len(binder.Occurrences)+1)
	if binder.Definition.Valid() {
		locations = append(locations, binder.Definition)
	}
	for _, occurrence := range binder.Occurrences {
		locations = append(locations, occurrence.Location)
	}
	return locations
}

func sourceLocationsOverlap(data []byte, left, right SourceLocation) bool {
	if left.Document != right.Document || left.ContentDigest != right.ContentDigest || !left.Valid() || !right.Valid() {
		return false
	}
	return left.ByteSpan.StartByte < right.ByteSpan.EndByte && right.ByteSpan.StartByte < left.ByteSpan.EndByte
}

func semanticTokenSiteLocation(location SourceLocation) SourceLocation {
	if !location.Valid() {
		return SourceLocation{}
	}
	location.Span.EndLine = location.Span.StartLine
	location.Span.EndCol = location.Span.StartCol
	location.ByteSpan.EndByte = location.ByteSpan.StartByte + 1
	return location
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

func anchorsFromJudgments(defaultDocument embedding.DocumentID, defaultDigest embedding.Digest, defaultFile string, items []judgment.Judgment) []anchoredSubject {
	var out []anchoredSubject
	for _, item := range items {
		if item.Subject.Anchor.IsZero() {
			continue
		}
		for _, span := range item.Spans {
			location, ok := judgmentLocation(defaultDocument, defaultDigest, defaultFile, span)
			if ok {
				out = append(out, anchoredSubject{location: location, anchor: item.Subject.Anchor})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return locationLess(out[i].location, out[j].location) })
	return out
}

func repairActionsFromJudgments(defaultFile string, items []judgment.Judgment) []RepairAction {
	return repairActionsFromJudgmentsWithSource(embedding.FileDocument(defaultFile), embedding.DigestBytes(nil), defaultFile, nil, items)
}

func repairActionsFromJudgmentsWithSource(defaultDocument embedding.DocumentID, defaultDigest embedding.Digest, defaultFile string, data []byte, items []judgment.Judgment) []RepairAction {
	registry := judgment.DefaultRegistry()
	var out []RepairAction
	for _, item := range items {
		spec, ok := registry.Lookup(item.Code)
		if !ok || len(spec.Repairs) == 0 {
			continue
		}
		target, ok := judgmentTarget(defaultDocument, defaultDigest, defaultFile, item)
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
			action := RepairAction{Code: item.Code, Kind: descriptor.Kind, Target: target}
			if descriptor.Kind == judgment.RepairAddAnnotation {
				action.Payload.Type = typeformat.Short(item.Expected.Type)
			}
			if descriptor.Kind == judgment.RepairConstructFixedShape {
				action.Payload.Fields = shapePolymorphicRepairFields(item)
			}
			action.Payload.Edits = repairEdits(defaultDocument, defaultDigest, defaultFile, data, item, descriptor.Kind)
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

func shapePolymorphicRepairFields(item judgment.Judgment) []string {
	seen := map[string]struct{}{}
	for _, evidence := range item.Evidence {
		if (evidence.Detail.Kind != judgment.EvidenceDetailAdviceShapeConditionalField && evidence.Detail.Kind != judgment.EvidenceDetailAdviceShapeUnionField) || evidence.Detail.Cause.Params.Field == "" {
			continue
		}
		seen[evidence.Detail.Cause.Params.Field] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for field := range seen {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func repairEdits(defaultDocument embedding.DocumentID, defaultDigest embedding.Digest, defaultFile string, data []byte, item judgment.Judgment, kind judgment.RepairKind) []RepairEdit {
	if len(data) == 0 || kind != judgment.RepairRemoveRedundantClaim || item.Code != judgment.CodeAdviceRedundantClaim || len(item.Spans) < 2 {
		return nil
	}
	claim, claimOK := judgmentLocation(defaultDocument, defaultDigest, defaultFile, item.Spans[0])
	operand, operandOK := judgmentLocation(defaultDocument, defaultDigest, defaultFile, item.Spans[1])
	if !claimOK || !operandOK || claim.Document != operand.Document || claim.ContentDigest != operand.ContentDigest {
		return nil
	}
	start, end := operand.ByteSpan.StartByte, operand.ByteSpan.EndByte
	if start < 0 || end > len(data) || start >= end {
		return nil
	}
	return []RepairEdit{{Target: claim, NewText: string(data[start:end])}}
}

func judgmentTarget(defaultDocument embedding.DocumentID, defaultDigest embedding.Digest, defaultFile string, item judgment.Judgment) (SourceLocation, bool) {
	for _, span := range item.Spans {
		if location, ok := judgmentLocation(defaultDocument, defaultDigest, defaultFile, span); ok {
			return location, true
		}
	}
	return SourceLocation{}, false
}

func judgmentLocation(defaultDocument embedding.DocumentID, defaultDigest embedding.Digest, defaultFile string, span judgment.SpanRef) (SourceLocation, bool) {
	if span.Location.Valid() {
		return SourceLocation{
			Document:      span.Location.Document,
			ContentDigest: span.Location.ContentDigest,
			ByteSpan:      span.Location.Span,
			Span:          SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol},
			File:          defaultFile,
		}, true
	}
	if !defaultDocument.Valid() || defaultDigest.IsZero() || span.StartLine <= 0 || span.StartCol <= 0 {
		return SourceLocation{}, false
	}
	return SourceLocation{
		Document:      defaultDocument,
		ContentDigest: defaultDigest,
		ByteSpan:      embedding.ByteSpan{},
		Span:          SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol},
		File:          defaultFile,
	}, true
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
	if !span.Valid() {
		return SourceLocation{}
	}
	start, end, ok := offsetsForSpan(b.source, b.lineIndex, span)
	if !ok {
		return SourceLocation{}
	}
	return SourceLocation{
		Document:      b.document,
		ContentDigest: b.digest,
		ByteSpan:      embedding.ByteSpan{StartByte: start, EndByte: end},
		Span:          SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol},
		File:          b.file,
	}
}

func (b *semanticProjectionBuilder) locationForFactSpan(span factflow.SourceSpan) SourceLocation {
	return b.locationForSpan(source.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol})
}

func (b *semanticProjectionBuilder) locationForBodySpan(span body.SourceSpan) SourceLocation {
	return b.locationForSpan(source.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol})
}

func (b *semanticProjectionBuilder) locationForPosition(position ast.Position, name string) SourceLocation {
	endLine, endCol := position.EndLine, position.EndColumn
	if endLine == 0 {
		endLine = position.Line
	}
	if endCol < position.Column || (endLine == position.Line && endCol == position.Column) {
		endCol = position.Column + len(name) - 1
	}
	return b.locationForSpan(source.Span{StartLine: position.Line, StartCol: position.Column, EndLine: endLine, EndCol: endCol})
}

func (b *semanticProjectionBuilder) functionDefinitionLocation(fn *ast.FunctionExpr) SourceLocation {
	if fn == nil {
		return SourceLocation{}
	}
	span := ast.SpanOf(fn)
	if !span.Valid() {
		return SourceLocation{}
	}
	return b.locationForSpan(source.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.StartLine, EndCol: span.StartCol + len("function") - 1})
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
	if left.Document != right.Document {
		return left.Document.String() < right.Document.String()
	}
	if left.ContentDigest != right.ContentDigest {
		return left.ContentDigest.String() < right.ContentDigest.String()
	}
	if left.ByteSpan.StartByte != right.ByteSpan.StartByte {
		return left.ByteSpan.StartByte < right.ByteSpan.StartByte
	}
	if left.ByteSpan.EndByte != right.ByteSpan.EndByte {
		return left.ByteSpan.EndByte < right.ByteSpan.EndByte
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
func offsetsForSpan(data []byte, index sourceLineIndex, span source.Span) (int, int, bool) {
	if !span.Valid() {
		return 0, 0, false
	}
	start, ok := index.offsetAt(data, span.StartLine, span.StartCol)
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
	end, ok := index.offsetAt(data, endLine, endCol)
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

// sourceLineIndex maps the parser's 1-indexed byte lines to their first byte.
// It belongs to one immutable source snapshot, identified by the builder's
// digest, and makes repeated span endpoint projection constant-time.
type sourceLineIndex struct {
	starts []int
}

func newSourceLineIndex(data []byte) sourceLineIndex {
	starts := make([]int, 1, 1+len(data)/32)
	starts[0] = 0
	for offset, b := range data {
		if b == '\n' {
			starts = append(starts, offset+1)
		}
	}
	return sourceLineIndex{starts: starts}
}

func (index sourceLineIndex) offsetAt(data []byte, line, column int) (int, bool) {
	if line < 1 || column < 1 {
		return 0, false
	}
	if line > len(index.starts) {
		return len(data), false
	}

	offset := index.starts[line-1] + column - 1
	if line < len(index.starts) {
		// The newline itself is the final byte-column of a non-final line.
		if offset < index.starts[line] {
			return offset, true
		}
		return len(data), false
	}
	if offset <= len(data) {
		return offset, true
	}
	return len(data), false
}

// wholeSourceSpan derives the final byte coordinate from the line starts that
// were already built for this source. Large units used to scan their complete
// source a second time here during semantic projection.
func (index sourceLineIndex) wholeSourceSpan(length int) source.Span {
	line := len(index.starts)
	if line == 0 { // Be defensive about a zero-value index.
		line = 1
		return source.Span{StartLine: 1, StartCol: 1, EndLine: line, EndCol: length + 1}
	}
	return source.Span{
		StartLine: 1,
		StartCol:  1,
		EndLine:   line,
		EndCol:    length - index.starts[line-1] + 1,
	}
}

// offsetAt is retained for the query API's occasional line/column conversion.
// Projection builds and reuses a sourceLineIndex instead.
func offsetAt(data []byte, line, column int) (int, bool) {
	return newSourceLineIndex(data).offsetAt(data, line, column)
}
