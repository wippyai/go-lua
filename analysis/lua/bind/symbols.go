package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type globalSymbol struct {
	id          symbol.ID
	predeclared bool
}

// LocalOrigin identifies the local declaration slot that introduced a symbol.
type LocalOrigin struct {
	Stmt  *ast.LocalAssignStmt
	Index int
}

// SymbolOf returns the declaration symbol bound to an identifier occurrence.
func (r *Result) SymbolOf(ident *ast.IdentExpr) (symbol.ID, bool) {
	if r == nil || ident == nil {
		return 0, false
	}
	id, ok := r.identSymbols[ident]
	return id, ok
}

// ReadIdents returns identifier read occurrences bound to id.
func (r *Result) ReadIdents(id symbol.ID) []*ast.IdentExpr {
	if r == nil || id == 0 {
		return nil
	}
	return cloneIdentExprs(r.readIdents[id])
}

// HasRead reports whether id has at least one identifier read occurrence.
func (r *Result) HasRead(id symbol.ID) bool {
	if r == nil || id == 0 {
		return false
	}
	return len(r.readIdents[id]) > 0
}

// FuncDefTargetSymbol returns the simple assignment target for a function
// definition of the form "function f(...) ... end".
func (r *Result) FuncDefTargetSymbol(stmt *ast.FuncDefStmt) (symbol.ID, bool) {
	if r == nil || stmt == nil || stmt.Name == nil {
		return 0, false
	}
	if stmt.Name.Receiver != nil || stmt.Name.Method != "" {
		return 0, false
	}
	ident, ok := stmt.Name.Func.(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	id, ok := r.SymbolOf(ident)
	return id, ok && id != 0
}

// IsImplicitGlobalUse reports whether ident is an unresolved read that created
// an implicit global symbol.
func (r *Result) IsImplicitGlobalUse(ident *ast.IdentExpr) bool {
	if r == nil || ident == nil {
		return false
	}
	_, ok := r.implicitGlobalUses[ident]
	return ok
}

// IsImplicitGlobalSymbol reports whether id was created by an unresolved
// global read rather than by the configured global set or a write target.
func (r *Result) IsImplicitGlobalSymbol(id symbol.ID) bool {
	if r == nil || id == 0 {
		return false
	}
	_, ok := r.implicitGlobalSymbols[id]
	return ok
}

// ResolvesToGlobal reports whether ident is bound to the global named name.
func (r *Result) ResolvesToGlobal(ident *ast.IdentExpr, name string) bool {
	if r == nil || ident == nil || name == "" || ident.Value != name {
		return false
	}
	id, ok := r.SymbolOf(ident)
	if !ok || id == 0 || r.Name(id) != name {
		return false
	}
	kind, ok := r.Kind(id)
	return ok && kind == symbol.Global
}

// GlobalSymbol returns the declaration symbol for a configured or discovered
// global name.
func (r *Result) GlobalSymbol(name string) (symbol.ID, bool) {
	if r == nil || name == "" {
		return 0, false
	}
	g, ok := r.globals[name]
	return g.id, ok && g.id != 0
}

// Name returns the declaration name for a symbol.
func (r *Result) Name(id symbol.ID) string {
	if r == nil {
		return ""
	}
	return r.names[id]
}

// Kind returns the declaration kind for a symbol.
func (r *Result) Kind(id symbol.ID) (symbol.Kind, bool) {
	if r == nil {
		return symbol.Unknown, false
	}
	kind, ok := r.kinds[id]
	return kind, ok
}

// LocalSymbols returns ordered local symbols declared by stmt.
func (r *Result) LocalSymbols(stmt *ast.LocalAssignStmt) []symbol.ID {
	if r == nil || stmt == nil {
		return nil
	}
	return cloneSymbols(r.localSymbols[stmt])
}

// LocalSymbolAt returns the local symbol at index for stmt.
func (r *Result) LocalSymbolAt(stmt *ast.LocalAssignStmt, index int) (symbol.ID, bool) {
	if r == nil || stmt == nil || index < 0 {
		return 0, false
	}
	ids := r.localSymbols[stmt]
	if index >= len(ids) {
		return 0, false
	}
	return ids[index], true
}

// LocalOrigin returns the declaration statement and slot for a local symbol.
func (r *Result) LocalOrigin(id symbol.ID) (LocalOrigin, bool) {
	if r == nil || id == 0 {
		return LocalOrigin{}, false
	}
	for stmt, ids := range r.localSymbols {
		for i, sym := range ids {
			if sym == id {
				return LocalOrigin{Stmt: stmt, Index: i}, true
			}
		}
	}
	return LocalOrigin{}, false
}

// SymbolTypeAnnotation returns the declared type expression for a parameter or
// local declaration symbol.
func (r *Result) SymbolTypeAnnotation(id symbol.ID) (ast.TypeExpr, bool) {
	if r == nil || id == 0 {
		return nil, false
	}
	if fn, ok := r.DeclaringFunction(id); ok {
		for _, slot := range r.ParamSlots(fn) {
			if slot.Symbol == id && slot.Type != nil {
				return slot.Type, true
			}
		}
	}
	for stmt, ids := range r.localSymbols {
		for i, sym := range ids {
			if sym == id && i < len(stmt.Types) && stmt.Types[i] != nil {
				return stmt.Types[i], true
			}
		}
	}
	return nil, false
}

// NumForSymbol returns the loop variable symbol for a numeric for statement.
func (r *Result) NumForSymbol(stmt *ast.NumberForStmt) (symbol.ID, bool) {
	if r == nil || stmt == nil {
		return 0, false
	}
	id, ok := r.numForSymbols[stmt]
	return id, ok
}

// GenericForSymbols returns ordered loop variable symbols for a generic for.
func (r *Result) GenericForSymbols(stmt *ast.GenericForStmt) []symbol.ID {
	if r == nil || stmt == nil {
		return nil
	}
	return cloneSymbols(r.genericForSymbols[stmt])
}

func (r *Result) newSymbol(name string, kind symbol.Kind) symbol.ID {
	id := symbol.Next()
	r.names[id] = name
	r.kinds[id] = kind
	return id
}

func (r *Result) global(name string, predeclared bool) symbol.ID {
	if g, ok := r.globals[name]; ok {
		if predeclared && !g.predeclared {
			g.predeclared = true
			r.globals[name] = g
		}
		return g.id
	}
	id := r.newSymbol(name, symbol.Global)
	r.globals[name] = globalSymbol{id: id, predeclared: predeclared}
	return id
}

type scope struct {
	names map[string]symbol.ID
}

type deferredScope struct {
	scopeIndex int
	names      map[string]symbol.ID
}

type binder struct {
	result     *Result
	scopes     []scope
	typeScopes []typeScope

	functionStack []*ast.FunctionExpr

	deferred        []deferredScope
	visibleDeferred int
}

func (b *binder) pushScope() {
	b.scopes = append(b.scopes, scope{names: make(map[string]symbol.ID)})
	b.pushTypeScope()
}

func (b *binder) popScope() {
	if len(b.scopes) == 0 {
		return
	}
	b.scopes = b.scopes[:len(b.scopes)-1]
	b.popTypeScope()
}

func (b *binder) define(name string, id symbol.ID) {
	if name == "" || len(b.scopes) == 0 || id == 0 {
		return
	}
	b.scopes[len(b.scopes)-1].names[name] = id
}

func (b *binder) newSymbol(name string, kind symbol.Kind) symbol.ID {
	id := b.result.newSymbol(name, kind)
	if fn := b.currentFunction(); fn != nil {
		b.result.declaringFunctions[id] = fn
	}
	return id
}

func (b *binder) lookup(name string) (symbol.ID, bool, bool) {
	if name == "" {
		return 0, false, false
	}
	for i := len(b.scopes) - 1; i >= 0; i-- {
		for j := b.visibleDeferred - 1; j >= 0; j-- {
			if b.deferred[j].scopeIndex != i {
				continue
			}
			if id, ok := b.deferred[j].names[name]; ok {
				return id, false, true
			}
		}
		if id, ok := b.scopes[i].names[name]; ok {
			return id, false, true
		}
	}
	if g, ok := b.result.globals[name]; ok {
		return g.id, true, true
	}
	return 0, false, false
}

func (b *binder) bindLValue(expr ast.Expr) {
	switch expr := expr.(type) {
	case nil:
	case *ast.IdentExpr:
		b.bindWriteIdent(expr)
	case *ast.AttrGetExpr:
		b.bindExpr(expr.Object)
		if expr.KeySyntax != ast.AttrKeyDot {
			b.bindExpr(expr.Key)
		}
	default:
		b.bindExpr(expr)
	}
}

func (b *binder) bindReadIdent(ident *ast.IdentExpr) {
	id := b.bindReadIdentSymbol(ident)
	if id == 0 {
		return
	}
	b.result.readIdents[id] = append(b.result.readIdents[id], ident)
	b.recordDirectCapture(id)
}

func (b *binder) bindTypeQueryIdent(ident *ast.IdentExpr) {
	b.bindReadIdentSymbol(ident)
}

func (b *binder) bindReadIdentSymbol(ident *ast.IdentExpr) symbol.ID {
	if ident == nil {
		return 0
	}
	id, _, ok := b.lookup(ident.Value)
	if !ok {
		if decl, hasType := b.lookupType(ident.Value); hasType {
			b.result.typeValueRefs[ident] = decl
		}
		id = b.result.global(ident.Value, false)
		b.result.implicitGlobalUses[ident] = struct{}{}
		b.result.implicitGlobalSymbols[id] = struct{}{}
	} else if _, isImplicitGlobal := b.result.implicitGlobalSymbols[id]; isImplicitGlobal {
		if decl, hasType := b.lookupType(ident.Value); hasType {
			b.result.typeValueRefs[ident] = decl
		}
	}
	b.result.identSymbols[ident] = id
	return id
}

func (b *binder) bindWriteIdent(ident *ast.IdentExpr) {
	if ident == nil {
		return
	}
	id, _, ok := b.lookup(ident.Value)
	if !ok {
		id = b.result.global(ident.Value, false)
	}
	b.result.identSymbols[ident] = id
	b.recordDirectCapture(id)
}
