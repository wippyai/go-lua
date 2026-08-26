package address

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

const (
	bookDigestDomain  = "analysis/relation/mount/address/book/v1"
	entryDigestDomain = "analysis/relation/mount/address/entry/v1"
)

// Bind resolves every logical identity certified by certificate exactly once
// against inventory and snapshots the results into an immutable Book.  The
// inventory fence must exactly match both the certificate schema and digest;
// missing resolutions and duplicate local coordinates are refused.
func Bind(cert certificate.Certificate, inventory Inventory) (Book, bool) {
	if inventory == nil || !cert.Available() || !cert.SchemaID().Available() || !cert.Digest().Available() {
		return Book{}, false
	}
	fence := inventory.Fence()
	if !fence.Available() || fence.SchemaID() != cert.SchemaID() || fence.CertificateDigest() != cert.Digest() {
		return Book{}, false
	}

	relations, relationIDs, ok := bindRelations(cert.Relations(), inventory, fence)
	if !ok {
		return Book{}, false
	}
	columns, columnIDs, ok := bindColumns(cert.Columns(), inventory, fence)
	if !ok {
		return Book{}, false
	}
	keys, keyIDs, ok := bindKeys(cert.Keys(), inventory, fence)
	if !ok {
		return Book{}, false
	}
	scopes, scopeIDs, ok := bindScopes(cert.Scopes(), inventory, fence)
	if !ok {
		return Book{}, false
	}
	expressions, expressionIDs, ok := bindExpressions(cert.ExpressionIDs(), inventory, fence)
	if !ok {
		return Book{}, false
	}
	dependencies, dependencyIDs, ok := bindDependencies(cert.DependencyIDs(), inventory, fence)
	if !ok {
		return Book{}, false
	}

	digest, ok := digestBook(cert.Digest(), fence,
		relationIDs, relations,
		columnIDs, columns,
		keyIDs, keys,
		scopeIDs, scopes,
		expressionIDs, expressions,
		dependencyIDs, dependencies)
	if !ok {
		return Book{}, false
	}

	return Book{data: &bookData{
		fence: fence, digest: digest,
		relations: relations, columns: columns, keys: keys, scopes: scopes,
		expressions: expressions, dependencies: dependencies,
		relationIDs: relationIDs, columnIDs: columnIDs, keyIDs: keyIDs,
		scopeIDs: scopeIDs, expressionIDs: expressionIDs, dependencyIDs: dependencyIDs,
	}}, true
}

func bindRelations(values []model.RelationSchema, inventory Inventory, fence Fence) (map[model.RelationID]Address[model.RelationID], []model.RelationID, bool) {
	result := make(map[model.RelationID]Address[model.RelationID], len(values))
	ids := make([]model.RelationID, 0, len(values))
	used := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		id := value.ID()
		if !value.Available() || !id.Available() {
			return nil, nil, false
		}
		if _, exists := result[id]; exists {
			return nil, nil, false
		}
		slot, ok := inventory.ResolveRelation(id)
		if !ok || slot == 0 {
			return nil, nil, false
		}
		if _, exists := used[slot]; exists {
			return nil, nil, false
		}
		used[slot] = struct{}{}
		result[id] = newAddress(id, slot, fence)
		ids = append(ids, id)
	}
	return result, ids, true
}

func bindColumns(values []model.ColumnSchema, inventory Inventory, fence Fence) (map[model.ColumnID]Address[model.ColumnID], []model.ColumnID, bool) {
	result := make(map[model.ColumnID]Address[model.ColumnID], len(values))
	ids := make([]model.ColumnID, 0, len(values))
	used := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		id := value.ID()
		if !value.Available() || !id.Available() {
			return nil, nil, false
		}
		if _, exists := result[id]; exists {
			return nil, nil, false
		}
		slot, ok := inventory.ResolveColumn(id)
		if !ok || slot == 0 {
			return nil, nil, false
		}
		if _, exists := used[slot]; exists {
			return nil, nil, false
		}
		used[slot] = struct{}{}
		result[id] = newAddress(id, slot, fence)
		ids = append(ids, id)
	}
	return result, ids, true
}

func bindKeys(values []model.KeySchema, inventory Inventory, fence Fence) (map[model.KeyID]Address[model.KeyID], []model.KeyID, bool) {
	result := make(map[model.KeyID]Address[model.KeyID], len(values))
	ids := make([]model.KeyID, 0, len(values))
	used := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		id := value.ID()
		if !value.Available() || !id.Available() {
			return nil, nil, false
		}
		if _, exists := result[id]; exists {
			return nil, nil, false
		}
		slot, ok := inventory.ResolveKey(id)
		if !ok || slot == 0 {
			return nil, nil, false
		}
		if _, exists := used[slot]; exists {
			return nil, nil, false
		}
		used[slot] = struct{}{}
		result[id] = newAddress(id, slot, fence)
		ids = append(ids, id)
	}
	return result, ids, true
}

func bindScopes(values []model.ScopeSchema, inventory Inventory, fence Fence) (map[model.ScopeID]Address[model.ScopeID], []model.ScopeID, bool) {
	result := make(map[model.ScopeID]Address[model.ScopeID], len(values))
	ids := make([]model.ScopeID, 0, len(values))
	used := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		id := value.ID()
		if !value.Available() || !id.Available() {
			return nil, nil, false
		}
		if _, exists := result[id]; exists {
			return nil, nil, false
		}
		slot, ok := inventory.ResolveScope(id)
		if !ok || slot == 0 {
			return nil, nil, false
		}
		if _, exists := used[slot]; exists {
			return nil, nil, false
		}
		used[slot] = struct{}{}
		result[id] = newAddress(id, slot, fence)
		ids = append(ids, id)
	}
	return result, ids, true
}

func bindExpressions(values []model.ExpressionID, inventory Inventory, fence Fence) (map[model.ExpressionID]Address[model.ExpressionID], []model.ExpressionID, bool) {
	result := make(map[model.ExpressionID]Address[model.ExpressionID], len(values))
	ids := make([]model.ExpressionID, 0, len(values))
	used := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		id := value
		if !id.Available() {
			return nil, nil, false
		}
		if _, exists := result[id]; exists {
			return nil, nil, false
		}
		slot, ok := inventory.ResolveExpression(id)
		if !ok || slot == 0 {
			return nil, nil, false
		}
		if _, exists := used[slot]; exists {
			return nil, nil, false
		}
		used[slot] = struct{}{}
		result[id] = newAddress(id, slot, fence)
		ids = append(ids, id)
	}
	return result, ids, true
}

func bindDependencies(values []model.DependencyID, inventory Inventory, fence Fence) (map[model.DependencyID]Address[model.DependencyID], []model.DependencyID, bool) {
	result := make(map[model.DependencyID]Address[model.DependencyID], len(values))
	ids := make([]model.DependencyID, 0, len(values))
	used := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		id := value
		if !id.Available() {
			return nil, nil, false
		}
		if _, exists := result[id]; exists {
			return nil, nil, false
		}
		slot, ok := inventory.ResolveDependency(id)
		if !ok || slot == 0 {
			return nil, nil, false
		}
		if _, exists := used[slot]; exists {
			return nil, nil, false
		}
		used[slot] = struct{}{}
		result[id] = newAddress(id, slot, fence)
		ids = append(ids, id)
	}
	return result, ids, true
}

func digestBook(certDigest identity.ContentID, fence Fence,
	relationIDs []model.RelationID, relations map[model.RelationID]Address[model.RelationID],
	columnIDs []model.ColumnID, columns map[model.ColumnID]Address[model.ColumnID],
	keyIDs []model.KeyID, keys map[model.KeyID]Address[model.KeyID],
	scopeIDs []model.ScopeID, scopes map[model.ScopeID]Address[model.ScopeID],
	expressionIDs []model.ExpressionID, expressions map[model.ExpressionID]Address[model.ExpressionID],
	dependencyIDs []model.DependencyID, dependencies map[model.DependencyID]Address[model.DependencyID]) (identity.ContentID, bool) {
	parts := make([][]byte, 0, 2+6+len(relationIDs)+len(columnIDs)+len(keyIDs)+len(scopeIDs)+len(expressionIDs)+len(dependencyIDs))
	parts = append(parts, contentBytes(certDigest))
	parts = append(parts, fenceParts(fence)...)
	appendRelationEntries(&parts, relationIDs, relations)
	appendColumnEntries(&parts, columnIDs, columns)
	appendKeyEntries(&parts, keyIDs, keys)
	appendScopeEntries(&parts, scopeIDs, scopes)
	appendExpressionEntries(&parts, expressionIDs, expressions)
	appendDependencyEntries(&parts, dependencyIDs, dependencies)
	return identity.DeriveContentID(bookDigestDomain, parts...)
}

func appendRelationEntries(parts *[][]byte, ids []model.RelationID, values map[model.RelationID]Address[model.RelationID]) {
	for _, id := range ids {
		appendEntry(parts, "relation", id.Owner().Content(), id.Content(), values[id].locator.Slot)
	}
}

func appendColumnEntries(parts *[][]byte, ids []model.ColumnID, values map[model.ColumnID]Address[model.ColumnID]) {
	for _, id := range ids {
		appendEntry(parts, "column", id.Owner().Content(), id.Content(), values[id].locator.Slot, id.Relation().Owner().Content(), id.Relation().Content())
	}
}

func appendKeyEntries(parts *[][]byte, ids []model.KeyID, values map[model.KeyID]Address[model.KeyID]) {
	for _, id := range ids {
		appendEntry(parts, "key", id.Owner().Content(), id.Content(), values[id].locator.Slot, id.Relation().Owner().Content(), id.Relation().Content())
	}
}

func appendScopeEntries(parts *[][]byte, ids []model.ScopeID, values map[model.ScopeID]Address[model.ScopeID]) {
	for _, id := range ids {
		appendEntry(parts, "scope", id.Owner().Content(), id.Content(), values[id].locator.Slot)
	}
}

func appendExpressionEntries(parts *[][]byte, ids []model.ExpressionID, values map[model.ExpressionID]Address[model.ExpressionID]) {
	for _, id := range ids {
		appendEntry(parts, "expression", id.Owner().Content(), id.Content(), values[id].locator.Slot)
	}
}

func appendDependencyEntries(parts *[][]byte, ids []model.DependencyID, values map[model.DependencyID]Address[model.DependencyID]) {
	for _, id := range ids {
		appendEntry(parts, "dependency", id.Owner().Content(), id.Content(), values[id].locator.Slot)
	}
}

func appendEntry(parts *[][]byte, namespace string, owner, content identity.ContentID, slot uint64, parent ...identity.ContentID) {
	var slotBytes [8]byte
	binary.BigEndian.PutUint64(slotBytes[:], slot)
	entryParts := make([][]byte, 0, 4+len(parent))
	entryParts = append(entryParts, []byte(namespace), contentBytes(owner), contentBytes(content))
	for _, value := range parent {
		entryParts = append(entryParts, contentBytes(value))
	}
	entryParts = append(entryParts, slotBytes[:])
	entry, ok := identity.DeriveContentID(entryDigestDomain, entryParts...)
	if ok {
		*parts = append(*parts, contentBytes(entry))
	}
}

func fenceParts(fence Fence) [][]byte {
	store := make([]byte, 8)
	binary.BigEndian.PutUint64(store, uint64(fence.StoreID()))
	generation := make([]byte, 8)
	binary.BigEndian.PutUint64(generation, uint64(fence.Generation()))
	mount := fence.MountID()
	return [][]byte{
		contentBytes(fence.SchemaID().Owner().Content()),
		contentBytes(fence.SchemaID().Content()),
		contentBytes(fence.CertificateDigest()),
		store,
		mount[:],
		generation,
	}
}

func contentBytes(value identity.ContentID) []byte {
	copyOf := make([]byte, len(value))
	copy(copyOf, value[:])
	return copyOf
}
