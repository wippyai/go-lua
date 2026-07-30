package typestate

import (
	"strings"
	"testing"
)

func TestDefinitionValidateAcceptsDeclaredFSM(t *testing.T) {
	def := Definition{
		Protocol:    "transaction",
		States:      []State{"active", "finished"},
		FinalStates: []State{"finished"},
		Transitions: []TransitionDecl{{From: "active", To: "finished"}},
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !def.HasState("active") || !def.IsFinal("finished") || !def.AllowsTransition("active", "finished") {
		t.Fatalf("definition queries failed for %#v", def)
	}
	if def.AllowsTransition("finished", "active") {
		t.Fatalf("unexpected reverse transition allowed")
	}
}

func TestDefinitionValidateRejectsMalformedFSM(t *testing.T) {
	tests := []struct {
		name string
		def  Definition
		want string
	}{
		{
			name: "missing protocol",
			def:  Definition{States: []State{"open"}},
			want: "missing name",
		},
		{
			name: "missing states",
			def:  Definition{Protocol: "cursor"},
			want: "has no states",
		},
		{
			name: "unknown final",
			def: Definition{
				Protocol:    "cursor",
				States:      []State{"open"},
				FinalStates: []State{"closed"},
			},
			want: "final state \"closed\" is not declared",
		},
		{
			name: "unknown transition target",
			def: Definition{
				Protocol:    "cursor",
				States:      []State{"open"},
				Transitions: []TransitionDecl{{From: "open", To: "closed"}},
			},
			want: "transition target \"closed\" is not declared",
		},
		{
			name: "duplicate transition",
			def: Definition{
				Protocol:    "cursor",
				States:      []State{"open", "closed"},
				Transitions: []TransitionDecl{{From: "open", To: "closed"}, {From: "open", To: "closed"}},
			},
			want: "duplicates transition",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDefinitionNormalizedSortsAndDeduplicates(t *testing.T) {
	got := (Definition{
		Protocol:    "tx",
		States:      []State{"finished", "active", "active"},
		FinalStates: []State{"finished", "finished"},
		Transitions: []TransitionDecl{
			{From: "active", To: "finished"},
			{From: "active", To: "finished"},
		},
	}).Normalized()
	if len(got.States) != 2 || got.States[0] != "active" || got.States[1] != "finished" {
		t.Fatalf("states = %#v, want sorted unique", got.States)
	}
	if len(got.FinalStates) != 1 || got.FinalStates[0] != "finished" {
		t.Fatalf("final states = %#v, want sorted unique", got.FinalStates)
	}
	if len(got.Transitions) != 1 || got.Transitions[0] != (TransitionDecl{From: "active", To: "finished"}) {
		t.Fatalf("transitions = %#v, want sorted unique", got.Transitions)
	}
}
