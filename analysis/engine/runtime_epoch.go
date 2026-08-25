// runtime_epoch.go holds the executor epoch vocabulary, the point queue, the failure recorders and epoch lifecycle.

package engine

import (
	"context"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// producerEpoch is an epoch-local candidate cache in graph Group order. A
// generation marks it dirty; the runnable identity is always its output Point.
type producerEpoch struct {
	state      contextfiber.StateOrdinal
	group      int
	generation uint64
	applied    uint64
	candidate  carrier.RuleContribution
	hasValue   bool
	// rememberAt stamps the operand clock at the closed input epoch this cache
	// holds. Every input operand marked after that stamp is a changed input,
	// so the cache retains no ordered input-version snapshot to diff against.
	// A stale read-registration cut deliberately retains its earlier transport
	// stamp for the next generation; zero means no reusable input epoch exists.
	rememberAt  uint64
	inputs      []carrier.PointState
	inputStates []carrier.State
	patches     []carrier.Patch
	patchRows   []contributionPatch
	reads       []demand.Observation
	// artifactReadKeys is the exact sparse inverse membership currently
	// contributed by this completed producer occurrence. It is kept apart from
	// reads because evaluate replaces reads before the contextual inverse has
	// authenticated and committed its new rows.
	artifactReadKeys []artifactProducerReadKey
	// artifactReadFactorKeys is the matching sparse factor-publication
	// membership. A FactorRegion can intentionally carry no UnitRegion rows;
	// its slot still identifies the exact factor whose completed exact reads
	// must be reconsidered in this StateOrdinal.
	artifactReadFactorKeys []artifactProducerReadFactorKey
}

// contributionPatch keeps carrier admission order separate from Rule callback
// order. Members execute in canonical graph order; only their already
// accepted patches are reordered into the carrier's physical slot order at
// the one contribution finish cut.
type contributionPatch struct {
	slot  shape.Slot
	patch carrier.Patch
}

// regionEpoch is a disposable localized-widening episode. exact is the last
// exact E⊔B head RHS; it never aliases a widened Point publication.
type regionEpoch struct {
	// phase belongs to this exact recurrence episode.  Nested regions may
	// restart independently: a child re-ascent must not turn an enclosing
	// narrowed episode into an ascent episode with its narrowed head retained.
	phase    solvePhase
	episode  uint64
	exact    carrier.PointRHS
	hasExact bool
	// accumulator is this episode's retained raw E⊔B fold, taken before the
	// support-axis discharge that turns it into exact. An ascent whose operand
	// evidence admits reuse folds only the moved operands onto it instead of
	// rebuilding the complete recurrence row.
	accumulator    carrier.PointRHS
	hasAccumulator bool
	invalid        bool
	// interfaceRefreshPending is an epoch-local barrier.  A stale boundary
	// version first dirties its ordinary Group owners and waits for their
	// candidate generations to settle; only then may the head refold and take
	// a new version snapshot.  Keeping the old snapshot during that interval
	// prevents a second head visit from re-dirtying the same Groups.
	interfaceRefreshPending bool
	// The remaining fields are stamps on the epoch's one operand clock. They
	// replace the seven parallel version vectors this episode used to keep
	// purely to diff: rememberAt closes the interface epoch, externalAt/backAt
	// record the newest mark on this Region's ingress rows, pointsAt records
	// the newest mark on its interior points, enterAt closes the WTO pass, and
	// postfixAt closes the relation certificate. pending accumulates the
	// classified evidence of every ingress mark since rememberAt.
	rememberAt uint64
	externalAt uint64
	backAt     uint64
	pointsAt   uint64
	enterAt    uint64
	postfixAt  uint64
	pending    change.Set
}

type solvePhase uint8

const (
	phaseAscent solvePhase = iota + 1
	phaseNarrow
)

type pointPublication uint8

const (
	publicationAscending pointPublication = iota + 1
	publicationMayDescend
)

// structuralEpoch stamps every point that published below its predecessor with
// a strictly increasing global descent generation.
type structuralEpoch struct {
	descent      uint64
	pointDescent []uint64
}

const (
	epochRunning uint32 = iota + 1
	epochCompleted
	epochIncomplete
)

// pointQueue is a dense, deduplicating Point queue. It is intentionally a
// readiness bitset: WTO events determine dequeue order, not Group ordering.
type pointQueue struct {
	ready []bool
	count int
}

func newPointQueue(count int) pointQueue { return pointQueue{ready: make([]bool, count)} }

func (queue *pointQueue) add(point int) bool {
	if queue == nil || point < 0 || point >= len(queue.ready) {
		return false
	}
	if !queue.ready[point] {
		queue.ready[point] = true
		queue.count++
	}
	return true
}

func (queue *pointQueue) take(point int) bool {
	if queue == nil || point < 0 || point >= len(queue.ready) || !queue.ready[point] {
		return false
	}
	queue.ready[point] = false
	queue.count--
	return true
}

func (queue *pointQueue) pending() bool { return queue != nil && queue.count != 0 }

type pointWTOFrame struct{ region int }

type executorEpoch struct {
	runtime *solverRuntime
	ctx     context.Context
	// report is a call-scoped first-failure sink. It is nil on the ordinary
	// Solve path, so the hot path does not allocate or retain diagnostics.
	report             *SolveReport
	diagnostics        *solveDiagnosticState
	diagnosticRevision identity.Generation
	work               *carrier.Work
	generatedCatalog   *executioncatalog.Catalog
	generatedWorkers   []generatedExecutionWorker
	relationRevision   uint64
	generation         uint64
	demand             *demand.Epoch
	points             []carrier.PointState
	producers          []producerEpoch
	regions            []regionEpoch
	// operands is the change layer over the sealed operand plane. It is the
	// sole representation-identity authority in the epoch: no point, producer
	// or region keeps a private version counter beside it.
	operands operandEpoch
	// operandScratch is the reusable delta row the recurrence fold consumes.
	operandScratch []int
	// candidatesPending is one dense counter per active Region. It counts the
	// producer Groups in that Region's complete event-point interval (and thus
	// in every nested descendant) whose latest wake generation has not yet
	// been evaluated. The counter is the only hot-path candidate-settled
	// witness; Group rows remain the source of the generation/applied state.
	candidatesPending []uint64
	queue             pointQueue
	structuralDirty   []bool
	structural        structuralEpoch
	postfixDirty      []bool
	postfixPending    []int
	postfixHead       int
	frames            []pointWTOFrame
	nested            []int
	regionScratch     []int
	terminal          atomic.Uint32
	// failed is the publication fail-stop. A publication path that returns
	// false after installing into points leaves that install half-landed, so
	// the flag makes the epoch refuse every later read through canceled().
	failed            bool
	activationPending bool
	activations       []equation.AcceptedMember
	storePub          *solvedPublication
	currentState      int
	// artifactProducerReads is the epoch-local sparse inverse for completed
	// exact reads. It is intentionally absent from the sealed runtime: exact
	// Units discovered by a Product belong to this solve generation only.
	artifactProducerReads artifactProducerReadIndex
}

type generatedExecutionWorker struct {
	run      *execution.Run
	executor execution.Executor
}

// generatedExecutionProgram is immutable Program-owned execution data. Its
// catalog and typed family descriptors are built once at Program seal; epochs
// allocate only bounded workers with fresh Run/scratch state.
type generatedExecutionProgram struct {
	catalog  *executioncatalog.Catalog
	families []execution.Family
}

type generatedFamilyAssignment struct {
	family uint32
	local  uint32
}

// generatedRowOutputWidth states how many Factor patch slots one generated
// row's invocation publishes through. The declared publication mode is the
// whole answer: a fact publication opens the one slot its output addresses,
// and a structural publication opens none, because its result is the branch
// set it settles and no Factor column receives it. The catalog row and the
// family's Run are sized from this one reading, so the two never disagree
// about a row's width.
func generatedRowOutputWidth(descriptor generated.CompiledRule) (uint16, bool) {
	mode, modeOK := descriptor.OutputMode()
	if !modeOK {
		return 0, false
	}
	if mode == ruleprogram.ModeStructural {
		return 0, true
	}
	return 1, true
}

// buildGeneratedExecutionProgram performs the one cold grouping step from
// sealed member rows to typed families. Each row takes the execution form its
// own descriptor declares; this pass only routes that row to its owning
// Factor. The solve loop later performs two dense indexes (ref ->
// row, family -> worker) and nothing else.
func buildGeneratedExecutionProgram(program *runtimeProgram) (*generatedExecutionProgram, topologyConstructionRefusal, bool) {
	if program == nil || !program.valid() {
		return nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
	}
	memberCount := program.memberCount()
	rowsByOwner := make([][]execution.FormRow, len(program.factorOwners))
	assignments := make([]generatedFamilyAssignment, memberCount)
	// installed is the row each member declared and was handed over as. A
	// declared Form is the presence proof, and the row carries the exact
	// Unit/Target the owner's family was built from, which is what the member's
	// invocation seal is authenticated against below.
	installed := make([]execution.FormRow, memberCount)
	assigned := make([]bool, memberCount)
	for memberIndex := 0; memberIndex < memberCount; memberIndex++ {
		row, rowOK := program.memberRowAt(memberIndex)
		if !rowOK || row.generated == nil {
			continue
		}
		descriptor, descriptorOK := program.generatedProgramAt(row.generated.rule)
		if !descriptorOK {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		if descriptor.OutputCount() != 1 || int(descriptor.OutputFactor()) >= len(rowsByOwner) {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		// A row's Target is its one static destination, and a routed row has
		// none: it publishes at the members its own relation derives. The
		// declared publication mode decides which of the two this row is, so a
		// missing Target is a refusal for an exact row and the whole point of a
		// routed one.
		mode, modeOK := descriptor.OutputMode()
		if !modeOK || (row.generated.target.Mode() == carrier.StrongTarget) != (mode == ruleprogram.ModeExact) {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		formRow, declared := execution.DeclaredForm(descriptor)
		if !declared {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		formRow.Member, formRow.Unit, formRow.Target = memberIndex, row.generated.unit, row.generated.target
		formRow.Candidate = row.generated.candidate
		formRow.Source = row.generated.source
		// The plan row names its rule by the member row's own sealed foreign
		// key. The descriptor beside it is the row at that ordinal and holds no
		// copy of it, so this is the only place the coordinate is carried.
		formRow.RuleOrdinal = row.generated.rule
		exact := 0
		for join := 0; join < descriptor.ReadCount(); join++ {
			plan, planOK := descriptor.ReadAt(join)
			if !planOK {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			if plan.Form != ruleprogram.Exact {
				continue
			}
			if exact >= len(row.generated.initial) || row.generated.initial[exact].Input != uint64(plan.Input) {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			var bound bool
			formRow, bound = formRow.BindExact(join, row.generated.initial[exact].Unit)
			if !bound {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			exact++
		}
		if exact != len(row.generated.initial) {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		// Every nested member set the rule's joins span is lowered onto the row
		// at its own join ordinal, already enumerated at the parent the read is
		// addressed by. The family reads it; nothing enumerates a second time.
		for _, set := range row.generated.memberSetsOf() {
			var bound bool
			formRow, bound = formRow.BindMembers(set.join, set.coordinates)
			if !bound {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
		}
		// A structural row's branch set is lowered as its census. The branches
		// themselves were resolved to mounted members at this row's bind, so
		// what the family needs is how many of them there are to settle.
		if branches, structural := row.generated.branchCensus(); structural {
			var bound bool
			formRow, bound = formRow.BindBranches(branches)
			if !bound {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
		}
		rowsByOwner[descriptor.OutputFactor()] = append(rowsByOwner[descriptor.OutputFactor()], formRow)
		installed[memberIndex] = formRow
	}
	// The read table is the whole Program's Factor read sides, indexed by
	// sealed Factor ordinal. It is built once here, before any family is
	// sealed, because a rule's own joins decide which entries its installer
	// sees and no Factor may choose that for another.
	foreign := make([]execution.ForeignFactor, len(program.factorOwners))
	for ownerIndex := range program.factorOwners {
		owner, ownerOK := program.factorOwnerAt(int32(ownerIndex))
		if !ownerOK || owner == nil {
			return nil, refuseProgramSeal(topologyConstructionStepBinding), false
		}
		read, readOK := owner.foreignRead()
		if !readOK {
			return nil, refuseProgramSeal(topologyConstructionStepBinding), false
		}
		foreign[ownerIndex] = read
	}
	families := make([]execution.Family, 0, len(rowsByOwner)*2)
	for ownerIndex, formRows := range rowsByOwner {
		if len(formRows) == 0 {
			continue
		}
		owner, ownerOK := program.factorOwnerAt(int32(ownerIndex))
		if !ownerOK || owner == nil {
			return nil, refuseProgramSeal(topologyConstructionStepDirectory), false
		}
		executors, addresses, built := owner.buildGeneratedFamilies(formRows, foreign)
		if !built || len(addresses) != len(formRows) || len(executors) == 0 {
			return nil, refuseProgramSeal(topologyConstructionStepDirectory), false
		}
		familyBase := uint32(len(families))
		for _, executor := range executors {
			// A family's output capacity is the number of Factor columns its
			// rows publish into. A structural family publishes none, so zero is
			// a declared width rather than a missing one.
			if executor == nil || executor.InputCapacity() < 0 || executor.OutputCapacity() < 0 {
				return nil, refuseProgramSeal(topologyConstructionStepDirectory), false
			}
			families = append(families, executor)
		}
		for _, address := range addresses {
			if address.Member < 0 || address.Member >= memberCount || !installed[address.Member].Form.Declared() || assigned[address.Member] || uint64(address.FamilyOffset) >= uint64(len(executors)) {
				return nil, refuseProgramSeal(topologyConstructionStepDirectory), false
			}
			assignments[address.Member] = generatedFamilyAssignment{family: familyBase + address.FamilyOffset, local: address.Local}
			assigned[address.Member] = true
		}
	}
	drafts := make([]executioncatalog.Draft, 0, memberCount)
	draftMembers := make([]int, 0, memberCount)
	for memberIndex := 0; memberIndex < memberCount; memberIndex++ {
		if !installed[memberIndex].Form.Declared() {
			continue
		}
		if !assigned[memberIndex] {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		row, rowOK := program.memberRowAt(memberIndex)
		if !rowOK || row.generated == nil {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		descriptor, descriptorOK := program.generatedProgramAt(row.generated.rule)
		if !descriptorOK || descriptor.InputCount() < 0 || descriptor.InputCount() > int(^uint16(0)) || descriptor.OutputCount() != 1 {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		outputs, widthOK := generatedRowOutputWidth(descriptor)
		if !widthOK {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		assignment := assignments[memberIndex]
		drafts = append(drafts, executioncatalog.Draft{Family: assignment.family, Local: assignment.local, Rule: row.generated.rule, Member: uint32(memberIndex), Candidate: row.generated.candidate, InputCount: uint16(descriptor.InputCount()), OutputCount: outputs})
		draftMembers = append(draftMembers, memberIndex)
	}
	catalog, sealed := executioncatalog.Seal(drafts)
	if !sealed {
		return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
	}
	// The catalog is the authority an address is minted against, so a member
	// row is sealed to its invocation only once that authority exists. This is
	// the one place the proof is established: dispatch reads the seal.
	for ref, memberIndex := range draftMembers {
		row, rowOK := program.memberRowAt(memberIndex)
		if !rowOK || row.generated == nil {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		if !row.generated.sealInvocationRow(catalog, executioncatalog.Ref(ref), uint32(memberIndex), installed[memberIndex].Unit, installed[memberIndex].Target) {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
	}
	return &generatedExecutionProgram{catalog: catalog, families: families}, topologyConstructionRefusal{}, true
}

func (epoch *executorEpoch) recordFailure(reason SolveFailureReason, boundary solveBoundary, point, group, member, rule composition.Key) {
	if epoch == nil || epoch.report == nil {
		return
	}
	epoch.report.record(reason, boundary, reportedSemanticKey(point), reportedSemanticKey(group), reportedSemanticKey(member), reportedSemanticKey(rule))
}

func (epoch *executorEpoch) recordPointFailure(reason SolveFailureReason, point equation.Point) {
	if epoch == nil || !point.Available() {
		return
	}
	epoch.recordFailure(reason, boundaryNone, point.Key(), composition.Key{}, composition.Key{}, composition.Key{})
}

// recordRefreshPointFailure is the point-level fallback for refreshPoint.
// Member/group failures record their more specific certificate first, so this
// deliberately relies on SolveReport's first-wins record boundary.
func (epoch *executorEpoch) recordRefreshPointFailure(boundary solveBoundary, point equation.Point) {
	if epoch == nil {
		return
	}
	pointKey := composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	epoch.recordFailure(SolveFailureReasonExecution, boundary, pointKey, composition.Key{}, composition.Key{}, composition.Key{})
}

// recordRunFailure records a closed executor-loop boundary with no invented
// Point/Group coordinate. Cancellation and diagnostic cutoff are terminal
// statuses, not execution failures, and are intentionally handled by run's
// callers without reaching this helper.
func (epoch *executorEpoch) recordRunFailure(boundary solveBoundary) {
	if epoch == nil || epoch.canceledByContext() {
		return
	}
	epoch.recordFailure(SolveFailureReasonExecution, boundary, composition.Key{}, composition.Key{}, composition.Key{}, composition.Key{})
}

func (epoch *executorEpoch) recordGroupFailure(reason SolveFailureReason, point equation.Point, group equation.GroupNode) {
	epoch.recordGroupBoundaryFailure(reason, boundaryNone, point, group)
}

// recordGroupBoundaryFailure preserves the exact engine boundary for failures
// that occur after a sealed Group has been selected but outside an individual
// member. The caller must not invent a member identity for group-wide
// transport, contribution, or publication failures.
func (epoch *executorEpoch) recordGroupBoundaryFailure(reason SolveFailureReason, boundary solveBoundary, point equation.Point, group equation.GroupNode) {
	if epoch == nil {
		return
	}
	epoch.recordFailure(reason, boundary, func() composition.Key {
		if point.Available() {
			return point.Key()
		}
		return composition.Key{}
	}(), func() composition.Key {
		if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsGroup(group) {
			return group.Key()
		}
		return composition.Key{}
	}(), composition.Key{}, composition.Key{})
}

// recordCandidateOrderFailure keeps the producer certificate at the exact
// order boundary. Program-artifact groups are singleton, so their Rule role
// remains classifiable by AnalyzeDiagnostics; multi-member groups retain only
// their Group identity rather than inventing one culprit.
func (epoch *executorEpoch) recordCandidateOrderFailure(boundary solveBoundary, point equation.Point, group equation.GroupNode) {
	if epoch == nil {
		return
	}
	if group.MemberCount() == 1 {
		if member, ok := group.MemberAt(0); ok {
			epoch.recordMemberFailure(SolveFailureReasonExecution, boundary, point, group, member)
			return
		}
	}
	pointKey, groupKey := composition.Key{}, composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsGroup(group) {
		groupKey = group.Key()
	}
	epoch.recordFailure(SolveFailureReasonExecution, boundary, pointKey, groupKey, composition.Key{}, composition.Key{})
}

func (epoch *executorEpoch) recordMemberFailure(reason SolveFailureReason, boundary solveBoundary, point equation.Point, group equation.GroupNode, member equation.RuleMember) {
	if epoch == nil {
		return
	}
	groupKey, memberKey, ruleKey := composition.Key{}, composition.Key{}, composition.Key{}
	if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsGroup(group) {
		groupKey = group.Key()
	}
	if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsMember(member) {
		memberKey, ruleKey = member.Key(), member.Rule()
	}
	pointKey := composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	epoch.recordFailure(reason, boundary, pointKey, groupKey, memberKey, ruleKey)
}

func newRuntimeEpoch(runtime *solverRuntime, relation equation.Relation, ctx context.Context) (*executorEpoch, bool) {
	// Owner/liveness fence: do not allocate an epoch for a missing or canceled
	// call, or for a runtime with a missing owner-owned root.
	if runtime == nil {
		return nil, false
	}
	if ctx == nil {
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	if runtime.carrier == nil || runtime.graph == nil || runtime.points == nil || runtime.demand == nil || runtime.topology == nil {
		return nil, false
	}

	// Accepted members must belong to this runtime generation's sealed
	// topology before any carrier or demand state is opened.
	if !relation.OwnedBy(runtime.topology) {
		return nil, false
	}

	// All dense runtime rows must remain aligned with their graph-owned
	// cardinalities. A mismatch is structural corruption, not an empty solve.
	pointCount := runtime.graph.PointCount()
	groupCount := runtime.graph.GroupCount()
	regionCount := len(runtime.regions)
	if len(runtime.producers) != groupCount ||
		len(runtime.activePoints) != pointCount ||
		len(runtime.pointInitials) != pointCount ||
		len(runtime.activeRegions) != regionCount ||
		len(runtime.regions) != regionCount ||
		len(runtime.regionChildren) != regionCount ||
		len(runtime.environmentIncoming) != pointCount ||
		len(runtime.factorIncoming) != pointCount ||
		len(runtime.overlay.factorOutgoing) != pointCount {
		return nil, false
	}
	stateCount := runtime.stateCount()
	if stateCount <= 0 || !runtime.producerRows.valid() {
		return nil, false
	}
	if runtime.artifactBacked {
		if runtime.executionPlan == nil || !runtime.executionPlan.Available() || runtime.executionPlan.Schedule() == nil {
			return nil, false
		}
		if len(runtime.pointRegion) != stateCount || len(runtime.activeStates) != stateCount || len(runtime.stateExecutionEvents) == 0 {
			return nil, false
		}
	}
	work, ok := runtime.carrier.NewWork()
	if !ok {
		return nil, false
	}
	var demandEpoch *demand.Epoch
	opened := false
	defer func() {
		if opened {
			return
		}
		if demandEpoch != nil {
			demandEpoch.Discard()
		}
		work.Close()
	}()
	selectedPoints := make([]int, 0, len(runtime.activePoints))
	for pointIndex, active := range runtime.activePoints {
		if active {
			selectedPoints = append(selectedPoints, pointIndex)
		}
	}
	demandEpoch, ok = demand.OpenSelected(runtime.demand, selectedPoints)
	if !ok {
		return nil, false
	}
	empty, ok := support.FromGuard(runtime.carrier.Guards(), runtime.carrier.Guards().False())
	if !ok {
		return nil, false
	}
	relationRevision := uint64(relation.Generation())
	generatedCatalog := (*executioncatalog.Catalog)(nil)
	var generatedWorkers []generatedExecutionWorker
	if runtime.program.generatedPresent {
		executionProgram := runtime.program.generatedExecution
		if executionProgram == nil || executionProgram.catalog == nil || len(executionProgram.families) == 0 {
			return nil, false
		}
		generatedCatalog = executionProgram.catalog
		generatedWorkers = make([]generatedExecutionWorker, len(executionProgram.families))
		for index, family := range executionProgram.families {
			if family == nil || family.InputCapacity() < 0 || family.OutputCapacity() < 0 {
				return nil, false
			}
			run := execution.NewRun(family.InputCapacity(), family.OutputCapacity())
			executor := family.NewExecutor(run)
			if run == nil || executor == nil {
				return nil, false
			}
			generatedWorkers[index] = generatedExecutionWorker{run: run, executor: executor}
		}
	}
	epoch := &executorEpoch{
		runtime:           runtime,
		ctx:               ctx,
		work:              work,
		generatedCatalog:  generatedCatalog,
		generatedWorkers:  generatedWorkers,
		relationRevision:  relationRevision,
		generation:        relationRevision,
		demand:            demandEpoch,
		points:            make([]carrier.PointState, stateCount),
		producers:         make([]producerEpoch, len(runtime.producerRows.rows)),
		regions:           make([]regionEpoch, len(runtime.regions)),
		candidatesPending: make([]uint64, len(runtime.regions)),
		queue:             newPointQueue(stateCount),
		structuralDirty:   make([]bool, stateCount),
		structural: structuralEpoch{
			pointDescent: make([]uint64, stateCount),
		},
		postfixDirty:   make([]bool, stateCount),
		postfixPending: make([]int, 0, stateCount),
		frames:         make([]pointWTOFrame, 0, regionCount),
		nested:         make([]int, regionCount),
		regionScratch:  make([]int, 0, regionCount),
		currentState:   -1,
	}
	if runtime.artifactBacked {
		epoch.artifactProducerReads.byKey = make(map[artifactProducerReadKey][]stateGroupKey)
		epoch.artifactProducerReads.byFactor = make(map[artifactProducerReadFactorKey][]stateGroupKey)
	}
	epoch.terminal.Store(epochRunning)
	if !work.SetCheckpoint(epoch.checkpoint) {
		return nil, false
	}
	if !epoch.operands.open(runtime.operands) {
		return nil, false
	}
	for index, region := range runtime.regions {
		if !runtime.activeRegions[index] {
			if region.active {
				return nil, false
			}
			continue
		}
		if !region.active {
			return nil, false
		}
		epoch.regions[index].phase = phaseAscent
		epoch.regions[index].episode = 1
	}
	for stateIndex := 0; stateIndex < stateCount; stateIndex++ {
		if !runtime.activeState(stateIndex) {
			continue
		}
		point, pointIndex, _, pointOK := runtime.graphPointAtState(stateIndex)
		if !pointOK || stateIndex < 0 || stateIndex >= len(epoch.points) {
			return nil, false
		}
		if pointIndex >= len(runtime.pointScopes) || !runtime.pointScopes[pointIndex].Valid() {
			return nil, false
		}
		feasible := empty
		if point.HasInit() {
			feasible = runtime.pointInitials[pointIndex]
			if !feasible.Valid() {
				return nil, false
			}
		}
		state, initialized := carrier.NewState(runtime.carrier, runtime.pointScopes[pointIndex], feasible)
		if !initialized {
			return nil, false
		}
		emptyContribution, paired := work.EmptyContribution(state)
		if !paired {
			return nil, false
		}
		emptyRule, paired := work.AsRuleContribution(emptyContribution)
		if !paired {
			return nil, false
		}
		initial, paired := work.PointStateFromRuleContribution(emptyRule)
		if !paired {
			return nil, false
		}
		// The initial point is a base install, not a publication: it carries no
		// classified transition.
		if !epoch.installPoint(stateIndex, initial, change.Set{}) {
			return nil, false
		}
		if !epoch.markPostfixDirty(stateIndex) {
			return nil, false
		}
		for producerIndex := 0; producerIndex < runtime.graph.ProducerCount(point); producerIndex++ {
			group, groupOK := runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := runtime.graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(runtime.producers) || runtime.producers[groupIndex].group.Output() != point {
				return nil, false
			}
			cache, cacheOK := epoch.producerCache(contextfiber.StateOrdinal(stateIndex), groupIndex)
			if !cacheOK {
				return nil, false
			}
			if cache.generation == 0 {
				metadata := &runtime.producers[groupIndex]
				inputCount := metadata.group.InputCount()
				*cache = producerEpoch{state: contextfiber.StateOrdinal(stateIndex), group: groupIndex, generation: 1, inputs: make([]carrier.PointState, inputCount), inputStates: make([]carrier.State, inputCount), patches: make([]carrier.Patch, 0, metadata.span.count()), patchRows: make([]contributionPatch, 0, metadata.span.count()), reads: make([]demand.Observation, 0, len(metadata.reads))}
			}
			if !epoch.enqueuePoint(stateIndex) {
				return nil, false
			}
		}
		if runtime.artifactBacked {
			if !epoch.markStructuralState(stateIndex, point) {
				return nil, false
			}
		} else if !epoch.markStructuralPoint(point) {
			return nil, false
		}
	}
	// Every producer starts with generation 1 and applied 0. Seed each active
	// Region's counter from its immutable event-point interval; that interval
	// includes every nested descendant point, so a Group is intentionally
	// counted once for each active Region that encloses its output.
	for regionIndex, region := range runtime.regions {
		if !runtime.activeRegions[regionIndex] {
			continue
		}
		if !region.active {
			return nil, false
		}
		var pending uint64
		for _, pointIndex := range region.points {
			if pointIndex < 0 || pointIndex >= stateCount {
				return nil, false
			}
			point, _, _, pointOK := runtime.graphPointAtState(pointIndex)
			if !pointOK {
				return nil, false
			}
			producerCount := runtime.graph.ProducerCount(point)
			if producerCount < 0 || uint64(producerCount) > ^uint64(0)-pending {
				return nil, false
			}
			pending += uint64(producerCount)
		}
		epoch.candidatesPending[regionIndex] = pending
	}
	opened = true
	return epoch, true
}

func (runtime *solverRuntime) executionEventCount() int {
	if runtime == nil {
		return 0
	}
	if runtime.artifactBacked {
		if runtime.stateExecutionEvents != nil {
			return len(runtime.stateExecutionEvents)
		}
		execution := runtime.stateExecution
		if execution == nil && runtime.executionPlan != nil {
			execution = runtime.executionPlan.Schedule()
		}
		if execution == nil {
			return 0
		}
		return execution.EventCount()
	}
	if runtime.executionDemand != nil {
		return runtime.executionDemand.EventCount()
	}
	if runtime.points == nil {
		return 0
	}
	return runtime.points.EventCount()
}

func (runtime *solverRuntime) executionEventAt(index int) (schedule.Event, bool) {
	if runtime == nil {
		return schedule.Event{}, false
	}
	if runtime.artifactBacked {
		if runtime.stateExecutionEvents != nil {
			if index < 0 || index >= len(runtime.stateExecutionEvents) {
				return schedule.Event{}, false
			}
			return runtime.stateExecutionEvents[index], true
		}
		execution := runtime.stateExecution
		if execution == nil && runtime.executionPlan != nil {
			execution = runtime.executionPlan.Schedule()
		}
		if execution == nil {
			return schedule.Event{}, false
		}
		return execution.EventAt(index)
	}
	if runtime.executionDemand != nil {
		event, _, ok := runtime.executionDemand.EventAt(index)
		return event, ok
	}
	if runtime.points == nil {
		return schedule.Event{}, false
	}
	event, _, ok := runtime.points.EventAt(index)
	return event, ok
}

func (runtime *solverRuntime) executionRegionAt(index int) (schedule.Region, bool) {
	if runtime == nil {
		return schedule.Region{}, false
	}
	if runtime.artifactBacked {
		execution := runtime.stateExecution
		if execution == nil && runtime.executionPlan != nil {
			execution = runtime.executionPlan.Schedule()
		}
		if execution == nil {
			return schedule.Region{}, false
		}
		return execution.RegionAt(index)
	}
	if runtime.execution != nil {
		return runtime.execution.RegionAt(index)
	}
	if runtime.graph == nil || runtime.graph.Schedule() == nil {
		return schedule.Region{}, false
	}
	return runtime.graph.Schedule().RegionAt(index)
}

func (epoch *executorEpoch) discard() {
	if epoch == nil {
		return
	}
	epoch.incomplete()
	if epoch.demand != nil {
		epoch.demand.Discard()
		epoch.demand = nil
	}
	if epoch.work != nil {
		epoch.work.Close()
		epoch.work = nil
	}
	epoch.storePub = nil
}

// fail is the terminal cut of a publication path. It is called at every exit
// that returns false after a point state has been installed, so a half-landed
// multi-point commit can never be observed: canceled() gates every point
// visit, region visit and carrier operation in the executor. The flag makes
// the fail-stop structural rather than a convention the callers happen to
// honour.
func (epoch *executorEpoch) fail() bool {
	if epoch != nil {
		epoch.failed = true
	}
	return false
}

// canceledByContext is the withdrawal-only status probe. Terminal-status and
// failure-recording sites ask this question: a fail-stopped epoch stopped
// because it refused its own work, which is an execution failure and must
// still be recorded as one.
func (epoch *executorEpoch) canceledByContext() bool {
	if epoch == nil || epoch.ctx == nil || epoch.terminal.Load() != epochRunning {
		return true
	}
	if epoch.ctx.Err() == nil {
		return false
	}
	epoch.terminal.CompareAndSwap(epochRunning, epochIncomplete)
	return true
}

// canceled is the liveness probe every operation consults. It refuses a
// withdrawn epoch and a fail-stopped one alike.
func (epoch *executorEpoch) canceled() bool {
	return epoch.canceledByContext() || epoch != nil && epoch.failed
}

// checkpoint is the evaluator liveness probe shared by ordinary carrier work
// and nested rule/query frames. It refuses on withdrawal and on the
// publication fail-stop alike, because neither leaves work worth continuing.
func (epoch *executorEpoch) checkpoint() bool {
	return epoch != nil && !epoch.canceled()
}

func (epoch *executorEpoch) incomplete() {
	if epoch != nil {
		epoch.terminal.CompareAndSwap(epochRunning, epochIncomplete)
	}
}

func (epoch *executorEpoch) complete() bool {
	if epoch == nil || epoch.ctx == nil || epoch.ctx.Err() != nil {
		if epoch != nil {
			epoch.incomplete()
		}
		return false
	}
	return epoch.terminal.CompareAndSwap(epochRunning, epochCompleted)
}

// dropPostfixProof drops the relation certificate a recomputed exact RHS
// invalidates. The certificate is one clock stamp, so there is no revision
// counter left to advance beside it.
func (episode *regionEpoch) dropPostfixProof() bool {
	if episode == nil {
		return false
	}
	episode.postfixAt = 0
	return true
}
