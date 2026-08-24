package placement

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/heap"
)

// schemaFormat is the Placement-family discriminator. Placement has no
// independent coordinate directory: its cold identity is derived solely from
// the Heap schema it projects.
const schemaFormat uint64 = 0x706c6163656d2d32 // "placem-2"

// Schema is Placement's immutable cold authority. It wraps exactly one valid
// Heap schema and projects only Heap's dense allocation-root coordinates.
// Boot roots are not allocation decisions and therefore have no Placement
// cell.
type Schema struct {
	heap heap.Schema
	id   identity.ContentID
}

// NewSchema derives Placement's cold authority from one already sealed Heap
// authority. Heap remains the sole source of allocation order and identity.
func NewSchema(source heap.Schema) (Schema, bool) {
	if !source.Valid() {
		return Schema{}, false
	}
	id := placementContentID(source.ContentID())
	if !id.Available() {
		return Schema{}, false
	}
	return Schema{heap: source, id: id}, true
}

func placementContentID(heapID identity.ContentID) identity.ContentID {
	if !heapID.Available() {
		return identity.ContentID{}
	}
	var payload [32 + 8]byte
	copy(payload[:32], heapID[:])
	binary.BigEndian.PutUint64(payload[32:], schemaFormat)
	return identity.ContentID(sha256.Sum256(payload[:]))
}

func (schema Schema) valid() bool {
	// Schema is an immutable value: the only issuer is NewSchema, which
	// verifies the derived Placement ContentID once at seal time. Rehashing
	// Heap's identity on every dense KeyAt/Heap call turns ordinary summary
	// and route walks into repeated SHA-256 work without adding an
	// owner-fence guarantee (the private fields cannot be forged by callers).
	return schema.heap.Valid() && schema.id.Available()
}

// Valid reports whether Schema is a complete Placement authority.
func (schema Schema) Valid() bool { return schema.valid() }

// ContentID identifies this Placement declaration and its exact Heap fence.
func (schema Schema) ContentID() identity.ContentID {
	if !schema.valid() {
		return identity.ContentID{}
	}
	return schema.id
}

// Heap returns the exact Heap authority Placement projects.
func (schema Schema) Heap() heap.Schema {
	if !schema.valid() {
		return heap.Schema{}
	}
	return schema.heap
}

// KeyCount returns Heap's dense allocation-root count. Placement does not
// maintain a second coordinate index and never allocates inert Boot cells.
func (schema Schema) KeyCount() int {
	if !schema.valid() {
		return 0
	}
	return schema.heap.AllocationKeyCount()
}

// DenseKeyCount names the same allocation-root coordinate count.
func (schema Schema) DenseKeyCount() int { return schema.KeyCount() }

// KeyAt delegates dense allocation-coordinate issuance to Heap.
func (schema Schema) KeyAt(index int) (heap.Key, bool) {
	if !schema.valid() {
		return heap.Key{}, false
	}
	return schema.heap.AllocationKeyAt(index)
}

// KeyIndex normalizes one owner-issued allocation key to Placement's dense
// coordinate.  Placement deliberately keeps Heap as the sole key directory;
// this method is only the typed owner projection consumed by generated member
// metadata and does not retain a second index.
func (schema Schema) KeyIndex(key heap.Key) (uint32, bool) {
	if !schema.valid() {
		return 0, false
	}
	index, ok := schema.heap.AllocationKeyIndex(key)
	if !ok || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(index), true
}
