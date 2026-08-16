package control

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestShapeAcceptsSourceControlFaultAlongsideAuthoredControl(t *testing.T) {
	counts := controlCounts(1, 0, 0, 0, 0, 0, 1, 1, 0, 0)
	counts[keyspace.FamilyControlFault] = 1
	body := terms(counts, keyspace.FamilyBody, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	gotoTerm := terms(counts, keyspace.FamilyGoto, 1)
	faultTerm := terms(counts, keyspace.FamilyControlFault, 1)
	input := authored.Input{Control: authored.ControlInput{
		Labels: []authored.Label{{Owner: body}},
		Gotos:  []authored.Goto{{Owner: body, Target: label}},
	}}
	input.Counts = counts
	fault := source.ControlFault{Owner: body, Kind: source.ControlFaultUndefinedGoto}
	f := openShapeFixture(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{label, faultTerm, gotoTerm}),
		flow:   input,
		faults: []source.ControlFault{fault},
	})
	shape := f.seal(t)

	order := f.preimage.Order()
	for index, want := range []keyspace.Term{label, faultTerm, gotoTerm} {
		if got, ok := order.BodyAt(body, index); !ok || got != want {
			t.Fatalf("Source Body order[%d] = %v/%v, want %v/true", index, got, ok, want)
		}
	}
	if got, ok := f.preimage.Faults().At(faultTerm); !ok || got != fault {
		t.Fatalf("Source Faults.At(%v) = %#v/%v, want %#v/true", faultTerm, got, ok, fault)
	}

	if got, ok := shape.LabelBody(label); !ok || got != body {
		t.Fatalf("LabelBody(label) = %v/%v, want %v/true", got, ok, body)
	}
	if got, ok := shape.GotoTargetBody(gotoTerm); !ok || got != body {
		t.Fatalf("GotoTargetBody(goto) = %v/%v, want %v/true", got, ok, body)
	}
	for name, query := range map[string]func() (keyspace.Term, bool){
		"LabelBody(fault)":      func() (keyspace.Term, bool) { return shape.LabelBody(faultTerm) },
		"BreakLoop(fault)":      func() (keyspace.Term, bool) { return shape.BreakLoop(faultTerm) },
		"GotoTargetBody(fault)": func() (keyspace.Term, bool) { return shape.GotoTargetBody(faultTerm) },
	} {
		if got, ok := query(); ok || got != 0 {
			t.Fatalf("%s = %v/%v, want zero/false", name, got, ok)
		}
	}
}
