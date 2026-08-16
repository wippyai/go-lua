package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
)

// SolveDiagnosticFlags selects the solve-local aggregate diagnostics. A zero
// value preserves ordinary solver semantics and allocates no aggregate
// diagnostic collector.
type SolveDiagnosticFlags uint32

const (
	SolveDiagnosticSchedule SolveDiagnosticFlags = 1 << iota
	SolveDiagnosticRestart
	SolveDiagnosticPublication
	SolveDiagnosticFold
	SolveDiagnosticAll = SolveDiagnosticSchedule | SolveDiagnosticRestart | SolveDiagnosticPublication | SolveDiagnosticFold
)

// SolveDiagnosticOptions controls one call to Solver.SolveWithDiagnostics.
// MaxRows bounds the detached restart aggregate; zero selects the default.
type SolveDiagnosticOptions struct {
	Flags   SolveDiagnosticFlags
	MaxRows int
	// MaxWork is an opt-in diagnostic checkpoint budget. It begins only after
	// epoch construction has completed, then stops the diagnostic solve with
	// SolveIncomplete once existing deep liveness probes consume the budget.
	// It is not a semantic solver limit; ordinary Solve and MaxWork zero are
	// unchanged.
	MaxWork uint64
}

// Valid accepts only the closed diagnostic vocabulary and explicit bounded
// resource settings. Invalid options are rejected before solver execution;
// callers never get silently clamped or reinterpreted diagnostics.
func (options SolveDiagnosticOptions) Valid() bool {
	return options.Flags&^SolveDiagnosticAll == 0 && options.MaxRows >= 0 && options.MaxRows <= maxSolveDiagnosticMaxRows &&
		(options.Flags != 0 || options.MaxRows == 0 && options.MaxWork == 0)
}

// SolveDiagnosticKind identifies the kind of event represented by a row.
// Keeping the kind in the detached row makes restart and localized refresh
// evidence independently aggregatable without strings.
type SolveDiagnosticKind uint8

const (
	SolveDiagnosticKindRestart SolveDiagnosticKind = iota + 1
	SolveDiagnosticKindInterfaceRefresh
)

// SolveDiagnosticRestartCallSite identifies the executor branch that began a
// fresh recurrence episode.
type SolveDiagnosticRestartCallSite uint8

const (
	SolveDiagnosticRestartCandidateInterface SolveDiagnosticRestartCallSite = iota + 1
	SolveDiagnosticRestartHeadInterface
	SolveDiagnosticRestartAscentIngress
	SolveDiagnosticRestartNarrowExact
	SolveDiagnosticRestartNarrowCurrent
	SolveDiagnosticRestartPostfixExact
)

// SolveDiagnosticRestartReason identifies the comparison which required a
// fresh exact episode.
type SolveDiagnosticRestartReason uint8

const (
	SolveDiagnosticRestartCandidateNotOrdered SolveDiagnosticRestartReason = iota + 1
	SolveDiagnosticRestartInterfaceChanged
	SolveDiagnosticRestartIngressNotBelowCurrent
	SolveDiagnosticRestartExactIncomparable
	SolveDiagnosticRestartExactNotBelowCurrent
)

// SolveDiagnosticDirection is the semantic order observed between the
// remembered source and its current value at a restart trigger. RawOnly is a
// representation change with equal lifted meaning.
type SolveDiagnosticDirection uint8

const (
	SolveDiagnosticDirectionUnknown SolveDiagnosticDirection = iota
	SolveDiagnosticDirectionOldLessEqNew
	SolveDiagnosticDirectionNewLessEqOld
	SolveDiagnosticDirectionEqual
	SolveDiagnosticDirectionRawOnly
	SolveDiagnosticDirectionIncomparable
)

// SolveDiagnosticRegionPhase identifies the recurrence phase in which a
// restart was requested.
type SolveDiagnosticRegionPhase uint8

const (
	SolveDiagnosticRegionAscent SolveDiagnosticRegionPhase = iota + 1
	SolveDiagnosticRegionNarrow
)

// SolveDiagnosticRow is one bounded aggregate restart bucket. Rows are keyed
// by revision, call site, reason, region, and head; all counters are sums of
// events in that bucket. It deliberately retains no runtime or carrier value.
type SolveDiagnosticRow struct {
	Revision uint64
	Kind     SolveDiagnosticKind
	CallSite SolveDiagnosticRestartCallSite
	Reason   SolveDiagnosticRestartReason
	Phase    SolveDiagnosticRegionPhase
	Region   int
	Head     int

	Attempts  uint64
	Completed uint64

	SubtreePoints  uint64
	ResetPoints    uint64
	ResetProducers uint64

	FaceIngressChanged                uint64
	ExternalProducerIngressChanged    uint64
	BackProducerIngressChanged        uint64
	ExternalEnvironmentIngressChanged uint64
	BackEnvironmentIngressChanged     uint64
	ExternalFactorIngressChanged      uint64
	BackFactorIngressChanged          uint64

	ExternalOrderFailures uint64
	BackOrderFailures     uint64

	RepresentationResets     uint64
	RepresentationOnlyResets uint64
	SemanticResets           uint64
	SemanticSupportResets    uint64
	SemanticValueResets      uint64

	DirectionOldLessEqNew uint64
	DirectionNewLessEqOld uint64
	DirectionEqual        uint64
	DirectionRawOnly      uint64
	DirectionIncomparable uint64
	DirectionUnknown      uint64

	InterfaceRefreshes           uint64
	InterfaceRefreshCompleted    uint64
	InterfaceRefreshFallbacks    uint64
	InterfaceRefreshChangedFaces uint64
	InterfaceRefreshOldLessEqNew uint64
	InterfaceRefreshNewLessEqOld uint64
	InterfaceRefreshEqual        uint64
	InterfaceRefreshIncomparable uint64
	InterfaceRefreshUnknown      uint64
}

// SolveDiagnostics is a detached snapshot of one solve's bounded runtime
// counters. Its slices are owned by the returned value and no solver, epoch,
// callback, or carrier handle is retained.
type SolveDiagnostics struct {
	Flags SolveDiagnosticFlags
	// Failure is the existing detached first-incomplete certificate issued by
	// this same solver invocation. It is zero for complete, canceled,
	// panicked, and intentional WorkCutoff calls.
	Failure SolveReport
	// Work is the number of consumed deep evaluator liveness probes, not an
	// outer iteration or event count. WorkCutoff distinguishes an intentional
	// bounded diagnostic stop from a semantic incomplete solve.
	Work       uint64
	MaxWork    uint64
	WorkCutoff bool

	Epochs           uint64
	Revisions        uint64
	EpochPasses      uint64
	Refreshes        uint64
	Evaluates        uint64
	EvaluateFailures uint64
	Folds            uint64
	RegionRHS        uint64
	Restarts         uint64
	Activations      uint64

	MaxQueue   uint64
	MaxEpisode uint64

	Publications         uint64
	SemanticPublications uint64
	RawPublications      uint64
	RawOnlyPublications  uint64
	VersionBumps         uint64
	SemanticWakes        uint64
	CoverageWakes        uint64

	InterfaceRefreshes           uint64
	InterfaceRefreshCompleted    uint64
	InterfaceRefreshFallbacks    uint64
	InterfaceRefreshChangedFaces uint64
	InterfaceRefreshOldLessEqNew uint64
	InterfaceRefreshNewLessEqOld uint64
	InterfaceRefreshEqual        uint64
	InterfaceRefreshIncomparable uint64
	InterfaceRefreshUnknown      uint64

	DroppedRows      uint64
	DroppedSnapshots uint64
	Rows             []SolveDiagnosticRow
}

const (
	defaultSolveDiagnosticMaxRows = 128
	maxSolveDiagnosticMaxRows     = 4096
)

type solveDiagnosticState struct {
	flags   SolveDiagnosticFlags
	maxRows int
	maxWork uint64
	work    uint64
	cutoff  bool

	epochs           uint64
	revisions        uint64
	epochPasses      uint64
	refreshes        uint64
	evaluates        uint64
	evaluateFailures uint64
	folds            uint64
	regionRHS        uint64
	restarts         uint64
	activations      uint64
	maxQueue         uint64
	maxEpisode       uint64

	publications                 uint64
	semanticPublications         uint64
	rawPublications              uint64
	rawOnlyPublications          uint64
	versionBumps                 uint64
	semanticWakes                uint64
	coverageWakes                uint64
	interfaceRefreshes           uint64
	interfaceRefreshCompleted    uint64
	interfaceRefreshFallbacks    uint64
	interfaceRefreshChangedFaces uint64
	interfaceRefreshOldLessEqNew uint64
	interfaceRefreshNewLessEqOld uint64
	interfaceRefreshEqual        uint64
	interfaceRefreshIncomparable uint64
	interfaceRefreshUnknown      uint64

	rows        []SolveDiagnosticRow
	rowAt       map[solveDiagnosticRowKey]int
	droppedRows uint64

	snapshots        map[solveDiagnosticInputKey]solveDiagnosticInput
	maxSnapshots     int
	droppedSnapshots uint64
}

type solveDiagnosticRowKey struct {
	revision uint64
	kind     SolveDiagnosticKind
	callSite SolveDiagnosticRestartCallSite
	reason   SolveDiagnosticRestartReason
	phase    SolveDiagnosticRegionPhase
	region   int
	head     int
}

type solveDiagnosticInputKind uint8

const (
	solveDiagnosticInputFace solveDiagnosticInputKind = iota + 1
	solveDiagnosticInputExternalProducer
	solveDiagnosticInputBackProducer
	solveDiagnosticInputExternalEnvironment
	solveDiagnosticInputBackEnvironment
	solveDiagnosticInputExternalFactor
	solveDiagnosticInputBackFactor
)

type solveDiagnosticInputKey struct {
	region int
	kind   solveDiagnosticInputKind
	index  int
}

type solveDiagnosticInput struct {
	point   carrier.PointState
	pointOK bool
	rule    carrier.RuleContribution
	ruleOK  bool
}

type solveDiagnosticRestartSample struct {
	key solveDiagnosticRowKey

	attempts  uint64
	completed uint64

	subtreePoints  uint64
	resetPoints    uint64
	resetProducers uint64

	faceIngressChanged                uint64
	externalProducerIngressChanged    uint64
	backProducerIngressChanged        uint64
	externalEnvironmentIngressChanged uint64
	backEnvironmentIngressChanged     uint64
	externalFactorIngressChanged      uint64
	backFactorIngressChanged          uint64

	externalOrderFailures uint64
	backOrderFailures     uint64

	representationResets     uint64
	representationOnlyResets uint64
	semanticResets           uint64
	semanticSupportResets    uint64
	semanticValueResets      uint64

	directionOldLessEqNew uint64
	directionNewLessEqOld uint64
	directionEqual        uint64
	directionRawOnly      uint64
	directionIncomparable uint64
	directionUnknown      uint64
}

func newSolveDiagnosticState(options SolveDiagnosticOptions) *solveDiagnosticState {
	if !options.Valid() {
		return nil
	}
	flags := options.Flags & SolveDiagnosticAll
	if flags == 0 {
		return nil
	}
	maxRows := options.MaxRows
	if maxRows <= 0 {
		maxRows = defaultSolveDiagnosticMaxRows
	}
	maxSnapshots := maxRows * 16
	if maxSnapshots < 64 {
		maxSnapshots = 64
	}
	if maxSnapshots > 65536 {
		maxSnapshots = 65536
	}
	return &solveDiagnosticState{
		flags:        flags,
		maxRows:      maxRows,
		maxWork:      options.MaxWork,
		rows:         make([]SolveDiagnosticRow, 0, maxRows),
		rowAt:        make(map[solveDiagnosticRowKey]int, maxRows),
		snapshots:    make(map[solveDiagnosticInputKey]solveDiagnosticInput, maxSnapshots),
		maxSnapshots: maxSnapshots,
	}
}

// checkpoint is installed at the evaluator's existing liveness seam. The
// disabled path never creates this state; the enabled unlimited path is one
// predictable zero comparison. It does not alter any completed semantics.
func (diagnostics *solveDiagnosticState) checkpoint() bool {
	if diagnostics == nil || diagnostics.maxWork == 0 {
		return true
	}
	if diagnostics.work >= diagnostics.maxWork {
		diagnostics.cutoff = true
		return false
	}
	diagnostics.work++
	return true
}

func (diagnostics *solveDiagnosticState) clearCutoff() {
	if diagnostics != nil {
		diagnostics.cutoff = false
	}
}

func (diagnostics *solveDiagnosticState) scheduleEnabled() bool {
	return diagnostics != nil && diagnostics.flags&SolveDiagnosticSchedule != 0
}

func (diagnostics *solveDiagnosticState) restartEnabled() bool {
	return diagnostics != nil && diagnostics.flags&SolveDiagnosticRestart != 0
}

func (diagnostics *solveDiagnosticState) publicationEnabled() bool {
	return diagnostics != nil && diagnostics.flags&SolveDiagnosticPublication != 0
}

func (diagnostics *solveDiagnosticState) foldEnabled() bool {
	return diagnostics != nil && diagnostics.flags&SolveDiagnosticFold != 0
}

func (diagnostics *solveDiagnosticState) epochStarted(epoch *executorEpoch, revision uint64) {
	if diagnostics == nil {
		return
	}
	diagnostics.epochs++
	diagnostics.revisions = revision + 1
	if epoch == nil {
		return
	}
	diagnostics.observeQueue(epoch.queue.count)
	for index := range epoch.regions {
		if epoch.activeRegion(index) {
			diagnostics.observeEpisode(epoch.regions[index].episode)
		}
	}
}

func (diagnostics *solveDiagnosticState) observeRevision(revision uint64) {
	if diagnostics == nil {
		return
	}
	if revision == ^uint64(0) {
		diagnostics.revisions = ^uint64(0)
		return
	}
	diagnostics.revisions = revision + 1
}

func (diagnostics *solveDiagnosticState) resetRevisionEvidence() {
	if diagnostics != nil && diagnostics.restartEnabled() {
		clear(diagnostics.snapshots)
	}
}

func (diagnostics *solveDiagnosticState) observeEpisode(episode uint64) {
	if diagnostics != nil && episode > diagnostics.maxEpisode {
		diagnostics.maxEpisode = episode
	}
}

func (diagnostics *solveDiagnosticState) observeQueue(size int) {
	if diagnostics != nil && size >= 0 && uint64(size) > diagnostics.maxQueue {
		diagnostics.maxQueue = uint64(size)
	}
}

func (diagnostics *solveDiagnosticState) snapshot() SolveDiagnostics {
	if diagnostics == nil {
		return SolveDiagnostics{}
	}
	rows := append([]SolveDiagnosticRow(nil), diagnostics.rows...)
	sort.Slice(rows, func(left, right int) bool {
		a, b := rows[left], rows[right]
		if a.Revision != b.Revision {
			return a.Revision < b.Revision
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.CallSite != b.CallSite {
			return a.CallSite < b.CallSite
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		if a.Phase != b.Phase {
			return a.Phase < b.Phase
		}
		if a.Region != b.Region {
			return a.Region < b.Region
		}
		return a.Head < b.Head
	})
	return SolveDiagnostics{
		Flags:                        diagnostics.flags,
		Work:                         diagnostics.work,
		MaxWork:                      diagnostics.maxWork,
		WorkCutoff:                   diagnostics.cutoff,
		Epochs:                       diagnostics.epochs,
		Revisions:                    diagnostics.revisions,
		EpochPasses:                  diagnostics.epochPasses,
		Refreshes:                    diagnostics.refreshes,
		Evaluates:                    diagnostics.evaluates,
		EvaluateFailures:             diagnostics.evaluateFailures,
		Folds:                        diagnostics.folds,
		RegionRHS:                    diagnostics.regionRHS,
		Restarts:                     diagnostics.restarts,
		Activations:                  diagnostics.activations,
		MaxQueue:                     diagnostics.maxQueue,
		MaxEpisode:                   diagnostics.maxEpisode,
		Publications:                 diagnostics.publications,
		SemanticPublications:         diagnostics.semanticPublications,
		RawPublications:              diagnostics.rawPublications,
		RawOnlyPublications:          diagnostics.rawOnlyPublications,
		VersionBumps:                 diagnostics.versionBumps,
		SemanticWakes:                diagnostics.semanticWakes,
		CoverageWakes:                diagnostics.coverageWakes,
		InterfaceRefreshes:           diagnostics.interfaceRefreshes,
		InterfaceRefreshCompleted:    diagnostics.interfaceRefreshCompleted,
		InterfaceRefreshFallbacks:    diagnostics.interfaceRefreshFallbacks,
		InterfaceRefreshChangedFaces: diagnostics.interfaceRefreshChangedFaces,
		InterfaceRefreshOldLessEqNew: diagnostics.interfaceRefreshOldLessEqNew,
		InterfaceRefreshNewLessEqOld: diagnostics.interfaceRefreshNewLessEqOld,
		InterfaceRefreshEqual:        diagnostics.interfaceRefreshEqual,
		InterfaceRefreshIncomparable: diagnostics.interfaceRefreshIncomparable,
		InterfaceRefreshUnknown:      diagnostics.interfaceRefreshUnknown,
		DroppedRows:                  diagnostics.droppedRows,
		DroppedSnapshots:             diagnostics.droppedSnapshots,
		Rows:                         rows,
	}
}

func (diagnostics *solveDiagnosticState) recordRefresh() {
	if diagnostics != nil && diagnostics.scheduleEnabled() {
		diagnostics.refreshes++
	}
}

func (diagnostics *solveDiagnosticState) recordEvaluate(ok *bool) {
	if diagnostics == nil || !diagnostics.scheduleEnabled() {
		return
	}
	diagnostics.evaluates++
	if ok != nil && !*ok {
		diagnostics.evaluateFailures++
	}
}

func (diagnostics *solveDiagnosticState) recordFold() {
	if diagnostics != nil && diagnostics.foldEnabled() {
		diagnostics.folds++
	}
}

func (diagnostics *solveDiagnosticState) recordRegionRHS() {
	if diagnostics != nil && diagnostics.foldEnabled() {
		diagnostics.regionRHS++
	}
}

func (diagnostics *solveDiagnosticState) recordPass(frameCount int) {
	if diagnostics == nil || !diagnostics.scheduleEnabled() {
		return
	}
	diagnostics.epochPasses++
	_ = frameCount
}

func (diagnostics *solveDiagnosticState) recordActivation(count int) {
	if diagnostics != nil && count > 0 {
		diagnostics.activations += uint64(count)
	}
}

func (diagnostics *solveDiagnosticState) recordPublication(semantic, raw bool) {
	if diagnostics == nil || !diagnostics.publicationEnabled() {
		return
	}
	diagnostics.publications++
	if semantic {
		diagnostics.semanticPublications++
	}
	if raw {
		diagnostics.rawPublications++
	}
	if raw && !semantic {
		diagnostics.rawOnlyPublications++
	}
}

func (diagnostics *solveDiagnosticState) recordVersionBump() {
	if diagnostics != nil && diagnostics.publicationEnabled() {
		diagnostics.versionBumps++
	}
}

func (diagnostics *solveDiagnosticState) recordWakes(semantic, coverage int) {
	if diagnostics == nil || !diagnostics.publicationEnabled() {
		return
	}
	if semantic > 0 {
		diagnostics.semanticWakes += uint64(semantic)
	}
	if coverage > 0 {
		diagnostics.coverageWakes += uint64(coverage)
	}
}

func (diagnostics *solveDiagnosticState) interfaceRefreshRow(epoch *executorEpoch, region int) *SolveDiagnosticRow {
	if diagnostics == nil || !diagnostics.restartEnabled() || epoch == nil || epoch.runtime == nil || region < 0 || region >= len(epoch.runtime.regions) || region >= len(epoch.regions) {
		return nil
	}
	state := epoch.regions[region]
	key := solveDiagnosticRowKey{revision: epoch.diagnosticRevision, kind: SolveDiagnosticKindInterfaceRefresh, callSite: SolveDiagnosticRestartHeadInterface, reason: SolveDiagnosticRestartInterfaceChanged, region: region, head: epoch.runtime.regions[region].head}
	if state.phase == phaseAscent {
		key.phase = SolveDiagnosticRegionAscent
	} else if state.phase == phaseNarrow {
		key.phase = SolveDiagnosticRegionNarrow
	}
	index, exists := diagnostics.rowAt[key]
	if !exists {
		if len(diagnostics.rows) >= diagnostics.maxRows {
			diagnostics.droppedRows++
			return nil
		}
		index = len(diagnostics.rows)
		diagnostics.rowAt[key] = index
		diagnostics.rows = append(diagnostics.rows, SolveDiagnosticRow{Revision: key.revision, Kind: key.kind, CallSite: key.callSite, Reason: key.reason, Phase: key.phase, Region: key.region, Head: key.head})
	}
	return &diagnostics.rows[index]
}

func (diagnostics *solveDiagnosticState) recordInterfaceRefreshBegin(epoch *executorEpoch, region int, changedFaces uint64) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	diagnostics.interfaceRefreshes++
	diagnostics.interfaceRefreshChangedFaces += changedFaces
	row := diagnostics.interfaceRefreshRow(epoch, region)
	if row == nil {
		return
	}
	row.InterfaceRefreshes++
	row.InterfaceRefreshChangedFaces += changedFaces
}

func interfaceRefreshDirection(work *carrier.Work, old, current carrier.PointRHS) SolveDiagnosticDirection {
	if work == nil || !work.OwnsPointRHS(old) || !work.OwnsPointRHS(current) {
		return SolveDiagnosticDirectionUnknown
	}
	if work.EqualPointRHS(old, current) {
		// PointRHS deliberately exposes only lifted semantic order. Raw-only
		// representation is source-publication telemetry, not an exact-RHS
		// direction and must not acquire a second carrier predicate here.
		return SolveDiagnosticDirectionEqual
	}
	oldLess := work.LessOrEqPointRHS(old, current)
	newLess := work.LessOrEqPointRHS(current, old)
	switch {
	case oldLess && !newLess:
		return SolveDiagnosticDirectionOldLessEqNew
	case newLess && !oldLess:
		return SolveDiagnosticDirectionNewLessEqOld
	case oldLess && newLess:
		return SolveDiagnosticDirectionEqual
	default:
		return SolveDiagnosticDirectionIncomparable
	}
}

func (diagnostics *solveDiagnosticState) recordInterfaceRefreshOutcome(epoch *executorEpoch, region int, old, current carrier.PointRHS, completed, fallback bool) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	if completed {
		diagnostics.interfaceRefreshCompleted++
	}
	if fallback {
		diagnostics.interfaceRefreshFallbacks++
	}
	var work *carrier.Work
	if epoch != nil {
		work = epoch.work
	}
	direction := interfaceRefreshDirection(work, old, current)
	switch direction {
	case SolveDiagnosticDirectionOldLessEqNew:
		diagnostics.interfaceRefreshOldLessEqNew++
	case SolveDiagnosticDirectionNewLessEqOld:
		diagnostics.interfaceRefreshNewLessEqOld++
	case SolveDiagnosticDirectionEqual:
		diagnostics.interfaceRefreshEqual++
	case SolveDiagnosticDirectionIncomparable:
		diagnostics.interfaceRefreshIncomparable++
	default:
		diagnostics.interfaceRefreshUnknown++
	}
	row := diagnostics.interfaceRefreshRow(epoch, region)
	if row == nil {
		return
	}
	if completed {
		row.InterfaceRefreshCompleted++
	}
	if fallback {
		row.InterfaceRefreshFallbacks++
	}
	switch direction {
	case SolveDiagnosticDirectionOldLessEqNew:
		row.InterfaceRefreshOldLessEqNew++
	case SolveDiagnosticDirectionNewLessEqOld:
		row.InterfaceRefreshNewLessEqOld++
	case SolveDiagnosticDirectionEqual:
		row.InterfaceRefreshEqual++
	case SolveDiagnosticDirectionIncomparable:
		row.InterfaceRefreshIncomparable++
	default:
		row.InterfaceRefreshUnknown++
	}
}

func (diagnostics *solveDiagnosticState) rememberPoint(key solveDiagnosticInputKey, point carrier.PointState) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	previous, exists := diagnostics.snapshots[key]
	if !exists && len(diagnostics.snapshots) >= diagnostics.maxSnapshots {
		diagnostics.droppedSnapshots++
		return
	}
	previous.point = point
	previous.pointOK = true
	diagnostics.snapshots[key] = previous
}

func (diagnostics *solveDiagnosticState) rememberRule(key solveDiagnosticInputKey, rule carrier.RuleContribution, valid bool) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	previous, exists := diagnostics.snapshots[key]
	if !exists && len(diagnostics.snapshots) >= diagnostics.maxSnapshots {
		diagnostics.droppedSnapshots++
		return
	}
	previous.rule = rule
	previous.ruleOK = valid
	diagnostics.snapshots[key] = previous
}

func (diagnostics *solveDiagnosticState) rememberRegionInterfaces(epoch *executorEpoch, region int) {
	if diagnostics == nil || !diagnostics.restartEnabled() || epoch == nil || epoch.runtime == nil || !epoch.activeRegion(region) {
		return
	}
	bound := epoch.runtime.regions[region]
	for index, point := range bound.faces {
		if point >= 0 && point < len(epoch.points) && epoch.work.OwnsPointState(epoch.points[point]) {
			diagnostics.rememberPoint(solveDiagnosticInputKey{region: region, kind: solveDiagnosticInputFace, index: index}, epoch.points[point])
		}
	}
	diagnostics.rememberProducerInterfaces(epoch, region, bound.external, solveDiagnosticInputExternalProducer)
	diagnostics.rememberProducerInterfaces(epoch, region, bound.back, solveDiagnosticInputBackProducer)
	diagnostics.rememberEnvironmentInterfaces(epoch, region, bound.environmentBack, solveDiagnosticInputBackEnvironment)
	diagnostics.rememberFactorInterfaces(epoch, region, bound.factorBack, solveDiagnosticInputBackFactor)
}

func (diagnostics *solveDiagnosticState) rememberProducerInterfaces(epoch *executorEpoch, region int, groups []int, kind solveDiagnosticInputKind) {
	for index, group := range groups {
		if group < 0 || group >= len(epoch.producers) {
			continue
		}
		cache := epoch.producers[group]
		diagnostics.rememberRule(solveDiagnosticInputKey{region: region, kind: kind, index: index}, cache.candidate, cache.hasValue && epoch.work.OwnsRuleContribution(cache.candidate))
	}
}

func (diagnostics *solveDiagnosticState) rememberEnvironmentInterfaces(epoch *executorEpoch, region int, edges []int, kind solveDiagnosticInputKind) {
	for index, edgeIndex := range edges {
		if edgeIndex < 0 || edgeIndex >= len(epoch.runtime.environments) {
			continue
		}
		source := epoch.runtime.environments[edgeIndex].source
		if source >= 0 && source < len(epoch.points) && epoch.work.OwnsPointState(epoch.points[source]) {
			diagnostics.rememberPoint(solveDiagnosticInputKey{region: region, kind: kind, index: index}, epoch.points[source])
		}
	}
}

func (diagnostics *solveDiagnosticState) rememberFactorInterfaces(epoch *executorEpoch, region int, edges []int, kind solveDiagnosticInputKind) {
	for index, edgeIndex := range edges {
		if edgeIndex < 0 || edgeIndex >= len(epoch.runtime.factorEdges) {
			continue
		}
		source := epoch.runtime.factorEdges[edgeIndex].source
		if source >= 0 && source < len(epoch.points) && epoch.work.OwnsPointState(epoch.points[source]) {
			diagnostics.rememberPoint(solveDiagnosticInputKey{region: region, kind: kind, index: index}, epoch.points[source])
		}
	}
}

func (diagnostics *solveDiagnosticState) beginRestart(epoch *executorEpoch, region int, callSite SolveDiagnosticRestartCallSite, reason SolveDiagnosticRestartReason, pendingGroup int, pending carrier.RuleContribution) solveDiagnosticRestartSample {
	sample := solveDiagnosticRestartSample{
		key: solveDiagnosticRowKey{
			revision: epoch.diagnosticRevision,
			kind:     SolveDiagnosticKindRestart,
			callSite: callSite,
			reason:   reason,
			region:   region,
			head:     -1,
		},
		attempts: 1,
	}
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return sample
	}
	diagnostics.restarts++
	if epoch == nil || epoch.runtime == nil || region < 0 || region >= len(epoch.runtime.regions) || region >= len(epoch.regions) {
		return sample
	}
	bound, state := epoch.runtime.regions[region], epoch.regions[region]
	sample.key.head = bound.head
	if state.phase == phaseAscent {
		sample.key.phase = SolveDiagnosticRegionAscent
	} else if state.phase == phaseNarrow {
		sample.key.phase = SolveDiagnosticRegionNarrow
	}
	sample.subtreePoints = uint64(len(bound.points))
	diagnostics.captureRestartMismatches(epoch, region, bound, state, pendingGroup, pending, &sample)
	switch callSite {
	case SolveDiagnosticRestartAscentIngress:
		sample.externalOrderFailures = 1
	case SolveDiagnosticRestartNarrowExact, SolveDiagnosticRestartNarrowCurrent, SolveDiagnosticRestartPostfixExact:
		sample.backOrderFailures = 1
	}
	return sample
}

func (diagnostics *solveDiagnosticState) captureRestartMismatches(epoch *executorEpoch, region int, bound runtimeRegion, state regionEpoch, pendingGroup int, pending carrier.RuleContribution, sample *solveDiagnosticRestartSample) {
	version := func(point int) uint64 {
		if point < 0 || point >= len(epoch.versions) {
			return 0
		}
		return epoch.versions[point]
	}
	if len(bound.faces) != len(state.interfaces) {
		sample.faceIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, point := range bound.faces {
		if index >= len(state.interfaces) || state.interfaces[index] != version(point) {
			sample.faceIngressChanged++
			diagnostics.classifyPoint(epoch, solveDiagnosticInputKey{region: region, kind: solveDiagnosticInputFace, index: index}, point, sample)
		}
	}
	if len(bound.external) != len(state.ingress) {
		sample.externalProducerIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, group := range bound.external {
		current := uint64(0)
		if group >= 0 && group < len(epoch.producers) {
			current = epoch.producers[group].version
		}
		if index >= len(state.ingress) || state.ingress[index] != current {
			sample.externalProducerIngressChanged++
			diagnostics.classifyRule(epoch, solveDiagnosticInputKey{region: region, kind: solveDiagnosticInputExternalProducer, index: index}, group, pendingGroup, pending, sample)
		}
	}
	if len(bound.back) != len(state.backIngress) {
		sample.backProducerIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, group := range bound.back {
		current := uint64(0)
		if group >= 0 && group < len(epoch.producers) {
			current = epoch.producers[group].version
		}
		if index >= len(state.backIngress) || state.backIngress[index] != current {
			sample.backProducerIngressChanged++
			diagnostics.classifyRule(epoch, solveDiagnosticInputKey{region: region, kind: solveDiagnosticInputBackProducer, index: index}, group, pendingGroup, pending, sample)
		}
	}
	if len(bound.environmentExternal) != len(state.environmentIngress) {
		sample.externalEnvironmentIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, edge := range bound.environmentExternal {
		current := epoch.environmentVersion(edge)
		if index >= len(state.environmentIngress) || state.environmentIngress[index] != current {
			sample.externalEnvironmentIngressChanged++
		}
	}
	if len(bound.environmentBack) != len(state.environmentBackIngress) {
		sample.backEnvironmentIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, edge := range bound.environmentBack {
		current := epoch.environmentVersion(edge)
		if index >= len(state.environmentBackIngress) || state.environmentBackIngress[index] != current {
			sample.backEnvironmentIngressChanged++
			diagnostics.classifyEnvironment(epoch, solveDiagnosticInputKey{region: region, kind: solveDiagnosticInputBackEnvironment, index: index}, edge, sample)
		}
	}
	if len(bound.factorExternal) != len(state.factorIngress) {
		sample.externalFactorIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, edge := range bound.factorExternal {
		current := epoch.factorEdgeVersion(edge)
		if index >= len(state.factorIngress) || state.factorIngress[index] != current {
			sample.externalFactorIngressChanged++
		}
	}
	if len(bound.factorBack) != len(state.factorBackIngress) {
		sample.backFactorIngressChanged++
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
	}
	for index, edge := range bound.factorBack {
		current := epoch.factorEdgeVersion(edge)
		if index >= len(state.factorBackIngress) || state.factorBackIngress[index] != current {
			sample.backFactorIngressChanged++
			diagnostics.classifyFactor(epoch, solveDiagnosticInputKey{region: region, kind: solveDiagnosticInputBackFactor, index: index}, edge, sample)
		}
	}
}

func (diagnostics *solveDiagnosticState) classifyPoint(epoch *executorEpoch, key solveDiagnosticInputKey, point int, sample *solveDiagnosticRestartSample) {
	if point < 0 || point >= len(epoch.points) {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	old, ok := diagnostics.snapshots[key]
	if !ok || !old.pointOK || !epoch.work.OwnsPointState(old.point) || !epoch.work.OwnsPointState(epoch.points[point]) {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.addPointDirection(epoch.work, old.point, epoch.points[point], sample)
}

func (diagnostics *solveDiagnosticState) classifyRule(epoch *executorEpoch, key solveDiagnosticInputKey, group, pendingGroup int, pending carrier.RuleContribution, sample *solveDiagnosticRestartSample) {
	if group < 0 || group >= len(epoch.producers) {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	old, ok := diagnostics.snapshots[key]
	current := epoch.producers[group]
	if group == pendingGroup && epoch.work.OwnsRuleContribution(pending) {
		current.candidate = pending
		current.hasValue = true
	}
	if !ok || !old.ruleOK || !current.hasValue || !epoch.work.OwnsRuleContribution(old.rule) || !epoch.work.OwnsRuleContribution(current.candidate) {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.addRuleDirection(epoch.work, old.rule, current.candidate, sample)
}

func (diagnostics *solveDiagnosticState) classifyEnvironment(epoch *executorEpoch, key solveDiagnosticInputKey, edge int, sample *solveDiagnosticRestartSample) {
	if edge < 0 || edge >= len(epoch.runtime.environments) {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.classifyPoint(epoch, key, epoch.runtime.environments[edge].source, sample)
}

func (diagnostics *solveDiagnosticState) classifyFactor(epoch *executorEpoch, key solveDiagnosticInputKey, edge int, sample *solveDiagnosticRestartSample) {
	if edge < 0 || edge >= len(epoch.runtime.factorEdges) {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.classifyPoint(epoch, key, epoch.runtime.factorEdges[edge].source, sample)
}

func (diagnostics *solveDiagnosticState) addPointDirection(work *carrier.Work, old, current carrier.PointState, sample *solveDiagnosticRestartSample) {
	if work.EqualPointState(old, current) {
		if work.ExactSamePointRepresentation(old, current) {
			diagnostics.addDirection(SolveDiagnosticDirectionEqual, sample)
		} else {
			diagnostics.addDirection(SolveDiagnosticDirectionRawOnly, sample)
		}
		return
	}
	oldRHS, oldOK := work.PointRHSFromPointState(old)
	newRHS, newOK := work.PointRHSFromPointState(current)
	if !oldOK || !newOK {
		diagnostics.addDirection(SolveDiagnosticDirectionUnknown, sample)
		return
	}
	oldLess := work.LessOrEqPointRHS(oldRHS, newRHS)
	newLess := work.LessOrEqPointRHS(newRHS, oldRHS)
	switch {
	case oldLess && !newLess:
		diagnostics.addDirection(SolveDiagnosticDirectionOldLessEqNew, sample)
	case newLess && !oldLess:
		diagnostics.addDirection(SolveDiagnosticDirectionNewLessEqOld, sample)
	case oldLess && newLess:
		diagnostics.addDirection(SolveDiagnosticDirectionEqual, sample)
	default:
		diagnostics.addDirection(SolveDiagnosticDirectionIncomparable, sample)
	}
}

func (diagnostics *solveDiagnosticState) addRuleDirection(work *carrier.Work, old, current carrier.RuleContribution, sample *solveDiagnosticRestartSample) {
	if work.EqualRuleContribution(old, current) {
		if work.ExactSameRuleContributionRepresentation(old, current) {
			diagnostics.addDirection(SolveDiagnosticDirectionEqual, sample)
		} else {
			diagnostics.addDirection(SolveDiagnosticDirectionRawOnly, sample)
		}
		return
	}
	oldLess := work.LessOrEqRuleContribution(old, current)
	newLess := work.LessOrEqRuleContribution(current, old)
	switch {
	case oldLess && !newLess:
		diagnostics.addDirection(SolveDiagnosticDirectionOldLessEqNew, sample)
	case newLess && !oldLess:
		diagnostics.addDirection(SolveDiagnosticDirectionNewLessEqOld, sample)
	case oldLess && newLess:
		diagnostics.addDirection(SolveDiagnosticDirectionEqual, sample)
	default:
		diagnostics.addDirection(SolveDiagnosticDirectionIncomparable, sample)
	}
}

func (diagnostics *solveDiagnosticState) addDirection(direction SolveDiagnosticDirection, sample *solveDiagnosticRestartSample) {
	if sample == nil {
		return
	}
	switch direction {
	case SolveDiagnosticDirectionOldLessEqNew:
		sample.directionOldLessEqNew++
	case SolveDiagnosticDirectionNewLessEqOld:
		sample.directionNewLessEqOld++
	case SolveDiagnosticDirectionEqual:
		sample.directionEqual++
	case SolveDiagnosticDirectionRawOnly:
		sample.directionRawOnly++
	case SolveDiagnosticDirectionIncomparable:
		sample.directionIncomparable++
	default:
		sample.directionUnknown++
	}
}

func (diagnostics *solveDiagnosticState) finishRestart(sample solveDiagnosticRestartSample, completed bool) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	if completed {
		sample.completed = 1
	}
	key := sample.key
	index, exists := diagnostics.rowAt[key]
	if !exists {
		if len(diagnostics.rows) >= diagnostics.maxRows {
			diagnostics.droppedRows++
			return
		}
		index = len(diagnostics.rows)
		diagnostics.rowAt[key] = index
		diagnostics.rows = append(diagnostics.rows, SolveDiagnosticRow{
			Revision: key.revision,
			Kind:     key.kind,
			CallSite: key.callSite,
			Reason:   key.reason,
			Phase:    key.phase,
			Region:   key.region,
			Head:     key.head,
		})
	}
	row := &diagnostics.rows[index]
	row.Attempts += sample.attempts
	row.Completed += sample.completed
	row.SubtreePoints += sample.subtreePoints
	row.ResetPoints += sample.resetPoints
	row.ResetProducers += sample.resetProducers
	row.FaceIngressChanged += sample.faceIngressChanged
	row.ExternalProducerIngressChanged += sample.externalProducerIngressChanged
	row.BackProducerIngressChanged += sample.backProducerIngressChanged
	row.ExternalEnvironmentIngressChanged += sample.externalEnvironmentIngressChanged
	row.BackEnvironmentIngressChanged += sample.backEnvironmentIngressChanged
	row.ExternalFactorIngressChanged += sample.externalFactorIngressChanged
	row.BackFactorIngressChanged += sample.backFactorIngressChanged
	row.ExternalOrderFailures += sample.externalOrderFailures
	row.BackOrderFailures += sample.backOrderFailures
	row.RepresentationResets += sample.representationResets
	row.RepresentationOnlyResets += sample.representationOnlyResets
	row.SemanticResets += sample.semanticResets
	row.SemanticSupportResets += sample.semanticSupportResets
	row.SemanticValueResets += sample.semanticValueResets
	row.DirectionOldLessEqNew += sample.directionOldLessEqNew
	row.DirectionNewLessEqOld += sample.directionNewLessEqOld
	row.DirectionEqual += sample.directionEqual
	row.DirectionRawOnly += sample.directionRawOnly
	row.DirectionIncomparable += sample.directionIncomparable
	row.DirectionUnknown += sample.directionUnknown
}
