package continuation

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestBodiesKeepTypedQueuesAndTokensPaired(t *testing.T) {
	stack := &Stack{}
	queue := NewBodies(stack)
	span := programsource.Span{File: "body.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if err := queue.PushStatements(nil, 0, body, span); err != nil {
		t.Fatal(err)
	}
	if request, err := queue.PopStatements(); err != nil || request.Body != body {
		t.Fatalf("PopStatements = %#v/%v", request, err)
	}
	if queue.Clean() != true {
		t.Fatal("Bodies retained a consumed statement")
	}
}
