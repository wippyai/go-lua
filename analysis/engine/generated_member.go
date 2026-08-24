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
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
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
	// routed says this member publishes through the route universe of its
	// output Factor rather than at one exact target. A routed row has no
	// static destination at all: its destinations are the members of the
	// relation its selected join derives, which exist only per invocation.
	routed bool
	writes bool

	factorOrdinal int32
	unit          carrier.Unit
	target        carrier.Target
	readInput     int // construction-only Plan coordinate; descriptor owns hot value
	rule          uint32
	candidate     uint32
	source        execution.ProgramSource
	inputCount    int // construction-only Plan shape
	outputCount   int // construction-only Plan shape
}

// generatedMemberDeclaration is the compact construction result of one
// generated issuance. Surfaces are minted from the bound Factor cells and are
// retained only until the matching runtime Unit/Target are attached.
type generatedMemberDeclaration struct {
	cell      *generatedRuleCell
	operand   equation.Operand
	candidate uint32
	// source is the row-local Program capability a rule whose candidates are
	// Program rows carries. It is the zero value for every other rule.
	source execution.ProgramSource
	// reads are this rule's declared read surfaces in plan order. A rule with
	// no join has none; every other rule has exactly the joins its Plan states,
	// each already in the form its own row declares.
	reads        []RuleReadSurface
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

	// source is the row-local Program capability this member's rule carries
	// when its candidates are Program rows.
	source execution.ProgramSource

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
	routed              bool
	writes              bool

	unit       carrier.Unit
	target     carrier.Target
	rule       uint32
	candidate  uint32
	invocation generatedRowSeal
}

// generatedRowSeal binds one member row to the one execution row it dispatches
// to. The catalog Ref is not a free field: it exists only inside a seal, beside
// the catalog that minted it, the member ordinal the row was drafted under, and
// the exact Unit/Target the typed family row was built from. The whole proof is
// established once, when the execution program installs the address; dispatch
// reads it and never re-derives an address of its own.
type generatedRowSeal struct {
	catalog *executioncatalog.Catalog
	unit    carrier.Unit
	target  carrier.Target
	ref     executioncatalog.Ref
	ordinal uint32
}

func (seal generatedRowSeal) installed() bool { return seal.catalog != nil }

var _ runtimeMemberGeometry = (*generatedMember)(nil)

// newGeneratedMember seals one generated member projection. A generated
// member owns at most one output axis: outputCount is therefore restricted to
// zero or one, and output geometry must agree with that cardinality.
func newGeneratedMember(spec generatedMemberSpec) (*generatedMember, bool) {
	if !spec.member.Key().Available() || spec.inputCount < 0 || spec.outputCount < 0 || spec.outputCount > 1 {
		return nil, false
	}
	if spec.factorOrdinal < 0 {
		return nil, false
	}
	// A routed row's destination is not a coordinate this member holds: it is
	// whichever members its selected join derives for one invocation. So the
	// exact target is present exactly when the row is not routed, and a routed
	// row must name the Factor whose route universe it claims instead.
	if spec.routed {
		// A routed row has no exact target and no static output vector. Its
		// carry closure may still be present: those are the coordinates the
		// row preserves, not the ones it publishes at.
		if spec.target != (carrier.Target{}) || len(spec.targets) != 0 || spec.route == nil {
			return nil, false
		}
		if len(spec.carryTargets) != 0 && len(spec.carries) == 0 {
			return nil, false
		}
	} else if spec.target == (carrier.Target{}) || spec.route != nil || spec.routeNarrow {
		return nil, false
	}
	// The exact Unit is the coordinate of this row's first exact observation.
	// A row whose every join is selected observes no standing coordinate, so it
	// carries none rather than an invented one.
	if len(spec.initial) == 0 {
		if spec.readInput != 0 || spec.unit != (carrier.Unit{}) {
			return nil, false
		}
	} else if spec.unit == (carrier.Unit{}) || spec.readInput < 0 || spec.readInput >= spec.inputCount {
		return nil, false
	}
	if spec.inputCount == 0 && (len(spec.initial) != 0 || len(spec.dynamic) != 0) {
		return nil, false
	}
	if spec.hasSlot != spec.writes || (spec.outputCount == 1) != spec.writes {
		return nil, false
	}
	if spec.hasSlot {
		if spec.outputSlot < 0 || !spec.factor.Available() || (len(spec.targets) == 0) != spec.routed {
			return nil, false
		}
	} else if spec.outputSlot != 0 || spec.factor.Available() || len(spec.targets) != 0 || len(spec.carryTargets) != 0 || len(spec.narrow) != 0 || spec.route != nil || spec.routeNarrow || spec.routed {
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
	if !spec.routed && spec.target.Mode() != carrier.StrongTarget {
		return nil, false
	}
	if len(spec.initial) != 0 && spec.unit.Kind() != carrier.ExactUnit {
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
		routed:              spec.routed,
		writes:              spec.writes,
		unit:                spec.unit,
		target:              spec.target,
		rule:                spec.rule,
		candidate:           spec.candidate,
		source:              spec.source,
	}, true
}

func (member *generatedMember) validRuntimeMember() bool {
	if member == nil || !member.value.Key().Available() || member.hasSlot != member.writes || member.rule == ^uint32(0) || member.routeNarrowEligible && member.route == nil {
		return false
	}
	if member.routed {
		if member.target != (carrier.Target{}) || member.route == nil {
			return false
		}
	} else if member.target.Mode() != carrier.StrongTarget {
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

// sealInvocationRow installs this row's one authenticated execution address.
// It runs once, cold, where the execution program mints the address: the
// catalog row must already name this member's ordinal, rule and candidate
// occurrence, and the handles the typed family row was built from must be the
// handles this row's contribution plane presents. A row that is already sealed
// refuses a second address.
func (member *generatedMember) sealInvocationRow(catalog *executioncatalog.Catalog, ref executioncatalog.Ref, ordinal uint32, unit carrier.Unit, target carrier.Target) bool {
	if member == nil || catalog == nil || member.invocation.installed() || !member.validRuntimeMember() {
		return false
	}
	row, rowOK := catalog.At(ref)
	if !rowOK || row.MemberOrdinal() != ordinal || row.RuleOrdinal() != member.rule || row.CandidateOrdinal() != member.candidate {
		return false
	}
	if member.unit != unit || member.target != target {
		return false
	}
	member.invocation = generatedRowSeal{catalog: catalog, unit: unit, target: target, ref: ref, ordinal: ordinal}
	return true
}

// authenticateInvocationRow reads the seal against the live epoch and answers
// the sealed row or the boundary that refused it. Nothing here re-proves an
// invariant: the catalog must be the authority the address was minted against,
// the row must still name this member's occurrence, and the live Unit/Target
// must still be the pair the typed row executes. Each fence names its own
// refusal, so a foreign handle and a stale address are never one verdict.
func (member *generatedMember) authenticateInvocationRow(catalog *executioncatalog.Catalog) (executioncatalog.Row, solveBoundary) {
	if !member.invocation.installed() {
		return executioncatalog.Row{}, refused(SolveFailureFamilyExecution, "unsealed-row")
	}
	if catalog != member.invocation.catalog {
		return executioncatalog.Row{}, refused(SolveFailureFamilyExecution, "row-owner")
	}
	row, rowOK := catalog.At(member.invocation.ref)
	if !rowOK {
		return executioncatalog.Row{}, refused(SolveFailureFamilyExecution, "catalog")
	}
	if row.MemberOrdinal() != member.invocation.ordinal || row.RuleOrdinal() != member.rule || row.CandidateOrdinal() != member.candidate {
		return executioncatalog.Row{}, refused(SolveFailureFamilyExecution, "row-ordinal")
	}
	if member.unit != member.invocation.unit {
		return executioncatalog.Row{}, refused(SolveFailureFamilyExecution, "foreign-unit")
	}
	if member.target != member.invocation.target {
		return executioncatalog.Row{}, refused(SolveFailureFamilyExecution, "foreign-target")
	}
	return row, boundaryNone
}

// buildGeneratedFamilies is the typed trampoline from one bound Factor to the
// execution form table. It contributes the Factor's sealed typed plane and
// nothing else: which forms exist, how a plan row is classified into one, and
// how each form seals its family are owned by analysis/engine/execution.
func (factor *boundFactor[K, V]) buildGeneratedFamilies(rows []execution.FormRow, foreign []execution.ForeignFactor) ([]execution.Family, []execution.FormAddress, bool) {
	if factor == nil {
		return nil, nil, false
	}
	plane, planeOK := execution.NewFormPlane(factor.binding, factor.sourceColumns, factor.sourcePresent, factor.routeGeometry(), foreign, factor.families)
	if !planeOK {
		return nil, nil, false
	}
	families, addresses, _, built := execution.BuildForms(plane, rows)
	if !built {
		return nil, nil, false
	}
	return families, addresses, true
}

// foreignRead erases this Factor's key and fact types so a rule that joins it
// from another Factor's plane can recover the typed read it declared.
func (factor *boundFactor[K, V]) foreignRead() (execution.ForeignFactor, bool) {
	if factor == nil {
		return nil, false
	}
	return execution.NewForeignFactor(factor.binding, factor.routeGeometry())
}

func (member *generatedMember) executeGeneratedAt(epoch *executorEpoch, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if member == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.program == nil || !member.validRuntimeMember() || !within.Valid() || epoch.work == nil || !epoch.work.OwnsRuleContributionStates(base, inputs) {
		return generatedMemberRefusal(member, "preflight")
	}
	// The invocation fences are the one authentication that cannot be sealed:
	// they name this solve generation, not this row's address.
	if epoch.relationRevision == 0 || epoch.generation == 0 {
		return generatedMemberRefusal(member, "generation")
	}
	row, boundary := member.authenticateInvocationRow(epoch.generatedCatalog)
	if boundary.available() {
		return memberResult{boundary: boundary, valid: false}
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
	// A transformed carry is authenticated against the coverage of the input it
	// carries. The solver resolves that coverage from its own contribution here,
	// once, so execution never reaches back into the contribution plane.
	var carried carrier.SlotCoverage
	if mode, present := descriptor.CarryMode(); present && mode == ruleprogram.CarryTransform && len(member.carryInputs) != 0 && member.hasSlot {
		coverage, coverageOK := epoch.work.RuleContributionCarrySlotCoverage(base, carrier.ContributionSource{Slot: member.slot, Input: member.carryInputs[0]})
		if !coverageOK {
			return generatedMemberRefusal(member, "carry-coverage")
		}
		carried = coverage
	}
	ticket, issued := worker.run.Issue(epoch.generatedCatalog, row, epoch.work, base.State(), within, inputs, carried, epoch.generation, epoch.relationRevision, epoch.generation)
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
	// The rule this member belongs to is the cell's own sealed foreign key.
	// It is read, never re-derived from the descriptor: the descriptor is the
	// row the seal placed at that ordinal and carries no copy of it.
	ruleOrdinal := declaration.cell.rule
	if uint64(ruleOrdinal) >= plane.runtime.schema.ruleCount() || member.Rule() != plane.runtime.schema.ruleSemanticAt(uint64(ruleOrdinal)) {
		return nil, false
	}
	shape, shapeOK := plane.runtime.schema.ruleShapeAt(uint64(ruleOrdinal))
	if !shapeOK || member.WriteCount() != 1 || member.Rule() == (composition.Key{}) || member.OperandFamily() != shape.OperandFamily {
		return nil, false
	}
	mode, modeOK := descriptor.OutputMode()
	writeSurface, writeOK := member.WriteAt(0)
	routeRead, routeReadOK := member.WriteRouteRead(0)
	if !modeOK || !writeOK || !routeReadOK || writeSurface != declaration.writeSurface.value {
		return nil, false
	}
	writeFactor, writeFactorOK := plane.byKey[writeSurface.Factor]
	if !writeFactorOK || writeFactor == nil {
		return nil, false
	}
	inputCount := descriptor.InputCount()
	if member.ReadCount() != descriptor.ReadCount() || len(declaration.reads) != descriptor.ReadCount() {
		return nil, false
	}
	spec := generatedMemberSpec{
		member: member, factor: writeSurface.Factor, hasSlot: true, writes: true,
		factorOrdinal: int32(descriptor.OutputFactor()),
		rule:          ruleOrdinal,
		candidate:     declaration.candidate, source: declaration.source, inputCount: inputCount, outputCount: descriptor.OutputCount(),
	}
	slot, slotOK := writeFactor.runtimeSlot()
	if !slotOK {
		return nil, false
	}
	spec.outputSlot = slot
	// Each declared join is delivered in the form its own row states. An exact
	// join names the one coordinate it observes; a selected join names none,
	// because its coordinates are the members of a relation derived per
	// invocation, and what the member carries instead is the standing
	// permission to observe that Factor.
	for index := 0; index < descriptor.ReadCount(); index++ {
		plan, planOK := descriptor.ReadAt(index)
		readSurface, readOK := member.ReadAt(index)
		if !planOK || !readOK || readSurface != declaration.reads[index].value {
			return nil, false
		}
		// A read may name a Factor other than the written one. A rule whose
		// join is foreign is exactly the rule the engine cannot type
		// generically, and it reaches execution through the family its owner
		// installs; the form builders refuse a cross-Factor Unit on their own.
		readFactor, readFactorOK := plane.byKey[readSurface.Factor]
		if !readFactorOK || readFactor == nil || uint64(plan.Input) >= uint64(inputCount) {
			return nil, false
		}
		switch readSurface.Form {
		case equation.SurfaceReadExact:
			if readSurface.Mode != equation.TargetModeNone || readSurface.Local == 0 {
				return nil, false
			}
			unit, unitOK := readFactor.readUnit(readSurface)
			if !unitOK || unit.Kind() != carrier.ExactUnit {
				return nil, false
			}
			if len(spec.initial) == 0 {
				spec.unit, spec.readInput = unit, int(plan.Input)
			}
			spec.initial = append(spec.initial, demand.Observation{Input: uint64(plan.Input), Unit: unit})
		case equation.SurfaceReadSelect:
			readSlot, readSlotOK := readFactor.runtimeSlot()
			if !readSlotOK {
				return nil, false
			}
			spec.dynamic = append(spec.dynamic, demand.DynamicRead{Input: uint64(plan.Input), Slot: readSlot})
		default:
			return nil, false
		}
	}
	carryInput := descriptor.CarryInput()
	switch mode {
	case ruleprogram.ModeExact:
		if writeSurface.Form != equation.SurfaceWriteExact || writeSurface.Mode != equation.TargetModeStrong || writeSurface.Local == 0 || routeRead != 0 {
			return nil, false
		}
		target, targetOK := writeFactor.writeTarget(writeSurface)
		if !targetOK || target.Mode() != carrier.StrongTarget {
			return nil, false
		}
		spec.target, spec.targets = target, []carrier.Target{target}
		if carryInput >= 0 {
			if carryInput >= inputCount {
				return nil, false
			}
			spec.carries = []int{carryInput}
			spec.carryTargets = []carrier.Target{target}
		}
	case ruleprogram.ModeRoute:
		if writeSurface.Form != equation.SurfaceWriteRoute || routeRead == 0 || uint64(routeRead) > uint64(descriptor.ReadCount()) {
			return nil, false
		}
		// A routed member publishes at coordinates its own derived relation
		// answers, so it claims the output Factor's route universe rather than
		// one exact target.
		if !writeFactor.hasRouteUniverse() {
			return nil, false
		}
		spec.routed = true
		spec.route = writeFactor
		spec.routeNarrow = writeFactor.supports(carrier.Narrow)
		if carryInput >= 0 {
			// A routed row that also carries preserves every coordinate its
			// routes did not select. Those coordinates are not a vector this
			// row can name - a route set is decided per invocation - so the
			// carry claims the Factor's whole route universe on top of its
			// exact closure, which is the same scope the authored arm claims
			// and the Region seal expands once per (Region, Factor).
			if carryInput >= inputCount || !writeFactor.carryRouteScopeFor(member) {
				return nil, false
			}
			carryTargets, carryTargetsOK := writeFactor.carryTargetsFor(member)
			if !carryTargetsOK {
				return nil, false
			}
			spec.carries = []int{carryInput}
			spec.carryTargets = carryTargets
		}
	default:
		return nil, false
	}
	if descriptor.ReadCount() == 0 && inputCount != 0 {
		return nil, false
	}
	return newGeneratedMember(spec)
}
