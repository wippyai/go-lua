package bootstrap

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/identity"
)

type catalogRow struct {
	root Root
	key  heapdomain.Key
}

// Catalog is Heap/bootstrap's Link-global BootRoot operand directory.  It is
// sealed once from the exact Heap schema and Host; unlike allocation catalogs
// it has no Program-mount dimension.
type Catalog struct {
	schema    heapdomain.Schema
	ids       []identity.ContentID
	rows      map[identity.ContentID]catalogRow
	bootCount int
}

// SealCatalog enumerates every exact Host BootRoot once, derives its existing
// Heap key, and preissues the complete bootstrap Root operand. Duplicate
// detached identities are rejected rather than overwritten.
func SealCatalog(schema heapdomain.Schema) (*Catalog, bool) {
	if !schema.Valid() {
		return nil, false
	}
	ids := make([]identity.ContentID, 0, schema.BootCount())
	rows := make(map[identity.ContentID]catalogRow, schema.BootCount())
	for index := 0; index < schema.BootCount(); index++ {
		id, idOK := schema.BootIDAt(index)
		key, keyOK := schema.KeyForBootID(id)
		root, rootOK := NewRoot(schema, key)
		if !idOK || !id.Available() || !keyOK || !rootOK || !root.fencedTo(schema) {
			return nil, false
		}
		if _, duplicate := rows[id]; duplicate {
			return nil, false
		}
		ids = append(ids, id)
		rows[id] = catalogRow{root: root, key: key}
	}
	return &Catalog{schema: schema, ids: ids, rows: rows, bootCount: schema.BootCount()}, true
}

// FencedTo authenticates the catalog against the exact Heap schema and its
// Link/Host owner. Equal heap content from another seal is rejected.
func (catalog *Catalog) FencedTo(schema heapdomain.Schema) bool {
	return catalog != nil && schema.Valid() && catalog.schema == schema &&
		catalog.bootCount >= 0 && len(catalog.ids) == catalog.bootCount && len(catalog.rows) == len(catalog.ids)
}

// Count returns the exact Link-global BootRoot denominator captured at seal.
func (catalog *Catalog) Count() int {
	if catalog == nil || !catalog.FencedTo(catalog.schema) {
		return 0
	}
	return len(catalog.ids)
}

// IDAt returns one BootRoot semantic identity in the Host owner's canonical
// order.  The returned scalar is immutable and never exposes the BootRoot.
func (catalog *Catalog) IDAt(index int) (identity.ContentID, bool) {
	if catalog == nil || !catalog.FencedTo(catalog.schema) || index < 0 || index >= len(catalog.ids) {
		return identity.ContentID{}, false
	}
	id := catalog.ids[index]
	_, _, exists := catalog.ReceiptForID(id)
	return id, id.Available() && exists
}

// ReceiptForID returns the exact preissued bootstrap Root and its paired Heap
// key from one owner-fenced catalog row in O(1).
func (catalog *Catalog) ReceiptForID(id identity.ContentID) (Root, heapdomain.Key, bool) {
	if catalog == nil || !catalog.FencedTo(catalog.schema) || !id.Available() {
		return Root{}, heapdomain.Key{}, false
	}
	row, ok := catalog.rows[id]
	if !ok || !row.root.fencedTo(catalog.schema) || !catalog.schema.OwnsKey(row.key) {
		return Root{}, heapdomain.Key{}, false
	}
	bootID, bootOK := row.key.BootID()
	if !bootOK || bootID != id {
		return Root{}, heapdomain.Key{}, false
	}
	return row.root, row.key, true
}
