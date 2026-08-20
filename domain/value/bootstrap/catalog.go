package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
)

// Catalog is Value/bootstrap's Link-global operand directory.  It is sealed
// once from the exact Host owned by schema and retains only Host-issued
// GlobalBinding operands keyed by their detached Host-content identities.
// Global bindings are Link-global, so this directory is deliberately not
// mounted per Program or per Module.
type Catalog struct {
	schema      *value.Schema
	rows        map[identity.ContentID]struct{}
	ids         []identity.ContentID
	globalCount int
}

// SealCatalog enumerates every exact Host global once and preissues its
// GlobalBinding operand.  Duplicate detached identities are rejected rather
// than silently overwriting an existing receipt.
func SealCatalog(schema *value.Schema) (*Catalog, bool) {
	if schema == nil {
		return nil, false
	}
	rows := make(map[identity.ContentID]struct{}, schema.GlobalBootstrapResultCount())
	ids := make([]identity.ContentID, 0, schema.GlobalBootstrapResultCount())
	for index := 0; index < schema.GlobalBootstrapResultCount(); index++ {
		id, idOK := schema.GlobalBootstrapResultIDAt(index)
		if !idOK || !id.Available() {
			return nil, false
		}
		if _, duplicate := rows[id]; duplicate {
			return nil, false
		}
		rows[id] = struct{}{}
		ids = append(ids, id)
	}
	return &Catalog{schema: schema, rows: rows, ids: ids, globalCount: len(ids)}, true
}

// FencedTo authenticates the catalog against the exact Link/Host and Value
// schema that issued every operand.  Equal content from another seal is not
// sufficient: the owner pointers are part of the receipt fence.
func (catalog *Catalog) FencedTo(schema *value.Schema) bool {
	return catalog != nil && schema != nil && catalog.schema == schema &&
		catalog.globalCount >= 0 && len(catalog.rows) == catalog.globalCount && len(catalog.ids) == len(catalog.rows)
}

// Count returns the exact Host-global order captured at catalog seal.
func (catalog *Catalog) Count() int {
	if catalog == nil || !catalog.FencedTo(catalog.schema) {
		return 0
	}
	return len(catalog.ids)
}

// IDAt returns the stable Host-global identity in Host declaration order.
// This ordered projection is the Link-global bootstrap witness vocabulary;
// callers never infer ordering from map iteration.
func (catalog *Catalog) IDAt(index int) (identity.ContentID, bool) {
	if catalog == nil || !catalog.FencedTo(catalog.schema) || index < 0 || index >= len(catalog.ids) {
		return identity.ContentID{}, false
	}
	id := catalog.ids[index]
	_, ok := catalog.ReceiptForID(id)
	return id, ok
}

// ReceiptForID returns one exact preissued GlobalBinding in O(1).  The ID is
// the Host's detached identity; no Host mapping or Program/Flow traversal is
// performed here.
func (catalog *Catalog) ReceiptForID(id identity.ContentID) (identity.ContentID, bool) {
	if catalog == nil || !catalog.FencedTo(catalog.schema) || !id.Available() {
		return identity.ContentID{}, false
	}
	_, ok := catalog.rows[id]
	if !ok {
		return identity.ContentID{}, false
	}
	return id, true
}
