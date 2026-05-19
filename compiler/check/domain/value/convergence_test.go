package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestUnsafePrecisionDrop_DetectsNestedUnionMemberDrop(t *testing.T) {
	withPending := typ.NewUnion(
		typ.LiteralString("pass"),
		typ.LiteralString("pending"),
		typ.LiteralString("fail"),
		typ.LiteralString("skip"),
	)
	withoutPending := typ.NewUnion(
		typ.LiteralString("pass"),
		typ.LiteralString("fail"),
		typ.LiteralString("skip"),
	)
	prev := typ.NewRecord().Field("status", withPending).Build()
	next := typ.NewRecord().Field("status", withoutPending).Build()
	if !UnsafePrecisionDrop(prev, next) {
		t.Fatalf("expected nested union member drop to be unsafe: prev=%v next=%v", prev, next)
	}
}
