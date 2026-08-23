package bind

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

// SymbolKind classifies how a binder-owned lexical symbol was declared.
type SymbolKind uint8

const (
	SymbolUnknown SymbolKind = iota
	SymbolParam
	SymbolLocal
	SymbolGlobal
	SymbolUpvalue
	SymbolFunction
)

// Symbol is a lexical declaration coordinate issued by one binding Result.
// It has no meaning outside that result and must be translated before Lua
// lowering publishes canonical Program identities.
type Symbol uint64

// symbolInfo is the single metadata row for one Result-scoped symbol.
// Name and Kind are two views of the same declaration identity; keeping them
// together prevents the binder from maintaining parallel keyed stores that
// can disagree.
type symbolInfo struct {
	name string
	kind SymbolKind
}

// SymbolOf returns the declaration symbol bound to an identifier occurrence.
func (r *Result) SymbolOf(ident *ast.IdentExpr) (Symbol, bool) {
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

// GlobalIdentity is an opaque binder-owned global selection. It can only be
// obtained by resolving an identifier occurrence through Result.
type GlobalIdentity struct {
	owner *Result
	id    Symbol
}

// GlobalIdentity resolves ident to its bound global declaration.
func (r *Result) GlobalIdentity(ident *ast.IdentExpr) (GlobalIdentity, bool) {
	if r == nil || ident == nil {
		return GlobalIdentity{}, false
	}
	id, ok := r.SymbolOf(ident)
	if !ok || !r.symbolResolvesToGlobal(id, ident.Value) {
		return GlobalIdentity{}, false
	}
	return GlobalIdentity{owner: r, id: id}, true
}

// GlobalIdentityOf returns the Result-scoped identity for an already-bound
// global symbol, including roots whose authored occurrence is a qualified
// TypeRef and therefore has no value-level IdentExpr node.
func (r *Result) GlobalIdentityOf(id Symbol) (GlobalIdentity, bool) {
	if r == nil || id == 0 || r.globalRecord(id) == nil {
		return GlobalIdentity{}, false
	}
	if kind, ok := r.Kind(id); !ok || kind != SymbolGlobal {
		return GlobalIdentity{}, false
	}
	return GlobalIdentity{owner: r, id: id}, true
}

// Valid reports whether identity is an unforgeable selection from its owning
// Result. The owner check is part of validity so equal local symbol numbers
// from separate binding passes never alias.
func (identity GlobalIdentity) Valid() bool {
	return identity.owner != nil && identity.id != 0 && identity.owner.globalRecord(identity.id) != nil
}

// ID returns the Result-scoped lexical symbol number. It is useful only as a
// diagnostic; lower consumers should pass the opaque identity itself.
func (identity GlobalIdentity) ID() Symbol {
	if !identity.Valid() {
		return 0
	}
	return identity.id
}

// Name returns the authored spelling selected by this identity.
func (identity GlobalIdentity) Name() string {
	if !identity.Valid() {
		return ""
	}
	return identity.owner.Name(identity.id)
}

// Same reports identity equality, including Result ownership.
func (identity GlobalIdentity) Same(other GlobalIdentity) bool {
	return identity.Valid() && other.Valid() && identity.owner == other.owner && identity.id == other.id
}

// Slot resolves the zero-based reserved global Cell slot in O(1).
func (identity GlobalIdentity) Slot() (uint32, bool) {
	if !identity.Valid() {
		return 0, false
	}
	return identity.owner.globalSlot(identity.id)
}

// Matches reports whether the bound global has the registry-selected name.
func (identity GlobalIdentity) Matches(name string) bool {
	return identity.Valid() && identity.Name() == name
}

// GlobalCensus returns the immutable global Cell denominator for this Result.
func (r *Result) GlobalCensus() GlobalCensus {
	if r == nil {
		return GlobalCensus{}
	}
	return GlobalCensus{owner: r, cells: r.globals.cells}
}

// GlobalCell resolves a same-Result identity in O(1), rejecting foreign
// identities before consulting the dense slot.
func (r *Result) GlobalCell(identity GlobalIdentity) (GlobalCell, bool) {
	if r == nil || identity.owner != r {
		return GlobalCell{}, false
	}
	return r.GlobalCensus().Cell(identity)
}

// DirectGlobalCalls returns the binder's complete source-order enumeration of
// plain global function-call occurrences. It is a detached slice; each
// occurrence retains only the parser-owned Call pointer as its occurrence key
// and carries argument facts copied by the binder.
func (r *Result) DirectGlobalCalls() []DirectGlobalCall {
	if r == nil || len(r.directGlobalCalls) == 0 {
		return nil
	}
	return append([]DirectGlobalCall(nil), r.directGlobalCalls...)
}

func (r *Result) symbolResolvesToGlobal(id Symbol, name string) bool {
	if r == nil || id == 0 || name == "" || r.Name(id) != name {
		return false
	}
	kind, ok := r.Kind(id)
	return ok && kind == SymbolGlobal
}

// Name returns the declaration name for a symbol.
func (r *Result) Name(id Symbol) string {
	if r == nil {
		return ""
	}
	return r.symbols[id].name
}

// CallSpelling returns the binder-owned optional authored name for one parser
// Call occurrence. Dynamic and indexed calls intentionally return false; the
// lowerer must not recover a name by reopening Call syntax.
func (r *Result) CallSpelling(call *ast.FuncCallExpr) (string, bool) {
	if r == nil || call == nil {
		return "", false
	}
	name, ok := r.callSpellings[call]
	return name, ok && name != ""
}

// Kind returns the declaration kind for a symbol.
func (r *Result) Kind(id Symbol) (SymbolKind, bool) {
	if r == nil {
		return SymbolUnknown, false
	}
	info, ok := r.symbols[id]
	return info.kind, ok
}

// LocalSymbolAt returns the local symbol at index for stmt.
func (r *Result) LocalSymbolAt(stmt *ast.LocalAssignStmt, index int) (Symbol, bool) {
	if r == nil || stmt == nil || index < 0 {
		return 0, false
	}
	ids := r.localSymbols[stmt]
	if index >= len(ids) {
		return 0, false
	}
	return ids[index], true
}

// SymbolTypeAnnotation returns the declared type expression for a parameter or
// local declaration symbol.
func (r *Result) SymbolTypeAnnotation(id Symbol) (ast.TypeExpr, bool) {
	if r == nil || id == 0 {
		return nil, false
	}
	expr, ok := r.symbolAnnotations[id]
	return expr, ok && expr != nil
}

// NumForSymbol returns the loop variable symbol for a numeric for statement.
func (r *Result) NumForSymbol(stmt *ast.NumberForStmt) (Symbol, bool) {
	if r == nil || stmt == nil {
		return 0, false
	}
	id, ok := r.numForSymbols[stmt]
	return id, ok
}

// GenericForSymbols returns ordered loop variable symbols for a generic for.
func (r *Result) GenericForSymbols(stmt *ast.GenericForStmt) []Symbol {
	if r == nil || stmt == nil {
		return nil
	}
	return cloneSymbols(r.genericForSymbols[stmt])
}

func cloneSymbols(ids []Symbol) []Symbol {
	if len(ids) == 0 {
		return nil
	}
	return append([]Symbol(nil), ids...)
}

// newSymbol allocates a declaration identity from this Result's own counter.
// Numbering is bind-local so identical source produces identical symbol IDs
// across independent solves, which keeps every downstream identity token and
// content digest deterministic. IDs are unique within a Result; they are never
// compared against symbols from another Result.
func (r *Result) newSymbol(name string, kind SymbolKind) Symbol {
	r.nextSymbolID++
	id := r.nextSymbolID
	r.symbols[id] = symbolInfo{name: name, kind: kind}
	return id
}

func (r *Result) setSymbolTypeAnnotation(id Symbol, expr ast.TypeExpr) {
	if r == nil || id == 0 || expr == nil {
		return
	}
	r.symbolAnnotations[id] = expr
}

type pendingScope struct {
	names map[string]Symbol
}

type valueHead struct {
	id    Symbol
	depth int
}

type valueUndo struct {
	name    string
	prior   valueHead
	existed bool
}

type pendingHead struct {
	batch int
	depth int
	id    Symbol
}

type binder struct {
	result *Result

	implicitGlobalSymbols map[Symbol]struct{}
	staticOnlyGlobals     map[Symbol]struct{}
	directCaptureSeen     map[*ast.FunctionExpr]map[Symbol]struct{}
	declaringFunctions    map[Symbol]*ast.FunctionExpr
	globals               map[string]Symbol
	runtimeChunkTypes     map[string]TypeDecl
	qualifiedTypeAliases  map[qualifiedTypeAliasKey]QualifiedTypeAlias

	valueHeads map[string]valueHead
	valueUndo  []valueUndo
	valueMarks []int

	functions []functionFrame

	pending        []pendingScope
	pendingByName  map[string][]pendingHead
	visiblePending int
	rootStmts      []ast.Stmt
	work           []bindStep

	typeHeads map[string]TypeDecl
	typeUndo  []typeUndo
	typeMarks []int

	control controlBinder
}

func newBinder(result *Result) binder {
	return binder{result: result}
}

func (b *binder) global(name string) Symbol {
	if id := b.globals[name]; id != 0 {
		return id
	}
	id := b.result.newSymbol(name, SymbolGlobal)
	if b.globals == nil {
		b.globals = make(map[string]Symbol)
	}
	b.globals[name] = id
	b.result.addGlobalRecord(id)
	return id
}

func (b *binder) pushScope() {
	b.valueMarks = append(b.valueMarks, len(b.valueUndo))
	b.pushTypeScope()
	b.control.pushBlock()
}

func (b *binder) popScope() {
	if len(b.valueMarks) == 0 {
		return
	}
	mark := b.valueMarks[len(b.valueMarks)-1]
	for i := len(b.valueUndo) - 1; i >= mark; i-- {
		undo := b.valueUndo[i]
		if undo.existed {
			b.valueHeads[undo.name] = undo.prior
		} else {
			delete(b.valueHeads, undo.name)
		}
	}
	b.valueUndo = b.valueUndo[:mark]
	b.valueMarks = b.valueMarks[:len(b.valueMarks)-1]
	b.popTypeScope()
	b.control.popBlock()
}

func (b *binder) define(name string, id Symbol) {
	if name == "" || len(b.valueMarks) == 0 || id == 0 {
		return
	}
	if b.valueHeads == nil {
		b.valueHeads = make(map[string]valueHead)
	}
	prior, existed := b.valueHeads[name]
	b.valueUndo = append(b.valueUndo, valueUndo{name: name, prior: prior, existed: existed})
	b.valueHeads[name] = valueHead{id: id, depth: len(b.valueMarks) - 1}
	b.control.define(id)
}

func (b *binder) newSymbol(name string, kind SymbolKind) Symbol {
	id := b.result.newSymbol(name, kind)
	if fn := b.currentFunction(); fn != nil {
		if b.declaringFunctions == nil {
			b.declaringFunctions = make(map[Symbol]*ast.FunctionExpr)
		}
		b.declaringFunctions[id] = fn
	}
	return id
}

func (b *binder) lookup(name string) (Symbol, bool, bool) {
	if name == "" {
		return 0, false, false
	}
	active, activeOK := b.valueHeads[name]
	pending, pendingOK := b.visiblePendingHead(name)
	if pendingOK && (!activeOK || pending.depth >= active.depth) {
		return pending.id, false, true
	}
	if activeOK {
		return active.id, false, true
	}
	if id := b.globals[name]; id != 0 {
		return id, true, true
	}
	return 0, false, false
}

func (b *binder) pushPending(names map[string]Symbol) int {
	mark := len(b.pending)
	if len(names) == 0 || len(b.valueMarks) == 0 {
		return mark
	}
	depth := len(b.valueMarks) - 1
	b.pending = append(b.pending, pendingScope{names: names})
	if b.pendingByName == nil {
		b.pendingByName = make(map[string][]pendingHead)
	}
	for name, id := range names {
		if name == "" || id == 0 {
			continue
		}
		b.pendingByName[name] = append(b.pendingByName[name], pendingHead{
			batch: mark,
			depth: depth,
			id:    id,
		})
	}
	return mark
}

func (b *binder) popPending(mark int) {
	if mark < 0 || mark > len(b.pending) {
		return
	}
	for i := len(b.pending) - 1; i >= mark; i-- {
		for name := range b.pending[i].names {
			heads := b.pendingByName[name]
			if len(heads) == 0 {
				continue
			}
			heads = heads[:len(heads)-1]
			if len(heads) == 0 {
				delete(b.pendingByName, name)
			} else {
				b.pendingByName[name] = heads
			}
		}
	}
	b.pending = b.pending[:mark]
	if b.visiblePending > mark {
		b.visiblePending = mark
	}
}

func (b *binder) visiblePendingHead(name string) (pendingHead, bool) {
	heads := b.pendingByName[name]
	limit := b.visiblePending
	if len(heads) == 0 || limit <= 0 {
		return pendingHead{}, false
	}
	lo, hi := 0, len(heads)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if heads[mid].batch < limit {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return pendingHead{}, false
	}
	return heads[lo-1], true
}

func (b *binder) bindReadIdent(ident *ast.IdentExpr) {
	id := b.bindReadIdentSymbol(ident)
	if id == 0 {
		return
	}
	b.recordDirectCapture(id)
}

func (b *binder) bindTypeQueryIdent(ident *ast.IdentExpr) {
	id := b.bindTypeQueryIdentSymbol(ident)
	if b.currentFunctionStatic() {
		b.recordDirectCapture(id)
	}
}

// bindTypeQueryIdentSymbol retains the ordinary lexical identity selected by
// source binding without publishing runtime read/global-use evidence. Static
// queries lower through the same Program expressions, but they are
// unreachable executable syntax.
func (b *binder) bindTypeQueryIdentSymbol(ident *ast.IdentExpr) Symbol {
	if ident == nil {
		return 0
	}
	id, _, ok := b.lookup(ident.Value)
	if !ok {
		if decl, hasType := b.lookupType(ident.Value); hasType {
			b.recordTypeValueRef(ident, decl)
		}
		id = b.global(ident.Value)
	}
	b.result.identSymbols[ident] = id
	if kind, ok := b.result.Kind(id); ok && kind == SymbolGlobal {
		b.result.observeGlobal(id, ident, true)
	}
	return id
}

func (b *binder) bindTypeQueryWriteIdent(ident *ast.IdentExpr) {
	if ident == nil {
		return
	}
	id, _, ok := b.lookup(ident.Value)
	if !ok {
		id = b.global(ident.Value)
	}
	b.result.identSymbols[ident] = id
	if kind, ok := b.result.Kind(id); ok && kind == SymbolGlobal {
		b.result.observeGlobal(id, ident, true)
	}
	if b.currentFunctionStatic() {
		b.recordDirectCapture(id)
	}
}

func (b *binder) bindReadIdentSymbol(ident *ast.IdentExpr) Symbol {
	if ident == nil {
		return 0
	}
	id, global, ok := b.lookup(ident.Value)
	if !ok {
		if decl, hasType := b.lookupType(ident.Value); hasType {
			b.recordTypeValueRef(ident, decl)
		}
		id = b.global(ident.Value)
		b.result.implicitGlobalUses[ident] = struct{}{}
		if b.implicitGlobalSymbols == nil {
			b.implicitGlobalSymbols = make(map[Symbol]struct{})
		}
		b.implicitGlobalSymbols[id] = struct{}{}
	} else if global {
		if _, staticOnly := b.staticOnlyGlobals[id]; staticOnly {
			delete(b.staticOnlyGlobals, id)
			b.result.implicitGlobalUses[ident] = struct{}{}
			if b.implicitGlobalSymbols == nil {
				b.implicitGlobalSymbols = make(map[Symbol]struct{})
			}
			b.implicitGlobalSymbols[id] = struct{}{}
		}
		if _, isImplicitGlobal := b.implicitGlobalSymbols[id]; isImplicitGlobal {
			if decl, hasType := b.lookupType(ident.Value); hasType {
				b.recordTypeValueRef(ident, decl)
			}
		}
	}
	b.result.identSymbols[ident] = id
	if kind, ok := b.result.Kind(id); ok && kind == SymbolGlobal {
		b.result.observeGlobal(id, ident, true)
	}
	return id
}

func (b *binder) bindWriteIdent(ident *ast.IdentExpr) {
	if ident == nil {
		return
	}
	id, _, ok := b.lookup(ident.Value)
	if !ok {
		id = b.global(ident.Value)
	}
	b.result.identSymbols[ident] = id
	if kind, ok := b.result.Kind(id); ok && kind == SymbolGlobal {
		b.result.observeGlobal(id, ident, true)
	}
	delete(b.staticOnlyGlobals, id)
	b.recordDirectCapture(id)
}

// recordTypeValueRef marks a value-position occurrence of a type name. Only an
// authored declaration can be read as a value: an ambient name declares an
// annotation spelling and has no value identity, so a global of the same name
// stays an ordinary value read.
func (b *binder) recordTypeValueRef(ident *ast.IdentExpr, decl TypeDecl) {
	if b == nil || b.result == nil || ident == nil || decl.ID == 0 || decl.Kind == TypeDeclAmbient {
		return
	}
	if b.result.typeValueRefs == nil {
		b.result.typeValueRefs = make(map[*ast.IdentExpr]TypeDecl)
	}
	b.result.typeValueRefs[ident] = decl
}
