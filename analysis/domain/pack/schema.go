package pack

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// Root and Values are Pack-private selectors over direct Program topology.
// Every root is one pack-valued occurrence: a Program Values expression, an
// ordinary-call boundary, or an open-tail producer.  In particular,
// two calls in one body never share a Pack factor coordinate.
type Root struct {
	schema *schema
	index  uint32
}
type Values struct {
	schema *schema
	index  uint32
}

func (root Root) valid() bool {
	return root.schema != nil && uint64(root.index) < uint64(len(root.schema.relations))
}
func (values Values) valid() bool {
	return values.schema != nil && uint64(values.index) < uint64(len(values.schema.values))
}

type rootKind uint8

const (
	rootInvalid rootKind = iota
	rootValues
	rootCall
	rootTail
	// The following roots are the small P0 boundary plane.  They are still
	// Pack roots (and consequently carry exactly one whole-Pack equation); the
	// source terms which identify them remain cold in the descriptor rows.
	rootBind
	rootBody
	rootOutcome
)

type rootRow struct {
	kind        rootKind
	shard       linkproject.Shard
	term        keyspace.Term
	application linkproject.Application
	port        Port
	sourceIndex uint32
	id          keyspace.ContentID
}

type valuesRow struct {
	root  uint32
	shard linkproject.Shard
	term  keyspace.Term
	port  Port
	fixed []Endpoint
	tail  Port
}

type callRow struct {
	root        uint32
	application linkproject.Application
	port        Port
	fixed       []Endpoint
	tail        Port
}

type tailRow struct {
	root  uint32
	shard linkproject.Shard
	term  keyspace.Term
	port  Port
	kind  TailProducerKind
}

// bindRow is the exact Source.BindOrder/Flow Bind occurrence descriptor.  A
// bind does not own Cell values; it only names the existing Cell endpoints
// which a Pack adjustment may expose.  The Value source remains a separate
// root and Value owns the Cell transfer itself.
type bindRow struct {
	root   uint32
	shard  linkproject.Shard
	term   keyspace.Term
	body   keyspace.Term
	values Values
	port   Port
	cells  []Endpoint
}

// bodyRow is one executable Program Body.  It deliberately has no caller or
// Application dimension: Call's Body capability is the only interprocedural
// selector.  Formal Cells and the Flow entry port are cold descriptors used
// by the body-boundary rules.
type bodyRow struct {
	root          uint32
	shard         linkproject.Shard
	term          keyspace.Term
	function      keyspace.Term
	port          Port
	entry         keyspace.Term
	formals       []Endpoint
	normal        uint32
	returnOutcome uint32
	hasReturn     bool
}

// outcomeRow names one Flow Body Outcome. Normal completion has no Values;
// an explicit return keeps every executable authored Values alternative in
// order. Outcome is a Flow-owned derived term; Pack stores only the exact
// occurrence descriptor and the Pack port issued for it.
type outcomeRow struct {
	root   uint32
	shard  linkproject.Shard
	term   keyspace.Term
	body   keyspace.Term
	kind   flowkind.OutcomeKind
	values []keyspace.Term
	port   Port
	finish keyspace.Term
}

type cellKey struct {
	shard linkproject.Shard
	term  keyspace.Term
}

type bodyKey struct {
	shard linkproject.Shard
	term  keyspace.Term
}

type outcomeKey struct {
	shard linkproject.Shard
	term  keyspace.Term
}

type schema struct {
	source *link.Link
	owner  *algebra

	roots     []rootRow
	relations []*relation
	values    []valuesRow
	calls     []callRow
	tails     []tailRow
	binds     []bindRow
	bodies    []bodyRow
	outcomes  []outcomeRow

	valueIndex    map[programValuesKey]uint32
	callIndex     map[linkproject.Application]uint32
	tailIndex     map[programValuesKey]uint32
	bindIndex     map[programValuesKey]uint32
	bodyIndex     map[bodyKey]uint32
	outcomeIndex  map[outcomeKey]uint32
	relationIndex map[*relation]uint32
	endpoints     []linkboundary.Value
	endpointIndex map[linkboundary.Value]Endpoint
	cellIndex     map[cellKey]Endpoint
}

type programValuesKey struct {
	shard linkproject.Shard
	term  keyspace.Term
}

type Schema struct{ state *schema }

// Link returns the one immutable Link whose Pack occurrences this Schema
// represents. Consumers use it only to authenticate shared domain owners.
func (schema *Schema) Link() *link.Link {
	if schema == nil || schema.state == nil {
		return nil
	}
	return schema.state.source
}

// Seal records only canonical Link/Program occurrences.  It does not create
// an Application×Target table: Call roots are one-per-Link Application and a
// Target InputSource is interpreted later by Pack's InputSelector.
func Seal(source *link.Link, authority *static.Authority) (*Schema, bool) {
	if source == nil || authority == nil || authority.Link() != source || authority.LinkID() != source.ContentID() {
		return nil, false
	}
	classes := authority.Classes()
	if classes == nil {
		return nil, false
	}
	offsets, offsetsOK := selectionOffsets(source)
	if !offsetsOK {
		return nil, false
	}
	owner, ok := newAlgebraWithOffsets(classes, nil, offsets)
	if !ok {
		return nil, false
	}
	boundary := source.Boundary()
	if boundary == nil {
		return nil, false
	}
	state := &schema{
		source: source, owner: owner,
		valueIndex:    make(map[programValuesKey]uint32),
		callIndex:     make(map[linkproject.Application]uint32),
		tailIndex:     make(map[programValuesKey]uint32),
		bindIndex:     make(map[programValuesKey]uint32),
		bodyIndex:     make(map[bodyKey]uint32),
		outcomeIndex:  make(map[outcomeKey]uint32),
		relationIndex: make(map[*relation]uint32),
		endpointIndex: make(map[linkboundary.Value]Endpoint, source.Boundary().Values().Count()),
		cellIndex:     make(map[cellKey]Endpoint),
	}
	valuesView := boundary.Values()
	for index := 0; index < valuesView.Count(); index++ {
		value, valueOK := valuesView.At(index)
		endpoint, endpointOK := newEndpoint(owner, uint32(index+1), classes.AnyValue())
		if !valueOK || !endpointOK {
			return nil, false
		}
		state.endpoints = append(state.endpoints, value)
		state.endpointIndex[value] = endpoint
	}

	// Values roots come first in canonical shard/Program order.  Registering
	// their tails on demand gives every Call/Vararg tail an owner-issued Pack
	// port without pretending it is another Values expression.
	mounts := source.Project().Mounts()
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, shardOK := mounts.At(shardIndex)
		p, programOK := mounts.Program(shard)
		if !shardOK || !programOK || p == nil {
			return nil, false
		}
		flowView := p.Flow()
		authored := flowView.Authored()
		values := authored.Values()
		for valueIndex := 0; valueIndex < values.Count(); valueIndex++ {
			term, termOK := values.At(valueIndex)
			if !termOK {
				return nil, false
			}
			// Authored Values includes static and dead expressions. Pack roots
			// are runtime occurrences only, so the executable plane is the
			// admission authority rather than the authored denominator.
			if !flowView.Executable().Contains(term) {
				continue
			}
			if !state.addValuesRoot(classes, shard, authored, flowView, term) {
				return nil, false
			}
		}
	}

	// A Call root is deliberately separate even when calls share a body. Its
	// source item is the receiver prepended to the call's actual Values.
	applications := source.Project().Applications()
	callApplications := applications.Calls()
	calls := boundary.Calls()
	mounts = source.Project().Mounts()
	for applicationIndex := 0; applicationIndex < callApplications.Count(); applicationIndex++ {
		application, applicationOK := callApplications.At(applicationIndex)
		shard, callTerm, callOK := applications.Call(application)
		p, programOK := mounts.Program(shard)
		form, receiver, actuals, operandsOK := calls.CallOperands(application)
		if !applicationOK || !callOK || !programOK || !operandsOK || p == nil {
			return nil, false
		}
		flowView := p.Flow()
		if !flowView.Executable().Contains(callTerm) {
			continue
		}
		if !state.addCallRoot(classes, application, shard, callTerm, flowView, form, receiver, actuals) {
			return nil, false
		}
	}

	// P0's remaining roots are the exact lexical/boundary occurrences needed
	// by Pack's adjustment and direct-body rules.  They are emitted once per
	// Program occurrence/body/outcome.  No Application×Body or
	// Program×Function product is materialized here.
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, shardOK := mounts.At(shardIndex)
		p, programOK := mounts.Program(shard)
		if !shardOK || !programOK || p == nil {
			return nil, false
		}
		if !state.addCellEndpoints(classes, shard, p) ||
			!state.addBindRoots(classes, shard, p) ||
			!state.addBodyRoots(classes, shard, p) {
			return nil, false
		}
	}
	if len(state.roots) == 0 || !state.validateBodyOutcomeRoots() {
		return nil, false
	}
	return &Schema{state: state}, true
}

func selectionOffsets(source *link.Link) ([]nat, bool) {
	if source == nil {
		return nil, false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return nil, false
	}
	maximum := 0
	for index := 0; index < contract.OperationCount(); index++ {
		operation, operationOK := contract.OperationAt(index)
		if !operationOK {
			return nil, false
		}
		if count := contract.ValueFormalCount(operation); count > 0 && count-1 > maximum {
			// Target value formals are zero-based selection indexes. Keep
			// offset zero as the mandatory closed/tail head, then retain only
			// the largest actual formal index.
			maximum = count - 1
		}
	}
	// Heap's indexed-write payload is a second Pack scalar-selection
	// consumer.  Its source position is owned by Program, not reconstructed
	// by Heap: an executable candidate IndexSet Write names the exact RHS
	// Values term and the Lua-adjusted source index. Only an open Values tail
	// needs a Pack offset; fixed members and nil fill need none. Freeze every required
	// tail offset now so TableIndex can never intern a new recurrent handle.
	mounts := source.Project().Mounts()
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, shardOK := mounts.At(shardIndex)
		p, programOK := mounts.Program(shard)
		if !shardOK || !programOK || p == nil {
			return nil, false
		}
		flowView := p.Flow()
		// Body entry selects every Source.Formals position and its exclusive
		// residual tail. Freeze the complete Program denominator before the
		// algebra is sealed; deriving it only from Target/Access/Bind misses
		// receiver and open-vararg body offsets.
		formalOrder := p.Source().Formals()
		functionCount := p.Source().Identity().FamilyCount(keyspace.FamilyFunction)
		for functionOrdinal := 1; functionOrdinal <= functionCount; functionOrdinal++ {
			function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(functionOrdinal))
			width, widthOK := formalOrder.Len(function)
			if !widthOK || width < 0 {
				return nil, false
			}
			if width > maximum {
				maximum = width
			}
		}
		var offsetsOK bool
		maximum, offsetsOK = selectionOffsetsForProgram(maximum, flowView.AccessGeometry(), flowView.Authored().Values())
		if !offsetsOK {
			return nil, false
		}
		// Fixed bind arities are also scalar-selection sites.  They are
		// authored by Source.BindOrder and must be frozen before the algebra
		// is sealed; the recurrent solver never interns an offset.
		binds := flowView.Authored().Storage().Binds()
		bindOrder := p.Source().Binds()
		for bindIndex := 0; bindIndex < binds.Count(); bindIndex++ {
			bind, bindOK := binds.At(bindIndex)
			if !bindOK {
				return nil, false
			}
			if !flowView.Executable().Contains(bind) {
				continue
			}
			width, widthOK := bindOrder.Len(bind)
			if !widthOK || width < 0 {
				return nil, false
			}
			if width > 0 && width-1 > maximum {
				maximum = width - 1
			}
			// An open bind also publishes the residual Pack after the
			// fixed slice.  Its tail offset is exactly `width`, so freeze
			// that one additional selector from the authored Values row;
			// closed binds need no exclusive offset.
			_, valueTerm, valuesOK := binds.Get(bind)
			_, tail, tailOK := flowView.Authored().Values().Get(valueTerm)
			if !valuesOK || !tailOK {
				return nil, false
			}
			if tail != 0 && width > maximum {
				maximum = width
			}
		}
	}
	if maximum < 0 || uint64(maximum) > uint64(^uint32(0)) {
		return nil, false
	}
	offsets := make([]nat, maximum+1)
	for index := range offsets {
		offsets[index] = natFromUint64(uint64(index))
	}
	return offsets, true
}

// selectionOffsetsForProgram consumes one owner-issued AccessGeometry plane.
// Its availability is a semantic prerequisite: a zero-row typed plane is
// valid, while an unavailable plane must not be treated as an empty candidate
// denominator. Values position lookup remains the authored Flow authority for
// the candidate write's exact Lua-adjusted tail offset.
func selectionOffsetsForProgram(maximum int, geometry flow.AccessGeometry, values flow.Values) (int, bool) {
	if !geometry.Available() {
		return maximum, false
	}
	writes := geometry.IndexAccesses().Writes()
	for writeIndex := 0; writeIndex < writes.Count(); writeIndex++ {
		write, writeOK := writes.At(writeIndex)
		if !writeOK {
			return maximum, false
		}
		_, _, valuesTerm, sourceIndex, _, operandsOK := writes.Get(write)
		if !operandsOK {
			// The candidate write plane is Flow's executable IndexSet
			// denominator. A malformed candidate cannot be ignored: its
			// exact Values position must be covered by the frozen offsets.
			return maximum, false
		}
		position, positionOK := values.Position(valuesTerm, sourceIndex)
		if !positionOK {
			return maximum, false
		}
		if position.Tail != 0 && position.TailOffset > maximum {
			maximum = position.TailOffset
		}
	}
	return maximum, true
}

func (state *schema) addRoot(classes *static.ClassSet, kind rootKind, shard linkproject.Shard, term keyspace.Term, application linkproject.Application) (uint32, Port, bool) {
	if state == nil || state.owner == nil || classes == nil || kind == rootInvalid {
		return 0, Port{}, false
	}
	port, ok := newPort(state.owner, uint32(len(state.roots)+1), classes.AnyValue(), true)
	if !ok {
		return 0, Port{}, false
	}
	index := uint32(len(state.roots))
	id, idOK := state.rootID(kind, shard, term, application)
	if !idOK || !id.Available() {
		return 0, Port{}, false
	}
	row := rootRow{kind: kind, shard: shard, term: term, application: application, port: port, id: id}
	state.roots = append(state.roots, row)
	relation := &relation{owner: state.owner, index: index + 1, targets: []equationTarget{{kind: EquationPack, index: port.index}}}
	state.relations = append(state.relations, relation)
	if !relation.valid() {
		return 0, Port{}, false
	}
	state.relationIndex[relation] = index
	return index, port, true
}

func (state *schema) rootID(kind rootKind, shard linkproject.Shard, term keyspace.Term, application linkproject.Application) (keyspace.ContentID, bool) {
	if state == nil || state.source == nil || !state.source.ContentID().Available() || kind == rootInvalid {
		return keyspace.ContentID{}, false
	}
	var occurrence keyspace.ContentID
	switch kind {
	case rootValues:
		mountIndex, mountOK := state.source.Project().Mounts().Index(shard)
		if !mountOK {
			return keyspace.ContentID{}, false
		}
		occurrence = programOccurrenceID(state.source.ContentID(), uint32(mountIndex+1), term, kind)
	case rootTail:
		values := state.source.Boundary().Values()
		value, valueOK := values.Of(shard, term)
		id, idOK := values.ID(value)
		if !valueOK || !idOK {
			return keyspace.ContentID{}, false
		}
		occurrence = id
	case rootCall:
		project := state.source.Project()
		if project == nil {
			return keyspace.ContentID{}, false
		}
		id, idOK := project.ApplicationID(application)
		if !idOK {
			return keyspace.ContentID{}, false
		}
		occurrence = id
	case rootBind, rootBody:
		mountIndex, mountOK := state.source.Project().Mounts().Index(shard)
		if !mountOK {
			return keyspace.ContentID{}, false
		}
		occurrence = programOccurrenceID(state.source.ContentID(), uint32(mountIndex+1), term, kind)
	case rootOutcome:
		mountIndex, mountOK := state.source.Project().Mounts().Index(shard)
		if !mountOK {
			return keyspace.ContentID{}, false
		}
		occurrence = programOccurrenceID(state.source.ContentID(), uint32(mountIndex+1), term, kind)
	default:
		return keyspace.ContentID{}, false
	}
	return rootedOccurrenceID(kind, occurrence), occurrence.Available()
}

func (state *schema) addValuesRoot(classes *static.ClassSet, shard linkproject.Shard, authored flow.Authored, flowView flow.View, term keyspace.Term) bool {
	if state == nil || !flowView.Executable().Contains(term) {
		return false
	}
	values := authored.Values()
	key := programValuesKey{shard: shard, term: term}
	if _, exists := state.valueIndex[key]; exists {
		return false
	}
	root, port, ok := state.addRoot(classes, rootValues, shard, term, linkproject.Application{})
	if !ok {
		return false
	}
	row := valuesRow{root: root, shard: shard, term: term, port: port}
	owner, tail, valuesOK := values.Get(term)
	fixed, fixedOK := values.Len(term)
	if !valuesOK || owner == 0 || !fixedOK {
		return false
	}
	for member := 0; member < fixed; member++ {
		child, childOK := values.Member(term, member)
		value, valueOK := state.source.Boundary().Values().Of(shard, child)
		endpoint, endpointOK := state.endpointIndex[value]
		if !childOK || !valueOK || !endpointOK {
			return false
		}
		row.fixed = append(row.fixed, endpoint)
	}
	if tail != 0 {
		port, tailOK := state.addTailRoot(classes, shard, tail, flowView)
		if !tailOK {
			return false
		}
		row.tail = port
	}
	state.valueIndex[key] = uint32(len(state.values))
	state.values = append(state.values, row)
	state.roots[root].sourceIndex = uint32(len(state.values) - 1)
	return true
}

func (state *schema) addTailRoot(classes *static.ClassSet, shard linkproject.Shard, term keyspace.Term, flowView flow.View) (Port, bool) {
	kind, kindOK := tailProducerKind(flowView, term)
	if !kindOK {
		return Port{}, false
	}
	key := programValuesKey{shard: shard, term: term}
	if index, found := state.tailIndex[key]; found {
		row := state.tails[index]
		return row.port, row.port.valid() && row.kind == kind
	}
	root, port, ok := state.addRoot(classes, rootTail, shard, term, linkproject.Application{})
	if !ok {
		return Port{}, false
	}
	state.tailIndex[key] = uint32(len(state.tails))
	state.tails = append(state.tails, tailRow{root: root, shard: shard, term: term, port: port, kind: kind})
	state.roots[root].sourceIndex = uint32(len(state.tails) - 1)
	return port, true
}

func tailProducerKind(flowView flow.View, term keyspace.Term) (TailProducerKind, bool) {
	if !flowView.ContentID().Available() || !flowView.Executable().Contains(term) {
		return TailProducerInvalid, false
	}
	authored := flowView.Authored()
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCall:
		_, _, _, _, callOK := authored.Calls().Get(term)
		boundary, boundaryOK := flowView.Causal().Boundaries().For(term)
		if !callOK || !boundaryOK || boundary.Call != term {
			return TailProducerInvalid, false
		}
		return TailProducerCall, true
	case keyspace.FamilyVararg:
		_, _, varargOK := authored.Storage().Varargs().Get(term)
		if !varargOK {
			return TailProducerInvalid, false
		}
		return TailProducerVararg, true
	default:
		return TailProducerInvalid, false
	}
}

func (state *schema) addCallRoot(classes *static.ClassSet, application linkproject.Application, shard linkproject.Shard, callTerm keyspace.Term, flowView flow.View, form flow.CallForm, receiver, actuals linkboundary.Value) bool {
	if state == nil || !flowView.Executable().Contains(callTerm) {
		return false
	}
	if form != flow.CallFormPlain && form != flow.CallFormMethod {
		return false
	}
	if _, exists := state.callIndex[application]; exists {
		return false
	}
	authored := flowView.Authored()
	owner, _, _, authoredActualTerm, operandsOK := authored.Calls().Get(callTerm)
	values := authored.Values()
	actualShard, actualTerm, actualValueOK := state.source.Boundary().Values().Origin(actuals)
	actualOwner, tail, valuesOK := values.Get(actualTerm)
	if !operandsOK || !actualValueOK || actualShard != shard || authoredActualTerm != actualTerm || actualTerm == 0 || !valuesOK || actualOwner != owner {
		return false
	}
	root, port, rootOK := state.addRoot(classes, rootCall, shard, actualTerm, application)
	if !rootOK {
		return false
	}
	row := callRow{root: root, application: application, port: port}
	if form == flow.CallFormMethod {
		if receiver == (linkboundary.Value{}) {
			return false
		}
		endpoint, endpointOK := state.endpointIndex[receiver]
		if !endpointOK {
			return false
		}
		row.fixed = append(row.fixed, endpoint)
	} else if receiver != (linkboundary.Value{}) {
		return false
	}
	fixed, fixedOK := values.Len(actualTerm)
	if !fixedOK {
		return false
	}
	for index := 0; index < fixed; index++ {
		term, termOK := values.Member(actualTerm, index)
		value, valueOK := state.source.Boundary().Values().Of(shard, term)
		endpoint, endpointOK := state.endpointIndex[value]
		if !termOK || !valueOK || !endpointOK {
			return false
		}
		row.fixed = append(row.fixed, endpoint)
	}
	if tail != 0 {
		tailPort, tailOK := state.addTailRoot(classes, shard, tail, flowView)
		if !tailOK {
			return false
		}
		row.tail = tailPort
	}
	state.callIndex[application] = uint32(len(state.calls))
	state.calls = append(state.calls, row)
	state.roots[root].sourceIndex = uint32(len(state.calls) - 1)
	return true
}

// cellEndpoint issues the one Pack scalar handle for an existing storage Cell.
// Cell contents are deliberately not copied into Pack; a Cell endpoint only
// provides a stable subject for Pack terms used by bind/body boundaries.
func (state *schema) cellEndpoint(classes *static.ClassSet, shard linkproject.Shard, cell keyspace.Term) (Endpoint, bool) {
	if state == nil || classes == nil || shard == (linkproject.Shard{}) || keyspace.TermFamily(cell) != keyspace.FamilyCell || keyspace.TermOrdinal(cell) == 0 {
		return Endpoint{}, false
	}
	key := cellKey{shard: shard, term: cell}
	if endpoint, ok := state.cellIndex[key]; ok {
		return endpoint, endpoint.valid()
	}
	endpoint, ok := newEndpoint(state.owner, uint32(len(state.endpoints)+len(state.cellIndex)+1), classes.AnyValue())
	if !ok {
		return Endpoint{}, false
	}
	state.cellIndex[key] = endpoint
	return endpoint, true
}

func (state *schema) addCellEndpoints(classes *static.ClassSet, shard linkproject.Shard, p *program.Program) bool {
	if state == nil || classes == nil || p == nil {
		return false
	}
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		cell, ok := cells.At(index)
		if !ok {
			return false
		}
		if _, ok := state.cellEndpoint(classes, shard, cell); !ok {
			return false
		}
	}
	return true
}

func (state *schema) appendOccurrenceRoot(classes *static.ClassSet, kind rootKind, shard linkproject.Shard, term keyspace.Term, port Port) (uint32, bool) {
	if state == nil || classes == nil || kind == rootInvalid || !port.valid() || port.owner != state.owner {
		return 0, false
	}
	root, _, ok := state.addRootWithPort(classes, kind, shard, term, linkproject.Application{}, port)
	return root, ok
}

// addRootWithPort is the common root admission path for rows whose port has
// already been allocated as part of the exact descriptor.  Keeping this
// separate from addRoot prevents callers from supplying raw relation targets.
func (state *schema) addRootWithPort(classes *static.ClassSet, kind rootKind, shard linkproject.Shard, term keyspace.Term, application linkproject.Application, supplied Port) (uint32, Port, bool) {
	return state.addRootWithPortTargets(classes, kind, shard, term, application, supplied, nil)
}

// addRootWithPortTargets admits a root whose complete output relation also
// includes existing scalar Cell endpoints.  Bind is the only P0 occurrence
// that uses this path: its Pack residual and fixed Cell outputs are one
// atomic equation, never two independently publishable facts.
func (state *schema) addRootWithPortTargets(classes *static.ClassSet, kind rootKind, shard linkproject.Shard, term keyspace.Term, application linkproject.Application, supplied Port, scalars []Endpoint) (uint32, Port, bool) {
	if state == nil || classes == nil || kind == rootInvalid || !supplied.valid() || supplied.owner != state.owner {
		return 0, Port{}, false
	}
	index := uint32(len(state.roots))
	id, idOK := state.rootID(kind, shard, term, application)
	if !idOK || !id.Available() {
		return 0, Port{}, false
	}
	row := rootRow{kind: kind, shard: shard, term: term, application: application, port: supplied, id: id}
	state.roots = append(state.roots, row)
	targets := make([]equationTarget, 0, len(scalars)+1)
	for _, endpoint := range scalars {
		if !endpoint.valid() || endpoint.owner != state.owner {
			return 0, Port{}, false
		}
		targets = append(targets, equationTarget{kind: EquationScalar, index: endpoint.index})
	}
	sort.Slice(targets, func(left, right int) bool { return compareTarget(targets[left], targets[right]) < 0 })
	for index := 1; index < len(targets); index++ {
		if compareTarget(targets[index-1], targets[index]) >= 0 {
			return 0, Port{}, false
		}
	}
	targets = append(targets, equationTarget{kind: EquationPack, index: supplied.index})
	relation := &relation{owner: state.owner, index: index + 1, targets: targets}
	state.relations = append(state.relations, relation)
	if !relation.valid() {
		return 0, Port{}, false
	}
	state.relationIndex[relation] = index
	return index, supplied, true
}

func (state *schema) addBindRoots(classes *static.ClassSet, shard linkproject.Shard, p *program.Program) bool {
	if state == nil || classes == nil || p == nil {
		return false
	}
	flowView := p.Flow()
	storage := flowView.Authored().Storage()
	binds := storage.Binds()
	bindOrder := p.Source().Binds()
	values := flowView.Authored().Values()
	for index := 0; index < binds.Count(); index++ {
		bind, bindOK := binds.At(index)
		body, valueTerm, rowOK := binds.Get(bind)
		if !bindOK || !rowOK || !flowView.Executable().Contains(bind) || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermFamily(valueTerm) != keyspace.FamilyValues {
			if !bindOK || !rowOK {
				return false
			}
			continue
		}
		width, widthOK := bindOrder.Len(bind)
		if !widthOK || width < 0 {
			return false
		}
		input, _, inputRootOK := (&Schema{state: state}).Values(shard, valueTerm)
		if !inputRootOK {
			return false
		}
		cells := make([]Endpoint, 0, width)
		for position := 0; position < width; position++ {
			cell, cellOK := bindOrder.At(bind, position)
			endpoint, endpointOK := state.cellEndpoint(classes, shard, cell)
			if !cellOK || !endpointOK {
				return false
			}
			cells = append(cells, endpoint)
		}
		port, portOK := newPort(state.owner, uint32(len(state.roots)+1), classes.AnyValue(), false)
		if !portOK {
			return false
		}
		root, port, rootOK := state.addRootWithPortTargets(classes, rootBind, shard, bind, linkproject.Application{}, port, cells)
		if !rootOK {
			return false
		}
		row := bindRow{root: root, shard: shard, term: bind, body: body, values: input, port: port, cells: cells}
		// The authored Values width is intentionally checked only as the
		// fixed source shape. An open tail remains a Pack fact and is not
		// expanded into copied cells here.
		if _, widthOK := values.Len(valueTerm); !widthOK {
			return false
		}
		state.bindIndex[programValuesKey{shard: shard, term: bind}] = uint32(len(state.binds))
		state.binds = append(state.binds, row)
		state.roots[root].sourceIndex = uint32(len(state.binds) - 1)
	}
	return true
}

func (state *schema) addBodyRoots(classes *static.ClassSet, shard linkproject.Shard, p *program.Program) bool {
	if state == nil || classes == nil || p == nil {
		return false
	}
	flowView := p.Flow()
	functions := flowView.Authored().Functions()
	functionForBody := make(map[keyspace.Term]keyspace.Term)
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		_, body, _, rowOK := functions.Get(function)
		if !ok || !rowOK {
			return false
		}
		if body != 0 {
			functionForBody[body] = function
		}
	}
	// Index Flow's executable Return occurrences once per Program. Body rows
	// consume these exact alternatives without rescanning the complete Return
	// list (which would create O(Body×Return) seal work).
	returnValuesByOutcome := make(map[keyspace.Term][]keyspace.Term)
	returnOutcomeByBody := make(map[keyspace.Term]keyspace.Term)
	outcomes := flowView.Outcomes()
	for outcomeIndex := 0; outcomeIndex < outcomes.Count(); outcomeIndex++ {
		outcomeTerm, outcomeOK := outcomes.At(outcomeIndex)
		outcome, outcomeInfoOK := outcomes.Get(outcomeTerm)
		if !outcomeOK || !outcomeInfoOK {
			return false
		}
		if outcome.Kind != flowkind.OutcomeReturn || outcome.Target != 0 {
			continue
		}
		if previous, exists := returnOutcomeByBody[outcome.Body]; exists && previous != outcomeTerm {
			return false
		}
		returnOutcomeByBody[outcome.Body] = outcomeTerm
	}
	returns := flowView.Authored().Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		ret, retOK := returns.At(index)
		owner, values, rowOK := returns.Get(ret)
		if !retOK || !rowOK {
			return false
		}
		if !flowView.Executable().Contains(ret) {
			continue
		}
		if values == 0 || !flowView.Executable().Contains(values) {
			return false
		}
		valuesOwner, _, valuesOK := flowView.Authored().Values().Get(values)
		exit, exitOK := flowView.Outcomes().ReturnExit(ret)
		ownerActivation, ownerActivationOK := flowView.Activation().For(owner)
		valuesActivation, valuesActivationOK := flowView.Activation().For(valuesOwner)
		if !valuesOK || !ownerActivationOK || !valuesActivationOK || ownerActivation != valuesActivation || !exitOK {
			return false
		}
		seen := make(map[keyspace.Term]struct{})
		for steps := 0; ; steps++ {
			if steps >= flowView.Outcomes().Count() || exit == 0 {
				return false
			}
			if _, duplicate := seen[exit]; duplicate {
				return false
			}
			seen[exit] = struct{}{}
			exitInfo, infoOK := flowView.Outcomes().Get(exit)
			exitActivation, exitActivationOK := flowView.Activation().For(exitInfo.Body)
			if !infoOK || exitInfo.Kind != flowkind.OutcomeReturn || !exitActivationOK || exitActivation != ownerActivation {
				return false
			}
			returnValuesByOutcome[exit] = append(returnValuesByOutcome[exit], values)
			next, propagated := flowView.Outcomes().Propagation(exit)
			if !propagated {
				break
			}
			exit = next
		}
	}
	identity := p.Source().Identity()
	bodyCount := identity.FamilyCount(keyspace.FamilyBody)
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		if !flowView.Executable().Contains(body) {
			continue
		}
		key := bodyKey{shard: shard, term: body}
		if _, exists := state.bodyIndex[key]; exists {
			return false
		}
		entry, entryOK := flowView.Ports().Entry(body)
		if !entryOK || entry == 0 {
			return false
		}
		row := bodyRow{shard: shard, term: body, function: functionForBody[body], entry: entry}
		if function := row.function; function != 0 {
			formalOrder := p.Source().Formals()
			width, widthOK := formalOrder.Len(function)
			if !widthOK || width < 0 {
				return false
			}
			for position := 0; position < width; position++ {
				cell, cellOK := formalOrder.At(function, position)
				endpoint, endpointOK := state.cellEndpoint(classes, shard, cell)
				if !cellOK || !endpointOK {
					return false
				}
				row.formals = append(row.formals, endpoint)
			}
		}
		port, portOK := newPort(state.owner, uint32(len(state.roots)+1), classes.AnyValue(), true)
		if !portOK {
			return false
		}
		root, port, rootOK := state.addRootWithPortTargets(classes, rootBody, shard, body, linkproject.Application{}, port, row.formals)
		if !rootOK {
			return false
		}
		row.root, row.port = root, port
		normal, normalOK := flowView.Outcomes().BodyExit(body, flowkind.OutcomeNormal)
		if !normalOK || normal == 0 {
			return false
		}
		outcomeIndex, outcomeOK := state.addOutcomeRoot(classes, shard, p, body, normal, flowkind.OutcomeNormal, nil)
		if !outcomeOK || uint64(outcomeIndex) >= uint64(len(state.outcomes)) {
			return false
		}
		outcomeRow := state.outcomes[outcomeIndex]
		if outcomeRow.body != body || outcomeRow.shard != shard || uint64(outcomeRow.root) >= uint64(len(state.roots)) ||
			state.roots[outcomeRow.root].kind != rootOutcome || state.roots[outcomeRow.root].sourceIndex != outcomeIndex {
			return false
		}
		// Store the descriptor coordinate, never the root coordinate. The
		// Outcome accessor owns the sole root projection for this row.
		row.normal = outcomeIndex

		// Flow exposes one direct/aggregate OutcomeReturn per Body. Return
		// occurrences attach to every same-activation descriptor along their
		// ReturnExit/Propagation chain, so branch descriptors and the enclosing
		// aggregate each retain the authored Values alternatives.
		returnTerm := returnOutcomeByBody[body]
		returnValues := returnValuesByOutcome[returnTerm]
		if returnTerm != 0 && len(returnValues) > 0 {
			returnIndex, returnOK := state.addOutcomeRoot(classes, shard, p, body, returnTerm, flowkind.OutcomeReturn, returnValues)
			if !returnOK || uint64(returnIndex) >= uint64(len(state.outcomes)) {
				return false
			}
			returnRow := state.outcomes[returnIndex]
			if returnRow.body != body || returnRow.shard != shard || returnRow.kind != flowkind.OutcomeReturn {
				return false
			}
			row.returnOutcome = returnIndex
			row.hasReturn = true
		}
		state.bodyIndex[key] = uint32(len(state.bodies))
		state.bodies = append(state.bodies, row)
		state.roots[root].sourceIndex = uint32(len(state.bodies) - 1)
	}
	return true
}

// validateBodyOutcomeRoots closes the Body/normal/Outcome association at the
// same seal boundary that issued both descriptors.  bodyRow.normal is a
// descriptor coordinate; only that descriptor may project the Outcome root.
// This is an integrity fence, not another lookup plane or conversion table.
func (state *schema) validateBodyOutcomeRoots() bool {
	if state == nil {
		return false
	}
	for bodyIndex, body := range state.bodies {
		if uint64(body.root) >= uint64(len(state.roots)) || state.roots[body.root].kind != rootBody || state.roots[body.root].sourceIndex != uint32(bodyIndex) || uint64(body.normal) >= uint64(len(state.outcomes)) {
			return false
		}
		outcome := state.outcomes[body.normal]
		if outcome.body != body.term || outcome.shard != body.shard || outcome.kind != flowkind.OutcomeNormal || uint64(outcome.root) >= uint64(len(state.roots)) || state.roots[outcome.root].kind != rootOutcome || state.roots[outcome.root].sourceIndex != body.normal {
			return false
		}
		if body.hasReturn {
			if uint64(body.returnOutcome) >= uint64(len(state.outcomes)) {
				return false
			}
			returnOutcome := state.outcomes[body.returnOutcome]
			if returnOutcome.body != body.term || returnOutcome.shard != body.shard || returnOutcome.kind != flowkind.OutcomeReturn || uint64(returnOutcome.root) >= uint64(len(state.roots)) || state.roots[returnOutcome.root].kind != rootOutcome || state.roots[returnOutcome.root].sourceIndex != body.returnOutcome {
				return false
			}
		} else if body.returnOutcome != 0 {
			return false
		}
	}
	for outcomeIndex, outcome := range state.outcomes {
		if outcome.kind != flowkind.OutcomeNormal && outcome.kind != flowkind.OutcomeReturn || uint64(outcome.root) >= uint64(len(state.roots)) || state.roots[outcome.root].kind != rootOutcome || state.roots[outcome.root].sourceIndex != uint32(outcomeIndex) {
			return false
		}
	}
	return true
}

func (state *schema) addOutcomeRoot(classes *static.ClassSet, shard linkproject.Shard, p *program.Program, body, outcome keyspace.Term, outcomeKind flowkind.OutcomeKind, returnValues []keyspace.Term) (uint32, bool) {
	if state == nil || classes == nil || p == nil || body == 0 || outcome == 0 {
		return 0, false
	}
	if outcomeKind != flowkind.OutcomeNormal && outcomeKind != flowkind.OutcomeReturn {
		return 0, false
	}
	key := outcomeKey{shard: shard, term: outcome}
	if _, exists := state.outcomeIndex[key]; exists {
		return 0, false
	}
	flowView := p.Flow()
	port, portOK := newPort(state.owner, uint32(len(state.roots)+1), classes.AnyValue(), false)
	if !portOK {
		return 0, false
	}
	root, port, rootOK := state.addRootWithPort(classes, rootOutcome, shard, outcome, linkproject.Application{}, port)
	if !rootOK {
		return 0, false
	}
	outcomeInfo, outcomeInfoOK := flowView.Outcomes().Get(outcome)
	if !outcomeInfoOK || outcomeInfo.Body != body || outcomeInfo.Kind != outcomeKind {
		return 0, false
	}
	values := append([]keyspace.Term(nil), returnValues...)
	if outcomeKind == flowkind.OutcomeReturn {
		if len(values) == 0 {
			return 0, false
		}
		outcomeActivation, outcomeActivationOK := flowView.Activation().For(body)
		if !outcomeActivationOK {
			return 0, false
		}
		for _, candidate := range values {
			valuesOwner, _, valuesOK := flowView.Authored().Values().Get(candidate)
			valuesActivation, valuesActivationOK := flowView.Activation().For(valuesOwner)
			if candidate == 0 || !valuesOK || !valuesActivationOK || valuesActivation != outcomeActivation || !flowView.Executable().Contains(candidate) {
				return 0, false
			}
		}
	} else if len(values) != 0 {
		return 0, false
	}
	// Normal completion has no authored Values. A return descriptor can have
	// several authored alternatives, so its finish port is only exposed when
	// there is one unambiguous Values expression.
	finish := keyspace.Term(0)
	if len(values) == 1 {
		var finishOK bool
		finish, finishOK = flowView.Ports().Finish(values[0])
		if !finishOK {
			return 0, false
		}
	}
	row := outcomeRow{root: root, shard: shard, term: outcome, body: body, kind: outcomeKind, values: values, port: port, finish: finish}
	state.outcomeIndex[key] = uint32(len(state.outcomes))
	state.outcomes = append(state.outcomes, row)
	state.roots[root].sourceIndex = uint32(len(state.outcomes) - 1)
	return uint32(len(state.outcomes) - 1), true
}

func programOccurrenceID(linkID keyspace.ContentID, shardOrdinal uint32, term keyspace.Term, kind rootKind) keyspace.ContentID {
	if !linkID.Available() || shardOrdinal == 0 || term == 0 || kind == rootInvalid {
		return keyspace.ContentID{}
	}
	var payload [41]byte
	copy(payload[:32], linkID[:])
	payload[32] = byte(kind)
	binary.BigEndian.PutUint32(payload[33:37], shardOrdinal)
	binary.BigEndian.PutUint32(payload[37:], uint32(term))
	return sha256.Sum256(payload[:])
}

func rootedOccurrenceID(kind rootKind, occurrence keyspace.ContentID) keyspace.ContentID {
	if !occurrence.Available() || kind == rootInvalid {
		return keyspace.ContentID{}
	}
	var payload [33]byte
	copy(payload[:32], occurrence[:])
	payload[32] = byte(kind)
	return sha256.Sum256(payload[:])
}

func (schema *Schema) RootCount() int {
	if schema == nil || schema.state == nil {
		return 0
	}
	return len(schema.state.roots)
}
func (schema *Schema) RootAt(index int) (Root, bool) {
	if schema == nil || schema.state == nil || index < 0 || index >= len(schema.state.roots) {
		return Root{}, false
	}
	root := Root{schema.state, uint32(index)}
	return root, root.valid()
}

// RootOrder returns the sealed Factor-coordinate order of one Pack root.
// Summary reads canonicalize their ClosedRefs by this order; a transformation
// which also has authored order can therefore carry only this small Pack
// permutation, never an engine coordinate or copied source row.
func (schema *Schema) RootOrder(root Root) (int, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return 0, false
	}
	return int(root.index), true
}

// RootID is the canonical cold identity for a Pack source/root descriptor.
func (schema *Schema) RootID(root Root) (keyspace.ContentID, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return keyspace.ContentID{}, false
	}
	id := schema.state.roots[root.index].id
	return id, id.Available()
}

func (schema *Schema) Values(shard linkproject.Shard, term keyspace.Term) (Values, Root, bool) {
	if schema == nil || schema.state == nil {
		return Values{}, Root{}, false
	}
	index, ok := schema.state.valueIndex[programValuesKey{shard: shard, term: term}]
	if !ok {
		return Values{}, Root{}, false
	}
	values := Values{schema.state, index}
	root := Root{schema.state, schema.state.values[index].root}
	return values, root, values.valid() && root.valid()
}
func (schema *Schema) Port(values Values) (Port, bool) {
	if schema == nil || schema.state == nil || !values.valid() || values.schema != schema.state {
		return Port{}, false
	}
	port := schema.state.values[values.index].port
	return port, port.valid()
}

// Endpoint returns Pack's opaque scalar subject for one exact Boundary value.
// It is the only construction path that preserves a Value allocation identity
// in the Pack carrier.
func (schema *Schema) Endpoint(value linkboundary.Value) (Endpoint, bool) {
	if schema == nil || schema.state == nil {
		return Endpoint{}, false
	}
	endpoint, ok := schema.state.endpointIndex[value]
	return endpoint, ok && endpoint.valid()
}

// EndpointTag exposes the owner-issued selector identity for an exact Pack
// endpoint.  It is meaningful only to this Schema and never decodes Link.
func (schema *Schema) EndpointTag(endpoint Endpoint) (uint64, bool) {
	if schema == nil || schema.state == nil || !endpoint.valid() || endpoint.owner != schema.state.owner {
		return 0, false
	}
	if endpoint.index == 0 || uint64(endpoint.index) > uint64(len(schema.state.endpoints)) {
		return 0, false
	}
	return uint64(endpoint.index), true
}

// ScalarSource projects an exact Pack endpoint back to its existing Boundary
// value.  Head and class-only scalars deliberately have no fabricated source.
func (schema *Schema) ScalarSource(scalar Scalar) (linkboundary.Value, bool) {
	if schema == nil || schema.state == nil || !scalar.valid() || scalar.owner != schema.state.owner {
		return linkboundary.Value{}, false
	}
	endpoint, ok := scalar.Endpoint()
	if !ok || uint64(endpoint.index) > uint64(len(schema.state.endpoints)) {
		return linkboundary.Value{}, false
	}
	value := schema.state.endpoints[endpoint.index-1]
	_, valid := schema.state.endpointIndex[value]
	return value, valid
}

// ScalarSourceRoute is the exact endpoint projection used by staged Value
// selectors.  The tag is Pack's schema-issued endpoint identity, not a Boundary
// ordinal or an application/formal coordinate.
func (schema *Schema) ScalarSourceRoute(scalar Scalar) (linkboundary.Value, uint64, bool) {
	if schema == nil || schema.state == nil {
		return linkboundary.Value{}, 0, false
	}
	value, ok := schema.ScalarSource(scalar)
	endpoint, endpointOK := scalar.Endpoint()
	if !ok || !endpointOK || endpoint.owner != schema.state.owner {
		return linkboundary.Value{}, 0, false
	}
	return value, uint64(endpoint.index), endpoint.index != 0
}

func (schema *Schema) relation(root Root) (*relation, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return nil, false
	}
	relation := schema.state.relations[root.index]
	return relation, relation != nil && relation.valid()
}

// RootForValue recovers the exact occurrence root carried by one non-extreme
// Pack value in O(1).  Bottom and Top intentionally name no occurrence.
func (schema *Schema) RootForValue(value Value) (Root, bool) {
	if schema == nil || schema.state == nil || !value.valid() || value.owner != schema.state.owner || value.relation == nil || value.bottom || value.top {
		return Root{}, false
	}
	index, ok := schema.state.relationIndex[value.relation]
	if !ok || uint64(index) >= uint64(len(schema.state.roots)) {
		return Root{}, false
	}
	root := Root{schema.state, index}
	return root, root.valid() && schema.state.relations[index] == value.relation
}
func (schema *Schema) Admit(root Root, value Value) bool {
	relation, ok := schema.relation(root)
	return ok && value.valid() && value.owner == schema.state.owner && (value.IsBottom() || value.IsTop() || value.relation == relation)
}
func (schema *Schema) Bottom() Value {
	if schema == nil || schema.state == nil {
		return Value{}
	}
	return bottomValue(schema.state.owner)
}
func (schema *Schema) Top() Value {
	if schema == nil || schema.state == nil {
		return Value{}
	}
	return topValue(schema.state.owner)
}
func (schema *Schema) Lattice() lattice.Lattice[Value] {
	owner := schema.state.owner
	return lattice.Lattice[Value]{
		Bottom: func() Value { return bottomValue(owner) }, Top: func() Value { return topValue(owner) },
		Equal: equalValue, Same: sameValueRepresentation, LessOrEq: lessOrEqualValue,
		Join: func(left, right Value) Value {
			value, ok := joinValue(left, right)
			if !ok {
				return topValue(owner)
			}
			return value
		},
		Widen: func(left, right Value) Value {
			value, ok := widenValue(left, right)
			if !ok {
				return topValue(owner)
			}
			return value
		},
	}
}
func (schema *Schema) Fingerprint(value Value) uint64 {
	if schema == nil || schema.state == nil || !value.valid() || value.owner != schema.state.owner {
		return 0
	}
	return value.hash
}
func (schema *Schema) At(root Root, value Value, component int) uint64 {
	if !schema.Admit(root, value) || component < 0 || component >= len(value.rank) {
		return 0
	}
	return value.rank[component]
}

// Source is the sealed authored construction descriptor for exactly one
// source-producing Root.  Tail roots intentionally have no Source: a later
// Pack producer Rule owns Call/Vararg result construction.
type Source struct {
	schema *schema
	root   Root
}
type SourceItem struct {
	schema *schema
	root   Root
}

func (schema *Schema) Source(root Root) (Source, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return Source{}, false
	}
	kind := schema.state.roots[root.index].kind
	if kind != rootValues && kind != rootCall {
		return Source{}, false
	}
	return Source{schema: schema.state, root: root}, true
}
func (source Source) valid() bool {
	return source.schema != nil && source.root.valid() && source.root.schema == source.schema && (source.schema.roots[source.root.index].kind == rootValues || source.schema.roots[source.root.index].kind == rootCall)
}
func (source Source) Root() (Root, bool) { return source.root, source.valid() }
func (source Source) ContentID() (keyspace.ContentID, bool) {
	if !source.valid() {
		return keyspace.ContentID{}, false
	}
	id := source.schema.roots[source.root.index].id
	return id, id.Available()
}

// Anchor returns the authored occurrence at which this source is evaluated.
// Values sources use their own Values term.  A call source stores its actual
// Values term for Pack construction, but its causal anchor is the Call
// application itself, so Pack resolves that private representation here.
func (source Source) Anchor() (linkproject.Shard, keyspace.Term, bool) {
	if !source.valid() || source.schema.source == nil {
		return linkproject.Shard{}, 0, false
	}
	row := source.schema.roots[source.root.index]
	switch row.kind {
	case rootValues:
		return row.shard, row.term, true
	case rootCall:
		return source.schema.source.Project().Applications().Call(row.application)
	default:
		return linkproject.Shard{}, 0, false
	}
}
func (source Source) Count() int {
	if !source.valid() {
		return 0
	}
	return 1
}
func (source Source) At(index int) (SourceItem, bool) {
	if !source.valid() || index != 0 {
		return SourceItem{}, false
	}
	return SourceItem{schema: source.schema, root: source.root}, true
}
func (item SourceItem) valid() bool {
	return item.schema != nil && item.root.valid() && item.root.schema == item.schema && (item.schema.roots[item.root.index].kind == rootValues || item.schema.roots[item.root.index].kind == rootCall)
}
func (item SourceItem) row() ([]Endpoint, Port, Port, bool) {
	if !item.valid() {
		return nil, Port{}, Port{}, false
	}
	root := item.schema.roots[item.root.index]
	switch root.kind {
	case rootValues:
		if uint64(root.sourceIndex) >= uint64(len(item.schema.values)) {
			return nil, Port{}, Port{}, false
		}
		row := item.schema.values[root.sourceIndex]
		if row.root == item.root.index {
			return row.fixed, row.port, row.tail, true
		}
	case rootCall:
		if uint64(root.sourceIndex) >= uint64(len(item.schema.calls)) {
			return nil, Port{}, Port{}, false
		}
		row := item.schema.calls[root.sourceIndex]
		if row.root == item.root.index {
			return row.fixed, row.port, row.tail, true
		}
	}
	return nil, Port{}, Port{}, false
}
func (item SourceItem) Port() (Port, bool) { _, port, _, ok := item.row(); return port, ok }
func (item SourceItem) FixedCount() int {
	fixed, _, _, ok := item.row()
	if !ok {
		return 0
	}
	return len(fixed)
}
func (item SourceItem) FixedAt(index int) (Endpoint, bool) {
	fixed, _, _, ok := item.row()
	if !ok || index < 0 || index >= len(fixed) {
		return Endpoint{}, false
	}
	return fixed[index], true
}
func (item SourceItem) Tail() (Port, Offset, bool) {
	_, _, tail, ok := item.row()
	if !ok || !tail.valid() {
		return Port{}, Offset{}, false
	}
	offset, offsetOK := zeroOffset(item.schema.owner)
	return tail, offset, offsetOK
}

// RootKind reports the cold occurrence family which issued root.  It is a
// diagnostic selector only; callers still need the typed descriptor accessors
// below before constructing a Rule operand.
type RootKind uint8

const (
	RootInvalid RootKind = iota
	RootValues
	RootCall
	RootTail
	RootBind
	RootBody
	RootOutcome
)

func (schema *Schema) RootKind(root Root) (RootKind, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return RootInvalid, false
	}
	switch schema.state.roots[root.index].kind {
	case rootValues:
		return RootValues, true
	case rootCall:
		return RootCall, true
	case rootTail:
		return RootTail, true
	case rootBind:
		return RootBind, true
	case rootBody:
		return RootBody, true
	case rootOutcome:
		return RootOutcome, true
	default:
		return RootInvalid, false
	}
}

// PackOnly reports whether a root's complete relation consists of exactly one
// whole-Pack target.  Scalar-adjustment and splice rules require this shape;
// Bind/body-entry rules intentionally use the richer scalar-plus-Pack shape.
func (schema *Schema) PackOnly(root Root) bool {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return false
	}
	relation, ok := schema.relation(root)
	return ok && len(relation.targets) == 1 && relation.targets[0].kind == EquationPack
}

// Cell returns the Pack scalar endpoint for one existing storage Cell.  Cell
// ownership is still Program/Value-owned; this endpoint is only the Pack
// expression subject used by Bind and body formal boundary Rules.
func (schema *Schema) Cell(shard linkproject.Shard, cell keyspace.Term) (Endpoint, bool) {
	if schema == nil || schema.state == nil {
		return Endpoint{}, false
	}
	endpoint, ok := schema.state.cellIndex[cellKey{shard: shard, term: cell}]
	return endpoint, ok && endpoint.valid() && endpoint.owner == schema.state.owner
}

// CellTag is the local opaque selector associated with a Cell endpoint. It is
// not a Source/Link ordinal and cannot be used to mint another endpoint.
func (schema *Schema) CellTag(endpoint Endpoint) (uint64, bool) {
	if schema == nil || schema.state == nil || !endpoint.valid() || endpoint.owner != schema.state.owner {
		return 0, false
	}
	for _, candidate := range schema.state.cellIndex {
		if candidate == endpoint {
			return uint64(endpoint.index), true
		}
	}
	return 0, false
}

// Bind is one executable fixed Cell-binding occurrence. It carries no Value
// fact and no copied authored row; all source order is reissued from the
// sealed descriptor.
type Bind struct {
	schema *schema
	index  uint32
}

func (bind Bind) valid() bool {
	return bind.schema != nil && uint64(bind.index) < uint64(len(bind.schema.binds)) && bind.schema.binds[bind.index].port.valid()
}
func (schema *Schema) Bind(shard linkproject.Shard, term keyspace.Term) (Bind, bool) {
	if schema == nil || schema.state == nil {
		return Bind{}, false
	}
	index, ok := schema.state.bindIndex[programValuesKey{shard: shard, term: term}]
	bind := Bind{schema: schema.state, index: index}
	return bind, ok && bind.valid()
}
func (schema *Schema) BindRoot(shard linkproject.Shard, term keyspace.Term) (Root, bool) {
	bind, ok := schema.Bind(shard, term)
	if !ok {
		return Root{}, false
	}
	return bind.Root()
}
func (bind Bind) Root() (Root, bool) {
	if !bind.valid() {
		return Root{}, false
	}
	root := Root{schema: bind.schema, index: bind.schema.binds[bind.index].root}
	return root, root.valid()
}
func (bind Bind) Body() (keyspace.Term, bool) {
	if !bind.valid() {
		return 0, false
	}
	return bind.schema.binds[bind.index].body, true
}
func (bind Bind) Term() (keyspace.Term, bool) {
	if !bind.valid() {
		return 0, false
	}
	return bind.schema.binds[bind.index].term, true
}
func (bind Bind) Input() (Values, bool) {
	if !bind.valid() {
		return Values{}, false
	}
	values := bind.schema.binds[bind.index].values
	return values, values.valid()
}
func (bind Bind) InputRoot() (Root, bool) {
	if !bind.valid() {
		return Root{}, false
	}
	values := bind.schema.binds[bind.index].values
	return Root{schema: bind.schema, index: values.schema.values[values.index].root}, true
}
func (bind Bind) Port() (Port, bool) {
	if !bind.valid() {
		return Port{}, false
	}
	port := bind.schema.binds[bind.index].port
	return port, port.valid()
}
func (bind Bind) CellCount() int {
	if !bind.valid() {
		return 0
	}
	return len(bind.schema.binds[bind.index].cells)
}
func (bind Bind) CellAt(index int) (Endpoint, bool) {
	if !bind.valid() || index < 0 || index >= len(bind.schema.binds[bind.index].cells) {
		return Endpoint{}, false
	}
	endpoint := bind.schema.binds[bind.index].cells[index]
	return endpoint, endpoint.valid()
}

// Body is one Call-owned executable function Body projected into Pack's
// boundary schema. The capability itself is issued by Call; this descriptor
// merely adds the exact Pack entry/normal-exit ports.
type Body struct {
	schema *schema
	index  uint32
}

func (body Body) valid() bool {
	if body.schema == nil || uint64(body.index) >= uint64(len(body.schema.bodies)) || !body.schema.bodies[body.index].port.valid() {
		return false
	}
	row := body.schema.bodies[body.index]
	if uint64(row.root) >= uint64(len(body.schema.roots)) || body.schema.roots[row.root].kind != rootBody || body.schema.roots[row.root].sourceIndex != body.index || row.normal >= uint32(len(body.schema.outcomes)) {
		return false
	}
	outcome := body.schema.outcomes[row.normal]
	if outcome.body != row.term || outcome.shard != row.shard || outcome.kind != flowkind.OutcomeNormal || !outcome.port.valid() || uint64(outcome.root) >= uint64(len(body.schema.roots)) ||
		body.schema.roots[outcome.root].kind != rootOutcome || body.schema.roots[outcome.root].sourceIndex != row.normal {
		return false
	}
	if row.hasReturn {
		if row.returnOutcome >= uint32(len(body.schema.outcomes)) {
			return false
		}
		returnOutcome := body.schema.outcomes[row.returnOutcome]
		if returnOutcome.body != row.term || returnOutcome.shard != row.shard || returnOutcome.kind != flowkind.OutcomeReturn || !returnOutcome.port.valid() || uint64(returnOutcome.root) >= uint64(len(body.schema.roots)) ||
			body.schema.roots[returnOutcome.root].kind != rootOutcome || body.schema.roots[returnOutcome.root].sourceIndex != row.returnOutcome {
			return false
		}
	} else if row.returnOutcome != 0 {
		return false
	}
	return outcome.body == row.term && outcome.shard == row.shard && outcome.port.valid() && uint64(outcome.root) < uint64(len(body.schema.roots)) &&
		body.schema.roots[outcome.root].kind == rootOutcome && body.schema.roots[outcome.root].sourceIndex == row.normal
}
func (schema *Schema) Body(shard linkproject.Shard, term keyspace.Term) (Body, bool) {
	if schema == nil || schema.state == nil {
		return Body{}, false
	}
	index, ok := schema.state.bodyIndex[bodyKey{shard: shard, term: term}]
	body := Body{schema: schema.state, index: index}
	return body, ok && body.valid()
}
func (schema *Schema) BodyRoot(shard linkproject.Shard, term keyspace.Term) (Root, bool) {
	body, ok := schema.Body(shard, term)
	if !ok {
		return Root{}, false
	}
	return body.Root()
}

// Outcome is one Body Flow Outcome projected into a Pack port. Outcome
// values remain Flow-owned; Pack only carries their whole-Pack expressions.
type Outcome struct {
	schema *schema
	index  uint32
}

func (outcome Outcome) valid() bool {
	if outcome.schema == nil || uint64(outcome.index) >= uint64(len(outcome.schema.outcomes)) || !outcome.schema.outcomes[outcome.index].port.valid() {
		return false
	}
	row := outcome.schema.outcomes[outcome.index]
	if row.kind != flowkind.OutcomeNormal && row.kind != flowkind.OutcomeReturn || uint64(row.root) >= uint64(len(outcome.schema.roots)) || outcome.schema.roots[row.root].kind != rootOutcome || outcome.schema.roots[row.root].sourceIndex != outcome.index {
		return false
	}
	for _, values := range row.values {
		if values == 0 {
			return false
		}
	}
	return true
}
func (schema *Schema) Outcome(shard linkproject.Shard, term keyspace.Term) (Outcome, bool) {
	if schema == nil || schema.state == nil {
		return Outcome{}, false
	}
	index, ok := schema.state.outcomeIndex[outcomeKey{shard: shard, term: term}]
	outcome := Outcome{schema: schema.state, index: index}
	return outcome, ok && outcome.valid()
}
func (schema *Schema) OutcomeRoot(shard linkproject.Shard, term keyspace.Term) (Root, bool) {
	outcome, ok := schema.Outcome(shard, term)
	if !ok {
		return Root{}, false
	}
	return outcome.Root()
}
func (outcome Outcome) Root() (Root, bool) {
	if !outcome.valid() {
		return Root{}, false
	}
	root := Root{schema: outcome.schema, index: outcome.schema.outcomes[outcome.index].root}
	return root, root.valid() && outcome.schema.roots[root.index].sourceIndex == outcome.index
}
func (outcome Outcome) Body() (keyspace.Term, bool) {
	if !outcome.valid() {
		return 0, false
	}
	return outcome.schema.outcomes[outcome.index].body, true
}
func (outcome Outcome) Kind() flowkind.OutcomeKind {
	if !outcome.valid() {
		return 0
	}
	return outcome.schema.outcomes[outcome.index].kind
}
func (outcome Outcome) Term() (keyspace.Term, bool) {
	if !outcome.valid() {
		return 0, false
	}
	return outcome.schema.outcomes[outcome.index].term, true
}
func (outcome Outcome) Values() (Values, bool) {
	if !outcome.valid() {
		return Values{}, false
	}
	row := outcome.schema.outcomes[outcome.index]
	if len(row.values) != 1 || row.values[0] == 0 {
		return Values{}, false
	}
	values, _, ok := (&Schema{state: outcome.schema}).Values(row.shard, row.values[0])
	return values, ok
}
func (outcome Outcome) ValuesCount() int {
	if !outcome.valid() {
		return 0
	}
	return len(outcome.schema.outcomes[outcome.index].values)
}
func (outcome Outcome) ValuesTermAt(index int) (keyspace.Term, bool) {
	if !outcome.valid() || index < 0 || index >= len(outcome.schema.outcomes[outcome.index].values) {
		return 0, false
	}
	return outcome.schema.outcomes[outcome.index].values[index], true
}
func (outcome Outcome) ValuesRootAt(index int) (Root, bool) {
	term, termOK := outcome.ValuesTermAt(index)
	if !termOK {
		return Root{}, false
	}
	row := outcome.schema.outcomes[outcome.index]
	valueIndex, valueOK := outcome.schema.valueIndex[programValuesKey{shard: row.shard, term: term}]
	if !valueOK || uint64(valueIndex) >= uint64(len(outcome.schema.values)) {
		return Root{}, false
	}
	valueRow := outcome.schema.values[valueIndex]
	if valueRow.shard != row.shard || valueRow.term != term || uint64(valueRow.root) >= uint64(len(outcome.schema.roots)) || outcome.schema.roots[valueRow.root].kind != rootValues || outcome.schema.roots[valueRow.root].sourceIndex != valueIndex {
		return Root{}, false
	}
	root := Root{schema: outcome.schema, index: valueRow.root}
	return root, root.valid()
}
func (outcome Outcome) Port() (Port, bool) {
	if !outcome.valid() {
		return Port{}, false
	}
	port := outcome.schema.outcomes[outcome.index].port
	return port, port.valid()
}
func (outcome Outcome) Finish() (keyspace.Term, bool) {
	if !outcome.valid() {
		return 0, false
	}
	finish := outcome.schema.outcomes[outcome.index].finish
	return finish, finish != 0
}

func (body Body) Root() (Root, bool) {
	if !body.valid() {
		return Root{}, false
	}
	root := Root{schema: body.schema, index: body.schema.bodies[body.index].root}
	return root, root.valid()
}
func (body Body) Port() (Port, bool) {
	if !body.valid() {
		return Port{}, false
	}
	port := body.schema.bodies[body.index].port
	return port, port.valid()
}
func (body Body) Entry() (keyspace.Term, bool) {
	if !body.valid() {
		return 0, false
	}
	return body.schema.bodies[body.index].entry, true
}
func (body Body) Term() (keyspace.Term, bool) {
	if !body.valid() {
		return 0, false
	}
	return body.schema.bodies[body.index].term, true
}
func (body Body) Function() (keyspace.Term, bool) {
	if !body.valid() || body.schema.bodies[body.index].function == 0 {
		return 0, false
	}
	return body.schema.bodies[body.index].function, true
}
func (body Body) FormalCount() int {
	if !body.valid() {
		return 0
	}
	return len(body.schema.bodies[body.index].formals)
}
func (body Body) FormalAt(index int) (Endpoint, bool) {
	if !body.valid() || index < 0 || index >= len(body.schema.bodies[body.index].formals) {
		return Endpoint{}, false
	}
	endpoint := body.schema.bodies[body.index].formals[index]
	return endpoint, endpoint.valid()
}
func (body Body) Normal() (Outcome, bool) {
	if !body.valid() {
		return Outcome{}, false
	}
	outcome := Outcome{schema: body.schema, index: body.schema.bodies[body.index].normal}
	return outcome, outcome.valid() && outcome.schema.outcomes[outcome.index].body == body.schema.bodies[body.index].term && outcome.schema.outcomes[outcome.index].shard == body.schema.bodies[body.index].shard
}
func (body Body) Return() (Outcome, bool) {
	if !body.valid() || !body.schema.bodies[body.index].hasReturn {
		return Outcome{}, false
	}
	outcome := Outcome{schema: body.schema, index: body.schema.bodies[body.index].returnOutcome}
	return outcome, outcome.valid() && outcome.Kind() == flowkind.OutcomeReturn && outcome.schema.outcomes[outcome.index].body == body.schema.bodies[body.index].term && outcome.schema.outcomes[outcome.index].shard == body.schema.bodies[body.index].shard
}

// Same is an owner-fenced capability comparison. Invalid or foreign
// descriptors are never considered equal merely because their coordinates
// happen to match.
func (body Body) Same(other Body) bool {
	return body.valid() && other.valid() && body.schema == other.schema && body.index == other.index
}
func (outcome Outcome) Same(other Outcome) bool {
	return outcome.valid() && other.valid() && outcome.schema == other.schema && outcome.index == other.index
}

// CallRoot returns Pack's exact root for one existing Link Call Application.
// It is a Pack coordinate lookup, not a second call-boundary identity.
func (schema *Schema) CallRoot(application linkproject.Application) (Root, bool) {
	if schema == nil || schema.state == nil {
		return Root{}, false
	}
	index, ok := schema.state.callIndex[application]
	if !ok || uint64(index) >= uint64(len(schema.state.calls)) {
		return Root{}, false
	}
	row := schema.state.calls[index]
	if row.application != application || uint64(row.root) >= uint64(len(schema.state.roots)) || schema.state.roots[row.root].kind != rootCall {
		return Root{}, false
	}
	root := Root{schema: schema.state, index: row.root}
	return root, root.valid()
}

// TailRoot returns the exact Pack producer root for one executable Call or
// Vararg term.  Producer identity stays in Flow; this is only the sealed Pack
// relation endpoint used by a producer/return Rule.
func (schema *Schema) TailRoot(shard linkproject.Shard, term keyspace.Term) (Root, bool) {
	if schema == nil || schema.state == nil {
		return Root{}, false
	}
	index, ok := schema.state.tailIndex[programValuesKey{shard: shard, term: term}]
	if !ok || uint64(index) >= uint64(len(schema.state.tails)) {
		return Root{}, false
	}
	row := schema.state.tails[index]
	if uint64(row.root) >= uint64(len(schema.state.roots)) || schema.state.roots[row.root].kind != rootTail {
		return Root{}, false
	}
	root := Root{schema: schema.state, index: row.root}
	return root, root.valid()
}

type inputSelectionKind uint8

const (
	inputSelectionInvalid inputSelectionKind = iota
	inputSelectionScalar
	inputSelectionTail
	inputSelectionWhole
)

// InputSelector is one cold, Pack-owned interpretation template for a Target
// input source. It is independent of any Application: the same
// sealed template is applied to each selected call later.  Scalar selectors
// retain their already-sealed TableIndex, so solving never searches or interns
// an offset from a Target formal ordinal.
type InputSelector struct {
	schema *schema
	kind   inputSelectionKind
	table  TableIndex
	start  int
	sealed bool
}

func (selector InputSelector) valid() bool {
	if !selector.sealed || selector.schema == nil || selector.start < 0 {
		return false
	}
	switch selector.kind {
	case inputSelectionScalar:
		return selector.table.valid() && selector.table.offset.owner == selector.schema.owner &&
			uint64(selector.start) == selector.table.value
	case inputSelectionTail, inputSelectionWhole:
		return !selector.table.sealed
	default:
		return false
	}
}

// InputSelector creates the one reusable cold template for a sealed Target
// input source.  Target validates the source shape here; Link already sealed
// every Call root before Pack construction. Selection of an operation
// for an application remains activation authority, not this template.
func (schema *Schema) InputSelector(operation target.Operation, source target.InputSource) (InputSelector, bool) {
	if schema == nil || schema.state == nil || operation == 0 {
		return InputSelector{}, false
	}
	contract, contractOK := schema.state.source.Boundary().Target()
	if !contractOK || contract == nil {
		return InputSelector{}, false
	}
	values, valuesOK := contract.Input(operation)
	if !valuesOK {
		return InputSelector{}, false
	}
	fixed := contract.ValuesCount(values)
	if fixed < 0 {
		return InputSelector{}, false
	}
	selector := InputSelector{schema: schema.state, start: 0, sealed: true}
	switch source.Kind {
	case target.InputSourceValueFormal:
		if uint64(source.Ordinal) >= uint64(fixed) {
			return InputSelector{}, false
		}
		table, tableOK := schema.TableIndex(int64(source.Ordinal))
		if !tableOK {
			return InputSelector{}, false
		}
		selector.kind, selector.table, selector.start = inputSelectionScalar, table, int(source.Ordinal)
	case target.InputSourceValuesVar:
		tail, variable, tailOK := contract.ValuesTail(values)
		if !tailOK || tail != target.ValuesVariable || variable != target.ValuesVar(source.Ordinal) {
			return InputSelector{}, false
		}
		selector.kind, selector.start = inputSelectionTail, fixed
	case target.InputSourceAllInputs:
		opaque, opaqueOK := contract.Opaque()
		if !opaqueOK || operation != opaque || source.Ordinal != 0 {
			return InputSelector{}, false
		}
		selector.kind = inputSelectionWhole
	default:
		return InputSelector{}, false
	}
	return selector, selector.valid()
}

// TailProducerKind distinguishes the two Program occurrences that produce a
// complete open Values tail.  It is not a type/class fallback: Call requires
// a selected-return Pack transfer and Vararg requires its incoming Pack
// boundary.  Neither is an authored zero-read Source.
type TailProducerKind uint8

const (
	TailProducerInvalid TailProducerKind = iota
	TailProducerCall
	TailProducerVararg
)

// TailProducer is Pack's capability for a Call/Vararg whole-Pack occurrence.
// A producer Rule must bind its Root and Port from this descriptor; callers
// cannot reinterpret a raw Program tail as an arbitrary free Port.
type TailProducer struct {
	schema *schema
	index  uint32
}

func (producer TailProducer) valid() bool {
	return producer.schema != nil && uint64(producer.index) < uint64(len(producer.schema.tails))
}
func (schema *Schema) TailProducer(root Root) (TailProducer, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state || schema.state.roots[root.index].kind != rootTail {
		return TailProducer{}, false
	}
	index := schema.state.roots[root.index].sourceIndex
	producer := TailProducer{schema: schema.state, index: index}
	return producer, producer.valid() && producer.schema.tails[index].root == root.index
}
func (producer TailProducer) Root() (Root, bool) {
	if !producer.valid() {
		return Root{}, false
	}
	root := Root{producer.schema, producer.schema.tails[producer.index].root}
	return root, root.valid()
}
func (producer TailProducer) Port() (Port, bool) {
	if !producer.valid() {
		return Port{}, false
	}
	port := producer.schema.tails[producer.index].port
	return port, port.valid()
}
func (producer TailProducer) ContentID() (keyspace.ContentID, bool) {
	root, rootOK := producer.Root()
	if !rootOK {
		return keyspace.ContentID{}, false
	}
	id := producer.schema.roots[root.index].id
	return id, id.Available()
}
func (producer TailProducer) Kind() TailProducerKind {
	if !producer.valid() {
		return TailProducerInvalid
	}
	return producer.schema.tails[producer.index].kind
}

// Payload is the direct Program-Values marginal consumed by Heap.
type PayloadRequest struct {
	Values Values
	Index  int
}
type Payload struct {
	schema    *schema
	root      Root
	values    Values
	selection ScalarSelection
	sources   []linkboundary.Value
}

func (schema *Schema) Payloads(requests []PayloadRequest) ([]Payload, bool) {
	if schema == nil || schema.state == nil {
		return nil, false
	}
	out := make([]Payload, len(requests))
	for i, request := range requests {
		if request.Index < 0 || !request.Values.valid() || request.Values.schema != schema.state {
			return nil, false
		}
		row := schema.state.values[request.Values.index]
		index, ok := schema.TableIndex(int64(request.Index))
		if !ok {
			return nil, false
		}
		selection := ScalarSelection{schema: schema.state, values: request.Values, kind: scalarSelectionTableIndex, tableIndex: index, sealed: true}
		sources := make([]linkboundary.Value, 0, len(row.fixed))
		for _, endpoint := range row.fixed {
			value, valueOK := schema.ScalarSource(Scalar{owner: endpoint.owner, kind: ScalarEndpoint, endpoint: endpoint, class: endpoint.class, sealed: true})
			if !valueOK {
				return nil, false
			}
			sources = append(sources, value)
		}
		out[i] = Payload{schema: schema.state, root: Root{schema.state, row.root}, values: request.Values, selection: selection, sources: sources}
	}
	return out, true
}
func (payload Payload) Root() (Root, bool) {
	return payload.root, payload.schema != nil && payload.root.valid()
}
func (payload Payload) Selection() (ScalarSelection, bool) {
	return payload.selection, payload.selection.valid()
}
func (payload Payload) SourceCount() int {
	if payload.schema == nil {
		return 0
	}
	return len(payload.sources)
}
func (payload Payload) SourceAt(index int) (linkboundary.Value, bool) {
	if payload.schema == nil || index < 0 || index >= len(payload.sources) {
		return linkboundary.Value{}, false
	}
	return payload.sources[index], true
}
