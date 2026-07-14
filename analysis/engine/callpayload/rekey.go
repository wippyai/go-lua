package callpayload

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// RekeyHeapTableObjects structurally imports every heap-object key carried by
// the outcome. The operation is transactional: an invalid authority or foreign
// nested key returns the original outcome and an error, never a partially
// imported caller payload. Nil provenance is valid only when every object is
// structurally key-free, as decided by TableObject.Rekey.
func (o CallOutcome) RekeyHeapTableObjects(from, to *keyspace.KeySpace) (CallOutcome, error) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return o, fmt.Errorf("call outcome rekey: invalid keyspace authority")
	}
	if len(o.HeapTableObjects) == 0 {
		return o, nil
	}
	rekeyed := make(map[identity.ID]heapidentity.TableObject, len(o.HeapTableObjects))
	for id, object := range o.HeapTableObjects {
		next, ok := object.Rekey(from, to)
		if !ok {
			return o, fmt.Errorf("call outcome rekey: heap table object %v has a foreign structural key", id)
		}
		rekeyed[id] = next
	}
	out := o
	out.HeapTableObjects = rekeyed
	return out, nil
}
