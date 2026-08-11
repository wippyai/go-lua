package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// runtimeReindexes is the private, immutable lowering of every equation
// coordinate boundary in one Graph.  Equation remains the source of
// semantic decision identities; carrier owns only dense atoms and executable
// plans.  The maps are keyed by canonical graph-owned identities, never by
// caller strings or synthetic coordinates.
type runtimeReindexes struct {
	scopes    map[composition.Key]carrier.Scope
	plans     map[composition.Key]carrier.ReindexPlan
	decisions map[composition.Key]guard.Atom
}

func (bound runtimeReindexes) scope(scope equation.Scope) (carrier.Scope, bool) {
	if !scope.Available() {
		return carrier.Scope{}, false
	}
	value, ok := bound.scopes[scope.Key()]
	return value, ok && value.Valid()
}

func (bound runtimeReindexes) plan(reindex equation.Reindex) (carrier.ReindexPlan, bool) {
	if !reindex.Available() {
		return carrier.ReindexPlan{}, false
	}
	value, ok := bound.plans[reindex.Key()]
	return value, ok
}

// bindRuntimeReindexes crosses the sole equation-to-carrier symbolic cut.
// It verifies that the already-attached carrier was prepared with exactly the
// graph-derived Manager, seals every equation scope, then lowers each unique
// edge relation once.  No execution Work is opened here.
func bindRuntimeReindexes(graph *equation.Graph, runtime *carrier.Composition) (runtimeReindexes, bool) {
	if graph == nil || runtime == nil || runtime.Guards() == nil {
		return runtimeReindexes{}, false
	}
	decisions, ok := runtimeDecisionAtoms(graph, runtime.Guards())
	if !ok {
		return runtimeReindexes{}, false
	}
	sources := make(map[composition.Key]equation.Scope)
	reindexes := make(map[composition.Key]equation.Reindex)
	collectInput := func(input equation.Input) bool {
		if !input.Available() || !input.Point().Available() || !collectRuntimeScope(sources, input.Point().Scope()) || !input.Reindex().Available() ||
			!collectRuntimeScope(sources, input.Reindex().Source()) || !collectRuntimeScope(sources, input.Reindex().Target()) {
			return false
		}
		if prior, exists := reindexes[input.Reindex().Key()]; exists {
			return prior.Source().Key() == input.Reindex().Source().Key() && prior.Target().Key() == input.Reindex().Target().Key()
		}
		reindexes[input.Reindex().Key()] = input.Reindex()
		return true
	}
	for index := 0; index < graph.PointCount(); index++ {
		point, pointOK := graph.PointAt(schedule.Node(index))
		if !pointOK || !graph.OwnsPoint(point) || !collectRuntimeScope(sources, point.Scope()) {
			return runtimeReindexes{}, false
		}
	}
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		if !groupOK || !graph.OwnsGroup(group) || !collectRuntimeScope(sources, group.Output().Scope()) {
			return runtimeReindexes{}, false
		}
		for inputIndex := 0; inputIndex < group.InputCount(); inputIndex++ {
			input, inputOK := group.InputAt(inputIndex)
			if !inputOK || !collectInput(input) {
				return runtimeReindexes{}, false
			}
		}
		if input, inputOK := group.EnvironmentInput(); inputOK {
			if !collectInput(input) {
				return runtimeReindexes{}, false
			}
		}
	}
	for index := 0; index < graph.EnvironmentEdgeTotal(); index++ {
		edge, edgeOK := graph.EnvironmentEdgeAtIndex(index)
		if !edgeOK || !collectInput(edge.Input()) {
			return runtimeReindexes{}, false
		}
	}
	for index := 0; index < graph.FactorEdgeTotal(); index++ {
		edge, edgeOK := graph.FactorEdgeAtIndex(index)
		if !edgeOK || !collectInput(edge.Input()) {
			return runtimeReindexes{}, false
		}
	}
	boundScopes, ok := sealRuntimeScopes(runtime, sources, decisions)
	if !ok {
		return runtimeReindexes{}, false
	}
	boundPlans := make(map[composition.Key]carrier.ReindexPlan, len(reindexes))
	keys := make([]composition.Key, 0, len(reindexes))
	for key := range reindexes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return lessRuntimeKey(keys[left], keys[right]) })
	for _, key := range keys {
		plan, planOK := lowerRuntimeReindex(runtime, reindexes[key], boundScopes, decisions)
		if !planOK {
			return runtimeReindexes{}, false
		}
		boundPlans[key] = plan
	}
	return runtimeReindexes{scopes: boundScopes, plans: boundPlans, decisions: decisions}, true
}

func runtimeDecisionAtoms(graph *equation.Graph, manager *guard.Manager) (map[composition.Key]guard.Atom, bool) {
	if graph == nil || manager == nil {
		return nil, false
	}
	result := make(map[composition.Key]guard.Atom, graph.DecisionCount())
	for index := 0; index < graph.DecisionCount(); index++ {
		decision, decisionOK := graph.DecisionAt(index)
		atom, atomOK := manager.AtomAt(uint64(index))
		if !decisionOK || !decision.Available() || !atomOK || atom != guard.Atom(index+1) {
			return nil, false
		}
		if _, duplicate := result[decision.Key()]; duplicate {
			return nil, false
		}
		result[decision.Key()] = atom
	}
	if _, extra := manager.AtomAt(uint64(graph.DecisionCount())); extra {
		return nil, false
	}
	return result, true
}

func collectRuntimeScope(scopes map[composition.Key]equation.Scope, scope equation.Scope) bool {
	if !scope.Available() {
		return false
	}
	if prior, exists := scopes[scope.Key()]; exists {
		return prior.Key() == scope.Key() && prior.Count() == scope.Count()
	}
	scopes[scope.Key()] = scope
	return true
}

func sealRuntimeScopes(runtime *carrier.Composition, source map[composition.Key]equation.Scope, decisions map[composition.Key]guard.Atom) (map[composition.Key]carrier.Scope, bool) {
	keys := make([]composition.Key, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return lessRuntimeKey(keys[left], keys[right]) })
	result := make(map[composition.Key]carrier.Scope, len(keys))
	for _, key := range keys {
		scope := source[key]
		atoms := make([]guard.Atom, scope.Count())
		for index := range atoms {
			decision, decisionOK := scope.At(index)
			atom, atomOK := decisions[decision.Key()]
			if !decisionOK || !decision.Available() || !atomOK || index > 0 && atoms[index-1] >= atom {
				return nil, false
			}
			atoms[index] = atom
		}
		bound, ok := runtime.SealScope(atoms)
		if !ok || !bound.Valid() {
			return nil, false
		}
		result[key] = bound
	}
	return result, true
}

func lowerRuntimeReindex(runtime *carrier.Composition, reindex equation.Reindex, scopes map[composition.Key]carrier.Scope, decisions map[composition.Key]guard.Atom) (carrier.ReindexPlan, bool) {
	if runtime == nil || !reindex.Available() || reindex.Count() != reindex.Source().Count() {
		return carrier.ReindexPlan{}, false
	}
	source, sourceOK := scopes[reindex.Source().Key()]
	target, targetOK := scopes[reindex.Target().Key()]
	if !sourceOK || !targetOK || !source.Valid() || !target.Valid() {
		return carrier.ReindexPlan{}, false
	}
	builder, ok := runtime.NewReindex(source, target)
	if !ok {
		return carrier.ReindexPlan{}, false
	}
	for index := 0; index < reindex.Count(); index++ {
		mapping, mappingOK := reindex.At(index)
		expected, expectedOK := reindex.Source().At(index)
		atom, atomOK := decisions[mapping.Source.Key()]
		if !mappingOK || !expectedOK || mapping.Source != expected || !atomOK {
			return carrier.ReindexPlan{}, false
		}
		switch mapping.Disposition {
		case equation.DecisionIdentity:
			if mapping.Target != mapping.Source || !builder.Identity(atom) {
				return carrier.ReindexPlan{}, false
			}
		case equation.DecisionForget:
			if mapping.Target.Available() || mapping.Expr.Available() || !builder.Forget(atom) {
				return carrier.ReindexPlan{}, false
			}
		case equation.DecisionRename:
			expression, expressionOK := runtimeDecisionExpression(runtime, target, mapping.Target, decisions)
			if mapping.Target == mapping.Source || mapping.Expr.Available() || !expressionOK || !builder.Set(atom, expression) {
				return carrier.ReindexPlan{}, false
			}
		case equation.DecisionSubstitute:
			expression, expressionOK := runtimeExpr(runtime, target, mapping.Expr, decisions)
			if mapping.Target.Available() || !expressionOK || !builder.Set(atom, expression) {
				return carrier.ReindexPlan{}, false
			}
		default:
			return carrier.ReindexPlan{}, false
		}
	}
	return builder.Seal()
}

func runtimeDecisionExpression(runtime *carrier.Composition, target carrier.Scope, decision equation.Decision, decisions map[composition.Key]guard.Atom) (carrier.Expr, bool) {
	if runtime == nil || !target.Valid() || !decision.Available() {
		return carrier.Expr{}, false
	}
	atom, ok := decisions[decision.Key()]
	if !ok {
		return carrier.Expr{}, false
	}
	work := support.New(runtime.Guards())
	if work == nil {
		return carrier.Expr{}, false
	}
	mask, ok := work.Literal(atom, true)
	if !ok || !work.Seal() {
		work.Discard()
		return carrier.Expr{}, false
	}
	return target.Expr(mask)
}

// runtimeExpr reconstructs the retained equation ROBDD exactly into the one
// carrier BDD. Every declared node is checked (including unreachable rows),
// terminals are the only 0/1 references, and child ordering is re-proved from
// the graph-derived dense atom order before publication.
func runtimeExpr(runtime *carrier.Composition, target carrier.Scope, expression equation.Expr, decisions map[composition.Key]guard.Atom) (carrier.Expr, bool) {
	mask, ok := runtimeFormula(runtime, target, expression, decisions)
	if !ok {
		return carrier.Expr{}, false
	}
	return target.Expr(mask)
}

// runtimeFormula reconstructs one retained equation formula into the sole
// carrier support representation.  It is shared by Reindex substitutions,
// Site initialization, and both sides of every Input transport so the engine
// cannot accidentally give pre/post conditions a second Boolean carrier.
func runtimeFormula(runtime *carrier.Composition, target carrier.Scope, expression equation.Expr, decisions map[composition.Key]guard.Atom) (support.Mask, bool) {
	if runtime == nil || !target.Valid() || !expression.Available() {
		return support.Mask{}, false
	}
	work := support.New(runtime.Guards())
	if work == nil {
		return support.Mask{}, false
	}
	values := make([]support.Mask, expression.NodeCount()+2)
	values[0], values[1] = work.False(), work.True()
	valid := true
	for index := 0; index < expression.NodeCount(); index++ {
		decision, low, high, nodeOK := expression.NodeAt(index)
		atom, atomOK := decisions[decision.Key()]
		if !nodeOK || !decision.Available() || !atomOK || low > uint32(expression.NodeCount()+1) || high > uint32(expression.NodeCount()+1) || low == high {
			valid = false
			break
		}
		for _, child := range []uint32{low, high} {
			if child >= 2 {
				if child >= uint32(index+2) {
					valid = false
					break
				}
				childDecision, _, _, childOK := expression.NodeAt(int(child - 2))
				childAtom, childAtomOK := decisions[childDecision.Key()]
				if !childOK || !childDecision.Available() || !childAtomOK || atom >= childAtom {
					valid = false
				}
			}
		}
		if !valid {
			break
		}
		values[index+2], valid = work.Decision(atom, values[low], values[high])
		if !valid {
			break
		}
	}
	root, rootOK := expression.Root()
	if !valid || !rootOK || root > uint32(expression.NodeCount()+1) || !work.Seal() {
		work.Discard()
		return support.Mask{}, false
	}
	rootMask := values[root]
	if _, scoped := target.Expr(rootMask); !scoped {
		return support.Mask{}, false
	}
	return rootMask, true
}
