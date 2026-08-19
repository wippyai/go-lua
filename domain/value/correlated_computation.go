package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

type computationKey struct {
	module     identity.ContentID
	occurrence identity.ContentID
}

type selectBranchKey struct {
	computationKey
	branch uint8
}

// BinaryEquality is Value's owner-fenced interpretation of one reusable
// Program equality computation. It retains only the exact mounted
// coordinates and the equality polarity; authored terms and Flow geometry
// remain behind ProgramArtifact.
type BinaryEquality struct {
	schema              *Schema
	key                 computationKey
	content             identity.ContentID
	result, left, right Coordinate
	notEqual            bool
}

// BinaryArithmetic is Value's owner-fenced interpretation of one reusable
// Program primitive arithmetic transfer.  Program owns the operator and
// occurrence geometry; Value owns only the mounted coordinates and abstract
// result relation.
type BinaryArithmetic struct {
	schema              *Schema
	key                 computationKey
	content             identity.ContentID
	result, left, right Coordinate
	op                  flowkind.BinaryOp
}

func (schema *Schema) BinaryArithmetic(module, occurrence identity.ContentID) (BinaryArithmetic, bool) {
	if schema == nil || schema.binaryArithmetics == nil {
		return BinaryArithmetic{}, false
	}
	row, ok := schema.binaryArithmetics[computationKey{module: module, occurrence: occurrence}]
	return row, ok && row.valid()
}

func (row BinaryArithmetic) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() &&
		row.content.Available() && flowkind.IsBinaryArithmetic(row.op)
}

func (schema *Schema) OwnsBinaryArithmetic(row BinaryArithmetic) bool {
	return schema != nil && row.schema == schema && row.valid()
}

func (row BinaryArithmetic) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

func (row BinaryArithmetic) Endpoints() (result, left, right Coordinate, op flowkind.BinaryOp, ok bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, Coordinate{}, 0, false
	}
	return row.result, row.left, row.right, row.op, true
}

// BinaryOrder is Value's owner-fenced interpretation of one reusable Program
// relational-order computation. It retains only exact mounted coordinates and
// the closed Lua operator; authored terms and Flow geometry remain behind the
// ProgramArtifact receipt.
type BinaryOrder struct {
	schema              *Schema
	key                 computationKey
	content             identity.ContentID
	result, left, right Coordinate
	op                  flowkind.BinaryOp
}

func (schema *Schema) BinaryOrder(module, occurrence identity.ContentID) (BinaryOrder, bool) {
	if schema == nil || schema.binaryOrders == nil {
		return BinaryOrder{}, false
	}
	row, ok := schema.binaryOrders[computationKey{module: module, occurrence: occurrence}]
	return row, ok && row.valid()
}

func (row BinaryOrder) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() &&
		row.content.Available() && flowkind.IsBinaryOrder(row.op)
}

func (schema *Schema) OwnsBinaryOrder(row BinaryOrder) bool {
	return schema != nil && row.schema == schema && row.valid()
}

func (row BinaryOrder) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

func (row BinaryOrder) Endpoints() (result, left, right Coordinate, op flowkind.BinaryOp, ok bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, Coordinate{}, 0, false
	}
	return row.result, row.left, row.right, row.op, true
}

// PresenceRefinement is Value's owner-fenced interpretation of one exact
// reusable nil-comparison arm. It names only the mounted storage coordinate
// and the closed presence conclusion; Program branch/route geometry remains
// behind the artifact receipt that issued this operand.
type PresenceRefinement struct {
	schema  *Schema
	key     computationKey
	content identity.ContentID
	target  Coordinate
	present bool
}

func (schema *Schema) PresenceRefinement(module, occurrence identity.ContentID) (PresenceRefinement, bool) {
	if schema == nil || schema.presenceRefinements == nil {
		return PresenceRefinement{}, false
	}
	row, ok := schema.presenceRefinements[computationKey{module: module, occurrence: occurrence}]
	return row, ok && row.valid()
}

func (row PresenceRefinement) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() && row.content.Available()
}

func (schema *Schema) OwnsPresenceRefinement(row PresenceRefinement) bool {
	return schema != nil && row.schema == schema && row.valid()
}

func (row PresenceRefinement) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

func (row PresenceRefinement) Target() (Coordinate, bool, bool) {
	if !row.valid() {
		return Coordinate{}, false, false
	}
	return row.target, row.present, true
}

func (schema *Schema) BinaryEquality(module, occurrence identity.ContentID) (BinaryEquality, bool) {
	if schema == nil || schema.binaryEqualities == nil {
		return BinaryEquality{}, false
	}
	row, ok := schema.binaryEqualities[computationKey{module: module, occurrence: occurrence}]
	return row, ok && row.valid()
}

func (row BinaryEquality) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() && row.content.Available()
}

func (schema *Schema) OwnsBinaryEquality(row BinaryEquality) bool {
	return schema != nil && row.schema == schema && row.valid()
}

func (row BinaryEquality) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

func (row BinaryEquality) Endpoints() (result, left, right Coordinate, notEqual bool, ok bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, Coordinate{}, false, false
	}
	return row.result, row.left, row.right, row.notEqual, true
}

type UnaryNot struct {
	schema                              *Schema
	key                                 computationKey
	content                             identity.ContentID
	resultCoordinate, operandCoordinate Coordinate
}

func (schema *Schema) UnaryNot(module, occurrence identity.ContentID) (UnaryNot, bool) {
	if schema == nil || schema.unaryNots == nil {
		return UnaryNot{}, false
	}
	row, ok := schema.unaryNots[computationKey{module, occurrence}]
	return row, ok && row.valid()
}

func (row UnaryNot) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() && row.content.Available()
}
func (schema *Schema) OwnsUnaryNot(row UnaryNot) bool {
	return schema != nil && row.schema == schema && row.valid()
}
func (row UnaryNot) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}
func (row UnaryNot) Endpoints() (Coordinate, Coordinate, bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	return row.resultCoordinate, row.operandCoordinate, true
}

type SelectBranch struct {
	schema               *Schema
	key                  computationKey
	content              identity.ContentID
	branch               uint8
	truthy, chosenIsLeft bool
	result, left, chosen Coordinate
}

func (schema *Schema) SelectBranch(module, occurrence identity.ContentID, branch int) (SelectBranch, bool) {
	if schema == nil || schema.selectBranches == nil || branch < 0 || branch > 1 {
		return SelectBranch{}, false
	}
	row, ok := schema.selectBranches[selectBranchKey{computationKey{module, occurrence}, uint8(branch)}]
	return row, ok && row.valid()
}
func (row SelectBranch) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() && row.content.Available() && row.branch <= 1
}
func (schema *Schema) OwnsSelectBranch(row SelectBranch) bool {
	return schema != nil && row.schema == schema && row.valid()
}
func (row SelectBranch) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}
func (row SelectBranch) Endpoints() (result, left, chosen Coordinate, truthy, chosenIsLeft bool, ok bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, Coordinate{}, false, false, false
	}
	return row.result, row.left, row.chosen, row.truthy, row.chosenIsLeft, true
}

type ValueClaim struct {
	schema          *Schema
	key             computationKey
	content         identity.ContentID
	result, operand Coordinate
	kind            flowkind.ValueClaimKind
}

func (schema *Schema) ValueClaim(module, occurrence identity.ContentID) (ValueClaim, bool) {
	if schema == nil || schema.valueClaims == nil {
		return ValueClaim{}, false
	}
	row, ok := schema.valueClaims[computationKey{module, occurrence}]
	return row, ok && row.valid()
}
func (row ValueClaim) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() && row.content.Available()
}
func (schema *Schema) OwnsValueClaim(row ValueClaim) bool {
	return schema != nil && row.schema == schema && row.valid()
}
func (row ValueClaim) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}
func (row ValueClaim) Endpoints() (Coordinate, Coordinate, bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	return row.result, row.operand, true
}
func (row ValueClaim) Kind() (flowkind.ValueClaimKind, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.kind, true
}

func (schema *valueBuilder) sealComputationRows() bool {
	if schema == nil || schema.sealProject() == nil || schema.artifacts == nil {
		return false
	}
	for module, mount := range schema.artifacts {
		artifact := mount.Snapshot()
		if artifact == nil {
			return false
		}
		for index := 0; index < artifact.OccurrenceCount(); index++ {
			row, ok := artifact.OccurrenceAt(index)
			if !ok {
				return false
			}
			key := computationKey{module, row.ID()}
			switch programartifact.OccurrenceKind(row.Kind()) {
			case programartifact.OccurrenceBinaryArithmetic:
				leftID, rightID, op, rowOK := row.BinaryArithmetic()
				result, resultOK := schema.sealBoundary().Values().ForMountedSpan(module, row.ID())
				left, leftOK := schema.sealBoundary().Values().ForMountedSpan(module, leftID)
				right, rightOK := schema.sealBoundary().Values().ForMountedSpan(module, rightID)
				rc, rcOK := schema.coordinateForCold(result)
				lc, lcOK := schema.coordinateForCold(left)
				rr, rrOK := schema.coordinateForCold(right)
				if !rowOK || !resultOK || !leftOK || !rightOK || !rcOK || !lcOK || !rrOK || !flowkind.IsBinaryArithmetic(op) {
					return false
				}
				content := computationContent(schema.linkID, "val-arithmetic!", module, row.ID(), row.Code())
				arithmetic := BinaryArithmetic{schema: schema.Schema, key: key, content: content, result: rc, left: lc, right: rr, op: op}
				if !arithmetic.valid() {
					return false
				}
				if _, duplicate := schema.binaryArithmetics[key]; duplicate {
					return false
				}
				schema.binaryArithmetics[key] = arithmetic
			case programartifact.OccurrenceBinaryEquality:
				leftID, rightID, op, rowOK := row.BinaryEquality()
				if !rowOK {
					return false
				}
				result, resultOK := schema.sealBoundary().Values().ForMountedSpan(module, row.ID())
				left, leftOK := schema.sealBoundary().Values().ForMountedSpan(module, leftID)
				right, rightOK := schema.sealBoundary().Values().ForMountedSpan(module, rightID)
				rc, rcOK := schema.coordinateForCold(result)
				lc, lcOK := schema.coordinateForCold(left)
				rr, rrOK := schema.coordinateForCold(right)
				if !resultOK || !leftOK || !rightOK || !rcOK || !lcOK || !rrOK {
					return false
				}
				notEqual := op == flowkind.BinaryNotEqual
				content := computationContent(schema.linkID, "val-eq!", module, row.ID(), row.Code())
				binary := BinaryEquality{schema: schema.Schema, key: key, content: content, result: rc, left: lc, right: rr, notEqual: notEqual}
				if binary.valid() {
					if _, duplicate := schema.binaryEqualities[key]; duplicate {
						return false
					}
					schema.binaryEqualities[key] = binary
				} else {
					return false
				}
			case programartifact.OccurrenceBinaryOrder:
				leftID, rightID, op, rowOK := row.BinaryOrder()
				result, resultOK := schema.sealBoundary().Values().ForMountedSpan(module, row.ID())
				left, leftOK := schema.sealBoundary().Values().ForMountedSpan(module, leftID)
				right, rightOK := schema.sealBoundary().Values().ForMountedSpan(module, rightID)
				rc, rcOK := schema.coordinateForCold(result)
				lc, lcOK := schema.coordinateForCold(left)
				rr, rrOK := schema.coordinateForCold(right)
				if !rowOK || !resultOK || !leftOK || !rightOK || !rcOK || !lcOK || !rrOK || !flowkind.IsBinaryOrder(op) {
					return false
				}
				content := computationContent(schema.linkID, "val-order!", module, row.ID(), row.Code())
				order := BinaryOrder{schema: schema.Schema, key: key, content: content, result: rc, left: lc, right: rr, op: op}
				if order.valid() {
					if _, duplicate := schema.binaryOrders[key]; duplicate {
						return false
					}
					schema.binaryOrders[key] = order
				} else {
					return false
				}
			case programartifact.OccurrenceCall:
				// The runtime-kind rule consumes only the sealed geometry of a
				// strict unary plain call. Join the occurrence to the existing
				// ingress Call directory by its parent-issued ID; do not infer
				// call shape from occurrence inputs or reconstruct Program data.
				call, callOK := artifact.CallForID(row.ID())
				if !callOK {
					return false
				}
				// Calls outside the strict unary plain shape are valid calls,
				// but are not RuntimeKindCall operands. Their own Call domain
				// rules continue to interpret them.
				if call.Form() == uint8(programartifact.CallFormMethod) || call.ArgumentCount() != 1 {
					continue
				}
				if call.Form() != uint8(programartifact.CallFormPlain) {
					return false
				}
				if _, hasReceiver := call.ReceiverID(); hasReceiver {
					continue
				}
				if _, hasTail := call.TailID(); hasTail {
					continue
				}
				argument, argumentOK := call.ArgumentAt(0)
				if !argumentOK || !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 || !argument.MemberID().Available() {
					return false
				}
				result, resultOK := schema.sealBoundary().Values().ForMountedSpan(module, call.SpanID())
				input, inputOK := schema.sealBoundary().Values().ForMountedSemantic(module, argument.MemberID())
				rc, rcOK := schema.coordinateForCold(result)
				ic, icOK := schema.coordinateForCold(input)
				if !resultOK || !inputOK || !rcOK || !icOK {
					return false
				}
				content := computationContent(schema.linkID, "val-runtime-kind-call!", module, row.ID())
				runtimeCall := RuntimeKindCall{schema: schema.Schema, key: key, content: content, result: rc, input: ic, comparison: ic, write: rc}
				if !runtimeCall.valid() {
					return false
				}
				if _, duplicate := schema.runtimeKindCalls[key]; duplicate {
					return false
				}
				schema.runtimeKindCalls[key] = runtimeCall
			case programartifact.OccurrenceOperationPredicateRefinement:
				sourceCallID, targetID, operandID, routeID, opCode, truth, rowOK := row.OperationPredicateRefinement()
				op := flowkind.BinaryOp(opCode)
				call, callOK := artifact.CallForID(sourceCallID)
				if !rowOK || !routeID.Available() || !callOK || call.ID() != sourceCallID ||
					call.Form() != uint8(programartifact.CallFormPlain) || call.ArgumentCount() != 1 {
					return false
				}
				if _, hasReceiver := call.ReceiverID(); hasReceiver {
					return false
				}
				if _, hasTail := call.TailID(); hasTail {
					return false
				}
				argument, argumentOK := call.ArgumentAt(0)
				if !argumentOK || !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 || argument.MemberID() != targetID {
					return false
				}
				result, resultOK := schema.sealBoundary().Values().ForMountedSpan(module, call.SpanID())
				input, inputOK := schema.sealBoundary().Values().ForMountedSemantic(module, targetID)
				// Program issues the compared operand as a value-subject span
				// identity, the same identity the BinaryEquality row carries,
				// so Boundary's mounted span directory is its total inverse.
				// The semantic directory keys parent-issued occurrence IDs and
				// names an operand span only when another row published it.
				comparison, comparisonOK := schema.sealBoundary().Values().ForMountedSpan(module, operandID)
				rc, rcOK := schema.coordinateForCold(result)
				ic, icOK := schema.coordinateForCold(input)
				pc, pcOK := schema.coordinateForCold(comparison)
				if !resultOK || !inputOK || !comparisonOK || !rcOK || !icOK || !pcOK ||
					(op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual) {
					return false
				}
				truthCode := uint64(0)
				if truth {
					truthCode = 1
				}
				content := computationContent(schema.linkID, "val-runtime-kind-predicate!", module, row.ID(), uint64(op), truthCode)
				runtimeCall := RuntimeKindCall{schema: schema.Schema, key: key, content: content, result: rc, input: ic, comparison: pc, write: ic, call: sourceCallID, op: op, truth: truth, refinement: true}
				if !runtimeCall.valid() {
					return false
				}
				if _, duplicate := schema.runtimeKindCalls[key]; duplicate {
					return false
				}
				schema.runtimeKindCalls[key] = runtimeCall
			case programartifact.OccurrenceBinaryPresenceRefinement:
				_, targetID, _, _, present, rowOK := row.BinaryPresenceRefinement()
				target, targetOK := schema.sealBoundary().Values().ForMountedSemantic(module, targetID)
				coordinate, coordinateOK := schema.coordinateForCold(target)
				if !rowOK || !targetOK || !coordinateOK {
					return false
				}
				content := computationContent(schema.linkID, "val-presence-refine!", module, row.ID(), row.Code())
				refinement := PresenceRefinement{schema: schema.Schema, key: key, content: content, target: coordinate, present: present}
				if !refinement.valid() {
					return false
				}
				if _, duplicate := schema.presenceRefinements[key]; duplicate {
					return false
				}
				schema.presenceRefinements[key] = refinement
			case programartifact.OccurrenceUnary:
				if row.Code() != uint64(flowkind.UnaryNot) || row.InputCount() != 1 {
					continue
				}
				operandID, ok := row.InputAt(0)
				if !ok {
					return false
				}
				result, resultOK := schema.sealBoundary().Values().ForMountedSemantic(module, row.ID())
				operand, operandOK := schema.sealBoundary().Values().ForMountedSemantic(module, operandID)
				rc, rcOK := schema.coordinateForCold(result)
				oc, ocOK := schema.coordinateForCold(operand)
				if !resultOK || !operandOK || !rcOK || !ocOK {
					return false
				}
				content := computationContent(schema.linkID, "val-not!", module, row.ID())
				schema.unaryNots[key] = UnaryNot{schema: schema.Schema, key: key, content: content, resultCoordinate: rc, operandCoordinate: oc}
			case programartifact.OccurrenceSelect:
				if row.InputCount() != 2 || (row.Code() != uint64(flowkind.SelectAnd) && row.Code() != uint64(flowkind.SelectOr)) {
					continue
				}
				leftID, leftOK := row.InputAt(0)
				rightID, rightOK := row.InputAt(1)
				if !leftOK || !rightOK {
					return false
				}
				result, resultOK := schema.sealBoundary().Values().ForMountedSemantic(module, row.ID())
				left, leftOK := schema.sealBoundary().Values().ForMountedSemantic(module, leftID)
				right, rightOK := schema.sealBoundary().Values().ForMountedSemantic(module, rightID)
				rc, rcOK := schema.coordinateForCold(result)
				lc, lcOK := schema.coordinateForCold(left)
				rr, rrOK := schema.coordinateForCold(right)
				if !resultOK || !leftOK || !rightOK || !rcOK || !lcOK || !rrOK {
					return false
				}
				for branch := 0; branch < 2; branch++ {
					truthy, chosenLeft := false, false
					chosen := rr
					if row.Code() == uint64(flowkind.SelectAnd) {
						if branch == 0 {
							truthy, chosen = true, rr
						} else {
							chosenLeft, chosen = true, lc
						}
					} else if branch == 0 {
						truthy, chosenLeft, chosen = true, true, lc
					}
					schema.selectBranches[selectBranchKey{key, uint8(branch)}] = SelectBranch{schema: schema.Schema, key: key, content: computationContent(schema.linkID, "val-sel!", module, row.ID(), uint64(branch)), branch: uint8(branch), truthy: truthy, chosenIsLeft: chosenLeft, result: rc, left: lc, chosen: chosen}
				}
			case programartifact.OccurrenceValueClaim:
				if row.InputCount() != 1 {
					continue
				}
				operandID, ok := row.InputAt(0)
				if !ok {
					return false
				}
				result, resultOK := schema.sealBoundary().Values().ForMountedSemantic(module, row.ID())
				operand, operandOK := schema.sealBoundary().Values().ForMountedSemantic(module, operandID)
				rc, rcOK := schema.coordinateForCold(result)
				oc, ocOK := schema.coordinateForCold(operand)
				if !resultOK || !operandOK || !rcOK || !ocOK {
					return false
				}
				schema.valueClaims[key] = ValueClaim{schema: schema.Schema, key: key, content: computationContent(schema.linkID, "val-clm!", module, row.ID()), result: rc, operand: oc, kind: flowkind.ValueClaimKind(row.Code())}
			case programartifact.OccurrenceReturnBoundary:
				if row.InputCount() != 1 {
					continue
				}
				valuesID, ok := row.InputAt(0)
				if !ok {
					return false
				}
				value, valueOK := schema.sealBoundary().Values().ForMountedSemantic(module, valuesID)
				coordinate, coordinateOK := schema.coordinateForCold(value)
				if !valueOK || !coordinateOK {
					return false
				}
				schema.returnBoundaries[key] = ReturnBoundary{schema: schema.Schema, key: key, content: computationContent(schema.linkID, "val-ret!", module, row.ID()), values: coordinate}
			}
		}
	}
	return true
}

func computationContent(linkID identity.ContentID, label string, module, occurrence identity.ContentID, extra ...uint64) identity.ContentID {
	h := sha256.New()
	h.Write(linkID[:])
	h.Write([]byte(label))
	h.Write(module[:])
	h.Write(occurrence[:])
	for _, value := range extra {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], value)
		h.Write(b[:])
	}
	var result identity.ContentID
	copy(result[:], h.Sum(nil))
	return result
}
