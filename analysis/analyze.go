package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

const resultFormat uint64 = 3

type AnalyzeStatus uint8

const (
	AnalyzeInvalid AnalyzeStatus = iota
	AnalyzeUnsupported
	AnalyzeIncomplete
	AnalyzeComplete
)

// CompileStatus reports whether one Link was admitted to an immutable
// reusable analyzer plan. Compilation owns only cold declarations; execution
// remains solve-local and is performed by Plan.Solve.
type CompileStatus uint8

const (
	CompileInvalid CompileStatus = iota
	CompileUnsupported
	CompileComplete
)

// Plan is an opaque immutable analyzer plan. It retains the sealed cold
// composition and its Link owner fence, but never retains a SourceAssembly,
// Solver, RuleInstance, QueryInstance, or other solve-local runtime handle.
type Plan struct {
	state *compiledState
}

type compiledState struct {
	declared *programAnalysis
	source   *link.Link
	sourceID keyspace.ContentID
	admitted bool
}

// Result is a detached projection of canonical body-root and query rows. It
// retains neither Link/domain/engine handles nor template classifications.
type Result struct {
	source  keyspace.ContentID
	content keyspace.ContentID
	bodies  []resultBody
}

type resultBody struct {
	id            keyspace.ContentID
	roots         []resultRoot
	values        []resultValue
	effectPresent bool
	effectTop     bool
	effects       []keyspace.ContentID
}

type resultRoot struct {
	id     keyspace.ContentID
	family keyspace.Family
}

// resultValue is a detached canonical Boundary value row for its Body.
type resultValue struct {
	id      keyspace.ContentID
	present bool
}

type Body struct {
	owner   *Result
	ordinal uint32
}

// Root is a detached exact executable root row of one Body.
type Root struct {
	owner *Result
	body  uint32
	index uint32
}

func Compile(source *link.Link) (*Plan, CompileStatus) {
	if source == nil || !source.ContentID().Available() {
		return nil, CompileInvalid
	}
	declared, ok := newProgramAnalysis(source)
	if !ok || declared == nil || !declared.coverageOK || len(declared.bodies) == 0 || declared.queries.value == nil || declared.queries.effect == nil {
		return nil, CompileUnsupported
	}
	state := &compiledState{declared: declared, source: source, sourceID: source.ContentID()}
	if !state.admit() {
		return nil, CompileUnsupported
	}
	state.admitted = true
	return &Plan{state: state}, CompileComplete
}

// Solve executes one fresh source transaction and returns only its detached
// public projection. A Plan may be solved repeatedly and concurrently.
func (plan *Plan) Solve(ctx context.Context) (*Result, AnalyzeStatus) {
	if ctx == nil {
		return nil, AnalyzeInvalid
	}
	if !plan.valid() {
		return nil, AnalyzeInvalid
	}
	state := plan.state
	observation, solved := state.declared.solveCanonicalBodies(ctx, state.declared.bodies)
	if !solved {
		return nil, AnalyzeIncomplete
	}
	result, detached := detachResult(state.sourceID, state.declared.bodies, observation)
	if !detached {
		return nil, AnalyzeIncomplete
	}
	return result, AnalyzeComplete
}

// SourceID is the content fence of the Link compiled into this plan.
func (plan *Plan) SourceID() keyspace.ContentID {
	if !plan.valid() {
		return keyspace.ContentID{}
	}
	return plan.state.sourceID
}

func (plan *Plan) valid() bool {
	if plan == nil || plan.state == nil || !plan.state.admitted || plan.state.declared == nil || plan.state.source == nil ||
		!plan.state.sourceID.Available() || plan.state.source.ContentID() != plan.state.sourceID {
		return false
	}
	return true
}

func (state *compiledState) admit() bool {
	if state == nil || state.declared == nil || state.source == nil || !state.sourceID.Available() || state.source.ContentID() != state.sourceID {
		return false
	}
	declared := state.declared
	if declared.composition == nil || !declared.composition.Sealed() || !declared.coverageOK || len(declared.bodies) == 0 ||
		declared.queries.value == nil || declared.queries.effect == nil || declared.callAlgebra == nil || declared.callActivation == nil ||
		declared.heapSchema.Link() != state.source || declared.valueSchema == nil || declared.valueSchema.Link() != state.source ||
		declared.callAlgebra.Link() != state.source || declared.packSchema == nil || declared.packSchema.Link() != state.source ||
		declared.effectAlgebra == nil || declared.effectAlgebra.Link() != state.source || declared.values == nil || declared.values.Schema() == nil || declared.values.Schema().Link() != state.source ||
		declared.calls == nil || declared.calls.Link() != state.source || declared.heap == nil || declared.heap.Schema().Link() != state.source ||
		declared.packs == nil || declared.packs.Schema() == nil || declared.packs.Schema().Link() != state.source || declared.effects == nil || declared.effects.Link() != state.source {
		return false
	}
	for _, body := range declared.bodies {
		if !body.valid(state.source) {
			return false
		}
	}
	return true
}

func Analyze(ctx context.Context, source *link.Link) (*Result, AnalyzeStatus) {
	if ctx == nil || source == nil || !source.ContentID().Available() {
		return nil, AnalyzeInvalid
	}
	plan, status := Compile(source)
	switch status {
	case CompileInvalid:
		return nil, AnalyzeInvalid
	case CompileUnsupported:
		return nil, AnalyzeUnsupported
	case CompileComplete:
		return plan.Solve(ctx)
	default:
		return nil, AnalyzeUnsupported
	}
}

func (result *Result) ContentID() keyspace.ContentID {
	if !result.valid() {
		return keyspace.ContentID{}
	}
	return result.content
}
func (result *Result) SourceID() keyspace.ContentID {
	if !result.valid() {
		return keyspace.ContentID{}
	}
	return result.source
}
func (result *Result) BodyCount() int {
	if !result.valid() {
		return 0
	}
	return len(result.bodies)
}

func (result *Result) BodyAt(index int) (Body, bool) {
	if !result.valid() || index < 0 || index >= len(result.bodies) {
		return Body{}, false
	}
	return Body{owner: result, ordinal: uint32(index + 1)}, true
}

func (body Body) row() (resultBody, bool) {
	if body.owner == nil || !body.owner.valid() || body.ordinal == 0 || uint64(body.ordinal) > uint64(len(body.owner.bodies)) {
		return resultBody{}, false
	}
	return body.owner.bodies[body.ordinal-1], true
}
func (body Body) ID() (keyspace.ContentID, bool) { row, ok := body.row(); return row.id, ok }
func (body Body) RootCount() int {
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.roots)
}
func (body Body) RootAt(index int) (Root, bool) {
	row, ok := body.row()
	if !ok || index < 0 || index >= len(row.roots) {
		return Root{}, false
	}
	return Root{owner: body.owner, body: body.ordinal, index: uint32(index + 1)}, true
}
func (root Root) row() (resultRoot, bool) {
	if root.owner == nil || !root.owner.valid() || root.body == 0 || root.index == 0 || uint64(root.body) > uint64(len(root.owner.bodies)) {
		return resultRoot{}, false
	}
	rows := root.owner.bodies[root.body-1].roots
	if uint64(root.index) > uint64(len(rows)) {
		return resultRoot{}, false
	}
	return rows[root.index-1], true
}
func (root Root) ID() (keyspace.ContentID, bool) { row, ok := root.row(); return row.id, ok }
func (root Root) Family() keyspace.Family {
	row, ok := root.row()
	if !ok {
		return keyspace.FamilyInvalid
	}
	return row.family
}

func (body Body) EffectDisposition() (present, top, ok bool) {
	row, ok := body.row()
	return row.effectPresent, row.effectTop, ok
}
func (body Body) EffectCount() int {
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.effects)
}
func (body Body) EffectAt(index int) (keyspace.ContentID, bool) {
	row, ok := body.row()
	if !ok || index < 0 || index >= len(row.effects) {
		return keyspace.ContentID{}, false
	}
	return row.effects[index], true
}

// ValueCount and ValueAt expose the per-body projection of the declared Value
// query. A body with no canonical coordinates has a valid empty projection.
func (body Body) ValueCount() int {
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.values)
}
func (body Body) ValueAt(index int) (id keyspace.ContentID, present, ok bool) {
	row, rowOK := body.row()
	if !rowOK || index < 0 || index >= len(row.values) {
		return keyspace.ContentID{}, false, false
	}
	value := row.values[index]
	return value.id, value.present, true
}

func detachResult(sourceID keyspace.ContentID, bodies []mountedBody, observation programObservation) (*Result, bool) {
	if !sourceID.Available() || len(bodies) == 0 || len(observation.bodies) != len(bodies) {
		return nil, false
	}
	result := &Result{source: sourceID, bodies: make([]resultBody, len(bodies))}
	for index, body := range bodies {
		observed := observation.bodies[index]
		id, idOK := bodyResultID(body)
		roots, rootsOK := detachBodyRoots(body)
		if !idOK || !rootsOK || !observed.values.valid || len(observed.values.ids) != len(observed.values.present) || !observed.effect.valid {
			return nil, false
		}
		values := make([]resultValue, len(observed.values.ids))
		for valueIndex, valueID := range observed.values.ids {
			if !valueID.Available() {
				return nil, false
			}
			values[valueIndex] = resultValue{id: valueID, present: observed.values.present[valueIndex]}
		}
		result.bodies[index] = resultBody{id: id, roots: roots, values: values, effectPresent: observed.effect.present, effectTop: observed.effect.top, effects: append([]keyspace.ContentID(nil), observed.effect.atoms...)}
	}
	result.content, _ = analysisResultID(result.source, result.bodies)
	return result, result.valid()
}

func bodyResultID(body mountedBody) (keyspace.ContentID, bool) {
	if !body.valid(body.linked) {
		return keyspace.ContentID{}, false
	}
	var term [8]byte
	binary.BigEndian.PutUint64(term[:], uint64(body.term))
	programID := body.program.ContentID()
	return analysisContentID("analysis/body-result", programID[:], []byte(body.module), term[:])
}

func detachBodyRoots(body mountedBody) ([]resultRoot, bool) {
	if !body.valid(body.linked) {
		return nil, false
	}
	count, countOK := body.program.Source().Index().BodyRootLen(body.term)
	if !countOK || count < 0 {
		return nil, false
	}
	rows := make([]resultRoot, 0, count)
	for index := 0; index < count; index++ {
		term, termOK := body.program.Source().Index().BodyRootAt(body.term, index)
		if !termOK || term == 0 || !body.program.Flow().Executable().Contains(term) {
			return nil, false
		}
		var payload [8]byte
		binary.BigEndian.PutUint64(payload[:], uint64(term))
		programID := body.program.ContentID()
		id, idOK := analysisContentID("analysis/body-root", programID[:], payload[:])
		if !idOK {
			return nil, false
		}
		rows = append(rows, resultRoot{id: id, family: keyspace.TermFamily(term)})
	}
	return rows, true
}

func (result *Result) valid() bool {
	if result == nil || !result.source.Available() || !result.content.Available() || len(result.bodies) == 0 {
		return false
	}
	for _, body := range result.bodies {
		if !body.id.Available() || body.effectTop && len(body.effects) != 0 {
			return false
		}
		for _, root := range body.roots {
			if !root.id.Available() || root.family == keyspace.FamilyInvalid {
				return false
			}
		}
		for _, value := range body.values {
			if !value.id.Available() {
				return false
			}
		}
		for _, effect := range body.effects {
			if !effect.Available() {
				return false
			}
		}
	}
	return true
}

func analysisResultID(source keyspace.ContentID, bodies []resultBody) (keyspace.ContentID, bool) {
	if !source.Available() || len(bodies) == 0 {
		return keyspace.ContentID{}, false
	}
	hash := sha256.New()
	write := func(value []byte) bool { return writeFramedHash(hash, value) }
	var version, count [8]byte
	binary.BigEndian.PutUint64(version[:], resultFormat)
	binary.BigEndian.PutUint64(count[:], uint64(len(bodies)))
	if !write([]byte("analysis/result")) || !write(version[:]) || !write(source[:]) || !write(count[:]) {
		return keyspace.ContentID{}, false
	}
	for _, body := range bodies {
		binary.BigEndian.PutUint64(count[:], uint64(len(body.roots)))
		if !write(body.id[:]) || !write(count[:]) {
			return keyspace.ContentID{}, false
		}
		for _, root := range body.roots {
			if !write(root.id[:]) || !write([]byte{byte(root.family)}) {
				return keyspace.ContentID{}, false
			}
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(body.values)))
		if !write(count[:]) {
			return keyspace.ContentID{}, false
		}
		for _, value := range body.values {
			if !write(value.id[:]) || !write([]byte{boolByte(value.present)}) {
				return keyspace.ContentID{}, false
			}
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(body.effects)))
		if !write([]byte{boolByte(body.effectPresent), boolByte(body.effectTop)}) || !write(count[:]) {
			return keyspace.ContentID{}, false
		}
		for _, effect := range body.effects {
			if !write(effect[:]) {
				return keyspace.ContentID{}, false
			}
		}
	}
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
