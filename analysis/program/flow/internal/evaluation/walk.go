package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Event is one typed short-circuit Select occurrence in semantic evaluation
// order. The term is an authored Select identity; its ordinal is never used to
// choose order.
type Event struct {
	Select keyspace.Term
}

// Session owns the one iterative authored evaluation-order machine. In its
// ordinary mode it emits Select events. SealPending attaches a private
// pendingBuilder; the same family switch then records persistent prefixes at
// function-style subject boundaries. A Session is not safe for concurrent use.
type Session struct {
	view    authored.View
	stack   []frame
	seen    [keyspace.FamilyCount][]bool
	failed  error
	done    bool
	pending *pendingBuilder
	event   Event
	emitted bool
}

type frame struct {
	term  keyspace.Term
	owner keyspace.Term
	stage uint8
	index int

	// prefix is the immutable set of executable payload Terms evaluated before
	// this frame's own boundary. The remaining fields are Seal-local cursor
	// state used to path-copy a child into that prefix only when a later
	// demanded child needs it.
	prefix           uint32
	pendingInit      bool
	pendingRemaining int
	// prefixCarry asks a structural wrapper to retain evaluated payload
	// children for a later demanded boundary outside that wrapper. It is set
	// only while a later sibling/ancestor subject is known; it never changes
	// the subject's own-boundary exclusion.
	prefixCarry   bool
	returnTerm    keyspace.Term
	returnEnabled bool
}

const (
	stageFirst uint8 = iota
	stageSecond
	stageThird
)

// New creates one explicit evaluation-order session for an authored View. It
// allocates dense occurrence planes exactly once. Branch/Loop roots remain
// rejected in this public event mode because recurrence owns their topology;
// SealPending uses the same machine with a private pending mode for their
// condition/header phases.
func New(view authored.View) (*Session, error) {
	if !view.Cold().ContentID().Available() {
		return nil, errors.New("program/flow/evaluation: authored view is unavailable")
	}
	walker := &Session{view: view, done: true}
	if err := walker.initSeen(); err != nil {
		return nil, err
	}
	return walker, nil
}

// Start begins one root walk. Composite occurrences remain marked between
// roots, making duplicate and cross-root aliases fail closed.
func (walker *Session) Start(root keyspace.Term) error {
	if walker == nil {
		return errors.New("program/flow/evaluation: nil session")
	}
	if walker.failed != nil {
		return walker.failed
	}
	if !walker.done || len(walker.stack) != 0 {
		walker.failed = errors.New("program/flow/evaluation: previous root is unfinished")
		return walker.failed
	}
	allowed := walker.rootAllowed(root)
	if walker.pending != nil && !walker.pending.discover {
		allowed = walker.pending.rootAllowed(root)
	}
	if !allowed {
		walker.failed = errors.New("program/flow/evaluation: root is not an evaluable expression")
		return walker.failed
	}
	walker.done = false
	if err := walker.pushWithPrefix(root, 0, 0); err != nil {
		walker.failed = err
		return err
	}
	return nil
}

// Next returns the next Select event. ok is false only after the entire root
// has been consumed. SealPending drains the same method and ignores emitted
// events while retaining its private subject roots.
func (walker *Session) Next() (event Event, ok bool, err error) {
	if walker == nil {
		return Event{}, false, errors.New("program/flow/evaluation: nil walker")
	}
	if walker.failed != nil {
		return Event{}, false, walker.failed
	}
	if walker.done {
		return Event{}, false, nil
	}
	walker.emitted = false
	for len(walker.stack) != 0 {
		at := len(walker.stack) - 1
		current := &walker.stack[at]
		if stepErr := walker.evaluationStep(current); stepErr != nil {
			walker.failed = stepErr
			return Event{}, false, stepErr
		}
		if walker.emitted {
			event := walker.event
			walker.emitted = false
			return event, true, nil
		}
	}
	walker.done = true
	return Event{}, false, nil
}

// pushChild is the sole child-ingress helper for the evaluation switch. It
// records discovery edges, applies demand pruning, and prepares the bounded
// path-copy update to the parent's prefix after this child completes.
func (walker *Session) pushChild(current *frame, child, owner keyspace.Term) error {
	return walker.pushChildAs(current, child, owner, child, true, false)
}

func (walker *Session) pushChildGuarded(current *frame, child, owner keyspace.Term) error {
	return walker.pushChildAs(current, child, owner, 0, false, true)
}

func (walker *Session) pushChildAs(current *frame, child, owner, returnTerm keyspace.Term, returnEnabled, guard bool) error {
	if !walker.validTerm(child) {
		return errors.New("program/flow/evaluation: child term is unavailable")
	}
	if walker.pending != nil && walker.pending.discover {
		// Static field spellings are metadata, not authored evaluation
		// occurrences. They deliberately never enter the parent/claim proof,
		// so one literal key may name multiple exact fields. Executable runtime
		// UnaryNeg exact operands do not satisfy this gate and remain structural
		// edges; dead edges stay solely in canonical Containment.
		if !walker.pendingStaticReference(current.term, child) {
			if err := walker.pending.recordEdge(current.term, child); err != nil {
				return err
			}
		}
		return walker.pushWithPrefix(child, owner, 0)
	}
	if walker.pending == nil {
		return walker.pushWithPrefix(child, owner, 0)
	}
	needed := walker.pending.needed(child)
	if needed {
		if !current.pendingInit {
			return errors.New("program/flow/evaluation: pending child cursor is uninitialized")
		}
		current.pendingRemaining--
	}
	carry := !guard && (current.prefixCarry || current.pendingRemaining > 0)
	addAfter := returnEnabled && carry
	if !needed {
		if carry && pendingPrefixWrapper(child) {
			if err := walker.pushWithPrefix(child, owner, current.prefix); err != nil {
				return err
			}
			childFrame := &walker.stack[len(walker.stack)-1]
			childFrame.prefixCarry = true
			childFrame.returnEnabled = returnEnabled
			childFrame.returnTerm = returnTerm
			return nil
		}
		if addAfter {
			var err error
			current.prefix, err = walker.pending.add(current.prefix, returnTerm)
			if err != nil {
				return err
			}
		}
		return nil
	}
	if err := walker.pushWithPrefix(child, owner, current.prefix); err != nil {
		return err
	}
	childFrame := &walker.stack[len(walker.stack)-1]
	childFrame.returnEnabled = addAfter
	childFrame.returnTerm = returnTerm
	childFrame.prefixCarry = carry
	return nil
}

func (walker *Session) pendingStaticReference(parent, child keyspace.Term) bool {
	if walker == nil {
		return false
	}
	switch keyspace.TermFamily(parent) {
	case keyspace.FamilyLensExact:
		_, _, source, fieldKind, ok := walker.view.Access().Exact().Get(parent)
		return ok && source == child && staticReference(source, fieldKind)
	case keyspace.FamilyTableField:
		_, key, _, fieldKind, ok := walker.view.Fields().Get(parent)
		return ok && key == child && staticReference(key, fieldKind)
	default:
		return false
	}
}

func (walker *Session) pushChildNoReturn(current *frame, child, owner keyspace.Term) error {
	return walker.pushChildAs(current, child, owner, 0, true, false)
}

// popCurrent closes one frame and applies its deferred prefix insertion to the
// parent. The insertion is the only retained cross-boundary state.
func (walker *Session) popCurrent() error {
	at := len(walker.stack) - 1
	if at < 0 {
		return errors.New("program/flow/evaluation: empty evaluation stack")
	}
	finished := walker.stack[at]
	walker.stack = walker.stack[:at]
	if walker.pending == nil || walker.pending.discover || !finished.returnEnabled || len(walker.stack) == 0 {
		return nil
	}
	parent := &walker.stack[len(walker.stack)-1]
	// Structural wrappers (Values and TableField in particular) are not
	// payload Terms. Their prefix therefore crosses the wrapper boundary so a
	// nested Call/Binary/Select remains visible to the enclosing subject. A
	// function-style payload child crosses only as its own Term: its private
	// operands belong to that child's boundary and must not be flattened into
	// the parent's set.
	if !pendingPayloadTerm(finished.returnTerm) {
		parent.prefix = finished.prefix
		return nil
	}
	var err error
	parent.prefix, err = walker.pending.add(parent.prefix, finished.returnTerm)
	return err
}

func (walker *Session) initPendingChildren(current *frame, terms ...keyspace.Term) {
	if walker.pending == nil || walker.pending.discover || current.pendingInit {
		return
	}
	current.pendingInit = true
	for _, term := range terms {
		if walker.pending.needed(term) {
			current.pendingRemaining++
		}
	}
}

func (walker *Session) selectStep(current *frame) (Event, bool, error) {
	owner, op, left, right, ok := walker.view.Operators().Selects().Get(current.term)
	if !ok || current.owner != owner || (op != kind.SelectAnd && op != kind.SelectOr) {
		return Event{}, false, errors.New("program/flow/evaluation: invalid Select row")
	}
	if current.stage == stageFirst {
		walker.initPendingChildren(current, left, right)
		current.stage = stageSecond
		if err := walker.pushChildGuarded(current, left, owner); err != nil {
			return Event{}, false, err
		}
	} else if current.stage == stageSecond {
		current.stage = stageThird
		return Event{Select: current.term}, true, nil
	} else if current.stage == stageThird {
		current.stage = 3
		if err := walker.pushChildGuarded(current, right, owner); err != nil {
			return Event{}, false, err
		}
	} else {
		if err := walker.popCurrent(); err != nil {
			return Event{}, false, err
		}
	}
	return Event{}, false, nil
}

func (walker *Session) evaluationStep(current *frame) error {
	family := keyspace.TermFamily(current.term)
	switch family {
	case keyspace.FamilySelect:
		event, emitted, err := walker.selectStep(current)
		if emitted {
			walker.event = event
			walker.emitted = true
		}
		return err

	case keyspace.FamilyUnary:
		owner, op, operand, ok := walker.view.Operators().Unaries().Get(current.term)
		if !ok || owner != current.owner || op < kind.UnaryNeg || op > kind.UnaryBitNot {
			return errors.New("program/flow/evaluation: invalid Unary row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, operand)
			current.stage = stageThird
			return walker.pushChild(current, operand, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyValueClaim:
		owner, operand, claimKind, ok := walker.view.Claims().Get(current.term)
		if !ok || owner != current.owner || claimKind < kind.ValueClaimTypeAs || claimKind > kind.ValueClaimNonNil {
			return errors.New("program/flow/evaluation: invalid ValueClaim row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, operand)
			current.stage = stageThird
			return walker.pushChild(current, operand, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyBinary:
		owner, op, left, right, ok := walker.view.Operators().Binaries().Get(current.term)
		if !ok || owner != current.owner || op < kind.BinaryAdd || op > kind.BinaryGreaterEqual {
			return errors.New("program/flow/evaluation: invalid Binary row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, left, right)
			current.stage = stageSecond
			return walker.pushChild(current, left, owner)
		}
		if current.stage == stageSecond {
			current.stage = stageThird
			return walker.pushChild(current, right, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyLensExact:
		owner, base, source, fieldKind, ok := walker.view.Access().Exact().Get(current.term)
		if !ok || owner != current.owner || !walker.staticLensSource(source, fieldKind) {
			return errors.New("program/flow/evaluation: invalid exact Lens row")
		}
		if current.stage == stageFirst {
			if walker.runtimeFieldOperand(source, fieldKind) {
				walker.initPendingChildren(current, base, source)
				current.stage = stageSecond
				return walker.pushChild(current, base, owner)
			}
			walker.initPendingChildren(current, base)
			current.stage = stageThird
			return walker.pushChild(current, base, owner)
		}
		if current.stage == stageSecond {
			current.stage = stageThird
			return walker.pushChild(current, source, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyLensKey:
		owner, base, key, ok := walker.view.Access().Dynamic().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid dynamic Lens row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, base, key)
			current.stage = stageSecond
			return walker.pushChild(current, base, owner)
		}
		if current.stage == stageSecond {
			current.stage = stageThird
			return walker.pushChild(current, key, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyRead:
		owner, source, _, ok := walker.view.Storage().Reads().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Read row")
		}
		sourceFamily := keyspace.TermFamily(source)
		if sourceFamily != keyspace.FamilyCell && sourceFamily != keyspace.FamilyLensExact && sourceFamily != keyspace.FamilyLensKey {
			return errors.New("program/flow/evaluation: Read source is not a storage or Lens term")
		}
		if sourceFamily == keyspace.FamilyCell {
			if !walker.validTerm(source) {
				return errors.New("program/flow/evaluation: Read Cell is unavailable")
			}
			return walker.popCurrent()
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, source)
			current.stage = stageThird
			return walker.pushChild(current, source, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyValues:
		owner, tail, ok := walker.view.Values().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Values row")
		}
		length, ok := walker.view.Values().Len(current.term)
		if !ok || length < 0 {
			return errors.New("program/flow/evaluation: invalid Values extent")
		}
		if !current.pendingInit {
			current.pendingInit = true
			for index := 0; index < length; index++ {
				member, memberOK := walker.view.Values().Member(current.term, index)
				if !memberOK {
					return errors.New("program/flow/evaluation: Values member is unavailable")
				}
				if walker.pending != nil && !walker.pending.discover && walker.pending.needed(member) {
					current.pendingRemaining++
				}
			}
			if tail != 0 && walker.pending != nil && !walker.pending.discover && walker.pending.needed(tail) {
				current.pendingRemaining++
			}
		}
		if current.index < length {
			member, memberOK := walker.view.Values().Member(current.term, current.index)
			if !memberOK {
				return errors.New("program/flow/evaluation: Values member is unavailable")
			}
			current.index++
			return walker.pushChild(current, member, owner)
		}
		if current.stage == stageFirst {
			current.stage = stageSecond
			if tail == 0 {
				return walker.popCurrent()
			}
			if keyspace.TermFamily(tail) != keyspace.FamilyCall && keyspace.TermFamily(tail) != keyspace.FamilyVararg {
				return errors.New("program/flow/evaluation: Values tail is not open")
			}
			return walker.pushChild(current, tail, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyTable:
		owner, ok := walker.view.Tables().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Table row")
		}
		count, ok := walker.view.Tables().FieldCount(current.term)
		if !ok || count < 0 {
			return errors.New("program/flow/evaluation: invalid Table field extent")
		}
		if !current.pendingInit {
			current.pendingInit = true
			for index := 0; index < count; index++ {
				field, fieldOK := walker.view.Tables().FieldAt(current.term, index)
				if !fieldOK {
					return errors.New("program/flow/evaluation: Table field is unavailable")
				}
				if walker.pending != nil && !walker.pending.discover && walker.pending.needed(field) {
					current.pendingRemaining++
				}
			}
		}
		if current.index < count {
			field, fieldOK := walker.view.Tables().FieldAt(current.term, current.index)
			if !fieldOK {
				return errors.New("program/flow/evaluation: Table field is unavailable")
			}
			current.index++
			return walker.pushChildNoReturn(current, field, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyTableField:
		table, key, values, fieldKind, ok := walker.view.Fields().Get(current.term)
		if !ok {
			return errors.New("program/flow/evaluation: invalid TableField row")
		}
		owner, ownerOK := walker.view.Tables().Get(table)
		if !ownerOK || owner != current.owner || !walker.fieldKey(key, fieldKind) {
			return errors.New("program/flow/evaluation: TableField containment is invalid")
		}
		if !current.pendingInit {
			if fieldKind == kind.FieldKey || walker.runtimeFieldOperand(key, fieldKind) {
				walker.initPendingChildren(current, key, values)
			} else {
				walker.initPendingChildren(current, values)
			}
			if walker.pending != nil && !walker.pending.discover {
				// A table allocation is observed before every dynamic field
				// key/value sequence. It must also survive a forced walk of an
				// earlier, non-demanded field when a later field owns the next
				// subject boundary. TableField itself is structural, so the
				// allocation is the retained payload rather than the wrapper.
				addTable := current.prefixCarry
				if (fieldKind == kind.FieldKey || walker.runtimeFieldOperand(key, fieldKind)) && walker.pending.needed(key) {
					addTable = true
				}
				if walker.pending.needed(values) {
					addTable = true
				}
				if addTable {
					var err error
					current.prefix, err = walker.pending.add(current.prefix, table)
					if err != nil {
						return err
					}
				}
			}
		}
		if current.stage == stageFirst {
			current.stage = stageSecond
			if fieldKind == kind.FieldKey || walker.runtimeFieldOperand(key, fieldKind) {
				return walker.pushChild(current, key, owner)
			}
		}
		if current.stage == stageSecond {
			current.stage = stageThird
			return walker.pushChild(current, values, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyCall:
		owner, callee, receiver, actuals, ok := walker.view.Calls().Get(current.term)
		if !ok || owner != current.owner || keyspace.TermFamily(actuals) != keyspace.FamilyValues {
			return errors.New("program/flow/evaluation: invalid Call row")
		}
		if !current.pendingInit {
			walker.initPendingChildren(current, callee, actuals)
		}
		if current.stage == stageFirst {
			current.stage = stageSecond
			return walker.pushChild(current, callee, owner)
		}
		if current.stage == stageSecond {
			current.stage = stageThird
			if walker.pending != nil && !walker.pending.discover && walker.pending.needed(actuals) && receiver != 0 {
				var err error
				current.prefix, err = walker.pending.add(current.prefix, receiver)
				if err != nil {
					return err
				}
			}
			return walker.pushChild(current, actuals, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyBind:
		owner, values, ok := walker.view.Storage().Binds().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Bind row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, values)
			current.stage = stageThird
			return walker.pushChild(current, values, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyAssign:
		owner, values, ok := walker.view.Storage().Assigns().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Assign row")
		}
		writes := walker.view.Storage().Writes()
		writeCount, countOK := walker.view.Storage().Assigns().WriteCount(current.term)
		if !countOK || writeCount <= 0 {
			return errors.New("program/flow/evaluation: invalid Assign write extent")
		}
		if !current.pendingInit {
			current.pendingInit = true
			for index := 0; index < writeCount; index++ {
				write, writeOK := walker.view.Storage().Assigns().WriteAt(current.term, index)
				if !writeOK {
					return errors.New("program/flow/evaluation: Assign write is unavailable")
				}
				if walker.pending != nil && !walker.pending.discover && walker.pending.needed(write) {
					current.pendingRemaining++
				}
			}
			if walker.pending != nil && !walker.pending.discover && walker.pending.needed(values) {
				current.pendingRemaining++
			}
		}
		if current.index < writeCount {
			write, writeOK := walker.view.Storage().Assigns().WriteAt(current.term, current.index)
			if !writeOK || !walker.validTerm(write) {
				return errors.New("program/flow/evaluation: Assign write is unavailable")
			}
			current.index++
			assign, target, rowOK := writes.Get(write)
			if !rowOK || assign != current.term {
				return errors.New("program/flow/evaluation: Assign write containment is invalid")
			}
			walkTarget, targetErr := walker.assignTarget(target, owner)
			if targetErr != nil {
				return targetErr
			}
			if !walkTarget {
				// A Cell target is a completed address. Preserve the target
				// as no payload; a later demanded child still advances the
				// assignment cursor through this write.
				if walker.pending != nil && !walker.pending.discover && walker.pending.needed(write) {
					current.pendingRemaining--
				}
				return nil
			}
			// A Write is its own function-style boundary. Its target is
			// therefore excluded from the Write prefix and is added to the
			// Assign prefix only after the Write completes.
			return walker.pushChildAs(current, write, owner, target, true, false)
		}
		if current.stage == stageFirst {
			current.stage = stageThird
			return walker.pushChild(current, values, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyWrite:
		assign, target, ok := walker.view.Storage().Writes().Get(current.term)
		if !ok {
			return errors.New("program/flow/evaluation: invalid Write row")
		}
		owner, _, ownerOK := walker.view.Storage().Assigns().Get(assign)
		if !ownerOK || owner != current.owner {
			return errors.New("program/flow/evaluation: Write containment is invalid")
		}
		if current.stage == stageFirst {
			current.stage = stageThird
			walkTarget, targetErr := walker.assignTarget(target, owner)
			if targetErr != nil {
				return targetErr
			}
			if !walkTarget {
				return walker.popCurrent()
			}
			walker.initPendingChildren(current, target)
			return walker.pushChildNoReturn(current, target, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyReturn:
		owner, values, ok := walker.view.Control().Returns().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Return row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, values)
			current.stage = stageThird
			return walker.pushChild(current, values, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyBranch:
		owner, condition, _, _, ok := walker.view.Control().Branches().Get(current.term)
		if !ok || owner != current.owner {
			return errors.New("program/flow/evaluation: invalid Branch row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, condition)
			current.stage = stageThird
			return walker.pushChild(current, condition, owner)
		}
		return walker.popCurrent()

	case keyspace.FamilyLoop:
		owner, body, loopKind, control, ok := walker.view.Control().Loops().Get(current.term)
		if !ok || owner != current.owner || loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
			return errors.New("program/flow/evaluation: invalid Loop row")
		}
		if current.stage == stageFirst {
			walker.initPendingChildren(current, control)
			current.stage = stageThird
			// A Repeat condition executes after its lexical body and is authored
			// by that body. While, numeric-for, and generic-for controls execute
			// at the enclosing loop owner frontier.
			controlOwner := owner
			if loopKind == kind.LoopRepeat {
				controlOwner = body
			}
			return walker.pushChild(current, control, controlOwner)
		}
		return walker.popCurrent()

	default:
		// All other admitted expression families are leaves. Their typed
		// validation happened at push time.
		return walker.popCurrent()
	}
}
