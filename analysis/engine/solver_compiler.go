package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// compileSolver is the one lowering for both the initial empty relation set
// and every later activation revision. The sealed equation topology is the
// sole structural authority; activation contributes only accepted Members and
// immutable evidence records.
func compileSolver(cold *Composition, topology *equation.Topology, operands *topologyOperands) (*Solver, bool) {
	compiler := coldSolverCompiler{cold: cold, topology: topology, operands: operands}
	runtime, _, ok := compiler.compile(nil)
	if !ok || runtime == nil {
		return nil, false
	}
	return &Solver{runtime: runtime, compiler: compiler}, true
}

// coldSolverCompiler retains only compilation scratch.  It is deliberately
// not a runtime object: before Solver escapes its Graph catalog and Factor
// surface maps have been released.
type coldSolverCompiler struct {
	cold     *Composition
	topology *equation.Topology
	operands *topologyOperands
}

func (compiler coldSolverCompiler) compile(accepted []equation.AcceptedMember) (*solverRuntime, SolveFailurePhase, bool) {
	if compiler.cold == nil || !compiler.cold.Sealed() || compiler.cold.coldComposition() == nil || compiler.topology == nil || compiler.operands == nil || !compiler.topology.OwnsComposition(compiler.cold.coldComposition()) || !validAcceptedActivations(compiler.topology, accepted) {
		return nil, SolveFailurePhaseCompileValidation, false
	}
	graph, ok := compiler.topology.Graph(accepted)
	if !ok || graph == nil || !graph.OwnsComposition(compiler.cold.coldComposition()) {
		return nil, SolveFailurePhaseCompileValidation, false
	}
	revision, ok := compiler.operands.revision(graph)
	if !ok || revision == nil {
		return nil, SolveFailurePhaseCompileOperandRevision, false
	}
	binding, ok := newRuntimeBinding(compiler.cold, graph)
	if !ok || binding == nil {
		return nil, SolveFailurePhaseCompileComposition, false
	}
	factors, byKey, ok := compiler.bindFactors(binding)
	if !ok {
		return nil, SolveFailurePhaseCompileComposition, false
	}
	if !binding.freezeCatalog() {
		return nil, SolveFailurePhaseCompileComposition, false
	}
	prepared, ordered, ok := prepareRuntimeComposition(factors, binding.guards)
	if !ok || prepared == nil {
		return nil, SolveFailurePhaseCompileComposition, false
	}
	runtime, ok := prepared.Attach()
	if !ok || runtime == nil {
		return nil, SolveFailurePhaseCompileComposition, false
	}
	for _, factor := range ordered {
		preparer, preparable := factor.(interface{ prepareRouteTransformClosure() bool })
		if !preparable || !preparer.prepareRouteTransformClosure() {
			return nil, SolveFailurePhaseCompileComposition, false
		}
	}
	members, ok := compiler.bindMembers(graph, byKey, revision)
	if !ok {
		return nil, SolveFailurePhaseCompileMemberBinding, false
	}
	queries, ok := compiler.bindQueries(graph, byKey)
	if !ok {
		return nil, SolveFailurePhaseCompileQueryBinding, false
	}
	for _, factor := range ordered {
		if factor == nil {
			return nil, SolveFailurePhaseCompileComposition, false
		}
		factor.releaseColdBindings()
	}
	assembled, ok := assembleRuntime(compiler.cold, graph, runtime, byKey, members, queries)
	if !ok || assembled == nil {
		return nil, SolveFailurePhaseCompileRuntimeAssembly, false
	}
	assembled.factors = append([]runtimeFactor(nil), ordered...)
	assembled.topology = compiler.topology
	return assembled, SolveFailurePhaseNone, true
}

func validAcceptedActivations(topology *equation.Topology, accepted []equation.AcceptedMember) bool {
	if topology == nil {
		return false
	}
	// Graph/Revision repeat this fail-closed validation at the authority
	// boundary. The compiler keeps the same predicate here so an untrusted caller
	// cannot get as far as carrier allocation with a foreign Member.
	if _, ok := topology.Revision(accepted); !ok {
		return false
	}
	return true
}

func (compiler coldSolverCompiler) bindFactors(binding *runtimeBinding) ([]runtimeFactor, map[composition.Key]runtimeFactor, bool) {
	sealed := compiler.cold.coldComposition().Factors()
	indexed := make([]*factorSchema, len(sealed))
	for _, schema := range compiler.cold.factors {
		if schema == nil || !schema.bound || !validFactorBind(schema) || schema.bindIndex >= uint64(len(indexed)) || sealed[schema.bindIndex].Key != schema.semantic.compositionKey() || indexed[schema.bindIndex] != nil {
			return nil, nil, false
		}
		indexed[schema.bindIndex] = schema
	}
	factors := make([]runtimeFactor, len(indexed))
	byKey := make(map[composition.Key]runtimeFactor, len(indexed))
	for index, schema := range indexed {
		if schema == nil {
			return nil, nil, false
		}
		factor, ok := bindRuntimeFactor(schema, binding)
		if !ok || factor == nil || factor.semantic() != schema.semantic || factor.semantic().compositionKey() != sealed[index].Key {
			return nil, nil, false
		}
		key := sealed[index].Key
		if _, duplicate := byKey[key]; duplicate {
			return nil, nil, false
		}
		factors[index], byKey[key] = factor, factor
	}
	return factors, byKey, true
}

func (compiler coldSolverCompiler) bindMembers(graph *equation.Graph, factors map[composition.Key]runtimeFactor, operands *topologyOperandRevision) ([]runtimeMember, bool) {
	sealed := compiler.cold.coldComposition().Rules()
	indexed := make([]*ruleSchema, len(sealed))
	for _, schema := range compiler.cold.rules {
		if schema == nil || !schema.bound || !validRuleBind(schema) || schema.bindIndex >= uint64(len(indexed)) || sealed[schema.bindIndex].Key != schema.semantic.compositionKey() || indexed[schema.bindIndex] != nil {
			return nil, false
		}
		indexed[schema.bindIndex] = schema
	}
	rows := make([]runtimeMember, 0)
	seen := make(map[composition.Key]struct{})
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok || !graph.OwnsGroup(group) {
			return nil, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok || !graph.OwnsMember(member) || !member.Key().Available() || member.Rule().Available() == false {
				return nil, false
			}
			if _, duplicate := seen[member.Key()]; duplicate {
				return nil, false
			}
			ruleIndex, ok := compiler.cold.coldComposition().RuleIndex(member.Rule())
			if !ok || ruleIndex >= uint64(len(indexed)) || indexed[ruleIndex] == nil {
				return nil, false
			}
			trigger := composition.Key{}
			if indexed[ruleIndex].activation != nil {
				if indexed[ruleIndex].activation.family == nil {
					return nil, false
				}
				if _, bound := compiler.topology.ActivationBinding(member.Key(), indexed[ruleIndex].activation.family.semantic.compositionKey()); !bound {
					return nil, false
				}
				trigger = member.Key()
			}
			var row runtimeMember
			if indexed[ruleIndex].outputKind == ruleFactorOutput {
				row, ok = operands.bind(member, factors)
			} else {
				row, ok = indexed[ruleIndex].bind.bindStructuralMember(indexed[ruleIndex], member, factors, compiler.topology, trigger, graph)
			}
			if !ok || row == nil || row.member().Key() != member.Key() {
				return nil, false
			}
			seen[member.Key()] = struct{}{}
			rows = append(rows, row)
		}
	}
	return rows, len(rows) != 0
}

func (compiler coldSolverCompiler) bindQueries(graph *equation.Graph, factors map[composition.Key]runtimeFactor) ([]runtimeQuery, bool) {
	sealed := compiler.cold.coldComposition().Queries()
	indexed := make([]*querySchema, len(sealed))
	for _, query := range compiler.cold.queries {
		if query.schema == nil {
			return nil, false
		}
		schema := query.schema
		if schema == nil || !schema.bound || !validQueryBind(schema) || schema.bindIndex >= uint64(len(indexed)) || sealed[schema.bindIndex].Key != schema.semantic.compositionKey() || indexed[schema.bindIndex] != nil {
			return nil, false
		}
		indexed[schema.bindIndex] = schema
	}
	// Runtime rows are indexed by canonical equation Query order, not by cold
	// family.  A family may therefore bind several concrete observations while
	// retaining one typed schema authority.
	rows := make([]runtimeQuery, graph.QueryCount())
	for index := 0; index < graph.QueryCount(); index++ {
		identity, ok := graph.QueryAt(index)
		if !ok || !graph.OwnsQuery(identity) || !identity.Key().Available() || !identity.Family().Available() {
			return nil, false
		}
		queryIndex, ok := compiler.cold.coldComposition().QueryIndex(identity.Family())
		if !ok || queryIndex >= uint64(len(indexed)) || indexed[queryIndex] == nil {
			return nil, false
		}
		row, ok := indexed[queryIndex].bind.bindRuntimeQuery(indexed[queryIndex], identity, factors)
		if !ok || row == nil || row.query().Key() != identity.Key() || !graph.OwnsQuery(row.query()) {
			return nil, false
		}
		rows[index] = row
	}
	for _, row := range rows {
		if row == nil {
			return nil, false
		}
	}
	return rows, true
}
