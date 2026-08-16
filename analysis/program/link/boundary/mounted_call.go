package boundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// MountedArtifactCall is Boundary's opaque mounted substitution receipt for
// one reusable Program Call semantic ID.  It carries no Project application
// or Program call proof; all operand Values were sealed by Boundary.
type MountedArtifactCall struct {
	component *Component
	ordinal   uint32
}

func (call MountedArtifactCall) row() (mountedCallRow, bool) {
	if call.component == nil || call.component.authority == nil || call.component.authority.mountedCalls == nil || uint64(call.ordinal) >= uint64(len(call.component.authority.mountedCalls.rows)) {
		return mountedCallRow{}, false
	}
	row := call.component.authority.mountedCalls.rows[call.ordinal]
	return row, row.ready && row.module.Available() && row.occurrence.Available()
}

func (call MountedArtifactCall) Available() bool { _, ok := call.row(); return ok }
func (call MountedArtifactCall) Form() flow.CallForm {
	row, ok := call.row()
	if !ok {
		return 0
	}
	return row.form
}
func (call MountedArtifactCall) Callee() (Value, bool) {
	row, ok := call.row()
	if !ok || uint64(row.callee) >= uint64(len(call.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: call.component, ordinal: row.callee}, true
}
func (call MountedArtifactCall) Actuals() (Value, bool) {
	row, ok := call.row()
	if !ok || uint64(row.actuals) >= uint64(len(call.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: call.component, ordinal: row.actuals}, true
}
func (call MountedArtifactCall) Receiver() (Value, bool) {
	row, ok := call.row()
	if !ok || !row.hasReceiver || uint64(row.receiver) >= uint64(len(call.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: call.component, ordinal: row.receiver}, true
}
func (call MountedArtifactCall) Result() (Value, bool) {
	row, ok := call.row()
	if !ok || uint64(row.result) >= uint64(len(call.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: call.component, ordinal: row.result}, true
}
func (call MountedArtifactCall) ArgumentCount() int {
	row, ok := call.row()
	if !ok {
		return 0
	}
	return len(row.arguments)
}
func (call MountedArtifactCall) ArgumentAt(index int) (Value, bool) {
	row, ok := call.row()
	if !ok || index < 0 || index >= len(row.arguments) || uint64(row.arguments[index]) >= uint64(len(call.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: call.component, ordinal: row.arguments[index]}, true
}
func (call MountedArtifactCall) ActualTail() (Value, bool) {
	row, ok := call.row()
	if !ok || !row.hasTail || uint64(row.actualTail) >= uint64(len(call.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: call.component, ordinal: row.actualTail}, true
}

// ForMountedSemantic resolves the exact link-local Call substitution from a
// ModuleKey and a reusable Program Call identity. It has no ordinal or raw
// Program proof input, so a consumer cannot construct a parallel join.
func (v Calls) ForMountedSemantic(module, callID identity.ContentID) (MountedArtifactCall, bool) {
	if v.component == nil || v.component.authority == nil || v.component.authority.mountedCalls == nil || !module.Available() || !callID.Available() {
		return MountedArtifactCall{}, false
	}
	index, ok := v.component.authority.mountedCalls.semantic[mountedCallSemanticKey{module: module, call: callID}]
	if !ok || uint64(index) >= uint64(len(v.component.authority.mountedCalls.rows)) {
		return MountedArtifactCall{}, false
	}
	call := MountedArtifactCall{component: v.component, ordinal: index}
	return call, call.Available()
}

// MountedCallCallee returns the exact sealed callee Value.
func (v Calls) MountedCallCallee(mounted linkproject.CallApplication, occurrence program.CallOccurrence) (Value, bool) {
	row, ok := v.mountedCall(mounted, occurrence)
	if !ok || uint64(row.callee) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: row.callee}, true
}

// MountedCallOperands returns the exact sealed Boundary operand projection.
func (v Calls) MountedCallOperands(mounted linkproject.CallApplication, occurrence program.CallOccurrence) (form flow.CallForm, receiver, actuals Value, ok bool) {
	row, ok := v.mountedCall(mounted, occurrence)
	if !ok {
		return 0, Value{}, Value{}, false
	}
	actuals = Value{component: v.component, ordinal: row.actuals}
	if row.hasReceiver {
		receiver = Value{component: v.component, ordinal: row.receiver}
	}
	return row.form, receiver, actuals, true
}

// MountedCallArgument returns one exact sealed ordered argument projection.
func (v Calls) MountedCallArgument(mounted linkproject.CallApplication, argument program.CallArgument) (Value, bool) {
	occurrence, occurrenceOK := mounted.Occurrence()
	row, rowOK := v.mountedCall(mounted, occurrence)
	position, positionOK := row.values.IssuedArgumentPosition(argument)
	if !occurrenceOK || !rowOK || !positionOK || position < 0 || position >= len(row.arguments) {
		return Value{}, false
	}
	ordinal := row.arguments[position]
	if uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: ordinal}, true
}

// MountedCallActualTail returns the sealed Boundary Value and exact Program
// Span for an open actual-values tail. Closed actuals return no tail proof.
func (v Calls) MountedCallActualTail(mounted linkproject.CallApplication, values program.CallValues) (Value, program.Span, bool) {
	occurrence, occurrenceOK := mounted.Occurrence()
	row, rowOK := v.mountedCall(mounted, occurrence)
	if !occurrenceOK || !rowOK || !row.values.Equal(values) || !row.hasTail || !row.tailContext.Available() || uint64(row.actualTail) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, program.Span{}, false
	}
	return Value{component: v.component, ordinal: row.actualTail}, row.tailSpan, true
}

// MountedCallResult returns the existing Boundary Value for the call result
// occurrence itself, used to locate its already-sealed Pack tail producer.
func (v Calls) MountedCallResult(mounted linkproject.CallApplication, occurrence program.CallOccurrence) (Value, bool) {
	row, ok := v.mountedCall(mounted, occurrence)
	if !ok || uint64(row.result) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: row.result}, true
}

func (v Calls) mountedCall(mounted linkproject.CallApplication, occurrence program.CallOccurrence) (mountedCallRow, bool) {
	if v.component == nil || v.component.authority == nil || v.component.authority.mountedCalls == nil || !v.component.authority.project.Applications().Calls().Owns(mounted) {
		return mountedCallRow{}, false
	}
	application, applicationOK := mounted.Application()
	index, indexOK := v.component.authority.project.Applications().Index(application)
	issued, issuedOK := mounted.Occurrence()
	if !applicationOK || !indexOK || index < 0 || index >= len(v.component.authority.mountedCalls.rows) || !issuedOK || issued != occurrence {
		return mountedCallRow{}, false
	}
	row := v.component.authority.mountedCalls.rows[index]
	if !row.ready || row.mounted != mounted || row.occurrence != occurrence || uint64(row.callee) >= uint64(len(v.component.authority.valueTable.rows)) || uint64(row.actuals) >= uint64(len(v.component.authority.valueTable.rows)) || uint64(row.result) >= uint64(len(v.component.authority.valueTable.rows)) || row.hasReceiver && uint64(row.receiver) >= uint64(len(v.component.authority.valueTable.rows)) || row.hasTail && (uint64(row.actualTail) >= uint64(len(v.component.authority.valueTable.rows)) || !row.tailContext.Available()) {
		return mountedCallRow{}, false
	}
	return row, true
}
