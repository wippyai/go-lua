package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/value/arithmetic/resultpolicy"
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
	endpoints           uint32
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
	endpoints           uint32
	op                  flowkind.BinaryOp
	// policy is Program's occurrence-scoped statement about this expression's
	// exact result image. It is carried on the row because the image is a
	// property of this occurrence, not of the operator or of the schema.
	policy resultpolicy.Policy
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
		row.content.Available() && flowkind.IsBinaryArithmetic(row.op) && row.policy.Available()
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

// Op is the closed Lua operator this transfer applies. The coordinates it
// used to be returned beside are read from the sealed endpoint projection.
func (row BinaryArithmetic) Op() (flowkind.BinaryOp, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.op, true
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
	endpoints           uint32
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

// Op is the closed Lua relational operator this comparison applies.
func (row BinaryOrder) Op() (flowkind.BinaryOp, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.op, true
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
	schema    *Schema
	key       computationKey
	content   identity.ContentID
	target    Coordinate
	endpoints uint32
	present   bool
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

// Present is the closed presence conclusion this arm narrows to.
func (row PresenceRefinement) Present() (bool, bool) {
	if !row.valid() {
		return false, false
	}
	return row.present, true
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

// NotEqual is the equality polarity this comparison publishes.
func (row BinaryEquality) NotEqual() (bool, bool) {
	if !row.valid() {
		return false, false
	}
	return row.notEqual, true
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

// returnBoundaryTopology is the seal-time copy of one Program Values
// topology. The child occurrence rows are the canonical member/tail source;
// ValuesFamily is deliberately not reopened here and no Program row escapes
// into Value's published Schema.
type returnBoundaryTopology struct {
	members  []identity.ContentID
	hasTail  bool
	tailKind programschema.ValuesTailKind
}

type returnBoundaryTopologyDraft struct {
	body        identity.ContentID
	memberByPos map[uint64]identity.ContentID
	hasTail     bool
	tailKind    programschema.ValuesTailKind
}

// sealReturnBoundaryTopologies indexes every Values root and its canonical
// OccurrenceValuesMember/Tail children once for one Program. ReturnBoundary
// sealing then performs an O(1) root lookup instead of rescanning the entire
// occurrence plane for every executable return.
func sealReturnBoundaryTopologies(program programschema.Program) (map[identity.ContentID]returnBoundaryTopology, bool) {
	if !program.Available() {
		return nil, false
	}
	count, countOK := program.OccurrenceCount()
	if !countOK {
		return nil, false
	}
	drafts := make(map[identity.ContentID]*returnBoundaryTopologyDraft)
	for index := 0; index < count; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK {
			return nil, false
		}
		if row.Kind() != programschema.OccurrenceValues {
			continue
		}
		id := row.ID()
		body, bodyOK := row.BodyID()
		if !id.Available() || !bodyOK {
			return nil, false
		}
		if _, duplicate := drafts[id]; duplicate {
			return nil, false
		}
		drafts[id] = &returnBoundaryTopologyDraft{body: body, memberByPos: make(map[uint64]identity.ContentID)}
	}
	for index := 0; index < count; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK {
			return nil, false
		}
		switch row.Kind() {
		case programschema.OccurrenceValuesMember:
			rootID, rootOK := program.OccurrenceInputID(index, 0)
			memberSpanID, memberOK := program.OccurrenceInputID(index, 1)
			draft, rootKnown := drafts[rootID]
			body, bodyOK := row.BodyID()
			if !rootOK || !memberOK || !rootKnown || !memberSpanID.Available() || !row.ID().Available() || !bodyOK || body != draft.body {
				return nil, false
			}
			position := row.Code()
			if _, duplicate := draft.memberByPos[position]; duplicate {
				return nil, false
			}
			// The member occurrence's own ID is the canonical ValuesMember
			// semantic row. Input 1 is only the producer span used to prove
			// member topology; it is not the member identity consumed by Value.
			draft.memberByPos[position] = row.ID()
		case programschema.OccurrenceValuesTail:
			rootID, rootOK := program.OccurrenceInputID(index, 0)
			draft, rootKnown := drafts[rootID]
			body, bodyOK := row.BodyID()
			kind := programschema.ValuesTailKind(row.Code())
			if !rootOK || !rootKnown || !bodyOK || body != draft.body || draft.hasTail || !kind.Valid() {
				return nil, false
			}
			draft.hasTail = true
			draft.tailKind = kind
		}
	}
	result := make(map[identity.ContentID]returnBoundaryTopology, len(drafts))
	for id, draft := range drafts {
		members := make([]identity.ContentID, len(draft.memberByPos))
		for position, member := range draft.memberByPos {
			if position >= uint64(len(members)) || !member.Available() || members[position].Available() {
				return nil, false
			}
			members[position] = member
		}
		for _, member := range members {
			if !member.Available() {
				return nil, false
			}
		}
		kind := draft.tailKind
		if !draft.hasTail {
			kind = programschema.ValuesTailInvalid
		}
		result[id] = returnBoundaryTopology{members: members, hasTail: draft.hasTail, tailKind: kind}
	}
	return result, true
}

func (schema *valueBuilder) sealComputationRows() bool {
	if schema == nil || schema.sealProject() == nil || schema.artifacts == nil || schema.Schema == nil ||
		schema.returnBoundaryMemberIndex == nil {
		return false
	}
	// The mounts are walked in project order rather than over the artifact map.
	// The member arena and the return-boundary directory are dense projections
	// of the order these rows are sealed in, and a map's order is not an order
	// a directory can be a projection of.
	mounts := schema.sealProject().Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		module, moduleOK := schema.sealProject().ModuleKey(shard)
		if !shardOK || !moduleOK || !module.Available() {
			return false
		}
		mount, mountOK := schema.artifacts[module]
		if !mountOK || !mount.Available() || mount.ModuleKey != module {
			return false
		}
		program := mount.Program.Program
		arithmeticPolicies, arithmeticPoliciesOK := resultpolicy.Seal(program)
		if !arithmeticPoliciesOK {
			return false
		}
		topologies, topologiesOK := sealReturnBoundaryTopologies(program)
		if !topologiesOK {
			return false
		}
		occurrenceCount, occurrenceCountOK := program.OccurrenceCount()
		if !occurrenceCountOK {
			return false
		}
		for index := 0; index < occurrenceCount; index++ {
			row, ok := program.OccurrenceAt(index)
			if !ok {
				return false
			}
			key := computationKey{module, row.ID()}
			switch row.Kind() {
			case programschema.OccurrenceBinaryArithmetic:
				leftID, leftOK := program.OccurrenceInputID(index, 0)
				rightID, rightOK := program.OccurrenceInputID(index, 1)
				op, rowOK := flowkind.BinaryOp(row.Code()), leftOK && rightOK
				result, resultOK := schema.sealBoundary().Values().ForMountedSpan(module, row.ID())
				left, leftOK := schema.sealBoundary().Values().ForMountedSpan(module, leftID)
				right, rightOK := schema.sealBoundary().Values().ForMountedSpan(module, rightID)
				rc, rcOK := schema.coordinateForCold(result)
				lc, lcOK := schema.coordinateForCold(left)
				rr, rrOK := schema.coordinateForCold(right)
				policy, policyOK := arithmeticPolicies.For(row.ID())
				if !rowOK || !resultOK || !leftOK || !rightOK || !rcOK || !lcOK || !rrOK || !flowkind.IsBinaryArithmetic(op) || !policyOK {
					return false
				}
				content := computationContent(schema.linkID, "val-arithmetic!", module, row.ID(), row.Code())
				arithmetic := BinaryArithmetic{
					schema: schema.Schema, key: key, content: content,
					result: rc, left: lc, right: rr, op: op, policy: policy,
				}
				if !arithmetic.valid() {
					return false
				}
				if _, duplicate := schema.binaryArithmetics[key]; duplicate {
					return false
				}
				schema.binaryArithmetics[key] = arithmetic
			case programschema.OccurrenceBinaryEquality:
				leftID, leftOK := program.OccurrenceInputID(index, 0)
				rightID, rightOK := program.OccurrenceInputID(index, 1)
				op, rowOK := flowkind.BinaryOp(row.Code()&0xff), leftOK && rightOK
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
			case programschema.OccurrenceBinaryOrder:
				leftID, leftOK := program.OccurrenceInputID(index, 0)
				rightID, rightOK := program.OccurrenceInputID(index, 1)
				op, rowOK := flowkind.BinaryOp(row.Code()), leftOK && rightOK
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
			case programschema.OccurrenceCall:
				// The runtime-kind rule consumes only the sealed geometry of a
				// strict unary plain call. Join the occurrence to the existing
				// canonical Program call family by its parent-issued ID; do not
				// infer call shape from occurrence inputs or reconstruct Program data.
				call, callOK := program.CallForID(row.ID())
				if !callOK {
					return false
				}
				// Calls outside the strict unary plain shape are valid calls,
				// but are not RuntimeKindCall operands. Their own Call domain
				// rules continue to interpret them.
				if call.Form() == programschema.CallFormMethod || call.ArgumentCount() != 1 {
					continue
				}
				if call.Form() != programschema.CallFormPlain {
					return false
				}
				if _, hasReceiver := call.ReceiverID(); hasReceiver {
					continue
				}
				if _, hasTail := call.TailID(); hasTail {
					continue
				}
				argument, argumentOK := program.CallArgumentForID(call.ID(), 0)
				if !argumentOK || !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 || !argument.MemberID().Available() {
					return false
				}
				resultSlot, resultOK := schema.Schema.MountedCallResultSlotFor(module, call.ID(), 0)
				// A discarded or open result has no finite Value coordinate and
				// therefore cannot be an operand for this result-writing rule. This
				// is cold schema non-admission, not a hot engine skip.
				if !resultOK {
					continue
				}
				input, inputOK := schema.sealBoundary().Values().ForMountedSemantic(module, argument.MemberID())
				rc, rcOK := resultSlot.Coordinate()
				ic, icOK := schema.coordinateForCold(input)
				if !resultOK || !inputOK || !rcOK || !icOK {
					return false
				}
				// Call names every executable mounted call application. An
				// occurrence Call does not name has no mounted-call coordinate
				// to publish, so this vertical seals no row for it; the row is
				// never issued with a default coordinate standing in for the
				// fact Call did not derive.
				coordinate, coordinateOK := schema.callCoordinateForOccurrence(module, call.ID())
				if !coordinateOK {
					continue
				}
				content := computationContent(schema.linkID, "val-runtime-kind-call!", module, row.ID())
				runtimeCall := RuntimeKindCall{schema: schema.Schema, key: key, content: content, result: rc, input: ic, comparison: ic, write: rc, coordinate: coordinate}
				if !runtimeCall.valid() {
					return false
				}
				if _, duplicate := schema.runtimeKindCalls[key]; duplicate {
					return false
				}
				schema.runtimeKindCalls[key] = runtimeCall
			case programschema.OccurrenceOperationPredicateRefinement:
				sourceCallID, sourceOK := program.OccurrenceInputID(index, 0)
				targetID, targetOK := program.OccurrenceInputID(index, 1)
				operandID, operandOK := program.OccurrenceInputID(index, 2)
				routeID, routeOK := program.OccurrenceInputID(index, 3)
				cellID, cellOK := program.OccurrenceInputID(index, 4)
				op := flowkind.BinaryOp(row.Code() & 0xff)
				truth := row.Code()&(1<<8) != 0
				rowOK := sourceOK && targetOK && operandOK && routeOK && cellOK
				call, callOK := program.CallForID(sourceCallID)
				if !rowOK || !routeID.Available() || !callOK || call.ID() != sourceCallID ||
					call.Form() != programschema.CallFormPlain || call.ArgumentCount() != 1 {
					return false
				}
				if _, hasReceiver := call.ReceiverID(); hasReceiver {
					return false
				}
				if _, hasTail := call.TailID(); hasTail {
					return false
				}
				argument, argumentOK := program.CallArgumentForID(call.ID(), 0)
				if !argumentOK || !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 || argument.MemberID() != targetID {
					return false
				}
				resultSlot, resultOK := schema.Schema.MountedCallResultSlotFor(module, call.ID(), 0)
				_, inputOK := schema.sealBoundary().Values().ForMountedSemantic(module, targetID)
				// Program issues the compared operand as a value-subject span
				// identity, the same identity the BinaryEquality row carries,
				// so Boundary's mounted span directory is its total inverse.
				// The semantic directory keys parent-issued occurrence IDs and
				// names an operand span only when another row published it.
				comparison, comparisonOK := schema.sealBoundary().Values().ForMountedSpan(module, operandID)
				// The predicate narrows the storage the subject was read from,
				// so the arm reads and writes that cell. Narrowing the call's
				// own argument coordinate would prove the fact about a value
				// no later Read is addressed to: the next mention of the
				// subject is a fresh read of this cell.
				cellValue, cellValueOK := schema.sealBoundary().Values().ForMountedSemantic(module, cellID)
				rc, rcOK := resultSlot.Coordinate()
				pc, pcOK := schema.coordinateForCold(comparison)
				cc, ccOK := schema.coordinateForCold(cellValue)
				// input authenticates that the subject really is this call's
				// own argument; the coordinate the arm narrows is the cell's.
				if !resultOK || !inputOK || !comparisonOK || !cellValueOK || !rcOK || !pcOK || !ccOK ||
					(op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual) {
					return false
				}
				truthCode := uint64(0)
				if truth {
					truthCode = 1
				}
				// The guarded predicate is an interpretation of the same
				// mounted call. Without Call's coordinate for that occurrence
				// the refinement has no call fact to read and seals no row.
				coordinate, coordinateOK := schema.callCoordinateForOccurrence(module, sourceCallID)
				if !coordinateOK {
					continue
				}
				content := computationContent(schema.linkID, "val-runtime-kind-predicate!", module, row.ID(), uint64(op), truthCode)
				runtimeCall := RuntimeKindCall{schema: schema.Schema, key: key, content: content, result: rc, input: cc, comparison: pc, write: cc, call: sourceCallID, op: op, truth: truth, refinement: true, coordinate: coordinate}
				if !runtimeCall.valid() {
					return false
				}
				if _, duplicate := schema.runtimeKindCalls[key]; duplicate {
					return false
				}
				schema.runtimeKindCalls[key] = runtimeCall
			case programschema.OccurrenceBinaryPresenceRefinement:
				_, targetID, _, _, present, rowOK := presenceRefinementInputs(program, index, row)
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
			case programschema.OccurrenceUnary:
				_, inputCount, inputSpanOK := row.InputSpan()
				if row.Code() != uint64(flowkind.UnaryNot) || !inputSpanOK || inputCount != 1 {
					continue
				}
				operandID, ok := program.OccurrenceInputID(index, 0)
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
			case programschema.OccurrenceSelect:
				_, inputCount, inputSpanOK := row.InputSpan()
				if !inputSpanOK || inputCount != 2 || (row.Code() != uint64(flowkind.SelectAnd) && row.Code() != uint64(flowkind.SelectOr)) {
					continue
				}
				leftID, leftOK := program.OccurrenceInputID(index, 0)
				rightID, rightOK := program.OccurrenceInputID(index, 1)
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
			case programschema.OccurrenceValueClaim:
				_, inputCount, inputSpanOK := row.InputSpan()
				if !inputSpanOK || inputCount != 1 {
					continue
				}
				operandID, ok := program.OccurrenceInputID(index, 0)
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
			case programschema.OccurrenceReturnBoundary:
				_, inputCount, inputSpanOK := row.InputSpan()
				if !inputSpanOK || inputCount != 1 {
					continue
				}
				valuesID, ok := program.OccurrenceInputID(index, 0)
				if !ok {
					return false
				}
				value, valueOK := schema.sealBoundary().Values().ForMountedSemantic(module, valuesID)
				coordinate, coordinateOK := schema.coordinateForCold(value)
				topology, topologyOK := topologies[valuesID]
				if !valueOK || !coordinateOK || !topologyOK || uint64(len(schema.returnBoundaryMembers))+uint64(len(topology.members)) > uint64(^uint32(0)) {
					return false
				}
				memberOffset := uint32(len(schema.returnBoundaryMembers))
				for position, memberID := range topology.members {
					member, memberOK := schema.sealBoundary().Values().ForMountedSemantic(module, memberID)
					memberCoordinate, memberCoordinateOK := schema.coordinateForCold(member)
					if !memberOK || !memberCoordinateOK {
						return false
					}
					content := computationContent(schema.linkID, "val-retmember!", module, row.ID(), uint64(position))
					memberKey := computationKey{module: module, occurrence: content}
					if _, duplicate := schema.returnBoundaryMemberIndex[memberKey]; duplicate {
						return false
					}
					schema.returnBoundaryMemberIndex[memberKey] = uint32(len(schema.returnBoundaryMembers))
					schema.returnBoundaryMembers = append(schema.returnBoundaryMembers, returnBoundaryMember{coordinate: memberCoordinate, content: content})
				}
				body, bodyOK := row.BodyID()
				boundary := ReturnBoundary{
					schema: schema.Schema, key: key,
					body:    body,
					ordinal: uint32(len(schema.returnBoundaryOrder)),
					content: computationContent(schema.linkID, "val-ret!", module, row.ID()),
					root:    coordinate, memberOffset: memberOffset, memberCount: uint32(len(topology.members)),
					hasTail: topology.hasTail, tailKind: topology.tailKind,
				}
				if !bodyOK || !body.Available() {
					return false
				}
				if _, duplicate := schema.returnBoundaries[key]; duplicate {
					return false
				}
				schema.returnBoundaries[key] = boundary
				schema.returnBoundaryOrder = append(schema.returnBoundaryOrder, key)
				bodyKey := computationKey{module: module, occurrence: body}
				schema.returnBoundariesByBody[bodyKey] = append(schema.returnBoundariesByBody[bodyKey], key)
				if !boundary.valid() {
					return false
				}
			}
		}
	}
	return true
}

func presenceRefinementInputs(program programschema.Program, index int, row programschema.Occurrence) (source, target, operand, route identity.ContentID, present bool, ok bool) {
	if !row.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false, false
	}
	ids := [4]identity.ContentID{}
	for position := range ids {
		id, held := program.OccurrenceInputID(index, position)
		if !held {
			return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false, false
		}
		ids[position] = id
	}
	return ids[0], ids[1], ids[2], ids[3], row.Code() == 1, true
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
