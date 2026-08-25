// Package publicationfreeze consumes Effect's sealed FreezeSeal publication
// receipts and projects only exact Recent allocation roots onto Heap. It owns
// no Placement state and does not reinterpret non-freeze publication rows.
package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type sourceTag uint64

type sourceSpec struct {
	tag        sourceTag
	rowID      identity.ContentID
	operation  vocabulary.Operation
	member     int
	coordinate valuedomain.Coordinate
}

type freezeRow struct {
	id          identity.ContentID
	operation   vocabulary.Operation
	subjectOpen bool
	subjectTag  sourceTag
	subjectTags []sourceTag
}

type preparedCall struct {
	id        identity.ContentID
	callKey   calldomain.Key
	callKeyOK bool
	rows      []freezeRow
	sources   sourceBuffer
}

const (
	inlineOperationCapacity = recentplan.InlineWidth
	inlineSourceCapacity    = recentplan.InlineWidth
	inlineFactCapacity      = recentplan.InlineWidth
	inlineIDCapacity        = recentplan.InlineWidth
)

// operationGate is the exact Call projection for one mounted publication
// batch. Opaque/open or non-operation alternatives never authorize a strong
// Heap transition.
type operationGate struct {
	inline      [inlineOperationCapacity]vocabulary.Operation
	count       int
	overflow    []vocabulary.Operation
	opaque      bool
	unsupported bool
}

func (gate operationGate) admits(operation vocabulary.Operation) bool {
	if operation == 0 || gate.count < 0 {
		return false
	}
	inline := gate.count
	if inline > len(gate.inline) {
		inline = len(gate.inline)
	}
	for index := 0; index < inline; index++ {
		if gate.inline[index] == operation {
			return true
		}
	}
	for index := len(gate.inline); index < gate.count; index++ {
		overflow := index - len(gate.inline)
		if overflow < 0 || overflow >= len(gate.overflow) {
			return false
		}
		if gate.overflow[overflow] == operation {
			return true
		}
	}
	return false
}

func (gate operationGate) at(index int) (vocabulary.Operation, bool) {
	if index < 0 || index >= gate.count || gate.count < 0 {
		return 0, false
	}
	if index < len(gate.inline) {
		return gate.inline[index], gate.inline[index] != 0
	}
	overflow := index - len(gate.inline)
	if overflow < 0 || overflow >= len(gate.overflow) {
		return 0, false
	}
	operation := gate.overflow[overflow]
	return operation, operation != 0
}

func (gate *operationGate) add(operation vocabulary.Operation) bool {
	if gate == nil || operation == 0 || gate.count < 0 {
		return false
	}
	if gate.admits(operation) {
		return true
	}
	if gate.count < len(gate.inline) {
		gate.inline[gate.count] = operation
		gate.count++
		return true
	}
	gate.overflow = append(gate.overflow, operation)
	gate.count++
	return true
}

// prepareCall authenticates one published call's FreezeSeal rows out of the
// Effect publication directory and retains only valid FreezeSeal rows. The
// Call key is cached here because module/occurrence provenance is static
// after BindHot; hot selector and fold paths must not repeat that projection.
func prepareCall(values *valuedomain.Schema, calls *calldomain.Algebra, directory effectfactor.PublicationDirectory, call effectfactor.PublicationCallRow) (*preparedCall, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() || !call.Available() {
		return nil, false
	}
	if int(call.RowOffset)+int(call.RowLength) > len(directory.Rows) {
		return nil, false
	}
	_, callKey, callKeyOK := calls.MountedCallKeyForOccurrence(call.Module, call.Call)
	if !callKeyOK || !callKey.Valid() {
		return nil, false
	}
	prepared := &preparedCall{id: call.ID, callKey: callKey, callKeyOK: true}
	var seen contentIDBuffer
	for _, publication := range directory.Rows[call.RowOffset : call.RowOffset+call.RowLength] {
		if !publication.MountedAt(call.Module, call.Call) {
			return nil, false
		}
		if !seen.add(publication.ID) {
			return nil, false
		}

		// The subject and context members of a published row are proven to
		// resolve at admission (domain/effect/publication.Detach); this
		// consumer does not re-derive that proof.
		if publication.Kind != vocabulary.PublicationEffectFreezeSeal || publication.Mutability != vocabulary.PublicationMutabilitySeal {
			continue
		}
		operation := publication.Operation
		if operation == 0 {
			return nil, false
		}
		if int(publication.SubjectOffset)+int(publication.SubjectLength) > len(directory.Members) {
			return nil, false
		}
		row := freezeRow{id: publication.ID, operation: operation, subjectOpen: publication.SubjectOpen}
		for _, subject := range directory.Members[publication.SubjectOffset : publication.SubjectOffset+publication.SubjectLength] {
			member := subject.Row()
			if member.RowID != publication.ID {
				return nil, false
			}
			// The coordinate and the tag are the algebra's, taken from the
			// subject it sealed. Resolving either here would be a second
			// authority over which Value cell is which member.
			coordinate, coordinateOK := subject.Coordinate()
			rawTag, tagOK := subject.Predicate()
			if !coordinateOK || !tagOK {
				return nil, false
			}
			tag := sourceTag(rawTag)
			row.subjectTags = append(row.subjectTags, tag)
			if member.Member == 0 {
				row.subjectTag = tag
			}
			if !prepared.sources.add(sourceSpec{tag: tag, rowID: publication.ID, operation: operation, member: int(member.Member), coordinate: coordinate}) {
				return nil, false
			}
		}
		prepared.rows = append(prepared.rows, row)
	}
	return prepared, true
}

type sourceBuffer struct {
	inline   [inlineSourceCapacity]sourceSpec
	count    int
	overflow []sourceSpec
}

func (sources sourceBuffer) len() int {
	if sources.count < 0 {
		return 0
	}
	return sources.count
}

func (sources sourceBuffer) at(index int) (sourceSpec, bool) {
	if index < 0 || index >= sources.count || sources.count < 0 {
		return sourceSpec{}, false
	}
	if index < len(sources.inline) {
		return sources.inline[index], true
	}
	overflow := index - len(sources.inline)
	if overflow < 0 || overflow >= len(sources.overflow) {
		return sourceSpec{}, false
	}
	return sources.overflow[overflow], true
}

func (sources *sourceBuffer) add(source sourceSpec) bool {
	if sources == nil || sources.count < 0 || source.tag == 0 {
		return false
	}
	for index := 0; index < sources.count; index++ {
		prior, priorOK := sources.at(index)
		if !priorOK {
			return false
		}
		if prior.tag == source.tag {
			return false
		}
	}
	if sources.count < len(sources.inline) {
		sources.inline[sources.count] = source
		sources.count++
		return true
	}
	sources.overflow = append(sources.overflow, source)
	sources.count++
	return true
}

func (sources sourceBuffer) find(tag sourceTag) (sourceSpec, bool) {
	if tag == 0 {
		return sourceSpec{}, false
	}
	for index := 0; index < sources.count; index++ {
		source, sourceOK := sources.at(index)
		if !sourceOK {
			return sourceSpec{}, false
		}
		if source.tag == tag {
			return source, true
		}
	}
	return sourceSpec{}, false
}

func (prepared *preparedCall) sourcesForGate(gate operationGate) sourceBuffer {
	if prepared == nil || gate.opaque || gate.unsupported {
		return sourceBuffer{}
	}
	var sources sourceBuffer
	for index := 0; index < prepared.sources.len(); index++ {
		source, sourceOK := prepared.sources.at(index)
		if !sourceOK {
			return sourceBuffer{}
		}
		if !gate.admits(source.operation) {
			continue
		}
		if !sources.add(source) {
			return sourceBuffer{}
		}
	}
	return sources
}

type contentIDBuffer struct {
	inline   [inlineIDCapacity]identity.ContentID
	count    int
	overflow []identity.ContentID
}

func (ids contentIDBuffer) at(index int) (identity.ContentID, bool) {
	if index < 0 || index >= ids.count || ids.count < 0 {
		return identity.ContentID{}, false
	}
	if index < len(ids.inline) {
		return ids.inline[index], true
	}
	overflow := index - len(ids.inline)
	if overflow < 0 || overflow >= len(ids.overflow) {
		return identity.ContentID{}, false
	}
	return ids.overflow[overflow], true
}

func (ids *contentIDBuffer) add(id identity.ContentID) bool {
	if ids == nil || ids.count < 0 || !id.Available() {
		return false
	}
	for index := 0; index < ids.count; index++ {
		prior, priorOK := ids.at(index)
		if !priorOK {
			return false
		}
		if prior == id {
			return false
		}
	}
	if ids.count < len(ids.inline) {
		ids.inline[ids.count] = id
		ids.count++
		return true
	}
	ids.overflow = append(ids.overflow, id)
	ids.count++
	return true
}

type factEntry struct {
	rowID   identity.ContentID
	value   valuedomain.Value
	present bool
}

type factBuffer struct {
	inline   [inlineFactCapacity]factEntry
	count    int
	overflow []factEntry
}

func (facts factBuffer) at(index int) (factEntry, bool) {
	if index < 0 || index >= facts.count || facts.count < 0 {
		return factEntry{}, false
	}
	if index < len(facts.inline) {
		return facts.inline[index], true
	}
	overflow := index - len(facts.inline)
	if overflow < 0 || overflow >= len(facts.overflow) {
		return factEntry{}, false
	}
	return facts.overflow[overflow], true
}

func (facts *factBuffer) set(entry factEntry) bool {
	if facts == nil || facts.count < 0 || !entry.rowID.Available() {
		return false
	}
	for index := 0; index < facts.count; index++ {
		prior, priorOK := facts.at(index)
		if !priorOK {
			return false
		}
		if prior.rowID == entry.rowID {
			return false
		}
	}
	if facts.count < len(facts.inline) {
		facts.inline[facts.count] = entry
		facts.count++
		return true
	}
	facts.overflow = append(facts.overflow, entry)
	facts.count++
	return true
}

// merge folds the exact fixed members of one heterogeneous freeze subject.
// Freeze planning still requires a complete, present aggregate; a missing
// member remains absent and cannot be replaced by an inferred tail fact.
func (facts *factBuffer) merge(schema *valuedomain.Schema, entry factEntry) bool {
	if facts == nil || schema == nil || !entry.rowID.Available() {
		return false
	}
	for index := 0; index < facts.count; index++ {
		prior, priorOK := facts.at(index)
		if !priorOK {
			return false
		}
		if prior.rowID != entry.rowID {
			continue
		}
		if !entry.present {
			prior.present = false
			return facts.setAt(index, prior)
		}
		if !prior.present {
			return true
		}
		joined, ok := schema.Join(prior.value, entry.value)
		if !ok {
			return false
		}
		prior.value = joined
		return facts.setAt(index, prior)
	}
	return facts.set(entry)
}

func (facts *factBuffer) setAt(index int, entry factEntry) bool {
	if facts == nil || index < 0 || index >= facts.count {
		return false
	}
	if index < len(facts.inline) {
		facts.inline[index] = entry
		return true
	}
	overflow := index - len(facts.inline)
	if overflow < 0 || overflow >= len(facts.overflow) {
		return false
	}
	facts.overflow[overflow] = entry
	return true
}

func (facts factBuffer) get(rowID identity.ContentID) (valuedomain.Value, bool, bool) {
	if !rowID.Available() {
		return valuedomain.Value{}, false, false
	}
	for index := 0; index < facts.count; index++ {
		entry, entryOK := facts.at(index)
		if !entryOK {
			return valuedomain.Value{}, false, false
		}
		if entry.rowID == rowID {
			return entry.value, entry.present, true
		}
	}
	return valuedomain.Value{}, false, false
}

// recentplan is Heap-owned so publication freeze and formal freeze share one
// exact Recent route-set authority without sharing their consumer policies.
type route = recentplan.Route
type routePlan = recentplan.Plan

// exactRecentAllocation accepts only one owner-fenced Value atom carrying one
// Recent root allocation. Open, Top, Summary, scalar, and ambiguous unions do
// not authorize a strong freeze.
func exactRecentAllocation(values *valuedomain.Schema, fact valuedomain.Value, present bool) (heapdomain.Key, bool) {
	return values.ExactRecentAllocation(fact, present)
}

// planFor intersects the exact Recent-root route set justified by each known
// operation alternative. Any open/top Call, unsupported target, missing
// FreezeSeal row, open subject, or non-exact Value fact yields an empty valid
// plan rather than a strong Heap transition.
func planFor(schema heapdomain.Schema, values *valuedomain.Schema, prepared *preparedCall, gate operationGate, facts factBuffer) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema) || prepared == nil {
		return routePlan{}, false
	}
	if gate.opaque || gate.unsupported || gate.count == 0 {
		return routePlan{}, true
	}
	var intersection routePlan
	haveRoutes := false
	for gateIndex := 0; gateIndex < gate.count; gateIndex++ {
		operation, operationOK := gate.at(gateIndex)
		if !operationOK {
			return routePlan{}, false
		}
		var targetRoutes routePlan
		found := false
		for rowIndex := 0; rowIndex < len(prepared.rows); rowIndex++ {
			row := prepared.rows[rowIndex]
			if row.operation != operation {
				continue
			}
			found = true
			if row.subjectOpen {
				return routePlan{}, true
			}
			tags := row.subjectTags
			if len(tags) == 0 && row.subjectTag != 0 {
				tags = []sourceTag{row.subjectTag}
			}
			if len(tags) == 0 {
				// A subject with no mounted semantic source is either proven nil
				// by Lua under-application or statically unknown behind an actual
				// tail. Neither authorizes a strong freeze, and both leave a valid
				// empty plan rather than refusing the batch.
				return routePlan{}, true
			}
			for _, tag := range tags {
				source, sourceOK := prepared.sources.find(tag)
				if !sourceOK || source.operation != operation {
					return routePlan{}, false
				}
			}
			fact, factPresent, factOK := facts.get(row.id)
			candidate, candidateOK := exactRecentAllocation(values, fact, factOK && factPresent)
			if !candidateOK {
				return routePlan{}, true
			}
			tag, tagOK := schema.RouteTag(candidate, materialization.Recent)
			if !tagOK || tag == 0 || !targetRoutes.Add(route{Key: candidate, Tag: tag}) {
				return routePlan{}, false
			}
		}
		if !found || targetRoutes.Count() == 0 {
			return routePlan{}, true
		}
		if !haveRoutes {
			intersection = targetRoutes
			haveRoutes = true
			continue
		}
		var intersectionOK bool
		intersection, intersectionOK = intersection.Intersection(targetRoutes)
		if !intersectionOK {
			return routePlan{}, false
		}
		if intersection.Count() == 0 {
			return routePlan{}, true
		}
	}
	if !haveRoutes || intersection.Count() == 0 {
		return routePlan{}, true
	}
	for index := 0; index < intersection.Count(); index++ {
		candidate, candidateOK := intersection.At(index)
		if !candidateOK || !candidate.Key.Valid() || candidate.Key.Kind() != heapdomain.RootAllocation || !schema.OwnsKey(candidate.Key) || candidate.Tag == 0 {
			return routePlan{}, false
		}
	}
	return intersection, true
}
