// Package lexical owns atomic Body publication, symbol visibility, and closures.
package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	modulelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/module"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Bodies is the one atomic authority for lexical scopes and their Program roots.
// Its fields stay private so restoring symbol visibility and publishing a Body
// cannot be split across packages.
type Bodies struct {
	collector  *assembly.Collector
	binding    *bind.Result
	sourceName string
	modules    *modulelower.Writer
	phases     *continuation.Stack
	values     *eval.Values
	statics    *continuation.Statics
	localSteps []localStep

	active map[bind.Symbol]keyspace.Term
	undo   []activeUndo
	frames []bodyFrame
	source []sourceItem

	cellInline   [4]keyspace.Term
	cellOverflow []keyspace.Term
	cellLen      int
	chunkVararg  keyspace.Term
	reserved     []reservedCell
	reservedIDs  map[bind.Symbol]struct{}
	captures     []flow.Capture
}

// localStep is lexical-private ordinary-local construction state. It carries
// the exact visibility transaction from reservation through Bind; no other
// vertical may reconstruct or complete that transaction.
type localStep struct {
	owner        keyspace.Term
	span         source.Span
	stmt         *ast.LocalAssignStmt
	cellMark     int
	reservedMark int
	index        int
	kind         localStepKind
}

type localStepKind uint8

const (
	localDeclaredTypes localStepKind = iota + 1
	localBind
)

type activeUndo struct {
	id      bind.Symbol
	prior   keyspace.Term
	existed bool
}

type reservedCell struct {
	id   bind.Symbol
	cell keyspace.Term
}

type bodyFrame struct {
	body       keyspace.Term
	function   *ast.FunctionExpr
	undoMark   int
	sourceMark int
}

// sourceItem retains the one authored Body order until its Body closes. A zero
// term with a nonzero cell is typed evidence for one source turn whose Cell is
// declared later; lexical never decides which semantic Term fills that turn.
type sourceItem struct {
	term keyspace.Term
	cell bind.Symbol
}

// CellEvidence identifies one reserved source turn and the later lexical Cell
// it requires. Its representation is private so only Bodies can resolve or
// fill it; semantic children retain the typed value without reconstructing
// lexical coordinates.
type CellEvidence struct {
	owner  keyspace.Term
	source int
	cell   bind.Symbol
}

// New creates the one lexical authority for one unfinished Program. It binds
// the sole phase stack once; copied lexical state or caller-supplied alternate
// stacks could split Cell visibility from Body publication and are forbidden.
func New(
	phases *continuation.Stack,
	collector *assembly.Collector,
	binding *bind.Result,
	sourceName string,
	modules *modulelower.Writer,
	values *eval.Values,
	statics *continuation.Statics,
) *Bodies {
	return &Bodies{
		collector:  collector,
		binding:    binding,
		sourceName: sourceName,
		modules:    modules,
		phases:     phases,
		values:     values,
		statics:    statics,
	}
}

// ScheduleVararg owns vararg identity because the Cell is lexical evidence
// from the active function boundary. Source dispatches the exact Comma3 AST
// case here; eval never reconstructs binder-backed function scope. The caller
// must name the currently active Body, preserving the same ownership boundary
// as ordinary local scheduling.
func (b *Bodies) ScheduleVararg(expr *ast.Comma3Expr, owner keyspace.Term, span source.Span) error {
	if b == nil || b.collector == nil || b.binding == nil || b.phases == nil || expr == nil || owner == 0 {
		return fmt.Errorf("lualower: invalid lexical vararg authority")
	}
	if len(b.frames) == 0 || owner != b.Owner() {
		return fmt.Errorf("lualower: vararg request crossed Body boundary")
	}
	cell, err := b.Vararg(span)
	if err != nil {
		return err
	}
	term := b.collector.Vararg(span, owner, cell)
	if term == 0 {
		return fmt.Errorf("lualower: could not create Vararg")
	}
	b.phases.SetResult(term, !expr.AdjustRet)
	return nil
}

// Entry creates, selects, and enters the canonical chunk Body.
func (b *Bodies) Entry(span source.Span) (keyspace.Term, error) {
	if b == nil || b.collector == nil {
		return 0, fmt.Errorf("lualower: invalid lexical source authority")
	}
	body := b.collector.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("lualower: could not create chunk body")
	}
	if !b.collector.SetEntry(body) {
		return 0, fmt.Errorf("lualower: could not set chunk Entry")
	}
	b.enter(body, nil)
	return body, nil
}

// EnterBlock creates one child Body that inherits its function boundary.
func (b *Bodies) EnterBlock(span source.Span) (keyspace.Term, error) {
	function := b.frames[len(b.frames)-1].function
	return b.enterBody(span, function)
}

// EnterFunction creates one child Body at a new function boundary.
func (b *Bodies) EnterFunction(
	span source.Span,
	function *ast.FunctionExpr,
) (keyspace.Term, error) {
	if function == nil {
		return 0, fmt.Errorf("lualower: nil function boundary")
	}
	return b.enterBody(span, function)
}

func (b *Bodies) enterBody(
	span source.Span,
	function *ast.FunctionExpr,
) (keyspace.Term, error) {
	body := b.collector.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("lualower: could not create Body")
	}
	b.enter(body, function)
	return body, nil
}

func (b *Bodies) enter(body keyspace.Term, function *ast.FunctionExpr) {
	b.frames = append(b.frames, bodyFrame{
		body:       body,
		function:   function,
		undoMark:   len(b.undo),
		sourceMark: len(b.source),
	})
}

// Owner returns the active lexical Body.
func (b *Bodies) Owner() keyspace.Term {
	return b.frames[len(b.frames)-1].body
}

// Finish atomically publishes the current Body, restores its lexical mappings,
// and leaves its parent active.
func (b *Bodies) Finish() (keyspace.Term, error) {
	if len(b.frames) == 0 {
		return 0, fmt.Errorf("lualower: no lexical body to finalize")
	}
	owner := b.Owner()
	for _, current := range b.localSteps {
		if current.owner == owner {
			return 0, fmt.Errorf("lualower: unfinished lexical local transaction")
		}
	}
	frame := b.frames[len(b.frames)-1]
	items := b.source[frame.sourceMark:]
	terms := make([]keyspace.Term, len(items))
	for index, item := range items {
		if item.term == 0 || item.cell != 0 {
			return 0, fmt.Errorf("lualower: unresolved Body source evidence")
		}
		terms[index] = item.term
	}
	if !b.collector.SetBody(frame.body, terms...) {
		return 0, fmt.Errorf("lualower: could not finalize Body")
	}
	b.source = b.source[:frame.sourceMark]
	b.restore(frame.undoMark)
	b.frames = b.frames[:len(b.frames)-1]
	return frame.body, nil
}

// Append publishes one existing statement, Label, TypeAlias, or Interface
// Term in exact authored order. Seal derives root and frontier projections.
func (b *Bodies) Append(term keyspace.Term) error {
	if term == 0 {
		return fmt.Errorf("lualower: could not append Body source Term")
	}
	b.source = append(b.source, sourceItem{term: term})
	return nil
}

// ReserveCell appends one authored source turn whose semantic Term depends on
// a Cell declared later in the same Body. It returns lexical evidence only;
// the owning semantic child must resolve and fill it explicitly before Finish.
func (b *Bodies) ReserveCell(cell bind.Symbol) (CellEvidence, error) {
	if cell == 0 || len(b.frames) == 0 {
		return CellEvidence{}, fmt.Errorf("lualower: invalid Body source Cell evidence")
	}
	evidence := CellEvidence{owner: b.Owner(), source: len(b.source), cell: cell}
	b.source = append(b.source, sourceItem{cell: cell})
	return evidence, nil
}

// ResolveCell returns the exact currently visible Cell required by evidence.
func (b *Bodies) ResolveCell(evidence CellEvidence) (keyspace.Term, error) {
	if !b.validEvidence(evidence) {
		return 0, fmt.Errorf("lualower: invalid Body source Cell evidence")
	}
	cell, ok := b.Resolve(evidence.cell)
	if !ok || cell == 0 {
		return 0, fmt.Errorf("lualower: Body source Cell is absent")
	}
	return cell, nil
}

// Fill replaces one reserved source turn with the semantic Term created by
// its owning child. It cannot change source order or be applied twice.
func (b *Bodies) Fill(evidence CellEvidence, term keyspace.Term) error {
	if term == 0 || !b.validEvidence(evidence) {
		return fmt.Errorf("lualower: invalid Body source evidence fill")
	}
	item := &b.source[evidence.source]
	if item.term != 0 || item.cell != evidence.cell {
		return fmt.Errorf("lualower: Body source evidence already filled")
	}
	item.term = term
	item.cell = 0
	return nil
}

func (b *Bodies) validEvidence(evidence CellEvidence) bool {
	if len(b.frames) == 0 || evidence.owner == 0 || evidence.owner != b.Owner() ||
		evidence.source < b.frames[len(b.frames)-1].sourceMark ||
		evidence.source < 0 || evidence.source >= len(b.source) || evidence.cell == 0 {
		return false
	}
	item := b.source[evidence.source]
	return item.term == 0 && item.cell == evidence.cell
}

// Resolve returns the currently visible Cell for one binder identity.
func (b *Bodies) Resolve(id bind.Symbol) (keyspace.Term, bool) {
	term, ok := b.active[id]
	return term, ok && term != 0
}

// Vararg resolves the active function boundary's exact vararg Cell from the
// construction's sole binder authority. The chunk has one implicit Cell,
// minted lazily only when authored source uses "...".
func (b *Bodies) Vararg(span source.Span) (keyspace.Term, error) {
	if b == nil || b.binding == nil || len(b.frames) == 0 {
		return 0, fmt.Errorf("lualower: vararg expression outside Body")
	}
	function := b.frames[len(b.frames)-1].function
	if function == nil {
		if b.chunkVararg != 0 {
			return b.chunkVararg, nil
		}
		entry := b.frames[0].body
		if b.collector == nil || b.collector.Entry() != entry {
			return 0, fmt.Errorf("lualower: chunk vararg requires the entry Body")
		}
		cell := b.collector.Cell(span, entry)
		if cell == 0 {
			return 0, fmt.Errorf("lualower: could not create chunk vararg Cell")
		}
		b.chunkVararg = cell
		return cell, nil
	}
	id, ok := b.binding.VarargSymbol(function)
	if !ok {
		return 0, fmt.Errorf("lualower: vararg expression in non-vararg function")
	}
	cell, ok := b.Resolve(id)
	if !ok {
		return 0, fmt.Errorf("lualower: missing vararg Cell")
	}
	return cell, nil
}

// Has reports whether a binder identity already has an active Cell.
func (b *Bodies) Has(id bind.Symbol) bool {
	_, ok := b.active[id]
	return ok
}

// CellMark identifies the start of one local or formal Cell group.
func (b *Bodies) CellMark() int {
	return b.cellLen
}

// CaptureMark identifies the start of one closure capture group.
func (b *Bodies) CaptureMark() int {
	return len(b.captures)
}

// Declare creates and installs one local, formal, or loop Cell.
func (b *Bodies) Declare(id bind.Symbol, span source.Span) (keyspace.Term, error) {
	return b.declare(id, span, true)
}

// Lua's ordinary local boundary is preserved exactly: Cells are reserved and
// may receive declared types, but their binder identities stay invisible until
// the initializer Values have completed and the Bind succeeds.
func (b *Bodies) ScheduleLocal(stmt *ast.LocalAssignStmt, owner keyspace.Term, span source.Span) error {
	if b == nil || b.collector == nil || b.binding == nil || b.modules == nil || b.phases == nil || b.values == nil || b.statics == nil || stmt == nil {
		return fmt.Errorf("lualower: invalid lexical local authority")
	}
	if owner == 0 || owner != b.Owner() || span.File == "" || len(stmt.Names) == 0 || len(stmt.Types) > len(stmt.Names) {
		return fmt.Errorf("lualower: invalid local declaration")
	}
	cellMark, reservedMark := b.CellMark(), len(b.reserved)
	for index := range stmt.Names {
		id, ok := b.binding.LocalSymbolAt(stmt, index)
		if !ok || id == 0 || b.Has(id) {
			return fmt.Errorf("lualower: invalid binder symbol for local slot %d", index)
		}
		if _, err := b.reserve(id, b.nameSpan(stmt, index)); err != nil {
			return fmt.Errorf("lualower: could not reserve local Cell %d: %w", index, err)
		}
	}
	b.localSteps = append(b.localSteps, localStep{
		kind:         localDeclaredTypes,
		owner:        owner,
		span:         span,
		stmt:         stmt,
		cellMark:     cellMark,
		reservedMark: reservedMark,
	})
	b.phases.Push(continuation.Lexical)
	return nil
}

// Run advances one lexical-private ordinary-local continuation. Its only
// external work uses the direct Values and typed Static capabilities injected
// at construction; source never reconstructs this transaction.
func (b *Bodies) Run() error {
	if b == nil || b.phases == nil || len(b.localSteps) == 0 {
		return fmt.Errorf("lualower: missing lexical continuation")
	}
	last := len(b.localSteps) - 1
	current := b.localSteps[last]
	b.localSteps = b.localSteps[:last]
	if current.owner == 0 || current.owner != b.Owner() {
		return fmt.Errorf("lualower: lexical local continuation crossed Body boundary")
	}

	switch current.kind {
	case localDeclaredTypes:
		return b.runLocalDeclaredTypes(current)
	case localBind:
		values, open := b.phases.Result()
		if values == 0 || open {
			return fmt.Errorf("lualower: invalid local initializer Values")
		}
		if err := b.bindReserved(current.cellMark, current.reservedMark, current.span, values); err != nil {
			return err
		}
		if len(current.stmt.Names) != 1 {
			return nil
		}
		id, ok := b.binding.LocalSymbolAt(current.stmt, 0)
		if !ok || id == 0 {
			return fmt.Errorf("lualower: missing one-cell local binder identity")
		}
		cell, ok := b.Resolve(id)
		if !ok || cell == 0 {
			return fmt.Errorf("lualower: missing one-cell local Cell")
		}
		return b.modules.AttachAlias(current.stmt, cell)
	default:
		return fmt.Errorf("lualower: invalid lexical local continuation %d", current.kind)
	}
}

func (b *Bodies) runLocalDeclaredTypes(current localStep) error {
	stmt := current.stmt
	if stmt == nil || current.index < 0 || current.index > len(stmt.Names) {
		return fmt.Errorf("lualower: invalid local declared-type cursor")
	}
	if current.index == len(stmt.Names) {
		current.kind = localBind
		b.pushLocal(current)
		return b.values.ScheduleValues(stmt.Exprs, current.owner, current.span)
	}

	index := current.index
	current.index++
	if index >= len(stmt.Types) || stmt.Types[index] == nil {
		b.pushLocal(current)
		return nil
	}
	declared := stmt.Types[index]
	id, ok := b.binding.LocalSymbolAt(stmt, index)
	if !ok || id == 0 {
		return fmt.Errorf("lualower: binder has no symbol for typed local slot %d", index)
	}
	bound, ok := b.binding.SymbolTypeAnnotation(id)
	if !ok || bound != declared {
		return fmt.Errorf("lualower: binder has mismatched declared type for local slot %d", index)
	}
	host, ok := b.reservedCell(current.reservedMark, index)
	if !ok || host == 0 {
		return fmt.Errorf("lualower: missing reserved Cell for typed local slot %d", index)
	}
	b.pushLocal(current)
	return b.statics.PushDeclaredCell(declared, host, current.owner, b.span(declared))
}

func (b *Bodies) pushLocal(current localStep) {
	b.localSteps = append(b.localSteps, current)
	b.phases.Push(continuation.Lexical)
}

// reserve creates one ordinary-local Cell without installing its binder
// identity. It is deliberately private so ScheduleLocal is the only ordinary
// local route.
func (b *Bodies) reserve(id bind.Symbol, span source.Span) (keyspace.Term, error) {
	if id == 0 {
		return 0, fmt.Errorf("lualower: cannot reserve zero binder symbol")
	}
	if _, exists := b.active[id]; exists {
		return 0, fmt.Errorf("lualower: duplicate active binder symbol")
	}
	if _, exists := b.reservedIDs[id]; exists {
		return 0, fmt.Errorf("lualower: duplicate reserved binder symbol")
	}
	cell := b.collector.Cell(span, b.Owner())
	if cell == 0 {
		return 0, fmt.Errorf("lualower: could not reserve Cell")
	}
	b.appendCell(cell)
	b.reserved = append(b.reserved, reservedCell{id: id, cell: cell})
	if b.reservedIDs == nil {
		b.reservedIDs = make(map[bind.Symbol]struct{})
	}
	b.reservedIDs[id] = struct{}{}
	return cell, nil
}

// DeclareLoop creates and installs one per-iteration Cell without retaining it
// in local/formal construction scratch.
func (b *Bodies) DeclareLoop(id bind.Symbol, span source.Span) (keyspace.Term, error) {
	return b.declare(id, span, false)
}

func (b *Bodies) declare(
	id bind.Symbol,
	span source.Span,
	retain bool,
) (keyspace.Term, error) {
	if id == 0 {
		return 0, fmt.Errorf("lualower: cannot declare zero binder symbol")
	}
	if _, exists := b.active[id]; exists {
		return 0, fmt.Errorf("lualower: duplicate active binder symbol")
	}
	owner := b.Owner()
	cell := b.collector.Cell(span, owner)
	if cell == 0 {
		return 0, fmt.Errorf("lualower: could not create Cell")
	}
	b.install(id, cell)
	if retain {
		b.appendCell(cell)
	}
	return cell, nil
}

// Bind completes the Function-owned recursive-local declaration group. Ordinary
// locals use ScheduleLocal's private bindReserved transaction instead.
func (b *Bodies) Bind(mark int, span source.Span, values keyspace.Term) error {
	if mark < 0 || mark > b.cellLen {
		return fmt.Errorf("lualower: invalid local Cell mark")
	}
	owner := b.Owner()
	term := b.collector.Bind(span, owner, b.cellSlice()[mark:], values)
	b.truncateCells(mark)
	if term == 0 {
		return fmt.Errorf("lualower: could not lower local declaration")
	}
	b.source = append(b.source, sourceItem{term: term})
	return nil
}

// reservedCell returns one Cell in the private ordinary-local transaction.
func (b *Bodies) reservedCell(mark, index int) (keyspace.Term, bool) {
	at := mark + index
	if mark < 0 || index < 0 || at < mark || at >= len(b.reserved) {
		return 0, false
	}
	return b.reserved[at].cell, true
}

// RetainedCell returns one Cell in the active local/formal construction range.
func (b *Bodies) RetainedCell(mark, index int) (keyspace.Term, bool) {
	at := mark + index
	cells := b.cellSlice()
	if mark < 0 || index < 0 || at < mark || at >= len(cells) {
		return 0, false
	}
	return cells[at], true
}

// bindReserved publishes one ordinary-local group after its initializer and
// declared types were lowered. Installation happens only after Collector Bind
// succeeds, preserving Lua's initializer visibility boundary.
func (b *Bodies) bindReserved(cellMark, reservedMark int, span source.Span, values keyspace.Term) error {
	if cellMark < 0 || cellMark > b.cellLen || reservedMark < 0 || reservedMark > len(b.reserved) {
		return fmt.Errorf("lualower: invalid reserved local mark")
	}
	cells := b.cellSlice()[cellMark:]
	reserved := b.reserved[reservedMark:]
	if len(cells) != len(reserved) {
		return fmt.Errorf("lualower: reserved local Cell range mismatch")
	}
	for index, item := range reserved {
		if item.id == 0 || item.cell != cells[index] || b.Has(item.id) {
			return fmt.Errorf("lualower: invalid reserved local Cell")
		}
		if _, exists := b.reservedIDs[item.id]; !exists {
			return fmt.Errorf("lualower: missing reserved binder symbol")
		}
	}
	owner := b.Owner()
	term := b.collector.Bind(span, owner, cells, values)
	if term == 0 {
		return fmt.Errorf("lualower: could not lower reserved local declaration")
	}
	for _, item := range reserved {
		b.install(item.id, item.cell)
		delete(b.reservedIDs, item.id)
	}
	b.truncateCells(cellMark)
	b.reserved = b.reserved[:reservedMark]
	b.source = append(b.source, sourceItem{term: term})
	return nil
}

// Capture creates and installs one inner Cell while retaining its exact outer.
func (b *Bodies) Capture(
	id bind.Symbol,
	span source.Span,
	outer keyspace.Term,
) (keyspace.Term, error) {
	owner := b.Owner()
	inner := b.collector.Cell(span, owner)
	if inner == 0 {
		return 0, fmt.Errorf("lualower: could not create capture Cell")
	}
	b.captures = append(b.captures, flow.Capture{Inner: inner, Outer: outer})
	b.install(id, inner)
	return inner, nil
}

// DeclareFunction mints the closure identity before entering its child Body.
// Seal later derives the generic-constraint frontier from its exact source
// occurrence; the lowerer supplies no placement number.
func (b *Bodies) DeclareFunction(span source.Span) (keyspace.Term, error) {
	if len(b.frames) == 0 {
		return 0, fmt.Errorf("lualower: Function has no lexical owner")
	}
	term := b.collector.DeclareFunction(span, b.Owner())
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not declare Function")
	}
	return term, nil
}

// FillFunction completes a previously declared closure from cells and
// captures installed by the machine's binder-ordered traversal.
func (b *Bodies) FillFunction(
	function keyspace.Term,
	cellMark int,
	captureMark int,
	varargIndex int,
) error {
	if len(b.frames) < 2 {
		return fmt.Errorf("lualower: Function has no lexical owner")
	}
	if cellMark < 0 || cellMark > b.cellLen ||
		captureMark < 0 || captureMark > len(b.captures) {
		return fmt.Errorf("lualower: invalid Function construction mark")
	}
	params := b.cellSlice()[cellMark:]
	formals := params
	var vararg keyspace.Term
	if varargIndex >= 0 {
		if varargIndex >= len(params) {
			return fmt.Errorf("lualower: invalid function vararg Cell")
		}
		if varargIndex != len(params)-1 {
			return fmt.Errorf("lualower: function vararg Cell is not final")
		}
		vararg = params[varargIndex]
		formals = params[:varargIndex]
	}
	body := b.frames[len(b.frames)-1].body
	if !b.collector.FillFunction(function, body, formals, vararg, b.captures[captureMark:]) {
		return fmt.Errorf("lualower: could not fill Function")
	}
	b.truncateCells(cellMark)
	b.captures = b.captures[:captureMark]
	return nil
}

func (b *Bodies) install(id bind.Symbol, term keyspace.Term) {
	if b.active == nil {
		b.active = make(map[bind.Symbol]keyspace.Term)
	}
	prior, existed := b.active[id]
	b.undo = append(b.undo, activeUndo{id: id, prior: prior, existed: existed})
	b.active[id] = term
}

func (b *Bodies) appendCell(cell keyspace.Term) {
	if b.cellLen < len(b.cellInline) {
		b.cellInline[b.cellLen] = cell
		b.cellLen++
		return
	}
	if b.cellLen == len(b.cellInline) {
		b.cellOverflow = append(b.cellOverflow[:0], b.cellInline[:]...)
	}
	b.cellOverflow = append(b.cellOverflow, cell)
	b.cellLen++
}

func (b *Bodies) cellSlice() []keyspace.Term {
	if b.cellLen <= len(b.cellInline) {
		return b.cellInline[:b.cellLen]
	}
	return b.cellOverflow[:b.cellLen]
}

func (b *Bodies) truncateCells(mark int) {
	b.cellLen = mark
	if mark <= len(b.cellInline) {
		b.cellOverflow = b.cellOverflow[:0]
		return
	}
	b.cellOverflow = b.cellOverflow[:mark]
}

func (b *Bodies) restore(mark int) {
	for i := len(b.undo) - 1; i >= mark; i-- {
		undo := b.undo[i]
		if undo.existed {
			b.active[undo.id] = undo.prior
		} else {
			delete(b.active, undo.id)
		}
	}
	b.undo = b.undo[:mark]
}

func (b *Bodies) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: b.sourceName}
	}
	span, ok := coord.Build(b.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(b.sourceName)
	}
	return span
}

func (b *Bodies) nameSpan(stmt *ast.LocalAssignStmt, index int) source.Span {
	if stmt == nil || index < 0 || index >= len(stmt.NamePositions) {
		return b.span(stmt)
	}
	position := stmt.NamePositions[index]
	if !position.Valid() {
		if position.Line == 0 && position.Column == 0 && position.EndLine == 0 && position.EndColumn == 0 {
			return source.Span{File: b.sourceName}
		}
		return coord.Invalid(b.sourceName)
	}
	span, ok := coord.Build(b.sourceName, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(b.sourceName)
	}
	return span
}

// Clean reports whether every lexical transaction and scratch range completed.
func (b *Bodies) Clean() bool {
	return len(b.frames) == 0 &&
		len(b.source) == 0 &&
		len(b.active) == 0 &&
		len(b.undo) == 0 &&
		b.cellLen == 0 &&
		len(b.cellOverflow) == 0 &&
		len(b.reserved) == 0 &&
		len(b.reservedIDs) == 0 &&
		len(b.captures) == 0 &&
		len(b.localSteps) == 0
}
