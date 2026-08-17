package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestStaticSignatureRowsRequireExplicitOneShotFills(t *testing.T) {
	rows := &staticRows{}
	term := staticTestTerm(keyspace.FamilyTypeFunction, 1)
	if err := rows.TypeFunctionDeclare(term, 0); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeFunctionGenerics(term, nil); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeFunctionParametersRaw(term, nil); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeFunctionVariadic(term, 0, source.Coordinate{}); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeFunctionReturns(term, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeFunctionReturns(term, true, nil); err == nil {
		t.Fatal("TypeFunctionReturns accepted a duplicate fill")
	}
}
