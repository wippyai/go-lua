package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// callResultAdmission is Flow's compact consumer geometry. It is derived once
// while Source and Authored are both live during Assemble and retained only as
// a private Values-ordinal-aligned column. No Source view or authored inverse
// is retained by the published Flow component.
type callResultAdmission struct {
	multiplicity programschema.CallResultMultiplicity
	count        uint32
	// capacity is the total number of consumer slots for bounded Bind/Assign
	// and loop-control rows. It gates fixed Call members as well as the
	// remaining tail count; open consumers leave it zero because they have no
	// finite bound.
	capacity uint32
	// tailSlotOffset/tailSlotCount address the flat, construction-only
	// consumer descriptors retained while Source's bind order is live. Fixed
	// Values members need no retained descriptor: their existing member ID is
	// already the scalar result coordinate.
	tailSlotOffset uint32
	tailSlotCount  uint32
}

func (admission callResultAdmission) valid() bool {
	return admission.multiplicity.Valid() &&
		(admission.multiplicity != programschema.CallResultMultiplicityOpen || admission.count == 0)
}

type callResultTailSlotAdmission struct {
	consumerKind programschema.CallResultSlotConsumerKind
	consumer     keyspace.Term
	position     uint32
}

// CallResultGeometry is Flow's neutral, target-independent witness for one
// authored Call output. It names either one fixed Values member or one open
// Values tail; the Target outcome/result identity is supplied by the mounted
// consumer join.
// The witness is delivered only during construction and is not a retained
// inverse map in Flow.
type CallResultGeometry struct {
	Call         keyspace.Term
	Values       identity.ContentID
	Value        identity.ContentID
	Tail         identity.ContentID
	Position     uint32
	Form         programschema.CallResultForm
	Multiplicity programschema.CallResultMultiplicity
	Count        uint32
}

// CallResultSlotGeometry is Flow's finite, ordinal result-coordinate witness.
// Fixed members name their existing scalar source. Bounded tails name the
// exact consumer coordinate captured while Source was still live; they never
// reuse the whole-tail producer identity as a scalar Value.
type CallResultSlotGeometry struct {
	Call         keyspace.Term
	Values       identity.ContentID
	Ordinal      uint32
	Position     uint32
	SourceKind   programschema.CallResultSlotSourceKind
	Source       identity.ContentID
	ConsumerKind programschema.CallResultSlotConsumerKind
	Consumer     keyspace.Term
}

// DirectScalarCallResultGeometry identifies a Call whose result is consumed
// directly by another scalar/control term rather than through Values. Flow's
// containment proof is the authority for that edge; the Artifact compiler
// resolves the existing evaluation-span and consumer semantic identities.
type DirectScalarCallResultGeometry struct {
	Call             keyspace.Term
	Consumer         keyspace.Term
	ConsumerPosition uint32
}

func (geometry DirectScalarCallResultGeometry) Available() bool {
	return keyspace.TermFamily(geometry.Call) == keyspace.FamilyCall && keyspace.TermOrdinal(geometry.Call) != 0 &&
		geometry.Consumer != 0 && keyspace.TermFamily(geometry.Consumer) != keyspace.FamilyValues &&
		keyspace.TermFamily(geometry.Consumer) != keyspace.FamilyBody
}

func (geometry CallResultSlotGeometry) Available() bool {
	if keyspace.TermFamily(geometry.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(geometry.Call) == 0 ||
		!geometry.Values.Available() || !geometry.SourceKind.Valid() || !geometry.ConsumerKind.Valid() {
		return false
	}
	if geometry.SourceKind == programschema.CallResultSlotSourceValue {
		return geometry.Ordinal == 0 && geometry.Source.Available() &&
			geometry.ConsumerKind == programschema.CallResultSlotConsumerValuesMember && geometry.Consumer == 0
	}
	return geometry.SourceKind == programschema.CallResultSlotSourceValuesTail && !geometry.Source.Available() &&
		geometry.Consumer != 0
}

func (geometry CallResultGeometry) Available() bool {
	if keyspace.TermFamily(geometry.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(geometry.Call) == 0 || !geometry.Form.Valid() ||
		(geometry.Form != programschema.CallResultDirectValue && !geometry.Values.Available()) {
		return false
	}
	switch geometry.Form {
	case programschema.CallResultValue:
		return geometry.Value.Available() && !geometry.Tail.Available() && geometry.Multiplicity == programschema.CallResultMultiplicityExact && geometry.Count == 1
	case programschema.CallResultValues:
		return !geometry.Value.Available() && geometry.Tail.Available() && geometry.Position == 0 && geometry.Multiplicity.Valid() &&
			(geometry.Multiplicity != programschema.CallResultMultiplicityOpen || geometry.Count == 0)
	case programschema.CallResultDirectValue:
		return !geometry.Values.Available() && geometry.Value.Available() && !geometry.Tail.Available() && geometry.Position == 0 &&
			geometry.Multiplicity == programschema.CallResultMultiplicityExact && geometry.Count == 1
	default:
		return false
	}
}

// deriveCallResultAdmissions joins each authored Values row to exactly one
// consumer geometry. Bind and Assign are finite because their destination
// Cells/Writes are finite; Return, Call actuals, and a final list TableField
// preserve Lua's unbounded expansion. A tail that has no remaining bounded
// destination slots is intentionally represented as exact zero and omitted
// by VisitCallResultGeometry.
func deriveCallResultAdmissions(sourceView source.View, view authored.View) ([]callResultAdmission, []callResultTailSlotAdmission, bool) {
	if !view.ContentID().Available() || !sourceView.Identity().ContentID().Available() {
		return nil, nil, false
	}
	valuesView := view.Values()
	result := make([]callResultAdmission, valuesView.Count())
	tailSlots := make([]callResultTailSlotAdmission, 0)
	add := func(values keyspace.Term, admission callResultAdmission, destination func(position int) (keyspace.Term, programschema.CallResultSlotConsumerKind, bool)) bool {
		if keyspace.TermFamily(values) != keyspace.FamilyValues || keyspace.TermOrdinal(values) == 0 || !admission.valid() {
			return false
		}
		index := int(keyspace.TermOrdinal(values)) - 1
		if index < 0 || index >= len(result) {
			return false
		}
		prior := result[index]
		if prior.valid() && prior != admission {
			return false
		}
		fixed, fixedOK := valuesView.Len(values)
		_, tail, rowOK := valuesView.Get(values)
		if !fixedOK || fixed < 0 || !rowOK {
			return false
		}
		if keyspace.TermFamily(tail) == keyspace.FamilyCall && admission.multiplicity == programschema.CallResultMultiplicityExact && admission.count != 0 {
			if destination == nil || uint64(len(tailSlots))+uint64(admission.count) > uint64(^uint32(0)) {
				return false
			}
			admission.tailSlotOffset = uint32(len(tailSlots))
			admission.tailSlotCount = admission.count
			for ordinal := uint32(0); ordinal < admission.count; ordinal++ {
				position := fixed + int(ordinal)
				consumer, kind, ok := destination(position)
				if !ok || consumer == 0 || !kind.Valid() {
					return false
				}
				tailSlots = append(tailSlots, callResultTailSlotAdmission{consumerKind: kind, consumer: consumer, position: uint32(position)})
			}
		}
		result[index] = admission
		return true
	}

	storage := view.Storage()
	binds := storage.Binds()
	for index := 0; index < binds.Count(); index++ {
		bind, bindOK := binds.At(index)
		_, values, rowOK := binds.Get(bind)
		width, widthOK := sourceView.Binds().Len(bind)
		fixed, fixedOK := view.Values().Len(values)
		if !bindOK || !rowOK || !widthOK || !fixedOK || width < 0 || fixed < 0 || uint64(width) > uint64(^uint32(0)) || uint64(fixed) > uint64(^uint32(0)) {
			return nil, nil, false
		}
		remaining := width - fixed
		if remaining < 0 {
			remaining = 0
		}
		if !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: uint32(remaining), capacity: uint32(width)}, func(position int) (keyspace.Term, programschema.CallResultSlotConsumerKind, bool) {
			cell, ok := sourceView.Binds().At(bind, position)
			return cell, programschema.CallResultSlotConsumerCell, ok
		}) {
			return nil, nil, false
		}
	}
	assigns := storage.Assigns()
	for index := 0; index < assigns.Count(); index++ {
		assign, assignOK := assigns.At(index)
		_, values, rowOK := assigns.Get(assign)
		width, widthOK := assigns.WriteCount(assign)
		fixed, fixedOK := view.Values().Len(values)
		if !assignOK || !rowOK || !widthOK || !fixedOK || width < 0 || fixed < 0 || uint64(width) > uint64(^uint32(0)) || uint64(fixed) > uint64(^uint32(0)) {
			return nil, nil, false
		}
		remaining := width - fixed
		if remaining < 0 {
			remaining = 0
		}
		if !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: uint32(remaining), capacity: uint32(width)}, func(position int) (keyspace.Term, programschema.CallResultSlotConsumerKind, bool) {
			write, writeOK := assigns.WriteAt(assign, position)
			actual, target, targetOK := storage.Writes().Get(write)
			if !writeOK || !targetOK || actual != assign {
				return 0, programschema.CallResultSlotConsumerInvalid, false
			}
			if _, _, _, cellOK := storage.Cells().Get(target); cellOK {
				return target, programschema.CallResultSlotConsumerCell, true
			}
			return target, programschema.CallResultSlotConsumerLens, target != 0
		}) {
			return nil, nil, false
		}
	}

	// Loop controls are Values consumers too, but they do not have Bind or
	// Assign rows carrying their destination width. Numeric-for controls are
	// scalar-adjusted two/three-slot tuples. Generic-for controls are adjusted
	// to the three iterator slots; a final Call tail therefore admits only the
	// remaining prefix, while fixed members beyond slot three are discarded.
	loops := view.Control().Loops()
	for index := 0; index < loops.Count(); index++ {
		loop, loopOK := loops.At(index)
		_, _, loopKind, control, rowOK := loops.Get(loop)
		if !loopOK || !rowOK {
			return nil, nil, false
		}
		switch loopKind {
		case flowkind.LoopNumericFor:
			fixed, fixedOK := view.Values().Len(control)
			_, tail, valuesOK := view.Values().Get(control)
			if !fixedOK || !valuesOK || fixed < 0 || tail != 0 || (fixed != 2 && fixed != 3) {
				return nil, nil, false
			}
			if !add(control, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, capacity: uint32(fixed)}, nil) {
				return nil, nil, false
			}
		case flowkind.LoopGenericFor:
			fixed, fixedOK := view.Values().Len(control)
			_, _, valuesOK := view.Values().Get(control)
			if !fixedOK || !valuesOK || fixed < 0 {
				return nil, nil, false
			}
			remaining := 3 - fixed
			if remaining < 0 {
				remaining = 0
			}
			if !add(control, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: uint32(remaining), capacity: 3}, func(position int) (keyspace.Term, programschema.CallResultSlotConsumerKind, bool) {
				// The three iterator/state/control inputs belong to the Loop
				// construct. Loops.CellAt names body iteration variables, not
				// these control slots, so using those Cells would create a false
				// Value coordinate. Preserve the finite ordinal geometry as a
				// structural Loop consumer for engine/JIT lowering.
				return loop, programschema.CallResultSlotConsumerStructural, position >= 0 && position < 3
			}) {
				return nil, nil, false
			}
		case flowkind.LoopWhile, flowkind.LoopRepeat:
			// These loop controls are scalar ValueOccurrences, not Values
			// rows; a defensive authored row here is not a Call-result
			// consumer geometry.
			continue
		default:
			return nil, nil, false
		}
	}

	returns := view.Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		returned, returnedOK := returns.At(index)
		_, values, rowOK := returns.Get(returned)
		if !returnedOK || !rowOK || !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityOpen}, nil) {
			return nil, nil, false
		}
	}

	calls := view.Calls()
	for index := 0; index < calls.Count(); index++ {
		call, callOK := calls.At(index)
		_, _, _, actuals, rowOK := calls.Get(call)
		if !callOK || !rowOK || !add(actuals, callResultAdmission{multiplicity: programschema.CallResultMultiplicityOpen}, nil) {
			return nil, nil, false
		}
	}

	fields := view.Fields()
	for index := 0; index < fields.Count(); index++ {
		field, fieldOK := fields.At(index)
		values, finalOpen, rowOK := fields.Values(field)
		if !fieldOK || !rowOK {
			return nil, nil, false
		}
		_, tail, valuesRowOK := view.Values().Get(values)
		if !valuesRowOK {
			return nil, nil, false
		}
		if tail != 0 {
			fixed, fixedOK := view.Values().Len(values)
			if !finalOpen || !fixedOK || fixed != 0 || !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityOpen}, nil) {
				return nil, nil, false
			}
		} else {
			// A fixed table-field value is a scalar consumer. Its Call
			// member, if any, admits exactly result zero; it is not a
			// discarded expression merely because the field has no tail.
			fixed, fixedOK := view.Values().Len(values)
			if !fixedOK || fixed != 1 || !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: 1, capacity: 1}, nil) {
				return nil, nil, false
			}
		}
	}
	// A Values row with no sequence consumer can still feed a scalar expression
	// (for example a binary predicate). Its fixed members are already the exact
	// scalar-adjusted consumer coordinates and must remain admissible. Only an
	// unbounded Call tail is discarded here; a bare call expression statement
	// is represented by that tail, not by a fixed member.
	for index := 0; index < valuesView.Count(); index++ {
		values, termOK := valuesView.At(index)
		fixed, fixedOK := valuesView.Len(values)
		if !termOK || !fixedOK || fixed < 0 || uint64(fixed) > uint64(^uint32(0)) {
			return nil, nil, false
		}
		if !result[index].valid() {
			result[index] = callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: 0, capacity: uint32(fixed)}
		}
	}
	return result, tailSlots, true
}

// VisitCallResultGeometry walks Authored.Values once and emits every exact
// Call output witness in authored fixed-member/tail order. A malformed Values
// row, duplicate Call occurrence, unavailable semantic identity, or rejected
// callback fails the walk. Consumers may build a private term index from this
// one pass; Flow itself retains no competing inverse.
func (view View) VisitCallResultGeometry(visit func(CallResultGeometry) bool) bool {
	if !view.available() || visit == nil {
		return false
	}
	values := view.Authored().Values()
	calls := view.Authored().Calls()
	executable := view.Executable()
	if executable == nil {
		return false
	}
	count := values.Count()
	if count < 0 {
		return false
	}
	seen := make(map[keyspace.Term]struct{}, count)
	emit := func(geometry CallResultGeometry) bool {
		if !geometry.Available() {
			return false
		}
		if _, duplicate := seen[geometry.Call]; duplicate {
			return false
		}
		_, _, _, _, callOK := calls.Get(geometry.Call)
		if !callOK {
			return false
		}
		seen[geometry.Call] = struct{}{}
		return visit(geometry)
	}
	for rowIndex := 0; rowIndex < count; rowIndex++ {
		parent, parentOK := values.At(rowIndex)
		width, widthOK := values.Len(parent)
		owner, tailTerm, rowOK := values.Get(parent)
		if !parentOK || !widthOK || width < 0 || !rowOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
			return false
		}
		valuesID, valuesIDOK := view.ValuesOccurrenceID(parent)
		if !valuesIDOK || !valuesID.Available() {
			return false
		}
		for memberIndex := 0; memberIndex < width; memberIndex++ {
			member, memberOK := values.Member(parent, memberIndex)
			if !memberOK {
				return false
			}
			if keyspace.TermFamily(member) != keyspace.FamilyCall || !executable.Contains(member) {
				continue
			}
			admissionOK := rowIndex >= 0 && rowIndex < len(view.component.callResultAdmissions)
			var admission callResultAdmission
			if admissionOK {
				admission = view.component.callResultAdmissions[rowIndex]
			}
			if !admissionOK || !admission.valid() || (admission.multiplicity == programschema.CallResultMultiplicityExact && uint64(memberIndex) >= uint64(admission.capacity)) {
				continue
			}
			memberID, memberIDOK := view.ValuesMemberID(parent, memberIndex)
			if !memberIDOK || !memberID.Available() || !emit(CallResultGeometry{
				Call: member, Values: valuesID, Value: memberID, Position: uint32(memberIndex), Form: programschema.CallResultValue,
				Multiplicity: programschema.CallResultMultiplicityExact, Count: 1,
			}) {
				return false
			}
		}
		if keyspace.TermFamily(tailTerm) != keyspace.FamilyCall || !executable.Contains(tailTerm) {
			continue
		}
		tailID, tailIDOK := view.ValuesTailID(parent)
		admissionOK := rowIndex >= 0 && rowIndex < len(view.component.callResultAdmissions)
		var admission callResultAdmission
		if admissionOK {
			admission = view.component.callResultAdmissions[rowIndex]
		}
		if !tailIDOK || !tailID.Available() || !admissionOK || !admission.valid() {
			return false
		}
		if admission.multiplicity == programschema.CallResultMultiplicityExact && admission.count == 0 {
			continue
		}
		if !emit(CallResultGeometry{Call: tailTerm, Values: valuesID, Tail: tailID, Form: programschema.CallResultValues, Multiplicity: admission.multiplicity, Count: admission.count}) {
			return false
		}
	}
	return true
}

// VisitCallResultSlotGeometry walks the finite scalar result coordinates in
// the same authored Values order as VisitCallResultGeometry. Fixed Call
// members reuse their ValuesMember identity. Only exact bounded tails emit
// ordinal slots; open tails remain whole-sequence producers.
func (view View) VisitCallResultSlotGeometry(visit func(CallResultSlotGeometry) bool) bool {
	if !view.available() || visit == nil {
		return false
	}
	values := view.Authored().Values()
	executable := view.Executable()
	if executable == nil || values.Count() != len(view.component.callResultAdmissions) {
		return false
	}
	seen := make(map[keyspace.Term]struct{}, values.Count())
	emit := func(geometry CallResultSlotGeometry) bool {
		if !geometry.Available() {
			return false
		}
		if geometry.Ordinal == 0 {
			if _, duplicate := seen[geometry.Call]; duplicate {
				return false
			}
			seen[geometry.Call] = struct{}{}
		}
		return visit(geometry)
	}
	for rowIndex := 0; rowIndex < values.Count(); rowIndex++ {
		parent, parentOK := values.At(rowIndex)
		width, widthOK := values.Len(parent)
		_, tail, rowOK := values.Get(parent)
		valuesID, valuesIDOK := view.ValuesOccurrenceID(parent)
		admission := view.component.callResultAdmissions[rowIndex]
		if !parentOK || !widthOK || width < 0 || !rowOK || !valuesIDOK || !valuesID.Available() || !admission.valid() {
			return false
		}
		for position := 0; position < width; position++ {
			member, memberOK := values.Member(parent, position)
			if !memberOK {
				return false
			}
			if keyspace.TermFamily(member) != keyspace.FamilyCall || !executable.Contains(member) ||
				(admission.multiplicity == programschema.CallResultMultiplicityExact && uint64(position) >= uint64(admission.capacity)) {
				continue
			}
			memberID, memberIDOK := view.ValuesMemberID(parent, position)
			if !memberIDOK || !emit(CallResultSlotGeometry{
				Call: member, Values: valuesID, Ordinal: 0, Position: uint32(position),
				SourceKind: programschema.CallResultSlotSourceValue, Source: memberID,
				ConsumerKind: programschema.CallResultSlotConsumerValuesMember,
			}) {
				return false
			}
		}
		if keyspace.TermFamily(tail) != keyspace.FamilyCall || !executable.Contains(tail) ||
			admission.multiplicity != programschema.CallResultMultiplicityExact || admission.count == 0 {
			continue
		}
		if admission.tailSlotCount != admission.count ||
			uint64(admission.tailSlotOffset)+uint64(admission.tailSlotCount) > uint64(len(view.component.callResultTailSlots)) {
			return false
		}
		for ordinal := uint32(0); ordinal < admission.tailSlotCount; ordinal++ {
			slot := view.component.callResultTailSlots[admission.tailSlotOffset+ordinal]
			if slot.position != uint32(width)+ordinal || !emit(CallResultSlotGeometry{
				Call: tail, Values: valuesID, Ordinal: ordinal, Position: slot.position,
				SourceKind:   programschema.CallResultSlotSourceValuesTail,
				ConsumerKind: slot.consumerKind, Consumer: slot.consumer,
			}) {
				return false
			}
		}
	}
	return true
}

// VisitDirectScalarCallResultGeometry emits exactly the executable Calls whose
// canonical containment parent consumes the Call as a scalar directly. Calls
// under Values are handled by the ordinary result geometry; Body-root Calls
// have discarded results and deliberately emit no slot.
func (view View) VisitDirectScalarCallResultGeometry(visit func(DirectScalarCallResultGeometry) bool) bool {
	if !view.available() || visit == nil {
		return false
	}
	calls := view.Authored().Calls()
	executable := view.Executable()
	containment := view.Containment()
	if executable == nil || containment == nil {
		return false
	}
	for index := 0; index < calls.Count(); index++ {
		call, callOK := calls.At(index)
		if !callOK {
			return false
		}
		if !executable.Contains(call) {
			continue
		}
		parent, parentOK := containment.Parent(call)
		if !parentOK {
			// A containment root has no value consumer. Like a Body-root
			// statement Call, it admits no finite result slot.
			continue
		}
		switch keyspace.TermFamily(parent) {
		case keyspace.FamilyValues, keyspace.FamilyBody:
			continue
		}
		position, consumerOK := directScalarCallConsumerPosition(view.Authored(), call, parent)
		if !consumerOK || !executable.Contains(parent) {
			return false
		}
		geometry := DirectScalarCallResultGeometry{Call: call, Consumer: parent, ConsumerPosition: position}
		if !geometry.Available() || !visit(geometry) {
			return false
		}
	}
	return true
}

func directScalarCallConsumerPosition(view authored.View, call, consumer keyspace.Term) (uint32, bool) {
	switch keyspace.TermFamily(consumer) {
	case keyspace.FamilyLensExact:
		_, base, _, _, ok := view.Access().Exact().Get(consumer)
		return 0, ok && base == call
	case keyspace.FamilyLensKey:
		_, base, key, ok := view.Access().Dynamic().Get(consumer)
		if !ok {
			return 0, false
		}
		if base == call {
			return 0, true
		}
		return 1, key == call
	case keyspace.FamilyUnary:
		_, _, operand, ok := view.Operators().Unaries().Get(consumer)
		return 0, ok && operand == call
	case keyspace.FamilyBinary:
		_, _, left, right, ok := view.Operators().Binaries().Get(consumer)
		if !ok {
			return 0, false
		}
		if left == call {
			return 0, true
		}
		return 1, right == call
	case keyspace.FamilySelect:
		_, _, left, right, ok := view.Operators().Selects().Get(consumer)
		if !ok {
			return 0, false
		}
		if left == call {
			return 0, true
		}
		return 1, right == call
	case keyspace.FamilyValueClaim:
		_, operand, _, ok := view.Claims().Get(consumer)
		return 0, ok && operand == call
	case keyspace.FamilyBranch:
		_, condition, _, _, ok := view.Control().Branches().Get(consumer)
		return 0, ok && condition == call
	case keyspace.FamilyLoop:
		_, _, loopKind, control, ok := view.Control().Loops().Get(consumer)
		return 0, ok && (loopKind == flowkind.LoopWhile || loopKind == flowkind.LoopRepeat) && control == call
	case keyspace.FamilyCall:
		_, callee, _, _, ok := view.Calls().Get(consumer)
		return 0, ok && callee == call
	case keyspace.FamilyTableField:
		_, key, _, _, ok := view.Fields().Get(consumer)
		return 0, ok && key == call
	default:
		return 0, false
	}
}
