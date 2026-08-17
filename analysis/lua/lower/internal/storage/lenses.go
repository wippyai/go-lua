package storage

import (
	"fmt"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DotLens records a parser-authored exact field Lens.
func (w *Writer) DotLens(
	span source.Span,
	owner keyspace.Term,
	base keyspace.Term,
	nameSpan source.Span,
	name string,
) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("lualower: missing storage authority")
	}
	key := w.collector.Name(nameSpan, owner, name)
	if key == 0 {
		return 0, fmt.Errorf("lualower: could not create attribute Name")
	}
	lens := w.collector.LensExact(
		span, owner, base, key, flowkind.FieldName,
	)
	if lens == 0 {
		return 0, fmt.Errorf("lualower: could not create Lens")
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
		return 0, fmt.Errorf("lualower: missing storage authority")
	}
	var term keyspace.Term
	if keyKind(source) == flowkind.FieldExact {
		term = w.collector.LensExact(
			span, owner, base, key, flowkind.FieldExact,
		)
	} else {
		term = w.collector.LensKey(span, owner, base, key)
	}
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create Lens")
	}
	return term, nil
}

func (w *Writer) beginLens(attr *ast.AttrGetExpr, owner keyspace.Term, span source.Span, read bool) error {
	if w == nil || w.expressions == nil || attr == nil || attr.Object == nil || attr.Key == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid attribute access")
	}
	w.schedule(step{kind: stepFinishLensBase, attr: attr, owner: owner, span: span, keySpan: w.span(attr.Key), read: read})
	return w.expressions.Push(attr.Object, owner, w.span(attr.Object))
}

func (w *Writer) finishLensBase(current step) error {
	base, _ := w.stack.Result()
	if base == 0 || current.attr == nil || current.attr.Key == nil || current.span.File == "" || current.keySpan.File == "" {
		return fmt.Errorf("lualower: missing Lens base")
	}
	switch current.attr.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := current.attr.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf("lualower: dot attribute key is not a string literal")
		}
		lens, err := w.DotLens(current.span, current.owner, base, current.keySpan, name.Value)
		if err != nil {
			return err
		}
		return w.finishLensRead(lens, current)
	case ast.AttrKeyIndex:
		if w.expressions == nil || current.attr.Key == nil || current.owner == 0 {
			return fmt.Errorf("lualower: invalid indexed Lens continuation")
		}
		w.schedule(step{kind: stepFinishLens, attr: current.attr, owner: current.owner, span: current.span, keySpan: current.keySpan, read: current.read, base: base})
		return w.expressions.Push(current.attr.Key, current.owner, current.keySpan)
	default:
		return fmt.Errorf("lualower: unsupported attribute key syntax %d", current.attr.KeySyntax)
	}
}

func (w *Writer) finishLens(current step) error {
	key, _ := w.stack.Result()
	if key == 0 || current.base == 0 || current.attr == nil {
		return fmt.Errorf("lualower: missing Lens key")
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
