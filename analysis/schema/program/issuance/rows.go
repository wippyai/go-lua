package issuance

import (
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// Row is an opaque address into one immutable Program issuance row space.
// Space is nominal; Index has meaning only to Rows, which authenticates both.
type Row struct {
	Space schema.Key
	Index int
}

func (row Row) Available() bool { return row.Space.Available() && row.Index >= 0 }

type ScalarKind uint8

const (
	ScalarInvalid ScalarKind = iota
	ScalarBool
	ScalarUint
	ScalarIdentity
)

// Scalar is the closed carrier of a declared Program field. Type preserves
// nominal identity even when two values share the same physical encoding.
type Scalar struct {
	Kind     ScalarKind
	Type     schema.Key
	Bool     bool
	Uint     uint64
	Identity identity.ContentID
}

func Bool(value bool) Scalar { return Scalar{Kind: ScalarBool, Bool: value} }
func Uint(typ schema.Key, value uint64) Scalar {
	return Scalar{Kind: ScalarUint, Type: typ, Uint: value}
}
func Identity(typ schema.Key, value identity.ContentID) Scalar {
	return Scalar{Kind: ScalarIdentity, Type: typ, Identity: value}
}

func (value Scalar) Available() bool {
	switch value.Kind {
	case ScalarBool:
		return !value.Type.Available()
	case ScalarUint:
		return value.Type.Available()
	case ScalarIdentity:
		return value.Type.Available() && value.Identity.Available()
	default:
		return false
	}
}

type occurrenceKey struct {
	kind programschema.OccurrenceKind
	id   identity.ContentID
}

type geometryDraft struct {
	entry  []identity.ContentID
	finish []identity.ContentID
}

type closureProof struct{ occurrence identity.ContentID }

type geometryPoint struct {
	occurrence     identity.ContentID
	occurrenceKind programschema.OccurrenceKind
	kind           uint64
	position       uint64
	point          identity.ContentID
}

type predecessor struct {
	occurrence identity.ContentID
	route      identity.ContentID
	point      identity.ContentID
}

// Builder is the sole mutable owner of construction-only evidence required by
// the issuance machine. It stores no copy of canonical Occurrence, Call, or
// ResultSlot rows; Rows reads those directly from Publication after sealing.
type Builder struct {
	geometry     map[occurrenceKey]geometryDraft
	closures     map[identity.ContentID]closureProof
	predecessors map[identity.ContentID]predecessor
	sealed       bool
}

func NewBuilder() *Builder {
	return &Builder{
		geometry:     make(map[occurrenceKey]geometryDraft),
		closures:     make(map[identity.ContentID]closureProof),
		predecessors: make(map[identity.ContentID]predecessor),
	}
}

func (builder *Builder) AddGeometry(kind programschema.OccurrenceKind, occurrence identity.ContentID, entry, finish []identity.ContentID) bool {
	if builder == nil || builder.sealed || !kind.Valid() || !occurrence.Available() {
		return false
	}
	key := occurrenceKey{kind: kind, id: occurrence}
	if _, duplicate := builder.geometry[key]; duplicate {
		return false
	}
	entryCopy, entryOK := distinctPoints(entry)
	finishCopy, finishOK := distinctPoints(finish)
	if !entryOK || !finishOK {
		return false
	}
	builder.geometry[key] = geometryDraft{entry: entryCopy, finish: finishCopy}
	return true
}

// Geometry returns the exact owner-issued construction geometry while the
// builder is live. Consumers receive copies and cannot mutate the eventual
// sealed row bundle.
func (builder *Builder) Geometry(kind programschema.OccurrenceKind, occurrence identity.ContentID) ([]identity.ContentID, []identity.ContentID, bool) {
	if builder == nil || builder.sealed || !kind.Valid() || !occurrence.Available() {
		return nil, nil, false
	}
	geometry, ok := builder.geometry[occurrenceKey{kind: kind, id: occurrence}]
	return append([]identity.ContentID(nil), geometry.entry...), append([]identity.ContentID(nil), geometry.finish...), ok
}

func distinctPoints(points []identity.ContentID) ([]identity.ContentID, bool) {
	copyOf := append([]identity.ContentID(nil), points...)
	seen := make(map[identity.ContentID]struct{}, len(copyOf))
	for _, point := range copyOf {
		if !point.Available() {
			return nil, false
		}
		if _, duplicate := seen[point]; duplicate {
			return nil, false
		}
		seen[point] = struct{}{}
	}
	return copyOf, true
}

// AddClosureProof admits the already-authenticated conclusion only. The
// allocation/target/body/capture traversal remains with its construction
// owner and is not copied into this generic row vocabulary.
func (builder *Builder) AddClosureProof(occurrence identity.ContentID) bool {
	if builder == nil || builder.sealed || !occurrence.Available() {
		return false
	}
	if _, duplicate := builder.closures[occurrence]; duplicate {
		return false
	}
	builder.closures[occurrence] = closureProof{occurrence: occurrence}
	return true
}

// AddPredecessor admits the environment owner's exact route target.
// No route lookup or finish-membership check occurs in the evaluator.
func (builder *Builder) AddPredecessor(occurrence, route, point identity.ContentID) bool {
	if builder == nil || builder.sealed || !occurrence.Available() || !route.Available() || !point.Available() {
		return false
	}
	if _, duplicate := builder.predecessors[occurrence]; duplicate {
		return false
	}
	builder.predecessors[occurrence] = predecessor{occurrence: occurrence, route: route, point: point}
	return true
}

type relationIndex struct {
	source  schema.Key
	target  schema.Key
	targets [][]Row
}

// Rows is the immutable canonical row bundle consumed by the generic machine.
// Relation joins are indexed exactly once at Seal; Follow never scans a
// canonical Program column or rebuilds an inverse.
type Rows struct {
	publication   *programpublication.Publication
	occurrences   int
	calls         int
	resultSlots   int
	moduleImports int
	subjectSpans  int
	closures      []closureProof
	geometry      []geometryPoint
	predecessors  []predecessor
	relations     map[schema.Key]relationIndex
}

func (builder *Builder) Seal(table schemaissuance.Table, publication *programpublication.Publication) (Rows, bool) {
	if builder == nil || builder.sealed || publication == nil {
		return Rows{}, false
	}
	rows := Rows{
		publication: publication,
		occurrences: len(publication.Occurrences), calls: len(publication.Calls), resultSlots: len(publication.CallResultSlots),
		moduleImports: len(publication.ModuleImports), subjectSpans: len(publication.Lifecycle.SubjectSpans),
		relations: make(map[schema.Key]relationIndex),
	}
	occurrenceIDs := make(map[identity.ContentID]uint32, len(publication.Occurrences))
	consumedGeometry := 0
	for _, occurrence := range publication.Occurrences {
		if !occurrence.Available() {
			return Rows{}, false
		}
		occurrenceIDs[occurrence.ID()]++
		geometry, present := builder.geometry[occurrenceKey{kind: occurrence.Kind(), id: occurrence.ID()}]
		if !present {
			continue
		}
		consumedGeometry++
		for position, point := range geometry.entry {
			rows.geometry = append(rows.geometry, geometryPoint{occurrence: occurrence.ID(), occurrenceKind: occurrence.Kind(), kind: geometryEntry, position: uint64(position), point: point})
		}
		for position, point := range geometry.finish {
			rows.geometry = append(rows.geometry, geometryPoint{occurrence: occurrence.ID(), occurrenceKind: occurrence.Kind(), kind: geometryFinish, position: uint64(position), point: point})
		}
	}
	if consumedGeometry != len(builder.geometry) {
		return Rows{}, false
	}
	for _, proof := range builder.closures {
		if occurrenceIDs[proof.occurrence] != 1 {
			return Rows{}, false
		}
		rows.closures = append(rows.closures, proof)
	}
	for _, proof := range builder.predecessors {
		if occurrenceIDs[proof.occurrence] != 1 {
			return Rows{}, false
		}
		rows.predecessors = append(rows.predecessors, proof)
	}
	identity.SortByContentID(rows.closures, func(row closureProof) identity.ContentID { return row.occurrence })
	identity.SortByContentID(rows.predecessors, func(row predecessor) identity.ContentID { return row.occurrence })
	sort.Slice(rows.geometry, func(left, right int) bool {
		if rows.geometry[left].occurrence != rows.geometry[right].occurrence {
			return contentIDBefore(rows.geometry[left].occurrence, rows.geometry[right].occurrence)
		}
		if rows.geometry[left].kind != rows.geometry[right].kind {
			return rows.geometry[left].kind < rows.geometry[right].kind
		}
		return rows.geometry[left].position < rows.geometry[right].position
	})
	for _, relation := range table.Entries(schemaissuance.KindRelation) {
		index, ok := rows.buildRelation(relation)
		if !ok {
			return Rows{}, false
		}
		rows.relations[relation.Key()] = index
	}
	builder.geometry, builder.closures, builder.predecessors = nil, nil, nil
	builder.sealed = true
	return rows, true
}

func contentIDBefore(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func (rows Rows) Count(space schema.Key) (int, bool) {
	switch space {
	case RowOccurrence:
		return rows.occurrences, true
	case RowCall:
		return rows.calls, true
	case RowCallResultSlot:
		return rows.resultSlots, true
	case RowClosureProof:
		return len(rows.closures), true
	case RowGeometryPoint:
		return len(rows.geometry), true
	case RowPredecessor:
		return len(rows.predecessors), true
	case RowModuleImport:
		return rows.moduleImports, true
	case RowSubjectLivenessSpan:
		return rows.subjectSpans, true
	default:
		return 0, false
	}
}

func (rows Rows) At(space schema.Key, index int) (Row, bool) {
	count, supported := rows.Count(space)
	row := Row{Space: space, Index: index}
	return row, supported && row.Available() && index < count
}

func (rows Rows) Read(row Row, field schema.Key) (Scalar, bool) {
	count, supported := rows.Count(row.Space)
	if !supported || !row.Available() || row.Index >= count || rows.publication == nil {
		return Scalar{}, false
	}
	switch row.Space {
	case RowOccurrence:
		value := rows.publication.Occurrences[row.Index]
		switch field {
		case FieldOccurrenceKind:
			return Uint(TypeOccurrenceKind, uint64(value.Kind())), true
		case FieldOccurrenceID:
			return Identity(TypeContentID, value.ID()), true
		case FieldOccurrenceCode:
			return Uint(TypeOccurrenceCode, value.Code()), true
		case FieldOccurrenceInput0, FieldOccurrenceInput1, FieldOccurrenceInput2:
			position := 0
			switch field {
			case FieldOccurrenceInput1:
				position = 1
			case FieldOccurrenceInput2:
				position = 2
			}
			id, ok := programschema.OccurrenceInputID(value, rows.publication.OccurrenceInputs, position)
			return Identity(TypeContentID, id), ok
		case FieldOccurrenceCallID:
			switch value.Kind() {
			case programschema.OccurrenceCall:
				return Identity(TypeContentID, value.ID()), true
			case programschema.OccurrenceSubjectLiveness:
				id, ok := programschema.OccurrenceInputID(value, rows.publication.OccurrenceInputs, 0)
				return Identity(TypeContentID, id), ok
			}
		}
	case RowCall:
		value := rows.publication.Calls[row.Index]
		switch field {
		case FieldCallID:
			return Identity(TypeContentID, value.ID()), true
		case FieldCallForm:
			return Uint(TypeCallForm, uint64(value.Form())), true
		case FieldCallArgumentCount:
			return Uint(TypeArgumentCount, uint64(value.ArgumentCount())), true
		case FieldCallReceiverPresent:
			_, present := value.ReceiverID()
			return Bool(present), true
		case FieldCallTailPresent:
			_, present := value.TailID()
			return Bool(present), true
		}
	case RowCallResultSlot:
		value := rows.publication.CallResultSlots[row.Index]
		switch field {
		case FieldResultSlotID:
			return Identity(TypeContentID, value.ID()), true
		case FieldResultSlotCallID:
			return Identity(TypeContentID, value.CallID()), true
		case FieldResultSlotOrdinal:
			ordinal, ok := value.Ordinal()
			return Uint(TypeResultSlotOrdinal, uint64(ordinal)), ok
		case FieldResultSlotValueID:
			id, ok := value.ValueID()
			return Identity(TypeContentID, id), ok
		case FieldResultSlotSourceKind:
			return Uint(TypeResultSlotSource, uint64(value.SourceKind())), true
		case FieldResultSlotConsumerKind:
			return Uint(TypeResultSlotConsumer, uint64(value.ConsumerKind())), true
		case FieldResultSlotConsumerID:
			return Identity(TypeContentID, value.ConsumerID()), true
		}
	case RowClosureProof:
		if field == FieldClosureProofOccurrenceID {
			return Identity(TypeContentID, rows.closures[row.Index].occurrence), true
		}
	case RowGeometryPoint:
		value := rows.geometry[row.Index]
		switch field {
		case FieldGeometryOccurrenceID:
			return Identity(TypeContentID, value.occurrence), true
		case FieldGeometryOccurrenceKind:
			return Uint(TypeOccurrenceKind, uint64(value.occurrenceKind)), true
		case FieldGeometryKind:
			return Uint(TypeGeometryKind, value.kind), true
		case FieldGeometryPosition:
			return Uint(TypeGeometryPosition, value.position), true
		case FieldGeometryPointID:
			return Identity(schemaissuance.TypePointIdentity, value.point), true
		}
	case RowPredecessor:
		value := rows.predecessors[row.Index]
		switch field {
		case FieldPredecessorOccurrenceID:
			return Identity(TypeContentID, value.occurrence), true
		case FieldPredecessorRouteID:
			return Identity(schemaissuance.TypeRouteIdentity, value.route), true
		case FieldPredecessorPointID:
			return Identity(schemaissuance.TypePointIdentity, value.point), true
		}
	case RowModuleImport:
		value := rows.publication.ModuleImports[row.Index]
		if field == FieldModuleImportCallID {
			return Identity(TypeContentID, value.CallID()), true
		}
	case RowSubjectLivenessSpan:
		value := rows.publication.Lifecycle.SubjectSpans[row.Index]
		if field == FieldSubjectLivenessSpanID {
			return Identity(TypeContentID, value.ID()), true
		}
	}
	return Scalar{}, false
}

func (rows Rows) Follow(source Row, relation *schemaissuance.Entry) ([]Row, bool) {
	if relation == nil || relation.Kind() != schemaissuance.KindRelation || source.Space != relation.Space() || source.Index < 0 {
		return nil, false
	}
	index, ok := rows.relations[relation.Key()]
	if !ok || index.source != source.Space || source.Index >= len(index.targets) {
		return nil, false
	}
	return append([]Row(nil), index.targets[source.Index]...), true
}

func (rows Rows) buildRelation(relation *schemaissuance.Entry) (relationIndex, bool) {
	if relation == nil || relation.Kind() != schemaissuance.KindRelation {
		return relationIndex{}, false
	}
	joins := relation.Joins()
	if len(joins) == 0 {
		return relationIndex{}, false
	}
	buckets := make(map[string][]Row)
	targetCount, targetOK := rows.Count(relation.Target())
	sourceCount, sourceOK := rows.Count(relation.Space())
	if !targetOK || !sourceOK {
		return relationIndex{}, false
	}
	for index := 0; index < targetCount; index++ {
		target := Row{Space: relation.Target(), Index: index}
		key, ok := rows.joinKey(target, joins, false)
		if ok {
			buckets[key] = append(buckets[key], target)
		}
	}
	result := relationIndex{source: relation.Space(), target: relation.Target(), targets: make([][]Row, sourceCount)}
	for index := range result.targets {
		source := Row{Space: relation.Space(), Index: index}
		key, ok := rows.joinKey(source, joins, true)
		if ok {
			result.targets[index] = append([]Row(nil), buckets[key]...)
		}
	}
	return result, true
}

func (rows Rows) joinKey(row Row, joins []schemaissuance.JoinField, source bool) (string, bool) {
	buffer := make([]byte, 0, len(joins)*42)
	for _, join := range joins {
		field := join.Target
		if source {
			field = join.Source
		}
		value, ok := rows.Read(row, field)
		if !ok || !value.Available() {
			return "", false
		}
		buffer = append(buffer, byte(value.Kind))
		buffer = binary.BigEndian.AppendUint32(buffer, uint32(len(value.Type)))
		buffer = append(buffer, value.Type...)
		switch value.Kind {
		case ScalarBool:
			if value.Bool {
				buffer = append(buffer, 1)
			} else {
				buffer = append(buffer, 0)
			}
		case ScalarUint:
			buffer = binary.BigEndian.AppendUint64(buffer, value.Uint)
		case ScalarIdentity:
			buffer = append(buffer, value.Identity[:]...)
		default:
			return "", false
		}
	}
	return string(buffer), true
}
