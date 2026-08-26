package index

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// mountedRowAt redeems the mounted relation-local directory for one geometry
// coordinate. Geometry keys are dense coordinates, but their logical meaning
// is owned exclusively by Mounted; no denominator-local or scalar identity is
// reconstructed here.
func mountedRowAt(mounted witness.Mounted, relation model.RelationID, key geometry.Key) (model.RowID, bool) {
	maxInt := uint64(^uint(0) >> 1)
	if uint64(key) > maxInt {
		return model.RowID{}, false
	}
	return mounted.RowAt(relation, int(key))
}

// materialize resolves the sealed layout into canonical rows. A keyed layout
// reads the owner-declared key columns and intersects their support fibers at
// identical geometry keys. An unkeyed semantic layout unions its delivered
// columns. An empty relation layout is different: it is the owner directory
// itself and therefore scans mounted RowIDs without fabricating payloads.
func materialize(mounted witness.Mounted, state store.Version, layout arrangement.Layout, within support.Mask, scratch *store.ReadScratch) ([]row, bool) {
	keyColumns := layout.KeyColumns()
	if len(keyColumns) != 0 {
		parts := make([]map[geometry.Key][]scanPart, len(keyColumns))
		for position, id := range keyColumns {
			value, ok := scanColumn(state, id, within, scratch, mounted.RuntimeFence())
			if !ok {
				return nil, false
			}
			parts[position] = value
		}
		return joinKeyParts(parts, within.Manager(), mounted, layout.Access().Relation())
	}
	// A relation access is the sealed owner-directory scan. It deliberately
	// has neither key nor delivered columns: its rows come only from the
	// mounted relation directory, while the requested cofiber is the posting
	// scope. Do not turn this into a semantic-column scan or synthesize a key
	// value; the geometry key below is only the directory coordinate needed to
	// redeem the owner-issued RowID.
	if len(layout.Columns()) == 0 {
		return materializeRelationDirectory(mounted, state, layout.Access().Relation(), within, scratch)
	}
	return materializeUnkeyed(state, layout.Columns(), within, scratch, mounted.RuntimeFence(), mounted, layout.Access().Relation())
}

// materializeRelationDirectory enumerates the mounted owner directory for a
// relation, restricted to rows actually committed in the aggregate root. The
// coordinate is only a dense directory address; semantic key and payload
// values remain owned by their declared columns and are never synthesized by
// this index. The first-class relation layout has no delivered columns, so its
// membership proof comes from the present state partitions of the relation's
// declared columns and only the owner-issued RowID is emitted.
func materializeRelationDirectory(mounted witness.Mounted, state store.Version, relation model.RelationID, within support.Mask, scratch *store.ReadScratch) ([]row, bool) {
	if !mounted.Available() || !state.Available() || !relation.Available() || !within.Valid() || within.Manager() == nil || scratch == nil || !scratch.Available() || scratch.Manager() != within.Manager() {
		return nil, false
	}
	// An empty physical cofiber is a valid empty view, not an executable
	// posting. Keep the result non-nil so the immutable empty trie remains a
	// truthful relation scan.
	if support.Empty(within) {
		return []row{}, true
	}
	byKey := make(map[geometry.Key]support.Mask)
	foundColumn := false
	for _, declaration := range mounted.Columns() {
		if declaration.Relation() != relation {
			continue
		}
		foundColumn = true
		projectionValid := true
		completed, valid := state.Scan(declaration.ID(), within, scratch, func(part store.ReadPart) bool {
			if !part.Region().Valid() || support.Empty(part.Region()) || part.Region().Manager() != within.Manager() || !part.Presence().Available() {
				projectionValid = false
				return false
			}
			// Proven absence is denominator evidence, not row membership. An
			// opaque present cell still proves that the owner row exists, while
			// its semantic payload remains entirely outside this layout.
			if !part.Presence().Is(model.Present) && !part.Presence().Is(model.AuthenticatedOpaque) {
				return true
			}
			prior, exists := byKey[part.Key()]
			if !exists {
				byKey[part.Key()] = part.Region()
				return true
			}
			merged, mergeOK := support.Union(prior, part.Region())
			if !mergeOK {
				projectionValid = false
				return false
			}
			byKey[part.Key()] = merged
			return true
		})
		if !completed || !valid || !projectionValid {
			return nil, false
		}
	}
	if !foundColumn {
		return []row{}, true
	}
	keys := make([]geometry.Key, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	rows := make([]row, 0, len(keys))
	for _, key := range keys {
		logical, ok := mountedRowAt(mounted, relation, key)
		if !ok {
			return nil, false
		}
		region := byKey[key]
		if !region.Valid() || support.Empty(region) || region.Manager() != within.Manager() {
			return nil, false
		}
		rows = append(rows, row{key: key, relation: relation, logical: logical, region: region})
	}
	return canonicalRowsWithEquality(mounted, rows)
}

func materializeUnkeyed(state store.Version, columns []model.ColumnID, within support.Mask, scratch *store.ReadScratch, fence binding.Fence, mounted witness.Mounted, relation model.RelationID) ([]row, bool) {
	byKey := make(map[geometry.Key]support.Mask)
	for _, id := range columns {
		parts, ok := scanColumn(state, id, within, scratch, fence)
		if !ok {
			return nil, false
		}
		for _, part := range flattenParts(parts) {
			prior, exists := byKey[part.key]
			if !exists {
				byKey[part.key] = part.region
				continue
			}
			merged, mergeOK := support.Union(prior, part.region)
			if !mergeOK {
				return nil, false
			}
			byKey[part.key] = merged
		}
	}
	keys := make([]geometry.Key, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	rows := make([]row, 0, len(keys))
	for _, key := range keys {
		logical, rowOK := mountedRowAt(mounted, relation, key)
		if !rowOK {
			return nil, false
		}
		region := byKey[key]
		if !region.Valid() || support.Empty(region) || region.Manager() != within.Manager() {
			return nil, false
		}
		rows = append(rows, row{key: key, relation: relation, logical: logical, region: region})
	}
	canonical, ok := canonicalRowsWithEquality(mounted, rows)
	if !ok {
		return nil, false
	}
	return canonical, true
}

func scanColumn(state store.Version, id model.ColumnID, within support.Mask, scratch *store.ReadScratch, fence binding.Fence) (map[geometry.Key][]scanPart, bool) {
	parts := make(map[geometry.Key][]scanPart)
	projectionValid := true
	completed, valid := state.Scan(id, within, scratch, func(part store.ReadPart) bool {
		if !part.Region().Valid() || support.Empty(part.Region()) || part.Region().Manager() != within.Manager() || !part.Presence().Available() {
			projectionValid = false
			return false
		}
		value := part.Value()
		if value.Available() && !value.ValidFor(fence) {
			projectionValid = false
			return false
		}
		parts[part.Key()] = append(parts[part.Key()], scanPart{key: part.Key(), region: part.Region(), value: value})
		return true
	})
	return parts, completed && valid && projectionValid
}

func flattenParts(parts map[geometry.Key][]scanPart) []scanPart {
	keys := make([]geometry.Key, 0, len(parts))
	for key := range parts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	result := make([]scanPart, 0)
	for _, key := range keys {
		values := parts[key]
		sort.SliceStable(values, func(left, right int) bool { return scanPartLess(values[left], values[right]) })
		result = append(result, values...)
	}
	return result
}

func joinKeyParts(parts []map[geometry.Key][]scanPart, manager *guard.Manager, mounted witness.Mounted, relation model.RelationID) ([]row, bool) {
	if len(parts) == 0 || manager == nil {
		return nil, false
	}
	rows := make([]row, 0)
	keys := make([]geometry.Key, 0, len(parts[0]))
	for key := range parts[0] {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	for _, key := range keys {
		logical, rowOK := mountedRowAt(mounted, relation, key)
		if !rowOK {
			return nil, false
		}
		var visit func(position int, region support.Mask, values []binding.ValueToken) bool
		visit = func(position int, region support.Mask, values []binding.ValueToken) bool {
			if position == len(parts) {
				rows = append(rows, row{key: key, relation: relation, logical: logical, region: region, values: append([]binding.ValueToken(nil), values...)})
				return true
			}
			for _, part := range parts[position][key] {
				if !part.value.Available() {
					continue
				}
				overlap, ok := support.Intersect(region, part.region)
				if !ok || support.Empty(overlap) {
					if !ok {
						return false
					}
					continue
				}
				nextValues := append(values, part.value)
				if !visit(position+1, overlap, nextValues) {
					return false
				}
			}
			return true
		}
		if len(parts[0][key]) == 0 {
			continue
		}
		for _, first := range parts[0][key] {
			if !first.value.Available() {
				continue
			}
			if !visit(1, first.region, []binding.ValueToken{first.value}) {
				return nil, false
			}
		}
	}
	canonical, ok := canonicalRowsWithEquality(mounted, rows)
	if !ok {
		return nil, false
	}
	return canonical, true
}
