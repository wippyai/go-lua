// runtime_member.go binds the Rule and activation members and proves their schema geometry.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// runtimeMember is one typed attachment for a graph-owned RuleMember. It has
// no independent schedule, candidate, or publication path.
type runtimeMember interface {
	member() equation.RuleMember
	outputSlot() (shape.Slot, bool)
	factorKey() (composition.Key, bool)
	// Slice results are binding-owned and immutable after attachment; assembly
	// only reads them while sealing the private runtime plan.
	carries() []int
	initialReads() []demand.Observation
	dynamicReads() []demand.DynamicRead
	targets() []carrier.Target
	carryTargets() []carrier.Target
	narrowTargets() []carrier.Target
	// routeScope is the Factor-owned authority for a route write/carry. The
	// member retains only this identity and the narrow capability bit; the
	// route target universe is expanded by bindRuntimeRegions once per
	// (active Region, Factor), never once per member or Group.
	routeScope() runtimeFactor
	routeNarrow() bool
	writesOutput() bool
	execute(*carrier.Work, carrier.RuleContributionBase, []carrier.State, support.Mask) memberResult
}

type memberResult struct {
	patch       carrier.Patch
	wrote       bool
	retained    support.Mask
	hasSupport  bool
	activations []equation.AcceptedMember
	reads       []demand.Observation
	boundary    solveBoundary
	valid       bool
}

type boundRuleMember[V, O any] struct {
	value          equation.RuleMember
	rule           *boundRule[V, O]
	slot           shape.Slot
	hasSlot        bool
	carry          []int
	outputTargets  []carrier.Target
	allTargets     []carrier.Target
	narrowEligible bool
	routeOwner     runtimeFactor
	outputWrite    bool
}

func (bound *boundRuleMember[V, O]) member() equation.RuleMember { return bound.value }

func (bound *boundRuleMember[V, O]) outputSlot() (shape.Slot, bool) {
	return bound.slot, bound != nil && bound.rule != nil && bound.hasSlot
}

func (bound *boundRuleMember[V, O]) factorKey() (composition.Key, bool) {
	if bound == nil || bound.rule == nil || bound.rule.proof == nil || !bound.rule.proof.valid() || !bound.rule.proof.output.Available() {
		return composition.Key{}, false
	}
	return bound.rule.proof.output, true
}

func (bound *boundRuleMember[V, O]) carries() []int {
	if bound == nil {
		return nil
	}
	return bound.carry
}

func (bound *boundRuleMember[V, O]) initialReads() []demand.Observation {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.initialReads()
}

func (bound *boundRuleMember[V, O]) dynamicReads() []demand.DynamicRead {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.dynamicReads()
}

func (bound *boundRuleMember[V, O]) targets() []carrier.Target {
	if bound == nil {
		return nil
	}
	return bound.outputTargets
}

func (bound *boundRuleMember[V, O]) carryTargets() []carrier.Target {
	if bound == nil {
		return nil
	}
	return bound.allTargets
}

func (bound *boundRuleMember[V, O]) narrowTargets() []carrier.Target {
	if bound == nil || !bound.narrowEligible {
		return nil
	}
	if len(bound.carry) != 0 {
		return bound.allTargets
	}
	return bound.outputTargets
}

func (bound *boundRuleMember[V, O]) routeScope() runtimeFactor {
	if bound == nil {
		return nil
	}
	return bound.routeOwner
}

func (bound *boundRuleMember[V, O]) routeNarrow() bool {
	return bound != nil && bound.routeOwner != nil && bound.narrowEligible
}

func (bound *boundRuleMember[V, O]) writesOutput() bool { return bound != nil && bound.outputWrite }

func (bound *boundRuleMember[V, O]) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if bound == nil || bound.rule == nil {
		return memberResult{boundary: refused(SolveFailureFamilyExecution, "preflight")}
	}
	patch, reads, wrote, ok, boundary := bound.rule.execute(work, base, inputs, within)
	return memberResult{patch: patch, wrote: wrote, reads: reads, boundary: boundary, valid: ok}
}

// bindSchemaRuleMember attaches the receipt-native zero-read Rule lane to the
// existing boundRule/Access executor. It intentionally has no declaration Rule or
// declaration argument: the private ruleRuntimeProof is the sole identity.
type schemaRuleMemberGeometry interface {
	Rule() composition.Key
	OperandFamily() composition.Key
	ReadCount() int
	WriteCount() int
	ActivationMember() (equation.Member, bool)
	WriteAt(int) (equation.Surface, bool)
	WriteRouteRead(int) (uint64, bool)
}

func exactSchemaRuleMemberGeometry(proof *ruleRuntimeProof, member schemaRuleMemberGeometry) (equation.Surface, bool) {
	if proof == nil || !proof.valid() || member == nil || member.Rule() != proof.semantic || member.OperandFamily() != proof.operandFamily || uint64(member.ReadCount()) != proof.reads || member.WriteCount() != 1 {
		return equation.Surface{}, false
	}
	if _, dynamic := member.ActivationMember(); dynamic {
		return equation.Surface{}, false
	}
	surface, surfaceOK := member.WriteAt(0)
	route, routeOK := member.WriteRouteRead(0)
	if !surfaceOK || surface.Factor != proof.output || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeStrong || surface.Local == 0 || !routeOK || route != 0 {
		return equation.Surface{}, false
	}
	return surface, true
}

func routeSchemaRuleMemberGeometry(proof *ruleRuntimeProof, member schemaRuleMemberGeometry) (equation.Surface, uint64, bool) {
	if proof == nil || !proof.valid() || proof.routeWrite == nil || !proof.routeWrite.Valid() || member == nil || member.Rule() != proof.semantic || member.OperandFamily() != proof.operandFamily || uint64(member.ReadCount()) != proof.reads || member.WriteCount() != 1 {
		return equation.Surface{}, 0, false
	}
	if _, dynamic := member.ActivationMember(); dynamic {
		return equation.Surface{}, 0, false
	}
	surface, surfaceOK := member.WriteAt(0)
	route, routeOK := member.WriteRouteRead(0)
	if !surfaceOK || surface.Factor != proof.output || surface.Form != equation.SurfaceWriteRoute || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() || !routeOK || route != proof.routeWrite.read+1 {
		return equation.Surface{}, 0, false
	}
	return surface, route, true
}

func bindSchemaRuleMember[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O], member equation.RuleMember, operand O, output runtimeFactor, factors map[composition.Key]runtimeFactor) (*boundRuleMember[V, O], bool) {
	if implementation == nil || !implementation.receipt.valid() || member.Key() == (composition.Key{}) || !member.Occurrence().Available() || output == nil || factors == nil {
		return nil, false
	}
	receipt := implementation.receipt
	proof := receipt.proof
	hot := receipt.cell.impl
	if proof == nil || hot == nil {
		return nil, false
	}
	shape, shapeOK := proof.schema.ruleShapeAt(proof.ordinal)
	if !shapeOK || shape.OutputKind != composition.FactorOutput || shape.CarryCount > 1 || shape.WriteCount != 1 || uint64(len(hot.reads)) != shape.ReadCount || shape.CarryCount == 0 && hot.carry != nil || shape.CarryCount == 1 && hot.carry == nil {
		return nil, false
	}
	boundOutput, outputOK := output.(*boundFactor[K, V])
	if !outputOK || boundOutput == nil || !boundOutput.receipt.valid() || boundOutput.receipt.state != receipt.state || boundOutput.receipt.authority != receipt.authority || boundOutput.receipt.schema != receipt.output.schema || boundOutput.receipt.ordinal != receipt.output.ordinal || boundOutput.receipt.semantic != receipt.output.semantic || boundOutput.receipt.algebra != receipt.output.algebra || boundOutput.receipt.semantic != shape.Output {
		return nil, false
	}
	surface, memberOK := exactSchemaRuleMemberGeometry(proof, member)
	_, routeRead, routeOK := routeSchemaRuleMemberGeometry(proof, member)
	if !memberOK && !routeOK {
		return nil, false
	}
	var target carrier.Target
	var targetProof ruleTargetProof
	if memberOK {
		var targetOK, targetProofOK bool
		target, targetOK = output.writeTarget(surface)
		targetProof, targetProofOK = newRuleTargetReceiptProof(boundOutput.receipt, surface)
		if !targetOK || !targetProofOK {
			return nil, false
		}
	} else if !output.hasRouteUniverse() {
		return nil, false
	}
	canonical, content, contentOK := hot.operandContent(operand)
	if !contentOK || content == [32]byte{} || !member.Operand().Available() {
		return nil, false
	}
	expected, expectedOK := operandEntityForContent(content)
	entity := OperandEntity{key: member.Operand().Entity()}
	if !expectedOK || entity.key != expected {
		return nil, false
	}
	anchor, anchorOK := semanticKeyFromComposition(member.Key())
	if !anchorOK {
		return nil, false
	}
	bound := &boundRule[V, O]{proof: proof, admission: hot.admission, anchor: anchor, operandContent: content, transfer: hot.transfer, operand: canonical}
	projection := &outputRuntime{writes: make([]outputWriteRuntime, 1)}
	if memberOK {
		projection.writes[0] = outputWriteRuntime{direct: target, directID: targetProof}
	} else {
		projection.writes[0] = outputWriteRuntime{routeRead: routeRead}
	}
	carryIndexes := []int(nil)
	allTargets := []carrier.Target(nil)
	if memberOK {
		allTargets = append(allTargets, target)
	}
	if hot.carry != nil {
		carryShape, carryOK := hot.carry.shape()
		if !carryOK || carryShape.Factor != shape.Output || carryShape.Input >= shape.Inputs || output.carryRouteScopeFor(member) {
			return nil, false
		}
		if carryShape.Input > uint64(^uint(0)>>1) {
			return nil, false
		}
		carryTargets, targetsOK := output.carryTargetsFor(member)
		if !targetsOK {
			return nil, false
		}
		for _, carryTarget := range carryTargets {
			allTargets = appendUniqueTarget(allTargets, carryTarget)
		}
		carryIndexes = []int{int(carryShape.Input)}
		if carryShape.Transform.Available() {
			if hot.carry.apply == nil {
				return nil, false
			}
			carrySemantic, carrySemanticOK := semanticKeyFromComposition(carryShape.Transform)
			if !carrySemanticOK {
				return nil, false
			}
			bound.carrySemantic = carrySemantic
			bound.carryTargets = carryTargets
			bound.carryApply = func(value V) (V, bool) {
				return hot.carry.apply(operand, value)
			}
		}
	}
	access, accessOK := newTypedOutputAccess(boundOutput, bound, projection)
	if !accessOK {
		return nil, false
	}
	bound.output = access
	for _, read := range hot.reads {
		if read == nil || !read.bind(bound, member, factors) {
			return nil, false
		}
	}
	if routeOK {
		bound.routeScope = output
	}
	slot, slotOK := output.runtimeSlot()
	if !slotOK {
		return nil, false
	}
	outputTargets := []carrier.Target(nil)
	if memberOK {
		outputTargets = []carrier.Target{target}
	}
	return &boundRuleMember[V, O]{rule: bound, slot: slot, hasSlot: true, carry: carryIndexes, outputTargets: outputTargets, allTargets: allTargets, narrowEligible: output.supports(carrier.Narrow), routeOwner: bound.routeScope, outputWrite: true, value: member}, true
}

// boundActivationMember is an output-free member in the same Group
// transaction as Factors and support pruning.  Its selection is returned to
// the executor; it has no second evaluation or publication route.
type boundActivationMember struct {
	value equation.RuleMember
	rule  *compiledActivationRule
}

func (bound *boundActivationMember) member() equation.RuleMember { return bound.value }

func (bound *boundActivationMember) outputSlot() (shape.Slot, bool) { return 0, false }

func (bound *boundActivationMember) factorKey() (composition.Key, bool) {
	return composition.Key{}, false
}

func (bound *boundActivationMember) carries() []int { return nil }

func (bound *boundActivationMember) initialReads() []demand.Observation {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.initialReads()
}

func (bound *boundActivationMember) dynamicReads() []demand.DynamicRead {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.dynamicReads()
}

func (bound *boundActivationMember) targets() []carrier.Target { return nil }

func (bound *boundActivationMember) carryTargets() []carrier.Target { return nil }

func (bound *boundActivationMember) narrowTargets() []carrier.Target { return nil }

func (bound *boundActivationMember) routeScope() runtimeFactor { return nil }

func (bound *boundActivationMember) routeNarrow() bool { return false }

func (bound *boundActivationMember) writesOutput() bool { return false }

func (bound *boundActivationMember) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if bound == nil || bound.rule == nil {
		return memberResult{}
	}
	selected, reads, ok, phase := bound.rule.execute(work, base, inputs, within)
	return memberResult{activations: selected, reads: reads, boundary: phase, valid: ok}
}

// bindActivationMemberReceipt attaches one graph-owned activation Member to a
// receipt-native activation implementation. The receipt compiler performs
// the exact Schema/family/trigger checks; this adapter adds only the Member
// anchor and returns the same runtime member consumed by the existing epoch
// executor.
func bindActivationMemberReceipt(member equation.RuleMember, implementation *ActivationRuleImplementation, topology *equation.Topology, trigger composition.Key, graph *equation.Graph, factors map[composition.Key]runtimeFactor) (*boundActivationMember, bool) {
	if implementation == nil || !implementation.receipt.valid() || !member.Key().Available() || topology == nil || graph == nil ||
		!topology.OwnsComposition(implementation.receipt.proof.schema.cold) || !topology.OwnsGraph(graph) || !graph.OwnsMember(member) ||
		!trigger.Available() || trigger != member.Key() || member.Rule() != implementation.receipt.proof.semantic ||
		member.OperandFamily() != implementation.receipt.proof.operandFamily || uint64(member.ReadCount()) != implementation.receipt.proof.reads || member.WriteCount() != 0 || factors == nil {
		return nil, false
	}
	compiled, ok := compileActivationRuleReceipt(implementation, topology, trigger, graph)
	if !ok {
		return nil, false
	}
	hot := implementation.receipt.cell.impl
	if hot == nil || uint64(len(hot.reads)) != implementation.receipt.proof.reads {
		return nil, false
	}
	for _, read := range hot.reads {
		if read == nil || !read.bind(compiled, member, factors) {
			return nil, false
		}
	}
	if uint64(len(compiled.reads)) != implementation.receipt.proof.reads {
		return nil, false
	}
	anchor, anchorOK := semanticKeyFromComposition(member.Key())
	if !anchorOK {
		return nil, false
	}
	compiled.anchor = anchor
	return &boundActivationMember{value: member, rule: compiled}, true
}
