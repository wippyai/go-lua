// runtime_member.go binds the Rule and activation members and validates their schema geometry.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/lifetime"
	"github.com/wippyai/go-lua/analysis/identity"
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

// contextualRuntimeMember is the narrow engine-only extension for mounted
// activation rows. Ordinary rules retain the runtimeMember contract; an
// artifact epoch supplies the exact source Context ID only to members that
// can consume it. No implementation may infer context from module identity.
type contextualRuntimeMember interface {
	executeAt(*carrier.Work, carrier.RuleContributionBase, []carrier.State, support.Mask, identity.ContentID) memberResult
}

type memberResult struct {
	patch       carrier.Patch
	wrote       bool
	activations []equation.AcceptedMember
	reads       []demand.Observation
	boundary    solveBoundary
	valid       bool
}

type boundRuleMember[V, O any] struct {
	value equation.RuleMember
	// cell+ordinal are the canonical sealed Rule owner identity. Geometry is
	// validated during construction; execution only compares this address.
	cell               schemaRuleBindingCell
	ordinal            uint64
	outputKey          composition.Key
	expectedReadCount  uint64
	fold               func(Frame[V, O]) RuleResult[V]
	operand            O
	reads              []readRuntime
	output             outputAccess[V]
	routeTransform     bool
	carrySemantic      identity.SemanticKey
	transformedTargets []carrier.Target
	carryApply         func(V) (V, bool)
	nextEpoch          lifetime.GenerationSequence
	slot               shape.Slot
	hasSlot            bool
	carry              []int
	outputTargets      []carrier.Target
	allTargets         []carrier.Target
	narrowEligible     bool
	routeOwner         runtimeFactor
	outputWrite        bool
}

func (bound *boundRuleMember[V, O]) member() equation.RuleMember {
	if bound == nil {
		return equation.RuleMember{}
	}
	return bound.value
}

func (bound *boundRuleMember[V, O]) outputSlot() (shape.Slot, bool) {
	if bound == nil {
		return 0, false
	}
	return bound.slot, bound.cell != nil && bound.ordinal == bound.cell.schemaRuleOrdinal() && bound.hasSlot
}

func (bound *boundRuleMember[V, O]) factorKey() (composition.Key, bool) {
	if bound == nil || bound.cell == nil || bound.ordinal != bound.cell.schemaRuleOrdinal() || !bound.outputKey.Available() {
		return composition.Key{}, false
	}
	return bound.outputKey, true
}

func (bound *boundRuleMember[V, O]) carries() []int {
	if bound == nil {
		return nil
	}
	return bound.carry
}

func (bound *boundRuleMember[V, O]) initialReads() []demand.Observation {
	return bound.initialRuleReads()
}

func (bound *boundRuleMember[V, O]) dynamicReads() []demand.DynamicRead {
	return bound.dynamicRuleReads()
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
	if bound == nil {
		return memberResult{boundary: refused(SolveFailureFamilyExecution, "preflight")}
	}
	patch, reads, wrote, ok, boundary := bound.executeRule(work, base, inputs, within)
	return memberResult{patch: patch, wrote: wrote, reads: reads, boundary: boundary, valid: ok}
}

// bindSchemaRuleMember attaches a sealed Rule lane to the canonical
// boundRuleMember/Access executor. Geometry is checked against the sealed
// owner cell and its canonical ordinal while the runtime member is built.
type schemaRuleMemberGeometry interface {
	Rule() composition.Key
	OperandFamily() composition.Key
	ReadCount() int
	WriteCount() int
	ActivationMember() (equation.Member, bool)
	WriteAt(int) (equation.Surface, bool)
	WriteRouteRead(int) (uint64, bool)
}

func exactSchemaRuleMemberGeometry[K ~uint32 | ~uint64, V, O any](cell *schemaRuleBindingCellImpl[K, V, O], ordinal uint64, member schemaRuleMemberGeometry) (equation.Surface, bool) {
	if cell == nil || cell.ordinal != ordinal || !cell.sealedRuleComplete() || member == nil || member.Rule() != cell.impl.ruleSemantic || member.OperandFamily() != cell.impl.operandFamily || uint64(member.ReadCount()) != uint64(len(cell.impl.reads)) || member.WriteCount() != 1 || cell.impl.writeMode != directRuleWriteExact {
		return equation.Surface{}, false
	}
	if _, dynamic := member.ActivationMember(); dynamic {
		return equation.Surface{}, false
	}
	surface, surfaceOK := member.WriteAt(0)
	route, routeOK := member.WriteRouteRead(0)
	if !surfaceOK || surface.Factor != cell.impl.output.schemaFactorSemanticKey() || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeStrong || surface.Local == 0 || !routeOK || route != 0 {
		return equation.Surface{}, false
	}
	return surface, true
}

func routeSchemaRuleMemberGeometry[K ~uint32 | ~uint64, V, O any](cell *schemaRuleBindingCellImpl[K, V, O], ordinal uint64, member schemaRuleMemberGeometry) (equation.Surface, uint64, bool) {
	if cell == nil || cell.ordinal != ordinal || !cell.sealedRuleComplete() || member == nil || member.Rule() != cell.impl.ruleSemantic || member.OperandFamily() != cell.impl.operandFamily || uint64(member.ReadCount()) != uint64(len(cell.impl.reads)) || member.WriteCount() != 1 || cell.impl.writeMode != directRuleWriteRoute || cell.impl.routeRead == 0 || cell.impl.routeRead > uint64(len(cell.impl.reads)) {
		return equation.Surface{}, 0, false
	}
	read := cell.impl.routeRead - 1
	row := cell.schemaRuleReadAt(read)
	if row == nil || row.owner != cell || row.ownerOrdinal != ordinal || row.readOrdinal != read || row.kind != composition.ReadSelect || row.factor != cell.impl.output.schemaFactorSemanticKey() || row.semantic != row.factor || row.normalizer.Available() || len(row.dependencies) == 0 {
		return equation.Surface{}, 0, false
	}
	if _, dynamic := member.ActivationMember(); dynamic {
		return equation.Surface{}, 0, false
	}
	surface, surfaceOK := member.WriteAt(0)
	route, routeOK := member.WriteRouteRead(0)
	if !surfaceOK || surface.Factor != cell.impl.output.schemaFactorSemanticKey() || surface.Form != equation.SurfaceWriteRoute || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() || !routeOK || route != cell.impl.routeRead {
		return equation.Surface{}, 0, false
	}
	return surface, route, true
}

// bindSealedRuleCellMember binds one canonical sealed Rule cell against a
// frozen factor plane. The plane fixes the authority the cell was sealed under
// and the Factor its output names; no typed implementation handle participates.
func bindSealedRuleCellMember[K ~uint32 | ~uint64, V, O any](plane *programPlane, cell *schemaRuleBindingCellImpl[K, V, O], member equation.RuleMember, declared declaredRuleOperand) (runtimeMember, bool) {
	if plane == nil || !plane.frozen || plane.runtime == nil || plane.runtime.graph == nil || plane.carrier == nil || plane.byKey == nil {
		return nil, false
	}
	if cell == nil || !cell.sealedRuleComplete() || cell.state != plane.runtime.state || cell.state.authority != plane.runtime.authority {
		return nil, false
	}
	if !plane.runtime.graph.OwnsMember(member) || !member.Key().Available() {
		return nil, false
	}
	if cell.impl == nil || cell.impl.output == nil {
		return nil, false
	}
	output, present := plane.byKey[cell.impl.output.schemaFactorSemanticKey()]
	if !present || output == nil {
		return nil, false
	}
	if !declared.Available() {
		return nil, false
	}
	canonical, typed := declared.value.(O)
	if !typed || declared.digest == [32]byte{} || !member.Operand().Available() {
		return nil, false
	}
	expected, expectedOK := operandEntityForContent(declared.digest)
	if !expectedOK || member.Operand().Entity() != expected {
		return nil, false
	}
	row, ok := bindSchemaRuleCellMember(cell, member, canonical, declared.digest, output, plane.byKey)
	if !ok || row == nil || row.member().Key() != member.Key() {
		return nil, false
	}
	return row, true
}

func bindSchemaRuleCellMember[K ~uint32 | ~uint64, V, O any](cell *schemaRuleBindingCellImpl[K, V, O], member equation.RuleMember, operand O, content [32]byte, output runtimeFactor, factors map[composition.Key]runtimeFactor) (*boundRuleMember[V, O], bool) {
	if cell == nil || member.Key() == (composition.Key{}) || !member.Occurrence().Available() || output == nil || factors == nil {
		return nil, false
	}
	hot := cell.impl
	if hot == nil {
		return nil, false
	}
	if !cell.sealedRuleComplete() || hot.writeMode == 0 || hot.output == nil || hot.carryPresent && hot.carryApply == nil && hot.carryTransform.Available() {
		return nil, false
	}
	outputKey := hot.output.schemaFactorSemanticKey()
	boundOutput, outputOK := output.(*boundFactor[K, V])
	if !outputOK || boundOutput == nil || boundOutput.implementation == nil || !factorRowAvailable(boundOutput.implementation.row) || boundOutput.implementation.row != cell.impl.output || boundOutput.implementation.row.schemaFactorSemanticKey() != outputKey {
		return nil, false
	}
	surface, memberOK := exactSchemaRuleMemberGeometry(cell, cell.ordinal, member)
	_, routeRead, routeOK := routeSchemaRuleMemberGeometry(cell, cell.ordinal, member)
	if !memberOK && !routeOK {
		return nil, false
	}
	var target carrier.Target
	var targetRow schemaFactorBinding
	var targetRaw uint64
	if memberOK {
		var targetOK, targetAddressOK bool
		target, targetOK = output.writeTarget(surface)
		targetRaw, targetAddressOK = exactWriteLocal(boundOutput.implementation.row, surface)
		if !targetOK || !targetAddressOK {
			return nil, false
		}
		targetRow = boundOutput.implementation.row
	} else if !output.hasRouteUniverse() {
		return nil, false
	}
	if content == [32]byte{} || !member.Operand().Available() {
		return nil, false
	}
	expected, expectedOK := operandEntityForContent(content)
	entity := operandEntity{key: member.Operand().Entity()}
	if !expectedOK || entity.key != expected {
		return nil, false
	}
	bound := &boundRuleMember[V, O]{
		value:             member,
		cell:              cell,
		ordinal:           cell.ordinal,
		outputKey:         outputKey,
		expectedReadCount: uint64(len(hot.reads)),
		fold:              hot.fold,
		operand:           operand,
	}
	projection := &outputRuntime{writes: make([]outputWriteRuntime, 1)}
	if memberOK {
		projection.writes[0] = outputWriteRuntime{direct: target, directRow: targetRow, directRaw: targetRaw}
	} else {
		projection.writes[0] = outputWriteRuntime{routeRead: routeRead}
	}
	carryIndexes := []int(nil)
	allTargets := []carrier.Target(nil)
	if memberOK {
		allTargets = append(allTargets, target)
	}
	carryRouteScope := false
	if hot.carryPresent {
		// A carry closure that reaches a route write has no finite exact target
		// vector: a routed write names its concrete target only at execution.
		// The member claims the output Factor's route universe on top of the
		// exact closure, the same authority a route-writing member claims; the
		// Region seal expands it once per (Region, Factor).
		carryRouteScope = output.carryRouteScopeFor(member)
		if carryRouteScope && !output.hasRouteUniverse() {
			return nil, false
		}
		carryTargets, targetsOK := output.carryTargetsFor(member)
		if !targetsOK {
			return nil, false
		}
		for _, carryTarget := range carryTargets {
			allTargets = appendUniqueTarget(allTargets, carryTarget)
		}
		carryIndexes = []int{int(hot.carryInput)}
		if hot.carryTransform.Available() {
			if hot.carryApply == nil {
				return nil, false
			}
			carrySemantic, carrySemanticOK := semanticKeyFromComposition(hot.carryTransform)
			if !carrySemanticOK {
				return nil, false
			}
			bound.carrySemantic = carrySemantic
			bound.transformedTargets = carryTargets
			// The transformed carry maps the route universe through the same
			// terminal as its exact closure, so the Factor-owned route
			// transform closure joins the member's carry transform.
			bound.routeTransform = carryRouteScope
			bound.carryApply = func(value V) (V, bool) {
				return hot.carryApply(operand, value)
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
	if routeOK || carryRouteScope {
		bound.routeOwner = output
	}
	slot, slotOK := output.runtimeSlot()
	if !slotOK {
		return nil, false
	}
	outputTargets := []carrier.Target(nil)
	if memberOK {
		outputTargets = []carrier.Target{target}
	}
	bound.slot = slot
	bound.hasSlot = true
	bound.carry = carryIndexes
	bound.outputTargets = outputTargets
	bound.allTargets = allTargets
	bound.narrowEligible = output.supports(carrier.Narrow)
	bound.outputWrite = true
	return bound, true
}

// boundActivationMember is an output-free member in the same Group
// transaction as Factors. Its selection is returned to the executor; it has
// no second evaluation or publication route.
type boundActivationMember struct {
	value equation.RuleMember
	rule  *compiledActivationRule
}

func (bound *boundActivationMember) member() equation.RuleMember {
	if bound == nil {
		return equation.RuleMember{}
	}
	return bound.value
}

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
	return bound.executeAt(work, base, inputs, within, identity.ContentID{})
}

func (bound *boundActivationMember) executeAt(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask, fromContextID identity.ContentID) memberResult {
	if bound == nil || bound.rule == nil {
		return memberResult{}
	}
	selected, reads, ok, phase := bound.rule.executeAt(work, base, inputs, within, fromContextID)
	return memberResult{activations: selected, reads: reads, boundary: phase, valid: ok}
}

// bindActivationMember attaches one graph-owned activation Member to a
// sealed activation implementation. The Schema binding performs
// the exact Schema/family/trigger checks; this bind adds only the Member anchor
// and returns the runtime member consumed by the epoch executor.
func bindActivationMember(member equation.RuleMember, implementation *ActivationRuleImplementation, topology *equation.Topology, trigger composition.Key, graph *equation.Graph, factors map[composition.Key]runtimeFactor) (*boundActivationMember, bool) {
	if implementation == nil {
		return nil, false
	}
	cell, cellOK := implementation.sealedActivationCell()
	if !cellOK {
		return nil, false
	}
	return bindActivationCellMember(member, cell, implementation.ordinal, topology, trigger, graph, factors)
}

func bindActivationCellMember(member equation.RuleMember, cell *schemaActivationRuleBindingCell, ordinal uint64, topology *equation.Topology, trigger composition.Key, graph *equation.Graph, factors map[composition.Key]runtimeFactor) (*boundActivationMember, bool) {
	if cell == nil || !cell.schemaRuleComplete() || !member.Key().Available() || topology == nil || graph == nil ||
		!topology.OwnsComposition(cell.schema.cold) || !topology.OwnsGraph(graph) || !graph.OwnsMember(member) ||
		!trigger.Available() || trigger != member.Key() || factors == nil {
		return nil, false
	}
	shape, shapeOK := cell.schema.ruleShapeAt(ordinal)
	semantic := cell.schema.ruleSemanticAt(ordinal)
	if !shapeOK || !semantic.Available() || member.Rule() != semantic ||
		member.OperandFamily() != shape.OperandFamily || uint64(member.ReadCount()) != shape.ReadCount || member.WriteCount() != 0 {
		return nil, false
	}
	compiled, ok := compileActivationCellRule(cell, ordinal, topology, trigger, graph)
	if !ok {
		return nil, false
	}
	hot := cell.impl
	if hot == nil || uint64(len(hot.reads)) != shape.ReadCount {
		return nil, false
	}
	for _, read := range hot.reads {
		if read == nil || !read.bind(compiled, member, factors) {
			return nil, false
		}
	}
	if uint64(len(compiled.reads)) != shape.ReadCount {
		return nil, false
	}
	anchor, anchorOK := semanticKeyFromComposition(member.Key())
	if !anchorOK {
		return nil, false
	}
	compiled.anchor = anchor
	return &boundActivationMember{value: member, rule: compiled}, true
}
