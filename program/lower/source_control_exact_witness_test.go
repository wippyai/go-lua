package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// The witness stays at the four final owner vocabularies.  It deliberately
// does not reconstruct the retired per-term Program Mu/continuation planes.
func TestSourceControlExactWitnesses(t *testing.T) {
	for _, sample := range []struct {
		name        string
		input       string
		loops       int
		branches    int
		returns     int
		breaks      int
		labels      int
		gotos       int
		faults      int
		staticFault bool
	}{
		{"assign", "value = 1", 0, 0, 0, 0, 0, 0, 0, false},
		{"local-assign", "local value = 1", 0, 0, 0, 0, 0, 0, 0, false},
		{"call", "invoke()", 0, 0, 0, 0, 0, 0, 0, false},
		{"while", "while true do local value = 1 end", 1, 0, 0, 0, 0, 0, 0, false},
		{"repeat", "repeat local value = 1 until true", 1, 0, 0, 0, 0, 0, 0, false},
		{"if", "if true then local yes = 1 else local no = 2 end", 0, 1, 0, 0, 0, 0, 0, false},
		{"numeric-for", "for index = 1, 2 do local value = index end", 1, 0, 0, 0, 0, 0, 0, false},
		{"generic-for", "for key in iterate() do local seen = key end", 1, 0, 0, 0, 0, 0, 0, false},
		{"return", "return 1", 0, 0, 1, 0, 0, 0, 0, false},
		{"legal-break", "while true do break end", 1, 0, 0, 1, 0, 0, 0, false},
		{"label", "::done::", 0, 0, 0, 0, 1, 0, 0, false},
		{"backward-goto", "::again::\ngoto again", 0, 0, 0, 0, 1, 1, 0, false},
		{"undefined-goto", "goto missing", 0, 0, 0, 0, 0, 0, 1, false},
		{"static-undefined-goto", "type Snapshot = typeof(function()\ngoto missing\nend)", 0, 0, 0, 0, 0, 0, 1, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			entry, ok := p.Source().Index().Entry()
			if !ok {
				t.Fatal("missing Source entry")
			}
			if count, ok := p.Source().Order().BodyLen(entry); !ok || count == 0 {
				t.Fatalf("entry Source order = %d/%v", count, ok)
			}
			control := p.Flow().Authored().Control()
			if got := control.Loops().Count(); got != sample.loops {
				t.Fatalf("Loop count = %d, want %d", got, sample.loops)
			}
			if got := control.Branches().Count(); got != sample.branches {
				t.Fatalf("Branch count = %d, want %d", got, sample.branches)
			}
			if got := control.Returns().Count(); got != sample.returns {
				t.Fatalf("Return count = %d, want %d", got, sample.returns)
			}
			if got := control.Breaks().Count(); got != sample.breaks {
				t.Fatalf("Break count = %d, want %d", got, sample.breaks)
			}
			if got := control.Labels().Count(); got != sample.labels {
				t.Fatalf("Label count = %d, want %d", got, sample.labels)
			}
			if got := control.Gotos().Count(); got != sample.gotos {
				t.Fatalf("Goto count = %d, want %d", got, sample.gotos)
			}
			if got := p.Source().Faults().Count(); got != sample.faults {
				t.Fatalf("ControlFault count = %d, want %d", got, sample.faults)
			}
			if sample.staticFault {
				fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
				row, faultOK := p.Source().Faults().At(fault)
				if !faultOK || row.Kind != source.ControlFaultUndefinedGoto || !p.Flow().Containment().Static(fault) {
					t.Fatalf("static fault = %#v/%v static=%v", row, faultOK, p.Flow().Containment().Static(fault))
				}
			}
		})
	}
}

func TestFlowControlRowsKeepExactOperandsAndOutcomes(t *testing.T) {
	p := parseBindLower(t, "while condition() do break end\nreturn 1")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing entry")
	}
	loop := controlSourceAt(t, p, entry, 0)
	returned := controlSourceAt(t, p, entry, 1)
	control := p.Flow().Authored().Control()
	owner, body, loopKind, condition, loopOK := control.Loops().Get(loop)
	if !loopOK || owner != entry || body == 0 || condition == 0 || loopKind != kind.LoopWhile {
		t.Fatalf("Loop = owner %v body %v kind %v condition %v ok %v", owner, body, loopKind, condition, loopOK)
	}
	breakTerm, breakOK := control.Breaks().At(0)
	if !breakOK {
		t.Fatal("missing Break")
	}
	if target, ok := control.Breaks().Get(breakTerm); !ok || target != loop {
		t.Fatalf("Break target = %v/%v, want %v", target, ok, loop)
	}
	exit, exitOK := p.Flow().Outcomes().BreakExit(breakTerm)
	outcome, outcomeOK := p.Flow().Outcomes().Get(exit)
	if !exitOK || !outcomeOK || outcome.Body != body || outcome.Kind != kind.OutcomeBreak || outcome.Target != loop {
		t.Fatalf("Break outcome = %#v/%v", outcome, outcomeOK)
	}
	if _, values, returnOK := control.Returns().Get(returned); !returnOK || values == 0 {
		t.Fatalf("Return = values %v/%v", values, returnOK)
	}
}

func TestFlowCausalRecurrenceIsAttachedToBackwardGotoEdge(t *testing.T) {
	p := parseBindLower(t, "::again::\ngoto again")
	label, labelOK := p.Flow().Authored().Control().Labels().At(0)
	jump, jumpOK := p.Flow().Authored().Control().Gotos().At(0)
	if !labelOK || !jumpOK {
		t.Fatalf("label/goto = %v/%v %v/%v", label, labelOK, jump, jumpOK)
	}
	if _, target, ok := p.Flow().Authored().Control().Gotos().Get(jump); !ok || target != label {
		t.Fatalf("Goto target = %v/%v, want %v", target, ok, label)
	}
	edges := p.Flow().Causal().Edges()
	found := false
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		found = found || edgeOK && edge.Mu == label
	}
	if !found {
		t.Fatal("backward Goto has no recurrence Edge")
	}
}
