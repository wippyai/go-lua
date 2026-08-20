package boundary

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/internal/framing"
)

const callResultRelationVersion = 3

// boundaryCallResultWitness is construction-only geometry. It is built once
// per mounted Program from Authored.Values and dies after sealCallResults has
// copied the parent-issued semantic IDs into the Boundary table.
type boundaryCallResultWitness struct {
	values       identity.ContentID
	value        identity.ContentID
	tail         identity.ContentID
	position     uint32
	form         programschema.CallResultForm
	multiplicity programschema.CallResultMultiplicity
	count        uint32
}

// sealCallResults retains one reusable output row per mounted ordinary Call
// which is actually consumed by an authored Value/Values context. Target
// outcome/result identities are intentionally not expanded here: the hot
// relation is factorized by ModuleKey+CallID and CallResult validates a
// supplied Target outcome/result through the exact Contract inverse.
func sealCallResults(a *authority) error {
	if a == nil || a.project == nil || a.callResults != nil {
		return errors.New("link/boundary: invalid call-result authority")
	}
	mounts := a.project.Mounts()
	type mountGeometry struct {
		module identity.ContentID
		calls  map[identity.ContentID]boundaryCallResultWitness
	}
	geometry := make([]mountGeometry, mounts.Count())
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		p, programOK := mounts.Program(shard)
		module, moduleOK := a.project.ModuleKey(shard)
		if !shardOK || !programOK || p == nil || !moduleOK || !module.Available() {
			return errors.New("link/boundary: unavailable mounted Program for call-result geometry")
		}
		byTerm, geometryOK := boundaryCallResultGeometryIndex(p)
		if !geometryOK {
			return fmt.Errorf("link/boundary: malformed mounted call-result geometry mount=%d", mountIndex)
		}
		byCall := make(map[identity.ContentID]boundaryCallResultWitness, len(byTerm))
		calls := p.Flow().Authored().Calls()
		for callIndex := 0; callIndex < calls.Count(); callIndex++ {
			term, termOK := calls.At(callIndex)
			witness, hasGeometry := byTerm[term]
			if !hasGeometry {
				continue
			}
			identities, identitiesOK := boundaryCallIdentitiesAt(p, callIndex)
			if !termOK || !identitiesOK || !identities.Call.Available() {
				return fmt.Errorf("link/boundary: call-result identity unavailable mount=%d call=%d", mountIndex, callIndex)
			}
			if _, duplicate := byCall[identities.Call]; duplicate {
				return fmt.Errorf("link/boundary: duplicate call-result identity mount=%d call=%d", mountIndex, callIndex)
			}
			byCall[identities.Call] = witness
		}
		geometry[mountIndex] = mountGeometry{module: module, calls: byCall}
	}

	projectCalls := a.project.Applications().Calls()
	rows := make([]callResultRow, 0, projectCalls.Count())
	for applicationIndex := 0; applicationIndex < projectCalls.Count(); applicationIndex++ {
		application, applicationOK := projectCalls.At(applicationIndex)
		proof, proofOK := projectCalls.ForApplication(application)
		_, module, callID, mountedOK := projectCalls.MountedIdentity(application)
		projectApplicationIndex, projectApplicationIndexOK := a.project.Applications().Index(application)
		applicationID, applicationIDOK := application.ContentID()
		shard, shardOK := proof.Mount()
		mountIndex, mountOK := mounts.Index(shard)
		if !applicationOK || !proofOK || !mountedOK || !projectApplicationIndexOK || !applicationIDOK || projectApplicationIndex < 0 || uint64(projectApplicationIndex) >= uint64(^uint32(0)) || !shardOK || !mountOK || mountIndex < 0 || mountIndex >= len(geometry) || geometry[mountIndex].module != module || !callID.Available() {
			return fmt.Errorf("link/boundary: malformed mounted Call application=%d", applicationIndex)
		}
		witness, found := geometry[mountIndex].calls[callID]
		if !found {
			// This is a valid statement Call: Lua discards its result and no
			// Value/Values coordinate is issued for it.
			continue
		}
		row := callResultRow{module: module, call: callID, application: uint32(projectApplicationIndex + 1), applicationID: applicationID, values: witness.values, value: witness.value, tail: witness.tail, position: witness.position, form: witness.form, multiplicity: witness.multiplicity, count: witness.count}
		if !validCallResultRow(row) {
			return fmt.Errorf("link/boundary: unavailable mounted Call result application=%d", applicationIndex)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool {
		if orderContentID(rows[left].module, rows[right].module) != 0 {
			return orderContentID(rows[left].module, rows[right].module) < 0
		}
		return bytes.Compare(rows[left].call[:], rows[right].call[:]) < 0
	})
	table := &callResultTable{rows: rows, index: make(map[callResultKey]uint32, len(rows))}
	for index, row := range rows {
		key := callResultKey{module: row.module, call: row.call}
		if _, duplicate := table.index[key]; duplicate {
			return errors.New("link/boundary: duplicate mounted Call result key")
		}
		table.index[key] = uint32(index + 1)
	}
	table.content = callResultRelationID(rows)
	if !table.content.Available() {
		return errors.New("link/boundary: unavailable mounted Call result relation identity")
	}
	a.callResults = table
	return nil
}

// boundaryCallResultGeometryIndex is the Boundary construction counterpart
// of the compiler's local index. It walks every authored Values row/member
// once and records only Call terms; no per-Call Values scan is permitted.
func boundaryCallResultGeometryIndex(p *program.Program) (map[keyspace.Term]boundaryCallResultWitness, bool) {
	if p == nil || !p.Available() {
		return nil, false
	}
	index := make(map[keyspace.Term]boundaryCallResultWitness)
	ok := p.Flow().VisitCallResultGeometry(func(geometry flow.CallResultGeometry) bool {
		if _, duplicate := index[geometry.Call]; duplicate {
			return false
		}
		index[geometry.Call] = boundaryCallResultWitness{
			values: geometry.Values, value: geometry.Value, tail: geometry.Tail,
			position: geometry.Position, form: geometry.Form, multiplicity: geometry.Multiplicity, count: geometry.Count,
		}
		return true
	})
	return index, ok
}

func validCallResultRow(row callResultRow) bool {
	if !row.module.Available() || !row.call.Available() || row.application == 0 || !row.applicationID.Available() || !row.values.Available() || !row.form.Valid() {
		return false
	}
	switch row.form {
	case programschema.CallResultValue:
		return row.value.Available() && !row.tail.Available() && row.multiplicity == programschema.CallResultMultiplicityExact && row.count == 1
	case programschema.CallResultValues:
		return !row.value.Available() && row.tail.Available() && row.position == 0 && row.multiplicity.Valid() &&
			(row.multiplicity != programschema.CallResultMultiplicityOpen || row.count == 0)
	default:
		return false
	}
}

func admitCallResult(row callResultRow, result uint32) bool {
	if !validCallResultRow(row) {
		return false
	}
	if row.form == programschema.CallResultValue {
		return result == 0
	}
	return row.multiplicity == programschema.CallResultMultiplicityOpen || result < row.count
}

func orderContentID(left, right identity.ContentID) int {
	return bytes.Compare(left[:], right[:])
}

func callResultRelationID(rows []callResultRow) (id identity.ContentID) {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/link/boundary/call-result", callResultRelationVersion) != nil || writer.Record(1) != nil || writer.Count(uint64(len(rows))) != nil {
		return id
	}
	for _, row := range rows {
		if !validCallResultRow(row) || writer.Bytes(row.module[:]) != nil || writer.Bytes(row.call[:]) != nil || writer.Bytes(row.applicationID[:]) != nil || writer.Bytes(row.values[:]) != nil || writer.Uint(uint64(row.form)) != nil || writer.Uint(uint64(row.multiplicity)) != nil || writer.Uint(uint64(row.count)) != nil || writer.Bytes(row.value[:]) != nil || writer.Bytes(row.tail[:]) != nil || writer.Uint(uint64(row.position)) != nil {
			return identity.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	sum := hash.Sum(id[:0])
	if len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// CallResultRelationID identifies the sealed, target-independent mounted
// call-output geometry. The Target outcome/result plane is selected virtually
// by Calls.CallResult and is intentionally absent from this relation digest.
func (c *Component) CallResultRelationID() (identity.ContentID, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || c.authority.callResults == nil || !c.authority.callResults.content.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.callResults.content, true
}

// CallResult resolves one exact mounted ordinary Call and one already-issued
// Target outcome/result identity. The sealed consumer multiplicity admits
// result zero for fixed Value calls, a finite exact prefix for bounded Bind /
// Assign consumers, or every ordinal only for genuinely open expansion. No
// unselected Target outcome is enumerated.
func (v Calls) CallResult(module, callID, outcomeResult identity.ContentID) (CallResult, bool) {
	if v.component == nil || v.component.authority == nil || v.component.authority.component != v.component || v.component.authority.callResults == nil || !module.Available() || !callID.Available() || !outcomeResult.Available() {
		return CallResult{}, false
	}
	target := v.component.authority.target
	operation, outcome, result, targetOK := target.FindOutcomeResultID(outcomeResult)
	if !targetOK || result < 0 || uint64(result) > uint64(^uint32(0)) || outcome < 0 || uint64(outcome) > uint64(^uint32(0)) {
		return CallResult{}, false
	}
	index, found := v.component.authority.callResults.index[callResultKey{module: module, call: callID}]
	if !found || index == 0 || uint64(index) > uint64(len(v.component.authority.callResults.rows)) {
		return CallResult{}, false
	}
	row := v.component.authority.callResults.rows[index-1]
	if !admitCallResult(row, uint32(result)) {
		return CallResult{}, false
	}
	resultRow := CallResult{component: v.component, ordinal: index, outcomeResult: outcomeResult, operation: operation, outcome: uint32(outcome), result: uint32(result)}
	return resultRow, resultRow.valid()
}

// Result is a short alias for CallResult, matching the Values/Call query
// vocabulary while keeping CallResult available for callers that prefer the
// explicit name.
func (v Calls) Result(module, callID, outcomeResult identity.ContentID) (CallResult, bool) {
	return v.CallResult(module, callID, outcomeResult)
}

func (row CallResult) valid() bool {
	if row.component == nil || row.component.authority == nil || row.component.authority.component != row.component || row.component.authority.callResults == nil || row.ordinal == 0 || uint64(row.ordinal) > uint64(len(row.component.authority.callResults.rows)) || !row.outcomeResult.Available() {
		return false
	}
	stored := row.component.authority.callResults.rows[row.ordinal-1]
	if !validCallResultRow(stored) || row.operation == 0 {
		return false
	}
	application, applicationOK := row.application(stored)
	if !applicationOK || !row.component.ApplicationOperationAvailable(row.component.authority.target, application, row.operation) {
		return false
	}
	operation, outcome, result, targetOK := row.component.authority.target.FindOutcomeResultID(row.outcomeResult)
	if !targetOK || operation != row.operation || outcome != int(row.outcome) || result != int(row.result) {
		return false
	}
	if !admitCallResult(stored, row.result) {
		return false
	}
	return true
}

// application authenticates the retained Project Application row against the
// exact Project authority still held by Boundary. The dense ordinal is only a
// local lookup coordinate; the parent-issued ApplicationID and mounted
// ModuleKey/CallID tuple make the row fail closed if any source correspondence
// is stale, foreign, or inconsistent.
func (row CallResult) application(stored callResultRow) (linkproject.Application, bool) {
	if row.component == nil || row.component.authority == nil || row.component.authority.project == nil || stored.application == 0 {
		return linkproject.Application{}, false
	}
	applications := row.component.authority.project.Applications()
	application, ok := applications.At(int(stored.application - 1))
	if !ok {
		return linkproject.Application{}, false
	}
	applicationID, applicationIDOK := application.ContentID()
	_, module, callID, mountedOK := applications.Calls().MountedIdentity(application)
	if !applicationIDOK || applicationID != stored.applicationID || !mountedOK || module != stored.module || callID != stored.call {
		return linkproject.Application{}, false
	}
	return application, true
}

// Available reports whether this handle belongs to the exact Boundary that
// issued it and still names a valid Target result identity.
func (row CallResult) Available() bool { return row.valid() }

func (row CallResult) ModuleID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].module, true
}

func (row CallResult) CallID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].call, true
}

// ApplicationID returns the exact parent-issued Project Application identity
// that originated this mounted Call. It is retained to authenticate the
// factorized relation before Target outcome/result coordinates are admitted.
func (row CallResult) ApplicationID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].applicationID, true
}

// OutcomeResultID returns the exact Target identity supplied to the query.
func (row CallResult) OutcomeResultID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.outcomeResult, true
}

func (row CallResult) Operation() (vocabulary.Operation, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.operation, true
}

func (row CallResult) Outcome() (int, bool) {
	if !row.valid() {
		return 0, false
	}
	return int(row.outcome), true
}

func (row CallResult) ResultIndex() (int, bool) {
	if !row.valid() {
		return 0, false
	}
	return int(row.result), true
}

// OutcomeResult returns the exact Target coordinates authenticated by the
// supplied outcome/result identity without asking callers to decode the
// identity a second time.
func (row CallResult) OutcomeResult() (vocabulary.Operation, int, int, bool) {
	if !row.valid() {
		return 0, 0, 0, false
	}
	return row.operation, int(row.outcome), int(row.result), true
}

func (row CallResult) Form() programschema.CallResultForm {
	if !row.valid() {
		return programschema.CallResultInvalid
	}
	return row.component.authority.callResults.rows[row.ordinal-1].form
}

func (row CallResult) ValuesID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].values, true
}

func (row CallResult) ValueID() (identity.ContentID, bool) {
	if !row.valid() || row.Form() != programschema.CallResultValue {
		return identity.ContentID{}, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].value, true
}

func (row CallResult) ValuesTailID() (identity.ContentID, bool) {
	if !row.valid() || row.Form() != programschema.CallResultValues {
		return identity.ContentID{}, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].tail, true
}

// Position returns the fixed authored Value position. Open Values results do
// not fabricate a fixed position; callers use Values, whose second return is
// the exact Target result ordinal.
func (row CallResult) Position() (uint32, bool) {
	if !row.valid() || row.Form() != programschema.CallResultValue {
		return 0, false
	}
	return row.component.authority.callResults.rows[row.ordinal-1].position, true
}

// Value returns the exact Boundary Value coordinate for a fixed call result.
func (row CallResult) Value() (Value, bool) {
	if !row.valid() || row.Form() != programschema.CallResultValue {
		return Value{}, false
	}
	id := row.component.authority.callResults.rows[row.ordinal-1].value
	module := row.component.authority.callResults.rows[row.ordinal-1].module
	return row.component.Values().ForMountedSemantic(module, id)
}

// Values returns the exact enclosing Values coordinate and the selected
// Target result ordinal for an open call tail. The fixed-prefix geometry stays
// in the Program Values row; the ordinal is never collapsed into a synthetic
// zero-shaped tuple.
func (row CallResult) Values() (Value, uint32, bool) {
	if !row.valid() || row.Form() != programschema.CallResultValues {
		return Value{}, 0, false
	}
	id := row.component.authority.callResults.rows[row.ordinal-1].values
	module := row.component.authority.callResults.rows[row.ordinal-1].module
	values, valuesOK := row.component.Values().ForMountedSemantic(module, id)
	return values, row.result, valuesOK
}

// ValuesTail returns the exact mounted semantic tail producer in addition to
// Values, for consumers that need the authored open-tail witness itself.
func (row CallResult) ValuesTail() (Value, bool) {
	if !row.valid() || row.Form() != programschema.CallResultValues {
		return Value{}, false
	}
	id := row.component.authority.callResults.rows[row.ordinal-1].tail
	module := row.component.authority.callResults.rows[row.ordinal-1].module
	return row.component.Values().ForMountedSemantic(module, id)
}
