package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// OccurrenceRole classifies one binder-owned runtime reference.
type OccurrenceRole uint8

const (
	OccurrenceRead OccurrenceRole = iota + 1
	OccurrenceWrite
	OccurrenceCapture
)

// Declaration records the parser-owned source token that introduces a value
// binder. Synthetic binders may have a syntax span but cannot be renamed as a
// textual identifier without a checker-verified transformation.
type Declaration struct {
	Span      source.Span
	Synthetic bool
}

func (d Declaration) Valid() bool { return d.Span.Valid() }

// Occurrence records one exact parser/binder-owned runtime reference.
type Occurrence struct {
	Role OccurrenceRole
	Span source.Span
}

func (o Occurrence) Valid() bool { return o.Span.Valid() }

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

// WriteIdents returns ordinary assignment target occurrences bound to id.
// Local declaration initializers are not writes; this records only mutations
// through assignment syntax after a symbol has been declared or discovered.
func (r *Result) WriteIdents(id symbol.ID) []*ast.IdentExpr {
	if r == nil || id == 0 {
		return nil
	}
	return cloneIdentExprs(r.writeIdents[id])
}

// Declaration returns the source declaration owned by the binder for id.
// A missing or synthetic declaration must never be re-derived from source
// text by a consumer.
func (r *Result) Declaration(id symbol.ID) (Declaration, bool) {
	if r == nil || id == 0 {
		return Declaration{}, false
	}
	declaration, ok := r.declarations[id]
	return declaration, ok
}

// Occurrences returns every runtime occurrence of id with its exact role and
// parser span. The returned slice never aliases binder-owned storage.
func (r *Result) Occurrences(id symbol.ID) []Occurrence {
	if r == nil || id == 0 {
		return nil
	}
	return append([]Occurrence(nil), r.occurrences[id]...)
}

// HasWrite reports whether id is assigned through ordinary assignment syntax.
func (r *Result) HasWrite(id symbol.ID) bool {
	if r == nil || id == 0 {
		return false
	}
	return len(r.writeIdents[id]) > 0
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

// GlobalIdentity is an opaque binder-owned global selection. It can only be
// obtained by resolving an identifier occurrence through Result.
type GlobalIdentity struct {
	id   symbol.ID
	name string
}

// GlobalIdentity resolves ident to its bound global declaration.
func (r *Result) GlobalIdentity(ident *ast.IdentExpr) (GlobalIdentity, bool) {
	if r == nil || ident == nil {
		return GlobalIdentity{}, false
	}
	id, ok := r.SymbolOf(ident)
	if !ok || !r.SymbolResolvesToGlobal(id, ident.Value) {
		return GlobalIdentity{}, false
	}
	return GlobalIdentity{id: id, name: ident.Value}, true
}

// Matches reports whether the bound global has the registry-selected name.
func (identity GlobalIdentity) Matches(name string) bool {
	return identity.id != 0 && identity.name != "" && identity.name == name
}

// ResolvesToGlobal reports whether ident is bound to the global named name.
func (r *Result) ResolvesToGlobal(ident *ast.IdentExpr, name string) bool {
	if r == nil || ident == nil || name == "" || ident.Value != name {
		return false
	}
	id, ok := r.SymbolOf(ident)
	return ok && r.SymbolResolvesToGlobal(id, name)
}

// SymbolResolvesToGlobal reports whether id is the global symbol with name.
func (r *Result) SymbolResolvesToGlobal(id symbol.ID, name string) bool {
	if r == nil || id == 0 || name == "" || r.Name(id) != name {
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
	if expr, ok := r.symbolAnnotations[id]; ok && expr != nil {
		return expr, true
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

// newSymbol allocates a declaration identity from this Result's own counter.
// Numbering is bind-local so identical source produces identical symbol IDs
// across independent solves, which keeps every downstream identity token and
// content digest deterministic. IDs are unique within a Result; they are never
// compared against symbols from another Result.
func (r *Result) newSymbol(name string, kind symbol.Kind) symbol.ID {
	r.nextSymbolID++
	id := r.nextSymbolID
	r.names[id] = name
	r.kinds[id] = kind
	return id
}

func (r *Result) setDeclaration(id symbol.ID, declaration Declaration) {
	if r == nil || id == 0 {
		return
	}
	r.declarations[id] = declaration
}

func (r *Result) addOccurrence(id symbol.ID, occurrence Occurrence) {
	if r == nil || id == 0 || !occurrence.Valid() {
		return
	}
	r.occurrences[id] = append(r.occurrences[id], occurrence)
}

func declarationForPosition(position ast.Position, name string, synthetic bool) Declaration {
	if !position.Valid() || name == "" {
		return Declaration{Synthetic: synthetic}
	}
	endLine, endColumn := position.EndLine, position.EndColumn
	if endLine == 0 {
		endLine = position.Line
	}
	if endLine == position.Line && endColumn < position.Column+len(name)-1 {
		endColumn = position.Column + len(name) - 1
	}
	return Declaration{
		Span:      source.Span{StartLine: position.Line, StartCol: position.Column, EndLine: endLine, EndCol: endColumn},
		Synthetic: synthetic,
	}
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

type pendingScope struct {
	names map[string]symbol.ID
}

type valueHead struct {
	id    symbol.ID
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
	id    symbol.ID
}

type binder struct {
	result *Result

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

func (b *binder) define(name string, id symbol.ID) {
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
	active, activeOK := b.valueHeads[name]
	pending, pendingOK := b.visiblePendingHead(name)
	if pendingOK && (!activeOK || pending.depth >= active.depth) {
		return pending.id, false, true
	}
	if activeOK {
		return active.id, false, true
	}
	if g, ok := b.result.globals[name]; ok {
		return g.id, true, true
	}
	return 0, false, false
}

func (b *binder) pushPending(names map[string]symbol.ID) int {
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
	b.result.readIdents[id] = append(b.result.readIdents[id], ident)
	b.result.addOccurrence(id, Occurrence{Role: b.occurrenceRole(id, OccurrenceRead), Span: ast.SpanOf(ident)})
	b.recordDirectCapture(id)
	b.recordDirectGlobalRead(id)
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
	b.result.writeIdents[id] = append(b.result.writeIdents[id], ident)
	b.result.addOccurrence(id, Occurrence{Role: b.occurrenceRole(id, OccurrenceWrite), Span: ast.SpanOf(ident)})
	b.recordDirectCapture(id)
}

func (b *binder) occurrenceRole(id symbol.ID, role OccurrenceRole) OccurrenceRole {
	current := b.currentFunction()
	if current == nil {
		return role
	}
	kind, ok := b.result.kinds[id]
	if !ok || (kind != symbol.Local && kind != symbol.Param) {
		return role
	}
	if b.result.declaringFunctions[id] != current {
		return OccurrenceCapture
	}
	return role
}
