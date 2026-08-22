package lifecycle

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/typestate"
)

func lifecycleObligation(t *testing.T, states ...typestate.State) typestate.Obligation {
	t.Helper()
	obligation, ok := typestate.NewObligation(states...)
	if !ok {
		t.Fatal("NewObligation rejected valid states")
	}
	return obligation
}

func TestLifecycleLabelsNormalizeAndCompareByFacts(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	acquire := Acquire{
		Target:     p0,
		Protocol:   typestate.Protocol("transaction"),
		State:      typestate.State("active"),
		Obligation: lifecycleObligation(t, typestate.State("finished")),
	}
	if !acquire.Equals(&acquire) {
		t.Fatalf("Acquire.Equals(pointer) = false")
	}
	if acquire.Equals(Acquire{Target: p1, Protocol: acquire.Protocol, State: acquire.State, Obligation: acquire.Obligation}) {
		t.Fatalf("Acquire.Equals ignored target")
	}

	transition := Transition{Target: p0, Protocol: typestate.Protocol("socket"), From: typestate.State("open"), To: typestate.State("closed")}
	if !transition.Equals(&transition) {
		t.Fatalf("Transition.Equals(pointer) = false")
	}
	if transition.Equals(Transition{Target: p0, Protocol: transition.Protocol, From: transition.From, To: typestate.State("half_closed")}) {
		t.Fatalf("Transition.Equals ignored target state")
	}

	escape := Escape{Target: p0, Protocol: typestate.Protocol("cursor")}
	if !escape.Equals(&escape) {
		t.Fatalf("Escape.Equals(pointer) = false")
	}
	if escape.Equals(Escape{Target: p0, Protocol: typestate.Protocol("socket")}) {
		t.Fatalf("Escape.Equals ignored protocol")
	}
}

func TestLifecycleLabelsExposeStableStrings(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	tests := []struct {
		label effect.Label
		want  string
	}{
		{
			label: Acquire{
				Target:     p0,
				Protocol:   typestate.Protocol("transaction"),
				State:      typestate.State("active"),
				Obligation: lifecycleObligation(t, typestate.State("finished")),
			},
			want: "lifecycle.acquire(param[0], transaction:active -> finished)",
		},
		{
			label: Acquire{
				Target:     p0,
				Protocol:   typestate.Protocol("transaction"),
				State:      typestate.State("active"),
				Obligation: lifecycleObligation(t, typestate.State("rolled_back"), typestate.State("committed")),
			},
			want: "lifecycle.acquire(param[0], transaction:active -> committed|rolled_back)",
		},
		{
			label: Transition{Target: p0, Protocol: typestate.Protocol("socket"), From: typestate.State("open"), To: typestate.State("closed")},
			want:  "lifecycle.transition(param[0], socket:open -> closed)",
		},
		{
			label: Escape{Target: p0, Protocol: typestate.Protocol("cursor")},
			want:  "lifecycle.escape(param[0], cursor)",
		},
	}

	for _, tt := range tests {
		if got := tt.label.String(); got != tt.want {
			t.Fatalf("%T.String() = %q, want %q", tt.label, got, tt.want)
		}
	}
}
