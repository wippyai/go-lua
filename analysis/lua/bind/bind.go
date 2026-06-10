package bind

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Options configures lexical binding.
type Options struct {
	Globals []string
}

// Result records lexical declaration identities for identifier occurrences.
type Result struct {
	identSymbols       map[*ast.IdentExpr]symbol.ID
	implicitGlobalUses map[*ast.IdentExpr]struct{}

	names map[symbol.ID]string
	kinds map[symbol.ID]symbol.Kind

	globals map[string]globalSymbol

	paramSymbols      map[*ast.FunctionExpr][]symbol.ID
	localSymbols      map[*ast.LocalAssignStmt][]symbol.ID
	numForSymbols     map[*ast.NumberForStmt]symbol.ID
	genericForSymbols map[*ast.GenericForStmt][]symbol.ID
}

type globalSymbol struct {
	id          symbol.ID
	predeclared bool
}

// BindFunction binds a single function expression with a fresh global seed.
func BindFunction(fn *ast.FunctionExpr, opts Options) *Result {
	r := newResult(opts)
	b := binder{result: r}
	b.bindFunction(fn, false)
	return r
}

// BindChunk binds a chunk statement list with a fresh global seed.
func BindChunk(stmts []ast.Stmt, opts Options) *Result {
	r := newResult(opts)
	b := binder{result: r}
	b.pushScope()
	b.bindStmts(stmts)
	b.popScope()
	return r
}

// PredeclaredGlobalNames returns deterministic non-empty global names.
func PredeclaredGlobalNames[T any](globals map[string]T) []string {
	names := make([]string, 0, len(globals))
	for name := range globals {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// SymbolOf returns the declaration symbol bound to an identifier occurrence.
func (r *Result) SymbolOf(ident *ast.IdentExpr) (symbol.ID, bool) {
	if r == nil || ident == nil {
		return 0, false
	}
	id, ok := r.identSymbols[ident]
	return id, ok
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

// ParamSymbols returns ordered parameter symbols for fn.
func (r *Result) ParamSymbols(fn *ast.FunctionExpr) []symbol.ID {
	if r == nil || fn == nil {
		return nil
	}
	return cloneSymbols(r.paramSymbols[fn])
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

func newResult(opts Options) *Result {
	r := &Result{
		identSymbols:       make(map[*ast.IdentExpr]symbol.ID),
		implicitGlobalUses: make(map[*ast.IdentExpr]struct{}),
		names:              make(map[symbol.ID]string),
		kinds:              make(map[symbol.ID]symbol.Kind),
		globals:            make(map[string]globalSymbol),
		paramSymbols:       make(map[*ast.FunctionExpr][]symbol.ID),
		localSymbols:       make(map[*ast.LocalAssignStmt][]symbol.ID),
		numForSymbols:      make(map[*ast.NumberForStmt]symbol.ID),
		genericForSymbols:  make(map[*ast.GenericForStmt][]symbol.ID),
	}
	for _, name := range normalizeNames(opts.Globals) {
		r.global(name, true)
	}
	return r
}

func normalizeNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func cloneSymbols(ids []symbol.ID) []symbol.ID {
	if len(ids) == 0 {
		return nil
	}
	return append([]symbol.ID(nil), ids...)
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
	result *Result
	scopes []scope

	deferred        []deferredScope
	visibleDeferred int
}

func (b *binder) pushScope() {
	b.scopes = append(b.scopes, scope{names: make(map[string]symbol.ID)})
}

func (b *binder) popScope() {
	if len(b.scopes) == 0 {
		return
	}
	b.scopes = b.scopes[:len(b.scopes)-1]
}

func (b *binder) define(name string, id symbol.ID) {
	if name == "" || len(b.scopes) == 0 || id == 0 {
		return
	}
	b.scopes[len(b.scopes)-1].names[name] = id
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

func (b *binder) bindStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		b.bindStmt(stmt)
	}
}

func (b *binder) bindStmt(stmt ast.Stmt) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		b.bindExprs(stmt.Rhs)
		for _, lhs := range stmt.Lhs {
			b.bindLValue(lhs)
		}
	case *ast.LocalAssignStmt:
		b.bindLocalAssign(stmt)
	case *ast.FuncCallStmt:
		b.bindExpr(stmt.Expr)
	case *ast.DoBlockStmt:
		b.pushScope()
		b.bindStmts(stmt.Stmts)
		b.popScope()
	case *ast.WhileStmt:
		b.bindExpr(stmt.Condition)
		b.pushScope()
		b.bindStmts(stmt.Stmts)
		b.popScope()
	case *ast.RepeatStmt:
		b.pushScope()
		b.bindStmts(stmt.Stmts)
		b.bindExpr(stmt.Condition)
		b.popScope()
	case *ast.IfStmt:
		b.bindExpr(stmt.Condition)
		b.pushScope()
		b.bindStmts(stmt.Then)
		b.popScope()
		if len(stmt.Else) > 0 {
			b.pushScope()
			b.bindStmts(stmt.Else)
			b.popScope()
		}
	case *ast.NumberForStmt:
		b.bindNumberFor(stmt)
	case *ast.GenericForStmt:
		b.bindGenericFor(stmt)
	case *ast.FuncDefStmt:
		b.bindFuncDef(stmt)
	case *ast.ReturnStmt:
		b.bindExprs(stmt.Exprs)
	case *ast.BreakStmt, *ast.LabelStmt, *ast.GotoStmt:
	case *ast.TypeDefStmt, *ast.InterfaceDefStmt:
	}
}

func (b *binder) bindLocalAssign(stmt *ast.LocalAssignStmt) {
	ids := make([]symbol.ID, len(stmt.Names))
	pending := make(map[string]symbol.ID, len(stmt.Names))
	for i, name := range stmt.Names {
		id := b.result.newSymbol(name, symbol.Local)
		ids[i] = id
		if name != "" {
			pending[name] = id
		}
	}
	b.result.localSymbols[stmt] = ids

	oldDeferredLen := len(b.deferred)
	if len(pending) > 0 && len(b.scopes) > 0 {
		b.deferred = append(b.deferred, deferredScope{
			scopeIndex: len(b.scopes) - 1,
			names:      pending,
		})
	}
	b.bindExprs(stmt.Exprs)
	b.deferred = b.deferred[:oldDeferredLen]
	if b.visibleDeferred > len(b.deferred) {
		b.visibleDeferred = len(b.deferred)
	}

	for i, name := range stmt.Names {
		b.define(name, ids[i])
	}
}

func (b *binder) bindNumberFor(stmt *ast.NumberForStmt) {
	b.bindExpr(stmt.Init)
	b.bindExpr(stmt.Limit)
	b.bindExpr(stmt.Step)

	id := b.result.newSymbol(stmt.Name, symbol.Local)
	b.result.numForSymbols[stmt] = id

	b.pushScope()
	b.define(stmt.Name, id)
	b.bindStmts(stmt.Stmts)
	b.popScope()
}

func (b *binder) bindGenericFor(stmt *ast.GenericForStmt) {
	b.bindExprs(stmt.Exprs)

	ids := make([]symbol.ID, len(stmt.Names))
	b.pushScope()
	for i, name := range stmt.Names {
		id := b.result.newSymbol(name, symbol.Local)
		ids[i] = id
		b.define(name, id)
	}
	b.result.genericForSymbols[stmt] = ids
	b.bindStmts(stmt.Stmts)
	b.popScope()
}

func (b *binder) bindFuncDef(stmt *ast.FuncDefStmt) {
	if stmt.Name != nil {
		if stmt.Name.Func != nil {
			b.bindLValue(stmt.Name.Func)
		}
		if stmt.Name.Receiver != nil {
			b.bindExpr(stmt.Name.Receiver)
		}
	}
	b.bindFunction(stmt.Func, stmt.Name != nil && stmt.Name.Method != "")
}

func (b *binder) bindExprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		b.bindExpr(expr)
	}
}

func (b *binder) bindExpr(expr ast.Expr) {
	switch expr := expr.(type) {
	case nil:
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr, *ast.Comma3Expr:
	case *ast.IdentExpr:
		b.bindReadIdent(expr)
	case *ast.AttrGetExpr:
		b.bindExpr(expr.Object)
		if expr.KeySyntax != ast.AttrKeyDot {
			b.bindExpr(expr.Key)
		}
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax != ast.AttrKeyDot {
				b.bindExpr(field.Key)
			}
			b.bindExpr(field.Value)
		}
	case *ast.FuncCallExpr:
		b.bindExpr(expr.Func)
		b.bindExpr(expr.Receiver)
		b.bindExprs(expr.Args)
	case *ast.LogicalOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.RelationalOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.StringConcatOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.ArithmeticOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.UnaryNotOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.UnaryLenOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.UnaryBNotOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.FunctionExpr:
		b.bindFunction(expr, false)
	case *ast.CastExpr:
		b.bindExpr(expr.Expr)
	case *ast.NonNilAssertExpr:
		b.bindExpr(expr.Expr)
	}
}

func (b *binder) bindFunction(fn *ast.FunctionExpr, method bool) {
	if fn == nil {
		return
	}

	oldVisibleDeferred := b.visibleDeferred
	b.visibleDeferred = len(b.deferred)

	b.pushScope()
	params := make([]symbol.ID, 0)
	names := []string(nil)
	if fn.ParList != nil {
		names = fn.ParList.Names
	}
	if method && (len(names) == 0 || names[0] != "self") {
		id := b.result.newSymbol("self", symbol.Param)
		params = append(params, id)
		b.define("self", id)
	}
	for _, name := range names {
		id := b.result.newSymbol(name, symbol.Param)
		params = append(params, id)
		b.define(name, id)
	}
	b.result.paramSymbols[fn] = params
	b.bindStmts(fn.Stmts)
	b.popScope()

	b.visibleDeferred = oldVisibleDeferred
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
	if ident == nil {
		return
	}
	id, _, ok := b.lookup(ident.Value)
	if !ok {
		id = b.result.global(ident.Value, false)
		b.result.implicitGlobalUses[ident] = struct{}{}
	}
	b.result.identSymbols[ident] = id
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
}
