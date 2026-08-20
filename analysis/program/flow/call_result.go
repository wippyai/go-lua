package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// callResultAdmission is Flow's compact consumer geometry. It is derived once
// while Source and Authored are both live during Assemble and retained only as
// a private Values-term keyed column. No Source view or authored inverse is
// retained by the published Flow component.
type callResultAdmission struct {
	multiplicity programschema.CallResultMultiplicity
	count        uint32
	// capacity is the total number of consumer slots for bounded Bind/Assign
	// rows. It gates fixed Call members as well as the remaining tail count;
	// open consumers leave it zero because they have no finite bound.
	capacity uint32
}

func (admission callResultAdmission) valid() bool {
	return admission.multiplicity.Valid()
}

// CallResultGeometry is Flow's neutral, target-independent witness for one
// authored Call output. It names either one fixed Values member or one open
// Values tail; the Target outcome/result identity is supplied by Boundary.
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

func (geometry CallResultGeometry) Available() bool {
	if keyspace.TermFamily(geometry.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(geometry.Call) == 0 || !geometry.Values.Available() || !geometry.Form.Valid() {
		return false
	}
	switch geometry.Form {
	case programschema.CallResultValue:
		return geometry.Value.Available() && !geometry.Tail.Available() && geometry.Multiplicity == programschema.CallResultMultiplicityExact && geometry.Count == 1
	case programschema.CallResultValues:
		return !geometry.Value.Available() && geometry.Tail.Available() && geometry.Position == 0 && geometry.Multiplicity.Valid()
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
func deriveCallResultAdmissions(sourceView source.View, view authored.View) ([]callResultAdmission, bool) {
	if !view.ContentID().Available() || !sourceView.Identity().ContentID().Available() {
		return nil, false
	}
	valuesView := view.Values()
	result := make([]callResultAdmission, valuesView.Count())
	add := func(values keyspace.Term, admission callResultAdmission) bool {
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
			return nil, false
		}
		remaining := width - fixed
		if remaining < 0 {
			remaining = 0
		}
		if !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: uint32(remaining), capacity: uint32(width)}) {
			return nil, false
		}
	}
	assigns := storage.Assigns()
	for index := 0; index < assigns.Count(); index++ {
		assign, assignOK := assigns.At(index)
		_, values, rowOK := assigns.Get(assign)
		width, widthOK := assigns.WriteCount(assign)
		fixed, fixedOK := view.Values().Len(values)
		if !assignOK || !rowOK || !widthOK || !fixedOK || width < 0 || fixed < 0 || uint64(width) > uint64(^uint32(0)) || uint64(fixed) > uint64(^uint32(0)) {
			return nil, false
		}
		remaining := width - fixed
		if remaining < 0 {
			remaining = 0
		}
		if !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: uint32(remaining), capacity: uint32(width)}) {
			return nil, false
		}
	}

	returns := view.Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		returned, returnedOK := returns.At(index)
		_, values, rowOK := returns.Get(returned)
		if !returnedOK || !rowOK || !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityOpen}) {
			return nil, false
		}
	}

	calls := view.Calls()
	for index := 0; index < calls.Count(); index++ {
		call, callOK := calls.At(index)
		_, _, _, actuals, rowOK := calls.Get(call)
		if !callOK || !rowOK || !add(actuals, callResultAdmission{multiplicity: programschema.CallResultMultiplicityOpen}) {
			return nil, false
		}
	}

	fields := view.Fields()
	for index := 0; index < fields.Count(); index++ {
		field, fieldOK := fields.At(index)
		values, finalOpen, rowOK := fields.Values(field)
		if !fieldOK || !rowOK {
			return nil, false
		}
		_, tail, valuesRowOK := view.Values().Get(values)
		if !valuesRowOK {
			return nil, false
		}
		if tail != 0 {
			if !finalOpen || !add(values, callResultAdmission{multiplicity: programschema.CallResultMultiplicityOpen}) {
				return nil, false
			}
		}
	}
	// A Body-owned Values row with no typed consumer is an expression
	// statement. Its result sequence is discarded, so a Call tail there is
	// construction evidence but admits no Target result ordinal.
	for index := 0; index < valuesView.Count(); index++ {
		_, termOK := valuesView.At(index)
		if !termOK {
			return nil, false
		}
		if !result[index].valid() {
			result[index] = callResultAdmission{multiplicity: programschema.CallResultMultiplicityExact, count: 0, capacity: 0}
		}
	}
	return result, true
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
