package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
)

func (rows *staticRows) TypeOf(term, scope, operand keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeOf, len(rows.typeOf)); err != nil {
		return err
	}
	if scope == 0 || operand == 0 {
		return errors.New("program/lower/collector: incomplete TypeOf")
	}
	rows.typeOf = append(rows.typeOf, staticoperators.TypeOf{Scope: scope, Operand: operand})
	return nil
}

func (rows *staticRows) KeyOf(term, inner keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeKeyOf, len(rows.keyOf)); err != nil {
		return err
	}
	if inner == 0 {
		return errors.New("program/lower/collector: missing KeyOf child")
	}
	rows.keyOf = append(rows.keyOf, staticoperators.KeyOf{Inner: inner})
	return nil
}

func (rows *staticRows) IndexAccess(term, object, index keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeIndexAccess, len(rows.indexAccess)); err != nil {
		return err
	}
	if object == 0 || index == 0 {
		return errors.New("program/lower/collector: incomplete IndexAccess")
	}
	rows.indexAccess = append(rows.indexAccess, staticoperators.IndexAccess{Object: object, Index: index})
	return nil
}

func (rows *staticRows) Conditional(term, check, extends, then, otherwise keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeConditional, len(rows.conditional)); err != nil {
		return err
	}
	if check == 0 || extends == 0 || then == 0 || otherwise == 0 {
		return errors.New("program/lower/collector: incomplete Conditional")
	}
	rows.conditional = append(rows.conditional, staticoperators.Conditional{Check: check, Extends: extends, Then: then, Else: otherwise})
	return nil
}
