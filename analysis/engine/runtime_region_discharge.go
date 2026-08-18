// runtime_region_discharge.go derives one Region's support-axis widening
// relation and applies it at the recurrence publication boundary.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// regionDischarge is the support-axis counterpart of a Region's value widening
// scope. Value widening bounds how far a coordinate's value climbs before the
// ascent stops; this relation bounds how finely the recurrence head partitions
// its guard support while it climbs. Both are sealed once per active Region,
// and both are applied at the same one recurrence publication.
//
// The relation is the head's own coordinate scope with every cycle-local
// decision existentially discharged. Discharging a decision joins the head's
// plane across that decision's two valuations, so the discharged head lies
// above the exact one in the order the ascent already climbs. This is an
// acceleration operator in the lattice, not a budget: nothing is abandoned,
// the ascent still runs to a post-fixpoint and the descent that follows reads
// the exact relations again. What the head stops doing is distinguishing which
// way its own cycle branched on an earlier iteration, which is the one
// distinction a loop header exists to merge.
//
// The operator has no numeric threshold, and deliberately so. A cycle-local
// coordinate carries no distinction the head can ever settle, so admitting it
// for some number of iterations only postpones the same join; a coordinate
// established outside the cycle is never discharged at all, at any iteration
// count. The selection is therefore structural and binary, read from the
// immutable WTO classification and the sealed transport relations.
type regionDischarge struct {
	plan  carrier.ReindexPlan
	atoms int
}

func (discharge regionDischarge) available() bool {
	return discharge.atoms > 0 && discharge.plan.Valid()
}

// decisionSet is cold derivation scratch. It never reaches a sealed row.
type decisionSet map[composition.Key]struct{}

func (set decisionSet) add(decision equation.Decision) bool {
	if !decision.Available() {
		return false
	}
	set[decision.Key()] = struct{}{}
	return true
}

func (set decisionSet) has(decision equation.Decision) bool {
	_, present := set[decision.Key()]
	return present
}

// addFormulaDecisions records every decision one sealed formula constrains.
func (set decisionSet) addFormulaDecisions(expr equation.Expr) bool {
	if !expr.Available() {
		return false
	}
	for _, decision := range expr.Decisions() {
		if !set.add(decision) {
			return false
		}
	}
	return true
}

// addInputDecisions records every target-scope decision one ingress
// establishes: the image of its coordinate relation plus every decision its
// post filter constrains. A forgotten source decision establishes nothing, so
// it contributes no target coordinate here.
func (set decisionSet) addInputDecisions(input equation.Input) bool {
	if !input.Available() {
		return false
	}
	reindex := input.Reindex()
	if !reindex.Available() {
		return false
	}
	for index := 0; index < reindex.Count(); index++ {
		mapping, ok := reindex.At(index)
		if !ok {
			return false
		}
		switch mapping.Disposition {
		case equation.DecisionIdentity, equation.DecisionRename:
			if !set.add(mapping.Target) {
				return false
			}
		case equation.DecisionSubstitute:
			if !set.addFormulaDecisions(mapping.Expr) {
				return false
			}
		case equation.DecisionForget:
		default:
			return false
		}
	}
	return set.addFormulaDecisions(input.Post())
}

// addGroupDecisions records every decision one head producer establishes at
// its output: the image of each of its transports and its own premise.
func (set decisionSet) addGroupDecisions(group equation.GroupNode) bool {
	for index := 0; index < group.InputCount(); index++ {
		input, ok := group.InputAt(index)
		if !ok || !set.addInputDecisions(input) {
			return false
		}
	}
	if environment, present := group.EnvironmentInput(); present && !set.addInputDecisions(environment) {
		return false
	}
	return set.addFormulaDecisions(group.Premise())
}

// regionHeadIngress splits the decisions established at one Region's head into
// the two sets the support-axis widening is derived from: those the Region's
// own recurrence establishes, and those anything outside that recurrence
// establishes. Membership is read from the immutable WTO classification and
// the sealed transport relations; no name, ordering heuristic, or cost measure
// takes part.
func regionHeadIngress(graph *equation.Graph, region equation.RegionView, head equation.Point) (recurrence, outside decisionSet, ok bool) {
	recurrence, outside = decisionSet{}, decisionSet{}
	for index := 0; index < region.BackEnvironmentEdgeCount(); index++ {
		edge, edgeOK := region.BackEnvironmentEdgeAt(index)
		if !edgeOK || !recurrence.addInputDecisions(edge.Input()) {
			return nil, nil, false
		}
	}
	for index := 0; index < region.BackFactorEdgeCount(); index++ {
		edge, edgeOK := region.BackFactorEdgeAt(index)
		if !edgeOK || !recurrence.addInputDecisions(edge.Input()) {
			return nil, nil, false
		}
	}
	for index := 0; index < region.BackHeadProducerCount(); index++ {
		group, groupOK := region.BackHeadProducerAt(index)
		if !groupOK || !recurrence.addGroupDecisions(group) {
			return nil, nil, false
		}
	}
	// A self-transport at the head is absent from the immutable recurrence
	// graph but still closes the head's cycle at runtime, exactly as the region
	// binder classifies it. It carries the same recurrence coordinates.
	for edgeIndex := 0; edgeIndex < graph.EnvironmentEdgeTotal(); edgeIndex++ {
		edge, edgeOK := graph.EnvironmentEdgeAtIndex(edgeIndex)
		if !edgeOK {
			return nil, nil, false
		}
		if !edge.TransportOnly() || edge.Target() != head {
			continue
		}
		if !recurrence.addInputDecisions(edge.Input()) {
			return nil, nil, false
		}
	}
	for index := 0; index < region.ExternalEnvironmentEdgeCount(); index++ {
		edge, edgeOK := region.ExternalEnvironmentEdgeAt(index)
		if !edgeOK || !outside.addInputDecisions(edge.Input()) {
			return nil, nil, false
		}
	}
	for index := 0; index < region.ExternalFactorEdgeCount(); index++ {
		edge, edgeOK := region.ExternalFactorEdgeAt(index)
		if !edgeOK || !outside.addInputDecisions(edge.Input()) {
			return nil, nil, false
		}
	}
	for index := 0; index < region.ExternalHeadProducerCount(); index++ {
		group, groupOK := region.ExternalHeadProducerAt(index)
		if !groupOK || !outside.addGroupDecisions(group) {
			return nil, nil, false
		}
	}
	// The head's own Init is the value the ascent starts from. Every decision
	// it names is established before the cycle runs and therefore belongs to
	// the outside half.
	if init, disposition, initOK := head.Init(); initOK && disposition == equation.InitPresent && !outside.addFormulaDecisions(init) {
		return nil, nil, false
	}
	return recurrence, outside, true
}

// regionLocalDecisions is the Region's cycle-local coordinate set: a decision
// its own recurrence establishes at the head and nothing outside the cycle
// establishes there. Those are exactly the coordinates whose distinctions at
// the head separate one iteration from another.
func regionLocalDecisions(graph *equation.Graph, region equation.RegionView, head equation.Point) ([]equation.Decision, bool) {
	recurrence, outside, ok := regionHeadIngress(graph, region, head)
	if !ok {
		return nil, false
	}
	scope := head.Scope()
	if !scope.Available() {
		return nil, true
	}
	local := make([]equation.Decision, 0, scope.Count())
	for index := 0; index < scope.Count(); index++ {
		decision, decisionOK := scope.At(index)
		if !decisionOK {
			return nil, false
		}
		if recurrence.has(decision) && !outside.has(decision) {
			local = append(local, decision)
		}
	}
	return local, true
}

// sealRegionDischarge lowers the Region's cycle-local coordinate set into one
// immutable self-relation on the head's scope. Every cycle-local decision is
// forgotten and every other decision of the head's scope is retained, so the
// head keeps its exact coordinate interface: downstream transports still read
// the scope they were sealed against, they simply see a head that no longer
// distinguishes its own iterations. A Region with no cycle-local coordinate
// seals no relation and pays nothing.
func sealRegionDischarge(graph *equation.Graph, region equation.RegionView, head equation.Point, runtime *carrier.Composition, plans runtimeReindexes, late bool) (regionDischarge, bool) {
	if graph == nil || runtime == nil || !head.Available() {
		return regionDischarge{}, false
	}
	local, ok := regionLocalDecisions(graph, region, head)
	if !ok {
		return regionDischarge{}, false
	}
	if len(local) == 0 {
		return regionDischarge{}, true
	}
	scope := head.Scope()
	bound, scoped := plans.scope(scope)
	if !scoped {
		return regionDischarge{}, false
	}
	discharged := decisionSet{}
	for _, decision := range local {
		if !discharged.add(decision) {
			return regionDischarge{}, false
		}
	}
	builder, builderOK := runtime.NewReindex(bound, bound)
	if late {
		builder, builderOK = runtime.NewRuntimeReindex(bound, bound)
	}
	if !builderOK || builder == nil {
		return regionDischarge{}, false
	}
	for index := 0; index < scope.Count(); index++ {
		decision, decisionOK := scope.At(index)
		if !decisionOK {
			return regionDischarge{}, false
		}
		atom, atomOK := plans.decisions[decision.Key()]
		if !atomOK || atom == guard.Atom(0) {
			return regionDischarge{}, false
		}
		if discharged.has(decision) {
			if !builder.Forget(atom) {
				return regionDischarge{}, false
			}
			continue
		}
		if !builder.Identity(atom) {
			return regionDischarge{}, false
		}
	}
	plan, sealed := builder.Seal()
	if !sealed || !plan.Valid() {
		return regionDischarge{}, false
	}
	return regionDischarge{plan: plan, atoms: len(local)}, true
}

// dischargeAscentRHS applies the Region's support-axis widening to one ascent
// right-hand side. It runs only in the ascent phase and only on the operands
// that publication widens, exactly as region.widen does: the descent that
// follows reads the exact relations again and recovers what it can.
func (epoch *executorEpoch) dischargeAscentRHS(region runtimeRegion, rhs carrier.PointRHS) (carrier.PointRHS, bool) {
	if epoch == nil || epoch.work == nil || epoch.runtime == nil || epoch.runtime.carrier == nil {
		return carrier.PointRHS{}, false
	}
	if !region.discharge.available() {
		return rhs, true
	}
	whole, wholeOK := support.True(epoch.runtime.carrier.Guards())
	if !wholeOK {
		return carrier.PointRHS{}, false
	}
	return epoch.work.TransportPointRHS(rhs, whole, region.discharge.plan, whole)
}
