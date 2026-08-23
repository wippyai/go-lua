package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// generatedMemberSpec is the immutable, engine-owned projection of one
// generated Rule row. Hot rows carry only compact descriptor and occurrence
// coordinates; capabilities and scratch are owned by the Factor and epoch.
type generatedMemberSpec struct {
	member equation.RuleMember

	outputSlot   shape.Slot
	hasSlot      bool
	factor       composition.Key
	carries      []int
	initial      []demand.Observation
	dynamic      []demand.DynamicRead
	targets      []carrier.Target
	carryTargets []carrier.Target
	narrow       []carrier.Target
	route        runtimeFactor
	routeNarrow  bool
	writes       bool

	factorOrdinal int32
	unit          carrier.Unit
	target        carrier.Target
	readInput     int // construction-only Plan coordinate; descriptor owns hot value
	rule          uint32
	candidate     uint32
	inputCount    int // construction-only Plan shape
	outputCount   int // construction-only Plan shape
}

// generatedMemberDeclaration is the compact construction result of one
// generated issuance. Surfaces are minted from the bound Factor cells and are
// retained only until the matching runtime Unit/Target are attached.
type generatedMemberDeclaration struct {
	cell         *generatedRuleCell
	operand      equation.Operand
	candidate    uint32
	readSurface  RuleReadSurface
	writeSurface ruleWriteSurface
}

// generatedMember is the generated execution arm of a sealed member row.
// There is intentionally no generated.Program or invocation scratch field.
// One runtimeProgram owns the immutable descriptor selected by Rule ordinal;
// the execution.Run below owns the one synchronous invocation transaction. The
// row retains only occurrence geometry and the Factor-issued exact Unit/strong
// Target.
type generatedMember struct {
	value equation.RuleMember

	slot                shape.Slot
	hasSlot             bool
	factor              composition.Key
	carryInputs         []int
	initial             []demand.Observation
	dynamic             []demand.DynamicRead
	targetRows          []carrier.Target
	carryTargetRows     []carrier.Target
	narrowRows          []carrier.Target
	route               runtimeFactor
	routeNarrowEligible bool
	writes              bool

	unit          carrier.Unit
	target        carrier.Target
	rule          uint32
	candidate     uint32
	invocationRef executioncatalog.Ref
}

var _ runtimeMemberGeometry = (*generatedMember)(nil)

// newGeneratedMember seals one generated member projection. A generated
// member owns at most one output axis: outputCount is therefore restricted to
// zero or one, and output geometry must agree with that cardinality.
func newGeneratedMember(spec generatedMemberSpec) (*generatedMember, bool) {
	if !spec.member.Key().Available() || spec.inputCount < 0 || spec.outputCount < 0 || spec.outputCount > 1 {
		return nil, false
	}
	if spec.factorOrdinal < 0 || spec.target == (carrier.Target{}) {
		return nil, false
	}
	if spec.inputCount == 0 {
		if spec.readInput != 0 || spec.unit != (carrier.Unit{}) {
			return nil, false
		}
	} else if spec.unit == (carrier.Unit{}) || spec.readInput < 0 || spec.readInput >= spec.inputCount {
		return nil, false
	}
	if spec.hasSlot != spec.writes || (spec.outputCount == 1) != spec.writes {
		return nil, false
	}
	if spec.hasSlot {
		if spec.outputSlot < 0 || !spec.factor.Available() || len(spec.targets) == 0 {
			return nil, false
		}
	} else if spec.outputSlot != 0 || spec.factor.Available() || len(spec.targets) != 0 || len(spec.carryTargets) != 0 || len(spec.narrow) != 0 || spec.route != nil || spec.routeNarrow {
		return nil, false
	}
	for _, carry := range spec.carries {
		if carry < 0 || carry >= spec.inputCount {
			return nil, false
		}
	}
	for _, read := range spec.initial {
		if read.Input >= uint64(spec.inputCount) {
			return nil, false
		}
	}
	for _, read := range spec.dynamic {
		if read.Input >= uint64(spec.inputCount) {
			return nil, false
		}
	}
	if spec.routeNarrow && spec.route == nil {
		return nil, false
	}
	if spec.target.Mode() != carrier.StrongTarget {
		return nil, false
	}
	if spec.inputCount != 0 && spec.unit.Kind() != carrier.ExactUnit {
		return nil, false
	}
	return &generatedMember{
		value:               spec.member,
		slot:                spec.outputSlot,
		hasSlot:             spec.hasSlot,
		factor:              spec.factor,
		carryInputs:         append([]int(nil), spec.carries...),
		initial:             append([]demand.Observation(nil), spec.initial...),
		dynamic:             append([]demand.DynamicRead(nil), spec.dynamic...),
		targetRows:          append([]carrier.Target(nil), spec.targets...),
		carryTargetRows:     append([]carrier.Target(nil), spec.carryTargets...),
		narrowRows:          append([]carrier.Target(nil), spec.narrow...),
		route:               spec.route,
		routeNarrowEligible: spec.routeNarrow,
		writes:              spec.writes,
		unit:                spec.unit,
		target:              spec.target,
		rule:                spec.rule,
		candidate:           spec.candidate,
		invocationRef:       executioncatalog.Ref(^uint32(0)),
	}, true
}

func (member *generatedMember) validRuntimeMember() bool {
	if member == nil || !member.value.Key().Available() || member.hasSlot != member.writes || member.rule == ^uint32(0) || member.target.Mode() != carrier.StrongTarget || member.routeNarrowEligible && member.route == nil {
		return false
	}
	if member.unit != (carrier.Unit{}) && member.unit.Kind() != carrier.ExactUnit {
		return false
	}
	return true
}

func (member *generatedMember) member() equation.RuleMember {
	if member == nil || !member.validRuntimeMember() {
		return equation.RuleMember{}
	}
	return member.value
}

func (member *generatedMember) outputSlot() (shape.Slot, bool) {
	if member == nil || !member.validRuntimeMember() || !member.hasSlot {
		return 0, false
	}
	return member.slot, true
}

func (member *generatedMember) factorKey() (composition.Key, bool) {
	if member == nil || !member.validRuntimeMember() || !member.hasSlot || !member.factor.Available() {
		return composition.Key{}, false
	}
	return member.factor, true
}

func (member *generatedMember) carries() []int {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.carryInputs
}

func (member *generatedMember) initialReads() []demand.Observation {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.initial
}

func (member *generatedMember) dynamicReads() []demand.DynamicRead {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.dynamic
}

func (member *generatedMember) targets() []carrier.Target {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.targetRows
}

func (member *generatedMember) carryTargets() []carrier.Target {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.carryTargetRows
}

func (member *generatedMember) narrowTargets() []carrier.Target {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.narrowRows
}

func (member *generatedMember) routeScope() runtimeFactor {
	if member == nil || !member.validRuntimeMember() {
		return nil
	}
	return member.route
}

func (member *generatedMember) routeNarrow() bool {
	return member != nil && member.validRuntimeMember() && member.routeNarrowEligible
}

func (member *generatedMember) writesOutput() bool {
	return member != nil && member.validRuntimeMember() && member.writes
}

func generatedMemberReads(member *generatedMember) []demand.Observation {
	if member == nil || !member.validRuntimeMember() || len(member.initial) == 0 {
		return nil
	}
	return member.initial
}

func generatedMemberRefusal(member *generatedMember, site string) memberResult {
	return memberResult{boundary: refused(SolveFailureFamilyExecution, site), valid: false}
}

// buildGeneratedFamilies is the typed trampoline from one bound Factor to the
// execution form table. It contributes the Factor's sealed typed plane and
// nothing else: which forms exist, how a plan row is classified into one, and
// how each form seals its family are owned by analysis/engine/execution.
func (factor *boundFactor[K, V]) buildGeneratedFamilies(rows []execution.FormRow) ([]execution.Family, []execution.FormAddress, bool) {
	if factor == nil {
		return nil, nil, false
	}
	plane, planeOK := execution.NewFormPlane(factor.binding, factor.sourceColumns, factor.sourcePresent)
	if !planeOK {
		return nil, nil, false
	}
	families, addresses, _, built := execution.BuildForms(plane, rows)
	if !built {
		return nil, nil, false
	}
	return families, addresses, true
}

func (member *generatedMember) executeGeneratedAt(epoch *executorEpoch, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if member == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.program == nil || !member.validRuntimeMember() || !within.Valid() || epoch.work == nil || !epoch.work.OwnsRuleContributionStates(base, inputs) || epoch.relationRevision == 0 || epoch.generation == 0 || epoch.generatedCatalog == nil {
		return generatedMemberRefusal(member, "preflight")
	}
	row, rowOK := epoch.generatedCatalog.At(member.invocationRef)
	if !rowOK {
		return generatedMemberRefusal(member, "catalog")
	}
	descriptor, descriptorOK := epoch.runtime.program.generatedProgramAt(member.rule)
	if !descriptorOK || descriptor.InputCount() != len(inputs) || descriptor.OutputCount() != 1 {
		return generatedMemberRefusal(member, "descriptor")
	}
	family := row.FamilyOrdinal()
	if uint64(family) >= uint64(len(epoch.generatedWorkers)) {
		return generatedMemberRefusal(member, "family")
	}
	worker := epoch.generatedWorkers[family]
	if worker.run == nil || worker.executor == nil {
		return generatedMemberRefusal(member, "executor")
	}
	ticket, issued := worker.run.Issue(epoch.generatedCatalog, row, epoch.work, base.State(), within, inputs, epoch.generation, epoch.relationRevision, epoch.generation)
	if !issued {
		return generatedMemberRefusal(member, "issue")
	}
	frame, framed := execution.NewFrame(ticket)
	if !framed {
		_ = worker.run.Abort()
		return generatedMemberRefusal(member, "frame")
	}
	result, executed := worker.executor.Execute(frame, ticket)
	if !executed || !result.Valid() {
		_ = worker.run.Abort()
		return generatedMemberRefusal(member, "execute")
	}
	disposition, patches, drained := worker.run.Consume()
	if !drained || disposition != result.Outcome() || len(patches) != result.Count() {
		_ = worker.run.Abort()
		return generatedMemberRefusal(member, "drain")
	}
	switch result.Outcome() {
	case structure.NoCandidate, structure.NoSelection:
		if result.Count() != 0 {
			return generatedMemberRefusal(member, "empty-outcome")
		}
		result := memberResult{boundary: boundaryNone, valid: true}
		result.reads = generatedMemberReads(member)
		return result
	case structure.Concrete:
		if result.Count() != 1 || len(patches) != 1 {
			return generatedMemberRefusal(member, "concrete-count")
		}
		result := memberResult{patch: patches[0], wrote: true, boundary: boundaryNone, valid: true}
		result.reads = generatedMemberReads(member)
		return result
	default:
		return generatedMemberRefusal(member, "outcome")
	}
}

// generatedMemberExecution is the named constructor seam for engine tests and
// the generated seal pass. It returns only compact occurrence geometry.
func generatedMemberExecution(spec generatedMemberSpec) (*generatedMember, bool) {
	return newGeneratedMember(spec)
}

// bindGeneratedMember resolves one generated declaration against the freshly
// attached Factor plane. The declaration already contains the owner-issued
// candidate and exact surfaces; this step only mints the matching carrier
// Unit/Target and compact geometry, then discards the construction surface
// authority.
func bindGeneratedMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, declaration *generatedMemberDeclaration) (*generatedMember, bool) {
	if plane == nil || topology == nil || declaration == nil || declaration.cell == nil || !plane.frozen || plane.runtime == nil || plane.runtime.graph == nil || plane.carrier == nil || plane.byKey == nil || !plane.runtime.graph.OwnsMember(member) || !member.Key().Available() {
		return nil, false
	}
	descriptor := declaration.cell.program
	if !descriptor.Available() {
		return nil, false
	}
	ruleOrdinal, ruleOK := descriptor.Ordinal()
	if !ruleOK || ruleOrdinal != declaration.cell.rule || uint64(ruleOrdinal) >= plane.runtime.schema.ruleCount() || member.Rule() != plane.runtime.schema.ruleSemanticAt(uint64(ruleOrdinal)) {
		return nil, false
	}
	shape, shapeOK := plane.runtime.schema.ruleShapeAt(uint64(ruleOrdinal))
	if !shapeOK || member.WriteCount() != 1 || member.Rule() == (composition.Key{}) || member.OperandFamily() != shape.OperandFamily {
		return nil, false
	}
	writeSurface, writeOK := member.WriteAt(0)
	if !writeOK || writeSurface != declaration.writeSurface.value ||
		writeSurface.Form != equation.SurfaceWriteExact || writeSurface.Mode != equation.TargetModeStrong || writeSurface.Local == 0 {
		return nil, false
	}
	routeRead, routeOK := member.WriteRouteRead(0)
	if !routeOK || routeRead != 0 {
		return nil, false
	}
	writeFactor, writeFactorOK := plane.byKey[writeSurface.Factor]
	if !writeFactorOK || writeFactor == nil {
		return nil, false
	}
	var unit carrier.Unit
	if member.ReadCount() != 0 {
		readSurface, readOK := member.ReadAt(0)
		if !readOK || readSurface != declaration.readSurface.value ||
			readSurface.Form != equation.SurfaceReadExact || readSurface.Mode != equation.TargetModeNone || readSurface.Local == 0 {
			return nil, false
		}
		readFactor, readFactorOK := plane.byKey[readSurface.Factor]
		if !readFactorOK || readFactor == nil || readFactor != writeFactor {
			return nil, false
		}
		var unitOK bool
		unit, unitOK = readFactor.readUnit(readSurface)
		if !unitOK || unit.Kind() != carrier.ExactUnit {
			return nil, false
		}
	}
	target, targetOK := writeFactor.writeTarget(writeSurface)
	slot, slotOK := writeFactor.runtimeSlot()
	if !targetOK || !slotOK || target.Mode() != carrier.StrongTarget {
		return nil, false
	}
	inputCount := descriptor.InputCount()
	spec := generatedMemberSpec{
		member: member, outputSlot: slot, hasSlot: true, factor: writeSurface.Factor,
		targets:       []carrier.Target{target},
		factorOrdinal: int32(descriptor.OutputFactor()), unit: unit, target: target,
		rule:      ruleOrdinal,
		candidate: declaration.candidate, inputCount: inputCount, outputCount: descriptor.OutputCount(), writes: true,
	}
	if descriptor.ReadCount() != 0 {
		carryInput := descriptor.CarryInput()
		if inputCount <= 0 || carryInput < 0 || carryInput >= inputCount || descriptor.ReadInput() < 0 || descriptor.ReadInput() >= inputCount {
			return nil, false
		}
		spec.carries = []int{carryInput}
		spec.initial = []demand.Observation{{Input: uint64(descriptor.ReadInput()), Unit: unit}}
		spec.carryTargets = []carrier.Target{target}
		spec.readInput = descriptor.ReadInput()
	} else if inputCount != 0 {
		return nil, false
	}
	return newGeneratedMember(spec)
}
