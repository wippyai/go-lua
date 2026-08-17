package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticOperandQueriesRejectAbsentOrInvalidOperands(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-static-operand-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	for _, term := range []uint64{0, ^uint64(0)} {
		if operand, ok := published.StaticOperandAt(keyspace.Term(term)); ok || operand.ID().Available() {
			t.Fatalf("StaticOperandAt(%d) = %#v/%v; want unavailable", term, operand, ok)
		}
	}
	if path, cursor, ok := published.StaticFrontier(0); ok || path.Available() || cursor != 0 {
		t.Fatalf("StaticFrontier(0) = %x/%d/%v; want unavailable zero frontier", path, cursor, ok)
	}
}
