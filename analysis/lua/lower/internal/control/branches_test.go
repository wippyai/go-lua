package control

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestBranchBodyAdmissionRequiresActiveHost(t *testing.T) {
	w := Writer{}
	if err := w.openBody(nil, source.Span{File: "control.lua"}, step{}); err == nil {
		t.Fatal("openBody accepted a host without an active lexical Body")
	}
}

func TestBranchCompletionRejectsMissingIfState(t *testing.T) {
	w := Writer{}
	if err := w.finishIfCondition(step{host: 1}); err == nil {
		t.Fatal("finishIfCondition accepted a missing if statement")
	}
}
