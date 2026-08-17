package storage

import (
	"fmt"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DeclareTable reserves allocation before its constructor fields evaluate.
func (w *Writer) DeclareTable(span source.Span, owner keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("lualower: missing storage authority")
	}
	term := w.collector.DeclareTable(span, owner)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not declare table allocation")
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
		return fmt.Errorf("lualower: missing storage authority")
	}
	key := w.collector.List(span, owner, ordinal)
	if key == 0 {
		return fmt.Errorf("lualower: could not create table list key")
	}
	w.tableKeys = append(w.tableKeys, key)
	w.tableKinds = append(w.tableKinds, flowkind.FieldList)
	return nil
}

// NameField records one parser-authored name key in source order.
func (w *Writer) NameField(span source.Span, owner keyspace.Term, value string) error {
	if w == nil || w.collector == nil {
		return fmt.Errorf("lualower: missing storage authority")
	}
	key := w.collector.Name(span, owner, value)
	if key == 0 {
		return fmt.Errorf("lualower: could not create table field Name")
	}
	w.tableKeys = append(w.tableKeys, key)
	w.tableKinds = append(w.tableKinds, flowkind.FieldName)
	return nil
}

// KeyField retains one evaluated bracket key and its exact/dynamic policy.
func (w *Writer) KeyField(key keyspace.Term, source ast.Expr) error {
	if w == nil || key == 0 || source == nil {
		return fmt.Errorf("lualower: invalid table field key")
	}
	w.tableKeys = append(w.tableKeys, key)
	w.tableKinds = append(w.tableKinds, keyKind(source))
	return nil
}

// Field completes one field against an already declared allocation.
func (w *Writer) Field(span source.Span, table, values keyspace.Term) error {
	if w == nil || len(w.tableKeys) == 0 || len(w.tableKinds) == 0 {
		return fmt.Errorf("lualower: missing table field key")
	}
	last := len(w.tableKeys) - 1
	if w.collector == nil {
		return fmt.Errorf("lualower: missing storage authority")
	}
	field := w.collector.TableField(
		span, table, w.tableKeys[last], values, w.tableKinds[last],
	)
	if field == 0 {
		return fmt.Errorf("lualower: could not create TableField")
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
		return 0, fmt.Errorf("lualower: invalid table field mark")
	}
	if len(w.tableKeys) != mark.key || len(w.tableKinds) != mark.kind {
		return 0, fmt.Errorf("lualower: incomplete table fields")
	}
	if w.collector == nil || !w.collector.FillTable(table, w.tableFields[mark.field:]) {
		return 0, fmt.Errorf("lualower: could not finalize table allocation")
	}
	w.tableKeys = w.tableKeys[:mark.key]
	w.tableKinds = w.tableKinds[:mark.kind]
	w.tableFields = w.tableFields[:mark.field]
	return table, nil
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
