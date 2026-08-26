package store

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Version is one immutable aggregate solve-local staging root. Its private
// map owns exactly one canonical column.Version for each certified ColumnID.
// The map is copied only while staging; unchanged column versions are shared
// by identity between roots. Database owns visible W2 publication.
type Version struct {
	state *state
}

type state struct {
	parent            *state
	fence             binding.Fence
	mountedDigest     identity.ContentID
	arrangementDigest identity.ContentID
	ids               []model.ColumnID
	columns           map[model.ColumnID]column.Version
	revision          uint64
	sealed            bool
}

// NewVersion authenticates one complete aggregate from the exact mounted
// catalogue and one initial column version for every canonical mounted ID.
// No caller-owned slice or map is retained. The mounted capability is the
// only public catalogue door; newVersion is the package-local validator used
// by laws without introducing a second public catalogue type.
func NewVersion(mounted witness.Mounted, initial []column.Version) (Version, bool) {
	if !mounted.Available() {
		return Version{}, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() || !mounted.Digest().Available() || !plan.Digest().Available() {
		return Version{}, false
	}
	return newVersion(mounted.RuntimeFence(), mounted.Digest(), plan.Digest(), mounted.ColumnIDs(), initial)
}

func newVersion(fence binding.Fence, mountedDigest, arrangementDigest identity.ContentID, catalogueIDs []model.ColumnID, initial []column.Version) (Version, bool) {
	if !fence.Available() || !mountedDigest.Available() || !arrangementDigest.Available() {
		return Version{}, false
	}
	ids, ok := canonicalCatalogueIDs(catalogueIDs)
	if !ok || initial == nil || len(initial) != len(ids) {
		return Version{}, false
	}

	columns := make(map[model.ColumnID]column.Version, len(ids))
	for _, value := range initial {
		if !value.Available() || !value.Fence().Same(fence) {
			return Version{}, false
		}
		id := value.ID()
		if !id.Available() || !containsColumnID(ids, id) || value.Relation() != id.Relation() {
			return Version{}, false
		}
		if _, duplicate := columns[id]; duplicate {
			return Version{}, false
		}
		columns[id] = value
	}
	for _, id := range ids {
		if _, present := columns[id]; !present {
			return Version{}, false
		}
	}

	value := sealVersion(Version{state: &state{
		fence: fence, mountedDigest: mountedDigest,
		arrangementDigest: arrangementDigest, ids: ids, columns: columns, revision: 1,
	}})
	if !value.Available() {
		return Version{}, false
	}
	return value, true
}

// Available reports whether version retains a complete immutable aggregate.
func (version Version) Available() bool {
	if version.state != nil && version.state.sealed {
		return true
	}
	return version.valid()
}

func (version Version) valid() bool {
	if version.state == nil || !version.state.fence.Available() || !version.state.mountedDigest.Available() || !version.state.arrangementDigest.Available() || version.state.ids == nil || version.state.columns == nil || version.state.revision == 0 || len(version.state.ids) != len(version.state.columns) {
		return false
	}
	if !isCanonicalIDs(version.state.ids) {
		return false
	}
	for _, id := range version.state.ids {
		value, ok := version.state.columns[id]
		if !ok || !value.Available() || value.ID() != id || value.Relation() != id.Relation() || !value.Fence().Same(version.state.fence) {
			return false
		}
	}
	return true
}

// sealVersion performs the complete aggregate validation at construction.
// Published roots are immutable, so every later Available call is a single
// private-proof read rather than a catalogue/column traversal.
func sealVersion(version Version) Version {
	if version.state == nil || !version.valid() {
		return Version{}
	}
	version.state.sealed = true
	return version
}

// Same reports exact aggregate publication-root identity.
func (version Version) Same(other Version) bool {
	return version.Available() && other.Available() && version.state == other.state
}

// SuccessorOf proves direct aggregate ancestry. Revision equality is never
// used as an ancestry proof: two forks may share every revision number.
func (version Version) SuccessorOf(base Version) bool {
	return version.Available() && base.Available() && !version.Same(base) && version.Fence().Same(base.Fence()) && version.state.parent == base.state
}

// Fence returns the exact runtime authority captured by this aggregate.
func (version Version) Fence() binding.Fence {
	if !version.Available() {
		return binding.Fence{}
	}
	return version.state.fence
}

// MountedDigest returns the exact mounted capability identity captured when
// this aggregate was bootstrapped.  It is part of every prepared/committed
// root fence; a matching binding.Fence alone is insufficient authority.
func (version Version) MountedDigest() identity.ContentID {
	if !version.Available() {
		return identity.ContentID{}
	}
	return version.state.mountedDigest
}

// ArrangementDigest returns the exact physical arrangement identity captured
// by the mounted root.  Arrangement identity is mandatory even when a
// particular transaction changes only semantic columns.
func (version Version) ArrangementDigest() identity.ContentID {
	if !version.Available() {
		return identity.ContentID{}
	}
	return version.state.arrangementDigest
}

// Revision returns the aggregate publication revision. One successful
// multi-column aggregate publication advances it exactly once.
func (version Version) Revision() uint64 {
	if !version.Available() {
		return 0
	}
	return version.state.revision
}

// Column resolves one exact certified logical column. Missing or foreign IDs
// are explicit misses; no default value is manufactured.
func (version Version) Column(id model.ColumnID) (column.Version, bool) {
	if !version.Available() || !id.Available() {
		return column.Version{}, false
	}
	value, ok := version.state.columns[id]
	if !ok || !value.Available() || value.ID() != id || !value.Fence().Same(version.state.fence) {
		return column.Version{}, false
	}
	return value, true
}

// ColumnIDs returns the canonical certified logical IDs as a defensive copy.
func (version Version) ColumnIDs() []model.ColumnID {
	if !version.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), version.state.ids...)
}

// Prepared is an opaque, immutable aggregate candidate. It owns the exact
// predecessor and candidate successor roots but does not publish either root.
// The database composition owner redeems it only while preparing a complete
// aggregate publication.
type Prepared struct {
	base              Version
	next              Version
	delta             Delta
	mountedDigest     identity.ContentID
	arrangementDigest identity.ContentID
	noop              bool
	sealed            bool
}

// Available authenticates a staged candidate without publishing it. Mounted
// and arrangement identities are mandatory even for a lineage-only change.
func (prepared Prepared) Available() bool {
	if prepared.sealed {
		return true
	}
	return prepared.valid()
}

func (prepared Prepared) valid() bool {
	if !prepared.base.Available() || !prepared.next.Available() || !prepared.mountedDigest.Available() || !prepared.arrangementDigest.Available() || prepared.base.MountedDigest() != prepared.mountedDigest || prepared.next.MountedDigest() != prepared.mountedDigest || prepared.base.ArrangementDigest() != prepared.arrangementDigest || prepared.next.ArrangementDigest() != prepared.arrangementDigest {
		return false
	}
	if prepared.noop {
		return prepared.base.Same(prepared.next) && !prepared.delta.Available()
	}
	return prepared.delta.Available() && prepared.delta.Base().Same(prepared.base) && prepared.delta.Next().Same(prepared.next)
}

func sealPrepared(prepared Prepared) Prepared {
	if prepared.valid() {
		prepared.sealed = true
	}
	return prepared
}

// Base returns the exact aggregate root from which this candidate was staged.
func (prepared Prepared) Base() Version {
	if !prepared.Available() {
		return Version{}
	}
	return prepared.base
}

// Delta returns the exact staged aggregate projection for downstream
// arrangement preparation. The projection is immutable and does not publish
// or replace the candidate root.
func (prepared Prepared) Delta() Delta {
	if !prepared.Available() || prepared.noop {
		return Delta{}
	}
	return prepared.delta
}

// MountedDigest returns the mounted identity authenticated by the candidate.
func (prepared Prepared) MountedDigest() identity.ContentID {
	if !prepared.Available() {
		return identity.ContentID{}
	}
	return prepared.mountedDigest
}

// ArrangementDigest returns the arrangement identity authenticated by the
// candidate. It is never inferred from the column delta.
func (prepared Prepared) ArrangementDigest() identity.ContentID {
	if !prepared.Available() {
		return identity.ContentID{}
	}
	return prepared.arrangementDigest
}

// Empty reports whether this candidate is the exact immutable no-op root.
func (prepared Prepared) Empty() bool { return prepared.Available() && prepared.noop }

// Prepare stages one or more sealed canonical column deltas without changing
// any published aggregate root. The candidate retains exact predecessor and
// successor roots for database.Prepare/Commit to authenticate at the sole
// publication door. A zero-delta prepare is an explicit immutable no-op.
func Prepare(version Version, changes ...column.Delta) (Prepared, bool) {
	if !version.Available() {
		return Prepared{}, false
	}
	if len(changes) == 0 {
		prepared := sealPrepared(Prepared{base: version, next: version, mountedDigest: version.MountedDigest(), arrangementDigest: version.ArrangementDigest(), noop: true})
		return prepared, prepared.Available()
	}
	next, delta, ok := version.prepareRoots(changes...)
	if !ok {
		return Prepared{}, false
	}
	prepared := sealPrepared(Prepared{base: version, next: next, delta: delta, mountedDigest: version.MountedDigest(), arrangementDigest: version.ArrangementDigest()})
	if !prepared.Available() {
		return Prepared{}, false
	}
	return prepared, true
}

// prepareRoots constructs private candidate roots and their aggregate delta.
// It is intentionally package-private: all public publication flows go
// through Prepare followed by the database publication owner.
func (version Version) prepareRoots(changes ...column.Delta) (Version, Delta, bool) {
	if !version.Available() || len(changes) == 0 || version.state.revision == ^uint64(0) {
		return Version{}, Delta{}, false
	}

	type candidate struct {
		delta column.Delta
		base  column.Version
		next  column.Version
		id    model.ColumnID
	}
	candidates := make([]candidate, 0, len(changes))
	seen := make(map[model.ColumnID]struct{}, len(changes))
	for _, change := range changes {
		if !change.Available() || !change.Fence().Same(version.state.fence) {
			return Version{}, Delta{}, false
		}
		base, next := change.Base(), change.Next()
		if !base.Available() || !next.Available() || !next.SuccessorOf(base) {
			return Version{}, Delta{}, false
		}
		id := change.ColumnID()
		if !id.Available() || id != base.ID() || id != next.ID() || change.RelationID() != id.Relation() || base.Relation() != id.Relation() || next.Relation() != id.Relation() {
			return Version{}, Delta{}, false
		}
		if change.FromRevision() != base.Revision() || change.ToRevision() != next.Revision() {
			return Version{}, Delta{}, false
		}
		if _, duplicate := seen[id]; duplicate {
			return Version{}, Delta{}, false
		}
		seen[id] = struct{}{}
		current, present := version.state.columns[id]
		if !present || !current.Same(base) || current.ID() != id || !current.Fence().Same(version.state.fence) {
			return Version{}, Delta{}, false
		}
		if !next.Fence().Same(version.state.fence) || next.Column() != current.Column() {
			return Version{}, Delta{}, false
		}
		semanticChanged := next.Revision() != base.Revision()
		lineageChanged := next.LineageRevision() != base.LineageRevision()
		if !semanticChanged && !lineageChanged {
			return Version{}, Delta{}, false
		}
		if change.Empty() {
			// A direct column successor must expose at least one changed atomic
			// extent. Semantic and lineage revisions are not a substitute for
			// the canonical stream itself.
			return Version{}, Delta{}, false
		}
		candidates = append(candidates, candidate{delta: change, base: base, next: next, id: id})
	}

	columns := make(map[model.ColumnID]column.Version, len(version.state.columns))
	for id, value := range version.state.columns {
		columns[id] = value
	}
	changed := make([]model.ColumnID, 0, len(candidates))
	semantic := make([]model.ColumnID, 0, len(candidates))
	lineage := make([]model.ColumnID, 0, len(candidates))
	for _, candidate := range candidates {
		columns[candidate.id] = candidate.next
		changed = append(changed, candidate.id)
		if candidate.next.Revision() != candidate.base.Revision() {
			semantic = append(semantic, candidate.id)
		}
		if candidate.next.LineageRevision() != candidate.base.LineageRevision() {
			lineage = append(lineage, candidate.id)
		}
	}
	sortColumnIDs(changed)
	sortColumnIDs(semantic)
	sortColumnIDs(lineage)

	next := sealVersion(Version{state: &state{
		parent: version.state, fence: version.state.fence, mountedDigest: version.state.mountedDigest,
		arrangementDigest: version.state.arrangementDigest, ids: version.state.ids,
		columns: columns, revision: version.state.revision + 1,
	}})
	if !next.Available() {
		return Version{}, Delta{}, false
	}
	columnChanges := make([]ColumnChange, 0, len(candidates))
	for _, candidate := range candidates {
		change, ok := projectColumnChange(candidate.delta)
		if !ok {
			return Version{}, Delta{}, false
		}
		columnChanges = append(columnChanges, change)
	}
	sort.Slice(columnChanges, func(left, right int) bool {
		return compareColumnID(columnChanges[left].column, columnChanges[right].column) < 0
	})
	delta := newDelta(version, next, changed, semantic, lineage, columnChanges)
	if !delta.Available() {
		return Version{}, Delta{}, false
	}
	return next, delta, true
}

// Delta is the aggregate publication difference between exact immutable
// roots. It retains one deterministic column change projection; semantic and
// lineage classification remains available as predicates over that stream.
type Delta struct {
	base     Version
	next     Version
	changed  []model.ColumnID
	semantic []model.ColumnID
	lineage  []model.ColumnID
	changes  []ColumnChange
	sealed   bool
}

func newDelta(base, next Version, changed, semantic, lineage []model.ColumnID, changes []ColumnChange) Delta {
	columnChanges := append([]ColumnChange(nil), changes...)
	for index := range columnChanges {
		bound, ok := columnChanges[index].bindRoots(base, next)
		if !ok {
			return Delta{}
		}
		columnChanges[index] = bound
	}
	return sealDelta(Delta{
		base: base, next: next,
		changed:  append([]model.ColumnID(nil), changed...),
		semantic: append([]model.ColumnID(nil), semantic...),
		lineage:  append([]model.ColumnID(nil), lineage...),
		changes:  columnChanges,
	})
}

// Available authenticates exact aggregate roots, direct ancestry, complete
// catalogue membership, unchanged sharing, and the semantic/lineage
// partition. It does not trust revision numbers as identity.
func (delta Delta) Available() bool {
	if delta.sealed {
		return true
	}
	return delta.valid()
}

func (delta Delta) valid() bool {
	if !delta.base.Available() || !delta.next.Available() || !delta.next.SuccessorOf(delta.base) || delta.next.Revision() != delta.base.Revision()+1 || !delta.base.Fence().Same(delta.next.Fence()) || !equalColumnIDSets(delta.changed, delta.semantic, delta.lineage) || len(delta.changed) == 0 || len(delta.changes) != len(delta.changed) {
		return false
	}
	for index, projected := range delta.changes {
		if !projected.Available() || projected.ColumnID() != delta.changed[index] || projected.Empty() || !projected.Base().Same(delta.base) || !projected.Next().Same(delta.next) || !projected.Fence().Same(delta.base.Fence()) {
			return false
		}
	}
	for _, id := range delta.changed {
		before, beforeOK := delta.base.Column(id)
		after, afterOK := delta.next.Column(id)
		if !beforeOK || !afterOK || !after.SuccessorOf(before) {
			return false
		}
		semanticChanged := containsColumnID(delta.semantic, id)
		lineageChanged := containsColumnID(delta.lineage, id)
		if !semanticChanged && !lineageChanged {
			return false
		}
		if semanticChanged != (after.Revision() != before.Revision()) || lineageChanged != (after.LineageRevision() != before.LineageRevision()) {
			return false
		}
	}
	for _, id := range delta.base.state.ids {
		if containsColumnID(delta.changed, id) {
			continue
		}
		before, beforeOK := delta.base.Column(id)
		after, afterOK := delta.next.Column(id)
		if !beforeOK || !afterOK || !after.Same(before) {
			return false
		}
	}
	return true
}

func sealDelta(delta Delta) Delta {
	if delta.valid() {
		delta.sealed = true
	}
	return delta
}

// Base returns the exact aggregate predecessor root authenticated by delta.
func (delta Delta) Base() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.base
}

// Next returns the exact aggregate successor root authenticated by delta.
func (delta Delta) Next() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.next
}

// ChangedColumnIDs returns all changed IDs in canonical order.
func (delta Delta) ChangedColumnIDs() []model.ColumnID {
	if !delta.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), delta.changed...)
}

// SemanticColumnIDs returns IDs with at least one semantic-changed entry in
// the canonical change stream.
func (delta Delta) SemanticColumnIDs() []model.ColumnID {
	if !delta.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), delta.semantic...)
}

// LineageColumnIDs returns every ID whose lineage changed. The set is
// independent of SemanticColumnIDs and may overlap it when one ascent changes
// both payload and provenance.
func (delta Delta) LineageColumnIDs() []model.ColumnID {
	if !delta.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), delta.lineage...)
}

// Changes returns one canonical semantic+lineage projection for every
// changed column, including lineage-only columns. Each projection is exact
// to this Delta's aggregate Base/Next roots and emits atomic key/mask extents
// without duplicating logical row or scope identities.
func (delta Delta) Changes() []ColumnChange {
	if !delta.Available() {
		return nil
	}
	return append([]ColumnChange(nil), delta.changes...)
}

// Change resolves one changed column in the complete semantic+
// lineage stream. A missing result means the column was unchanged.
func (delta Delta) Change(id model.ColumnID) (ColumnChange, bool) {
	if !delta.Available() || !id.Available() {
		return ColumnChange{}, false
	}
	for _, value := range delta.changes {
		if value.ColumnID() == id {
			return value, true
		}
	}
	return ColumnChange{}, false
}

func canonicalCatalogueIDs(input []model.ColumnID) ([]model.ColumnID, bool) {
	if input == nil {
		return nil, false
	}
	ids := make([]model.ColumnID, len(input))
	copy(ids, input)
	for index, id := range ids {
		if !id.Available() {
			return nil, false
		}
		for _, prior := range ids[:index] {
			if prior == id {
				return nil, false
			}
		}
	}
	sortColumnIDs(ids)
	return ids, true
}

func containsColumnID(ids []model.ColumnID, wanted model.ColumnID) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}

func sortColumnIDs(ids []model.ColumnID) {
	sort.Slice(ids, func(left, right int) bool { return compareColumnID(ids[left], ids[right]) < 0 })
}

func compareColumnID(left, right model.ColumnID) int {
	leftOwner, rightOwner := left.Relation().Owner().Content(), right.Relation().Owner().Content()
	if result := bytes.Compare(leftOwner[:], rightOwner[:]); result != 0 {
		return result
	}
	leftRelation, rightRelation := left.Relation().Content(), right.Relation().Content()
	if result := bytes.Compare(leftRelation[:], rightRelation[:]); result != 0 {
		return result
	}
	leftColumn, rightColumn := left.Content(), right.Content()
	return bytes.Compare(leftColumn[:], rightColumn[:])
}

func equalColumnIDSets(changed, semantic, lineage []model.ColumnID) bool {
	if !isCanonicalIDs(changed) || !isCanonicalIDs(semantic) || !isCanonicalIDs(lineage) {
		return false
	}
	union := make(map[model.ColumnID]struct{}, len(semantic)+len(lineage))
	for _, id := range semantic {
		if !containsColumnID(changed, id) {
			return false
		}
		union[id] = struct{}{}
	}
	for _, id := range lineage {
		if !containsColumnID(changed, id) {
			return false
		}
		union[id] = struct{}{}
	}
	return len(union) == len(changed)
}

func isCanonicalIDs(ids []model.ColumnID) bool {
	for index, id := range ids {
		if !id.Available() || (index > 0 && compareColumnID(ids[index-1], id) >= 0) {
			return false
		}
	}
	return true
}
