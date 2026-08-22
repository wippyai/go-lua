// Package issuance interprets the sealed issuance machine over canonical
// Program rows. It switches only on machine opcodes; occurrence families,
// requirements, forms, inputs, and stages remain sealed declaration data.
package issuance

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

type Input struct {
	declaration *schemaissuance.Entry
	points      []identity.ContentID
}

func (input Input) Declaration() *schemaissuance.Entry { return input.declaration }
func (input Input) Points() []identity.ContentID {
	return append([]identity.ContentID(nil), input.points...)
}

// Request is one admitted, still-unmaterialized stage request. The scheduler
// owns the only transition from this instruction to a final point+input
// receipt; no RuleOccurrence exists yet and therefore none can be rewritten.
type Request struct {
	subscription schemaissuance.Subscription
	occurrence   uint32
	stage        *schemaissuance.Entry
	base         identity.ContentID
	parameters   []value
	input        Input
	route        identity.ContentID
	driverIndex  int
}

func (request Request) Subscription() schemaissuance.Subscription { return request.subscription }
func (request Request) Occurrence() uint32                        { return request.occurrence }
func (request Request) Stage() *schemaissuance.Entry              { return request.stage }
func (request Request) Base() identity.ContentID                  { return request.base }
func (request Request) Input() Input {
	return Input{declaration: request.input.declaration, points: request.input.Points()}
}
func (request Request) Route() (identity.ContentID, bool) {
	return request.route, request.route.Available()
}
func (request Request) DriverIndex() int { return request.driverIndex }

type value struct {
	typ      schemaissuance.DataType
	present  bool
	boolean  bool
	unsigned uint64
	identity identity.ContentID
	key      schema.Key
	row      programissuance.Row
	rows     []programissuance.Row
	points   []identity.ContentID
	input    Input
	requests []Request
	route    identity.ContentID
}

type context struct {
	plan         schemaissuance.Plan
	table        schemaissuance.Table
	rows         programissuance.Rows
	current      programissuance.Row
	item         programissuance.Row
	itemIndex    uint64
	selections   map[schema.Key]value
	subscription schemaissuance.Subscription
	occurrence   uint32
	arena        *registerArena
}

// registerArena is the register store one Evaluate owns. Execution is
// strictly nested -- a relation predicate runs to completion inside the
// instruction that reads it -- so register files obey stack discipline, and
// the whole evaluation needs one growable store rather than one heap
// allocation per evaluated row. Frames address the store by offset, never by
// retained slice, so growing it cannot invalidate an enclosing frame.
type registerArena struct {
	values []value
	set    []bool
}

// registerFrame is one execution's register file: a dense span of the arena.
// The machine numbers registers from one, and register zero is its "no
// operand" spelling, so a frame reads zero as absent rather than as a slot.
type registerFrame struct {
	arena *registerArena
	base  int
	width int
}

// open reserves one frame. The reserved span is zero on entry, which is what
// makes an unwritten register read as absent.
func (arena *registerArena) open(width int) (registerFrame, bool) {
	if arena == nil || width <= 0 {
		return registerFrame{}, false
	}
	base := len(arena.values)
	for index := 0; index < width; index++ {
		arena.values = append(arena.values, value{})
		arena.set = append(arena.set, false)
	}
	return registerFrame{arena: arena, base: base, width: width}, true
}

// close releases the frame and clears the span it occupied. Clearing is part
// of the release: a register can hold row, point, and request slices, and a
// reused frame must not keep the previous execution's storage reachable.
func (frame registerFrame) close() {
	if frame.arena == nil {
		return
	}
	for index := frame.base; index < frame.base+frame.width; index++ {
		frame.arena.values[index] = value{}
		frame.arena.set[index] = false
	}
	frame.arena.values = frame.arena.values[:frame.base]
	frame.arena.set = frame.arena.set[:frame.base]
}

func (frame registerFrame) read(id uint16) (value, bool) {
	if frame.arena == nil || id == 0 || int(id) >= frame.width || !frame.arena.set[frame.base+int(id)] {
		return value{}, false
	}
	return frame.arena.values[frame.base+int(id)], true
}

func (frame registerFrame) write(id uint16, written value) bool {
	if frame.arena == nil || int(id) >= frame.width {
		return false
	}
	frame.arena.values[frame.base+int(id)], frame.arena.set[frame.base+int(id)] = written, true
	return true
}

// Evaluate applies every sealed subscription to the canonical heterogeneous
// occurrence row space. Requests are returned in declaration order and then
// occurrence order, which is the final publication order.
func Evaluate(plan schemaissuance.Plan, rows programissuance.Rows) ([]Request, bool) {
	table := plan.Table()
	var arena registerArena
	var requests []Request
	for subscriptionIndex := 0; subscriptionIndex < plan.Count(); subscriptionIndex++ {
		subscription, subscriptionOK := plan.At(subscriptionIndex)
		if !subscriptionOK {
			return nil, false
		}
		family, requirement, form := subscription.Family(), subscription.Requirement(), subscription.Form()
		rowCount, supported := rows.Count(family.Space())
		if !supported {
			return nil, false
		}
		for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
			if uint64(rowIndex) > uint64(^uint32(0)) {
				return nil, false
			}
			current, currentOK := rows.At(family.Space(), rowIndex)
			if !currentOK {
				return nil, false
			}
			base := context{plan: plan, table: table, rows: rows, current: current, subscription: subscription, occurrence: uint32(rowIndex), arena: &arena}
			familyResult, familyOK := execute(family, base)
			if !familyOK || familyResult.typ != schemaissuance.BoolType() || !familyResult.present {
				return nil, false
			}
			if !familyResult.boolean {
				continue
			}
			admitted, selections, requirementOK := executeRequirement(requirement, base)
			if !requirementOK {
				return nil, false
			}
			if !admitted {
				continue
			}
			base.selections = selections
			formResult, formOK := execute(form, base)
			if !formOK {
				return nil, false
			}
			formRequests := formResult.requests
			if len(formRequests) == 0 && form.EmptyPolicy() == schemaissuance.EmptyRefuse {
				return nil, false
			}
			requests = append(requests, formRequests...)
		}
	}
	return requests, true
}

func executeRequirement(entry *schemaissuance.Entry, ctx context) (bool, map[schema.Key]value, bool) {
	registers, ok := executeRegisters(entry, ctx)
	if !ok {
		return false, nil, false
	}
	defer registers.close()
	result, resultOK := registers.read(entry.Result())
	if !resultOK || result.typ != schemaissuance.BoolType() || !result.present {
		return false, nil, false
	}
	if !result.boolean {
		return false, nil, true
	}
	selections := make(map[schema.Key]value, len(entry.Outputs()))
	for _, output := range entry.Outputs() {
		selected, selectedOK := registers.read(output.Register)
		proof, proofOK := registers.read(output.Proof)
		if !selectedOK || !selected.present || !proofOK || proof.typ != schemaissuance.BoolType() || !proof.boolean {
			return false, nil, false
		}
		selections[output.Output] = selected
	}
	return true, selections, true
}

func execute(entry *schemaissuance.Entry, ctx context) (value, bool) {
	registers, ok := executeRegisters(entry, ctx)
	if !ok {
		return value{}, false
	}
	defer registers.close()
	if entry.Kind() == schemaissuance.KindForm {
		var requests []Request
		for _, register := range entry.Emissions() {
			emission, present := registers.read(register)
			if !present || emission.typ.Value != schemaissuance.ValueEmissionRange {
				return value{}, false
			}
			requests = append(requests, emission.requests...)
		}
		return value{typ: schemaissuance.DataType{Value: schemaissuance.ValueEmissionRange, Name: schemaissuance.TypeEmission, Cardinality: schemaissuance.CardinalityMany}, present: true, requests: requests}, true
	}
	result, ok := registers.read(entry.Result())
	return result, ok
}

// executeRegisters runs one entry's sealed program into a fresh register
// frame. The frame belongs to the caller: it is returned live so the caller
// can read the entry's result and emissions out of it, and the caller closes
// it.
func executeRegisters(entry *schemaissuance.Entry, ctx context) (registerFrame, bool) {
	if entry == nil {
		return registerFrame{}, false
	}
	registers, opened := ctx.arena.open(entry.RegisterWidth())
	if !opened {
		return registerFrame{}, false
	}
	if !runProgram(entry, ctx, registers) {
		registers.close()
		return registerFrame{}, false
	}
	return registers, true
}

// runProgram is executeRegisters' whole interpretation step. It is separate
// so that a refusal anywhere in the opcode switch releases the frame through
// one path instead of one per opcode.
func runProgram(entry *schemaissuance.Entry, ctx context, registers registerFrame) bool {
	read := registers.read
	for programIndex := 0; programIndex < entry.ProgramLen(); programIndex++ {
		instruction, instructionOK := entry.InstructionAt(programIndex)
		if !instructionOK {
			return false
		}
		var output value
		switch instruction.Op {
		case schemaissuance.OpCurrent:
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: entry.Space(), Cardinality: schemaissuance.CardinalityOne}, present: true, row: ctx.current}
		case schemaissuance.OpItem:
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: entry.Target(), Cardinality: schemaissuance.CardinalityOne}, present: true, row: ctx.item}
		case schemaissuance.OpItemIndex:
			output = value{typ: schemaissuance.UintType(schemaissuance.TypeRelationIndex), present: true, unsigned: ctx.itemIndex}
		case schemaissuance.OpLiteral:
			output = value{typ: instruction.Type, present: true}
			if instruction.Type.Value == schemaissuance.ValueBool {
				output.boolean = instruction.Literal != 0
			} else if instruction.Type.Value == schemaissuance.ValueUint {
				output.unsigned = instruction.Literal
			} else {
				return false
			}
		case schemaissuance.OpRead:
			row, rowOK := read(instruction.Args[0])
			field, fieldOK := ctx.table.Entry(instruction.Ref, schemaissuance.KindField)
			if !rowOK || !fieldOK {
				return false
			}
			output.typ = field.Type()
			output.typ.Cardinality = field.Cardinality()
			if !row.present {
				if field.Cardinality() != schemaissuance.CardinalityOptional {
					return false
				}
				break
			}
			scalar, present := ctx.rows.Read(row.row, field.Key())
			if !present {
				if field.Cardinality() != schemaissuance.CardinalityOptional {
					return false
				}
				break
			}
			converted, convertedOK := scalarValue(output.typ, scalar)
			if !convertedOK {
				return false
			}
			output = converted
		case schemaissuance.OpFollow:
			source, sourceOK := read(instruction.Args[0])
			relation, relationOK := ctx.table.Entry(instruction.Ref, schemaissuance.KindRelation)
			if !sourceOK || !source.present || !relationOK {
				return false
			}
			candidates, candidatesOK := ctx.rows.Follow(source.row, relation)
			if !candidatesOK {
				return false
			}
			selected := make([]programissuance.Row, 0, len(candidates))
			for index, candidate := range candidates {
				relationContext := ctx
				relationContext.current, relationContext.item, relationContext.itemIndex = source.row, candidate, uint64(index)
				result, resultOK := execute(relation, relationContext)
				if !resultOK || result.typ != schemaissuance.BoolType() || !result.present {
					return false
				}
				if result.boolean {
					selected = append(selected, candidate)
				}
			}
			if !cardinalityHolds(relation.Cardinality(), len(selected)) {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueRange, Space: relation.Target(), Relation: relation.Key(), Cardinality: relation.Cardinality()}, present: true, rows: selected}
		case schemaissuance.OpAt:
			rangeValue, rangeOK := read(instruction.Args[0])
			indexValue, indexOK := read(instruction.Args[1])
			output.typ = schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: rangeValue.typ.Space, Cardinality: schemaissuance.CardinalityOptional}
			if !rangeOK || !indexOK || !rangeValue.present || !indexValue.present || indexValue.unsigned >= uint64(len(rangeValue.rows)) {
				if rangeOK && indexOK && rangeValue.present && indexValue.present {
					break
				}
				return false
			}
			output.present, output.row = true, rangeValue.rows[indexValue.unsigned]
		case schemaissuance.OpCount:
			rangeValue, ok := read(instruction.Args[0])
			if !ok || !rangeValue.present {
				return false
			}
			output = value{typ: schemaissuance.UintType(schemaissuance.TypeRelationCount), present: true, unsigned: uint64(len(rangeValue.rows))}
		case schemaissuance.OpEqual, schemaissuance.OpEqualIfPresent, schemaissuance.OpGreater, schemaissuance.OpLess, schemaissuance.OpAnd, schemaissuance.OpOr:
			left, leftOK := read(instruction.Args[0])
			right, rightOK := read(instruction.Args[1])
			if !leftOK || !rightOK {
				return false
			}
			output = value{typ: schemaissuance.BoolType(), present: true}
			switch instruction.Op {
			case schemaissuance.OpEqual:
				output.boolean = left.present && right.present && equalValue(left, right)
			case schemaissuance.OpEqualIfPresent:
				output.boolean = !left.present || right.present && equalValue(left, right)
			case schemaissuance.OpGreater:
				output.boolean = left.present && right.present && left.unsigned > right.unsigned
			case schemaissuance.OpLess:
				output.boolean = left.present && right.present && left.unsigned < right.unsigned
			case schemaissuance.OpAnd:
				output.boolean = left.present && right.present && left.boolean && right.boolean
			case schemaissuance.OpOr:
				output.boolean = left.present && right.present && (left.boolean || right.boolean)
			}
		case schemaissuance.OpNot, schemaissuance.OpPresent, schemaissuance.OpExactlyOne:
			left, ok := read(instruction.Args[0])
			if !ok {
				return false
			}
			output = value{typ: schemaissuance.BoolType(), present: true}
			switch instruction.Op {
			case schemaissuance.OpNot:
				output.boolean = left.present && !left.boolean
			case schemaissuance.OpPresent:
				output.boolean = left.present
			case schemaissuance.OpExactlyOne:
				output.boolean = left.present && len(left.rows) == 1
			}
		case schemaissuance.OpOnly:
			rangeValue, rangeOK := read(instruction.Args[0])
			proof, proofOK := read(instruction.Args[1])
			if !rangeOK || !proofOK || !rangeValue.present || proof.typ != schemaissuance.BoolType() {
				return false
			}
			// ExactlyOne is a proof guard, not a totality obligation. A relation
			// with no candidate is a normal non-admission (for example a fixed
			// Values member against the tail-transfer requirement); OpOnly must
			// publish an absent optional row so the enclosing conjunction can
			// evaluate false rather than refusing the entire occurrence plane.
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueRow, Space: rangeValue.typ.Space, Cardinality: schemaissuance.CardinalityOptional}}
			if !proof.present || !proof.boolean || len(rangeValue.rows) != 1 {
				break
			}
			output.typ.Cardinality = schemaissuance.CardinalityOne
			output.present, output.row = true, rangeValue.rows[0]
		case schemaissuance.OpRequirePresent:
			candidate, candidateOK := read(instruction.Args[0])
			proof, proofOK := read(instruction.Args[1])
			if !candidateOK || !proofOK || proof.typ != schemaissuance.BoolType() || !proof.present {
				return false
			}
			output = candidate
			if !proof.boolean {
				output.present = false
				output.typ.Cardinality = schemaissuance.CardinalityOptional
				break
			}
			if !candidate.present {
				return false
			}
			output.typ.Cardinality = schemaissuance.CardinalityOne
		case schemaissuance.OpSelection:
			selected, ok := ctx.selections[instruction.Ref]
			if !ok || !selected.present {
				return false
			}
			output = selected
		case schemaissuance.OpRuleKey:
			output = value{typ: schemaissuance.IdentityType(schemaissuance.TypeRuleKey), present: true, key: ctx.subscription.Rule()}
		case schemaissuance.OpWritesKey:
			output = value{typ: schemaissuance.IdentityType(schemaissuance.TypeAxisKey), present: true, key: ctx.subscription.Writes()}
		case schemaissuance.OpProjectPoints:
			rangeValue, ok := read(instruction.Args[0])
			if !ok || !rangeValue.present {
				return false
			}
			points, pointsOK := projectPoints(ctx.rows, rangeValue.rows, instruction.Ref, instruction.Aux)
			if !pointsOK {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityMany}, present: true, points: points}
		case schemaissuance.OpPoint:
			point, ok := read(instruction.Args[0])
			if !ok || !point.present || !point.identity.Available() {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityOne}, present: true, points: []identity.ContentID{point.identity}}
		case schemaissuance.OpRoute:
			route, ok := read(instruction.Args[0])
			if !ok || !route.present || !route.identity.Available() {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueRoute, Name: schemaissuance.TypeRoute, Cardinality: schemaissuance.CardinalityOne}, present: true, route: route.identity}
		case schemaissuance.OpInput:
			declaration, ok := ctx.table.Entry(instruction.Ref, schemaissuance.KindInput)
			if !ok {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueInputRange, Name: declaration.Key(), Cardinality: schemaissuance.CardinalityMany}, present: true, input: Input{declaration: declaration}}
			if instruction.Args[0] != 0 {
				points, pointsOK := read(instruction.Args[0])
				if !pointsOK || !points.present {
					return false
				}
				output.input.points = append([]identity.ContentID(nil), points.points...)
			}
		case schemaissuance.OpRequestStage:
			stage, ok := ctx.table.Entry(instruction.Ref, schemaissuance.KindStage)
			if !ok {
				return false
			}
			parameters := stage.Parameters()
			arguments := make([]value, len(parameters))
			for index := range parameters {
				argument, argumentOK := read(instruction.Args[index])
				if !argumentOK || !argument.present {
					return false
				}
				arguments[index] = argument
			}
			inputValue, inputOK := read(instruction.Args[len(parameters)])
			baseIndex := int(stage.BaseParameter()) - 1
			if !inputOK || !inputValue.present || inputValue.input.declaration == nil ||
				baseIndex < 0 || baseIndex >= len(arguments) {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueStageRequestRange, Name: stage.Key(), Cardinality: schemaissuance.CardinalityMany}, present: true}
			for pointIndex, point := range arguments[baseIndex].points {
				requestArguments := append([]value(nil), arguments...)
				requestArguments[baseIndex].points = []identity.ContentID{point}
				output.requests = append(output.requests, Request{
					subscription: ctx.subscription, occurrence: ctx.occurrence, stage: stage, base: point,
					parameters: requestArguments, input: inputValue.input, driverIndex: pointIndex,
				})
			}
		case schemaissuance.OpEmit:
			requests, ok := read(instruction.Args[0])
			if !ok || !requests.present {
				return false
			}
			output = value{typ: schemaissuance.DataType{Value: schemaissuance.ValueEmissionRange, Name: schemaissuance.TypeEmission, Cardinality: schemaissuance.CardinalityMany}, present: true, requests: append([]Request(nil), requests.requests...)}
			if instruction.Args[1] != 0 {
				route, routeOK := read(instruction.Args[1])
				if !routeOK || !route.present || !route.route.Available() {
					return false
				}
				for index := range output.requests {
					output.requests[index].route = route.route
				}
			}
		default:
			return false
		}
		if !registers.write(instruction.Out, output) {
			return false
		}
	}
	return true
}

func scalarValue(typ schemaissuance.DataType, scalar programissuance.Scalar) (value, bool) {
	result := value{typ: typ, present: true}
	switch typ.Value {
	case schemaissuance.ValueBool:
		if scalar.Kind != programissuance.ScalarBool {
			return value{}, false
		}
		result.boolean = scalar.Bool
	case schemaissuance.ValueUint:
		if scalar.Kind != programissuance.ScalarUint || scalar.Type != typ.Name {
			return value{}, false
		}
		result.unsigned = scalar.Uint
	case schemaissuance.ValueIdentity:
		if scalar.Kind != programissuance.ScalarIdentity || scalar.Type != typ.Name {
			return value{}, false
		}
		result.identity = scalar.Identity
	default:
		return value{}, false
	}
	return result, true
}

func equalValue(left, right value) bool {
	if left.typ.Value != right.typ.Value {
		return false
	}
	switch left.typ.Value {
	case schemaissuance.ValueBool:
		return left.boolean == right.boolean
	case schemaissuance.ValueUint:
		return left.unsigned == right.unsigned
	case schemaissuance.ValueIdentity:
		if left.typ.Name == schemaissuance.TypeRuleKey || left.typ.Name == schemaissuance.TypeAxisKey {
			return left.key == right.key
		}
		return left.identity == right.identity
	case schemaissuance.ValueRow:
		return left.row == right.row
	default:
		return false
	}
}

func cardinalityHolds(cardinality schemaissuance.Cardinality, count int) bool {
	switch cardinality {
	case schemaissuance.CardinalityOptional:
		return count <= 1
	case schemaissuance.CardinalityOne:
		return count == 1
	case schemaissuance.CardinalityMany:
		return true
	default:
		return false
	}
}

func projectPoints(rows programissuance.Rows, source []programissuance.Row, pointField, orderField schema.Key) ([]identity.ContentID, bool) {
	type orderedPoint struct {
		position uint64
		point    identity.ContentID
	}
	ordered := make([]orderedPoint, 0, len(source))
	seenPositions := make(map[uint64]struct{}, len(source))
	seenPoints := make(map[identity.ContentID]struct{}, len(source))
	for _, row := range source {
		point, pointOK := rows.Read(row, pointField)
		position, positionOK := rows.Read(row, orderField)
		if !pointOK || !positionOK || point.Kind != programissuance.ScalarIdentity || position.Kind != programissuance.ScalarUint || !point.Identity.Available() {
			return nil, false
		}
		if _, duplicate := seenPositions[position.Uint]; duplicate {
			return nil, false
		}
		if _, duplicate := seenPoints[point.Identity]; duplicate {
			return nil, false
		}
		seenPositions[position.Uint] = struct{}{}
		seenPoints[point.Identity] = struct{}{}
		ordered = append(ordered, orderedPoint{position: position.Uint, point: point.Identity})
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].position < ordered[right].position })
	points := make([]identity.ContentID, len(ordered))
	for index, point := range ordered {
		if point.position != uint64(index) {
			return nil, false
		}
		points[index] = point.point
	}
	return points, true
}
