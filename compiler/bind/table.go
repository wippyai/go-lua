// Package bind provides name resolution for Lua programs.
//
// The binder traverses AST nodes and resolves identifier references to
// unique symbols. This establishes the binding between uses and definitions,
// enabling type checking and CFG construction to work with stable identities.
//
// # Symbol Assignment
//
// Symbols are assigned to:
//   - Local variables at their declaration point
//   - Function parameters at function entry
//   - For loop iteration variables at loop entry
//   - Global references (assumed external)
//   - Field paths like M.f.g (each qualified access gets a symbol)
//
// # Usage
//
// Call [Bind] with a function AST and optional global names:
//
//	bindings := bind.Bind(funcExpr, "print", "error", "assert")
//
// The returned [BindingTable] provides:
//   - [BindingTable.SymbolOf]: Look up symbol for an IdentExpr
//   - [BindingTable.Name]: Get the source name for a symbol
//   - [BindingTable.Kind]: Get the declaration kind (local/param/global)
//   - [BindingTable.ParamSymbols]: Get parameter symbols for a function
package bind

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
)

// BindingTable stores the results of name resolution for a Lua program.
//
// The table provides bidirectional mapping between AST nodes and symbols:
//   - IdentExpr nodes map to their resolved SymbolID
//   - FunctionExpr nodes map to their parameter symbols
//   - LocalAssignStmt nodes map to their declared local symbols
//   - For loop statements map to their iteration variable symbols
//
// Each symbol has an associated kind (global, local, param) and name.
// The table also supports field paths (M.f.g) and anonymous function symbols.
//
// BindingTable is the primary output of the binding phase and is consumed
// by type checking and CFG construction phases.
type BindingTable struct {
	// symbols maps identifier references to their resolved symbols
	symbols map[*ast.IdentExpr]cfg.SymbolID

	// kind stores the declaration kind for each symbol
	kind map[cfg.SymbolID]cfg.SymbolKind

	// names stores the original source name for each symbol
	names map[cfg.SymbolID]string

	// paramSymbols maps functions to their parameter symbol list
	paramSymbols map[*ast.FunctionExpr][]cfg.SymbolID

	// localSymbols maps local declarations to their symbol list
	localSymbols map[*ast.LocalAssignStmt][]cfg.SymbolID

	// numForSymbols maps numeric for loops to their iteration variable
	numForSymbols map[*ast.NumberForStmt]cfg.SymbolID

	// genericForSymbols maps generic for loops to their iteration variables
	genericForSymbols map[*ast.GenericForStmt][]cfg.SymbolID

	// fieldSymbols maps qualified paths (base + field chain) to symbols
	fieldSymbols map[fieldPathKey]cfg.SymbolID

	// funcLitSymbols maps anonymous function expressions to symbols
	funcLitSymbols map[*ast.FunctionExpr]cfg.SymbolID

	// funcLitBySymbol maps symbols back to their function literals
	funcLitBySymbol map[cfg.SymbolID]*ast.FunctionExpr
}

// fieldPathKey identifies a field access path rooted at a base symbol.
// Used to give unique symbols to qualified names like M.f or M.f.g.
type fieldPathKey struct {
	base cfg.SymbolID
	path string
}

// NewBindingTable creates an empty binding table with all maps initialized.
func NewBindingTable() *BindingTable {
	return &BindingTable{
		symbols:           make(map[*ast.IdentExpr]cfg.SymbolID),
		kind:              make(map[cfg.SymbolID]cfg.SymbolKind),
		names:             make(map[cfg.SymbolID]string),
		paramSymbols:      make(map[*ast.FunctionExpr][]cfg.SymbolID),
		localSymbols:      make(map[*ast.LocalAssignStmt][]cfg.SymbolID),
		numForSymbols:     make(map[*ast.NumberForStmt]cfg.SymbolID),
		genericForSymbols: make(map[*ast.GenericForStmt][]cfg.SymbolID),
		fieldSymbols:      make(map[fieldPathKey]cfg.SymbolID),
		funcLitSymbols:    make(map[*ast.FunctionExpr]cfg.SymbolID),
		funcLitBySymbol:   make(map[cfg.SymbolID]*ast.FunctionExpr),
	}
}

// SymbolOf returns the symbol that an identifier reference resolves to.
// Returns zero and false if the identifier is nil or not bound.
func (t *BindingTable) SymbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool) {
	if ident == nil {
		return 0, false
	}
	sym, ok := t.symbols[ident]
	return sym, ok
}

// Bind records that an identifier references a specific symbol.
func (t *BindingTable) Bind(ident *ast.IdentExpr, sym cfg.SymbolID) {
	if ident == nil {
		return
	}
	t.symbols[ident] = sym
}

// SetKind records whether a symbol is a global, local, or parameter.
func (t *BindingTable) SetKind(sym cfg.SymbolID, k cfg.SymbolKind) {
	t.kind[sym] = k
}

// Kind returns the declaration kind of a symbol.
// Returns false if the symbol has no recorded kind.
func (t *BindingTable) Kind(sym cfg.SymbolID) (cfg.SymbolKind, bool) {
	k, ok := t.kind[sym]
	return k, ok
}

// SetName records the source name associated with a symbol.
func (t *BindingTable) SetName(sym cfg.SymbolID, name string) {
	t.names[sym] = name
}

// Name returns the source name of a symbol, or empty if unknown.
func (t *BindingTable) Name(sym cfg.SymbolID) string {
	return t.names[sym]
}

// SetParamSymbols records the ordered parameter symbols for a function.
// For methods with implicit self, self is the first symbol in the list.
func (t *BindingTable) SetParamSymbols(fn *ast.FunctionExpr, syms []cfg.SymbolID) {
	if fn == nil {
		return
	}
	t.paramSymbols[fn] = syms
}

// ParamSymbols returns the parameter symbols for a function in declaration order.
func (t *BindingTable) ParamSymbols(fn *ast.FunctionExpr) []cfg.SymbolID {
	return t.paramSymbols[fn]
}

// SetLocalSymbols records the symbols declared by a local assignment.
// For 'local a, b, c = ...', the symbols correspond to a, b, c in order.
func (t *BindingTable) SetLocalSymbols(stmt *ast.LocalAssignStmt, syms []cfg.SymbolID) {
	if stmt == nil {
		return
	}
	t.localSymbols[stmt] = syms
}

// LocalSymbols returns the symbols declared by a local assignment.
func (t *BindingTable) LocalSymbols(stmt *ast.LocalAssignStmt) []cfg.SymbolID {
	return t.localSymbols[stmt]
}

// SetNumForSymbol records the iteration variable for a numeric for loop.
// For 'for i = 1, 10 do ... end', this records the symbol for 'i'.
func (t *BindingTable) SetNumForSymbol(stmt *ast.NumberForStmt, sym cfg.SymbolID) {
	if stmt == nil {
		return
	}
	t.numForSymbols[stmt] = sym
}

// NumForSymbol returns the iteration variable symbol for a numeric for.
func (t *BindingTable) NumForSymbol(stmt *ast.NumberForStmt) (cfg.SymbolID, bool) {
	sym, ok := t.numForSymbols[stmt]
	return sym, ok
}

// SetGenericForSymbols records the iteration variables for a generic for loop.
// For 'for k, v in pairs(t) do ... end', this records symbols for k and v.
func (t *BindingTable) SetGenericForSymbols(stmt *ast.GenericForStmt, syms []cfg.SymbolID) {
	if stmt == nil {
		return
	}
	t.genericForSymbols[stmt] = syms
}

// GenericForSymbols returns the iteration variable symbols for a generic for.
func (t *BindingTable) GenericForSymbols(stmt *ast.GenericForStmt) []cfg.SymbolID {
	return t.genericForSymbols[stmt]
}

// GetOrCreateFieldSymbol returns or creates a symbol for a qualified field path.
//
// Field paths represent nested property access like M.f or M.f.g. The baseSym
// identifies the root object, and path is the dot-separated field chain.
//
// The created symbol gets a composite name (e.g., "M.f") and SymbolLocal kind.
// Subsequent calls with the same base and path return the existing symbol.
func (t *BindingTable) GetOrCreateFieldSymbol(baseSym cfg.SymbolID, path string) cfg.SymbolID {
	key := fieldPathKey{base: baseSym, path: path}
	if sym, ok := t.fieldSymbols[key]; ok {
		return sym
	}
	sym := cfg.NextSymbolID()
	t.fieldSymbols[key] = sym
	baseName := t.names[baseSym]
	if baseName != "" {
		t.names[sym] = baseName + "." + path
	} else {
		t.names[sym] = path
	}
	t.kind[sym] = cfg.SymbolLocal
	return sym
}

// FieldSymbol looks up an existing symbol for a field path.
// Returns zero and false if no symbol exists for the given base and path.
func (t *BindingTable) FieldSymbol(baseSym cfg.SymbolID, path string) (cfg.SymbolID, bool) {
	key := fieldPathKey{base: baseSym, path: path}
	sym, ok := t.fieldSymbols[key]
	return sym, ok
}

// GetOrCreateFuncLitSymbol returns or creates a symbol for an anonymous function.
//
// Anonymous functions (function literals) need symbols for type assignment
// and reference tracking. Each unique FunctionExpr AST node gets its own symbol.
func (t *BindingTable) GetOrCreateFuncLitSymbol(fn *ast.FunctionExpr) cfg.SymbolID {
	if fn == nil {
		return 0
	}
	if sym, ok := t.funcLitSymbols[fn]; ok {
		return sym
	}
	sym := cfg.NextSymbolID()
	t.funcLitSymbols[fn] = sym
	t.funcLitBySymbol[sym] = fn
	t.kind[sym] = cfg.SymbolLocal
	return sym
}

// SetFuncLitSymbol assigns a specific symbol to a function literal.
func (t *BindingTable) SetFuncLitSymbol(fn *ast.FunctionExpr, sym cfg.SymbolID) {
	if fn == nil {
		return
	}
	t.funcLitSymbols[fn] = sym
	if sym != 0 {
		t.funcLitBySymbol[sym] = fn
	}
}

// FuncLitSymbol returns the symbol for a function literal if one exists.
func (t *BindingTable) FuncLitSymbol(fn *ast.FunctionExpr) (cfg.SymbolID, bool) {
	if fn == nil {
		return 0, false
	}
	sym, ok := t.funcLitSymbols[fn]
	return sym, ok
}

// FuncLitBySymbol returns the function literal for a symbol if known.
func (t *BindingTable) FuncLitBySymbol(sym cfg.SymbolID) (*ast.FunctionExpr, bool) {
	if sym == 0 {
		return nil, false
	}
	fn, ok := t.funcLitBySymbol[sym]
	return fn, ok
}

// CapturedSymbols identifies free variables in a function.
//
// A captured symbol is one that is referenced inside the function body but
// declared outside it. This includes both upvalues (locals from enclosing
// functions) and globals.
//
// The analysis walks the function body to find all referenced symbols,
// then subtracts symbols declared within the function (parameters and locals).
func (t *BindingTable) CapturedSymbols(fn *ast.FunctionExpr) []cfg.SymbolID {
	if fn == nil {
		return nil
	}

	declared := make(map[cfg.SymbolID]bool)

	for _, sym := range t.paramSymbols[fn] {
		if sym != 0 {
			declared[sym] = true
		}
	}

	t.collectDeclaredInStmts(fn.Stmts, declared)

	referenced := make(map[cfg.SymbolID]bool)
	t.collectReferencedInStmts(fn.Stmts, referenced)

	var captured []cfg.SymbolID
	for sym := range referenced {
		if !declared[sym] {
			captured = append(captured, sym)
		}
	}
	return captured
}

// collectDeclaredInStmts gathers all symbols declared within a statement list.
func (t *BindingTable) collectDeclaredInStmts(stmts []ast.Stmt, declared map[cfg.SymbolID]bool) {
	for _, stmt := range stmts {
		t.collectDeclaredInStmt(stmt, declared)
	}
}

// collectDeclaredInStmt gathers symbols declared by a single statement.
// Recurses into nested blocks but not into nested function bodies.
func (t *BindingTable) collectDeclaredInStmt(stmt ast.Stmt, declared map[cfg.SymbolID]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		for _, sym := range t.localSymbols[s] {
			if sym != 0 {
				declared[sym] = true
			}
		}
	case *ast.NumberForStmt:
		if sym, ok := t.numForSymbols[s]; ok && sym != 0 {
			declared[sym] = true
		}
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.GenericForStmt:
		for _, sym := range t.genericForSymbols[s] {
			if sym != 0 {
				declared[sym] = true
			}
		}
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.DoBlockStmt:
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.WhileStmt:
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.RepeatStmt:
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.IfStmt:
		t.collectDeclaredInStmts(s.Then, declared)
		t.collectDeclaredInStmts(s.Else, declared)
	case *ast.FuncDefStmt:
		// Nested function parameters belong to the nested scope, not this one
	}
}

// collectReferencedInStmts gathers all symbols referenced in a statement list.
func (t *BindingTable) collectReferencedInStmts(stmts []ast.Stmt, referenced map[cfg.SymbolID]bool) {
	for _, stmt := range stmts {
		t.collectReferencedInStmt(stmt, referenced)
	}
}

// collectReferencedInStmt gathers symbols referenced in a single statement.
// Recurses into nested blocks and function bodies to find all references.
func (t *BindingTable) collectReferencedInStmt(stmt ast.Stmt, referenced map[cfg.SymbolID]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, e := range s.Lhs {
			t.collectReferencedInExpr(e, referenced)
		}
		for _, e := range s.Rhs {
			t.collectReferencedInExpr(e, referenced)
		}
	case *ast.LocalAssignStmt:
		for _, e := range s.Exprs {
			t.collectReferencedInExpr(e, referenced)
		}
	case *ast.FuncCallStmt:
		t.collectReferencedInExpr(s.Expr, referenced)
	case *ast.DoBlockStmt:
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.WhileStmt:
		t.collectReferencedInExpr(s.Condition, referenced)
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.RepeatStmt:
		t.collectReferencedInStmts(s.Stmts, referenced)
		t.collectReferencedInExpr(s.Condition, referenced)
	case *ast.IfStmt:
		t.collectReferencedInExpr(s.Condition, referenced)
		t.collectReferencedInStmts(s.Then, referenced)
		t.collectReferencedInStmts(s.Else, referenced)
	case *ast.NumberForStmt:
		t.collectReferencedInExpr(s.Init, referenced)
		t.collectReferencedInExpr(s.Limit, referenced)
		t.collectReferencedInExpr(s.Step, referenced)
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.GenericForStmt:
		for _, e := range s.Exprs {
			t.collectReferencedInExpr(e, referenced)
		}
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.FuncDefStmt:
		if s.Func != nil {
			t.collectReferencedInStmts(s.Func.Stmts, referenced)
		}
	case *ast.ReturnStmt:
		for _, e := range s.Exprs {
			t.collectReferencedInExpr(e, referenced)
		}
	}
}

// collectReferencedInExpr gathers symbols referenced in an expression.
// Recurses into subexpressions and nested function bodies.
func (t *BindingTable) collectReferencedInExpr(expr ast.Expr, referenced map[cfg.SymbolID]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := t.symbols[e]; ok && sym != 0 {
			referenced[sym] = true
		}
	case *ast.AttrGetExpr:
		t.collectReferencedInExpr(e.Object, referenced)
		t.collectReferencedInExpr(e.Key, referenced)
	case *ast.TableExpr:
		for _, f := range e.Fields {
			if f != nil {
				t.collectReferencedInExpr(f.Key, referenced)
				t.collectReferencedInExpr(f.Value, referenced)
			}
		}
	case *ast.FuncCallExpr:
		t.collectReferencedInExpr(e.Func, referenced)
		t.collectReferencedInExpr(e.Receiver, referenced)
		for _, a := range e.Args {
			t.collectReferencedInExpr(a, referenced)
		}
	case *ast.FunctionExpr:
		// Recurse into nested function bodies
		t.collectReferencedInStmts(e.Stmts, referenced)
	case *ast.LogicalOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.RelationalOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.StringConcatOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.ArithmeticOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.UnaryMinusOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.UnaryNotOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.UnaryLenOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.UnaryBNotOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.CastExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.NonNilAssertExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	}
}

// Globals returns all symbols with SymbolGlobal kind.
// This includes both predeclared globals and implicitly created globals.
func (t *BindingTable) Globals() []cfg.SymbolID {
	result := make([]cfg.SymbolID, 0)
	for sym, k := range t.kind {
		if k == cfg.SymbolGlobal && sym != 0 {
			result = append(result, sym)
		}
	}
	if len(result) > 1 {
		sort.Slice(result, func(i, j int) bool {
			return result[i] < result[j]
		})
	}
	return result
}

// AllSymbols returns every symbol in the table without duplicates.
//
// This collects symbols from all sources: bound identifiers, kinds map,
// parameter lists, local declarations, and for loop variables.
// Zero symbols are excluded.
func (t *BindingTable) AllSymbols() []cfg.SymbolID {
	seen := make(map[cfg.SymbolID]bool)

	for sym := range t.kind {
		if sym != 0 {
			seen[sym] = true
		}
	}

	for _, sym := range t.symbols {
		if sym != 0 {
			seen[sym] = true
		}
	}

	for _, syms := range t.paramSymbols {
		for _, sym := range syms {
			if sym != 0 {
				seen[sym] = true
			}
		}
	}

	for _, syms := range t.localSymbols {
		for _, sym := range syms {
			if sym != 0 {
				seen[sym] = true
			}
		}
	}

	for _, sym := range t.numForSymbols {
		if sym != 0 {
			seen[sym] = true
		}
	}

	for _, syms := range t.genericForSymbols {
		for _, sym := range syms {
			if sym != 0 {
				seen[sym] = true
			}
		}
	}

	result := make([]cfg.SymbolID, 0, len(seen))
	for sym := range seen {
		result = append(result, sym)
	}
	if len(result) > 1 {
		sort.Slice(result, func(i, j int) bool {
			return result[i] < result[j]
		})
	}
	return result
}
