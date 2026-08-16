package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func canonicalReceipt(view static.View) (static.CommitInput, error) {
	typeOfs := view.Operators().TypeOfs()
	annotations := view.Operands().Annotations()
	publications := view.Publications()
	receipt := static.CommitInput{
		TypeOf:       make([]keyspace.Term, typeOfs.Count()),
		Annotations:  make([]keyspace.Term, annotations.Count()),
		Publications: make([]keyspace.Term, publications.Count()),
	}
	for index := range receipt.TypeOf {
		term, ok := typeOfs.At(index)
		want := keyspace.MakeTerm(keyspace.FamilyTypeOf, uint32(index+1))
		if !ok || term != want {
			return static.CommitInput{}, errors.New("program/flow/staticcheck: TypeOf receipt is not canonical")
		}
		receipt.TypeOf[index] = term
	}
	for index := range receipt.Annotations {
		term, ok := annotations.At(index)
		want := keyspace.MakeTerm(keyspace.FamilyAnnotation, uint32(index+1))
		if !ok || term != want {
			return static.CommitInput{}, errors.New("program/flow/staticcheck: Annotation receipt is not canonical")
		}
		receipt.Annotations[index] = term
	}
	for index := range receipt.Publications {
		term, ok := publications.At(index)
		want := keyspace.MakeTerm(keyspace.FamilyTypePublication, uint32(index+1))
		if !ok || term != want {
			return static.CommitInput{}, errors.New("program/flow/staticcheck: Publication receipt is not canonical")
		}
		receipt.Publications[index] = term
	}
	return receipt, nil
}
