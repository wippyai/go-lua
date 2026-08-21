// Package storage owns canonical table-constructor scheduling and field
// construction. It reserves the allocation before its children run, while
// store remains the sole authority for field keys and commits.
package storage

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TableWriter owns one unfinished table-constructor walk. It has no builder of its
// own: lexical owns the current Body, eval owns Values adjustment, and store
// owns table allocation, key policy, and field commits.
type TableWriter struct {
	phases      *continuation.Stack
	expressions *continuation.Expressions
	values      *eval.Values
	store       *Writer
	sourceName  string
	steps       []tableStep
}

type tableStepKind uint8

const (
	tableFieldsStep tableStepKind = iota + 1
	tableFinishKeyStep
	tableFinishValueStep
)

// tableStep is table-private continuation payload. continuation.Stack carries the one
// global continuation token; this payload is never a second execution path.
type tableStep struct {
	kind tableStepKind

	table     *ast.TableExpr
	tableTerm keyspace.Term
	field     *ast.Field
	value     ast.Expr
	key       ast.Expr
	host      keyspace.Term
	span      source.Span
	valueSpan source.Span
	fieldSpan source.Span
	index     int
	ordinal   int
	mark      TableMark
	allowOpen bool
}

// NewTable binds table scheduling to the existing lowering authorities.
func NewTable(
	phases *continuation.Stack,
	expressions *continuation.Expressions,
	values *eval.Values,
	storage *Writer,
	sourceName string,
) TableWriter {
	return TableWriter{
		phases:      phases,
		expressions: expressions,
		values:      values,
		store:       storage,
		sourceName:  sourceName,
	}
}

// Schedule reserves the table allocation before any key or value expression
// runs, then schedules the first field. Expression evaluation crosses the
// narrow expression inbox; table continuation remains private to this writer.
func (w *TableWriter) Schedule(table *ast.TableExpr, host keyspace.Term, span source.Span) error {
	if w == nil || w.phases == nil || w.values == nil || w.store == nil ||
		w.expressions == nil || table == nil || host == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid table constructor")
	}
	allocation, err := w.store.DeclareTable(span, host)
	if err != nil {
		return err
	}
	return w.schedule(tableStep{
		kind:      tableFieldsStep,
		table:     table,
		tableTerm: allocation,
		host:      host,
		span:      span,
		mark:      w.store.TableMark(),
	})
}

// Run completes one table-private continuation after the parent has selected
// its Table token from continuation.Stack. It publishes the completed constructor in
// continuation.Result, never through a callback or a shadow result channel.
func (w *TableWriter) Run() error {
	if w == nil || w.phases == nil || w.values == nil || w.store == nil || w.expressions == nil {
		return fmt.Errorf("lualower: missing table authority")
	}
	if len(w.steps) == 0 {
		return fmt.Errorf("lualower: missing table continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]
	if current.host == 0 || current.span.File == "" {
		return fmt.Errorf("lualower: table continuation lost source identity")
	}

	switch current.kind {
	case tableFieldsStep:
		return w.runFields(current)
	case tableFinishKeyStep:
		if current.key == nil || current.valueSpan.File == "" {
			return fmt.Errorf("lualower: invalid table key continuation")
		}
		key, _ := w.phases.Result()
		if err := w.store.KeyField(key, current.key); err != nil {
			return err
		}
		if err := w.schedule(tableStep{
			kind:      tableFinishValueStep,
			tableTerm: current.tableTerm,
			field:     current.field,
			value:     current.value,
			host:      current.host,
			span:      current.valueSpan,
			fieldSpan: current.fieldSpan,
			allowOpen: current.allowOpen,
		}); err != nil {
			return err
		}
		return w.expression(current.value, current.host, current.valueSpan)
	case tableFinishValueStep:
		value, open := w.phases.Result()
		values, err := w.values.Field(
			current.span,
			current.host,
			value,
			open,
			current.allowOpen,
		)
		if err != nil {
			return err
		}
		if current.field == nil {
			return fmt.Errorf("lualower: invalid table field continuation")
		}
		if current.fieldSpan.File == "" {
			return fmt.Errorf("lualower: table field continuation lost source span")
		}
		return w.store.Field(current.fieldSpan, current.tableTerm, values)
	default:
		return fmt.Errorf("lualower: unknown table continuation %d", current.kind)
	}
}

func (w *TableWriter) runFields(current tableStep) error {
	if current.table == nil {
		return fmt.Errorf("lualower: invalid table continuation")
	}
	if current.index == len(current.table.Fields) {
		result, err := w.store.Table(
			current.span,
			current.mark,
			current.tableTerm,
		)
		if err != nil {
			return err
		}
		w.phases.SetResult(result, false)
		return nil
	}
	if current.index < 0 || current.index > len(current.table.Fields) {
		return fmt.Errorf("lualower: invalid table-field cursor")
	}
	index := current.index
	field := current.table.Fields[index]
	if field == nil || field.Value == nil {
		return fmt.Errorf("lualower: absent table field value %d", index)
	}
	next := current
	next.index++
	allowOpen := field.Key == nil && index == len(current.table.Fields)-1
	fieldSpan := w.span(field)
	valueSpan, valueOK := continuation.ExpressionSpan(field.Value, current.span.File)
	if fieldSpan.File == "" || !valueOK {
		return fmt.Errorf("lualower: unresolved table field span %d", index)
	}

	if field.Key == nil {
		ordinal := current.ordinal + 1
		if err := w.store.ListField(
			fieldSpan,
			current.host,
			int64(ordinal),
		); err != nil {
			return fmt.Errorf("lualower: could not create table list key %d", index)
		}
		next.ordinal = ordinal
		if err := w.schedule(next, tableStep{
			kind:      tableFinishValueStep,
			tableTerm: current.tableTerm,
			field:     field,
			value:     field.Value,
			host:      current.host,
			span:      valueSpan,
			fieldSpan: fieldSpan,
			allowOpen: allowOpen,
		}); err != nil {
			return err
		}
		return w.expression(field.Value, current.host, valueSpan)
	}
	switch field.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := field.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf("lualower: table field %d dot key is not a string literal", index)
		}
		nameSpan := w.span(name)
		if nameSpan.File == "" {
			return fmt.Errorf("lualower: unresolved table field Name span %d", index)
		}
		if err := w.store.NameField(nameSpan, current.host, name.Value); err != nil {
			return fmt.Errorf("lualower: could not create table field Name %d", index)
		}
		if err := w.schedule(next, tableStep{
			kind:      tableFinishValueStep,
			tableTerm: current.tableTerm,
			field:     field,
			value:     field.Value,
			host:      current.host,
			span:      valueSpan,
			fieldSpan: fieldSpan,
		}); err != nil {
			return err
		}
		return w.expression(field.Value, current.host, valueSpan)
	case ast.AttrKeyIndex:
		keySpan, keyOK := continuation.ExpressionSpan(field.Key, current.span.File)
		if !keyOK {
			return fmt.Errorf("lualower: unresolved table field key span %d", index)
		}
		if err := w.schedule(next, tableStep{
			kind:      tableFinishKeyStep,
			tableTerm: current.tableTerm,
			field:     field,
			value:     field.Value,
			key:       field.Key,
			host:      current.host,
			span:      keySpan,
			valueSpan: valueSpan,
			fieldSpan: fieldSpan,
		}); err != nil {
			return err
		}
		return w.expression(field.Key, current.host, keySpan)
	default:
		return fmt.Errorf(
			"lualower: unsupported table field %d key syntax %d",
			index,
			field.KeySyntax,
		)
	}
}

func (w *TableWriter) schedule(next ...tableStep) error {
	for _, nextStep := range next {
		if nextStep.kind == 0 || nextStep.host == 0 || nextStep.span.File == "" {
			return fmt.Errorf("lualower: invalid table continuation identity")
		}
	}
	for _, nextStep := range next {
		w.steps = append(w.steps, nextStep)
		w.phases.Push(continuation.Table)
	}
	return nil
}

func (w *TableWriter) expression(expr ast.Expr, host keyspace.Term, span source.Span) error {
	if expr == nil || host == 0 || span.File == "" {
		return fmt.Errorf("lualower: absent table expression")
	}
	return w.expressions.Push(expr, host, span)
}

func (w *TableWriter) span(holder ast.PositionHolder) source.Span {
	return coord.Span(w.sourceName, holder)
}

// Clean reports whether this writer has no unconsumed private continuation.
// The shared phase stack is owned and checked by the parent after all verticals
// have completed.
func (w *TableWriter) Clean() bool {
	return w != nil && len(w.steps) == 0
}
