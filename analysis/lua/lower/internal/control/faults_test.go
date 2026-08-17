package control

import (
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	"github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestControlFaultDeferralRejectsUnownedEvidence(t *testing.T) {
	var writer Writer
	if err := writer.DeferFault(source.Span{File: "control.lua"}, 0, source.ControlFaultGotoEntersLocal, nil, lexical.CellEvidence{}); err == nil {
		t.Fatal("DeferFault accepted an absent owner and label")
	}
}
