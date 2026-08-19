package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
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

// SolveDiagnosticPresentation selects which aggregates to record. It is
// presentation-only: a collection choice never enters Snapshot identity.
type SolveDiagnosticPresentation struct {
	Flags SolveDiagnosticFlags
}

// Valid accepts only the closed collection vocabulary.
func (presentation SolveDiagnosticPresentation) Valid() bool {
	return presentation.Flags&^SolveDiagnosticAll == 0
}

// SolveDiagnosticResources bounds detached collector storage. It is a
// resource setting: it never enters Snapshot identity.
type SolveDiagnosticResources struct {
	MaxRows int
}

// Valid accepts only an explicit bounded row cap. Zero selects the default
// once a presentation collection is selected.
func (resources SolveDiagnosticResources) Valid() bool {
	return resources.MaxRows >= 0 && resources.MaxRows <= maxSolveDiagnosticMaxRows
}

// SolveDiagnosticOptions is one SolveWithDiagnostics call's sealed
// presentation collection and resource bound. Presentation never enters
// Snapshot identity. A resource bound without a collection is rejected.
type SolveDiagnosticOptions struct {
	Presentation SolveDiagnosticPresentation
	Resources    SolveDiagnosticResources
}

// Valid accepts only the closed diagnostic vocabulary and explicit bounded
// resource settings. Invalid options are rejected before solver execution;
// callers never get silently clamped or reinterpreted diagnostics.
func (options SolveDiagnosticOptions) Valid() bool {
	return options.Presentation.Valid() && options.Resources.Valid() &&
		(options.Presentation.Flags != 0 || options.Resources.MaxRows == 0)
}

// SolveDiagnosticKind identifies the kind of event represented by a row.
// Keeping the kind in the detached row makes restart and localized refresh
// evidence independently aggregatable without strings.
type SolveDiagnosticKind uint8

const (
	SolveDiagnosticKindRestart SolveDiagnosticKind = iota + 1
	SolveDiagnosticKindInterfaceRefresh
)

// solveDiagnosticRestartCallSite identifies the executor branch that began a
// fresh recurrence episode. It stays inside this package.
type solveDiagnosticRestartCallSite uint8

const (
	solveDiagnosticRestartCandidateInterface solveDiagnosticRestartCallSite = iota + 1
	solveDiagnosticRestartHeadInterface
	solveDiagnosticRestartAscentIngress
	solveDiagnosticRestartNarrowExact
	solveDiagnosticRestartNarrowCurrent
	solveDiagnosticRestartPostfixExact
)

// solveDiagnosticRestartReason identifies the comparison which required a
// fresh exact episode.
type solveDiagnosticRestartReason uint8

const (
	solveDiagnosticRestartCandidateNotOrdered solveDiagnosticRestartReason = iota + 1
	solveDiagnosticRestartInterfaceChanged
	solveDiagnosticRestartIngressNotBelowCurrent
	solveDiagnosticRestartExactIncomparable
	solveDiagnosticRestartExactNotBelowCurrent
)

// solveDiagnosticDirection is the semantic order observed between the
// remembered source and its current value at a restart trigger. RawOnly is a
// representation change with equal lifted meaning. It classifies the private
// per-bucket counters and reaches no caller.
type solveDiagnosticDirection uint8

const (
	solveDiagnosticDirectionUnknown solveDiagnosticDirection = iota
	solveDiagnosticDirectionOldLessEqNew
	solveDiagnosticDirectionNewLessEqOld
	solveDiagnosticDirectionEqual
	solveDiagnosticDirectionRawOnly
	solveDiagnosticDirectionIncomparable
)

// solveDiagnosticRegionPhase identifies the recurrence phase in which a
// restart was requested.
type solveDiagnosticRegionPhase uint8

const (
	solveDiagnosticRegionAscent solveDiagnosticRegionPhase = iota + 1
	solveDiagnosticRegionNarrow
)

// SolveDiagnosticRow is one bounded aggregate restart bucket. The public
// identity is the activation stamp, the event kind, and an opaque site for the
// executor interior that formed the bucket. Attempt count is the published
// evidence. Remaining per-bucket sums stay unexported.
type SolveDiagnosticRow struct {
	Revision identity.Generation
	Kind     SolveDiagnosticKind
	Site     identity.ContentID

	callSite solveDiagnosticRestartCallSite
	reason   solveDiagnosticRestartReason
	phase    solveDiagnosticRegionPhase
	region   int
	head     int

	Attempts  uint64
	completed uint64

	subtreePoints  uint64
	resetPoints    uint64
	resetProducers uint64

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

	interfaceRefreshes           uint64
	interfaceRefreshCompleted    uint64
	interfaceRefreshFallbacks    uint64
	interfaceRefreshOldLessEqNew uint64
	interfaceRefreshNewLessEqOld uint64
	interfaceRefreshEqual        uint64
	interfaceRefreshIncomparable uint64
	interfaceRefreshUnknown      uint64
}

// SolveDiagnostics is a detached snapshot of one solve's bounded runtime
// counters. Its slices are owned by the returned value and no solver, epoch,
// callback, or carrier handle is retained.
type SolveDiagnostics struct {
	Flags SolveDiagnosticFlags
	// Failure is the existing detached first-incomplete certificate issued by
	// this same solver invocation. It is zero for complete, canceled, and
	// panicked calls.
	Failure SolveReport

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

	// UnclassifiedPublications counts the installs whose producing operation
	// issued no ordering classification. PublicationsByReason attributes the
	// classified ones: position i counts the installs carrying the reason at
	// change position i.
	UnclassifiedPublications uint64
	PublicationsByReason     [change.ReasonWidth]uint64

	// Wakes counts the Groups a publication woke. One route accumulates every
	// channel, so a Group reached by several channels is one wake carrying
	// several reasons. WakesByReason attributes those reasons: position i
	// counts the wakes carrying the reason at change position i, and the
	// positions therefore sum to at least Wakes.
	Wakes         uint64
	WakesByReason [change.ReasonWidth]uint64

	InterfaceRefreshes           uint64
	InterfaceRefreshCompleted    uint64
	InterfaceRefreshFallbacks    uint64
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
	unclassifiedPublications     uint64
	publicationsByReason         [change.ReasonWidth]uint64
	wakes                        uint64
	wakesByReason                [change.ReasonWidth]uint64
	interfaceRefreshes           uint64
	interfaceRefreshCompleted    uint64
	interfaceRefreshFallbacks    uint64
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
	revision identity.Generation
	kind     SolveDiagnosticKind
	callSite solveDiagnosticRestartCallSite
	reason   solveDiagnosticRestartReason
	phase    solveDiagnosticRegionPhase
	region   int
	head     int
}

const (
	solveDiagnosticRowSiteDomain  = "engine/solve-diagnostic-row-site"
	solveDiagnosticRowSiteVersion = 1
)

func solveDiagnosticRowSite(key solveDiagnosticRowKey) identity.ContentID {
	return framedContentID(solveDiagnosticRowSiteDomain, solveDiagnosticRowSiteVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Uint(uint64(key.revision)) == nil &&
			writer.Uint(uint64(key.kind)) == nil &&
			writer.Uint(uint64(key.callSite)) == nil &&
			writer.Uint(uint64(key.reason)) == nil &&
			writer.Uint(uint64(key.phase)) == nil &&
			writer.Uint(uint64(key.region+1)) == nil &&
			writer.Uint(uint64(key.head+1)) == nil
	})
}

// solveDiagnosticRowKeyLess is the sole canonical diagnostic row order. It
// governs the snapshot projection and cap eviction alike, so a bounded report
// holds the canonically first rows of the run rather than the first rows the
// executor happened to reach.
func solveDiagnosticRowKeyLess(left, right solveDiagnosticRowKey) bool {
	if left.revision != right.revision {
		return left.revision < right.revision
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	if left.callSite != right.callSite {
		return left.callSite < right.callSite
	}
	if left.reason != right.reason {
		return left.reason < right.reason
	}
	if left.phase != right.phase {
		return left.phase < right.phase
	}
	if left.region != right.region {
		return left.region < right.region
	}
	return left.head < right.head
}

func solveDiagnosticRowKeyOf(row SolveDiagnosticRow) solveDiagnosticRowKey {
	return solveDiagnosticRowKey{
		revision: row.Revision, kind: row.Kind, callSite: row.callSite,
		reason: row.reason, phase: row.phase, region: row.region, head: row.head,
	}
}

// admitRow returns the retained storage index for key. Once the row cap is
// reached the canonically last retained row is the only eviction candidate, so
// the retained set is a function of the row multiset alone.
func (diagnostics *solveDiagnosticState) admitRow(key solveDiagnosticRowKey) (int, bool) {
	if index, exists := diagnostics.rowAt[key]; exists {
		return index, true
	}
	row := SolveDiagnosticRow{
		Revision: key.revision, Kind: key.kind, Site: solveDiagnosticRowSite(key),
		callSite: key.callSite, reason: key.reason, phase: key.phase, region: key.region, head: key.head,
	}
	if len(diagnostics.rows) < diagnostics.maxRows {
		index := len(diagnostics.rows)
		diagnostics.rowAt[key] = index
		diagnostics.rows = append(diagnostics.rows, row)
		return index, true
	}
	diagnostics.droppedRows++
	index, evicted, found := diagnostics.canonicalLastRow()
	if !found || !solveDiagnosticRowKeyLess(key, evicted) {
		return -1, false
	}
	delete(diagnostics.rowAt, evicted)
	diagnostics.rowAt[key] = index
	diagnostics.rows[index] = row
	return index, true
}

func (diagnostics *solveDiagnosticState) canonicalLastRow() (int, solveDiagnosticRowKey, bool) {
	last, lastKey, found := 0, solveDiagnosticRowKey{}, false
	for key, index := range diagnostics.rowAt {
		if !found || solveDiagnosticRowKeyLess(lastKey, key) {
			last, lastKey, found = index, key, true
		}
	}
	return last, lastKey, found
}

type solveDiagnosticInputKind uint8

const (
	solveDiagnosticInputExternalProducer solveDiagnosticInputKind = iota + 1
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

// solveDiagnosticInputKeyLess is the canonical restart-input snapshot order.
// It selects the eviction candidate under the snapshot cap, so the retained
// snapshot set does not depend on executor arrival order.
func solveDiagnosticInputKeyLess(left, right solveDiagnosticInputKey) bool {
	if left.region != right.region {
		return left.region < right.region
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	return left.index < right.index
}

// admitSnapshot returns the retained entry for key. Once the snapshot cap is
// reached the canonically last retained key is the only eviction candidate.
func (diagnostics *solveDiagnosticState) admitSnapshot(key solveDiagnosticInputKey) (solveDiagnosticInput, bool) {
	previous, exists := diagnostics.snapshots[key]
	if exists || len(diagnostics.snapshots) < diagnostics.maxSnapshots {
		return previous, true
	}
	diagnostics.droppedSnapshots++
	evicted, found := diagnostics.canonicalLastSnapshot()
	if !found || !solveDiagnosticInputKeyLess(key, evicted) {
		return solveDiagnosticInput{}, false
	}
	delete(diagnostics.snapshots, evicted)
	return solveDiagnosticInput{}, true
}

func (diagnostics *solveDiagnosticState) canonicalLastSnapshot() (solveDiagnosticInputKey, bool) {
	last, found := solveDiagnosticInputKey{}, false
	for key := range diagnostics.snapshots {
		if !found || solveDiagnosticInputKeyLess(last, key) {
			last, found = key, true
		}
	}
	return last, found
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
	flags := options.Presentation.Flags & SolveDiagnosticAll
	if flags == 0 {
		return nil
	}
	maxRows := options.Resources.MaxRows
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
		rows:         make([]SolveDiagnosticRow, 0, maxRows),
		rowAt:        make(map[solveDiagnosticRowKey]int, maxRows),
		snapshots:    make(map[solveDiagnosticInputKey]solveDiagnosticInput, maxSnapshots),
		maxSnapshots: maxSnapshots,
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

func (diagnostics *solveDiagnosticState) epochStarted(epoch *executorEpoch, relation identity.Generation) {
	if diagnostics == nil {
		return
	}
	diagnostics.epochs++
	diagnostics.revisions = uint64(relation)
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

// observeRevision records the live activation-relation stamp. The stamp is
// one-based, so it is also the count of relations this Solver has published.
func (diagnostics *solveDiagnosticState) observeRevision(relation identity.Generation) {
	if diagnostics == nil {
		return
	}
	diagnostics.revisions = uint64(relation)
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
		return solveDiagnosticRowKeyLess(solveDiagnosticRowKeyOf(rows[left]), solveDiagnosticRowKeyOf(rows[right]))
	})
	return SolveDiagnostics{
		Flags:                        diagnostics.flags,
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
		Wakes:                        diagnostics.wakes,
		UnclassifiedPublications:     diagnostics.unclassifiedPublications,
		PublicationsByReason:         diagnostics.publicationsByReason,
		WakesByReason:                diagnostics.wakesByReason,
		InterfaceRefreshes:           diagnostics.interfaceRefreshes,
		InterfaceRefreshCompleted:    diagnostics.interfaceRefreshCompleted,
		InterfaceRefreshFallbacks:    diagnostics.interfaceRefreshFallbacks,
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

// recordPublicationEvidence attributes one install's issued change facts. An
// operation that published no classified transition is counted as such rather
// than being silently attributed to an ascent.
func (diagnostics *solveDiagnosticState) recordPublicationEvidence(evidence change.Set) {
	if diagnostics == nil || !diagnostics.publicationEnabled() {
		return
	}
	if evidence.Unknown() {
		diagnostics.unclassifiedPublications++
		return
	}
	for position := 0; position < change.ReasonWidth; position++ {
		reason, ok := change.ReasonAt(position)
		if ok && evidence.Has(reason) {
			diagnostics.publicationsByReason[position]++
		}
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

// recordWakes counts one publication's routed Groups and attributes the
// reasons they accumulated. The two channels were never disjoint, so a split
// total counted a routing artefact rather than a fact about the solve; the
// histogram reports the distinction the split was reaching for.
func (diagnostics *solveDiagnosticState) recordWakes(wakes []demand.Wake) {
	if diagnostics == nil || !diagnostics.publicationEnabled() {
		return
	}
	diagnostics.wakes += uint64(len(wakes))
	for _, wake := range wakes {
		for position := 0; position < change.ReasonWidth; position++ {
			reason, ok := change.ReasonAt(position)
			if ok && wake.Reasons.Has(reason) {
				diagnostics.wakesByReason[position]++
			}
		}
	}
}

func (diagnostics *solveDiagnosticState) interfaceRefreshRow(epoch *executorEpoch, region int) *SolveDiagnosticRow {
	if diagnostics == nil || !diagnostics.restartEnabled() || epoch == nil || epoch.runtime == nil || region < 0 || region >= len(epoch.runtime.regions) || region >= len(epoch.regions) {
		return nil
	}
	state := &epoch.regions[region]
	key := solveDiagnosticRowKey{revision: epoch.diagnosticRevision, kind: SolveDiagnosticKindInterfaceRefresh, callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged, region: region, head: epoch.runtime.regions[region].head}
	if state.phase == phaseAscent {
		key.phase = solveDiagnosticRegionAscent
	} else if state.phase == phaseNarrow {
		key.phase = solveDiagnosticRegionNarrow
	}
	index, retained := diagnostics.admitRow(key)
	if !retained {
		return nil
	}
	return &diagnostics.rows[index]
}

func (diagnostics *solveDiagnosticState) recordInterfaceRefreshBegin(epoch *executorEpoch, region int) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	diagnostics.interfaceRefreshes++
	row := diagnostics.interfaceRefreshRow(epoch, region)
	if row == nil {
		return
	}
	row.interfaceRefreshes++
}

func interfaceRefreshDirection(work *carrier.Work, old, current carrier.PointRHS) solveDiagnosticDirection {
	if work == nil || !work.OwnsPointRHS(old) || !work.OwnsPointRHS(current) {
		return solveDiagnosticDirectionUnknown
	}
	if work.EqualPointRHS(old, current) {
		// PointRHS deliberately exposes only lifted semantic order. Raw-only
		// representation is source-publication telemetry, not an exact-RHS
		// direction and must not acquire a second carrier predicate here.
		return solveDiagnosticDirectionEqual
	}
	oldLess := work.LessOrEqPointRHS(old, current)
	newLess := work.LessOrEqPointRHS(current, old)
	switch {
	case oldLess && !newLess:
		return solveDiagnosticDirectionOldLessEqNew
	case newLess && !oldLess:
		return solveDiagnosticDirectionNewLessEqOld
	case oldLess && newLess:
		return solveDiagnosticDirectionEqual
	default:
		return solveDiagnosticDirectionIncomparable
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
	case solveDiagnosticDirectionOldLessEqNew:
		diagnostics.interfaceRefreshOldLessEqNew++
	case solveDiagnosticDirectionNewLessEqOld:
		diagnostics.interfaceRefreshNewLessEqOld++
	case solveDiagnosticDirectionEqual:
		diagnostics.interfaceRefreshEqual++
	case solveDiagnosticDirectionIncomparable:
		diagnostics.interfaceRefreshIncomparable++
	default:
		diagnostics.interfaceRefreshUnknown++
	}
	row := diagnostics.interfaceRefreshRow(epoch, region)
	if row == nil {
		return
	}
	if completed {
		row.interfaceRefreshCompleted++
	}
	if fallback {
		row.interfaceRefreshFallbacks++
	}
	switch direction {
	case solveDiagnosticDirectionOldLessEqNew:
		row.interfaceRefreshOldLessEqNew++
	case solveDiagnosticDirectionNewLessEqOld:
		row.interfaceRefreshNewLessEqOld++
	case solveDiagnosticDirectionEqual:
		row.interfaceRefreshEqual++
	case solveDiagnosticDirectionIncomparable:
		row.interfaceRefreshIncomparable++
	default:
		row.interfaceRefreshUnknown++
	}
}

func (diagnostics *solveDiagnosticState) rememberPoint(key solveDiagnosticInputKey, point carrier.PointState) {
	if diagnostics == nil || !diagnostics.restartEnabled() {
		return
	}
	previous, retained := diagnostics.admitSnapshot(key)
	if !retained {
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
	previous, retained := diagnostics.admitSnapshot(key)
	if !retained {
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
	bound := &epoch.runtime.regions[region]
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

func (diagnostics *solveDiagnosticState) beginRestart(epoch *executorEpoch, region int, callSite solveDiagnosticRestartCallSite, reason solveDiagnosticRestartReason, pendingGroup int, pending carrier.RuleContribution) solveDiagnosticRestartSample {
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
	bound, state := &epoch.runtime.regions[region], &epoch.regions[region]
	sample.key.head = bound.head
	if state.phase == phaseAscent {
		sample.key.phase = solveDiagnosticRegionAscent
	} else if state.phase == phaseNarrow {
		sample.key.phase = solveDiagnosticRegionNarrow
	}
	sample.subtreePoints = uint64(len(bound.points))
	diagnostics.captureRestartMismatches(epoch, region, bound, state, pendingGroup, pending, &sample)
	switch callSite {
	case solveDiagnosticRestartAscentIngress:
		sample.externalOrderFailures = 1
	case solveDiagnosticRestartNarrowExact, solveDiagnosticRestartNarrowCurrent, solveDiagnosticRestartPostfixExact:
		sample.backOrderFailures = 1
	}
	return sample
}

// captureRestartMismatches reports which of this Region's ingress operands
// moved since it last remembered its interfaces. That is the operand plane's
// own question, asked over the same stamps the executor decides restarts
// with, so the diagnostic reads the plane instead of rescanning every
// external producer, environment edge and factor edge to diff a private
// version vector against them.
func (diagnostics *solveDiagnosticState) captureRestartMismatches(epoch *executorEpoch, region int, bound *runtimeRegion, state *regionEpoch, pendingGroup int, pending carrier.RuleContribution, sample *solveDiagnosticRestartSample) {
	rows := [6]struct {
		kind     operandKind
		members  []int
		count    *uint64
		input    solveDiagnosticInputKind
		classify func(key solveDiagnosticInputKey, member int)
	}{
		{kind: operandExternalProducer, members: bound.external, count: &sample.externalProducerIngressChanged, input: solveDiagnosticInputExternalProducer, classify: func(key solveDiagnosticInputKey, member int) {
			diagnostics.classifyRule(epoch, key, member, pendingGroup, pending, sample)
		}},
		{kind: operandBackProducer, members: bound.back, count: &sample.backProducerIngressChanged, input: solveDiagnosticInputBackProducer, classify: func(key solveDiagnosticInputKey, member int) {
			diagnostics.classifyRule(epoch, key, member, pendingGroup, pending, sample)
		}},
		{kind: operandExternalEnvironment, members: bound.environmentExternal, count: &sample.externalEnvironmentIngressChanged},
		{kind: operandBackEnvironment, members: bound.environmentBack, count: &sample.backEnvironmentIngressChanged, input: solveDiagnosticInputBackEnvironment, classify: func(key solveDiagnosticInputKey, member int) {
			diagnostics.classifyEnvironment(epoch, key, member, sample)
		}},
		{kind: operandExternalFactor, members: bound.factorExternal, count: &sample.externalFactorIngressChanged},
		{kind: operandBackFactor, members: bound.factorBack, count: &sample.backFactorIngressChanged, input: solveDiagnosticInputBackFactor, classify: func(key solveDiagnosticInputKey, member int) {
			diagnostics.classifyFactor(epoch, key, member, sample)
		}},
	}
	for _, row := range rows {
		begin, end, ok := epoch.operands.plane.regionWindow(region, row.kind)
		if !ok || end-begin != len(row.members) {
			// A row the plane does not describe cannot be diffed at all. It is
			// reported once, in the same unknown direction the old width
			// mismatch reported.
			*row.count++
			diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
			continue
		}
		for position, member := range row.members {
			if !epoch.operands.changedSince(uint32(begin+position), state.rememberAt) {
				continue
			}
			*row.count++
			if row.classify != nil {
				row.classify(solveDiagnosticInputKey{region: region, kind: row.input, index: position}, member)
			}
		}
	}
}

func (diagnostics *solveDiagnosticState) classifyPoint(epoch *executorEpoch, key solveDiagnosticInputKey, point int, sample *solveDiagnosticRestartSample) {
	if point < 0 || point >= len(epoch.points) {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	old, ok := diagnostics.snapshots[key]
	if !ok || !old.pointOK || !epoch.work.OwnsPointState(old.point) || !epoch.work.OwnsPointState(epoch.points[point]) {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.addPointDirection(epoch.work, old.point, epoch.points[point], sample)
}

func (diagnostics *solveDiagnosticState) classifyRule(epoch *executorEpoch, key solveDiagnosticInputKey, group, pendingGroup int, pending carrier.RuleContribution, sample *solveDiagnosticRestartSample) {
	if group < 0 || group >= len(epoch.producers) {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	old, ok := diagnostics.snapshots[key]
	current := epoch.producers[group]
	if group == pendingGroup && epoch.work.OwnsRuleContribution(pending) {
		current.candidate = pending
		current.hasValue = true
	}
	if !ok || !old.ruleOK || !current.hasValue || !epoch.work.OwnsRuleContribution(old.rule) || !epoch.work.OwnsRuleContribution(current.candidate) {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.addRuleDirection(epoch.work, old.rule, current.candidate, sample)
}

func (diagnostics *solveDiagnosticState) classifyEnvironment(epoch *executorEpoch, key solveDiagnosticInputKey, edge int, sample *solveDiagnosticRestartSample) {
	if edge < 0 || edge >= len(epoch.runtime.environments) {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.classifyPoint(epoch, key, epoch.runtime.environments[edge].source, sample)
}

func (diagnostics *solveDiagnosticState) classifyFactor(epoch *executorEpoch, key solveDiagnosticInputKey, edge int, sample *solveDiagnosticRestartSample) {
	if edge < 0 || edge >= len(epoch.runtime.factorEdges) {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	diagnostics.classifyPoint(epoch, key, epoch.runtime.factorEdges[edge].source, sample)
}

func (diagnostics *solveDiagnosticState) addPointDirection(work *carrier.Work, old, current carrier.PointState, sample *solveDiagnosticRestartSample) {
	if work.EqualPointState(old, current) {
		if work.ExactSamePointRepresentation(old, current) {
			diagnostics.addDirection(solveDiagnosticDirectionEqual, sample)
		} else {
			diagnostics.addDirection(solveDiagnosticDirectionRawOnly, sample)
		}
		return
	}
	oldRHS, oldOK := work.PointRHSFromPointState(old)
	newRHS, newOK := work.PointRHSFromPointState(current)
	if !oldOK || !newOK {
		diagnostics.addDirection(solveDiagnosticDirectionUnknown, sample)
		return
	}
	oldLess := work.LessOrEqPointRHS(oldRHS, newRHS)
	newLess := work.LessOrEqPointRHS(newRHS, oldRHS)
	switch {
	case oldLess && !newLess:
		diagnostics.addDirection(solveDiagnosticDirectionOldLessEqNew, sample)
	case newLess && !oldLess:
		diagnostics.addDirection(solveDiagnosticDirectionNewLessEqOld, sample)
	case oldLess && newLess:
		diagnostics.addDirection(solveDiagnosticDirectionEqual, sample)
	default:
		diagnostics.addDirection(solveDiagnosticDirectionIncomparable, sample)
	}
}

func (diagnostics *solveDiagnosticState) addRuleDirection(work *carrier.Work, old, current carrier.RuleContribution, sample *solveDiagnosticRestartSample) {
	if work.EqualRuleContribution(old, current) {
		if work.ExactSameRuleContributionRepresentation(old, current) {
			diagnostics.addDirection(solveDiagnosticDirectionEqual, sample)
		} else {
			diagnostics.addDirection(solveDiagnosticDirectionRawOnly, sample)
		}
		return
	}
	oldLess := work.LessOrEqRuleContribution(old, current)
	newLess := work.LessOrEqRuleContribution(current, old)
	switch {
	case oldLess && !newLess:
		diagnostics.addDirection(solveDiagnosticDirectionOldLessEqNew, sample)
	case newLess && !oldLess:
		diagnostics.addDirection(solveDiagnosticDirectionNewLessEqOld, sample)
	case oldLess && newLess:
		diagnostics.addDirection(solveDiagnosticDirectionEqual, sample)
	default:
		diagnostics.addDirection(solveDiagnosticDirectionIncomparable, sample)
	}
}

func (diagnostics *solveDiagnosticState) addDirection(direction solveDiagnosticDirection, sample *solveDiagnosticRestartSample) {
	if sample == nil {
		return
	}
	switch direction {
	case solveDiagnosticDirectionOldLessEqNew:
		sample.directionOldLessEqNew++
	case solveDiagnosticDirectionNewLessEqOld:
		sample.directionNewLessEqOld++
	case solveDiagnosticDirectionEqual:
		sample.directionEqual++
	case solveDiagnosticDirectionRawOnly:
		sample.directionRawOnly++
	case solveDiagnosticDirectionIncomparable:
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
	index, retained := diagnostics.admitRow(sample.key)
	if !retained {
		return
	}
	row := &diagnostics.rows[index]
	row.Attempts += sample.attempts
	row.completed += sample.completed
	row.subtreePoints += sample.subtreePoints
	row.resetPoints += sample.resetPoints
	row.resetProducers += sample.resetProducers
	row.externalProducerIngressChanged += sample.externalProducerIngressChanged
	row.backProducerIngressChanged += sample.backProducerIngressChanged
	row.externalEnvironmentIngressChanged += sample.externalEnvironmentIngressChanged
	row.backEnvironmentIngressChanged += sample.backEnvironmentIngressChanged
	row.externalFactorIngressChanged += sample.externalFactorIngressChanged
	row.backFactorIngressChanged += sample.backFactorIngressChanged
	row.externalOrderFailures += sample.externalOrderFailures
	row.backOrderFailures += sample.backOrderFailures
	row.representationResets += sample.representationResets
	row.representationOnlyResets += sample.representationOnlyResets
	row.semanticResets += sample.semanticResets
	row.semanticSupportResets += sample.semanticSupportResets
	row.semanticValueResets += sample.semanticValueResets
	row.directionOldLessEqNew += sample.directionOldLessEqNew
	row.directionNewLessEqOld += sample.directionNewLessEqOld
	row.directionEqual += sample.directionEqual
	row.directionRawOnly += sample.directionRawOnly
	row.directionIncomparable += sample.directionIncomparable
	row.directionUnknown += sample.directionUnknown
}
