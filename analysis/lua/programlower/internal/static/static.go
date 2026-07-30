// Package static lowers parser-authored static type syntax into Program.
package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program"
)

// Writer owns static declaration identity while one Program is unfinished.
// Declaration IDs, rather than source names, are the sole lexical reference
// authority; names are retained only as authored reference spelling.
type Writer struct {
	builder    *program.Builder
	binding    *bind.Result
	sourceName string
	terms      map[bind.TypeDeclID]program.Term
	children   []program.Term
}

// Mark starts one LIFO ordered child range for an iterative type walk.
func (w *Writer) Mark() int {
	if w == nil {
		return -1
	}
	return len(w.children)
}

// Append retains one completed static type child in source order.
func (w *Writer) Append(term program.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("programlower: invalid static type child")
	}
	w.children = append(w.children, term)
	return nil
}

// Take completes one scalar child hold.
func (w *Writer) Take(mark int) (program.Term, error) {
	if w == nil || mark < 0 || mark != len(w.children)-1 {
		return 0, fmt.Errorf("programlower: incomplete static type child")
	}
	term := w.children[mark]
	w.children = w.children[:mark]
	return term, nil
}

// Clean reports whether the iterative type-child scratch is empty.
func (w *Writer) Clean() bool { return w != nil && len(w.children) == 0 }

// New creates a static writer over an unfinished Program builder.
func New(builder *program.Builder, binding *bind.Result, sourceName string) *Writer {
	return &Writer{
		builder:    builder,
		binding:    binding,
		sourceName: sourceName,
		terms:      make(map[bind.TypeDeclID]program.Term),
	}
}

// Predeclare records identities for the direct alias statements in body.
// Placement and parameters are deliberately deferred until the lowering walk
// reaches each source statement's actual executable cursor.
func (w *Writer) Predeclare(body program.Term, stmts []ast.Stmt) error {
	if w == nil || w.builder == nil || w.binding == nil {
		return fmt.Errorf("programlower: static writer is not initialized")
	}
	for _, stmt := range stmts {
		def, ok := stmt.(*ast.TypeDefStmt)
		if !ok {
			continue
		}
		decl, ok := w.binding.TypeDef(def)
		if !ok || decl.Kind != bind.TypeDeclAlias || decl.ID == 0 || decl.Name != def.Name {
			return fmt.Errorf("programlower: missing type alias binding for %q", def.Name)
		}
		if _, exists := w.terms[decl.ID]; exists {
			return fmt.Errorf("programlower: duplicate type alias identity for %q", def.Name)
		}
		term := w.builder.DeclareTypeAlias(w.span(def), body, def.Name)
		if term == 0 {
			return fmt.Errorf("programlower: could not predeclare type alias %q", def.Name)
		}
		w.terms[decl.ID] = term
	}
	return nil
}

// Place installs one alias's actual body cursor and ordered parameter hosts.
func (w *Writer) Place(def *ast.TypeDefStmt, gap int) error {
	alias, decl, err := w.alias(def)
	if err != nil {
		return err
	}
	if !w.builder.SetTypeAliasGap(alias, gap) {
		return fmt.Errorf("programlower: could not place type alias %q", decl.Name)
	}
	params := w.binding.TypeDefParams(def)
	terms := make([]program.Term, 0, len(params))
	for _, param := range params {
		if param.ID == 0 || param.Kind != bind.TypeDeclParam || param.Name == "" {
			return fmt.Errorf("programlower: invalid type parameter on %q", decl.Name)
		}
		if _, exists := w.terms[param.ID]; exists {
			return fmt.Errorf("programlower: duplicate type parameter identity %q", param.Name)
		}
		term := w.builder.DeclareTypeParam(w.span(def), alias, param.Name)
		if term == 0 {
			return fmt.Errorf("programlower: could not declare type parameter %q", param.Name)
		}
		w.terms[param.ID] = term
		terms = append(terms, term)
	}
	if !w.builder.SetTypeAliasParams(alias, terms) {
		return fmt.Errorf("programlower: could not set type parameters for %q", decl.Name)
	}
	return nil
}

// Host returns the exact Program host for a bound alias or type parameter.
func (w *Writer) Host(decl bind.TypeDecl) (program.Term, bool) {
	if w == nil || decl.ID == 0 {
		return 0, false
	}
	term, ok := w.terms[decl.ID]
	return term, ok
}

// Alias returns the exact Program alias host for def.
func (w *Writer) Alias(def *ast.TypeDefStmt) (program.Term, bool) {
	term, _, err := w.alias(def)
	return term, err == nil
}

// FinishParam fills a predeclared parameter's one exact constraint attachment.
// A zero constraint explicitly denotes an unconstrained parameter.
func (w *Writer) FinishParam(decl bind.TypeDecl, constraint program.Term) error {
	term, ok := w.Host(decl)
	if !ok || decl.Kind != bind.TypeDeclParam || !w.builder.FillTypeParam(term, constraint) {
		return fmt.Errorf("programlower: could not finalize type parameter %q", decl.Name)
	}
	return nil
}

// FinishAlias fills a predeclared, placed alias with its already-lowered type.
func (w *Writer) FinishAlias(def *ast.TypeDefStmt, target program.Term) error {
	alias, decl, err := w.alias(def)
	if err != nil {
		return err
	}
	if !w.builder.FillTypeAlias(alias, target) {
		return fmt.Errorf("programlower: could not finalize type alias %q", decl.Name)
	}
	return nil
}

// Leaf lowers one static type leaf. handled is false for structural and typeof
// nodes so an iterative caller can schedule their children explicitly.
func (w *Writer) Leaf(expr ast.TypeExpr) (term program.Term, handled bool, err error) {
	if w == nil || w.builder == nil || w.binding == nil || expr == nil {
		return 0, true, fmt.Errorf("programlower: invalid static type leaf")
	}
	switch expr := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		term, err = w.PrimitiveOrRef(expr)
		return term, true, err
	case *ast.LiteralTypeExpr:
		term, err = w.Literal(expr)
		return term, true, err
	case *ast.TypeRefExpr:
		term, err = w.TypeRef(expr)
		return term, true, err
	default:
		return 0, false, nil
	}
}

// PrimitiveOrRef lowers a parser primitive token either as its closed built-in
// kind or as a binder-resolved/unresolved bare declaration reference.
func (w *Writer) PrimitiveOrRef(expr *ast.PrimitiveTypeExpr) (program.Term, error) {
	if w == nil || w.builder == nil || w.binding == nil || expr == nil || expr.Name == "" {
		return 0, fmt.Errorf("programlower: invalid primitive type")
	}
	span := w.span(expr)
	if decl, ok := w.binding.PrimitiveTypeRef(expr); ok {
		return w.declarationRef(span, "", expr.Name, decl)
	}
	if kind, ok := primitive(expr.Name); ok {
		return w.term(w.builder.Primitive(span, kind), "primitive type")
	}
	return w.term(w.builder.UnresolvedTypeRef(span, "", expr.Name), "unresolved primitive reference")
}

// Literal lowers a parser literal type without accepting opaque values.
func (w *Writer) Literal(expr *ast.LiteralTypeExpr) (program.Term, error) {
	if w == nil || w.builder == nil || expr == nil {
		return 0, fmt.Errorf("programlower: invalid literal type")
	}
	span := w.span(expr)
	switch value := expr.Value.(type) {
	case bool:
		return w.term(w.builder.TypeBool(span, value), "boolean literal type")
	case int64:
		return w.term(w.builder.TypeInteger(span, value), "integer literal type")
	case float64:
		return w.term(w.builder.TypeFloat(span, value), "float literal type")
	case string:
		return w.term(w.builder.TypeString(span, value), "string literal type")
	case nil:
		return 0, fmt.Errorf("programlower: malformed numeric literal type")
	default:
		return 0, fmt.Errorf("programlower: unsupported literal type %T", value)
	}
}

// Optional completes an OptionalTypeExpr after its one child has been lowered.
func (w *Writer) Optional(expr *ast.OptionalTypeExpr, inner program.Term) (program.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("programlower: nil optional type")
	}
	return w.term(w.builder.Optional(w.span(expr), inner), "optional type")
}

// Union completes a UnionTypeExpr from one ordered child range.
func (w *Writer) Union(expr *ast.UnionTypeExpr, mark, count int) (program.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("programlower: nil union type")
	}
	members, err := w.rangeTerms(mark, count)
	if err != nil {
		return 0, err
	}
	return w.term(w.builder.Union(w.span(expr), members), "union type")
}

// Intersection completes an IntersectionTypeExpr after its ordered members
// have been lowered.
func (w *Writer) Intersection(expr *ast.IntersectionTypeExpr, mark, count int) (program.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("programlower: nil intersection type")
	}
	members, err := w.rangeTerms(mark, count)
	if err != nil {
		return 0, err
	}
	return w.term(w.builder.Intersection(w.span(expr), members), "intersection type")
}

// Generic completes a GenericTypeExpr after its base and ordered arguments are
// lowered by the caller.
func (w *Writer) Generic(expr *ast.GenericTypeExpr, base program.Term, mark, count int) (program.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("programlower: nil generic type")
	}
	args, err := w.rangeTerms(mark, count)
	if err != nil {
		return 0, err
	}
	return w.term(w.builder.Generic(w.span(expr), base, args), "generic type")
}

// TypeOf lowers typeof with the exact declaration/parameter host and an
// already-lowered ordinary expression operand.
func (w *Writer) TypeOf(expr *ast.TypeOfExpr, host, operand program.Term) (program.Term, error) {
	if w == nil || w.builder == nil || expr == nil || operand == 0 {
		return 0, fmt.Errorf("programlower: invalid typeof")
	}
	return w.term(w.builder.TypeOf(w.span(expr), host, operand), "typeof")
}

// TypeRef lowers a bare or qualified parser reference using binder evidence.
func (w *Writer) TypeRef(ref *ast.TypeRefExpr) (program.Term, error) {
	if ref == nil || len(ref.Path) == 0 || len(ref.Path) > 2 || ref.Path[len(ref.Path)-1] == "" || (len(ref.Path) == 2 && ref.Path[0] == "") {
		return 0, fmt.Errorf("programlower: invalid type reference")
	}
	pkg, name := sourceRef(ref.Path)
	span := w.span(ref)
	if decl, ok := w.binding.TypeRef(ref); ok {
		return w.declarationRef(span, pkg, name, decl)
	}
	if resolved, ok := w.binding.QualifiedTypeRef(ref); ok {
		if resolved.Decl.ID != 0 {
			return w.declarationRef(span, pkg, name, resolved.Decl)
		}
		if len(resolved.Path) != 0 {
			path := make([]program.Key, 0, len(resolved.Path))
			for _, part := range resolved.Path {
				key := w.builder.TypeKey(part)
				if key == 0 {
					return 0, fmt.Errorf("programlower: invalid canonical type path")
				}
				path = append(path, key)
			}
			return w.term(w.builder.QualifiedTypeRef(span, pkg, name, path), "qualified type reference")
		}
	}
	return w.term(w.builder.UnresolvedTypeRef(span, pkg, name), "unresolved type reference")
}

func (w *Writer) declarationRef(span program.Span, pkg, name string, decl bind.TypeDecl) (program.Term, error) {
	target, ok := w.Host(decl)
	if !ok || (decl.Kind != bind.TypeDeclAlias && decl.Kind != bind.TypeDeclParam) {
		return 0, fmt.Errorf("programlower: unavailable type declaration %q", decl.Name)
	}
	return w.term(w.builder.TypeRef(span, pkg, name, target), "declaration type reference")
}

func (w *Writer) alias(def *ast.TypeDefStmt) (program.Term, bind.TypeDecl, error) {
	if w == nil || w.binding == nil || def == nil {
		return 0, bind.TypeDecl{}, fmt.Errorf("programlower: invalid type alias")
	}
	decl, ok := w.binding.TypeDef(def)
	if !ok || decl.Kind != bind.TypeDeclAlias {
		return 0, bind.TypeDecl{}, fmt.Errorf("programlower: missing type alias binding")
	}
	term, ok := w.Host(decl)
	if !ok {
		return 0, bind.TypeDecl{}, fmt.Errorf("programlower: type alias %q was not predeclared", decl.Name)
	}
	return term, decl, nil
}

func (w *Writer) term(term program.Term, what string) (program.Term, error) {
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create %s", what)
	}
	return term, nil
}

func (w *Writer) rangeTerms(mark, count int) ([]program.Term, error) {
	if w == nil || mark < 0 || count < 0 || mark > len(w.children) || len(w.children)-mark != count {
		return nil, fmt.Errorf("programlower: incomplete static type children")
	}
	terms := w.children[mark:]
	w.children = w.children[:mark]
	return terms, nil
}

func (w *Writer) span(holder ast.PositionHolder) program.Span {
	if holder == nil {
		return program.Span{File: w.sourceName}
	}
	endLine, endCol := holder.LastLine(), holder.LastColumn()
	if endLine <= 0 || endCol <= 0 {
		endLine, endCol = 0, 0
	}
	return program.Span{File: w.sourceName, StartLine: holder.Line(), StartCol: holder.Column(), EndLine: endLine, EndCol: endCol}
}

func sourceRef(path []string) (pkg, name string) {
	name = path[len(path)-1]
	if len(path) == 2 {
		pkg = path[0]
	}
	return pkg, name
}

func primitive(name string) (program.PrimitiveKind, bool) {
	switch name {
	case "nil":
		return program.PrimitiveNil, true
	case "boolean":
		return program.PrimitiveBoolean, true
	case "number":
		return program.PrimitiveNumber, true
	case "integer":
		return program.PrimitiveInteger, true
	case "string":
		return program.PrimitiveString, true
	case "function":
		return program.PrimitiveFunction, true
	case "any":
		return program.PrimitiveAny, true
	case "unknown":
		return program.PrimitiveUnknown, true
	case "never":
		return program.PrimitiveNever, true
	case "self":
		return program.PrimitiveSelf, true
	default:
		return 0, false
	}
}
