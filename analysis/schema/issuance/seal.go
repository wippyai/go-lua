package issuance

import (
	"github.com/wippyai/go-lua/analysis/schema"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
)

const (
	TypeRelationIndex schema.Key = "machine-type/relation-index"
	TypeRelationCount schema.Key = "machine-type/relation-count"
	TypePoint         schema.Key = "machine-type/point"
	TypePointIdentity schema.Key = "machine-type/point-identity"
	TypeRoute         schema.Key = "machine-type/route"
	TypeRouteIdentity schema.Key = "machine-type/route-identity"
	TypeEmission      schema.Key = "machine-type/emission"
	TypeRuleKey       schema.Key = "machine-type/rule-key"
	TypeAxisKey       schema.Key = "machine-type/axis-key"
)

func (surface *Surface) Seal(view seal.View, _ seal.Sealed) schema.SealFailure {
	if surface == nil || view.Kind() != schema.SurfaceKindIssuance {
		return seal.SurfaceLawFailure(schema.SurfaceKindIssuance, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
	}
	entries := make(map[schema.Key]*Entry, view.Count())
	ordinals := make(map[Kind]map[uint16]schema.EntryID)
	framings := make(map[string]schema.EntryID)
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil || !entry.declarationComplete() {
			return failure(entry, LawEntryShape, schema.DispositionMalformed)
		}
		if entry.id != schema.NewEntryID(schema.SurfaceKindIssuance, entry.key) {
			return failure(entry, LawEntryIdentity, schema.DispositionMalformed)
		}
		entries[entry.key] = entry
		byOrdinal := ordinals[entry.kind]
		if byOrdinal == nil {
			byOrdinal = make(map[uint16]schema.EntryID)
			ordinals[entry.kind] = byOrdinal
		}
		if _, duplicate := byOrdinal[entry.ordinal]; duplicate {
			return failure(entry, LawOrdinalUnique, schema.DispositionDuplicate)
		}
		byOrdinal[entry.ordinal] = entry.id
		if entry.framing != "" {
			if _, duplicate := framings[entry.framing]; duplicate {
				return failure(entry, LawFramingUnique, schema.DispositionDuplicate)
			}
			framings[entry.framing] = entry.id
		}
		for _, edge := range entry.edges {
			if _, duplicate := framings[edge.Framing]; duplicate {
				return failure(entry, LawFramingUnique, schema.DispositionDuplicate)
			}
			framings[edge.Framing] = entry.id
		}
	}
	for kind, byOrdinal := range ordinals {
		if len(byOrdinal) > int(^uint16(0)) {
			return failure(firstOfKind(entries, kind), LawOrdinalDense, schema.DispositionMalformed)
		}
		for ordinal := 1; ordinal <= len(byOrdinal); ordinal++ {
			if _, present := byOrdinal[uint16(ordinal)]; !present {
				return failure(firstOfKind(entries, kind), LawOrdinalDense, schema.DispositionIncomplete)
			}
		}
	}
	stageOrders := make(map[uint16]schema.EntryID)
	for _, entry := range entries {
		if entry.kind != KindStage {
			continue
		}
		if prior, duplicate := stageOrders[entry.order]; duplicate {
			_ = prior
			return failure(entry, LawStageAcyclic, schema.DispositionDuplicate)
		}
		stageOrders[entry.order] = entry.id
	}
	for order := 1; order <= len(stageOrders); order++ {
		if _, present := stageOrders[uint16(order)]; !present {
			return failure(firstOfKind(entries, KindStage), LawStageAcyclic, schema.DispositionIncomplete)
		}
	}
	for _, entry := range entries {
		if !sealReferences(entry, entries) {
			return failure(entry, LawReferenceKind, schema.DispositionMalformed)
		}
		types, ok := validProgram(entry, entries)
		if !ok || !validProgramOutput(entry, types, entries) {
			return failure(entry, LawProgramShape, schema.DispositionMalformed)
		}
	}
	if cyclicRelations(entries) {
		return failure(firstOfKind(entries, KindRelation), LawRelationAcyclic, schema.DispositionMalformed)
	}
	if cyclicStages(entries) {
		return failure(firstOfKind(entries, KindStage), LawStageAcyclic, schema.DispositionMalformed)
	}
	return schema.SealFailure{}
}

func failure(entry *Entry, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	var id schema.EntryID
	if entry != nil {
		id = entry.id
	}
	return seal.SurfaceLawFailure(schema.SurfaceKindIssuance, id, law, disposition)
}

func firstOfKind(entries map[schema.Key]*Entry, kind Kind) *Entry {
	for _, entry := range entries {
		if entry.kind == kind {
			return entry
		}
	}
	return nil
}

func sealReferences(entry *Entry, entries map[schema.Key]*Entry) bool {
	resolve := func(key schema.Key, kind Kind) bool {
		row := entries[key]
		return row != nil && row.kind == kind
	}
	switch entry.kind {
	case KindField:
		return resolve(entry.space, KindRowSpace) && typeResolves(entry.typ, entries)
	case KindRelation:
		if !resolve(entry.space, KindRowSpace) || !resolve(entry.target, KindRowSpace) {
			return false
		}
		seen := make(map[JoinField]struct{}, len(entry.joins))
		for _, join := range entry.joins {
			source, target := entries[join.Source], entries[join.Target]
			if source == nil || target == nil || source.kind != KindField ||
				target.kind != KindField || source.space != entry.space ||
				target.space != entry.target || source.typ != target.typ ||
				join.Missing != JoinMissingNoEdge {
				return false
			}
			if _, duplicate := seen[join]; duplicate {
				return false
			}
			seen[join] = struct{}{}
		}
		return true
	case KindOutput:
		return typeResolves(entry.typ, entries)
	case KindRequirement:
		if !resolve(entry.space, KindRowSpace) {
			return false
		}
		for _, output := range entry.outputs {
			if !resolve(output.Output, KindOutput) {
				return false
			}
		}
		return true
	case KindFamily:
		return resolve(entry.space, KindRowSpace)
	case KindForm:
		if !resolve(entry.subject, KindOutput) {
			return false
		}
		for _, required := range entry.requires {
			if !resolve(required, KindOutput) {
				return false
			}
		}
		return true
	case KindInput:
		switch entry.inputSource {
		case InputSourceRelation:
			return resolve(entry.source, KindRelation)
		case InputSourceStage:
			return resolve(entry.source, KindStage)
		default:
			return !entry.source.Available()
		}
	case KindStage:
		for _, parameter := range entry.parameters {
			if !typeResolves(parameter, entries) {
				return false
			}
		}
		for _, edge := range entry.edges {
			if (edge.Source == StageEdgeSourceStage || edge.Source == StageEdgeSourceBeforeStage) && !resolve(edge.Stage, KindStage) {
				return false
			}
			if edge.Source == StageEdgeSourceBeforeStage && entries[edge.Stage].order >= entry.order {
				return false
			}
			if edge.Source == StageEdgeSourceRoute && !stageCarriesRoute(entry) {
				return false
			}
			for _, stage := range edge.WriterStages {
				if !resolve(stage, KindStage) {
					return false
				}
			}
		}
		for _, predecessor := range entry.predecessors {
			if !resolve(predecessor, KindStage) || entries[predecessor].order >= entry.order ||
				!stageAutoClosable(entries[predecessor]) {
				return false
			}
		}
		if entry.constructor == StageConstructorPassthrough {
			return len(entry.parameters) == 1 &&
				entry.base == 1 &&
				len(entry.identity) == 1 && entry.identity[0] == 1 &&
				entry.parameters[0] == (DataType{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany})
		}
		return true
	default:
		return true
	}
}

func typeResolves(typ DataType, entries map[schema.Key]*Entry) bool {
	if !typ.available() {
		return false
	}
	resolve := func(key schema.Key, kind Kind) bool {
		row := entries[key]
		return row != nil && row.kind == kind
	}
	switch typ.Value {
	case ValueUint, ValueIdentity:
		return resolve(typ.Name, KindType)
	case ValueRow:
		return resolve(typ.Space, KindRowSpace)
	case ValueRange:
		relation := entries[typ.Relation]
		return resolve(typ.Space, KindRowSpace) && relation != nil &&
			relation.kind == KindRelation && relation.target == typ.Space &&
			relation.cardinality == typ.Cardinality
	case ValuePointRange, ValueRoute, ValueEmissionRange:
		return resolve(typ.Name, KindType)
	case ValueInputRange:
		return resolve(typ.Name, KindInput)
	case ValueStageRequestRange:
		return resolve(typ.Name, KindStage)
	default:
		return true
	}
}

func validProgram(entry *Entry, entries map[schema.Key]*Entry) (map[uint16]DataType, bool) {
	if len(entry.program) == 0 {
		return nil, entry.kind != KindRelation && entry.kind != KindFamily &&
			entry.kind != KindRequirement && entry.kind != KindForm
	}
	if entry.kind != KindRelation && entry.kind != KindFamily &&
		entry.kind != KindRequirement && entry.kind != KindForm {
		return nil, false
	}
	registers := make(map[uint16]DataType, len(entry.program))
	origins := make(map[uint16]schema.Key, len(entry.program))
	type proof struct {
		op     Opcode
		source uint16
	}
	proofs := make(map[uint16]proof)
	needs := func(id uint16) (DataType, bool) {
		typ, ok := registers[id]
		return typ, id != 0 && ok
	}
	noArgs := func(instruction Instruction) bool { return instruction.Args == [6]uint16{} }
	for _, instruction := range entry.program {
		if !instruction.Op.valid() || instruction.Out == 0 || !phaseAllows(entry.kind, instruction.Op) {
			return nil, false
		}
		if instruction.Op != OpProjectPoints && instruction.Aux.Available() {
			return nil, false
		}
		if _, duplicate := registers[instruction.Out]; duplicate {
			return nil, false
		}
		var output DataType
		switch instruction.Op {
		case OpCurrent:
			if !noArgs(instruction) || instruction.Ref.Available() ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValueRow, Space: entry.space, Cardinality: CardinalityOne}
		case OpItem:
			if entry.kind != KindRelation || !noArgs(instruction) ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValueRow, Space: entry.target, Cardinality: CardinalityOne}
		case OpItemIndex:
			if entry.kind != KindRelation || !noArgs(instruction) ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			output = UintType(TypeRelationIndex)
		case OpLiteral:
			if !noArgs(instruction) || instruction.Ref.Available() ||
				!scalar(instruction.Type) || instruction.Type.Cardinality != CardinalityOne ||
				instruction.Type.Value == ValueIdentity ||
				!typeResolves(instruction.Type, entries) ||
				instruction.Type.Value == ValueBool && instruction.Literal > 1 {
				return nil, false
			}
			output = instruction.Type
		case OpRead:
			left, ok := needs(instruction.Args[0])
			field := entries[instruction.Ref]
			if !ok || !onlyArgs(instruction.Args, 1) || field == nil ||
				field.kind != KindField || left.Value != ValueRow ||
				left.Cardinality != CardinalityOne ||
				left.Space != field.space || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			output = field.typ
			output.Cardinality = field.cardinality
			origins[instruction.Out] = origins[instruction.Args[0]]
		case OpFollow:
			left, ok := needs(instruction.Args[0])
			relation := entries[instruction.Ref]
			if !ok || !onlyArgs(instruction.Args, 1) || relation == nil ||
				relation.kind != KindRelation ||
				left != (DataType{Value: ValueRow, Space: relation.space, Cardinality: CardinalityOne}) ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValueRange, Space: relation.target, Relation: relation.key, Cardinality: relation.cardinality}
			origins[instruction.Out] = relation.key
		case OpAt:
			rangeType, rangeOK := needs(instruction.Args[0])
			indexType, indexOK := needs(instruction.Args[1])
			if !rangeOK || !indexOK || !onlyArgs(instruction.Args, 2) ||
				rangeType.Value != ValueRange || indexType != UintType(TypeRelationIndex) ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValueRow, Space: rangeType.Space, Cardinality: CardinalityOptional}
			origins[instruction.Out] = origins[instruction.Args[0]]
		case OpCount:
			left, ok := needs(instruction.Args[0])
			if !ok || !onlyArgs(instruction.Args, 1) || left.Value != ValueRange ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			output = UintType(TypeRelationCount)
		case OpEqual, OpGreater, OpLess, OpAnd, OpOr:
			left, leftOK := needs(instruction.Args[0])
			right, rightOK := needs(instruction.Args[1])
			if !leftOK || !rightOK || !onlyArgs(instruction.Args, 2) ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			switch instruction.Op {
			case OpEqual:
				if left != right || left.Cardinality != CardinalityOne {
					return nil, false
				}
			case OpGreater, OpLess:
				if left != right || left.Value != ValueUint || left.Cardinality != CardinalityOne {
					return nil, false
				}
			case OpAnd, OpOr:
				if left != BoolType() || right != BoolType() {
					return nil, false
				}
			}
			output = BoolType()
		case OpEqualIfPresent:
			left, leftOK := needs(instruction.Args[0])
			right, rightOK := needs(instruction.Args[1])
			required := left
			required.Cardinality = CardinalityOne
			if !leftOK || !rightOK || !onlyArgs(instruction.Args, 2) ||
				left.Cardinality != CardinalityOptional || required != right ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			output = BoolType()
		case OpNot, OpPresent, OpExactlyOne:
			left, ok := needs(instruction.Args[0])
			if !ok || !onlyArgs(instruction.Args, 1) || instruction.Ref.Available() ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			switch instruction.Op {
			case OpNot:
				if left != BoolType() {
					return nil, false
				}
			case OpPresent:
				if left.Cardinality != CardinalityOptional {
					return nil, false
				}
				proofs[instruction.Out] = proof{op: OpPresent, source: instruction.Args[0]}
			case OpExactlyOne:
				if left.Value != ValueRange {
					return nil, false
				}
				proofs[instruction.Out] = proof{op: OpExactlyOne, source: instruction.Args[0]}
			}
			output = BoolType()
		case OpOnly, OpRequirePresent:
			value, valueOK := needs(instruction.Args[0])
			proofType, proofOK := needs(instruction.Args[1])
			requiredProof := OpExactlyOne
			if instruction.Op == OpRequirePresent {
				requiredProof = OpPresent
			}
			if !valueOK || !proofOK || !onlyArgs(instruction.Args, 2) ||
				proofType != BoolType() || proofs[instruction.Args[1]] != (proof{op: requiredProof, source: instruction.Args[0]}) ||
				instruction.Ref.Available() || instruction.Type != (DataType{}) ||
				instruction.Literal != 0 {
				return nil, false
			}
			if instruction.Op == OpOnly {
				if value.Value != ValueRange {
					return nil, false
				}
				output = DataType{Value: ValueRow, Space: value.Space, Cardinality: CardinalityOne}
				origins[instruction.Out] = origins[instruction.Args[0]]
			} else {
				if value.Cardinality != CardinalityOptional || value.Value == ValueRange {
					return nil, false
				}
				output = value
				output.Cardinality = CardinalityOne
			}
		case OpSelection:
			selected := entries[instruction.Ref]
			if selected == nil || selected.kind != KindOutput ||
				!containsKey(entry.requires, selected.key) || !noArgs(instruction) ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			output = selected.typ
		case OpRuleKey, OpWritesKey:
			if !noArgs(instruction) || instruction.Ref.Available() ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			name := TypeRuleKey
			if instruction.Op == OpWritesKey {
				name = TypeAxisKey
			}
			output = IdentityType(name)
		case OpProjectPoints:
			rangeType, rangeOK := needs(instruction.Args[0])
			pointField, orderField := entries[instruction.Ref], entries[instruction.Aux]
			if !rangeOK || !onlyArgs(instruction.Args, 1) || rangeType.Value != ValueRange ||
				pointField == nil || pointField.kind != KindField || pointField.space != rangeType.Space ||
				pointField.typ != IdentityType(TypePointIdentity) || pointField.cardinality != CardinalityOne ||
				orderField == nil || orderField.kind != KindField || orderField.space != rangeType.Space ||
				orderField.typ.Value != ValueUint || orderField.cardinality != CardinalityOne ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany}
			origins[instruction.Out] = rangeType.Relation
		case OpPoint:
			point, ok := needs(instruction.Args[0])
			if !ok || !onlyArgs(instruction.Args, 1) ||
				point != IdentityType(TypePointIdentity) || instruction.Ref.Available() ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityOne}
			origins[instruction.Out] = origins[instruction.Args[0]]
		case OpRoute:
			route, ok := needs(instruction.Args[0])
			if !ok || !onlyArgs(instruction.Args, 1) ||
				route != IdentityType(TypeRouteIdentity) || instruction.Ref.Available() ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			output = DataType{Value: ValueRoute, Name: TypeRoute, Cardinality: CardinalityOne}
			origins[instruction.Out] = origins[instruction.Args[0]]
		case OpInput:
			input := entries[instruction.Ref]
			if input == nil || input.kind != KindInput ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			switch input.inputSource {
			case InputSourceNone, InputSourceStage, InputSourcePrevious, InputSourceRoute:
				// A routed input names no operand: the route it reads is the
				// one the stage it feeds already carries in its identity.
				if !noArgs(instruction) {
					return nil, false
				}
			case InputSourceRelation:
				source, ok := needs(instruction.Args[0])
				if !ok || !onlyArgs(instruction.Args, 1) || source.Value != ValuePointRange ||
					origins[instruction.Args[0]] != input.source {
					return nil, false
				}
			default:
				return nil, false
			}
			output = DataType{Value: ValueInputRange, Name: input.key, Cardinality: CardinalityMany}
		case OpRequestStage:
			stage := entries[instruction.Ref]
			if stage == nil || stage.kind != KindStage ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 ||
				!stageArgumentsMatch(instruction.Args, stage, registers, entries) {
				return nil, false
			}
			output = DataType{Value: ValueStageRequestRange, Name: stage.key, Cardinality: CardinalityMany}
		case OpEmit:
			request, requestOK := needs(instruction.Args[0])
			if !requestOK || request.Value != ValueStageRequestRange ||
				instruction.Args[2] != 0 || instruction.Args[3] != 0 ||
				instruction.Args[4] != 0 || instruction.Args[5] != 0 ||
				instruction.Ref.Available() ||
				instruction.Type != (DataType{}) || instruction.Literal != 0 {
				return nil, false
			}
			if instruction.Args[1] != 0 {
				route, routeOK := needs(instruction.Args[1])
				if !routeOK || route.Value != ValueRoute {
					return nil, false
				}
			}
			output = DataType{Value: ValueEmissionRange, Name: TypeEmission, Cardinality: CardinalityMany}
		default:
			return nil, false
		}
		registers[instruction.Out] = output
	}
	return registers, true
}

func onlyArgs(arguments [6]uint16, count int) bool {
	for index, argument := range arguments {
		if index < count {
			if argument == 0 {
				return false
			}
			continue
		}
		if argument != 0 {
			return false
		}
	}
	return true
}

func phaseAllows(kind Kind, opcode Opcode) bool {
	switch kind {
	case KindRelation:
		return opcode >= OpCurrent && opcode <= OpRequirePresent
	case KindRequirement:
		return opcode >= OpCurrent && opcode <= OpRequirePresent &&
			opcode != OpItem && opcode != OpItemIndex
	case KindFamily:
		return opcode >= OpCurrent && opcode <= OpRequirePresent &&
			opcode != OpItem && opcode != OpItemIndex
	case KindForm:
		switch opcode {
		case OpRead, OpFollow, OpExactlyOne, OpOnly, OpPresent, OpRequirePresent,
			OpSelection, OpRuleKey, OpWritesKey,
			OpProjectPoints, OpPoint, OpRoute, OpInput, OpRequestStage, OpEmit:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func containsKey(keys []schema.Key, sought schema.Key) bool {
	for _, key := range keys {
		if key == sought {
			return true
		}
	}
	return false
}

// stageCarriesRoute reports whether a stage names its route as part of its own
// identity. Only such a stage may take a routed input or a routed edge: the
// route is what tells two stages standing on two routes into one point apart,
// so a stage that does not carry it cannot honour a per-route fact.
func stageCarriesRoute(stage *Entry) bool {
	if stage == nil {
		return false
	}
	for _, parameter := range stage.identity {
		if int(parameter) < 1 || int(parameter) > len(stage.parameters) {
			return false
		}
		if stage.parameters[parameter-1] == IdentityType(TypeRouteIdentity) {
			return true
		}
	}
	return false
}

func stageArgumentsMatch(arguments [6]uint16, stage *Entry, registers map[uint16]DataType, entries map[schema.Key]*Entry) bool {
	if stage == nil || len(stage.parameters)+int(stage.inputCount) > len(arguments) {
		return false
	}
	for index := 0; index < len(stage.parameters); index++ {
		typ, ok := registers[arguments[index]]
		if !ok || typ != stage.parameters[index] {
			return false
		}
	}
	for index := len(stage.parameters); index < len(stage.parameters)+int(stage.inputCount); index++ {
		typ, ok := registers[arguments[index]]
		input := entries[typ.Name]
		if !ok || typ.Value != ValueInputRange || typ.Cardinality != CardinalityMany ||
			input == nil || input.kind != KindInput || input.input == InputNone {
			return false
		}
		if input.inputSource == InputSourceRoute && !stageCarriesRoute(stage) {
			return false
		}
	}
	end := len(stage.parameters) + int(stage.inputCount)
	// Existing zero-input forms encode an InputNone range in one trailing
	// register. It carries no role and is ignored by the interpreter; accepting
	// it here keeps the ABI dense for authored forms while positive-width stages
	// still require exactly their declared N operands.
	if stage.inputCount == 0 && end < len(arguments) && arguments[end] != 0 {
		typ, ok := registers[arguments[end]]
		input := entries[typ.Name]
		if !ok || typ.Value != ValueInputRange || typ.Cardinality != CardinalityMany ||
			input == nil || input.kind != KindInput || input.input != InputNone {
			return false
		}
		end++
	}
	for index := end; index < len(arguments); index++ {
		if arguments[index] != 0 {
			return false
		}
	}
	return true
}

// stageAutoClosable states the complete ABI for a predecessor the scheduler
// may have to synthesize from the current execution base. Any additional
// parameter would require undeclared inference and is therefore refused while
// sealing the schema rather than compensated for during execution.
func stageAutoClosable(stage *Entry) bool {
	return stage != nil && len(stage.parameters) == 1 && stage.base == 1 &&
		(stage.parameters[0] == (DataType{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany}) ||
			stage.parameters[0] == (DataType{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityOne}))
}

func validProgramOutput(entry *Entry, types map[uint16]DataType, entries map[schema.Key]*Entry) bool {
	switch entry.kind {
	case KindRelation:
		return types[entry.result] == BoolType()
	case KindRequirement:
		if types[entry.result] != BoolType() {
			return false
		}
		for _, binding := range entry.outputs {
			output := entries[binding.Output]
			if binding.Proof != entry.result || output == nil ||
				output.kind != KindOutput || types[binding.Register] != output.typ {
				return false
			}
		}
		return true
	case KindFamily:
		return types[entry.result] == BoolType()
	case KindForm:
		subject := entries[entry.subject]
		if subject == nil || subject.kind != KindOutput || subject.typ.Value != ValueRow ||
			subject.typ.Cardinality != CardinalityOne {
			return false
		}
		selected := false
		for _, instruction := range entry.program {
			if instruction.Op == OpSelection && instruction.Ref == entry.subject {
				selected = true
				break
			}
		}
		if !selected {
			return false
		}
		seen := make(map[uint16]struct{}, len(entry.emissions))
		for _, register := range entry.emissions {
			typ, ok := types[register]
			if !ok || typ.Value != ValueEmissionRange {
				return false
			}
			if _, duplicate := seen[register]; duplicate {
				return false
			}
			seen[register] = struct{}{}
		}
		return true
	default:
		return len(entry.program) == 0
	}
}

func cyclicRelations(entries map[schema.Key]*Entry) bool {
	return cyclic(entries, KindRelation, func(entry *Entry) []schema.Key {
		var dependencies []schema.Key
		for _, instruction := range entry.program {
			if instruction.Op == OpFollow {
				dependencies = append(dependencies, instruction.Ref)
			}
		}
		return dependencies
	})
}

func cyclicStages(entries map[schema.Key]*Entry) bool {
	return cyclic(entries, KindStage, func(entry *Entry) []schema.Key {
		dependencies := make([]schema.Key, 0, len(entry.edges)+len(entry.predecessors))
		for _, edge := range entry.edges {
			if edge.Source == StageEdgeSourceStage || edge.Source == StageEdgeSourceBeforeStage {
				dependencies = append(dependencies, edge.Stage)
			}
		}
		dependencies = append(dependencies, entry.predecessors...)
		return dependencies
	})
}

func cyclic(entries map[schema.Key]*Entry, kind Kind, dependencies func(*Entry) []schema.Key) bool {
	state := make(map[schema.Key]uint8)
	var visit func(schema.Key) bool
	visit = func(key schema.Key) bool {
		if state[key] == 1 {
			return true
		}
		if state[key] == 2 {
			return false
		}
		state[key] = 1
		for _, dependency := range dependencies(entries[key]) {
			if row := entries[dependency]; row != nil && row.kind == kind && visit(dependency) {
				return true
			}
		}
		state[key] = 2
		return false
	}
	for key, entry := range entries {
		if entry.kind == kind && visit(key) {
			return true
		}
	}
	return false
}
