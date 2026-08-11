package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/lower/internal/collector"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/source"
)

func TestScheduleVarargRequiresActiveBodyOwner(t *testing.T) {
	const name = "lexical-vararg-owner.lua"
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	construction := collector.New(name, 0, bind.GlobalCensus{})
	scopes := New(new(phase.Stack), construction, &bind.Result{}, name, nil, nil, nil)
	entry, err := scopes.Entry(span)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := scopes.EnterBlock(span)
	if err != nil {
		t.Fatal(err)
	}
	expr := &ast.Comma3Expr{}
	if err := scopes.ScheduleVararg(expr, entry, span); err == nil {
		t.Fatalf("ScheduleVararg accepted foreign owner %v while active owner is %v", entry, nested)
	}
	if err := scopes.ScheduleVararg(expr, nested, span); err != nil {
		t.Fatalf("ScheduleVararg rejected active owner %v: %v", nested, err)
	}
	cell, err := scopes.Vararg(span)
	if err != nil || cell == 0 {
		t.Fatalf("Vararg = Cell %v/%v, want cached chunk Cell", cell, err)
	}
}
