// Package recurrence derives the source-control recurrence annotation used by
// Flow assembly.
//
// This package is deliberately not a scheduler and does not build a second
// graph. Sourcecontrol owns topology and reachability; recurrence only
// computes the cyclic components of that already-sealed topology and projects
// exact reset ranges onto the sourcecontrol Arc denominator. Its published
// result contains no coordinates, SCC numbering, sourcecontrol graph, or
// synthetic Mu terms.
package recurrence

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal derives recurrence from the sealed sourcecontrol topology. It does
// not reconstruct adjacency from Source order, and Function availability is
// already reflected only in sourcecontrol.Reachable. The returned Result is
// assembly-local and can be discarded after final Edge materialization. The
// top-level Flow assembler is the provenance authority for Flow, Body,
// containment, and sourcecontrol results; this private projection therefore
// does not add an identity token or adapter. It still applies exact owner
// denominators and the source-fenced Coordinate query at every graph use, so
// a same-shaped foreign sourcecontrol result fails closed.
func Seal(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	graph *sourcecontrol.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	result, _, err := sealWithPlan(sourceView, flow, bodies, forest, graph, nil, nil, staticID, moduleID)
	return result, err
}

// SealWithPlan derives recurrence and atomically binds one already-sealed
// causal route plan while the private SCC partition remains live. The Plan is
// assembly-local; neither it nor the Binding is retained by Result.
func SealWithPlan(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	graph *sourcecontrol.Result,
	plan *routeplan.Plan,
	outcomePhases *sourcecontrol.OutcomePhases,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, *Binding, error) {
	if plan == nil {
		return nil, nil, errors.New("program/flow/recurrence: route plan is unavailable")
	}
	return sealWithPlan(sourceView, flow, bodies, forest, graph, plan, outcomePhases, staticID, moduleID)
}

func sealWithPlan(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	graph *sourcecontrol.Result,
	plan *routeplan.Plan,
	outcomePhases *sourcecontrol.OutcomePhases,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, *Binding, error) {
	sourceID := sourceView.Identity().ContentID()
	flowID := flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, nil, errors.New("program/flow/recurrence: owner identity is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return nil, nil, errors.New("program/flow/recurrence: Body provenance disagrees with Source or Flow")
	}
	if !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return nil, nil, errors.New("program/flow/recurrence: containment provenance disagrees with Source, Flow, Static, or Module")
	}
	if !sourcecontrol.Matches(graph, sourceID, flowID, staticID, moduleID) {
		return nil, nil, errors.New("program/flow/recurrence: sourcecontrol provenance disagrees with Source, Flow, Static, or Module")
	}
	counts, err := validateOwners(sourceView, flow, bodies, forest, graph)
	if err != nil {
		return nil, nil, err
	}
	if !graph.VertexCatalogAvailable() {
		return nil, nil, errors.New("program/flow/recurrence: semantic vertex catalog is unavailable")
	}
	parts, err := deriveComponents(graph)
	if err != nil {
		return nil, nil, err
	}
	heads, err := deriveHeads(sourceView, flow, forest, graph, parts, counts)
	if err != nil {
		return nil, nil, err
	}
	// The local WTO denominator is SourceControl's complete reachable graph.
	// A causal Plan selects transfer rows only; it must never erase a vertex
	// from the parent-issued schedule.
	live := reachableNodes(graph)
	draft, err := deriveHierarchy(graph, live)
	if err != nil {
		return nil, nil, err
	}
	if outcomePhases != nil {
		if !outcomePhases.Matches(graph) {
			return nil, nil, errors.New("program/flow/recurrence: Outcome-phase view is foreign")
		}
		if err := draft.placeOutcomePhasePoints(outcomePhases, plan, graph, parts, heads); err != nil {
			return nil, nil, err
		}
	}
	hierarchy, err := draft.transfer()
	if err != nil {
		return nil, nil, err
	}
	trace, err := buildEventTrace(sourceView, flow, bodies, forest, graph, parts)
	if err != nil {
		return nil, nil, err
	}
	if err := validateDecisionCoverage(sourceView, flow, forest, graph, parts, trace, counts); err != nil {
		return nil, nil, err
	}
	layout := deriveNestedHeadLayout(flow, bodies, forest, parts, heads, trace, counts)
	work, err := classifyArcs(sourceView, flow, graph, parts, heads, trace, counts, layout)
	if err != nil {
		return nil, nil, err
	}
	if err := requireCyclicWitness(parts, heads, work); err != nil {
		return nil, nil, err
	}
	result, err := materialize(counts, parts, heads, trace, work, layout.aliases, sourceID, flowID, staticID, moduleID)
	if err != nil {
		return nil, nil, err
	}
	if plan == nil {
		return result, nil, nil
	}
	binding, err := bindPlan(result, plan, graph, parts, heads, hierarchy, sourceView, flow)
	if err != nil {
		return nil, nil, err
	}
	return result, binding, nil
}

func reachableNodes(graph *sourcecontrol.Result) []bool {
	if graph == nil || graph.NodeCount() == 0 {
		return nil
	}
	live := make([]bool, graph.NodeCount())
	for node := range live {
		live[node] = graph.Reachable(uint32(node))
	}
	return live
}

func validateOwners(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	graph *sourcecontrol.Result,
) ([keyspace.FamilyCount]uint32, error) {
	var counts [keyspace.FamilyCount]uint32
	identity := sourceView.Identity()
	if !identity.ContentID().Available() || !flow.Cold().ContentID().Available() || bodies == nil || forest == nil || graph == nil {
		return counts, errors.New("program/flow/recurrence: one or more sealed owners are unavailable")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) || uint64(count) >= uint64(^uint(0)>>1) {
			return counts, errors.New("program/flow/recurrence: Source family denominator is invalid")
		}
		counts[family] = uint32(count)
	}
	var authoredTotal uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family != keyspace.FamilyOutcome {
			authoredTotal += uint64(counts[family])
		}
	}
	// Containment is the pre-Outcome forest. Outcome identities are installed
	// by Source commit and have no containment parent, so the exact owner
	// denominator intentionally excludes that derived family.
	if authoredTotal >= uint64(^uint(0)>>1) || counts[keyspace.FamilyBody] == 0 || bodies.BodyCount() != int(counts[keyspace.FamilyBody]) ||
		uint64(forest.Count()) != authoredTotal || graph.NodeCount() == 0 {
		// structural denominator diagnostics are deliberately kept in the
		// returned error below; no partial owner is accepted.
		return counts, errors.New("program/flow/recurrence: structural denominators disagree")
	}
	if err := exactDecisionCounts(flow, counts); err != nil {
		return counts, err
	}
	return counts, nil
}

func exactDecisionCounts(flow authored.View, counts [keyspace.FamilyCount]uint32) error {
	if flow.Operators().Selects().Count() != int(counts[keyspace.FamilySelect]) ||
		flow.Control().Branches().Count() != int(counts[keyspace.FamilyBranch]) ||
		flow.Control().Loops().Count() != int(counts[keyspace.FamilyLoop]) ||
		flow.Control().Labels().Count() != int(counts[keyspace.FamilyLabel]) ||
		flow.Control().Gotos().Count() != int(counts[keyspace.FamilyGoto]) {
		return errors.New("program/flow/recurrence: authored decision denominator disagrees with Source")
	}
	return nil
}

func deriveHeads(
	sourceView source.View,
	flow authored.View,
	forest *containment.Result,
	graph *sourcecontrol.Result,
	parts components,
	counts [keyspace.FamilyCount]uint32,
) ([]keyspace.Term, error) {
	heads := make([]keyspace.Term, len(parts.sizes))
	labels := flow.Control().Labels()
	for index := 0; index < labels.Count(); index++ {
		term, ok := labels.At(index)
		if !ok || !validExisting(term, counts) || forest.Static(term) {
			continue
		}
		installHead(sourceView, flow, graph, parts, heads, term)
	}
	loops := flow.Control().Loops()
	for index := 0; index < loops.Count(); index++ {
		term, ok := loops.At(index)
		if !ok || !validExisting(term, counts) || forest.Static(term) {
			continue
		}
		installHead(sourceView, flow, graph, parts, heads, term)
	}
	for component, cyclic := range parts.cyclic {
		if cyclic && heads[component] == 0 {
			return nil, errors.New("program/flow/recurrence: cyclic component has no dynamic Label or Loop head")
		}
	}
	return heads, nil
}

func installHead(
	sourceView source.View,
	flow authored.View,
	graph *sourcecontrol.Result,
	parts components,
	heads []keyspace.Term,
	term keyspace.Term,
) {
	node, ok := decisionCoordinate(sourceView, flow, graph, term)
	if !ok || !graph.Reachable(node) || uint64(node) >= uint64(len(parts.of)) {
		return
	}
	component := parts.of[node]
	if component == unassignedComponent || !parts.cyclic[component] {
		return
	}
	// Head identity is selected by the existing authored Term order.  Vertex
	// paths identify the component directory after this choice; they are not a
	// substitute for the canonical Label/Loop identity.  In an irreducible
	// crossed-goto component, path order can place a later Label before the
	// authored first Label and thereby leak a noncanonical Mu head.
	if heads[component] == 0 {
		heads[component] = term
		return
	}
	if term < heads[component] {
		heads[component] = term
	}
}

// decisionCoordinate maps an authored decision to the sourcecontrol node at
// which it is re-evaluated. Dynamic numeric, generic, and repeat loops own a
// hidden decision coordinate; their visible Loop Term is the canonical public
// head even though its ordinary source root is outside the cyclic component.
func decisionCoordinate(
	sourceView source.View,
	flow authored.View,
	graph *sourcecontrol.Result,
	term keyspace.Term,
) (uint32, bool) {
	if keyspace.TermFamily(term) != keyspace.FamilyLoop {
		return graph.Coordinate(sourceView, term)
	}
	_, _, loopKind, _, ok := flow.Control().Loops().Get(term)
	if !ok {
		return 0, false
	}
	if loopKind != kind.LoopWhile {
		// Hidden decisions are keyed only by the authored Loop ordinal. Force
		// the ordinary occurrence through Source's identity fence first so a
		// foreign graph cannot make a same-shaped hidden coordinate appear
		// valid merely because its dense sidecar has that ordinal.
		if _, ordinaryOK := graph.Coordinate(sourceView, term); !ordinaryOK {
			return 0, false
		}
		return graph.Decision(term)
	}
	return graph.Coordinate(sourceView, term)
}

func validExisting(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= counts[family]
}

func validateDecisionCoverage(
	sourceView source.View,
	flow authored.View,
	forest *containment.Result,
	graph *sourcecontrol.Result,
	parts components,
	trace eventTrace,
	counts [keyspace.FamilyCount]uint32,
) error {
	var seen [keyspace.FamilyCount][]bool
	for _, family := range [...]keyspace.Family{keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop} {
		seen[family] = make([]bool, int(counts[family])+1)
	}
	for _, event := range trace.events {
		family, ordinal := keyspace.TermFamily(event.term), keyspace.TermOrdinal(event.term)
		if family != keyspace.FamilySelect && family != keyspace.FamilyBranch && family != keyspace.FamilyLoop ||
			ordinal == 0 || uint64(ordinal) >= uint64(len(seen[family])) || seen[family][ordinal] {
			return errors.New("program/flow/recurrence: semantic decision was emitted twice or is invalid")
		}
		if event.component == unassignedComponent || uint64(event.node) >= uint64(len(parts.of)) || parts.of[event.node] != event.component {
			return errors.New("program/flow/recurrence: semantic decision has no SCC")
		}
		seen[family][ordinal] = true
	}
	active := func(term keyspace.Term) bool {
		if forest.Static(term) {
			return false
		}
		node, ok := decisionCoordinate(sourceView, flow, graph, term)
		return ok && graph.Reachable(node)
	}
	check := func(family keyspace.Family, term keyspace.Term) error {
		if !active(term) {
			return nil
		}
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(seen[family])) || !seen[family][ordinal] {
			return errors.New("program/flow/recurrence: reachable decision was not emitted")
		}
		return nil
	}
	selects := flow.Operators().Selects()
	for index := 0; index < selects.Count(); index++ {
		term, ok := selects.At(index)
		if !ok || !validExisting(term, counts) {
			return errors.New("program/flow/recurrence: Select denominator row is invalid")
		}
		if err := check(keyspace.FamilySelect, term); err != nil {
			return err
		}
	}
	branches := flow.Control().Branches()
	for index := 0; index < branches.Count(); index++ {
		term, ok := branches.At(index)
		if !ok || !validExisting(term, counts) {
			return errors.New("program/flow/recurrence: Branch denominator row is invalid")
		}
		if err := check(keyspace.FamilyBranch, term); err != nil {
			return err
		}
	}
	loops := flow.Control().Loops()
	for index := 0; index < loops.Count(); index++ {
		term, ok := loops.At(index)
		if !ok || !validExisting(term, counts) {
			return errors.New("program/flow/recurrence: Loop denominator row is invalid")
		}
		if err := check(keyspace.FamilyLoop, term); err != nil {
			return err
		}
	}
	return nil
}
