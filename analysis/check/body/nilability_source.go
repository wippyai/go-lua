package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NilabilityOriginKind identifies one exact source-owned step in a nilability
// proof. These are semantic events; user-facing prose remains in diagnostics.
type NilabilityOriginKind uint8

const (
	NilabilityOriginLocalNilBirth NilabilityOriginKind = iota + 1
	NilabilityOriginJoin
	NilabilityOriginOptionalDeclaration
	NilabilityOriginUse
)

// NilabilityOrigin is one source-backed step in the provenance of a value
// which is still nilable at a use. The chain is complete only when every step
// was resolved through binder identity and parser-owned syntax.
type NilabilityOrigin struct {
	Kind      NilabilityOriginKind
	Span      SourceSpan
	Subject   string
	Field     string
	TypeLabel string
	FieldUse  bool
	// MissingBranch is "then" or "else" for a one-sided assignment join.
	MissingBranch string
}

// NilabilitySourceInfo is the body-owned source identity for a nilable use.
type NilabilitySourceInfo struct {
	OptionalField bool
	CallPoint     cfg.Point
	HasCallPoint  bool
	Origins       []NilabilityOrigin
	OriginsExact  bool
}

func (r *Result) NilabilitySourceInfoFor(expr any) NilabilitySourceInfo {
	astExpr, ok := expr.(ast.Expr)
	if !ok || astExpr == nil {
		return NilabilitySourceInfo{}
	}
	info := NilabilitySourceInfo{}
	if attr, ok := astExpr.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyDot {
		info.OptionalField = true
	}
	info.Origins, info.OriginsExact = r.nilabilityOrigins(astExpr)
	if call, ok := astExpr.(*ast.FuncCallExpr); ok {
		info.CallPoint, info.HasCallPoint = r.callExprPoint(call)
	}
	return info
}

func (r *Result) nilabilityOrigins(expr ast.Expr) ([]NilabilityOrigin, bool) {
	if r == nil || r.bindings == nil || expr == nil {
		return nil, false
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyDot {
		return r.optionalFieldNilabilityOrigins(attr)
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident == nil {
		return nil, false
	}
	id, ok := r.bindings.SymbolOf(ident)
	if !ok {
		return nil, false
	}
	origin, ok := r.bindings.LocalOrigin(id)
	if !ok || origin.Stmt == nil || origin.Index < 0 || origin.Index >= len(origin.Stmt.Exprs) {
		return nil, false
	}
	if _, nilBirth := origin.Stmt.Exprs[origin.Index].(*ast.NilExpr); !nilBirth {
		return nil, false
	}
	annotation, annotated := r.bindings.SymbolTypeAnnotation(id)
	if !annotated || !optionalTypeSyntax(annotation) {
		return nil, false
	}
	name := ident.Value
	origins := []NilabilityOrigin{
		{Kind: NilabilityOriginLocalNilBirth, Span: localNilabilityNameSpan(origin.Stmt, origin.Index, name), Subject: name},
	}
	contributingWrites := make(map[*ast.IdentExpr]struct{})
	for _, stmt := range r.functionStatements() {
		collectNilabilityJoinOrigins(r.bindings, stmt, id, ast.SpanOf(expr).StartLine, name, &origins, contributingWrites)
	}
	for _, write := range r.bindings.WriteIdents(id) {
		if write == nil || write.Line() >= ast.SpanOf(expr).StartLine {
			continue
		}
		if _, contributes := contributingWrites[write]; !contributes {
			return nil, false
		}
	}
	if len(origins) == 1 || statementsBeforeLineContainReturn(r.functionStatements(), ast.SpanOf(expr).StartLine) {
		return nil, false
	}
	if len(origins) > 1 {
		origins[0].MissingBranch = origins[1].MissingBranch
	}
	sort.SliceStable(origins[1:], func(i, j int) bool {
		return origins[i+1].Span.StartLine < origins[j+1].Span.StartLine
	})
	origins = append(origins,
		NilabilityOrigin{Kind: NilabilityOriginUse, Span: sourceSpanFromAST(ast.SpanOf(expr)), Subject: name},
		NilabilityOrigin{Kind: NilabilityOriginOptionalDeclaration, Span: nilabilityTypeAnnotationSpan(annotation), Subject: name, TypeLabel: TypeAnnotationLabel(annotation)},
	)
	return origins, true
}

func (r *Result) functionStatements() []ast.Stmt {
	if r == nil {
		return nil
	}
	if r.function != nil {
		return r.function.Stmts
	}
	return r.sourceStmts
}

func (r *Result) optionalFieldNilabilityOrigins(attr *ast.AttrGetExpr) ([]NilabilityOrigin, bool) {
	root, ok := attr.Object.(*ast.IdentExpr)
	if !ok || root == nil || root.Value == "self" {
		return nil, false
	}
	id, ok := r.bindings.SymbolOf(root)
	if !ok {
		return nil, false
	}
	for _, slot := range r.bindings.ParamSlots(r.function) {
		if slot.Symbol == id && slot.ImplicitSelf {
			return nil, false
		}
	}
	annotation, ok := r.bindings.SymbolTypeAnnotation(id)
	if !ok {
		return nil, false
	}
	record, ok := r.recordTypeSyntax(annotation, make(map[ast.TypeExpr]struct{}))
	if !ok {
		return nil, false
	}
	fieldName := ast.KeyName(attr.Key)
	for _, field := range record.Fields {
		if field.Name != fieldName || (!field.Optional && !optionalTypeSyntax(field.Type)) {
			continue
		}
		label := TypeAnnotationLabel(field.Type)
		if field.Optional && !optionalTypeSyntax(field.Type) && label != "" {
			label += "?"
		}
		declSpan := sourceSpanFromPosition(field.NamePosition, len(field.Name))
		if !sourceSpanValid(declSpan) {
			return nil, false
		}
		subject := root.Value + "." + fieldName
		return []NilabilityOrigin{
			{Kind: NilabilityOriginOptionalDeclaration, Span: declSpan, Subject: subject, Field: fieldName, TypeLabel: label, FieldUse: true},
			{Kind: NilabilityOriginUse, Span: sourceSpanFromAST(ast.SpanOf(attr)), Subject: subject, Field: fieldName, FieldUse: true},
		}, true
	}
	return nil, false
}

func nilabilityTypeAnnotationSpan(expr ast.TypeExpr) SourceSpan {
	span := sourceSpanFromAST(ast.SpanOf(expr))
	if sourceSpanValid(span) {
		return span
	}
	if optional, ok := expr.(*ast.OptionalTypeExpr); ok {
		return sourceSpanFromAST(ast.SpanOf(optional.Inner))
	}
	return SourceSpan{}
}

func statementsBeforeLineContainReturn(stmts []ast.Stmt, beforeLine int) bool {
	for _, stmt := range stmts {
		if stmt == nil || (beforeLine > 0 && stmt.Line() >= beforeLine) {
			continue
		}
		switch typed := stmt.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.IfStmt:
			if statementsBeforeLineContainReturn(typed.Then, beforeLine) || statementsBeforeLineContainReturn(typed.Else, beforeLine) {
				return true
			}
		case *ast.DoBlockStmt:
			if statementsBeforeLineContainReturn(typed.Stmts, beforeLine) {
				return true
			}
		}
	}
	return false
}

func (r *Result) recordTypeSyntax(expr ast.TypeExpr, active map[ast.TypeExpr]struct{}) (*ast.RecordTypeExpr, bool) {
	if expr == nil {
		return nil, false
	}
	if _, seen := active[expr]; seen {
		return nil, false
	}
	active[expr] = struct{}{}
	defer delete(active, expr)
	switch typed := expr.(type) {
	case *ast.RecordTypeExpr:
		return typed, true
	case *ast.TypeRefExpr:
		decl, ok := r.bindings.TypeRef(typed)
		if !ok || decl.Type == nil {
			return nil, false
		}
		return r.recordTypeSyntax(decl.Type.Type, active)
	case *ast.PrimitiveTypeExpr:
		decl, ok := r.bindings.PrimitiveTypeRef(typed)
		if !ok || decl.Type == nil {
			return nil, false
		}
		return r.recordTypeSyntax(decl.Type.Type, active)
	default:
		return nil, false
	}
}

func collectNilabilityJoinOrigins(bindings interface {
	SymbolOf(*ast.IdentExpr) (symbol.ID, bool)
}, stmt ast.Stmt, id symbol.ID, beforeLine int, subject string, out *[]NilabilityOrigin, contributingWrites map[*ast.IdentExpr]struct{}) {
	if stmt == nil || (beforeLine > 0 && stmt.Line() >= beforeLine) {
		return
	}
	switch typed := stmt.(type) {
	case *ast.IfStmt:
		thenWrites := statementsWriteSymbol(bindings, typed.Then, id)
		elseWrites := statementsWriteSymbol(bindings, typed.Else, id)
		if thenWrites != elseWrites {
			span := SourceSpan{StartLine: typed.LastLine(), StartCol: typed.Column(), EndLine: typed.LastLine(), EndCol: typed.Column() + 3}
			missing := "else"
			assigned := typed.Then
			if elseWrites {
				missing = "then"
				assigned = typed.Else
			}
			*out = append(*out, NilabilityOrigin{Kind: NilabilityOriginJoin, Span: span, Subject: subject, MissingBranch: missing})
			collectSymbolWriteIdents(bindings, assigned, id, contributingWrites)
		}
		for _, nested := range typed.Then {
			collectNilabilityJoinOrigins(bindings, nested, id, beforeLine, subject, out, contributingWrites)
		}
		for _, nested := range typed.Else {
			collectNilabilityJoinOrigins(bindings, nested, id, beforeLine, subject, out, contributingWrites)
		}
	case *ast.DoBlockStmt:
		for _, nested := range typed.Stmts {
			collectNilabilityJoinOrigins(bindings, nested, id, beforeLine, subject, out, contributingWrites)
		}
	}
}

func collectSymbolWriteIdents(bindings interface {
	SymbolOf(*ast.IdentExpr) (symbol.ID, bool)
}, stmts []ast.Stmt, id symbol.ID, out map[*ast.IdentExpr]struct{}) {
	for _, stmt := range stmts {
		switch typed := stmt.(type) {
		case *ast.AssignStmt:
			for _, target := range typed.Lhs {
				ident, ok := target.(*ast.IdentExpr)
				if !ok {
					continue
				}
				if targetID, resolved := bindings.SymbolOf(ident); resolved && targetID == id {
					out[ident] = struct{}{}
				}
			}
		case *ast.IfStmt:
			collectSymbolWriteIdents(bindings, typed.Then, id, out)
			collectSymbolWriteIdents(bindings, typed.Else, id, out)
		case *ast.DoBlockStmt:
			collectSymbolWriteIdents(bindings, typed.Stmts, id, out)
		}
	}
}

func statementsWriteSymbol(bindings interface {
	SymbolOf(*ast.IdentExpr) (symbol.ID, bool)
}, stmts []ast.Stmt, id symbol.ID) bool {
	for _, stmt := range stmts {
		switch typed := stmt.(type) {
		case *ast.AssignStmt:
			for _, target := range typed.Lhs {
				ident, ok := target.(*ast.IdentExpr)
				if !ok {
					continue
				}
				if targetID, resolved := bindings.SymbolOf(ident); resolved && targetID == id {
					return true
				}
			}
		case *ast.IfStmt:
			if statementsWriteSymbol(bindings, typed.Then, id) || statementsWriteSymbol(bindings, typed.Else, id) {
				return true
			}
		case *ast.DoBlockStmt:
			if statementsWriteSymbol(bindings, typed.Stmts, id) {
				return true
			}
		}
	}
	return false
}

func optionalTypeSyntax(expr ast.TypeExpr) bool {
	switch typed := expr.(type) {
	case *ast.OptionalTypeExpr:
		return true
	case *ast.UnionTypeExpr:
		for _, member := range typed.Types {
			if primitive, ok := member.(*ast.PrimitiveTypeExpr); ok && primitive.Name == "nil" {
				return true
			}
		}
	}
	return false
}

func localNilabilityNameSpan(stmt *ast.LocalAssignStmt, index int, name string) SourceSpan {
	if stmt != nil && index >= 0 && index < len(stmt.NamePositions) {
		return sourceSpanFromPosition(stmt.NamePositions[index], len(name))
	}
	return sourceSpanFromAST(ast.SpanOf(stmt))
}

func sourceSpanFromPosition(pos ast.Position, width int) SourceSpan {
	if !pos.Valid() {
		return SourceSpan{}
	}
	endLine, endCol := pos.EndLine, pos.EndColumn
	if endLine == 0 {
		endLine = pos.Line
	}
	if endCol == 0 {
		endCol = pos.Column + width
	}
	return SourceSpan{StartLine: pos.Line, StartCol: pos.Column, EndLine: endLine, EndCol: endCol}
}

func (r *Result) NilabilityAccessEvidenceFor(point cfg.Point, expr any) []NilableAccessEvidence {
	astExpr, ok := expr.(ast.Expr)
	if !ok || astExpr == nil {
		return nil
	}
	return r.AssignmentNilableAccessEvidence(point, astExpr)
}
