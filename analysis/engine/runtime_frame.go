// runtime_frame.go exposes the public row API: Row, the product session and the staging verbs.

package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Row is an opaque, synchronous product row. Its support and typed values
// remain inside the private product session; it expires when Transfer returns.
type Row struct {
	session *productSession
	epoch   identity.Generation
	index   int
}

type readRuntime interface {
	inputPort() int
	refine(*productSession, int) bool
	observations() []demand.Observation
	dynamicReads() []demand.DynamicRead
	exactProof() ruleReadProof
	summaryProof() ruleSummaryReadProof
}

type productRow = provenanceRow

type productSession struct {
	execution *ruleExecution
	work      *carrier.Work
	inputs    []carrier.State
	reads     []readRuntime
	sessions  []readSession
	rows      product.Rows
	values    []productRow
	columns   [][]uint64
	live      bool
	started   atomic.Bool
	ready     bool
	// current is the one Product row whose callback owns the synchronous row
	// capability. It is -1 outside that callback; a Row never grants access to
	// a prior or future region while a transfer is still running.
	current int
}

// readSession is payload-free at the generic executor boundary. Concrete
// typed stores are reached only by the declaration-time Read resolver. A
// staged session additionally exposes its epoch-local exact selected routes;
// static declaration reads remain on readRuntime.observations.
type readSession interface {
	close()
	dynamicObservations() []demand.Observation
}

func (session *productSession) valid(execution *ruleExecution, epoch identity.Generation) bool {
	return session != nil && session.live && session.execution == execution && execution != nil && execution.active.holds(epoch) && session.work != nil && session.work.Checkpoint()
}

// checkpoint samples the one carrier-owned epoch probe. Product owns no
// second cancellation authority; it merely stops its private rows before a
// Rule callback, patch, or evidence can escape.
func (session *productSession) checkpoint() bool {
	return session != nil && session.execution != nil && session.work != nil && session.execution.active.holds(session.execution.epoch) && session.work.Checkpoint()
}

func (session *productSession) requireCheckpoint() bool {
	if !session.checkpoint() {
		if session != nil && session.execution != nil {
			session.execution.failed.Store(true)
		}
		return false
	}
	return true
}

func (session *productSession) close() {
	if session == nil {
		return
	}
	session.rows = product.Rows{}
	session.values = nil
	for index := range session.columns {
		session.columns[index] = nil
	}
	session.columns = nil
	for index := range session.sessions {
		if session.sessions[index] != nil {
			session.sessions[index].close()
		}
	}
	session.sessions = nil
	session.reads = nil
	session.inputs = nil
	session.ready = false
	session.current = -1
	session.live = false
}

func newProductSession(execution *ruleExecution, reads []readRuntime, work *carrier.Work, inputs []carrier.State, within support.Mask) (*productSession, bool) {
	if execution == nil || work == nil || !work.Checkpoint() || !work.OwnsRuleContributionStates(execution.base, inputs) || !within.Valid() {
		return nil, false
	}
	// BeginContribution computed the exact input intersection under the sealed
	// group premise and retained that immutable handle in the shared
	// predecessor. Product members only consume the established region;
	// rebuilding the identical meet per member is unnecessary.
	if !execution.base.State().Support().SameHandle(within) {
		return nil, false
	}
	session := &productSession{execution: execution, work: work, inputs: append([]carrier.State(nil), inputs...), reads: append([]readRuntime(nil), reads...), sessions: make([]readSession, len(reads)), live: true, current: -1}
	if support.Empty(within) {
		// An unreachable group is a valid zero-row product. It retains no
		// observation frame and cannot stage a patch, but still lets the
		// monomorphic Transfer confirm its own no-row behavior.
		session.ready = true
		return session, true
	}
	rows, ok := product.NewRows(within)
	if !ok {
		session.close()
		return nil, false
	}
	session.rows, session.values = rows, []productRow{{}}
	for index, read := range reads {
		if !session.requireCheckpoint() || read == nil || read.inputPort() < 0 || read.inputPort() >= len(inputs) || !read.refine(session, index) || session.rows.Count() != len(session.values) {
			session.close()
			return nil, false
		}
	}
	// Every typed refinement has already compacted its own output against its
	// source prefix. Freeze the completed tuple once so hot ReadValue lookups
	// use read-major provenance columns rather than walking that prefix.
	if !session.requireCheckpoint() || !session.freezeProvenance() {
		session.close()
		return nil, false
	}
	if session.rows.Count() == 0 {
		session.close()
		return nil, false
	}
	session.ready = true
	return session, true
}

// Product invokes a bound Rule's declared product once. There is no ambient
// State path: a stale Access, duplicate Product, or escaped Row fails closed.
func Product[V, O any](access Access[V, O], visit func(Row) bool) bool {
	session := access.execution
	if session == nil || access.owner == nil || session.owner != access.owner || !session.active.holds(access.epoch) || session.product == nil || visit == nil {
		return false
	}
	product := session.product
	if !product.valid(session, access.epoch) || !product.ready || !product.started.CompareAndSwap(false, true) {
		session.failed.Store(true)
		return false
	}
	for index := 0; index < product.rows.Count(); index++ {
		product.current = index
		visited := visit(Row{session: product, epoch: access.epoch, index: index})
		settled := session.output == nil || session.output.settled(index)
		product.current = -1
		if !product.requireCheckpoint() || !visited || !settled || !product.requireCheckpoint() {
			session.failed.Store(true)
			return false
		}
	}
	return true
}

// ReadValue reaches only the type witness installed at cold declaration. The
// matching E runtime is checked first, so a typed Read from another rule or a
// stale row cannot be used as an erased observation channel.
func ReadValue[V, O, S any](access Access[V, O], row Row, read Read[S]) (S, bool) {
	var zero S
	execution := access.execution
	if execution == nil || execution.owner == nil || access.owner != execution.owner || !execution.active.holds(access.epoch) || execution.product == nil || !execution.product.requireCheckpoint() || !read.matchesRuntimeOwner(execution.owner) || row.session == nil || row.session != execution.product || row.epoch != access.epoch || row.index != execution.product.current || row.index < 0 || row.index >= len(row.session.values) || read.index >= len(row.session.reads) || row.session.reads[read.index] == nil {
		if execution != nil {
			execution.failed.Store(true)
		}
		return zero, false
	}
	id, found := row.session.readID(row.index, read.index)
	if !found {
		execution.failed.Store(true)
		return zero, false
	}
	value, ok := read.resolve(row.session, read.index, id)
	if !ok || !row.session.requireCheckpoint() {
		execution.failed.Store(true)
		return zero, false
	}
	return value, true
}

// readID resolves a row's exact identity for one declared read. Before the
// final freeze it follows only the persistent prefix; afterwards it reads the
// read-major column directly. The latter is the path exposed to Product and
// RuleAdmission callbacks.
func (session *productSession) readID(row, read int) (uint64, bool) {
	if session == nil || row < 0 || row >= len(session.values) || read < 0 || read >= len(session.reads) {
		return 0, false
	}
	return provenanceID(session.values, session.columns, row, read, len(session.reads))
}

// freezeProvenance converts the retained prefix forest into compact columns
// exactly once, after coalescing has selected the final rows. This is the only
// tuple copy: rows × reads, rather than one copy for every prior read at every
// refinement and another copy during coalescing.
func (session *productSession) freezeProvenance() bool {
	if session == nil || len(session.values) != session.rows.Count() {
		return false
	}
	columns, ok := freezeProvenanceColumns(session.checkpoint, session.values, len(session.reads))
	if !ok {
		return false
	}
	session.columns = columns
	return true
}

// observations is the precise typed surface used by this completed Product.
// It is copied before the session is revoked, so demand owns only carrier
// Units and ordered input positions rather than any transient row payload.
func (session *productSession) observations() []demand.Observation {
	if session == nil {
		return nil
	}
	result := make([]demand.Observation, 0, len(session.reads))
	for index, read := range session.reads {
		if !session.requireCheckpoint() {
			return nil
		}
		if read != nil {
			result = append(result, read.observations()...)
		}
		if index < len(session.sessions) && session.sessions[index] != nil {
			result = append(result, session.sessions[index].dynamicObservations()...)
		}
	}
	if !session.requireCheckpoint() {
		return nil
	}
	return result
}

// StageValue is the only typed output mutation capability. It cannot select a
// target, support region, predecessor, or Factor slot.
func StageValue[V, O any](access Access[V, O], row Row, value V) bool {
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || !access.execution.active.holds(access.epoch) || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.stage == nil || !access.output.stage(access.execution, access.epoch, row.index, value) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// StageSelection is the sole route-output capability. It consumes every
// selected ordinal of exactly one preceding Selection and stages one atomic
// row batch of authenticated target/value pairs. A transfer cannot select a
// different Ref, omit a nonempty route, or send a value through the ordinary
// StageValue path.
func StageSelection[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S], derive func(Tag, S) (V, bool)) bool {
	if derive == nil || !validSelection(access, row, selection) || selection.count == nil || selection.at == nil || selection.route == nil {
		poisonSelection(access)
		return false
	}
	count, ok := selection.count(row.index)
	if !ok || count <= 0 {
		poisonSelection(access)
		return false
	}
	batch := routeOutputBatch[V]{
		read: selection.read, selectionID: selection.selectionID, count: count,
		at: func(ordinal int) (exactRef, V, bool) {
			var zero V
			tag, value, valueOK := selection.at(row.index, ordinal)
			ref, refOK := selection.route(row.index, ordinal)
			output, outputOK := derive(tag, value)
			if !valueOK || !refOK || ref == nil || !outputOK {
				return nil, zero, false
			}
			return ref, output, true
		},
	}
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || !access.execution.active.holds(access.epoch) || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.stageSelection == nil || !access.output.stageSelection(access.execution, access.epoch, row.index, batch) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// NoSelection settles an explicitly empty route selection. It is separate
// from NoCandidate because the empty proof is the same authenticated
// Selection that would otherwise have carried all exact route targets.
func NoSelection[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S]) bool {
	if !validSelection(access, row, selection) || selection.count == nil || selection.route == nil {
		poisonSelection(access)
		return false
	}
	count, ok := selection.count(row.index)
	if !ok || count != 0 {
		poisonSelection(access)
		return false
	}
	batch := routeOutputBatch[V]{read: selection.read, selectionID: selection.selectionID, count: 0}
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || !access.execution.active.holds(access.epoch) || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.noSelection == nil || !access.output.noSelection(access.execution, access.epoch, row.index, batch) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// NoCandidate settles one Product row with an explicit empty successor. It
// is not a sparse write of Default or Bottom: it publishes no Fact update.
// Like StageValue, it is row-, owner-, and epoch-fenced, and a row may take
// exactly one of the two dispositions.
func NoCandidate[V, O any](access Access[V, O], row Row) bool {
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || !access.execution.active.holds(access.epoch) || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.noCandidate == nil || !access.output.noCandidate(access.execution, access.epoch, row.index) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// Operand returns the typed immutable instance payload installed by the cold
// compiler. It is unavailable outside a live transfer frame and cannot be
// retained as a capability to a later solve.
func Operand[V, O any](access Access[V, O]) (O, bool) {
	var zero O
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || !access.execution.active.holds(access.epoch) {
		return zero, false
	}
	return access.owner.operand, true
}
