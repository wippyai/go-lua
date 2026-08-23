package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/heap"
)

// FreshResultCall is the Value-owned, detached operand for one Target fresh
// result whose ordinary Call has a fixed mounted CallResultValue output.
// Heap owns the structural key and its content identity; Value retains only
// the exact Call application/operation join and the existing Value coordinate
// written by the fresh-result rule.
type FreshResultCall struct {
	schema      *Schema
	key         heap.Key
	content     identity.ContentID
	application identity.ContentID
	operation   vocabulary.Operation
	coordinate  Coordinate
}

func (row FreshResultCall) valid() bool {
	return row.schema != nil && row.schema.Valid() && row.key.Valid() &&
		row.schema.heap.OwnsKey(row.key) && row.key.Kind() == heap.RootAllocation &&
		row.content.Available() && row.application.Available() && row.operation != 0 &&
		row.coordinate.schema == row.schema && row.coordinate.Valid()
}

// FreshResultCallFor resolves an admitted fixed fresh-result row by its exact
// owner-issued Heap key. Unknown, non-fresh, and foreign keys fail closed.
func (schema *Schema) FreshResultCallFor(key heap.Key) (FreshResultCall, bool) {
	if schema == nil || !schema.Valid() || schema.freshResultCalls == nil || !schema.heap.OwnsKey(key) {
		return FreshResultCall{}, false
	}
	row, ok := schema.freshResultCalls[key]
	return row, ok && schema.OwnsFreshResultCall(row)
}

// FreshResultCallCount returns the admitted fixed-result interval in the
// canonical Heap FreshAt order. The interval is sealed once and is therefore
// O(1) for Link-catalog enumeration.
func (schema *Schema) FreshResultCallCount() int {
	if schema == nil || !schema.Valid() || schema.freshResultCalls == nil || len(schema.freshResultCalls) != len(schema.freshResultCallKeys) {
		return 0
	}
	return len(schema.freshResultCallKeys)
}

// FreshResultCallAt returns one admitted fixed-result operand in canonical
// Heap FreshAt order. It is an owner-fenced projection, not a new occurrence
// identity or a second denominator.
func (schema *Schema) FreshResultCallAt(index int) (FreshResultCall, bool) {
	if schema == nil || index < 0 || index >= schema.FreshResultCallCount() {
		return FreshResultCall{}, false
	}
	return schema.FreshResultCallFor(schema.freshResultCallKeys[index])
}

// FreshResultCallForID resolves an admitted fixed fresh-result row by the Heap
// key content identity it is addressed by across the Link. It redeems the
// occurrence through Heap's own key directory rather than keeping a second
// identity index of its own.
func (schema *Schema) FreshResultCallForID(id identity.ContentID) (FreshResultCall, bool) {
	if schema == nil || !schema.Valid() || !id.Available() {
		return FreshResultCall{}, false
	}
	key, keyOK := schema.heap.KeyForID(id)
	if !keyOK || key.Kind() != heap.RootAllocation {
		return FreshResultCall{}, false
	}
	return schema.FreshResultCallFor(key)
}

// FreshResultCallIDAt is the occurrence identity of one admitted fixed result
// in the same canonical order FreshResultCallAt enumerates. It mints no second
// identity: the row is addressed by the Heap key content identity it already
// carries.
func (schema *Schema) FreshResultCallIDAt(index int) (identity.ContentID, bool) {
	row, rowOK := schema.FreshResultCallAt(index)
	if !rowOK {
		return identity.ContentID{}, false
	}
	return row.KeyID()
}

// FreshResultCallOrdinal is the dense position of one admitted row in the
// canonical order. It is the exact inverse of FreshResultCallAt.
func (schema *Schema) FreshResultCallOrdinal(row FreshResultCall) (uint32, bool) {
	if schema == nil || !schema.OwnsFreshResultCall(row) || schema.freshResultCallOrdinals == nil {
		return 0, false
	}
	ordinal, ok := schema.freshResultCallOrdinals[row.key]
	return ordinal, ok
}

// OwnsFreshResultCall is the exact Schema owner fence for a detached
// fresh-result operand. Equal-content Value schemas cannot exchange rows.
func (schema *Schema) OwnsFreshResultCall(row FreshResultCall) bool {
	if schema == nil || row.schema != schema || !row.valid() || schema.freshResultCalls == nil {
		return false
	}
	canonical, ok := schema.freshResultCalls[row.key]
	return ok && canonical == row
}

// Key returns Heap's exact structural coordinate for this fresh result.
func (row FreshResultCall) Key() (heap.Key, bool) {
	if !row.valid() {
		return heap.Key{}, false
	}
	return row.key, true
}

// KeyID returns the existing Heap Key content identity used as the Link
// occurrence ID for this operand.
func (row FreshResultCall) KeyID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

// ApplicationID returns the exact Project ordinary-call application bound to
// this fresh result's mounted Call.
func (row FreshResultCall) ApplicationID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.application, true
}

// Operation returns the Target operation authenticated against the exact
// Project application and mounted Call for this row.
func (row FreshResultCall) Operation() (vocabulary.Operation, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.operation, true
}

// Coordinate returns the existing detached mounted Value coordinate receiving
// the fresh result. FreshResultCall never creates a second Value coordinate.
func (row FreshResultCall) Coordinate() (Coordinate, bool) {
	if !row.valid() {
		return Coordinate{}, false
	}
	return row.coordinate, true
}

// Age applies the Value-owned carry transition for this exact fresh-result
// candidate.  The candidate already carries Heap's issued key, so the
// transform is a direct owner method: callers provide the candidate and the
// prior Value fact, and no callback or key reconstruction is needed.
func (row FreshResultCall) Age(prior Value) (Value, bool) {
	if !row.valid() {
		return Value{}, false
	}
	return row.schema.Age(prior, row.key)
}

type freshResultCallOrigin struct {
	application linkproject.Application
	module      identity.ContentID
	call        identity.ContentID
}

// sealFreshResultCalls constructs Value's detached fresh-result directory.
// Heap supplies the complete fresh-root denominator; Project supplies only
// the ordinary-call application-to-mount join. The canonical mounted
// Artifact/Snapshot CallResultSlot directory supplies reusable finite output
// geometry;
// Boundary contributes only generic Link application-operation membership.
// The detached mounted-coordinate directory supplies the existing Value
// coordinate, so no second Link-level result-coordinate row is read.
// Open or structural slots have no admitted Value coordinate and are omitted.
func (schema *valueBuilder) sealFreshResultCalls() bool {
	if schema == nil || schema.Schema == nil || schema.sealProject() == nil || schema.sealBoundary() == nil || schema.Schema.mountedCallResultSlots == nil || schema.freshResultCalls == nil || schema.freshResultCallOrdinals == nil || len(schema.freshResultCalls) != 0 || len(schema.freshResultCallKeys) != 0 || len(schema.freshResultCallOrdinals) != 0 || !schema.heap.Valid() {
		return false
	}

	applications := schema.sealProject().Applications().Calls()
	origins := make(map[identity.ContentID]freshResultCallOrigin, applications.Count())
	for index := 0; index < applications.Count(); index++ {
		application, applicationOK := applications.At(index)
		applicationID, moduleID, callID, mountedOK := applications.MountedIdentity(application)
		if !applicationOK || !mountedOK || !applicationID.Available() || !moduleID.Available() || !callID.Available() {
			return false
		}
		if _, duplicate := origins[applicationID]; duplicate {
			return false
		}
		origins[applicationID] = freshResultCallOrigin{application: application, module: moduleID, call: callID}
	}

	target, targetOK := schema.sealBoundary().Target()
	if !targetOK || target == nil {
		return false
	}
	for index := 0; index < schema.heap.FreshCount(); index++ {
		content, key, keyOK := schema.heap.FreshAt(index)
		keyContent, keyContentOK := key.ContentID()
		applicationID, outcomeResultID, _, freshOK := key.FreshResultID()
		if !keyOK || !content.Available() || !keyContentOK || keyContent != content || !freshOK || !applicationID.Available() || !outcomeResultID.Available() {
			return false
		}
		operation, outcome, resultIndex, outcomeOK := target.FindOutcomeResultID(outcomeResultID)
		if !outcomeOK || operation == 0 || outcome < 0 || uint64(outcome) > uint64(^uint32(0)) || resultIndex < 0 || uint64(resultIndex) > uint64(^uint32(0)) {
			return false
		}
		origin, found := origins[applicationID]
		if !found {
			continue
		}
		slot, slotOK := schema.MountedCallResultSlotFor(origin.module, origin.call, uint32(resultIndex))
		if !slotOK || !schema.OwnsMountedCallResultSlot(slot) {
			continue
		}
		if !schema.sealBoundary().ApplicationOperationAvailable(target, origin.application, operation) {
			continue
		}
		coordinate, coordinateOK := slot.Coordinate()
		if !coordinateOK {
			return false
		}
		row := FreshResultCall{
			schema: schema.Schema, key: key, content: content,
			application: applicationID, operation: operation, coordinate: coordinate,
		}
		if !row.valid() {
			return false
		}
		if _, duplicate := schema.freshResultCalls[key]; duplicate {
			return false
		}
		schema.freshResultCalls[key] = row
		schema.freshResultCallOrdinals[key] = uint32(len(schema.freshResultCallKeys))
		schema.freshResultCallKeys = append(schema.freshResultCallKeys, key)
	}
	return len(schema.freshResultCalls) == len(schema.freshResultCallKeys) &&
		len(schema.freshResultCallOrdinals) == len(schema.freshResultCallKeys)
}
