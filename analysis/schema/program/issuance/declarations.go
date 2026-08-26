// Package issuance declares the canonical Program row vocabulary consumed by
// the generic issuance machine. It owns only Program construction meaning:
// which immutable row spaces exist and which typed fields the generic machine
// may read. Predicate, join, placement, and stage declarations are added to
// this same contribution; neither composite nor compiler restates these keys.
package issuance

import (
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// Canonical row spaces.
const (
	RowOccurrence     schema.Key = "program-row/occurrence"
	RowCall           schema.Key = "program-row/call"
	RowCallResultSlot schema.Key = "program-row/call-result-slot"
	RowClosureProof   schema.Key = "program-row/closure-capture-proof"
	RowGeometryPoint  schema.Key = "program-row/occurrence-geometry-point"
	RowPredecessor    schema.Key = "program-row/routed-predecessor"
	RowModuleImport   schema.Key = "program-row/module-import"
	// RowSubjectLivenessSpan is the Program owner's published lifecycle
	// judgment, one row per subject and maximal run of one answer over the
	// ordered yield boundary. It is a row space of its own rather than a
	// projection of RowOccurrence because a consumer addressed by an
	// occurrence ordinal would still have to find the span by identity;
	// naming the space directly makes the ordinal the address.
	RowSubjectLivenessSpan schema.Key = "program-row/subject-liveness-span"
)

// Nominal scalar types. Shared identity types are shared deliberately only
// where Program rows carry the same canonical identity namespace.
const (
	TypeContentID          schema.Key = "program-type/content-id"
	TypeOccurrenceKind     schema.Key = "program-type/occurrence-kind"
	TypeOccurrenceCode     schema.Key = "program-type/occurrence-code"
	TypeCallForm           schema.Key = "program-type/call-form"
	TypeArgumentCount      schema.Key = "program-type/argument-count"
	TypeResultSlotOrdinal  schema.Key = "program-type/result-slot-ordinal"
	TypeResultSlotSource   schema.Key = "program-type/result-slot-source"
	TypeResultSlotConsumer schema.Key = "program-type/result-slot-consumer"
	TypeGeometryKind       schema.Key = "program-type/geometry-kind"
	TypeGeometryPosition   schema.Key = "program-type/geometry-position"
)

const (
	InputNone                schema.Key = "program-input/none"
	InputEntryGeometry       schema.Key = "program-input/entry-geometry"
	InputLocalStage          schema.Key = "program-input/local-stage"
	InputPreviousStage       schema.Key = "program-input/previous-stage"
	InputCallDispatchStage   schema.Key = "program-input/call-dispatch-stage"
	InputCallSummaryStage    schema.Key = "program-input/call-summary-stage"
	InputPredecessorGeometry schema.Key = "program-input/predecessor-geometry"
	// InputRouteArrival is the state a routed stage's own route carries into
	// it. A stage that stands on a route reads what the route delivers, not
	// what the stage before it at the same point holds.
	InputRouteArrival schema.Key = "program-input/route-arrival"

	StageBase        schema.Key = "program-stage/base"
	StageLocal       schema.Key = "program-stage/local"
	StageSuccessor   schema.Key = "program-stage/local-successor"
	StageComputation schema.Key = "program-stage/computation"
	StagePredecessor schema.Key = "program-stage/local-predecessor"
	// StageRoutePredecessor carries a fact its route proves. It stands between
	// that route and the point the route lands on, so the point assembles the
	// fact with the rest of what arrives instead of publishing it past its own
	// readers.
	StageRoutePredecessor schema.Key = "program-stage/route-predecessor"
	StageCallDispatch     schema.Key = "program-stage/call-dispatch"
	StageCallSummary      schema.Key = "program-stage/call-summary"
	StageCallEffect       schema.Key = "program-stage/call-effect"

	FormBaseNone           schema.Key = "program-form/base-none"
	FormBaseNoneAllowEmpty schema.Key = "program-form/base-none-allow-empty"
	FormLocalFinish        schema.Key = "program-form/local-finish"
	FormLocalEntry         schema.Key = "program-form/local-entry"
	FormLocalSuccessor     schema.Key = "program-form/local-successor"
	FormComputation        schema.Key = "program-form/computation"
	FormLocalPredecessor   schema.Key = "program-form/local-predecessor"
	FormRoutePredecessor   schema.Key = "program-form/route-predecessor"
	FormCallDispatch       schema.Key = "program-form/call-dispatch"
	FormCallSummary        schema.Key = "program-form/call-summary"
	FormCallEffect         schema.Key = "program-form/call-effect"
)

// Canonical typed fields. Optionality is part of each declaration and is
// therefore interpreted as absence only where Program itself declares it.
const (
	FieldOccurrenceKind   schema.Key = "program-field/occurrence.kind"
	FieldOccurrenceID     schema.Key = "program-field/occurrence.id"
	FieldOccurrenceCode   schema.Key = "program-field/occurrence.code"
	FieldOccurrenceInput0 schema.Key = "program-field/occurrence.input.0"
	FieldOccurrenceInput1 schema.Key = "program-field/occurrence.input.1"
	FieldOccurrenceInput2 schema.Key = "program-field/occurrence.input.2"
	// FieldOccurrenceCallID is the canonical Call foreign key of an executable
	// Call-bearing occurrence. The row projection derives it from the owning
	// occurrence
	// shape; relation declarations never guess which physical input carries it.
	FieldOccurrenceCallID schema.Key = "program-field/occurrence.call-id"

	FieldCallID              schema.Key = "program-field/call.id"
	FieldCallForm            schema.Key = "program-field/call.form"
	FieldCallArgumentCount   schema.Key = "program-field/call.argument-count"
	FieldCallReceiverPresent schema.Key = "program-field/call.receiver-present"
	FieldCallTailPresent     schema.Key = "program-field/call.tail-present"

	FieldResultSlotID           schema.Key = "program-field/call-result-slot.id"
	FieldResultSlotCallID       schema.Key = "program-field/call-result-slot.call-id"
	FieldResultSlotOrdinal      schema.Key = "program-field/call-result-slot.ordinal"
	FieldResultSlotValueID      schema.Key = "program-field/call-result-slot.value-id"
	FieldResultSlotSourceKind   schema.Key = "program-field/call-result-slot.source-kind"
	FieldResultSlotConsumerKind schema.Key = "program-field/call-result-slot.consumer-kind"
	FieldResultSlotConsumerID   schema.Key = "program-field/call-result-slot.consumer-id"

	FieldClosureProofOccurrenceID schema.Key = "program-field/closure-capture-proof.occurrence-id"

	FieldGeometryOccurrenceID   schema.Key = "program-field/occurrence-geometry-point.occurrence-id"
	FieldGeometryOccurrenceKind schema.Key = "program-field/occurrence-geometry-point.occurrence-kind"
	FieldGeometryKind           schema.Key = "program-field/occurrence-geometry-point.kind"
	FieldGeometryPosition       schema.Key = "program-field/occurrence-geometry-point.position"
	FieldGeometryPointID        schema.Key = "program-field/occurrence-geometry-point.point-id"

	FieldPredecessorOccurrenceID schema.Key = "program-field/routed-predecessor.occurrence-id"
	FieldPredecessorRouteID      schema.Key = "program-field/routed-predecessor.route-id"
	FieldPredecessorPointID      schema.Key = "program-field/routed-predecessor.point-id"

	FieldModuleImportCallID schema.Key = "program-field/module-import.call-id"

	// FieldSubjectLivenessSpanID is the canonical identity of one liveness
	// span. It is the same identity the OccurrenceSubjectLiveness occurrence
	// carries by owner law, which is what lets the relation below join the two
	// spaces without a second key.
	FieldSubjectLivenessSpanID schema.Key = "program-field/subject-liveness-span.id"
)

// Canonical relation, output, and requirement declarations.
const (
	RelationOccurrenceCall           schema.Key = "program-relation/occurrence-call"
	RelationCallValuedResultZero     schema.Key = "program-relation/call-valued-result-zero"
	RelationTailTransferResult       schema.Key = "program-relation/tail-transfer-result"
	RelationOccurrenceClosureProof   schema.Key = "program-relation/occurrence-closure-proof"
	RelationOccurrenceEntryGeometry  schema.Key = "program-relation/occurrence-entry-geometry"
	RelationOccurrenceFinishGeometry schema.Key = "program-relation/occurrence-finish-geometry"
	RelationOccurrencePredecessor    schema.Key = "program-relation/occurrence-predecessor"
	RelationCallModuleImport         schema.Key = "program-relation/call-module-import"
	// RelationOccurrenceSubjectLiveness reaches the liveness row an executable
	// subject-liveness occurrence is the view of. It is the candidate source a
	// rule issued on that occurrence family draws its rows through.
	RelationOccurrenceSubjectLiveness schema.Key = "program-relation/occurrence-subject-liveness"

	OutputOccurrence   schema.Key = "program-output/occurrence"
	OutputCall         schema.Key = "program-output/call"
	OutputResultSlot   schema.Key = "program-output/result-slot"
	OutputResultValue  schema.Key = "program-output/result-value"
	OutputClosureProof schema.Key = "program-output/closure-proof"

	RequirementUnrestricted   schema.Key = "program-requirement/unrestricted"
	RequirementCallResult     schema.Key = "program-requirement/call-result"
	RequirementCallResultSlot schema.Key = "program-requirement/call-result-slot"
	RequirementTailTransfer   schema.Key = "program-requirement/tail-transfer-result"
	RequirementClosureCapture schema.Key = "program-requirement/closure-capture"
	RequirementModuleLoadCall schema.Key = "program-requirement/module-load-call-result"
)

type fieldDeclaration struct {
	key         schema.Key
	space       schema.Key
	typ         schemaissuance.DataType
	cardinality schemaissuance.Cardinality
}

// CodeFamily is a domain-authored refinement of one canonical Program
// occurrence family. The payload value remains data in the sealed predicate;
// neither Plan nor the executor receives a code side channel.
type CodeFamily struct {
	Key  schema.Key
	Kind programschema.OccurrenceKind
	Code uint64
}

const (
	geometryEntry uint64 = iota + 1
	geometryFinish
)

// Entries returns this owner package's immutable declaration contribution.
// It refuses as one transaction: a missing row or field never produces a
// partially usable machine vocabulary.
func Entries(codeFamilies ...CodeFamily) ([]*schemaissuance.Entry, bool) {
	types := []schema.Key{
		schemaissuance.TypeRelationIndex,
		schemaissuance.TypeRelationCount,
		schemaissuance.TypePoint,
		schemaissuance.TypePointIdentity,
		schemaissuance.TypeRoute,
		schemaissuance.TypeRouteIdentity,
		schemaissuance.TypeEmission,
		schemaissuance.TypeRuleKey,
		schemaissuance.TypeAxisKey,
		TypeContentID,
		TypeOccurrenceKind,
		TypeOccurrenceCode,
		TypeCallForm,
		TypeArgumentCount,
		TypeResultSlotOrdinal,
		TypeResultSlotSource,
		TypeResultSlotConsumer,
		TypeGeometryKind,
		TypeGeometryPosition,
	}
	rows := []schema.Key{RowOccurrence, RowCall, RowCallResultSlot, RowClosureProof, RowGeometryPoint, RowPredecessor, RowModuleImport, RowSubjectLivenessSpan}
	fields := []fieldDeclaration{
		{FieldOccurrenceKind, RowOccurrence, schemaissuance.UintType(TypeOccurrenceKind), schemaissuance.CardinalityOne},
		{FieldOccurrenceID, RowOccurrence, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldOccurrenceCode, RowOccurrence, schemaissuance.UintType(TypeOccurrenceCode), schemaissuance.CardinalityOne},
		{FieldOccurrenceInput0, RowOccurrence, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOptional},
		{FieldOccurrenceInput1, RowOccurrence, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOptional},
		{FieldOccurrenceInput2, RowOccurrence, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOptional},
		{FieldOccurrenceCallID, RowOccurrence, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOptional},
		{FieldCallID, RowCall, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldCallForm, RowCall, schemaissuance.UintType(TypeCallForm), schemaissuance.CardinalityOne},
		{FieldCallArgumentCount, RowCall, schemaissuance.UintType(TypeArgumentCount), schemaissuance.CardinalityOne},
		{FieldCallReceiverPresent, RowCall, schemaissuance.BoolType(), schemaissuance.CardinalityOne},
		{FieldCallTailPresent, RowCall, schemaissuance.BoolType(), schemaissuance.CardinalityOne},
		{FieldResultSlotID, RowCallResultSlot, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldResultSlotCallID, RowCallResultSlot, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldResultSlotOrdinal, RowCallResultSlot, schemaissuance.UintType(TypeResultSlotOrdinal), schemaissuance.CardinalityOne},
		{FieldResultSlotValueID, RowCallResultSlot, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOptional},
		{FieldResultSlotSourceKind, RowCallResultSlot, schemaissuance.UintType(TypeResultSlotSource), schemaissuance.CardinalityOne},
		{FieldResultSlotConsumerKind, RowCallResultSlot, schemaissuance.UintType(TypeResultSlotConsumer), schemaissuance.CardinalityOne},
		{FieldResultSlotConsumerID, RowCallResultSlot, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldClosureProofOccurrenceID, RowClosureProof, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldGeometryOccurrenceID, RowGeometryPoint, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldGeometryOccurrenceKind, RowGeometryPoint, schemaissuance.UintType(TypeOccurrenceKind), schemaissuance.CardinalityOne},
		{FieldGeometryKind, RowGeometryPoint, schemaissuance.UintType(TypeGeometryKind), schemaissuance.CardinalityOne},
		{FieldGeometryPosition, RowGeometryPoint, schemaissuance.UintType(TypeGeometryPosition), schemaissuance.CardinalityOne},
		{FieldGeometryPointID, RowGeometryPoint, schemaissuance.IdentityType(schemaissuance.TypePointIdentity), schemaissuance.CardinalityOne},
		{FieldPredecessorOccurrenceID, RowPredecessor, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldPredecessorRouteID, RowPredecessor, schemaissuance.IdentityType(schemaissuance.TypeRouteIdentity), schemaissuance.CardinalityOne},
		{FieldPredecessorPointID, RowPredecessor, schemaissuance.IdentityType(schemaissuance.TypePointIdentity), schemaissuance.CardinalityOne},
		{FieldModuleImportCallID, RowModuleImport, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
		{FieldSubjectLivenessSpanID, RowSubjectLivenessSpan, schemaissuance.IdentityType(TypeContentID), schemaissuance.CardinalityOne},
	}
	entries := make([]*schemaissuance.Entry, 0, len(types)+len(rows)+len(fields))
	for index, key := range types {
		entry, ok := schemaissuance.New(schemaissuance.Spec{Key: key, Kind: schemaissuance.KindType, Ordinal: uint16(index + 1)})
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	for index, key := range rows {
		entry, ok := schemaissuance.New(schemaissuance.Spec{Key: key, Kind: schemaissuance.KindRowSpace, Ordinal: uint16(index + 1)})
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	for index, field := range fields {
		entry, ok := schemaissuance.New(schemaissuance.Spec{
			Key: field.key, Kind: schemaissuance.KindField, Ordinal: uint16(index + 1),
			Space: field.space, Type: field.typ, Cardinality: field.cardinality,
		})
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	if !appendMachineDeclarations(&entries, codeFamilies) {
		return nil, false
	}
	return entries, true
}

func appendMachineDeclarations(entries *[]*schemaissuance.Entry, codeFamilies []CodeFamily) bool {
	add := func(spec schemaissuance.Spec) bool {
		entry, ok := schemaissuance.New(spec)
		if ok {
			*entries = append(*entries, entry)
		}
		return ok
	}
	join := func(source, target schema.Key) schemaissuance.JoinField {
		return schemaissuance.JoinField{Source: source, Target: target, Missing: schemaissuance.JoinMissingNoEdge}
	}
	trueProgram := schemaissuance.Program{{Op: schemaissuance.OpLiteral, Out: 1, Type: schemaissuance.BoolType(), Literal: 1}}
	relations := []schemaissuance.Spec{
		{Key: RelationOccurrenceCall, Kind: schemaissuance.KindRelation, Ordinal: 1, Space: RowOccurrence, Target: RowCall, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceCallID, FieldCallID)}, Program: trueProgram, Result: 1},
		{Key: RelationCallValuedResultZero, Kind: schemaissuance.KindRelation, Ordinal: 2, Space: RowCall, Target: RowCallResultSlot, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldCallID, FieldResultSlotCallID)}, Program: schemaissuance.Program{
				{Op: schemaissuance.OpItem, Out: 1},
				{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldResultSlotOrdinal},
				{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeResultSlotOrdinal)},
				{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
				{Op: schemaissuance.OpRead, Out: 5, Args: [6]uint16{1}, Ref: FieldResultSlotValueID},
				{Op: schemaissuance.OpPresent, Out: 6, Args: [6]uint16{5}},
				{Op: schemaissuance.OpAnd, Out: 7, Args: [6]uint16{4, 6}},
			}, Result: 7},
		{Key: RelationTailTransferResult, Kind: schemaissuance.KindRelation, Ordinal: 3, Space: RowOccurrence, Target: RowCallResultSlot, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceInput1, FieldResultSlotValueID), join(FieldOccurrenceInput2, FieldResultSlotConsumerID)}, Program: schemaissuance.Program{
				{Op: schemaissuance.OpItem, Out: 1},
				{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldResultSlotSourceKind},
				{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeResultSlotSource), Literal: uint64(programschema.CallResultSlotSourceValuesTail)},
				{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
				{Op: schemaissuance.OpRead, Out: 5, Args: [6]uint16{1}, Ref: FieldResultSlotConsumerKind},
				{Op: schemaissuance.OpLiteral, Out: 6, Type: schemaissuance.UintType(TypeResultSlotConsumer), Literal: uint64(programschema.CallResultSlotConsumerCell)},
				{Op: schemaissuance.OpEqual, Out: 7, Args: [6]uint16{5, 6}},
				{Op: schemaissuance.OpAnd, Out: 8, Args: [6]uint16{4, 7}},
			}, Result: 8},
		{Key: RelationOccurrenceClosureProof, Kind: schemaissuance.KindRelation, Ordinal: 4, Space: RowOccurrence, Target: RowClosureProof, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceID, FieldClosureProofOccurrenceID)}, Program: trueProgram, Result: 1},
		{Key: RelationOccurrenceEntryGeometry, Kind: schemaissuance.KindRelation, Ordinal: 5, Space: RowOccurrence, Target: RowGeometryPoint, Cardinality: schemaissuance.CardinalityMany,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceID, FieldGeometryOccurrenceID), join(FieldOccurrenceKind, FieldGeometryOccurrenceKind)}, Program: geometryRelationProgram(geometryEntry), Result: 4},
		{Key: RelationOccurrenceFinishGeometry, Kind: schemaissuance.KindRelation, Ordinal: 6, Space: RowOccurrence, Target: RowGeometryPoint, Cardinality: schemaissuance.CardinalityMany,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceID, FieldGeometryOccurrenceID), join(FieldOccurrenceKind, FieldGeometryOccurrenceKind)}, Program: geometryRelationProgram(geometryFinish), Result: 4},
		{Key: RelationOccurrencePredecessor, Kind: schemaissuance.KindRelation, Ordinal: 7, Space: RowOccurrence, Target: RowPredecessor, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceID, FieldPredecessorOccurrenceID)}, Program: trueProgram, Result: 1},
		{Key: RelationCallModuleImport, Kind: schemaissuance.KindRelation, Ordinal: 8, Space: RowCall, Target: RowModuleImport, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldCallID, FieldModuleImportCallID)}, Program: trueProgram, Result: 1},
		{Key: RelationOccurrenceSubjectLiveness, Kind: schemaissuance.KindRelation, Ordinal: 9, Space: RowOccurrence, Target: RowSubjectLivenessSpan, Cardinality: schemaissuance.CardinalityOptional,
			Joins: []schemaissuance.JoinField{join(FieldOccurrenceID, FieldSubjectLivenessSpanID)}, Program: trueProgram, Result: 1},
	}
	for _, spec := range relations {
		if !add(spec) {
			return false
		}
	}
	families, familiesOK := familyDeclarations(codeFamilies)
	if !familiesOK {
		return false
	}
	for _, spec := range families {
		if !add(spec) {
			return false
		}
	}
	outputs := []schemaissuance.Spec{
		{Key: OutputOccurrence, Kind: schemaissuance.KindOutput, Ordinal: 1, Type: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: RowOccurrence, Cardinality: schemaissuance.CardinalityOne}},
		{Key: OutputCall, Kind: schemaissuance.KindOutput, Ordinal: 2, Type: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: RowCall, Cardinality: schemaissuance.CardinalityOne}},
		{Key: OutputResultSlot, Kind: schemaissuance.KindOutput, Ordinal: 3, Type: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: RowCallResultSlot, Cardinality: schemaissuance.CardinalityOne}},
		{Key: OutputResultValue, Kind: schemaissuance.KindOutput, Ordinal: 4, Type: schemaissuance.IdentityType(TypeContentID)},
		{Key: OutputClosureProof, Kind: schemaissuance.KindOutput, Ordinal: 5, Type: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: RowClosureProof, Cardinality: schemaissuance.CardinalityOne}},
	}
	for _, spec := range outputs {
		if !add(spec) {
			return false
		}
	}
	for _, spec := range requirementDeclarations() {
		if !add(spec) {
			return false
		}
	}
	for _, spec := range inputDeclarations() {
		if !add(spec) {
			return false
		}
	}
	for _, spec := range stageDeclarations() {
		if !add(spec) {
			return false
		}
	}
	for _, spec := range formDeclarations() {
		if !add(spec) {
			return false
		}
	}
	return true
}

func geometryRelationProgram(kind uint64) schemaissuance.Program {
	return schemaissuance.Program{
		{Op: schemaissuance.OpItem, Out: 1},
		{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldGeometryKind},
		{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeGeometryKind), Literal: kind},
		{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
	}
}

func familyDeclarations(codeFamilies []CodeFamily) ([]schemaissuance.Spec, bool) {
	families := []struct {
		key  schema.Key
		kind programschema.OccurrenceKind
	}{
		{"occurrence/point-attachment", programschema.OccurrencePointAttachment},
		{"occurrence/values", programschema.OccurrenceValues},
		{"occurrence/values-member", programschema.OccurrenceValuesMember},
		{"occurrence/values-tail", programschema.OccurrenceValuesTail},
		{"occurrence/value-source", programschema.OccurrenceValueSource},
		{"occurrence/storage-read", programschema.OccurrenceStorageRead},
		{"occurrence/storage-bind", programschema.OccurrenceStorageBind},
		{"occurrence/storage-bind-transfer", programschema.OccurrenceStorageBindTransfer},
		{"occurrence/storage-assignment", programschema.OccurrenceStorageAssignment},
		{"occurrence/storage-write", programschema.OccurrenceStorageWrite},
		{"occurrence/index-read", programschema.OccurrenceIndexRead},
		{"occurrence/index-write", programschema.OccurrenceIndexWrite},
		{"occurrence/allocation", programschema.OccurrenceAllocation},
		{"occurrence/allocation-field", programschema.OccurrenceAllocationField},
		{"occurrence/call", programschema.OccurrenceCall},
		{"occurrence/call-activation", programschema.OccurrenceCallActivation},
		{"occurrence/call-boundary", programschema.OccurrenceCallBoundary},
		{"occurrence/call-arm", programschema.OccurrenceCallArm},
		{"occurrence/call-argument", programschema.OccurrenceCallArgument},
		{"occurrence/call-type-argument", programschema.OccurrenceCallTypeArgument},
		{"occurrence/unary", programschema.OccurrenceUnary},
		{"occurrence/select", programschema.OccurrenceSelect},
		{"occurrence/value-claim", programschema.OccurrenceValueClaim},
		{"occurrence/binary-arithmetic", programschema.OccurrenceBinaryArithmetic},
		{"occurrence/binary-equality", programschema.OccurrenceBinaryEquality},
		{"occurrence/binary-order", programschema.OccurrenceBinaryOrder},
		{"occurrence/binary-presence-refinement", programschema.OccurrenceBinaryPresenceRefinement},
		{"occurrence/return-boundary", programschema.OccurrenceReturnBoundary},
		{"occurrence/formal-entry", programschema.OccurrenceFormalEntry},
		{"occurrence/global-entry", programschema.OccurrenceGlobalEntry},
		{"occurrence/operation-predicate-refinement", programschema.OccurrenceOperationPredicateRefinement},
		{"occurrence/binary-concat", programschema.OccurrenceBinaryConcat},
		{"occurrence/subject-liveness", programschema.OccurrenceSubjectLiveness},
	}
	declarations := make([]schemaissuance.Spec, 0, len(families))
	for index, family := range families {
		declarations = append(declarations, schemaissuance.Spec{
			Key: family.key, Kind: schemaissuance.KindFamily, Ordinal: uint16(index + 1), Space: RowOccurrence,
			Program: schemaissuance.Program{
				{Op: schemaissuance.OpCurrent, Out: 1},
				{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldOccurrenceKind},
				{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeOccurrenceKind), Literal: uint64(family.kind)},
				{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
			}, Result: 4,
		})
	}
	seen := make(map[schema.Key]struct{}, len(declarations)+len(codeFamilies))
	for _, declaration := range declarations {
		seen[declaration.Key] = struct{}{}
	}
	for _, family := range codeFamilies {
		if !family.Key.Available() || !family.Kind.Valid() {
			return nil, false
		}
		if _, duplicate := seen[family.Key]; duplicate {
			return nil, false
		}
		seen[family.Key] = struct{}{}
		declarations = append(declarations, schemaissuance.Spec{
			Key: family.Key, Kind: schemaissuance.KindFamily, Ordinal: uint16(len(declarations) + 1), Space: RowOccurrence,
			Program: schemaissuance.Program{
				{Op: schemaissuance.OpCurrent, Out: 1},
				{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldOccurrenceKind},
				{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeOccurrenceKind), Literal: uint64(family.Kind)},
				{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
				{Op: schemaissuance.OpRead, Out: 5, Args: [6]uint16{1}, Ref: FieldOccurrenceCode},
				{Op: schemaissuance.OpLiteral, Out: 6, Type: schemaissuance.UintType(TypeOccurrenceCode), Literal: family.Code},
				{Op: schemaissuance.OpEqual, Out: 7, Args: [6]uint16{5, 6}},
				{Op: schemaissuance.OpAnd, Out: 8, Args: [6]uint16{4, 7}},
			}, Result: 8,
		})
	}
	return declarations, true
}

func inputDeclarations() []schemaissuance.Spec {
	return []schemaissuance.Spec{
		{Key: InputNone, Kind: schemaissuance.KindInput, Ordinal: 1, Input: schemaissuance.InputNone, InputSource: schemaissuance.InputSourceNone, Selection: schemaissuance.InputSelectionNone},
		{Key: InputEntryGeometry, Kind: schemaissuance.KindInput, Ordinal: 2, Input: schemaissuance.InputEntry, InputSource: schemaissuance.InputSourceRelation, Selection: schemaissuance.InputSelectionOnly, Source: RelationOccurrenceEntryGeometry},
		{Key: InputLocalStage, Kind: schemaissuance.KindInput, Ordinal: 3, Input: schemaissuance.InputFinish, InputSource: schemaissuance.InputSourceStage, Selection: schemaissuance.InputSelectionStage, Source: StageLocal},
		{Key: InputPreviousStage, Kind: schemaissuance.KindInput, Ordinal: 4, Input: schemaissuance.InputFinish, InputSource: schemaissuance.InputSourcePrevious, Selection: schemaissuance.InputSelectionPrevious},
		{Key: InputCallDispatchStage, Kind: schemaissuance.KindInput, Ordinal: 5, Input: schemaissuance.InputFinish, InputSource: schemaissuance.InputSourceStage, Selection: schemaissuance.InputSelectionStage, Source: StageCallDispatch},
		{Key: InputCallSummaryStage, Kind: schemaissuance.KindInput, Ordinal: 6, Input: schemaissuance.InputFinish, InputSource: schemaissuance.InputSourceStage, Selection: schemaissuance.InputSelectionStage, Source: StageCallSummary},
		{Key: InputRouteArrival, Kind: schemaissuance.KindInput, Ordinal: 8, Input: schemaissuance.InputPredecessor, InputSource: schemaissuance.InputSourceRoute, Selection: schemaissuance.InputSelectionRoute},
		{Key: InputPredecessorGeometry, Kind: schemaissuance.KindInput, Ordinal: 7, Input: schemaissuance.InputPredecessor, InputSource: schemaissuance.InputSourceRelation, Selection: schemaissuance.InputSelectionOnly, Source: RelationOccurrencePredecessor},
	}
}

func stageDeclarations() []schemaissuance.Spec {
	pointMany := schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityMany}
	pointOne := pointMany
	pointOne.Cardinality = schemaissuance.CardinalityOne
	content := schemaissuance.IdentityType(TypeContentID)
	all := schemaissuance.StageTransportAll
	writes := schemaissuance.StageTransportWritesOfStages
	exceptWrites := schemaissuance.StageTransportAllExceptWritesOfStages
	exceptTarget := schemaissuance.StageTransportAllExceptTargetWrites
	return []schemaissuance.Spec{
		{Key: StageBase, Kind: schemaissuance.KindStage, Ordinal: 1,
			Constructor: schemaissuance.StageConstructorPassthrough, Parameters: []schemaissuance.DataType{pointMany}, Base: 1, Identity: []uint16{1}, Order: 1},
		{Key: StageLocal, Kind: schemaissuance.KindStage, Ordinal: 2,
			Constructor: schemaissuance.StageConstructorFramed, Parameters: []schemaissuance.DataType{pointMany}, Base: 1, Identity: []uint16{1},
			Order: 2, Predecessors: []schema.Key{StageBase}, Edges: []schemaissuance.StageEdge{{Source: schemaissuance.StageEdgeSourcePrevious, Transport: all, Framing: "analysis/program-artifact/local-transfer"}},
			Framing: "analysis/program-artifact/local-stage", InputCount: 1},
		{Key: StageSuccessor, Kind: schemaissuance.KindStage, Ordinal: 3,
			Constructor: schemaissuance.StageConstructorFramed, Parameters: []schemaissuance.DataType{pointMany}, Base: 1, Identity: []uint16{1},
			Order: 4, Predecessors: []schema.Key{StageLocal}, Edges: []schemaissuance.StageEdge{{Source: schemaissuance.StageEdgeSourceStage, Stage: StageLocal, Transport: all, Framing: "analysis/program-artifact/local-successor-transfer"}},
			Framing: "analysis/program-artifact/local-successor-stage", InputCount: 1},
		{Key: StageComputation, Kind: schemaissuance.KindStage, Ordinal: 4,
			Constructor: schemaissuance.StageConstructorFramed,
			Parameters:  []schemaissuance.DataType{pointMany, schemaissuance.IdentityType(schemaissuance.TypeRuleKey), content, content, content}, Base: 1,
			Identity: []uint16{1, 2, 3},
			Order:    5, Node: 3, Dependencies: []uint16{4, 5}, Predecessors: []schema.Key{StageBase},
			Edges:   []schemaissuance.StageEdge{{Source: schemaissuance.StageEdgeSourcePrevious, Transport: all, Framing: "analysis/program-artifact/local-computation-transfer"}},
			Framing: "analysis/program-artifact/local-computation-stage", InputCount: 1},
		{Key: StagePredecessor, Kind: schemaissuance.KindStage, Ordinal: 5,
			Constructor: schemaissuance.StageConstructorFramed,
			Parameters:  []schemaissuance.DataType{pointOne, schemaissuance.IdentityType(schemaissuance.TypeAxisKey)}, Base: 1,
			Identity: []uint16{1, 2},
			Order:    3, Predecessors: []schema.Key{StageLocal}, Edges: []schemaissuance.StageEdge{{Source: schemaissuance.StageEdgeSourcePrevious, Transport: exceptTarget, Framing: "analysis/program-artifact/local-predecessor-transfer"}},
			Framing: "analysis/program-artifact/local-predecessor-stage", InputCount: 1},
		// A route-proved fact stands on its route, not in its point's chain.
		// The route is part of the identity because two arms of one branch, and
		// two routes reconverging on one destination, prove different things
		// about the same coordinate: without it they would collapse onto one
		// stage and compose instead of staying separate.
		{Key: StageRoutePredecessor, Kind: schemaissuance.KindStage, Ordinal: 9,
			Constructor: schemaissuance.StageConstructorFramed,
			Parameters:  []schemaissuance.DataType{pointOne, schemaissuance.IdentityType(schemaissuance.TypeAxisKey), schemaissuance.IdentityType(schemaissuance.TypeRouteIdentity)}, Base: 1,
			Identity: []uint16{1, 2, 3},
			Order:    9, Predecessors: []schema.Key{StageBase}, Edges: []schemaissuance.StageEdge{{Source: schemaissuance.StageEdgeSourceRoute, Transport: exceptTarget, Framing: "analysis/program-artifact/route-predecessor-transfer"}},
			Framing: "analysis/program-artifact/route-predecessor-stage", InputCount: 1},
		{Key: StageCallDispatch, Kind: schemaissuance.KindStage, Ordinal: 6,
			Constructor: schemaissuance.StageConstructorFramed, Parameters: []schemaissuance.DataType{pointMany}, Base: 1, Identity: []uint16{1},
			Order: 6, Predecessors: []schema.Key{StageBase}, Edges: []schemaissuance.StageEdge{{Source: schemaissuance.StageEdgeSourcePrevious, Transport: exceptTarget, Framing: "analysis/program-artifact/call-base-dispatch-transfer"}},
			Framing: "analysis/program-artifact/call-dispatch-stage", Native: true, InputCount: 1},
		{Key: StageCallSummary, Kind: schemaissuance.KindStage, Ordinal: 7,
			Constructor: schemaissuance.StageConstructorFramed, Parameters: []schemaissuance.DataType{pointMany}, Base: 1, Identity: []uint16{1},
			Order: 7, Predecessors: []schema.Key{StageCallDispatch},
			Edges: []schemaissuance.StageEdge{
				{Source: schemaissuance.StageEdgeSourceBeforeStage, Stage: StageCallDispatch, Transport: exceptWrites, WriterStages: []schema.Key{StageCallDispatch}, Framing: "analysis/program-artifact/call-base-summary-transfer"},
				{Source: schemaissuance.StageEdgeSourceStage, Stage: StageCallDispatch, Transport: writes, WriterStages: []schema.Key{StageCallDispatch}, Framing: "analysis/program-artifact/call-dispatch-summary-transfer"},
			},
			Framing: "analysis/program-artifact/call-summary-stage", Native: true, InputCount: 1},
		{Key: StageCallEffect, Kind: schemaissuance.KindStage, Ordinal: 8,
			Constructor: schemaissuance.StageConstructorFramed, Parameters: []schemaissuance.DataType{pointMany}, Base: 1, Identity: []uint16{1},
			Order: 8, Predecessors: []schema.Key{StageCallSummary},
			Edges: []schemaissuance.StageEdge{
				{Source: schemaissuance.StageEdgeSourceStage, Stage: StageCallSummary, Transport: all, Framing: "analysis/program-artifact/call-summary-effect-transfer"},
			},
			Framing: "analysis/program-artifact/call-effect-stage", Native: true, InputCount: 1},
	}
}

func formDeclarations() []schemaissuance.Spec {
	return []schemaissuance.Spec{
		simpleForm(FormBaseNone, 1, StageBase, InputNone, false, RelationOccurrenceFinishGeometry, schemaissuance.EmptyRefuse),
		simpleForm(FormBaseNoneAllowEmpty, 2, StageBase, InputNone, false, RelationOccurrenceFinishGeometry, schemaissuance.EmptyEmitNone),
		simpleForm(FormLocalFinish, 3, StageLocal, InputPreviousStage, false, RelationOccurrenceFinishGeometry, schemaissuance.EmptyRefuse),
		localEntryForm(),
		successorForm(),
		computationForm(),
		predecessorForm(),
		routePredecessorForm(),
		callForm(FormCallDispatch, 8, StageCallDispatch, InputPreviousStage),
		callForm(FormCallSummary, 9, StageCallSummary, InputCallDispatchStage),
		callForm(FormCallEffect, 10, StageCallEffect, InputCallSummaryStage),
	}
}

func geometryProgram(relation schema.Key) schemaissuance.Program {
	return schemaissuance.Program{
		{Op: schemaissuance.OpSelection, Out: 1, Ref: OutputOccurrence},
		{Op: schemaissuance.OpFollow, Out: 2, Args: [6]uint16{1}, Ref: relation},
		{Op: schemaissuance.OpProjectPoints, Out: 3, Args: [6]uint16{2}, Ref: FieldGeometryPointID, Aux: FieldGeometryPosition},
	}
}

func simpleForm(key schema.Key, ordinal uint16, stage, input schema.Key, inputPoints bool, relation schema.Key, empty schemaissuance.EmptyPolicy) schemaissuance.Spec {
	program := geometryProgram(relation)
	inputArgs := [6]uint16{}
	if inputPoints {
		inputArgs[0] = 3
	}
	program = append(program,
		schemaissuance.Instruction{Op: schemaissuance.OpInput, Out: 4, Args: inputArgs, Ref: input},
		schemaissuance.Instruction{Op: schemaissuance.OpRequestStage, Out: 5, Args: [6]uint16{3, 4}, Ref: stage},
		schemaissuance.Instruction{Op: schemaissuance.OpEmit, Out: 6, Args: [6]uint16{5}},
	)
	return schemaissuance.Spec{Key: key, Kind: schemaissuance.KindForm, Ordinal: ordinal, Empty: empty,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence}, Program: program, Emissions: []uint16{6}}
}

func localEntryForm() schemaissuance.Spec {
	return schemaissuance.Spec{Key: FormLocalEntry, Kind: schemaissuance.KindForm, Ordinal: 4, Empty: schemaissuance.EmptyRefuse,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence},
		Program: schemaissuance.Program{
			{Op: schemaissuance.OpSelection, Out: 1, Ref: OutputOccurrence},
			{Op: schemaissuance.OpFollow, Out: 2, Args: [6]uint16{1}, Ref: RelationOccurrenceFinishGeometry},
			{Op: schemaissuance.OpProjectPoints, Out: 3, Args: [6]uint16{2}, Ref: FieldGeometryPointID, Aux: FieldGeometryPosition},
			{Op: schemaissuance.OpFollow, Out: 4, Args: [6]uint16{1}, Ref: RelationOccurrenceEntryGeometry},
			{Op: schemaissuance.OpProjectPoints, Out: 5, Args: [6]uint16{4}, Ref: FieldGeometryPointID, Aux: FieldGeometryPosition},
			{Op: schemaissuance.OpInput, Out: 6, Args: [6]uint16{5}, Ref: InputEntryGeometry},
			{Op: schemaissuance.OpRequestStage, Out: 7, Args: [6]uint16{3, 6}, Ref: StageLocal},
			{Op: schemaissuance.OpEmit, Out: 8, Args: [6]uint16{7}},
		}, Emissions: []uint16{8}}
}

func successorForm() schemaissuance.Spec {
	program := geometryProgram(RelationOccurrenceFinishGeometry)
	program = append(program,
		schemaissuance.Instruction{Op: schemaissuance.OpInput, Out: 4, Ref: InputLocalStage},
		schemaissuance.Instruction{Op: schemaissuance.OpRequestStage, Out: 5, Args: [6]uint16{3, 4}, Ref: StageSuccessor},
		schemaissuance.Instruction{Op: schemaissuance.OpEmit, Out: 6, Args: [6]uint16{5}},
	)
	return schemaissuance.Spec{Key: FormLocalSuccessor, Kind: schemaissuance.KindForm, Ordinal: 5, Empty: schemaissuance.EmptyRefuse,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence}, Program: program, Emissions: []uint16{6}}
}

func computationForm() schemaissuance.Spec {
	return schemaissuance.Spec{Key: FormComputation, Kind: schemaissuance.KindForm, Ordinal: 6, Empty: schemaissuance.EmptyRefuse,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence},
		Program: schemaissuance.Program{
			{Op: schemaissuance.OpSelection, Out: 1, Ref: OutputOccurrence},
			{Op: schemaissuance.OpFollow, Out: 2, Args: [6]uint16{1}, Ref: RelationOccurrenceFinishGeometry},
			{Op: schemaissuance.OpProjectPoints, Out: 3, Args: [6]uint16{2}, Ref: FieldGeometryPointID, Aux: FieldGeometryPosition},
			{Op: schemaissuance.OpRead, Out: 4, Args: [6]uint16{1}, Ref: FieldOccurrenceID},
			{Op: schemaissuance.OpRead, Out: 5, Args: [6]uint16{1}, Ref: FieldOccurrenceInput0},
			{Op: schemaissuance.OpPresent, Out: 6, Args: [6]uint16{5}},
			{Op: schemaissuance.OpRequirePresent, Out: 7, Args: [6]uint16{5, 6}},
			{Op: schemaissuance.OpRead, Out: 8, Args: [6]uint16{1}, Ref: FieldOccurrenceInput1},
			{Op: schemaissuance.OpPresent, Out: 9, Args: [6]uint16{8}},
			{Op: schemaissuance.OpRequirePresent, Out: 10, Args: [6]uint16{8, 9}},
			{Op: schemaissuance.OpRuleKey, Out: 11},
			{Op: schemaissuance.OpInput, Out: 12, Ref: InputPreviousStage},
			{Op: schemaissuance.OpRequestStage, Out: 13, Args: [6]uint16{3, 11, 4, 7, 10, 12}, Ref: StageComputation},
			{Op: schemaissuance.OpEmit, Out: 14, Args: [6]uint16{13}},
		}, Emissions: []uint16{14}}
}

func predecessorForm() schemaissuance.Spec {
	return schemaissuance.Spec{Key: FormLocalPredecessor, Kind: schemaissuance.KindForm, Ordinal: 7, Empty: schemaissuance.EmptyRefuse,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence},
		Program: schemaissuance.Program{
			{Op: schemaissuance.OpSelection, Out: 1, Ref: OutputOccurrence},
			{Op: schemaissuance.OpFollow, Out: 2, Args: [6]uint16{1}, Ref: RelationOccurrencePredecessor},
			{Op: schemaissuance.OpExactlyOne, Out: 3, Args: [6]uint16{2}},
			{Op: schemaissuance.OpOnly, Out: 4, Args: [6]uint16{2, 3}},
			{Op: schemaissuance.OpRead, Out: 5, Args: [6]uint16{4}, Ref: FieldPredecessorPointID},
			{Op: schemaissuance.OpPoint, Out: 6, Args: [6]uint16{5}},
			{Op: schemaissuance.OpRead, Out: 7, Args: [6]uint16{4}, Ref: FieldPredecessorRouteID},
			{Op: schemaissuance.OpRoute, Out: 8, Args: [6]uint16{7}},
			{Op: schemaissuance.OpWritesKey, Out: 9},
			{Op: schemaissuance.OpInput, Out: 10, Ref: InputPreviousStage},
			{Op: schemaissuance.OpRequestStage, Out: 11, Args: [6]uint16{6, 9, 10}, Ref: StagePredecessor},
			{Op: schemaissuance.OpEmit, Out: 12, Args: [6]uint16{11, 8}},
		}, Emissions: []uint16{12}}
}

// routePredecessorForm mounts an occurrence on the route that proves it. It
// reads the same routed-predecessor relation as the local-predecessor form -
// the route and the point it lands on - but requests the routed stage, whose
// input and transfer both come from the route rather than from the chain at
// that point.
func routePredecessorForm() schemaissuance.Spec {
	return schemaissuance.Spec{Key: FormRoutePredecessor, Kind: schemaissuance.KindForm, Ordinal: 11, Empty: schemaissuance.EmptyRefuse,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence},
		Program: schemaissuance.Program{
			{Op: schemaissuance.OpSelection, Out: 1, Ref: OutputOccurrence},
			{Op: schemaissuance.OpFollow, Out: 2, Args: [6]uint16{1}, Ref: RelationOccurrencePredecessor},
			{Op: schemaissuance.OpExactlyOne, Out: 3, Args: [6]uint16{2}},
			{Op: schemaissuance.OpOnly, Out: 4, Args: [6]uint16{2, 3}},
			{Op: schemaissuance.OpRead, Out: 5, Args: [6]uint16{4}, Ref: FieldPredecessorPointID},
			{Op: schemaissuance.OpPoint, Out: 6, Args: [6]uint16{5}},
			{Op: schemaissuance.OpRead, Out: 7, Args: [6]uint16{4}, Ref: FieldPredecessorRouteID},
			{Op: schemaissuance.OpRoute, Out: 8, Args: [6]uint16{7}},
			{Op: schemaissuance.OpWritesKey, Out: 9},
			{Op: schemaissuance.OpInput, Out: 10, Ref: InputRouteArrival},
			{Op: schemaissuance.OpRequestStage, Out: 11, Args: [6]uint16{6, 9, 7, 10}, Ref: StageRoutePredecessor},
			{Op: schemaissuance.OpEmit, Out: 12, Args: [6]uint16{11, 8}},
		}, Emissions: []uint16{12}}
}

func callForm(key schema.Key, ordinal uint16, stage, input schema.Key) schemaissuance.Spec {
	program := geometryProgram(RelationOccurrenceFinishGeometry)
	program = append(program,
		schemaissuance.Instruction{Op: schemaissuance.OpInput, Out: 4, Ref: input},
		schemaissuance.Instruction{Op: schemaissuance.OpRequestStage, Out: 5, Args: [6]uint16{3, 4}, Ref: stage},
		schemaissuance.Instruction{Op: schemaissuance.OpEmit, Out: 6, Args: [6]uint16{5}},
	)
	return schemaissuance.Spec{Key: key, Kind: schemaissuance.KindForm, Ordinal: ordinal, Empty: schemaissuance.EmptyRefuse,
		Subject: OutputOccurrence, Requires: []schema.Key{OutputOccurrence}, Program: program, Emissions: []uint16{6}}
}

func requirementDeclarations() []schemaissuance.Spec {
	current := schemaissuance.Instruction{Op: schemaissuance.OpCurrent, Out: 1}
	trueLiteral := schemaissuance.Instruction{Op: schemaissuance.OpLiteral, Out: 2, Type: schemaissuance.BoolType(), Literal: 1}
	return []schemaissuance.Spec{
		{Key: RequirementUnrestricted, Kind: schemaissuance.KindRequirement, Ordinal: 1, Space: RowOccurrence,
			Program: schemaissuance.Program{current, trueLiteral}, Result: 2,
			Outputs: []schemaissuance.OutputBinding{{Output: OutputOccurrence, Register: 1, Proof: 2}}},
		{Key: RequirementCallResult, Kind: schemaissuance.KindRequirement, Ordinal: 2, Space: RowOccurrence,
			Program: callResultRequirementProgram(), Result: 32,
			Outputs: []schemaissuance.OutputBinding{
				{Output: OutputOccurrence, Register: 1, Proof: 32},
				{Output: OutputCall, Register: 7, Proof: 32},
				{Output: OutputResultSlot, Register: 22, Proof: 32},
				{Output: OutputResultValue, Register: 25, Proof: 32},
			}},
		{Key: RequirementTailTransfer, Kind: schemaissuance.KindRequirement, Ordinal: 3, Space: RowOccurrence,
			Program: tailTransferRequirementProgram(), Result: 8,
			Outputs: []schemaissuance.OutputBinding{
				{Output: OutputOccurrence, Register: 1, Proof: 8},
				{Output: OutputResultSlot, Register: 7, Proof: 8},
			}},
		{Key: RequirementClosureCapture, Kind: schemaissuance.KindRequirement, Ordinal: 4, Space: RowOccurrence,
			Program: closureRequirementProgram(), Result: 8,
			Outputs: []schemaissuance.OutputBinding{
				{Output: OutputOccurrence, Register: 1, Proof: 8},
				{Output: OutputClosureProof, Register: 7, Proof: 8},
			}},
		{Key: RequirementModuleLoadCall, Kind: schemaissuance.KindRequirement, Ordinal: 5, Space: RowOccurrence,
			Program: moduleLoadCallRequirementProgram(), Result: 35,
			Outputs: []schemaissuance.OutputBinding{
				{Output: OutputOccurrence, Register: 1, Proof: 35},
				{Output: OutputCall, Register: 7, Proof: 35},
				{Output: OutputResultSlot, Register: 22, Proof: 35},
				{Output: OutputResultValue, Register: 25, Proof: 35},
			}},
		{Key: RequirementCallResultSlot, Kind: schemaissuance.KindRequirement, Ordinal: 6, Space: RowOccurrence,
			Program: callResultSlotRequirementProgram(), Result: 16,
			Outputs: []schemaissuance.OutputBinding{
				{Output: OutputOccurrence, Register: 1, Proof: 16},
				{Output: OutputCall, Register: 7, Proof: 16},
				{Output: OutputResultSlot, Register: 10, Proof: 16},
				{Output: OutputResultValue, Register: 13, Proof: 16},
			}},
	}
}

func moduleLoadCallRequirementProgram() schemaissuance.Program {
	program := callResultRequirementProgram()
	return append(program,
		schemaissuance.Instruction{Op: schemaissuance.OpFollow, Out: 33, Args: [6]uint16{7}, Ref: RelationCallModuleImport},
		schemaissuance.Instruction{Op: schemaissuance.OpExactlyOne, Out: 34, Args: [6]uint16{33}},
		schemaissuance.Instruction{Op: schemaissuance.OpAnd, Out: 35, Args: [6]uint16{32, 34}},
	)
}

// callResultSlotRequirementProgram admits one call occurrence that owns a
// fixed valued result-zero slot, whatever its argument geometry. It is the
// exact denominator of Value's mounted CallResultSlot directory: a nullary,
// method, multi-argument, or tail-expanded call still seals a result-slot
// operand, so the strict unary plain shape of RequirementCallResult would
// leave those rows unplaced.
func callResultSlotRequirementProgram() schemaissuance.Program {
	return schemaissuance.Program{
		{Op: schemaissuance.OpCurrent, Out: 1},
		{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldOccurrenceKind},
		{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeOccurrenceKind), Literal: uint64(programschema.OccurrenceCall)},
		{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
		{Op: schemaissuance.OpFollow, Out: 5, Args: [6]uint16{1}, Ref: RelationOccurrenceCall},
		{Op: schemaissuance.OpExactlyOne, Out: 6, Args: [6]uint16{5}},
		{Op: schemaissuance.OpOnly, Out: 7, Args: [6]uint16{5, 6}},
		{Op: schemaissuance.OpFollow, Out: 8, Args: [6]uint16{7}, Ref: RelationCallValuedResultZero},
		{Op: schemaissuance.OpExactlyOne, Out: 9, Args: [6]uint16{8}},
		{Op: schemaissuance.OpOnly, Out: 10, Args: [6]uint16{8, 9}},
		{Op: schemaissuance.OpRead, Out: 11, Args: [6]uint16{10}, Ref: FieldResultSlotValueID},
		{Op: schemaissuance.OpPresent, Out: 12, Args: [6]uint16{11}},
		{Op: schemaissuance.OpRequirePresent, Out: 13, Args: [6]uint16{11, 12}},
		{Op: schemaissuance.OpAnd, Out: 14, Args: [6]uint16{4, 6}},
		{Op: schemaissuance.OpAnd, Out: 15, Args: [6]uint16{14, 9}},
		{Op: schemaissuance.OpAnd, Out: 16, Args: [6]uint16{15, 12}},
	}
}

func callResultRequirementProgram() schemaissuance.Program {
	return schemaissuance.Program{
		{Op: schemaissuance.OpCurrent, Out: 1},
		{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldOccurrenceKind},
		{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeOccurrenceKind), Literal: uint64(programschema.OccurrenceCall)},
		{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
		{Op: schemaissuance.OpFollow, Out: 5, Args: [6]uint16{1}, Ref: RelationOccurrenceCall},
		{Op: schemaissuance.OpExactlyOne, Out: 6, Args: [6]uint16{5}},
		{Op: schemaissuance.OpOnly, Out: 7, Args: [6]uint16{5, 6}},
		{Op: schemaissuance.OpRead, Out: 8, Args: [6]uint16{7}, Ref: FieldCallForm},
		{Op: schemaissuance.OpLiteral, Out: 9, Type: schemaissuance.UintType(TypeCallForm), Literal: uint64(programschema.CallFormPlain)},
		{Op: schemaissuance.OpEqual, Out: 10, Args: [6]uint16{8, 9}},
		{Op: schemaissuance.OpRead, Out: 11, Args: [6]uint16{7}, Ref: FieldCallArgumentCount},
		{Op: schemaissuance.OpLiteral, Out: 12, Type: schemaissuance.UintType(TypeArgumentCount), Literal: 1},
		{Op: schemaissuance.OpEqual, Out: 13, Args: [6]uint16{11, 12}},
		{Op: schemaissuance.OpRead, Out: 14, Args: [6]uint16{7}, Ref: FieldCallReceiverPresent},
		{Op: schemaissuance.OpLiteral, Out: 15, Type: schemaissuance.BoolType()},
		{Op: schemaissuance.OpEqual, Out: 16, Args: [6]uint16{14, 15}},
		{Op: schemaissuance.OpRead, Out: 17, Args: [6]uint16{7}, Ref: FieldCallTailPresent},
		{Op: schemaissuance.OpLiteral, Out: 18, Type: schemaissuance.BoolType()},
		{Op: schemaissuance.OpEqual, Out: 19, Args: [6]uint16{17, 18}},
		{Op: schemaissuance.OpFollow, Out: 20, Args: [6]uint16{7}, Ref: RelationCallValuedResultZero},
		{Op: schemaissuance.OpExactlyOne, Out: 21, Args: [6]uint16{20}},
		{Op: schemaissuance.OpOnly, Out: 22, Args: [6]uint16{20, 21}},
		{Op: schemaissuance.OpRead, Out: 23, Args: [6]uint16{22}, Ref: FieldResultSlotValueID},
		{Op: schemaissuance.OpPresent, Out: 24, Args: [6]uint16{23}},
		{Op: schemaissuance.OpRequirePresent, Out: 25, Args: [6]uint16{23, 24}},
		{Op: schemaissuance.OpAnd, Out: 26, Args: [6]uint16{4, 6}},
		{Op: schemaissuance.OpAnd, Out: 27, Args: [6]uint16{26, 10}},
		{Op: schemaissuance.OpAnd, Out: 28, Args: [6]uint16{27, 13}},
		{Op: schemaissuance.OpAnd, Out: 29, Args: [6]uint16{28, 16}},
		{Op: schemaissuance.OpAnd, Out: 30, Args: [6]uint16{29, 19}},
		{Op: schemaissuance.OpAnd, Out: 31, Args: [6]uint16{30, 21}},
		{Op: schemaissuance.OpAnd, Out: 32, Args: [6]uint16{31, 24}},
	}
}

func tailTransferRequirementProgram() schemaissuance.Program {
	return schemaissuance.Program{
		{Op: schemaissuance.OpCurrent, Out: 1},
		{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldOccurrenceKind},
		{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeOccurrenceKind), Literal: uint64(programschema.OccurrenceStorageBindTransfer)},
		{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
		{Op: schemaissuance.OpFollow, Out: 5, Args: [6]uint16{1}, Ref: RelationTailTransferResult},
		{Op: schemaissuance.OpExactlyOne, Out: 6, Args: [6]uint16{5}},
		{Op: schemaissuance.OpOnly, Out: 7, Args: [6]uint16{5, 6}},
		{Op: schemaissuance.OpAnd, Out: 8, Args: [6]uint16{4, 6}},
	}
}

func closureRequirementProgram() schemaissuance.Program {
	return schemaissuance.Program{
		{Op: schemaissuance.OpCurrent, Out: 1},
		{Op: schemaissuance.OpRead, Out: 2, Args: [6]uint16{1}, Ref: FieldOccurrenceKind},
		{Op: schemaissuance.OpLiteral, Out: 3, Type: schemaissuance.UintType(TypeOccurrenceKind), Literal: uint64(programschema.OccurrenceAllocation)},
		{Op: schemaissuance.OpEqual, Out: 4, Args: [6]uint16{2, 3}},
		{Op: schemaissuance.OpFollow, Out: 5, Args: [6]uint16{1}, Ref: RelationOccurrenceClosureProof},
		{Op: schemaissuance.OpExactlyOne, Out: 6, Args: [6]uint16{5}},
		{Op: schemaissuance.OpOnly, Out: 7, Args: [6]uint16{5, 6}},
		{Op: schemaissuance.OpAnd, Out: 8, Args: [6]uint16{4, 6}},
	}
}
