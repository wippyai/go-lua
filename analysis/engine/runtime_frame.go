// runtime_frame.go owns the private product session and exposes the opaque Fold frame/result API.

package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

type readRuntime interface {
	inputPort() int
	refine(*productSession, int) bool
	observations() []demand.Observation
	dynamicReads() []demand.DynamicRead
	exactAddress() (schemaFactorBinding, uint64, bool)
	summaryAddress() (schemaFactorBinding, uint64, []uint64, [32]byte, bool)
}

type productSession struct {
	execution *ruleExecution
	work      *carrier.Work
	inputs    []carrier.State
	reads     []readRuntime
	sessions  []readSession
	rows      product.Rows
	values    []provenanceRow
	columns   [][]uint64
	live      bool
	started   atomic.Bool
	ready     bool
	// current is the one Product row whose Fold owns the synchronous frame
	// capability. It is -1 outside that callback; a Frame never grants access to
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
	return session != nil && session.live && session.execution == execution && execution != nil && execution.active.Holds(epoch) && session.work != nil && session.work.Checkpoint()
}

// checkpoint samples the one carrier-owned epoch probe. Product owns no
// second cancellation authority; it merely stops its private rows before a
// Rule callback, patch, or evidence can escape.
func (session *productSession) checkpoint() bool {
	return session != nil && session.execution != nil && session.work != nil && session.execution.active.Holds(session.execution.epoch) && session.work.Checkpoint()
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
	session := &productSession{execution: execution, work: work, inputs: inputs, reads: reads, sessions: make([]readSession, len(reads)), live: true, current: -1}
	if support.Empty(within) {
		// An unreachable group is a valid zero-row product. It retains no
		// observation frame and cannot stage a patch, but still lets the
		// Fold is not invoked for a zero-row Product.
		session.ready = true
		return session, true
	}
	rows, ok := product.NewRows(within)
	if !ok {
		session.close()
		return nil, false
	}
	session.rows, session.values = rows, []provenanceRow{{}}
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

func validFrame[V, O any](frame Frame[V, O]) bool {
	execution := frame.execution
	return execution != nil && frame.owner != nil && execution.owner == frame.owner && execution.active.Holds(frame.epoch) && execution.product != nil && execution.product.ready && execution.product.current == frame.row && frame.row >= 0 && frame.row < len(execution.product.values) && execution.product.requireCheckpoint()
}

func poisonFrame[V, O any](frame Frame[V, O]) {
	if frame.execution != nil {
		frame.execution.failed.Store(true)
	}
}

// ReadValue reaches only the type witness installed at cold declaration. The
// matching E runtime is checked first, so a typed Read from another rule or a
// stale row cannot be used as an erased observation channel.
func ReadValue[V, O, S any](frame Frame[V, O], read Read[S]) (S, bool) {
	var zero S
	execution := frame.execution
	if !validFrame(frame) || !read.matchesRuntimeOwner(execution.owner) || read.index < 0 || read.index >= len(execution.product.reads) || execution.product.reads[read.index] == nil {
		if execution != nil {
			execution.failed.Store(true)
		}
		return zero, false
	}
	id, found := execution.product.readID(frame.row, read.index)
	if !found {
		execution.failed.Store(true)
		return zero, false
	}
	value, ok := read.resolve(execution.product, read.index, id)
	if !ok || !execution.product.requireCheckpoint() {
		execution.failed.Store(true)
		return zero, false
	}
	return value, true
}

// readID resolves a row's exact identity for one declared read. Before the
// final freeze it follows only the persistent prefix; afterwards it reads the
// read-major column directly. The latter is the path exposed to Product and
// domain callbacks.
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

// Staged returns one direct candidate value. It does not select or mutate the
// output target; the sealed member plan owns publication geometry.
func Staged[V, O any](frame Frame[V, O], value V) RuleResult[V] {
	if !validFrame(frame) {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	return RuleResult[V]{execution: frame.execution, epoch: frame.epoch, row: frame.row, kind: ruleResultStaged, value: value}
}

// Routed is the sole route-output capability. It consumes every
// selected ordinal of exactly one preceding Selection and stages one atomic
// row batch of authenticated target/value pairs. A transfer cannot select a
// different Ref, omit a nonempty route, or send a value through the ordinary
// direct Staged path.
func Routed[V, O any, Tag selectionTag, S any](frame Frame[V, O], selection Selection[Tag, S], derive func(Tag, S) (V, bool)) RuleResult[V] {
	if derive == nil || !validSelection(frame, selection) || selection.count == nil || selection.at == nil || selection.route == nil {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	count, ok := selection.count(frame.row)
	if !ok || count <= 0 {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	if frame.owner.output.routePreflight == nil {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	if frame.owner.output.routeReserve == nil {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	refs, values, lease, reserved := frame.owner.output.routeReserve(frame.execution, frame.epoch, frame.row, selection.read, selection.selectionID, count)
	if !reserved || lease == 0 || len(refs) != count || len(values) != count {
		if reserved && frame.owner.output.routeRelease != nil {
			_ = frame.owner.output.routeRelease(frame.execution, frame.epoch, frame.row, selection.read, selection.selectionID, lease)
		}
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	keepReservation := false
	defer func() {
		if !keepReservation && frame.owner.output.routeRelease != nil {
			_ = frame.owner.output.routeRelease(frame.execution, frame.epoch, frame.row, selection.read, selection.selectionID, lease)
		}
	}()
	for ordinal := 0; ordinal < count; ordinal++ {
		if !validSelection(frame, selection) || !frame.execution.product.requireCheckpoint() || !frame.owner.output.routePreflight(frame.execution, frame.epoch, frame.row, selection.read, selection.selectionID) {
			poisonFrame(frame)
			return RuleResult[V]{}
		}
		tag, value, valueOK := selection.at(frame.row, ordinal)
		ref, refOK := selection.route(frame.row, ordinal)
		if !valueOK || !refOK || ref == nil {
			poisonFrame(frame)
			return RuleResult[V]{}
		}
		output, outputOK := derive(tag, value)
		if !outputOK {
			poisonFrame(frame)
			return RuleResult[V]{}
		}
		if !validSelection(frame, selection) || !frame.execution.product.requireCheckpoint() || !frame.owner.output.routePreflight(frame.execution, frame.epoch, frame.row, selection.read, selection.selectionID) {
			poisonFrame(frame)
			return RuleResult[V]{}
		}
		refs[ordinal], values[ordinal] = ref, output
	}
	keepReservation = true
	return RuleResult[V]{execution: frame.execution, epoch: frame.epoch, row: frame.row, kind: ruleResultRouted, route: routeOutputBatch[V]{
		read: selection.read, selectionID: selection.selectionID, lease: lease,
		refs: refs, values: values,
	}}
}

// NoSelection settles an explicitly empty route selection. It is separate
// from NoCandidate because the empty proof is the same authenticated
// Selection that would otherwise have carried all exact route targets.
func NoSelection[V, O any, Tag selectionTag, S any](frame Frame[V, O], selection Selection[Tag, S]) RuleResult[V] {
	if !validSelection(frame, selection) || selection.count == nil || selection.route == nil || frame.owner.output.routePreflight == nil {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	count, ok := selection.count(frame.row)
	if !ok || count != 0 || !frame.owner.output.routePreflight(frame.execution, frame.epoch, frame.row, selection.read, selection.selectionID) {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	return RuleResult[V]{execution: frame.execution, epoch: frame.epoch, row: frame.row, kind: ruleResultRouted, route: routeOutputBatch[V]{read: selection.read, selectionID: selection.selectionID}}
}

// NoCandidate settles one Product row with an explicit empty successor. It
// is not a sparse write of Default or Bottom: it publishes no Fact update.
// Like Staged, it is row-, owner-, and epoch-fenced, and a row may take
// exactly one of the two dispositions.
func NoCandidate[V, O any](frame Frame[V, O]) RuleResult[V] {
	if !validFrame(frame) {
		poisonFrame(frame)
		return RuleResult[V]{}
	}
	return RuleResult[V]{execution: frame.execution, epoch: frame.epoch, row: frame.row, kind: ruleResultNoCandidate}
}

// Operand returns the typed immutable instance payload installed by the cold
// compiler. It is unavailable outside a live transfer frame and cannot be
// retained as a capability to a later solve.
func Operand[V, O any](frame Frame[V, O]) (O, bool) {
	var zero O
	if !validFrame(frame) {
		return zero, false
	}
	return frame.owner.operand, true
}
