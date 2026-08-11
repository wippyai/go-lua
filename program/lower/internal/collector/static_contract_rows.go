package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
)

// FunctionContract and CallContract are dense sidecars anchored at Flow
// identities. Their placeholder declarations make the denominator explicit
// before Flow supplies the authored fills.
func (rows *staticRows) FunctionContractDeclare(term keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyFunction, len(rows.functionContracts)); err != nil {
		return err
	}
	rows.functionContracts = append(rows.functionContracts, staticRawFunctionContract{})
	return nil
}

func (rows *staticRows) FunctionContractPlaceholder(term keyspace.Term) error {
	return rows.FunctionContractDeclare(term)
}

func (rows *staticRows) FunctionContractGenerics(term keyspace.Term, params []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyFunction, len(rows.functionContracts))
	if err != nil {
		return err
	}
	row := &rows.functionContracts[index]
	if row.typeParamsSet {
		return errors.New("program/lower/collector: FunctionContract generics filled twice")
	}
	row.typeParams, row.typeParamsSet = append([]keyspace.Term(nil), params...), true
	return nil
}

func (rows *staticRows) FunctionContractReturns(term keyspace.Term, known bool, returns []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyFunction, len(rows.functionContracts))
	if err != nil {
		return err
	}
	row := &rows.functionContracts[index]
	if row.returnsSet || (!known && len(returns) != 0) {
		return errors.New("program/lower/collector: invalid FunctionContract returns fill")
	}
	row.returnsKnown, row.returns, row.returnsSet = known, append([]keyspace.Term(nil), returns...), true
	return nil
}

func (rows *staticRows) CallContractPlaceholder(term keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyCall, len(rows.callContracts)); err != nil {
		return err
	}
	rows.callContracts = append(rows.callContracts, staticRawCallContract{})
	return nil
}

func (rows *staticRows) CallContractArguments(term keyspace.Term, arguments []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyCall, len(rows.callContracts))
	if err != nil {
		return err
	}
	if rows.callContracts[index].filled {
		return errors.New("program/lower/collector: CallContract filled twice")
	}
	rows.callContracts[index].arguments = append([]keyspace.Term(nil), arguments...)
	rows.callContracts[index].filled = true
	return nil
}
