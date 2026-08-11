package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	programlower "github.com/wippyai/go-lua/program/lower"
)

// lowerSource is the sole source-gate entry point used by lower-package tests.
func lowerSource(source string) (*program.Program, error) {
	return programlower.Lower(programlower.Source{Name: "fixture.lua", Text: []byte(source)})
}

func parseBindLower(t *testing.T, source string) *program.Program {
	t.Helper()
	lowered, err := lowerSource(source)
	if err != nil {
		t.Fatal(err)
	}
	return lowered
}

func valuesTail(t *testing.T, p *program.Program, values keyspace.Term) keyspace.Term {
	t.Helper()
	_, tail, ok := p.Flow().Authored().Values().Get(values)
	if !ok {
		t.Fatalf("%v is not Values", values)
	}
	return tail
}

func valueAt(t *testing.T, p *program.Program, values keyspace.Term, index int) keyspace.Term {
	t.Helper()
	value, ok := p.Flow().Authored().Values().Member(values, index)
	if !ok {
		t.Fatalf("Values(%v) has no value at %d", values, index)
	}
	return value
}

func boundCell(t *testing.T, p *program.Program, bind keyspace.Term, index int) keyspace.Term {
	t.Helper()
	cell, ok := p.Source().Binds().At(bind, index)
	if !ok {
		t.Fatalf("Bind(%v) has no cell at %d", bind, index)
	}
	return cell
}

func functionCapture(t *testing.T, p *program.Program, function keyspace.Term, index int) (keyspace.Term, keyspace.Term) {
	t.Helper()
	inner, outer, ok := p.Flow().Authored().Functions().CaptureAt(function, index)
	if !ok {
		t.Fatalf("Function(%v) has no capture at %d", function, index)
	}
	return inner, outer
}
