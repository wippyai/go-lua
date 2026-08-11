package engine

import "sync/atomic"

// SelectorContext is the synchronous, row-local capability passed only while
// a sealed staged locator or write selector is running. It cannot construct a
// carrier capability, recover a key/unit, or retain a route after the call.
type SelectorContext struct {
	frame *selectorFrame
	call  uint64
}

// selectorFrame is one current Product-row selector invocation. Read and
// write selectors are deliberately distinct schema kinds: only write
// selectors own a positional candidate vector, while read selectors emit
// sparse owner-issued exact routes through routes.
type selectorFrame struct {
	execution *ruleExecution
	epoch     uint64
	read      *coldReadSelector
	write     *coldWriteSelector
	product   *productSession
	row       int
	current   int

	// requireCurrent distinguishes output selector execution, which consumes
	// a public Product-row capability, from staged read materialization before
	// Product invokes the transfer callback.
	requireCurrent bool
	routes         selectorRouteSink
	active         atomic.Bool
	call           atomic.Uint64

	selected func(int, int) (bool, bool)
}

func (frame *selectorFrame) valid() bool {
	return frame != nil && frame.execution != nil && frame.epoch != 0 &&
		frame.execution.active.Load() == frame.epoch && (frame.read != nil || frame.write != nil) &&
		(frame.read == nil || frame.write == nil) && frame.active.Load()
}

func (frame *selectorFrame) rowLive() bool {
	return frame != nil && frame.product != nil && (!frame.requireCurrent || frame.product.current == frame.row)
}

func (context SelectorContext) valid() bool {
	frame := context.frame
	return context.call != 0 && frame != nil && frame.valid() && frame.call.Load() == context.call
}

func (frame *selectorFrame) poison() {
	if frame != nil && frame.execution != nil {
		frame.execution.failed.Store(true)
	}
}

func runReadSelector(frame *selectorFrame, locate func(SelectorContext) bool) bool {
	if frame == nil || frame.read == nil || frame.write != nil {
		frame.poison()
		return false
	}
	return runSelector(frame, locate)
}

func runWriteSelector(frame *selectorFrame, decide func(SelectorContext) bool) bool {
	if frame == nil || frame.write == nil || frame.read != nil {
		frame.poison()
		return false
	}
	return runSelector(frame, decide)
}

// runSelector is the sole callback entry. The atomic latch rejects copied
// contexts re-entering while active and invalidates escaped contexts before
// the next row/candidate.
func runSelector(frame *selectorFrame, invoke func(SelectorContext) bool) bool {
	if frame == nil || frame.execution == nil || frame.epoch == 0 || frame.execution.active.Load() != frame.epoch || invoke == nil || !frame.active.CompareAndSwap(false, true) {
		frame.poison()
		return false
	}
	defer frame.active.Store(false)
	call := frame.call.Add(1)
	if call == 0 {
		frame.poison()
		return false
	}
	return invoke(SelectorContext{frame: frame, call: call}) && !frame.execution.failed.Load()
}

func (frame *selectorFrame) declaresRead(index int) bool {
	if frame == nil {
		return false
	}
	var dependencies []Dependency
	if frame.read != nil {
		dependencies = frame.read.depends
	} else if frame.write != nil {
		dependencies = frame.write.depends
	} else {
		return false
	}
	for _, dependency := range dependencies {
		if dependency.kind == readDependency && dependency.index == index {
			return true
		}
	}
	return false
}

func (frame *selectorFrame) isCurrent(index int) bool {
	return frame != nil && frame.write != nil && frame.current >= 0 && frame.current < len(frame.write.candidates) && frame.write.candidates[frame.current] == index
}

func (frame *selectorFrame) declaresCandidate(index int) bool {
	if frame == nil || frame.write == nil {
		return false
	}
	for _, candidate := range frame.write.candidates {
		if candidate == index {
			return true
		}
	}
	return false
}

func (frame *selectorFrame) declaresWrite(index int) bool {
	if frame == nil || frame.write == nil {
		return false
	}
	for _, dependency := range frame.write.depends {
		if dependency.kind == writeDependency && dependency.index == index {
			return true
		}
	}
	return false
}

// SelectorRead returns an already-completed declared predecessor observation
// for the current selector. A later staged locator can therefore consume an
// earlier Selection without a final Product freeze or any ambient State read.
func SelectorRead[S any](context SelectorContext, read Read[S]) (S, bool) {
	var zero S
	frame := context.frame
	if !context.valid() || !frame.rowLive() || read.rule == nil || read.rule != frame.execution.owner.ruleSchema() || read.index < 0 || !frame.declaresRead(read.index) || read.resolve == nil || frame.product == nil || frame.product.execution != frame.execution || frame.row < 0 || frame.row >= len(frame.product.values) || read.index >= len(frame.product.reads) || frame.product.reads[read.index] == nil || !frame.product.requireCheckpoint() {
		frame.poison()
		return zero, false
	}
	id, found := frame.product.readID(frame.row, read.index)
	if !found {
		frame.poison()
		return zero, false
	}
	value, ok := read.resolve(frame.product, read.index, id)
	if !ok || !frame.product.requireCheckpoint() {
		frame.poison()
		return zero, false
	}
	// A staged locator may inspect a Selection returned by an earlier staged
	// read, but that inspection is deliberately scoped to this exact callback
	// and Product row.  The public Access/Row projection is unavailable while
	// Product is still being assembled; conversely, a Selection retained from
	// another locator invocation must not become an ambient observation
	// capability here.  Only engine's unexported marker can receive this scope.
	if scoped, present := any(&value).(selectorScopedSelection); present &&
		!scoped.scopeSelector(frame.product, frame.execution.epoch, frame.row, context.call, read.index) {
		frame.poison()
		return zero, false
	}
	return value, true
}

// CurrentCandidate reports whether a read is the currently evaluated write
// selector candidate. It is invalid for staged read locators.
func CurrentCandidate[S any](context SelectorContext, read Read[S]) bool {
	frame := context.frame
	if !context.valid() || !frame.rowLive() || frame.write == nil || read.rule == nil || read.rule != frame.execution.owner.ruleSchema() || read.index < 0 || !frame.declaresCandidate(read.index) {
		frame.poison()
		return false
	}
	return frame.isCurrent(read.index)
}

// SelectorSelected resolves a presealed write-target relation for the current
// positional write selector candidate. It is unavailable to staged reads.
func SelectorSelected[V, S any](context SelectorContext, prior Write[V], current Read[S]) bool {
	frame := context.frame
	if !context.valid() || !frame.rowLive() || frame.write == nil || prior.rule == nil || prior.rule != frame.execution.owner.ruleSchema() || prior.index < 0 || current.rule == nil || current.rule != frame.execution.owner.ruleSchema() || current.index < 0 || !frame.declaresWrite(prior.index) || !frame.isCurrent(current.index) || frame.selected == nil {
		frame.poison()
		return false
	}
	selected, ok := frame.selected(prior.index, frame.current)
	if !ok {
		frame.poison()
		return false
	}
	return selected
}

// selectorEmission is private transport from the generic SelectRoute helper
// to the typed route sink installed for one staged read. It prevents raw key,
// Unit, and heterogeneous payload access from entering SelectorContext.
type selectorEmission interface{ selectorEmission() }

type selectorRouteSink interface{ accept(selectorEmission) bool }

// selectionTag is the canonical semantic route identity. It intentionally
// excludes floats, pointers, strings, and arbitrary comparable values: a
// Selection's observable ordinal order must have one stable numeric meaning
// across executions and serialized semantic routes.
type selectionTag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}

type emittedRoute[Tag selectionTag] struct {
	ref exactRef
	tag Tag
}

func (emittedRoute[Tag]) selectorEmission() {}

// SelectRoute emits one owner-issued exact target Ref and typed opaque tag for
// the current staged read row. Duplicate (Ref,Tag) routes fail closed; the
// same Ref with distinct tags is retained as distinct semantic evidence.
func SelectRoute[K ~uint32 | ~uint64, Tag selectionTag](context SelectorContext, ref Ref[K], tag Tag) bool {
	frame := context.frame
	if !context.valid() || !frame.rowLive() || frame.read == nil || frame.routes == nil {
		frame.poison()
		return false
	}
	if !frame.routes.accept(emittedRoute[Tag]{ref: ref, tag: tag}) {
		frame.poison()
		return false
	}
	return true
}

// Selection is the typed, row-local multi-route result of a staged read. The
// tag and value are retrieved atomically by SelectionAt, so distinct routes
// cannot be accidentally paired across ordinals.
type Selection[Tag selectionTag, S any] struct {
	session     *productSession
	epoch       uint64
	read        int
	selectionID uint64
	count       func(int) (int, bool)
	at          func(int, int) (Tag, S, bool)
	// route remains private: it is the authenticated exact target paired with
	// this ordinal. StageSelection can consume it atomically but ordinary Rule
	// code can never recover a Ref from a Selection.
	route func(int, int) (exactRef, bool)

	// selectorScope is installed only by SelectorRead.  It is not a second
	// authority: it is a short-lived capability fence for a predecessor
	// Selection while the current staged locator is running.  The ordinary
	// Access/Row projection below remains the only post-Product path.
	selectorSession *productSession
	selectorEpoch   uint64
	selectorRow     int
	selectorCall    uint64
	selectorRead    int
}

// selectorScopedSelection is private so external callers cannot attach a
// SelectorContext scope to an arbitrary value.  A generic SelectorRead
// detects Selection values through this marker without turning its general S
// result into an erased runtime payload.
type selectorScopedSelection interface {
	scopeSelector(*productSession, uint64, int, uint64, int) bool
}

func (selection *Selection[Tag, S]) scopeSelector(session *productSession, epoch uint64, row int, call uint64, read int) bool {
	if selection == nil || session == nil || selection.session != session || selection.epoch != epoch || selection.read != read ||
		selection.selectionID == 0 || row < 0 || row >= len(session.values) || call == 0 {
		return false
	}
	actual, ok := session.readID(row, read)
	if !ok || actual != selection.selectionID {
		return false
	}
	selection.selectorSession = session
	selection.selectorEpoch = epoch
	selection.selectorRow = row
	selection.selectorCall = call
	selection.selectorRead = read
	return true
}

// SelectorSelectionCount exposes a predecessor Selection only during the
// locator invocation which received it through SelectorRead.  This is the
// staged-read counterpart to SelectionCount: it requires no ambient Access
// or Row, yet rejects a foreign dependency, another Product row, a stale
// callback, and a Selection retained after the locator returns.
func SelectorSelectionCount[Tag selectionTag, S any](context SelectorContext, selection Selection[Tag, S]) (int, bool) {
	frame := context.frame
	if !validSelectorSelection(context, selection) || selection.count == nil {
		if frame != nil {
			frame.poison()
		}
		return 0, false
	}
	count, ok := selection.count(frame.row)
	if !ok || count < 0 {
		frame.poison()
		return 0, false
	}
	return count, true
}

// SelectorSelectionAt returns one route's typed tag and value atomically
// under the same locator scope as SelectorSelectionCount.  A tag can therefore
// never be paired with a value from another selected route or Product row.
func SelectorSelectionAt[Tag selectionTag, S any](context SelectorContext, selection Selection[Tag, S], index int) (Tag, S, bool) {
	var tag Tag
	var value S
	frame := context.frame
	if !validSelectorSelection(context, selection) || index < 0 || selection.at == nil {
		if frame != nil {
			frame.poison()
		}
		return tag, value, false
	}
	tag, value, ok := selection.at(frame.row, index)
	if !ok {
		frame.poison()
		return tag, value, false
	}
	return tag, value, true
}

func validSelectorSelection[Tag selectionTag, S any](context SelectorContext, selection Selection[Tag, S]) bool {
	frame := context.frame
	if !context.valid() || frame == nil || !frame.rowLive() || frame.read == nil || frame.write != nil ||
		selection.session != frame.product || selection.epoch != frame.execution.epoch || selection.read < 0 || selection.read >= len(frame.product.reads) || selection.selectionID == 0 ||
		selection.selectorSession != frame.product || selection.selectorEpoch != frame.execution.epoch ||
		selection.selectorRow != frame.row || selection.selectorCall != context.call ||
		selection.selectorRead != selection.read || !frame.declaresRead(selection.read) {
		return false
	}
	actual, ok := frame.product.readID(frame.row, selection.read)
	return ok && actual == selection.selectionID
}

func SelectionCount[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S]) (int, bool) {
	if !validSelection(access, row, selection) || selection.count == nil {
		poisonSelection(access)
		return 0, false
	}
	count, ok := selection.count(row.index)
	if !ok || count < 0 {
		poisonSelection(access)
		return 0, false
	}
	return count, true
}

func SelectionAt[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S], index int) (Tag, S, bool) {
	var tag Tag
	var value S
	if !validSelection(access, row, selection) || index < 0 || selection.at == nil {
		poisonSelection(access)
		return tag, value, false
	}
	tag, value, ok := selection.at(row.index, index)
	if !ok {
		poisonSelection(access)
		return tag, value, false
	}
	return tag, value, true
}

func validSelection[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S]) bool {
	execution := access.execution
	if execution == nil || access.owner == nil || execution.owner != access.owner || access.epoch == 0 || execution.active.Load() != access.epoch || execution.product == nil ||
		row.session == nil || row.session != execution.product || row.epoch != access.epoch || row.index != execution.product.current || row.index < 0 || row.index >= len(row.session.values) ||
		selection.session != row.session || selection.epoch != access.epoch || selection.read < 0 || selection.read >= len(row.session.reads) || selection.selectionID == 0 {
		return false
	}
	actual, ok := row.session.readID(row.index, selection.read)
	return ok && actual == selection.selectionID
}

func poisonSelection[V, O any](access Access[V, O]) {
	if access.execution != nil {
		access.execution.failed.Store(true)
	}
}
