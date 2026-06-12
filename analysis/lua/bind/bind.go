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
	typeValueRefs      map[*ast.IdentExpr]TypeDecl

	names map[symbol.ID]string
	kinds map[symbol.ID]symbol.Kind

	globals map[string]globalSymbol

	functionSymbols    map[*ast.FunctionExpr]symbol.ID
	functionsBySymbol  map[symbol.ID]*ast.FunctionExpr
	functions          []*ast.FunctionExpr
	nestedFunctions    map[*ast.FunctionExpr][]*ast.FunctionExpr
	functionOrigins    map[*ast.FunctionExpr]FunctionOrigin
	declaringFunctions map[symbol.ID]*ast.FunctionExpr
	directCaptures     map[*ast.FunctionExpr][]Capture
	directCaptureSeen  map[*ast.FunctionExpr]map[symbol.ID]struct{}

	paramSymbols      map[*ast.FunctionExpr][]symbol.ID
	varargSymbols     map[*ast.FunctionExpr]symbol.ID
	paramSlots        map[*ast.FunctionExpr][]ParamSlot
	localSymbols      map[*ast.LocalAssignStmt][]symbol.ID
	numForSymbols     map[*ast.NumberForStmt]symbol.ID
	genericForSymbols map[*ast.GenericForStmt][]symbol.ID

	nextTypeDeclID     TypeDeclID
	typeRefs           map[*ast.TypeRefExpr]TypeDecl
	primitiveTypeRefs  map[*ast.PrimitiveTypeExpr]TypeDecl
	typeDefDecls       map[*ast.TypeDefStmt]TypeDecl
	interfaceDecls     map[*ast.InterfaceDefStmt]TypeDecl
	typeDefParams      map[*ast.TypeDefStmt][]TypeDecl
	functionTypeParams map[*ast.FunctionExpr][]TypeDecl
}

type globalSymbol struct {
	id          symbol.ID
	predeclared bool
}

// TypeDeclID identifies a lexical type declaration independently of value
// symbols.
type TypeDeclID uint64

// TypeDeclKind classifies entries in the lexical type namespace.
type TypeDeclKind uint8

const (
	TypeDeclUnknown TypeDeclKind = iota
	TypeDeclAlias
	TypeDeclInterface
	TypeDeclParam
)

// TypeDecl records one declaration in the lexical type namespace.
type TypeDecl struct {
	ID         TypeDeclID
	Kind       TypeDeclKind
	Name       string
	Type       *ast.TypeDefStmt
	Interface  *ast.InterfaceDefStmt
	Constraint ast.TypeExpr
}

// Stmt returns the declaration statement for alias and interface declarations.
func (d TypeDecl) Stmt() ast.Stmt {
	switch d.Kind {
	case TypeDeclAlias:
		return d.Type
	case TypeDeclInterface:
		return d.Interface
	default:
		return nil
	}
}

// ParamSlot describes one runtime parameter slot for a function.
type ParamSlot struct {
	Symbol       symbol.ID
	Name         string
	Type         ast.TypeExpr
	SourceIndex  int
	Vararg       bool
	ImplicitSelf bool
}

// Capture describes one declaration directly captured by a function body.
type Capture struct {
	Captured          symbol.ID
	CapturedName      string
	DeclaringFunction *ast.FunctionExpr
}

// FunctionOriginKind classifies the syntactic form that introduced a function.
type FunctionOriginKind uint8

const (
	FunctionOriginUnknown FunctionOriginKind = iota
	FunctionOriginDeclaration
	FunctionOriginLocalAssignment
	FunctionOriginLiteral
	FunctionOriginMethod
)

// FunctionOrigin records where a function expression was introduced.
type FunctionOrigin struct {
	Func   *ast.FunctionExpr
	Symbol symbol.ID
	Parent *ast.FunctionExpr
	Kind   FunctionOriginKind

	Stmt       ast.Stmt
	LocalIndex int
	Method     string

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
}

// BindFunction binds a single function expression with a fresh global seed.
func BindFunction(fn *ast.FunctionExpr, opts Options) *Result {
	r := newResult(opts)
	b := binder{result: r}
	b.bindFunction(fn, false, functionOriginDetails{
		kind:       FunctionOriginLiteral,
		localIndex: -1,
	})
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

// TypeValueRef returns the lexical type declaration named by an identifier
// used in value position, such as the receiver in Point:is(v) or callee in
// Point(v).
func (r *Result) TypeValueRef(ident *ast.IdentExpr) (TypeDecl, bool) {
	if r == nil || ident == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeValueRefs[ident]
	return decl, ok && decl.ID != 0
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

// FunctionSymbol returns the function identity symbol for fn.
func (r *Result) FunctionSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if r == nil || fn == nil {
		return 0, false
	}
	id, ok := r.functionSymbols[fn]
	return id, ok && id != 0
}

// FunctionBySymbol returns the function expression identified by sym.
func (r *Result) FunctionBySymbol(sym symbol.ID) (*ast.FunctionExpr, bool) {
	if r == nil || sym == 0 {
		return nil, false
	}
	fn, ok := r.functionsBySymbol[sym]
	return fn, ok
}

// Functions returns all bound functions in parent-before-child order.
func (r *Result) Functions() []*ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return cloneFunctions(r.functions)
}

// NestedFunctions returns the direct nested functions declared under parent.
func (r *Result) NestedFunctions(parent *ast.FunctionExpr) []*ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return cloneFunctions(r.nestedFunctions[parent])
}

// FunctionOrigins returns all bound function origins in parent-before-child order.
func (r *Result) FunctionOrigins() []FunctionOrigin {
	if r == nil {
		return nil
	}
	if len(r.functions) == 0 {
		return nil
	}
	origins := make([]FunctionOrigin, 0, len(r.functions))
	for _, fn := range r.functions {
		origin, ok := r.functionOrigins[fn]
		if !ok {
			continue
		}
		origins = append(origins, origin)
	}
	return origins
}

// FunctionOrigin returns the origin metadata for fn.
func (r *Result) FunctionOrigin(fn *ast.FunctionExpr) (FunctionOrigin, bool) {
	if r == nil || fn == nil {
		return FunctionOrigin{}, false
	}
	origin, ok := r.functionOrigins[fn]
	return origin, ok && origin.Func != nil
}

// ParentFunction returns the direct lexical parent of fn, if fn is known.
func (r *Result) ParentFunction(fn *ast.FunctionExpr) (*ast.FunctionExpr, bool) {
	origin, ok := r.FunctionOrigin(fn)
	if !ok {
		return nil, false
	}
	return origin.Parent, true
}

// DeclaringFunction returns the function that owns a declaration symbol.
func (r *Result) DeclaringFunction(sym symbol.ID) (*ast.FunctionExpr, bool) {
	if r == nil || sym == 0 {
		return nil, false
	}
	fn, ok := r.declaringFunctions[sym]
	return fn, ok
}

// DirectCaptures returns declarations directly captured by fn in first-use order.
func (r *Result) DirectCaptures(fn *ast.FunctionExpr) []Capture {
	if r == nil || fn == nil {
		return nil
	}
	return cloneCaptures(r.directCaptures[fn])
}

// ParamSymbols returns ordered parameter symbols for fn.
func (r *Result) ParamSymbols(fn *ast.FunctionExpr) []symbol.ID {
	if r == nil || fn == nil {
		return nil
	}
	return cloneSymbols(r.paramSymbols[fn])
}

// VarargSymbol returns the vararg parameter identity for fn, when present.
func (r *Result) VarargSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if r == nil || fn == nil {
		return 0, false
	}
	id, ok := r.varargSymbols[fn]
	return id, ok && id != 0
}

// ParamSlots returns the bind-owned runtime parameter layout for fn.
func (r *Result) ParamSlots(fn *ast.FunctionExpr) []ParamSlot {
	if r == nil || fn == nil {
		return nil
	}
	return cloneParamSlots(r.paramSlots[fn])
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

// TypeRef returns the lexical type declaration bound to ref.
func (r *Result) TypeRef(ref *ast.TypeRefExpr) (TypeDecl, bool) {
	if r == nil || ref == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeRefs[ref]
	return decl, ok && decl.ID != 0
}

// PrimitiveTypeRef returns the lexical type declaration bound to a non-built-in
// primitive-name type expression.
func (r *Result) PrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) (TypeDecl, bool) {
	if r == nil || expr == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.primitiveTypeRefs[expr]
	return decl, ok && decl.ID != 0
}

// TypeDef returns the lexical type declaration introduced by stmt.
func (r *Result) TypeDef(stmt *ast.TypeDefStmt) (TypeDecl, bool) {
	if r == nil || stmt == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeDefDecls[stmt]
	return decl, ok && decl.ID != 0
}

// InterfaceDef returns the lexical type declaration introduced by stmt.
func (r *Result) InterfaceDef(stmt *ast.InterfaceDefStmt) (TypeDecl, bool) {
	if r == nil || stmt == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.interfaceDecls[stmt]
	return decl, ok && decl.ID != 0
}

// TypeDefParams returns the lexical type parameters declared by stmt.
func (r *Result) TypeDefParams(stmt *ast.TypeDefStmt) []TypeDecl {
	if r == nil || stmt == nil {
		return nil
	}
	return cloneTypeDecls(r.typeDefParams[stmt])
}

// FunctionTypeParams returns the lexical type parameters declared by fn.
func (r *Result) FunctionTypeParams(fn *ast.FunctionExpr) []TypeDecl {
	if r == nil || fn == nil {
		return nil
	}
	return cloneTypeDecls(r.functionTypeParams[fn])
}

func newResult(opts Options) *Result {
	r := &Result{
		identSymbols:       make(map[*ast.IdentExpr]symbol.ID),
		implicitGlobalUses: make(map[*ast.IdentExpr]struct{}),
		typeValueRefs:      make(map[*ast.IdentExpr]TypeDecl),
		names:              make(map[symbol.ID]string),
		kinds:              make(map[symbol.ID]symbol.Kind),
		globals:            make(map[string]globalSymbol),
		functionSymbols:    make(map[*ast.FunctionExpr]symbol.ID),
		functionsBySymbol:  make(map[symbol.ID]*ast.FunctionExpr),
		nestedFunctions:    make(map[*ast.FunctionExpr][]*ast.FunctionExpr),
		functionOrigins:    make(map[*ast.FunctionExpr]FunctionOrigin),
		declaringFunctions: make(map[symbol.ID]*ast.FunctionExpr),
		directCaptures:     make(map[*ast.FunctionExpr][]Capture),
		directCaptureSeen:  make(map[*ast.FunctionExpr]map[symbol.ID]struct{}),
		paramSymbols:       make(map[*ast.FunctionExpr][]symbol.ID),
		varargSymbols:      make(map[*ast.FunctionExpr]symbol.ID),
		paramSlots:         make(map[*ast.FunctionExpr][]ParamSlot),
		localSymbols:       make(map[*ast.LocalAssignStmt][]symbol.ID),
		numForSymbols:      make(map[*ast.NumberForStmt]symbol.ID),
		genericForSymbols:  make(map[*ast.GenericForStmt][]symbol.ID),
		typeRefs:           make(map[*ast.TypeRefExpr]TypeDecl),
		primitiveTypeRefs:  make(map[*ast.PrimitiveTypeExpr]TypeDecl),
		typeDefDecls:       make(map[*ast.TypeDefStmt]TypeDecl),
		interfaceDecls:     make(map[*ast.InterfaceDefStmt]TypeDecl),
		typeDefParams:      make(map[*ast.TypeDefStmt][]TypeDecl),
		functionTypeParams: make(map[*ast.FunctionExpr][]TypeDecl),
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

func cloneFunctions(fns []*ast.FunctionExpr) []*ast.FunctionExpr {
	if len(fns) == 0 {
		return nil
	}
	return append([]*ast.FunctionExpr(nil), fns...)
}

func cloneParamSlots(slots []ParamSlot) []ParamSlot {
	if len(slots) == 0 {
		return nil
	}
	return append([]ParamSlot(nil), slots...)
}

func cloneCaptures(captures []Capture) []Capture {
	if len(captures) == 0 {
		return nil
	}
	return append([]Capture(nil), captures...)
}

func cloneTypeDecls(decls []TypeDecl) []TypeDecl {
	if len(decls) == 0 {
		return nil
	}
	return append([]TypeDecl(nil), decls...)
}

func (r *Result) newSymbol(name string, kind symbol.Kind) symbol.ID {
	id := symbol.Next()
	r.names[id] = name
	r.kinds[id] = kind
	return id
}

func (r *Result) newTypeDecl(kind TypeDeclKind, name string, typeDef *ast.TypeDefStmt, iface *ast.InterfaceDefStmt, constraint ast.TypeExpr) TypeDecl {
	if name == "" {
		return TypeDecl{}
	}
	r.nextTypeDeclID++
	decl := TypeDecl{
		ID:         r.nextTypeDeclID,
		Kind:       kind,
		Name:       name,
		Type:       typeDef,
		Interface:  iface,
		Constraint: constraint,
	}
	switch kind {
	case TypeDeclAlias:
		if typeDef != nil {
			r.typeDefDecls[typeDef] = decl
		}
	case TypeDeclInterface:
		if iface != nil {
			r.interfaceDecls[iface] = decl
		}
	}
	return decl
}

type functionOriginDetails struct {
	kind            FunctionOriginKind
	stmt            ast.Stmt
	localIndex      int
	method          string
	targetSymbol    symbol.ID
	hasTargetSymbol bool
}

func (r *Result) registerFunction(fn, parent *ast.FunctionExpr, details functionOriginDetails) symbol.ID {
	if fn == nil {
		return 0
	}
	if id, ok := r.functionSymbols[fn]; ok {
		return id
	}
	id := r.newSymbol("", symbol.Function)
	r.functionSymbols[fn] = id
	r.functionsBySymbol[id] = fn
	r.functions = append(r.functions, fn)
	r.nestedFunctions[parent] = append(r.nestedFunctions[parent], fn)
	r.declaringFunctions[id] = fn
	r.functionOrigins[fn] = FunctionOrigin{
		Func:            fn,
		Symbol:          id,
		Parent:          parent,
		Kind:            details.kind,
		Stmt:            details.stmt,
		LocalIndex:      details.localIndex,
		Method:          details.method,
		TargetSymbol:    details.targetSymbol,
		HasTargetSymbol: details.hasTargetSymbol,
	}
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

type typeScope struct {
	names map[string]TypeDecl
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

func (b *binder) pushTypeScope() {
	b.typeScopes = append(b.typeScopes, typeScope{names: make(map[string]TypeDecl)})
}

func (b *binder) popTypeScope() {
	if len(b.typeScopes) == 0 {
		return
	}
	b.typeScopes = b.typeScopes[:len(b.typeScopes)-1]
}

func (b *binder) define(name string, id symbol.ID) {
	if name == "" || len(b.scopes) == 0 || id == 0 {
		return
	}
	b.scopes[len(b.scopes)-1].names[name] = id
}

func (b *binder) defineType(name string, decl TypeDecl) {
	if name == "" || len(b.typeScopes) == 0 || decl.ID == 0 {
		return
	}
	b.typeScopes[len(b.typeScopes)-1].names[name] = decl
}

func (b *binder) currentFunction() *ast.FunctionExpr {
	if len(b.functionStack) == 0 {
		return nil
	}
	return b.functionStack[len(b.functionStack)-1]
}

func (b *binder) newSymbol(name string, kind symbol.Kind) symbol.ID {
	id := b.result.newSymbol(name, kind)
	if fn := b.currentFunction(); fn != nil {
		b.result.declaringFunctions[id] = fn
	}
	return id
}

func (b *binder) recordDirectCapture(id symbol.ID) {
	if id == 0 {
		return
	}
	current := b.currentFunction()
	if current == nil {
		return
	}
	kind, ok := b.result.kinds[id]
	if !ok || (kind != symbol.Local && kind != symbol.Param) {
		return
	}
	declaringFn := b.result.declaringFunctions[id]
	if declaringFn == current {
		return
	}
	seen := b.result.directCaptureSeen[current]
	if seen == nil {
		seen = make(map[symbol.ID]struct{})
		b.result.directCaptureSeen[current] = seen
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	b.result.directCaptures[current] = append(b.result.directCaptures[current], Capture{
		Captured:          id,
		CapturedName:      b.result.names[id],
		DeclaringFunction: declaringFn,
	})
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

func (b *binder) lookupType(name string) (TypeDecl, bool) {
	if name == "" {
		return TypeDecl{}, false
	}
	for i := len(b.typeScopes) - 1; i >= 0; i-- {
		if decl, ok := b.typeScopes[i].names[name]; ok && decl.ID != 0 {
			return decl, true
		}
	}
	return TypeDecl{}, false
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
	case *ast.TypeDefStmt:
		b.bindTypeDef(stmt)
	case *ast.InterfaceDefStmt:
		b.bindInterfaceDef(stmt)
	}
}

func (b *binder) bindLocalAssign(stmt *ast.LocalAssignStmt) {
	b.bindTypeExprs(stmt.Types)

	ids := make([]symbol.ID, len(stmt.Names))
	pending := make(map[string]symbol.ID, len(stmt.Names))
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
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
	for i, expr := range stmt.Exprs {
		if fn, ok := expr.(*ast.FunctionExpr); ok {
			details := functionOriginDetails{
				kind:       FunctionOriginLiteral,
				localIndex: -1,
			}
			if i < len(ids) {
				details.kind = FunctionOriginLocalAssignment
				details.stmt = stmt
				details.localIndex = i
				details.targetSymbol = ids[i]
				details.hasTargetSymbol = ids[i] != 0
			}
			b.bindFunction(fn, false, details)
			continue
		}
		b.bindExpr(expr)
	}
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

	id := b.newSymbol(stmt.Name, symbol.Local)
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
		id := b.newSymbol(name, symbol.Local)
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
	details := functionOriginDetails{
		kind:       FunctionOriginDeclaration,
		stmt:       stmt,
		localIndex: -1,
	}
	if stmt.Name != nil && stmt.Name.Method != "" {
		details.kind = FunctionOriginMethod
		details.method = stmt.Name.Method
	} else if id, ok := b.result.FuncDefTargetSymbol(stmt); ok {
		details.targetSymbol = id
		details.hasTargetSymbol = true
	}
	b.bindFunction(stmt.Func, stmt.Name != nil && stmt.Name.Method != "", details)
}

func (b *binder) bindExprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		b.bindExpr(expr)
	}
}

func (b *binder) bindExpr(expr ast.Expr) {
	switch expr := expr.(type) {
	case nil:
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr:
	case *ast.Comma3Expr:
		b.bindVararg()
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
		b.bindTypeExprs(expr.TypeArgs)
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
		b.bindFunction(expr, false, functionOriginDetails{
			kind:       FunctionOriginLiteral,
			localIndex: -1,
		})
	case *ast.CastExpr:
		b.bindExpr(expr.Expr)
		b.bindTypeExpr(expr.Type)
	case *ast.NonNilAssertExpr:
		b.bindExpr(expr.Expr)
	}
}

func (b *binder) bindVararg() {
	current := b.currentFunction()
	if current == nil {
		return
	}
	id, ok := b.result.varargSymbols[current]
	if !ok || id == 0 {
		return
	}
	b.recordDirectCapture(id)
}

func (b *binder) bindFunction(fn *ast.FunctionExpr, method bool, origin functionOriginDetails) {
	if fn == nil {
		return
	}

	parent := b.currentFunction()
	b.result.registerFunction(fn, parent, origin)
	b.functionStack = append(b.functionStack, fn)

	oldVisibleDeferred := b.visibleDeferred
	b.visibleDeferred = len(b.deferred)

	b.pushScope()
	b.bindTypeParamConstraints(fn.TypeParams)
	fnTypeParams := b.defineTypeParams(fn.TypeParams)
	if len(fnTypeParams) > 0 {
		b.result.functionTypeParams[fn] = fnTypeParams
	}

	params := make([]symbol.ID, 0)
	slots := make([]ParamSlot, 0)
	names := []string(nil)
	types := []ast.TypeExpr(nil)
	hasVargs := false
	varargType := ast.TypeExpr(nil)
	if fn.ParList != nil {
		names = fn.ParList.Names
		types = fn.ParList.Types
		hasVargs = fn.ParList.HasVargs
		varargType = fn.ParList.VarargType
	}
	if method && (len(names) == 0 || names[0] != "self") {
		id := b.newSymbol("self", symbol.Param)
		params = append(params, id)
		b.define("self", id)
		slots = append(slots, ParamSlot{
			Symbol:       id,
			Name:         "self",
			SourceIndex:  -1,
			ImplicitSelf: true,
		})
	}
	for i, name := range names {
		id := b.newSymbol(name, symbol.Param)
		params = append(params, id)
		b.define(name, id)
		slots = append(slots, ParamSlot{
			Symbol:      id,
			Name:        name,
			Type:        typeAt(types, i),
			SourceIndex: i,
		})
	}
	b.result.paramSymbols[fn] = params
	if hasVargs {
		id := b.newSymbol("...", symbol.Param)
		b.result.varargSymbols[fn] = id
		slots = append(slots, ParamSlot{
			Symbol:      id,
			Name:        "...",
			Type:        varargType,
			SourceIndex: len(names),
			Vararg:      true,
		})
	}
	b.result.paramSlots[fn] = slots
	b.bindTypeExprs(types)
	b.bindTypeExpr(varargType)
	b.bindTypeExprs(fn.ReturnTypes)
	b.bindStmts(fn.Stmts)
	b.popScope()

	b.visibleDeferred = oldVisibleDeferred
	b.functionStack = b.functionStack[:len(b.functionStack)-1]
}

func typeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
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
		if decl, hasType := b.lookupType(ident.Value); hasType {
			b.result.typeValueRefs[ident] = decl
		}
		id = b.result.global(ident.Value, false)
		b.result.implicitGlobalUses[ident] = struct{}{}
	}
	b.result.identSymbols[ident] = id
	b.recordDirectCapture(id)
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

func (b *binder) bindTypeDef(stmt *ast.TypeDefStmt) {
	if stmt == nil {
		return
	}
	decl := b.result.newTypeDecl(TypeDeclAlias, stmt.Name, stmt, nil, nil)
	b.defineType(stmt.Name, decl)
	b.bindTypeParamConstraints(stmt.TypeParams)
	b.pushTypeScope()
	params := b.defineTypeParams(stmt.TypeParams)
	if len(params) > 0 {
		b.result.typeDefParams[stmt] = params
	}
	b.bindTypeExpr(stmt.Type)
	b.popTypeScope()
}

func (b *binder) bindInterfaceDef(stmt *ast.InterfaceDefStmt) {
	if stmt == nil {
		return
	}
	decl := b.result.newTypeDecl(TypeDeclInterface, stmt.Name, nil, stmt, nil)
	b.defineType(stmt.Name, decl)
	for _, ref := range stmt.Extends {
		b.bindTypeRef(ref)
	}
	for _, field := range stmt.Fields {
		b.bindTypeExpr(field.Type)
	}
	for _, method := range stmt.Methods {
		if method.Type != nil {
			b.bindTypeExpr(method.Type)
		}
	}
}

func (b *binder) bindTypeParamConstraints(params []ast.TypeParamExpr) {
	for _, param := range params {
		b.bindTypeExpr(param.Constraint)
	}
}

func (b *binder) defineTypeParams(params []ast.TypeParamExpr) []TypeDecl {
	if len(params) == 0 {
		return nil
	}
	decls := make([]TypeDecl, 0, len(params))
	for _, param := range params {
		decl := b.result.newTypeDecl(TypeDeclParam, param.Name, nil, nil, param.Constraint)
		if decl.ID == 0 {
			continue
		}
		b.defineType(param.Name, decl)
		decls = append(decls, decl)
	}
	return decls
}

func (b *binder) bindTypeExprs(exprs []ast.TypeExpr) {
	for _, expr := range exprs {
		b.bindTypeExpr(expr)
	}
}

func (b *binder) bindTypeExpr(expr ast.TypeExpr) {
	switch expr := expr.(type) {
	case nil:
	case *ast.PrimitiveTypeExpr:
		b.bindPrimitiveTypeRef(expr)
	case *ast.SelfTypeExpr, *ast.LiteralTypeExpr:
	case *ast.OptionalTypeExpr:
		b.bindTypeExpr(expr.Inner)
	case *ast.UnionTypeExpr:
		b.bindTypeExprs(expr.Types)
	case *ast.IntersectionTypeExpr:
		b.bindTypeExprs(expr.Types)
	case *ast.ArrayTypeExpr:
		b.bindTypeExpr(expr.Element)
	case *ast.MapTypeExpr:
		b.bindTypeExpr(expr.Key)
		b.bindTypeExpr(expr.Value)
	case *ast.RecordTypeExpr:
		for _, field := range expr.Fields {
			b.bindTypeExpr(field.Type)
		}
	case *ast.FunctionTypeExpr:
		b.bindTypeParamConstraints(expr.TypeParams)
		b.pushTypeScope()
		b.defineTypeParams(expr.TypeParams)
		for _, param := range expr.Params {
			b.bindTypeExpr(param.Type)
		}
		b.bindTypeExpr(expr.Variadic)
		b.bindTypeExprs(expr.Returns)
		b.popTypeScope()
	case *ast.AssertsTypeExpr:
		b.bindTypeExpr(expr.NarrowTo)
	case *ast.TypeRefExpr:
		b.bindTypeRef(expr)
	case *ast.GenericTypeExpr:
		b.bindTypeRef(expr.Base)
		b.bindTypeExprs(expr.Args)
	case *ast.MetaTypeExpr:
		b.bindTypeExpr(expr.Inner)
	case *ast.TupleTypeExpr:
		b.bindTypeExprs(expr.Elements)
	case *ast.TypeOfExpr:
	case *ast.KeyOfExpr:
		b.bindTypeExpr(expr.Inner)
	case *ast.IndexAccessExpr:
		b.bindTypeExpr(expr.Object)
		b.bindTypeExpr(expr.Index)
	case *ast.ConditionalTypeExpr:
		b.bindTypeExpr(expr.Check)
		b.bindTypeExpr(expr.Extends)
		b.bindTypeExpr(expr.Then)
		b.bindTypeExpr(expr.Else)
	}
}

func (b *binder) bindTypeRef(ref *ast.TypeRefExpr) {
	if ref == nil || len(ref.Path) != 1 {
		return
	}
	decl, ok := b.lookupType(ref.Path[0])
	if !ok {
		return
	}
	b.result.typeRefs[ref] = decl
}

func (b *binder) bindPrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) {
	if expr == nil || isBuiltinPrimitiveTypeName(expr.Name) {
		return
	}
	decl, ok := b.lookupType(expr.Name)
	if !ok {
		return
	}
	b.result.primitiveTypeRefs[expr] = decl
}

func isBuiltinPrimitiveTypeName(name string) bool {
	switch name {
	case "nil", "boolean", "number", "integer", "string", "any", "unknown", "never", "self":
		return true
	default:
		return false
	}
}
