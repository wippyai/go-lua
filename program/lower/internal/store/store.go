// Package store owns evaluated storage identities and mutation construction.
package store

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	lowercollector "github.com/wippyai/go-lua/program/lower/internal/collector"
	"github.com/wippyai/go-lua/program/lower/internal/eval"
	"github.com/wippyai/go-lua/program/lower/internal/inbox"
	"github.com/wippyai/go-lua/program/lower/internal/lexical"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
	staticlower "github.com/wippyai/go-lua/program/lower/internal/static"
	"github.com/wippyai/go-lua/program/source"
)

// TargetMark is an opaque boundary around one ordered assignment-target range.
// A caller can retain and pass it back, but cannot manufacture a raw scratch
// offset or inspect the private range it denotes.
type TargetMark struct {
	owner  *Writer
	target int
}

// TableMark is an opaque boundary around one nested table-constructor range.
// Keys, field kinds, and completed fields always move together.
type TableMark struct {
	owner            *Writer
	key, kind, field int
}

// Writer is the sole lowering authority for storage selection, reads, lenses,
// writes, and table-field construction. It has direct concrete dependencies;
// it does not call back into a parent lowerer.
type Writer struct {
	stack       *phase.Stack
	binding     *bind.Result
	lexical     *lexical.Bodies
	values      *eval.Values
	expressions *inbox.Expressions
	static      *staticlower.Writer
	collector   *lowercollector.Collector
	sourceName  string

	targets     []keyspace.Term
	targetSpans []source.Span
	tableKeys   []keyspace.Term
	tableKinds  []flowkind.FieldKind
	tableFields []keyspace.Term
	steps       []step
}

// New creates the one storage authority for a source Program assembly.
func New(
	stack *phase.Stack,
	binding *bind.Result,
	lexical *lexical.Bodies,
	values *eval.Values,
	expressions *inbox.Expressions,
	static *staticlower.Writer,
	collector *lowercollector.Collector,
	sourceName string,
) *Writer {
	return &Writer{
		stack:       stack,
		binding:     binding,
		lexical:     lexical,
		values:      values,
		expressions: expressions,
		static:      static,
		collector:   collector,
		sourceName:  sourceName,
	}
}

// TargetMark starts one delayed assignment-target range.
func (w *Writer) TargetMark() TargetMark {
	return TargetMark{owner: w, target: len(w.targets)}
}

// RememberTarget retains one evaluated target in source order.
func (w *Writer) RememberTarget(span source.Span, target keyspace.Term) error {
	if w == nil || target == 0 {
		return fmt.Errorf("programlower: invalid assignment target")
	}
	w.targets = append(w.targets, target)
	w.targetSpans = append(w.targetSpans, span)
	return nil
}

// Assign commits one delayed target group, then records its additive static
// publication metadata before its lexical Body observes the statement. assign
// is nil only for a Function-owned definition commit, whose target is already
// exact and which has no assignment-publication syntax. Import facts remain
// candidates; Rules later test the abstract callee value at each Call.
func (w *Writer) Assign(
	span source.Span,
	owner keyspace.Term,
	mark TargetMark,
	values keyspace.Term,
	assign *ast.AssignStmt,
) (keyspace.Term, error) {
	if w == nil || w.collector == nil || w.static == nil ||
		mark.owner != w || mark.target < 0 || mark.target > len(w.targets) {
		return 0, fmt.Errorf("programlower: invalid assignment target mark")
	}
	if assign != nil && len(assign.Lhs) != len(w.targets)-mark.target {
		return 0, fmt.Errorf("programlower: assignment source targets disagree with evaluated targets")
	}
	targets := w.targets[mark.target:]
	targetSpans := make([]source.Span, len(w.targetSpans)-mark.target)
	for index, targetSpan := range w.targetSpans[mark.target:] {
		targetSpans[index] = targetSpan
	}
	term := w.collector.Flow().Storage().Assign(
		span, owner, targets, targetSpans, values,
	)
	w.targets = w.targets[:mark.target]
	w.targetSpans = w.targetSpans[:mark.target]
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower assignment")
	}
	if assign != nil {
		if err := w.static.PublishTypePublications(assign, term); err != nil {
			return 0, err
		}
	}
	return term, nil
}

// Global selects the one Program-scoped Cell for a binder-authorized identity.
func (w *Writer) Global(identity bind.GlobalIdentity) (keyspace.Term, error) {
	if w == nil || w.collector == nil || !identity.Valid() {
		return 0, fmt.Errorf("programlower: missing storage authority")
	}
	term := w.collector.Flow().Storage().Global(identity)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not select global Cell")
	}
	return term, nil
}

// Read records an ordinary observation of an already selected Cell or Lens.
// Implicit-global evidence is selected only by ScheduleExpression from binder
// evidence and Static's owned depth; callers never supply that policy.
func (w *Writer) Read(span source.Span, owner, source keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("programlower: missing storage authority")
	}
	term := w.collector.Flow().Storage().Read(span, owner, source)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not read storage")
	}
	return term, nil
}

func (w *Writer) implicitRead(span source.Span, owner, global keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("programlower: missing storage authority")
	}
	term := w.collector.Flow().Storage().ImplicitRead(span, owner, global)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not read implicit global Cell")
	}
	return term, nil
}

// ResolveCell selects an identifier's exact Cell without evaluating it. It is
// for source-only consumers that need storage identity rather than a Read.
func (w *Writer) ResolveCell(expr *ast.IdentExpr) (keyspace.Term, error) {
	if w == nil || w.binding == nil || w.lexical == nil || expr == nil {
		return 0, fmt.Errorf("programlower: invalid identifier storage selection")
	}
	id, ok := w.binding.SymbolOf(expr)
	if !ok || id == 0 {
		return 0, fmt.Errorf("programlower: binder has no symbol for identifier occurrence")
	}
	if cell, visible := w.lexical.Resolve(id); visible {
		return cell, nil
	}
	identity, global := w.binding.GlobalIdentity(expr)
	if !global {
		return 0, fmt.Errorf("programlower: unsupported non-local identifier binding")
	}
	return w.Global(identity)
}

// DotLens records a parser-authored exact field Lens.
func (w *Writer) DotLens(
	span source.Span,
	owner keyspace.Term,
	base keyspace.Term,
	nameSpan source.Span,
	name string,
) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("programlower: missing storage authority")
	}
	key := w.collector.Source().Keys().Name(nameSpan, owner, name)
	if key == 0 {
		return 0, fmt.Errorf("programlower: could not create attribute Name")
	}
	lens := w.collector.Flow().Access().LensExact(
		span, owner, base, key, flowkind.FieldName,
	)
	if lens == 0 {
		return 0, fmt.Errorf("programlower: could not create Lens")
	}
	return lens, nil
}

// IndexLens records one exact or dynamic bracket field target.
func (w *Writer) IndexLens(
	span source.Span,
	owner keyspace.Term,
	base keyspace.Term,
	key keyspace.Term,
	source ast.Expr,
) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("programlower: missing storage authority")
	}
	var term keyspace.Term
	if keyKind(source) == flowkind.FieldExact {
		term = w.collector.Flow().Access().LensExact(
			span, owner, base, key, flowkind.FieldExact,
		)
	} else {
		term = w.collector.Flow().Access().LensKey(span, owner, base, key)
	}
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create Lens")
	}
	return term, nil
}

// DeclareTable reserves allocation before its constructor fields evaluate.
func (w *Writer) DeclareTable(span source.Span, owner keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("programlower: missing storage authority")
	}
	term := w.collector.Flow().Tables().DeclareTable(span, owner)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not declare table allocation")
	}
	return term, nil
}

// TableMark starts one table constructor's private scratch ranges.
func (w *Writer) TableMark() TableMark {
	return TableMark{owner: w, key: len(w.tableKeys), kind: len(w.tableKinds), field: len(w.tableFields)}
}

// ListField records one generated list key in source order.
func (w *Writer) ListField(span source.Span, owner keyspace.Term, ordinal int64) error {
	if w == nil || w.collector == nil {
		return fmt.Errorf("programlower: missing storage authority")
	}
	key := w.collector.Source().Keys().List(span, owner, ordinal)
	if key == 0 {
		return fmt.Errorf("programlower: could not create table list key")
	}
	w.tableKeys = append(w.tableKeys, key)
	w.tableKinds = append(w.tableKinds, flowkind.FieldList)
	return nil
}

// NameField records one parser-authored name key in source order.
func (w *Writer) NameField(span source.Span, owner keyspace.Term, value string) error {
	if w == nil || w.collector == nil {
		return fmt.Errorf("programlower: missing storage authority")
	}
	key := w.collector.Source().Keys().Name(span, owner, value)
	if key == 0 {
		return fmt.Errorf("programlower: could not create table field Name")
	}
	w.tableKeys = append(w.tableKeys, key)
	w.tableKinds = append(w.tableKinds, flowkind.FieldName)
	return nil
}

// KeyField retains one evaluated bracket key and its exact/dynamic policy.
func (w *Writer) KeyField(key keyspace.Term, source ast.Expr) error {
	if w == nil || key == 0 || source == nil {
		return fmt.Errorf("programlower: invalid table field key")
	}
	w.tableKeys = append(w.tableKeys, key)
	w.tableKinds = append(w.tableKinds, keyKind(source))
	return nil
}

// Field completes one field against an already declared allocation.
func (w *Writer) Field(span source.Span, table, values keyspace.Term) error {
	if w == nil || len(w.tableKeys) == 0 || len(w.tableKinds) == 0 {
		return fmt.Errorf("programlower: missing table field key")
	}
	last := len(w.tableKeys) - 1
	if w.collector == nil {
		return fmt.Errorf("programlower: missing storage authority")
	}
	field := w.collector.Flow().Tables().TableField(
		span, table, w.tableKeys[last], values, w.tableKinds[last],
	)
	if field == 0 {
		return fmt.Errorf("programlower: could not create TableField")
	}
	w.tableKeys = w.tableKeys[:last]
	w.tableKinds = w.tableKinds[:last]
	w.tableFields = append(w.tableFields, field)
	return nil
}

// Table completes one allocation and releases all private field ranges.
func (w *Writer) Table(span source.Span, mark TableMark, table keyspace.Term) (keyspace.Term, error) {
	if w == nil || mark.owner != w || mark.key < 0 || mark.key > len(w.tableKeys) ||
		mark.kind < 0 || mark.kind > len(w.tableKinds) ||
		mark.field < 0 || mark.field > len(w.tableFields) {
		return 0, fmt.Errorf("programlower: invalid table field mark")
	}
	if len(w.tableKeys) != mark.key || len(w.tableKinds) != mark.kind {
		return 0, fmt.Errorf("programlower: incomplete table fields")
	}
	if w.collector == nil || !w.collector.Flow().Tables().FillTable(table, w.tableFields[mark.field:]) {
		return 0, fmt.Errorf("programlower: could not finalize table allocation")
	}
	w.tableKeys = w.tableKeys[:mark.key]
	w.tableKinds = w.tableKinds[:mark.kind]
	w.tableFields = w.tableFields[:mark.field]
	return table, nil
}

// ScheduleExpression queues one source-dispatched storage expression. The
// dispatcher supplies its already resolved span so no delayed step recovers
// position data from an active source context.
func (w *Writer) ScheduleExpression(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if w == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid storage expression")
	}
	switch node := expr.(type) {
	case *ast.IdentExpr:
		if node == nil {
			return fmt.Errorf("programlower: absent storage identifier")
		}
		w.schedule(step{kind: stepExpression, expr: node, owner: owner, span: span})
		return nil
	case *ast.AttrGetExpr:
		if node == nil {
			return fmt.Errorf("programlower: absent storage attribute")
		}
		w.schedule(step{kind: stepExpression, expr: node, owner: owner, span: span})
		return nil
	default:
		return fmt.Errorf("programlower: expression %T is not storage-owned", expr)
	}
}

// ScheduleTarget queues one assignment-target expression. It never creates a
// Read; the resulting Cell or Lens is retained only as an address.
func (w *Writer) ScheduleTarget(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if w == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid assignment target")
	}
	switch node := expr.(type) {
	case *ast.IdentExpr:
		if node == nil {
			return fmt.Errorf("programlower: absent assignment identifier target")
		}
		w.schedule(step{kind: stepTarget, expr: node, owner: owner, span: span})
		return nil
	case *ast.AttrGetExpr:
		if node == nil {
			return fmt.Errorf("programlower: absent assignment attribute target")
		}
		w.schedule(step{kind: stepTarget, expr: node, owner: owner, span: span})
		return nil
	default:
		return fmt.Errorf("programlower: expression %T is not an assignment target", expr)
	}
}

// ScheduleAssignment queues exact left-to-right target address evaluation followed
// by one ordinary Values route for the right-hand expression list.
func (w *Writer) ScheduleAssignment(assign *ast.AssignStmt, owner keyspace.Term, span source.Span) error {
	if w == nil || assign == nil || len(assign.Lhs) == 0 || owner == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid assignment")
	}
	targetSpans := make([]source.Span, len(assign.Lhs))
	for index, target := range assign.Lhs {
		switch node := target.(type) {
		case *ast.IdentExpr:
			if node == nil {
				return fmt.Errorf("programlower: absent assignment identifier target")
			}
		case *ast.AttrGetExpr:
			if node == nil {
				return fmt.Errorf("programlower: absent assignment attribute target")
			}
		default:
			return fmt.Errorf("programlower: expression %T is not an assignment target", target)
		}
		targetSpans[index] = w.span(target)
	}
	w.schedule(step{
		kind:        stepTargets,
		assign:      assign,
		owner:       owner,
		span:        span,
		targetSpans: targetSpans,
		mark:        w.TargetMark(),
	})
	return nil
}

// Run completes exactly one storage-private continuation. Expression children
// are explicitly handed to source's expression inbox; Values is a direct
// dependency because it owns exact Lua list adjustment.
func (w *Writer) Run() error {
	if w == nil || w.stack == nil || len(w.steps) == 0 {
		return fmt.Errorf("programlower: missing storage continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]
	switch current.kind {
	case stepExpression:
		return w.runExpression(current)
	case stepTarget:
		return w.runTarget(current)
	case stepFinishLensBase:
		return w.finishLensBase(current)
	case stepFinishLens:
		return w.finishLens(current)
	case stepTargets:
		return w.runTargets(current)
	case stepAppendTarget:
		term, _ := w.stack.Result()
		return w.RememberTarget(current.span, term)
	case stepFinishAssign:
		values, _ := w.stack.Result()
		term, err := w.Assign(current.span, current.owner, current.mark, values, current.assign)
		if err != nil {
			return err
		}
		if err := w.lexical.Append(term); err != nil {
			return err
		}
		w.stack.SetResult(term, false)
		return nil
	default:
		return fmt.Errorf("programlower: invalid storage continuation %d", current.kind)
	}
}

func (w *Writer) runExpression(current step) error {
	switch expr := current.expr.(type) {
	case *ast.IdentExpr:
		if expr == nil {
			return fmt.Errorf("programlower: absent storage identifier")
		}
		term, err := w.resolveIdentifier(expr, current.owner, current.span, true)
		if err != nil {
			return err
		}
		w.stack.SetResult(term, false)
		return nil
	case *ast.AttrGetExpr:
		if expr == nil {
			return fmt.Errorf("programlower: absent storage attribute")
		}
		return w.beginLens(expr, current.owner, current.span, true)
	default:
		return fmt.Errorf("programlower: invalid storage expression %T", current.expr)
	}
}

func (w *Writer) runTarget(current step) error {
	switch expr := current.expr.(type) {
	case *ast.IdentExpr:
		if expr == nil {
			return fmt.Errorf("programlower: absent assignment identifier target")
		}
		term, err := w.resolveIdentifier(expr, current.owner, current.span, false)
		if err != nil {
			return err
		}
		w.stack.SetResult(term, false)
		return nil
	case *ast.AttrGetExpr:
		if expr == nil {
			return fmt.Errorf("programlower: absent assignment attribute target")
		}
		return w.beginLens(expr, current.owner, current.span, false)
	default:
		return fmt.Errorf("programlower: invalid assignment target %T", current.expr)
	}
}

func (w *Writer) resolveIdentifier(expr *ast.IdentExpr, owner keyspace.Term, span source.Span, read bool) (keyspace.Term, error) {
	cell, err := w.ResolveCell(expr)
	if err != nil || !read {
		return cell, err
	}
	if w.static == nil {
		return 0, fmt.Errorf("programlower: missing static authority")
	}
	if w.binding.IsImplicitGlobalUse(expr) && w.static.StaticDepth() == 0 {
		return w.implicitRead(span, owner, cell)
	}
	return w.Read(span, owner, cell)
}

func (w *Writer) beginLens(attr *ast.AttrGetExpr, owner keyspace.Term, span source.Span, read bool) error {
	if w == nil || w.expressions == nil || attr == nil || attr.Object == nil || attr.Key == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid attribute access")
	}
	w.schedule(step{kind: stepFinishLensBase, attr: attr, owner: owner, span: span, keySpan: w.span(attr.Key), read: read})
	return w.expressions.Push(attr.Object, owner, w.span(attr.Object))
}

func (w *Writer) finishLensBase(current step) error {
	base, _ := w.stack.Result()
	if base == 0 || current.attr == nil || current.attr.Key == nil || current.span.File == "" || current.keySpan.File == "" {
		return fmt.Errorf("programlower: missing Lens base")
	}
	switch current.attr.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := current.attr.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf("programlower: dot attribute key is not a string literal")
		}
		lens, err := w.DotLens(current.span, current.owner, base, current.keySpan, name.Value)
		if err != nil {
			return err
		}
		return w.finishLensRead(lens, current)
	case ast.AttrKeyIndex:
		if w.expressions == nil || current.attr.Key == nil || current.owner == 0 {
			return fmt.Errorf("programlower: invalid indexed Lens continuation")
		}
		w.schedule(step{kind: stepFinishLens, attr: current.attr, owner: current.owner, span: current.span, keySpan: current.keySpan, read: current.read, base: base})
		return w.expressions.Push(current.attr.Key, current.owner, current.keySpan)
	default:
		return fmt.Errorf("programlower: unsupported attribute key syntax %d", current.attr.KeySyntax)
	}
}

func (w *Writer) finishLens(current step) error {
	key, _ := w.stack.Result()
	if key == 0 || current.base == 0 || current.attr == nil {
		return fmt.Errorf("programlower: missing Lens key")
	}
	lens, err := w.IndexLens(current.span, current.owner, current.base, key, current.attr.Key)
	if err != nil {
		return err
	}
	return w.finishLensRead(lens, current)
}

func (w *Writer) finishLensRead(lens keyspace.Term, current step) error {
	if !current.read {
		w.stack.SetResult(lens, false)
		return nil
	}
	term, err := w.Read(current.span, current.owner, lens)
	if err != nil {
		return err
	}
	w.stack.SetResult(term, false)
	return nil
}

func (w *Writer) runTargets(current step) error {
	if current.assign == nil || current.index < 0 || current.index > len(current.assign.Lhs) {
		return fmt.Errorf("programlower: invalid assignment-target cursor")
	}
	if current.index == len(current.assign.Lhs) {
		if w.values == nil {
			return fmt.Errorf("programlower: missing Values authority")
		}
		w.schedule(step{
			kind: stepFinishAssign, assign: current.assign, owner: current.owner,
			span: current.span, mark: current.mark,
		})
		return w.values.ScheduleValues(current.assign.Rhs, current.owner, current.span)
	}
	expr := current.assign.Lhs[current.index]
	if expr == nil {
		return fmt.Errorf("programlower: absent assignment target")
	}
	current.index++
	span := current.targetSpans[current.index-1]
	w.schedule(step{kind: stepTargets, assign: current.assign, index: current.index, owner: current.owner, span: current.span, targetSpans: current.targetSpans, mark: current.mark})
	w.schedule(step{kind: stepAppendTarget, expr: expr, owner: current.owner, span: span})
	return w.scheduleTargetChild(expr, current.owner, span)
}

func (w *Writer) scheduleTargetChild(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	return w.ScheduleTarget(expr, owner, span)
}

func (w *Writer) schedule(current step) {
	w.steps = append(w.steps, current)
	w.stack.Push(phase.Store)
}

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: w.sourceName}
	}
	span, ok := sourcecoord.Build(w.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return sourcecoord.Invalid(w.sourceName)
	}
	return span
}

func keyKind(expr ast.Expr) flowkind.FieldKind {
	switch expr := expr.(type) {
	case *ast.NilExpr:
		if expr == nil {
			return flowkind.FieldKey
		}
		return flowkind.FieldExact
	case *ast.TrueExpr:
		if expr == nil {
			return flowkind.FieldKey
		}
		return flowkind.FieldExact
	case *ast.FalseExpr:
		if expr == nil {
			return flowkind.FieldKey
		}
		return flowkind.FieldExact
	case *ast.NumberExpr:
		if expr == nil {
			return flowkind.FieldKey
		}
		return flowkind.FieldExact
	case *ast.StringExpr:
		if expr == nil {
			return flowkind.FieldKey
		}
		return flowkind.FieldExact
	case *ast.UnaryMinusOpExpr:
		if expr == nil {
			return flowkind.FieldKey
		}
		if number, numeric := expr.Expr.(*ast.NumberExpr); numeric && number != nil {
			return flowkind.FieldExact
		}
	}
	return flowkind.FieldKey
}

// Clean reports whether every storage-owned continuation and scratch range
// completed before Program sealing.
func (w *Writer) Clean() bool {
	return w != nil && len(w.steps) == 0 && len(w.targets) == 0 &&
		len(w.targetSpans) == 0 && len(w.tableKeys) == 0 &&
		len(w.tableKinds) == 0 && len(w.tableFields) == 0
}

type stepKind uint8

const (
	stepExpression stepKind = iota + 1
	stepTarget
	stepFinishLensBase
	stepFinishLens
	stepTargets
	stepAppendTarget
	stepFinishAssign
)

type step struct {
	kind        stepKind
	expr        ast.Expr
	attr        *ast.AttrGetExpr
	assign      *ast.AssignStmt
	owner       keyspace.Term
	span        source.Span
	keySpan     source.Span
	read        bool
	index       int
	targetSpans []source.Span
	mark        TargetMark
	base        keyspace.Term
}
