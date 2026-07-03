package diagnostics

import (
	"os"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestLifecycleFactTraceKeepsLatestDominatingSite(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeCall)
	second := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), first, true)
	graph.AddEdge(first, second, true)
	graph.AddEdge(second, graph.Exit(), true)

	resource := typestate.Resource{ID: "tx", Protocol: "transaction"}
	trace := lifecycleFactTrace{
		graph: graph,
		sites: []lifecycleFactSite{
			{
				point:    first,
				kind:     callboundary.LifecycleTransition,
				resource: resource,
				target:   pathdom.NewPath(1, "tx"),
				from:     "active",
				to:       "prepared",
			},
			{
				point:    second,
				kind:     callboundary.LifecycleTransition,
				resource: resource,
				target:   pathdom.NewPath(1, "tx"),
				from:     "prepared",
				to:       "finished",
			},
		},
	}

	got := trace.Transitions(resource)
	if len(got) != 1 || got[0].point != second {
		t.Fatalf("Transitions = %#v, want only latest dominating transition at %d", got, second)
	}
}

func TestLifecycleFactTraceKeepsBranchLocalSites(t *testing.T) {
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeCall)
	right := graph.AddNode(cfg.NodeCall)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, true)
	graph.AddEdge(branch, left, true)
	graph.AddEdge(branch, right, false)
	graph.AddEdge(left, join, true)
	graph.AddEdge(right, join, true)
	graph.AddEdge(join, graph.Exit(), true)

	resource := typestate.Resource{ID: "tx", Protocol: "transaction"}
	trace := lifecycleFactTrace{
		graph: graph,
		sites: []lifecycleFactSite{
			{
				point:    left,
				kind:     callboundary.LifecycleEscape,
				resource: resource,
				target:   pathdom.NewPath(1, "tx"),
			},
			{
				point:    right,
				kind:     callboundary.LifecycleEscape,
				resource: resource,
				target:   pathdom.NewPath(1, "tx"),
			},
		},
	}

	got := trace.Escapes(resource)
	if len(got) != 2 {
		t.Fatalf("Escapes = %#v, want both branch-local escape sites", got)
	}
}

func TestLifecycleObligationProducerDelegatesTraceOwnership(t *testing.T) {
	srcBytes, err := os.ReadFile("lifecycle_obligation.go")
	if err != nil {
		t.Fatalf("read lifecycle_obligation.go: %v", err)
	}
	src := string(srcBytes)
	for _, forbidden := range []string{
		"func lifecycleAcquireSites",
		"func lifecycleTransitionSites",
		"func lifecycleEscapeSites",
		"func lifecycleLatestSites",
		"CallOutcomeAt(",
		"callGuardCallBindings(",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("lifecycle_obligation.go contains %q; lifecycle fact scanning/pruning belongs to lifecycleFactTrace", forbidden)
		}
	}
}
