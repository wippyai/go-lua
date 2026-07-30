// Package store owns mutation-target, key-policy, and table-field scratch.
package store

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program"
)

// Access is the one authority for evaluated storage targets and table fields.
type Access struct {
	builder *program.Builder

	targets []program.Term

	tableKeys   []program.Term
	tableValues []program.Term
	tableKinds  []program.FieldKind
}

// New creates the access authority for one unfinished Program.
func New(builder *program.Builder) Access {
	return Access{builder: builder}
}

// TargetMark identifies the start of one delayed assignment target group.
func (a *Access) TargetMark() int {
	return len(a.targets)
}

// RememberTarget retains one evaluated target in source order.
func (a *Access) RememberTarget(target program.Term) {
	a.targets = append(a.targets, target)
}

// Assign completes and releases one delayed target group.
func (a *Access) Assign(
	span program.Span,
	owner program.Term,
	mark int,
	values program.Term,
) (program.Term, error) {
	if mark < 0 || mark > len(a.targets) {
		return 0, fmt.Errorf("programlower: invalid assignment target mark")
	}
	term := a.builder.Assign(span, owner, a.targets[mark:], values)
	a.targets = a.targets[:mark]
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower assignment")
	}
	return term, nil
}

// DotLens records one parser-owned name and its exact field Lens atomically.
func (a *Access) DotLens(
	span program.Span,
	owner program.Term,
	base program.Term,
	nameSpan program.Span,
	name string,
) (program.Term, error) {
	key := a.builder.Name(nameSpan, owner, name)
	if key == 0 {
		return 0, fmt.Errorf("programlower: could not create attribute Name")
	}
	lens := a.builder.LensExact(span, owner, base, key, program.FieldName)
	if lens == 0 {
		return 0, fmt.Errorf("programlower: could not create Lens")
	}
	return lens, nil
}

// IndexLens classifies and records one exact or dynamic bracket field target.
func (a *Access) IndexLens(
	span program.Span,
	owner program.Term,
	base program.Term,
	key program.Term,
	source ast.Expr,
) (program.Term, error) {
	kind := keyKind(source)
	var term program.Term
	if kind == program.FieldExact {
		term = a.builder.LensExact(span, owner, base, key, kind)
	} else {
		term = a.builder.LensKey(span, owner, base, key)
	}
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create Lens")
	}
	return term, nil
}

// TableMark identifies the three private ranges of one table constructor.
func (a *Access) TableMark() (int, int, int) {
	return len(a.tableKeys), len(a.tableValues), len(a.tableKinds)
}

// ListField records one generated list key in source order.
func (a *Access) ListField(span program.Span, owner program.Term, ordinal int64) error {
	key := a.builder.List(span, owner, ordinal)
	if key == 0 {
		return fmt.Errorf("programlower: could not create table list key")
	}
	a.tableKeys = append(a.tableKeys, key)
	a.tableKinds = append(a.tableKinds, program.FieldList)
	return nil
}

// NameField records one parser-owned name key in source order.
func (a *Access) NameField(span program.Span, owner program.Term, value string) error {
	key := a.builder.Name(span, owner, value)
	if key == 0 {
		return fmt.Errorf("programlower: could not create table field Name")
	}
	a.tableKeys = append(a.tableKeys, key)
	a.tableKinds = append(a.tableKinds, program.FieldName)
	return nil
}

// KeyField retains one evaluated bracket key and its exact/dynamic policy.
func (a *Access) KeyField(key program.Term, source ast.Expr) {
	a.tableKeys = append(a.tableKeys, key)
	a.tableKinds = append(a.tableKinds, keyKind(source))
}

// FieldValues retains one completed field pack in source order.
func (a *Access) FieldValues(values program.Term) {
	a.tableValues = append(a.tableValues, values)
}

// Table completes one allocation and releases all of its private field ranges.
func (a *Access) Table(
	span program.Span,
	owner program.Term,
	keyMark int,
	valueMark int,
	kindMark int,
) (program.Term, error) {
	if keyMark < 0 || keyMark > len(a.tableKeys) ||
		valueMark < 0 || valueMark > len(a.tableValues) ||
		kindMark < 0 || kindMark > len(a.tableKinds) {
		return 0, fmt.Errorf("programlower: invalid table field mark")
	}
	keys := a.tableKeys[keyMark:]
	values := a.tableValues[valueMark:]
	kinds := a.tableKinds[kindMark:]
	if len(keys) != len(values) || len(keys) != len(kinds) {
		return 0, fmt.Errorf("programlower: incomplete table fields")
	}
	term := a.builder.Table(span, owner, keys, values, kinds)
	a.tableKeys = a.tableKeys[:keyMark]
	a.tableValues = a.tableValues[:valueMark]
	a.tableKinds = a.tableKinds[:kindMark]
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create table allocation")
	}
	return term, nil
}

func keyKind(expr ast.Expr) program.FieldKind {
	switch expr.(type) {
	case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		return program.FieldExact
	default:
		return program.FieldKey
	}
}

// Clean reports whether every access and table scratch range completed.
func (a *Access) Clean() bool {
	return len(a.targets) == 0 &&
		len(a.tableKeys) == 0 &&
		len(a.tableValues) == 0 &&
		len(a.tableKinds) == 0
}
