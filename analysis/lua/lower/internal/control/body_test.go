package control

import (
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestControlBodySchedulingPairsPayloadQueuesWithOwnerTokens(t *testing.T) {
	stack := &continuation.Stack{}
	writer := Writer{phases: stack, bodies: continuation.NewBodies(stack)}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	span := programsource.Span{File: "control.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	if err := writer.scheduleBody(nil, span, step{body: body}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.bodies.PopPrepare(); err != nil {
		t.Fatalf("PopPrepare: %v", err)
	}
}
