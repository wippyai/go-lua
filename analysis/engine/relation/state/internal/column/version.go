package column

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// CellVersion is one semantic terminal plus its independent lineage sidecar.
// No physical key or support mask is carried here; those belong to a streamed
// ReadPart or a candidate Update.
type CellVersion struct {
	cell    Cell
	lineage model.LineageRef
	sealed  bool
}

// NewCellVersion validates the committed semantic/lineage pair without
// attaching a physical coordinate.
func NewCellVersion(cell Cell, lineage model.LineageRef) (CellVersion, bool) {
	if !cell.Available() || !lineage.Available() {
		return CellVersion{}, false
	}
	return CellVersion{cell: cell, lineage: lineage, sealed: true}, true
}

// Available reports whether both semantic cell and lineage sidecar exist.
func (version CellVersion) Available() bool {
	if version.sealed {
		return true
	}
	return version.cell.Available() && version.lineage.Available()
}

// Cell returns the semantic payload.
func (version CellVersion) Cell() Cell { return version.cell }

// Lineage returns the independent lineage sidecar.
func (version CellVersion) Lineage() model.LineageRef { return version.lineage }

// SemanticSame excludes lineage from equality.
func (version CellVersion) SemanticSame(other CellVersion) bool {
	return version.cell.SemanticSame(other.cell)
}

// Same compares semantic payload and lineage.
func (version CellVersion) Same(other CellVersion) bool {
	return version.SemanticSame(other) && version.lineage == other.lineage
}

// Version is one immutable pair of sealed sparse diagram roots. The state
// pointer is the publication identity; roots are never mutated after seal.
type Version struct {
	column *Column
	state  *versionState
}

type versionState struct {
	parent           *versionState
	semantic         diagram.Root[uint64, geometry.Key, Cell]
	lineage          diagram.Root[uint64, geometry.Key, model.LineageRef]
	keys             []geometry.Key
	semanticRevision uint64
	lineageRevision  uint64
	sealed           bool
}

// Same reports exact immutable publication-root identity, not merely equal
// semantic contents.
func (version Version) Same(other Version) bool {
	return version.Available() && other.Available() && version.column == other.column && version.state == other.state
}

// SuccessorOf proves direct immutable ancestry. Equal revisions or equal
// roots from another Column are insufficient authority for replacement.
func (version Version) SuccessorOf(base Version) bool {
	if !version.Available() || !base.Available() || version.Same(base) || version.column != base.column || !version.Fence().Same(base.Fence()) {
		return false
	}
	return version.state.parent == base.state
}

// Available validates both sealed roots against their exact diagram owners.
func (version Version) Available() bool {
	if version.state == nil {
		return false
	}
	if version.state.sealed {
		return true
	}
	return version.column != nil && version.column.Available() && version.column.semanticGraph.Valid(version.state.semantic) && version.column.lineageGraph.Valid(version.state.lineage) && version.keysValid()
}

// sealVersion performs the complete immutable-root validation exactly once at
// construction. All successful roots thereafter carry only this private proof
// bit through the hot Available path; no diagram traversal is repeated.
func sealVersion(version Version) Version {
	if version.state == nil || !version.valid() {
		return Version{}
	}
	version.state.sealed = true
	return version
}

func (version Version) valid() bool {
	return version.column != nil && version.column.Available() && version.state != nil && version.column.semanticGraph.Valid(version.state.semantic) && version.column.lineageGraph.Valid(version.state.lineage) && version.keysValid()
}

// keysValid proves the private sparse index is canonical and that every
// enumerated semantic key has its required lineage column under the same
// immutable roots. The key slice never crosses the package boundary.
func (version Version) keysValid() bool {
	if version.state == nil || version.column == nil {
		return false
	}
	semanticCount, semanticCountValid := version.column.semanticGraph.Count(version.state.semantic)
	lineageCount, lineageCountValid := version.column.lineageGraph.Count(version.state.lineage)
	if !semanticCountValid || !lineageCountValid || semanticCount != len(version.state.keys) || lineageCount != len(version.state.keys) {
		return false
	}
	for index, key := range version.state.keys {
		if index > 0 && version.state.keys[index-1] >= key {
			return false
		}
		_, semanticPresent, semanticValid := version.column.semanticGraph.Get(version.state.semantic, factor, key)
		_, lineagePresent, lineageValid := version.column.lineageGraph.Get(version.state.lineage, factor, key)
		if !semanticValid || !lineageValid || !semanticPresent || !lineagePresent {
			return false
		}
	}
	return true
}

// Column returns the immutable logical column owner.
func (version Version) Column() *Column {
	if !version.Available() {
		return nil
	}
	return version.column
}

// Schema returns the logical schema declaration.
func (version Version) Schema() model.ColumnSchema {
	if !version.Available() {
		return model.ColumnSchema{}
	}
	return version.column.schema
}

// ID returns the owner-issued logical column identity from the immutable
// schema projection.
func (version Version) ID() model.ColumnID {
	if !version.Available() {
		return model.ColumnID{}
	}
	return version.column.schema.ID()
}

// Type returns the owner-issued semantic type identity from the schema.
func (version Version) Type() model.TypeID {
	if !version.Available() {
		return model.TypeID{}
	}
	return version.column.schema.Type()
}

// Relation returns the logical relation owning this column.
func (version Version) Relation() model.RelationID {
	if !version.Available() {
		return model.RelationID{}
	}
	return version.column.schema.Relation()
}

// Fence returns the exact schema/mount/generation read fence.
func (version Version) Fence() binding.Fence {
	if !version.Available() {
		return binding.Fence{}
	}
	return version.column.fence
}

// Guards returns the exact support manager used by both roots.
func (version Version) Guards() *guard.Manager {
	if !version.Available() {
		return nil
	}
	return version.column.guards
}

// Generation returns the runtime generation fence, distinct from semantic
// publication Revision.
func (version Version) Generation() identity.Generation {
	if !version.Available() {
		return 0
	}
	return version.column.fence.Generation()
}

// Revision advances only when semantic terminal regions change.
func (version Version) Revision() uint64 {
	if !version.Available() {
		return 0
	}
	return version.state.semanticRevision
}

// LineageRevision advances independently when lineage regions change.
func (version Version) LineageRevision() uint64 {
	if !version.Available() {
		return 0
	}
	return version.state.lineageRevision
}

// Borrow opens a no-copy read handle. Caller-owned ReadScratch supplies all
// partition storage for the read stream.
func (version Version) Borrow() (Borrowed, bool) {
	if !version.Available() {
		return Borrowed{}, false
	}
	return Borrowed{version: version, fence: version.Fence()}, true
}

// Next validates the complete batch, builds semantic and lineage candidates
// over the same key/mask writes, and seals both roots as one publication
// result. Any failed validation or candidate construction preserves the
// predecessor and returns no candidate roots.
func (version Version) Next(updates ...Update) (Version, Delta, bool) {
	if !version.Available() {
		return Version{}, Delta{}, false
	}
	if len(updates) == 0 {
		return version, emptyDelta(version), true
	}
	normalized, ok := version.normalizeBatch(updates)
	if !ok {
		return Version{}, Delta{}, false
	}
	updates = normalized

	semanticWork := version.column.semanticArena.Begin()
	lineageWork := version.column.lineageArena.Begin()
	semanticBuilder := version.column.semanticGraph.BeginWithTerminals(semanticWork)
	lineageBuilder := version.column.lineageGraph.BeginWithTerminals(lineageWork)
	if semanticWork == nil || lineageWork == nil || semanticBuilder == nil || lineageBuilder == nil {
		if semanticBuilder != nil {
			semanticBuilder.Discard()
		}
		if lineageBuilder != nil {
			lineageBuilder.Discard()
		}
		return Version{}, Delta{}, false
	}
	semanticRoot := version.state.semantic
	lineageRoot := version.state.lineage
	for _, update := range updates {
		if update.remove {
			var ok bool
			semanticRoot, ok = semanticBuilder.Delete(semanticRoot, factor, update.key, update.mask)
			if !ok {
				semanticBuilder.Discard()
				lineageBuilder.Discard()
				return Version{}, Delta{}, false
			}
			lineageRoot, ok = lineageBuilder.Delete(lineageRoot, factor, update.key, update.mask)
			if !ok {
				semanticBuilder.Discard()
				lineageBuilder.Discard()
				return Version{}, Delta{}, false
			}
			continue
		}
		semanticID, ok := semanticWork.Admit(update.cell)
		if !ok {
			semanticBuilder.Discard()
			lineageBuilder.Discard()
			return Version{}, Delta{}, false
		}
		lineageID, ok := lineageWork.Admit(update.lineage)
		if !ok {
			semanticBuilder.Discard()
			lineageBuilder.Discard()
			return Version{}, Delta{}, false
		}
		semanticRoot, ok = semanticBuilder.Set(semanticRoot, factor, update.key, update.mask, semanticID)
		if !ok {
			semanticBuilder.Discard()
			lineageBuilder.Discard()
			return Version{}, Delta{}, false
		}
		lineageRoot, ok = lineageBuilder.Set(lineageRoot, factor, update.key, update.mask, lineageID)
		if !ok {
			semanticBuilder.Discard()
			lineageBuilder.Discard()
			return Version{}, Delta{}, false
		}
	}
	semanticRoot, semanticOK := semanticBuilder.Seal(semanticRoot)
	lineageRoot, lineageOK := lineageBuilder.Seal(lineageRoot)
	if !semanticOK || !lineageOK {
		if !semanticOK {
			semanticBuilder.Discard()
		}
		if !lineageOK {
			lineageBuilder.Discard()
		}
		return Version{}, Delta{}, false
	}
	semanticChanged := semanticRoot != version.state.semantic
	lineageChanged := lineageRoot != version.state.lineage
	if !semanticChanged && !lineageChanged {
		return version, emptyDelta(version), true
	}
	semanticRevision, lineageRevision := version.state.semanticRevision, version.state.lineageRevision
	if semanticChanged {
		if semanticRevision == ^uint64(0) {
			return Version{}, Delta{}, false
		}
		semanticRevision++
	}
	if lineageChanged {
		if lineageRevision == ^uint64(0) {
			return Version{}, Delta{}, false
		}
		lineageRevision++
	}
	keys, keysOK := version.successorKeys(updates, semanticRoot, lineageRoot)
	if !keysOK {
		return Version{}, Delta{}, false
	}
	next := sealVersion(Version{column: version.column, state: &versionState{parent: version.state, semantic: semanticRoot, lineage: lineageRoot, keys: keys, semanticRevision: semanticRevision, lineageRevision: lineageRevision}})
	if !next.Available() {
		return Version{}, Delta{}, false
	}
	delta, ok := next.makeDelta(version, updates)
	if !ok {
		return Version{}, Delta{}, false
	}
	return next, delta, true
}

func (version Version) normalizeBatch(updates []Update) ([]Update, bool) {
	type candidate struct {
		update   Update
		identity guard.FormulaID
	}
	candidates := make([]candidate, len(updates))
	for index, update := range updates {
		if !version.column.accept(update) {
			return nil, false
		}
		identity, ok := update.mask.Identity()
		if !ok {
			return nil, false
		}
		candidates[index] = candidate{update: update, identity: identity}
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].update.key != candidates[right].update.key {
			return candidates[left].update.key < candidates[right].update.key
		}
		return bytes.Compare(candidates[left].identity[:], candidates[right].identity[:]) < 0
	})
	accepted := make([]candidate, 0, len(candidates))
	for _, current := range candidates {
		duplicate := false
		for _, prior := range accepted {
			if prior.update.key != current.update.key {
				continue
			}
			overlap, ok := support.Intersect(prior.update.mask, current.update.mask)
			if !ok {
				return nil, false
			}
			if support.Empty(overlap) {
				continue
			}
			if prior.update.mask.Equal(current.update.mask) && prior.update.remove == current.update.remove && (current.update.remove || (prior.update.cell.SemanticSame(current.update.cell) && prior.update.lineage == current.update.lineage)) {
				duplicate = true
				break
			}
			return nil, false
		}
		if !duplicate {
			accepted = append(accepted, current)
		}
	}
	normalized := make([]Update, len(accepted))
	for index, current := range accepted {
		normalized[index] = current.update
	}
	return normalized, true
}

func (column *Column) acceptCell(cell Cell) bool {
	if !cell.Available() {
		return false
	}
	if cell.value.Available() {
		return cell.value.ValidFor(column.fence) && cell.value.Type() == column.schema.Type()
	}
	return !cell.value.Available()
}

func (column *Column) accept(update Update) bool {
	if column == nil || !column.Available() || !update.mask.Valid() || support.Empty(update.mask) || update.mask.Manager() != column.guards {
		return false
	}
	if update.remove {
		return !update.cell.Available() && !update.lineage.Available()
	}
	return column.acceptCell(update.cell) && update.lineage.Available()
}

func (version Version) successorKeys(updates []Update, semanticRoot diagram.Root[uint64, geometry.Key, Cell], lineageRoot diagram.Root[uint64, geometry.Key, model.LineageRef]) ([]geometry.Key, bool) {
	keys := append([]geometry.Key(nil), version.state.keys...)
	for _, update := range updates {
		keys = append(keys, update.key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	unique := 0
	for _, key := range keys {
		if unique == 0 || keys[unique-1] != key {
			keys[unique] = key
			unique++
		}
	}
	keys = keys[:unique]
	result := make([]geometry.Key, 0, len(keys))
	for _, key := range keys {
		_, semanticPresent, semanticOK := version.column.semanticGraph.Get(semanticRoot, factor, key)
		_, lineagePresent, lineageOK := version.column.lineageGraph.Get(lineageRoot, factor, key)
		if !semanticOK || !lineageOK || semanticPresent != lineagePresent {
			return nil, false
		}
		if semanticPresent {
			result = append(result, key)
		}
	}
	return result, true
}

func emptyDelta(version Version) Delta {
	return sealDelta(Delta{base: version, next: version, fence: version.Fence(), guards: version.Guards(), fromRevision: version.Revision(), toRevision: version.Revision()})
}

// makeDelta partitions predecessor and successor semantic and lineage roots
// over the union of every updated support region. The semantic stream remains
// available for index maintenance; the unified stream retains every atomic
// extent where either side changed.
func (version Version) makeDelta(base Version, updates []Update) (Delta, bool) {
	regions := make(map[geometry.Key]support.Mask, len(updates))
	keys := make([]geometry.Key, 0, len(updates))
	work := support.New(version.column.guards)
	if work == nil {
		return Delta{}, false
	}
	for _, update := range updates {
		prior, exists := regions[update.key]
		if !exists {
			regions[update.key] = update.mask
			keys = append(keys, update.key)
			continue
		}
		merged, ok := support.UnionWithWork(work, nil, prior, update.mask)
		if !ok {
			work.Close()
			return Delta{}, false
		}
		regions[update.key] = merged
	}
	work.Close()
	for left := 0; left < len(keys); left++ {
		for right := left + 1; right < len(keys); right++ {
			if keys[right] < keys[left] {
				keys[left], keys[right] = keys[right], keys[left]
			}
		}
	}
	entries := make([]DeltaEntry, 0, len(updates))
	for _, key := range keys {
		if !version.appendDeltaForKey(base, key, regions[key], &entries) {
			return Delta{}, false
		}
	}
	sort.Slice(entries, func(left, right int) bool { return deltaEntryLess(entries[left], entries[right]) })
	delta := sealDelta(Delta{base: base, next: version, fence: version.Fence(), guards: version.Guards(), fromRevision: base.Revision(), toRevision: version.Revision(), entries: entries})
	return delta, delta.Available()
}

func (version Version) appendDeltaForKey(base Version, key geometry.Key, region support.Mask, entries *[]DeltaEntry) bool {
	oldValue, oldPresent, oldValid := version.column.semanticGraph.Get(base.state.semantic, factor, key)
	newValue, newPresent, newValid := version.column.semanticGraph.Get(version.state.semantic, factor, key)
	oldLineage, oldLineagePresent, oldLineageValid := version.column.lineageGraph.Get(base.state.lineage, factor, key)
	newLineage, newLineagePresent, newLineageValid := version.column.lineageGraph.Get(version.state.lineage, factor, key)
	if !oldValid || !newValid || !oldLineageValid || !newLineageValid || oldPresent != oldLineagePresent || newPresent != newLineagePresent || !region.Valid() || region.Manager() != version.column.guards {
		return false
	}
	oldParts := make([]semanticPartition, 0, 4)
	newParts := make([]semanticPartition, 0, 4)
	oldLineageParts := make([]lineagePartition, 0, 4)
	newLineageParts := make([]lineagePartition, 0, 4)
	work := support.New(version.column.guards)
	if work == nil {
		return false
	}
	if oldPresent {
		if completed, valid := version.column.semanticGraph.PartitionValueTerminals(oldValue, region, work, func(id terminal.ID[Cell], part support.Mask) bool {
			if support.Empty(part) {
				return true
			}
			oldParts = append(oldParts, semanticPartition{id: id, region: part})
			return true
		}); !completed || !valid {
			work.Close()
			return false
		}
	} else {
		oldParts = append(oldParts, semanticPartition{region: region})
	}
	if newPresent {
		if completed, valid := version.column.semanticGraph.PartitionValueTerminals(newValue, region, work, func(id terminal.ID[Cell], part support.Mask) bool {
			if support.Empty(part) {
				return true
			}
			newParts = append(newParts, semanticPartition{id: id, region: part})
			return true
		}); !completed || !valid {
			work.Close()
			return false
		}
	} else {
		newParts = append(newParts, semanticPartition{region: region})
	}
	if oldLineagePresent {
		if completed, valid := version.column.lineageGraph.PartitionValueTerminals(oldLineage, region, work, func(id terminal.ID[model.LineageRef], part support.Mask) bool {
			if support.Empty(part) {
				return true
			}
			oldLineageParts = append(oldLineageParts, lineagePartition{id: id, region: part})
			return true
		}); !completed || !valid {
			work.Close()
			return false
		}
	} else {
		oldLineageParts = append(oldLineageParts, lineagePartition{region: region})
	}
	if newLineagePresent {
		if completed, valid := version.column.lineageGraph.PartitionValueTerminals(newLineage, region, work, func(id terminal.ID[model.LineageRef], part support.Mask) bool {
			if support.Empty(part) {
				return true
			}
			newLineageParts = append(newLineageParts, lineagePartition{id: id, region: part})
			return true
		}); !completed || !valid {
			work.Close()
			return false
		}
	} else {
		newLineageParts = append(newLineageParts, lineagePartition{region: region})
	}

	// Refine the predecessor and successor semantic partitions with their
	// independent lineage partitions before intersecting the two roots. This
	// is the atomic extent law: one support region is emitted once even when
	// both semantic and lineage terminals change together.
	for _, before := range oldParts {
		for _, beforeLineage := range oldLineageParts {
			beforeRegion, ok := support.IntersectWithWork(work, nil, before.region, beforeLineage.region)
			if !ok {
				work.Close()
				return false
			}
			if support.Empty(beforeRegion) {
				continue
			}
			for _, after := range newParts {
				for _, afterLineage := range newLineageParts {
					overlap, ok := support.IntersectWithWork(work, nil, beforeRegion, after.region)
					if !ok {
						work.Close()
						return false
					}
					overlap, ok = support.IntersectWithWork(work, nil, overlap, afterLineage.region)
					if !ok {
						work.Close()
						return false
					}
					if support.Empty(overlap) {
						continue
					}
					semanticChanged := !version.semanticIDsEqual(before.id, after.id)
					lineageChanged := !version.lineageIDsEqual(beforeLineage.id, afterLineage.id)
					if !semanticChanged && !lineageChanged {
						continue
					}
					beforeCell, beforeOK := version.semanticValue(before.id)
					afterCell, afterOK := version.semanticValue(after.id)
					beforeLineageValue, beforeLineageOK := version.lineageValue(beforeLineage.id)
					afterLineageValue, afterLineageOK := version.lineageValue(afterLineage.id)
					entry := DeltaEntry{key: key, region: overlap, before: beforeCell, beforePresent: beforeOK, after: afterCell, afterPresent: afterOK, beforeLineage: beforeLineageValue, beforeLineagePresent: beforeLineageOK, afterLineage: afterLineageValue, afterLineagePresent: afterLineageOK}
					*entries = append(*entries, entry)
				}
			}
		}
	}
	work.Close()
	return true
}

type semanticPartition struct {
	id     terminal.ID[Cell]
	region support.Mask
}

func (version Version) semanticIDsEqual(left, right terminal.ID[Cell]) bool {
	if left == (terminal.ID[Cell]{}) || right == (terminal.ID[Cell]{}) {
		return left == right
	}
	return version.column.semanticArena.Equal(left, right)
}

func (version Version) lineageIDsEqual(left, right terminal.ID[model.LineageRef]) bool {
	if left == (terminal.ID[model.LineageRef]{}) || right == (terminal.ID[model.LineageRef]{}) {
		return left == right
	}
	return version.column.lineageArena.Equal(left, right)
}

func (version Version) semanticValue(id terminal.ID[Cell]) (Cell, bool) {
	if id == (terminal.ID[Cell]{}) {
		return Cell{}, false
	}
	return version.column.semanticArena.Value(id)
}

func (version Version) lineageValue(id terminal.ID[model.LineageRef]) (model.LineageRef, bool) {
	if id == (terminal.ID[model.LineageRef]{}) {
		return model.LineageRef{}, false
	}
	return version.column.lineageArena.Value(id)
}
