package programartifact

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
)

// PackReceipt is the artifact-owned complete source plane required by Pack.
// It is deliberately separate from Pack itself: Program copies these rows
// while its proofs are live, then Link substitutes only a ModuleKey and its
// already-sealed Boundary values.  No row contains a Program term, Flow
// handle, Link ordinal, or domain handle.
//
// This receipt is intentionally not a generic occurrence format.  Its three
// tables are the exact denominators Pack needs: bind destinations (including
// positions past a fixed Values prefix), function formal Cells, and calls.
type PackReceipt struct {
	binds  []PackBindRow
	bodies []PackBodyReceiptRow
	calls  []PackCallReceiptRow
}

func (receipt PackReceipt) Available() bool {
	for _, row := range receipt.binds {
		if !row.Available() {
			return false
		}
	}
	for _, row := range receipt.bodies {
		if !row.Available() {
			return false
		}
	}
	for _, row := range receipt.calls {
		if !row.Available() {
			return false
		}
	}
	return true
}

func (receipt PackReceipt) BindCount() int {
	if !receipt.Available() {
		return 0
	}
	return len(receipt.binds)
}
func (receipt PackReceipt) BindAt(index int) (PackBindRow, bool) {
	if !receipt.Available() || index < 0 || index >= len(receipt.binds) {
		return PackBindRow{}, false
	}
	return receipt.binds[index], true
}
func (receipt PackReceipt) BodyCount() int {
	if !receipt.Available() {
		return 0
	}
	return len(receipt.bodies)
}
func (receipt PackReceipt) BodyAt(index int) (PackBodyReceiptRow, bool) {
	if !receipt.Available() || index < 0 || index >= len(receipt.bodies) {
		return PackBodyReceiptRow{}, false
	}
	return receipt.bodies[index], true
}
func (receipt PackReceipt) CallCount() int {
	if !receipt.Available() {
		return 0
	}
	return len(receipt.calls)
}
func (receipt PackReceipt) CallAt(index int) (PackCallReceiptRow, bool) {
	if !receipt.Available() || index < 0 || index >= len(receipt.calls) {
		return PackCallReceiptRow{}, false
	}
	return receipt.calls[index], true
}

// PackBindRow preserves every destination Cell in authored bind order.  A
// shorter fixed Values prefix does not remove later destination Cells.
type PackBindRow struct {
	id, body, values keyspace.ContentID
	cells            []keyspace.ContentID
}

func (row PackBindRow) Available() bool {
	if !row.id.Available() || !row.body.Available() || !row.values.Available() {
		return false
	}
	for _, cell := range row.cells {
		if !cell.Available() {
			return false
		}
	}
	return true
}
func (row PackBindRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row PackBindRow) BodyID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body
}
func (row PackBindRow) ValuesID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.values
}
func (row PackBindRow) CellCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.cells)
}
func (row PackBindRow) CellAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.cells) {
		return keyspace.ContentID{}, false
	}
	return row.cells[index], true
}

type PackFormalCell struct{ formal, cell, storage keyspace.ContentID }

func (row PackFormalCell) Available() bool {
	return row.formal.Available() && row.cell.Available() && row.storage.Available()
}
func (row PackFormalCell) FormalID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.formal
}
func (row PackFormalCell) CellID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.cell
}

// StorageCellID is the exact parent-issued bridge to Boundary's storage Cell
// value namespace. CellID remains the formal-role identity for formal laws.
func (row PackFormalCell) StorageCellID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.storage
}

// PackBodyReceiptRow preserves callable formal order while retaining every
// Body's stable path/context identity. Non-callable bodies have no formals.
type PackBodyReceiptRow struct {
	id, context keyspace.ContentID
	formals     []PackFormalCell
	callable    bool
}

func (row PackBodyReceiptRow) Available() bool {
	if !row.id.Available() || !row.context.Available() {
		return false
	}
	for _, formal := range row.formals {
		if !formal.Available() {
			return false
		}
	}
	return row.callable || len(row.formals) == 0
}
func (row PackBodyReceiptRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row PackBodyReceiptRow) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.context
}
func (row PackBodyReceiptRow) Callable() bool { return row.Available() && row.callable }
func (row PackBodyReceiptRow) FormalCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.formals)
}
func (row PackBodyReceiptRow) FormalAt(index int) (PackFormalCell, bool) {
	if !row.Available() || index < 0 || index >= len(row.formals) {
		return PackFormalCell{}, false
	}
	return row.formals[index], true
}

// PackCallReceiptRow is one reusable Program call.  All operands are
// occurrence identities; Boundary is responsible for their mounted Value
// substitutions.  Type argument identities retain exact order for Static's
// existing authority without importing a static-domain type here.
type PackCallReceiptRow struct {
	id, body, formal, values, types, callee, actuals, receiver, tail keyspace.ContentID
	form                                                             flow.CallForm
	arguments                                                        []keyspace.ContentID
	typeArguments                                                    []keyspace.ContentID
	hasReceiver, hasTail                                             bool
}

func (row PackCallReceiptRow) Available() bool {
	if !row.id.Available() || !row.body.Available() || !row.formal.Available() || !row.values.Available() || !row.types.Available() || !row.callee.Available() || !row.actuals.Available() || (row.form != flow.CallFormPlain && row.form != flow.CallFormMethod) || row.hasReceiver != row.receiver.Available() || row.hasTail != row.tail.Available() {
		return false
	}
	for _, id := range row.arguments {
		if !id.Available() {
			return false
		}
	}
	for _, id := range row.typeArguments {
		if !id.Available() {
			return false
		}
	}
	return true
}
func (row PackCallReceiptRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row PackCallReceiptRow) BodyID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body
}
func (row PackCallReceiptRow) FormalID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.formal
}
func (row PackCallReceiptRow) ValuesID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.values
}
func (row PackCallReceiptRow) TypeArgumentsID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.types
}
func (row PackCallReceiptRow) CalleeID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.callee
}
func (row PackCallReceiptRow) ActualsID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.actuals
}
func (row PackCallReceiptRow) Form() flow.CallForm {
	if !row.Available() {
		return 0
	}
	return row.form
}
func (row PackCallReceiptRow) ReceiverID() (keyspace.ContentID, bool) {
	return row.receiver, row.Available() && row.hasReceiver
}
func (row PackCallReceiptRow) TailID() (keyspace.ContentID, bool) {
	return row.tail, row.Available() && row.hasTail
}
func (row PackCallReceiptRow) ArgumentCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.arguments)
}
func (row PackCallReceiptRow) ArgumentAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.arguments) {
		return keyspace.ContentID{}, false
	}
	return row.arguments[index], true
}
func (row PackCallReceiptRow) TypeArgumentCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.typeArguments)
}
func (row PackCallReceiptRow) TypeArgumentAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.typeArguments) {
		return keyspace.ContentID{}, false
	}
	return row.typeArguments[index], true
}

// copyPackReceiptFailure records the exact Pack source denominators while
// Program owns the corresponding proofs. It is intentionally called before
// Program publication; Pack later receives only the immutable receipt.
func (compiler *compiler) copyPackReceiptFailure() (PackReceipt, CompileFailure) {
	if compiler == nil || !compiler.input.Available() {
		return PackReceipt{}, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	receipt := PackReceipt{}
	for index := 0; index < compiler.input.StorageBindCount(); index++ {
		bind, ok := compiler.input.StorageBindAt(index)
		if !ok {
			continue
		}
		body, bodyOK := bind.Body()
		values, valuesOK := bind.Values()
		row := PackBindRow{id: bind.ContextID()}
		if bodyOK {
			row.body = body.PathID()
		}
		if valuesOK {
			row.values = values.ID()
		}
		for position := 0; position < bind.TransferCount(); position++ {
			cell, cellOK := bind.CellAt(position)
			if !cellOK {
				return PackReceipt{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceStorageBind)
			}
			row.cells = append(row.cells, cell.ContextID())
		}
		if !compiler.input.OwnsStorageBind(bind) || !bodyOK || !valuesOK || !row.Available() {
			return PackReceipt{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
		}
		receipt.binds = append(receipt.binds, row)
	}
	for index := 0; index < compiler.input.BodyCount(); index++ {
		body, ok := compiler.input.BodyAt(index)
		if !ok || !compiler.input.OwnsBody(body) {
			return PackReceipt{}, compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		row := PackBodyReceiptRow{id: body.PathID(), context: body.ContextID()}
		if function, callable := body.TransformerFunction(); callable {
			row.callable = true
			for position := 0; position < function.FormalCount(); position++ {
				formal, formalOK := function.FormalAt(position)
				cell, cellOK := formal.Cell()
				storage, storageOK := formal.StorageCell()
				if !formalOK || !cellOK || !storageOK || !compiler.input.OwnsFormal(formal) {
					return PackReceipt{}, compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, position, CompileReasonBodyUnavailable)
				}
				row.formals = append(row.formals, PackFormalCell{formal: formal.ContextID(), cell: cell.ContextID(), storage: storage.ContextID()})
			}
		}
		if !row.Available() {
			return PackReceipt{}, compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		receipt.bodies = append(receipt.bodies, row)
	}
	for index := 0; index < compiler.input.CallCount(); index++ {
		call, ok := compiler.input.CallAt(index)
		if !ok {
			continue
		}
		body, bodyOK := call.Body()
		formal, formalOK := call.Formal()
		values, valuesOK := call.Values()
		types, typesOK := call.TypeArguments()
		callee, calleeOK := call.Callee()
		actuals, actualsOK := call.Actuals()
		form, formOK := call.Form()
		row := PackCallReceiptRow{id: call.ContextID(), form: form}
		if bodyOK {
			row.body = body.PathID()
		}
		if formalOK {
			row.formal = formal.ID()
		}
		if valuesOK {
			row.values = values.ContextID()
		}
		if typesOK {
			row.types = types.ContextID()
		}
		if calleeOK {
			row.callee = callee.ContextID()
		}
		if actualsOK {
			row.actuals = actuals.ContextID()
		}
		if receiver, receiverOK := call.Receiver(); receiverOK {
			row.receiver, row.hasReceiver = receiver.ContextID(), true
		}
		if valuesOK {
			for position := 0; position < values.Count(); position++ {
				argument, argumentOK := values.At(position)
				if !argumentOK {
					return PackReceipt{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceCall)
				}
				row.arguments = append(row.arguments, argument.ContextID())
			}
			if tail, tailOK := values.Tail(); tailOK {
				row.tail, row.hasTail = tail.ContextID(), true
			}
		}
		if typesOK {
			for position := 0; position < types.Count(); position++ {
				argument, argumentOK := types.At(position)
				if !argumentOK {
					return PackReceipt{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceCall)
				}
				row.typeArguments = append(row.typeArguments, argument.ContextID())
			}
		}
		if !compiler.input.OwnsCallOccurrence(call) || !bodyOK || !formalOK || !valuesOK || !typesOK || !calleeOK || !actualsOK || !formOK || !row.Available() {
			return PackReceipt{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		receipt.calls = append(receipt.calls, row)
	}
	return receipt, CompileFailure{}
}

// validAgainst proves the receipt is a refinement of the already-sealed
// generic artifact planes, not a second source authority.  Every Bind and
// Call must resolve to its exact generic occurrence; every referenced Body
// and Values row must already exist in the parent artifact.
func (receipt PackReceipt) validAgainst(artifact *Artifact) bool {
	if !receipt.Available() || artifact == nil {
		return false
	}
	bodies := make(map[keyspace.ContentID]keyspace.ContentID, len(artifact.bodies))
	for _, body := range artifact.bodies {
		if !body.Available() || bodies[body.ID()].Available() {
			return false
		}
		bodies[body.ID()] = body.ContextID()
	}
	functions := make(map[keyspace.ContentID]FunctionBoundaryRow, len(artifact.functionBoundaries))
	for _, function := range artifact.functionBoundaries {
		if !function.Available() || functions[function.BodyID()].Available() {
			return false
		}
		functions[function.BodyID()] = function
	}
	values := make(map[keyspace.ContentID]struct{}, len(artifact.values))
	for _, value := range artifact.values {
		if !value.Available() {
			return false
		}
		values[value.ID()] = struct{}{}
	}
	occurrence := func(kind OccurrenceKind, id keyspace.ContentID) (OccurrenceRow, bool) {
		index, ok := artifact.occurrenceByID[occurrenceLookup{kind: kind, id: id}]
		if !ok || uint64(index) >= uint64(len(artifact.occurrences)) {
			return OccurrenceRow{}, false
		}
		row := artifact.occurrences[index]
		return row, row.Available() && row.Kind() == kind && row.ID() == id
	}
	seenBinds := make(map[keyspace.ContentID]struct{}, len(receipt.binds))
	for _, row := range receipt.binds {
		if !row.Available() || !bodies[row.body].Available() {
			return false
		}
		if _, exists := values[row.values]; !exists {
			return false
		}
		if _, duplicate := seenBinds[row.id]; duplicate {
			return false
		}
		occurrenceRow, occurrenceOK := occurrence(OccurrenceStorageBind, row.id)
		body, bodyOK := occurrenceRow.BodyID()
		if !occurrenceOK || !bodyOK || body != row.body {
			return false
		}
		seenBinds[row.id] = struct{}{}
	}
	seenBodies := make(map[keyspace.ContentID]struct{}, len(receipt.bodies))
	for _, row := range receipt.bodies {
		context, exists := bodies[row.id]
		if !row.Available() || !exists || context != row.context {
			return false
		}
		function, callable := functions[row.id]
		if row.Callable() != callable || row.FormalCount() != function.FormalCount() {
			return false
		}
		for index := 0; index < row.FormalCount(); index++ {
			formal, formalOK := row.FormalAt(index)
			port, portOK := function.FormalAt(index)
			if !formalOK || !portOK || formal.FormalID() != port.ID() || formal.CellID() != port.CellID() || formal.StorageCellID() != port.StorageCellID() {
				return false
			}
		}
		if _, duplicate := seenBodies[row.id]; duplicate {
			return false
		}
		seenBodies[row.id] = struct{}{}
	}
	seenCalls := make(map[keyspace.ContentID]struct{}, len(receipt.calls))
	for _, row := range receipt.calls {
		if !row.Available() || !bodies[row.body].Available() {
			return false
		}
		if _, duplicate := seenCalls[row.id]; duplicate {
			return false
		}
		occurrenceRow, occurrenceOK := occurrence(OccurrenceCall, row.id)
		body, bodyOK := occurrenceRow.BodyID()
		if !occurrenceOK || !bodyOK || body != row.body {
			return false
		}
		seenCalls[row.id] = struct{}{}
	}
	return len(seenBodies) == len(artifact.bodies)
}
