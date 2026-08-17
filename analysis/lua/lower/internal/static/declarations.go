package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Predeclare records identities for the direct alias and interface statements
// in body.
// Parameters are deliberately deferred until the lowering walk reaches the
// declaration's authored source turn.
func (w *Writer) Predeclare(body keyspace.Term, stmts []ast.Stmt) error {
	if w == nil || w.binding == nil {
		return fmt.Errorf("lualower: static writer is not initialized")
	}
	for _, stmt := range stmts {
		switch def := stmt.(type) {
		case *ast.TypeDefStmt:
			if w.terms == nil {
				w.terms = make(map[bind.TypeDeclID]keyspace.Term)
			}
			decl, ok := w.binding.TypeDef(def)
			if !ok || decl.Kind != bind.TypeDeclAlias || decl.ID == 0 || decl.Name != def.Name {
				return fmt.Errorf("lualower: missing type alias binding for %q", def.Name)
			}
			if _, exists := w.terms[decl.ID]; exists {
				return fmt.Errorf("lualower: duplicate type alias identity for %q", def.Name)
			}
			term := w.static.Alias(
				w.span(def), w.nameSpan(def.NamePosition), body, def.Name,
			)
			if term == 0 {
				return fmt.Errorf("lualower: could not predeclare type alias %q", def.Name)
			}
			w.terms[decl.ID] = term
		case *ast.InterfaceDefStmt:
			if w.terms == nil {
				w.terms = make(map[bind.TypeDeclID]keyspace.Term)
			}
			decl, ok := w.binding.InterfaceDef(def)
			if !ok || decl.Kind != bind.TypeDeclInterface || decl.ID == 0 || decl.Name != def.Name {
				return fmt.Errorf("lualower: missing interface binding for %q", def.Name)
			}
			if _, exists := w.terms[decl.ID]; exists {
				return fmt.Errorf("lualower: duplicate interface identity for %q", def.Name)
			}
			term := w.static.Interface(
				w.span(def), w.nameSpan(def.NamePosition), body, def.Name,
			)
			if term == 0 {
				return fmt.Errorf("lualower: could not predeclare interface %q", def.Name)
			}
			w.terms[decl.ID] = term
		}
	}
	return nil
}

// BeginAlias installs one predeclared alias's ordered parameter hosts and
// returns the existing identity for the active Body source sequence.
func (w *Writer) BeginAlias(def *ast.TypeDefStmt) (keyspace.Term, error) {
	alias, decl, err := w.alias(def)
	if err != nil {
		return 0, err
	}
	params := w.binding.TypeDefParams(def)
	if len(params) != len(def.TypeParams) {
		return 0, fmt.Errorf("lualower: type alias parameter binding/source count mismatch for %q", decl.Name)
	}
	terms := make([]keyspace.Term, 0, len(params))
	for index, param := range params {
		if param.ID == 0 || param.Kind != bind.TypeDeclParam || param.Name == "" {
			return 0, fmt.Errorf("lualower: invalid type parameter on %q", decl.Name)
		}
		if _, exists := w.terms[param.ID]; exists {
			return 0, fmt.Errorf("lualower: duplicate type parameter identity %q", param.Name)
		}
		if index >= len(def.TypeParams) || def.TypeParams[index].Name != param.Name {
			return 0, fmt.Errorf("lualower: invalid type parameter source position %q", param.Name)
		}
		term := w.static.TypeParam(w.nameSpan(param.NamePosition), alias, param.Name)
		if term == 0 {
			return 0, fmt.Errorf("lualower: could not declare type parameter %q", param.Name)
		}
		w.terms[param.ID] = term
		terms = append(terms, term)
	}
	if !w.static.AliasParams(alias, terms) {
		return 0, fmt.Errorf("lualower: could not set type parameters for %q", decl.Name)
	}
	return alias, nil
}

// Host returns the exact Program host for a bound alias, interface, or type
// parameter.
func (w *Writer) Host(decl bind.TypeDecl) (keyspace.Term, bool) {
	if w == nil || decl.ID == 0 {
		return 0, false
	}
	term, ok := w.terms[decl.ID]
	return term, ok
}

// FinishParam fills a predeclared parameter's one exact constraint attachment.
// A zero constraint explicitly denotes an unconstrained parameter.
func (w *Writer) FinishParam(decl bind.TypeDecl, constraint keyspace.Term) error {
	term, ok := w.Host(decl)
	if !ok || decl.Kind != bind.TypeDeclParam || !w.static.TypeParamConstraint(term, constraint) {
		return fmt.Errorf("lualower: could not finalize type parameter %q", decl.Name)
	}
	return nil
}

// FinishAlias fills a predeclared source-indexed alias with its lowered type.
func (w *Writer) FinishAlias(def *ast.TypeDefStmt, target keyspace.Term) error {
	alias, decl, err := w.alias(def)
	if err != nil {
		return err
	}
	if !w.static.AliasTarget(alias, target) {
		return fmt.Errorf("lualower: could not finalize type alias %q", decl.Name)
	}
	return nil
}

func (w *Writer) declarationRef(span source.Span, source []string, decl bind.TypeDecl) (keyspace.Term, error) {
	target, err := w.declarationTarget(decl)
	if err != nil {
		return 0, err
	}
	if len(source) == 0 {
		return 0, fmt.Errorf("lualower: invalid declaration type reference path")
	}
	for _, part := range source {
		if part == "" {
			return 0, fmt.Errorf("lualower: empty declaration type reference component")
		}
	}
	return w.term(w.static.Declaration(span, source, 0, target), "declaration type reference")
}

func (w *Writer) declarationTarget(decl bind.TypeDecl) (keyspace.Term, error) {
	target, ok := w.Host(decl)
	if !ok || (decl.Kind != bind.TypeDeclAlias &&
		decl.Kind != bind.TypeDeclInterface && decl.Kind != bind.TypeDeclParam) {
		return 0, fmt.Errorf("lualower: unavailable type declaration %q", decl.Name)
	}
	return target, nil
}

func (w *Writer) alias(def *ast.TypeDefStmt) (keyspace.Term, bind.TypeDecl, error) {
	if w == nil || w.binding == nil || def == nil {
		return 0, bind.TypeDecl{}, fmt.Errorf("lualower: invalid type alias")
	}
	decl, ok := w.binding.TypeDef(def)
	if !ok || decl.Kind != bind.TypeDeclAlias {
		return 0, bind.TypeDecl{}, fmt.Errorf("lualower: missing type alias binding")
	}
	term, ok := w.Host(decl)
	if !ok {
		return 0, bind.TypeDecl{}, fmt.Errorf("lualower: type alias %q was not predeclared", decl.Name)
	}
	return term, decl, nil
}
