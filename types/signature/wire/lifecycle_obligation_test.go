package wire

import (
	"testing"

	"github.com/wippyai/go-lua/domain/typestate"
)

func testLifecycleObligation(t *testing.T, states ...typestate.State) typestate.Obligation {
	t.Helper()
	obligation, ok := typestate.NewObligation(states...)
	if !ok {
		t.Fatal("NewObligation rejected valid states")
	}
	return obligation
}

func canonicalLifecycleObligation(states ...typestate.State) typestate.Obligation {
	obligation, ok := typestate.NewObligation(states...)
	if !ok {
		panic("invalid lifecycle obligation fixture")
	}
	return obligation
}
