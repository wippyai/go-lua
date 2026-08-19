package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// selectedOverlayInstallFixture is one settled epoch over a real sealed
// runtime together with the real prepared overlay its accepted activation
// materializes. It reproduces exactly the sequence the solve loop performs
// between the epoch's fixed point and the installation.
type selectedOverlayInstallFixture struct {
	runtime *solverRuntime
	epoch   *executorEpoch
	overlay *preparedSelectedFactorOverlay
}

func newSelectedOverlayInstallFixture(t *testing.T) selectedOverlayInstallFixture {
	t.Helper()
	law := newSelectedOverlayLawFixture(t)
	solver := law.solver
	runtime := solver.runtime
	epoch, opened := newRuntimeEpoch(runtime, solver.relation, context.Background())
	if !opened {
		t.Fatal("selected-overlay install fixture epoch did not open")
	}
	if !epoch.run() || !epoch.activationPending {
		t.Fatal("selected-overlay install fixture epoch settled without an accepted activation")
	}
	frontier, canonical := canonicalizeAcceptedActivations(runtime.topology, epoch.activations)
	delta, subtracted := subtractAcceptedActivations(runtime.topology, frontier, solver.relation.Rows())
	if !canonical || !subtracted || len(delta) == 0 {
		t.Fatalf("selected-overlay install fixture delta canonical=%t subtracted=%t rows=%d", canonical, subtracted, len(delta))
	}
	epoch.activations, epoch.activationPending = nil, false
	accepted, merged := mergeAcceptedActivations(runtime.topology, solver.relation.Rows(), delta)
	if !merged {
		t.Fatal("selected-overlay install fixture relation did not merge")
	}
	published, publishedOK := runtime.topology.Publish(solver.relation, accepted)
	if !publishedOK {
		t.Fatal("selected-overlay install fixture relation did not publish")
	}
	overlay, prepared := runtime.prepareSelectedFactorOverlay(delta, published)
	if !prepared || overlay == nil || len(overlay.additions) == 0 || len(overlay.targets) == 0 {
		t.Fatalf("selected-overlay install fixture overlay prepared=%t additions=%d", prepared, len(overlay.additions))
	}
	return selectedOverlayInstallFixture{runtime: runtime, epoch: epoch, overlay: overlay}
}

// TestSelectedFactorOverlayRefusalLeavesTheRunningEpochUntouched states the
// installation transaction: an overlay refused at any admission leaves the
// epoch on the frontier it already runs. None of the refused frontier - its
// edges, CSR rows, demand epoch, region rows, execution view or operand
// plane - is observable afterwards, so the caller's teardown is a consequence
// of the refusal rather than the thing that makes it safe.
func TestSelectedFactorOverlayRefusalLeavesTheRunningEpochUntouched(t *testing.T) {
	fixture := newSelectedOverlayInstallFixture(t)
	runtime, epoch, overlay := fixture.runtime, fixture.epoch, fixture.overlay

	// One region row whose factor operand names an edge outside the frontier
	// it is bound to. Every activation admission accepts that row; the operand
	// plane is the exact authority that refuses it, and it is the last
	// derivation the installation performs.
	cycles, _, _ := newRegionDischargeGraph(t)
	pointRegion := make([]int, len(overlay.pointRegion))
	for index := range pointRegion {
		pointRegion[index] = schedule.NoRegion
	}
	overlay.execution = cycles.Schedule()
	overlay.regions = []runtimeRegion{{parent: schedule.NoRegion, factorExternal: []int{len(overlay.additions) + overlay.previousEdgeCount}}}
	overlay.activeRegions = []bool{false}
	overlay.regionChildren = make([][]int, 1)
	overlay.pointRegion = pointRegion
	if overlay.execution.RegionCount() != len(overlay.regions) {
		t.Fatalf("refusal region view rows=%d schedule=%d", len(overlay.regions), overlay.execution.RegionCount())
	}

	edges := append([]runtimeFactorEdge(nil), runtime.factorEdges...)
	incoming := append([][]int(nil), runtime.factorIncoming...)
	outgoing := append([][]int(nil), runtime.overlay.factorOutgoing...)
	demandEpoch := epoch.demand
	plane := runtime.operands
	generation := runtime.overlay.generation
	points, execution, executionDemand := runtime.points, runtime.execution, runtime.executionDemand
	regions, activeRegions := len(runtime.regions), len(runtime.activeRegions)

	if epoch.installSelectedFactorOverlay(overlay) {
		t.Fatal("an overlay whose region rows have no operand plane installed")
	}

	if len(runtime.factorEdges) != len(edges) {
		t.Fatalf("refused overlay published %d factor edges over %d", len(runtime.factorEdges), len(edges))
	}
	for index := range edges {
		if runtime.factorEdges[index] != edges[index] {
			t.Fatalf("refused overlay replaced factor edge %d", index)
		}
	}
	for index := range incoming {
		if len(runtime.factorIncoming[index]) != len(incoming[index]) || len(runtime.overlay.factorOutgoing[index]) != len(outgoing[index]) {
			t.Fatalf("refused overlay published CSR rows at point %d", index)
		}
	}
	if epoch.demand != demandEpoch || !epoch.demand.Live() {
		t.Fatalf("refused overlay replaced the demand epoch changed=%t live=%t", epoch.demand != demandEpoch, epoch.demand.Live())
	}
	if runtime.points != points || runtime.execution != execution || runtime.executionDemand != executionDemand {
		t.Fatal("refused overlay published its execution view")
	}
	if len(runtime.regions) != regions || len(runtime.activeRegions) != activeRegions {
		t.Fatalf("refused overlay published %d region rows over %d", len(runtime.regions), regions)
	}
	if runtime.operands != plane || epoch.operands.plane != plane {
		t.Fatal("refused overlay published its operand plane")
	}
	if runtime.overlay.generation != generation {
		t.Fatal("refused overlay advanced the frontier generation")
	}
	if epoch.queue.count != 0 {
		t.Fatalf("refused overlay woke %d points", epoch.queue.count)
	}
	for _, target := range overlay.targets {
		if epoch.structuralDirty[target] || epoch.postfixDirty[target] || epoch.queue.ready[target] {
			t.Fatalf("refused overlay woke target %d", target)
		}
	}
}

// TestSelectedFactorOverlayInstallPublishesOneFrontier is the same prepared
// overlay installed unchanged: one commit publishes the whole frontier -
// edges, CSR rows, demand epoch, execution view, operand plane - wakes
// exactly the points it named, and leaves an epoch that runs to its next
// fixed point.
func TestSelectedFactorOverlayInstallPublishesOneFrontier(t *testing.T) {
	fixture := newSelectedOverlayInstallFixture(t)
	runtime, epoch, overlay := fixture.runtime, fixture.epoch, fixture.overlay

	previousEdges := len(runtime.factorEdges)
	additions := append([]preparedFactorAddition(nil), overlay.additions...)
	targets := append([]int(nil), overlay.targets...)
	demandEpoch := epoch.demand
	plane := runtime.operands
	generation := runtime.overlay.generation

	if !epoch.installSelectedFactorOverlay(overlay) {
		t.Fatal("a prepared selected overlay did not install")
	}

	if len(runtime.factorEdges) != previousEdges+len(additions) {
		t.Fatalf("installed factor edges=%d want %d", len(runtime.factorEdges), previousEdges+len(additions))
	}
	for index, addition := range additions {
		if runtime.factorEdges[previousEdges+index] != addition.edge {
			t.Fatalf("installed factor edge %d is not the prepared addition", previousEdges+index)
		}
		if !containsEdgeIndex(runtime.overlay.factorOutgoing[addition.edge.source], addition.edge.index) || !containsEdgeIndex(runtime.factorIncoming[addition.edge.target], addition.edge.index) {
			t.Fatalf("installed edge %d is absent from its CSR rows", addition.edge.index)
		}
	}
	if epoch.demand == demandEpoch || !epoch.demand.Live() || demandEpoch.Live() {
		t.Fatal("the widened demand epoch did not replace the previous one")
	}
	if runtime.execution != overlay.execution || runtime.executionDemand != overlay.executionDemand || runtime.points != overlay.executionDemand {
		t.Fatal("the execution view was not published")
	}
	if runtime.operands == plane || epoch.operands.plane != runtime.operands {
		t.Fatal("the operand plane was not re-derived over the installed rows")
	}
	if runtime.overlay.generation != generation.Next() {
		t.Fatal("the frontier generation did not advance exactly once")
	}
	for _, target := range targets {
		if !epoch.structuralDirty[target] || !epoch.postfixDirty[target] || !epoch.queue.ready[target] {
			t.Fatalf("installed target %d was not woken", target)
		}
	}
	if epoch.queue.count != len(epoch.postfixPending) || epoch.postfixHead != 0 || epoch.queue.count == 0 {
		t.Fatalf("installed wake frontier queue=%d postfix=%d head=%d", epoch.queue.count, len(epoch.postfixPending), epoch.postfixHead)
	}
	if !epoch.run() {
		t.Fatal("the installed frontier did not run to its fixed point")
	}
}

func containsEdgeIndex(row []int, edge int) bool {
	for _, index := range row {
		if index == edge {
			return true
		}
	}
	return false
}
