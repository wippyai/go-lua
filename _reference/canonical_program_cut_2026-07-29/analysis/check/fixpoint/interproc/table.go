package interproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
)

// InstanceKey is the exact identity of one demanded, concrete body instance.
// Its semantic bytes are exactly the body artifact content ID and the
// canonical projected entry.  The complete artifact bytes are retained only
// to reject a (cryptographic) ContentID collision during table lookup.
type InstanceKey struct {
	artifact          ContentID
	projection        []byte
	artifactCanonical []byte
}

// NewInstanceKey forms an exact key after projecting entry through artifact's
// frozen read certificate.  It intentionally has no full-entry or generation
// fallback.
func NewInstanceKey(artifact DemandedBodyArtifact, entry EntryBinding) (InstanceKey, error) {
	artifactCanonical := artifact.CanonicalBytes()
	if artifactCanonical == nil || !artifact.ContentID().Valid() {
		return InstanceKey{}, fmt.Errorf("interproc: malformed demanded body artifact")
	}
	projection, err := artifact.ReadCertificate().Project(entry)
	if err != nil {
		return InstanceKey{}, err
	}
	projectionBytes := projection.CanonicalBytes()
	if projectionBytes == nil {
		return InstanceKey{}, fmt.Errorf("interproc: malformed projected entry")
	}
	return InstanceKey{
		artifact:          artifact.ContentID(),
		projection:        append([]byte(nil), projectionBytes...),
		artifactCanonical: append([]byte(nil), artifactCanonical...),
	}, nil
}

func (k InstanceKey) ArtifactID() ContentID { return k.artifact }
func (k InstanceKey) ProjectionBytes() []byte {
	return append([]byte(nil), k.projection...)
}
func (k InstanceKey) ProjectionID() ContentID {
	if !k.valid() {
		return ContentID{}
	}
	return contentID(k.projection)
}

// CanonicalBytes is the portable instance identity.  It excludes the retained
// artifact collision witness, which is not part of the specified key.
func (k InstanceKey) CanonicalBytes() []byte {
	if !k.valid() {
		return nil
	}
	out := appendText(nil, "interproc-instance-key/content-v1")
	out = append(out, k.artifact[:]...)
	return appendBytes(out, k.projection)
}

func (k InstanceKey) ContentID() ContentID {
	encoded := k.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

func (k InstanceKey) valid() bool {
	return k.artifact.Valid() && k.projection != nil && k.artifactCanonical != nil
}

// equal confirms both retained canonical byte surfaces after the hash lookup.
// Never change this to ContentID-only equality: a digest is an index, not a
// permission to merge two distinct canonical values.
func (k InstanceKey) equal(other InstanceKey) bool {
	return k.artifact == other.artifact &&
		bytes.Equal(k.artifactCanonical, other.artifactCanonical) &&
		bytes.Equal(k.projection, other.projection)
}

// ClosedOutcome is the sealed, caller-context-free transport kept by the
// table.  The summaryinstance codec owns the meaning and validation of these
// bytes; this layer accepts no State, callback, diagnostic span, or local
// allocation handle.
type ClosedOutcome struct {
	canonical []byte
	id        ContentID
}

func NewClosedOutcome(canonical []byte) (ClosedOutcome, error) {
	if len(canonical) == 0 {
		return ClosedOutcome{}, fmt.Errorf("interproc: empty portable closed outcome")
	}
	cloned := append([]byte(nil), canonical...)
	return ClosedOutcome{canonical: cloned, id: contentID(cloned)}, nil
}

func (o ClosedOutcome) Valid() bool {
	return len(o.canonical) != 0 && o.id.Valid() && o.id == contentID(o.canonical)
}
func (o ClosedOutcome) CanonicalBytes() []byte { return append([]byte(nil), o.canonical...) }
func (o ClosedOutcome) ContentID() ContentID   { return o.id }

// DirectCallRunner is the owner-only integration point for a direct call.  A
// runner binds the complete entry to one canonical VM transaction on a miss,
// then returns its already closed portable result and every observed entry
// read. It must not use a caller context when producing outcome.
type DirectCallRunner func(context.Context, DemandedBodyArtifact, EntryBinding) (ClosedOutcome, []ReadObservation, error)

// DirectCall routes a direct interprocedural call through the exact table.
// It is deliberately small so later VM binding supplies a runner instead of
// inventing an alternate symbolic specialization path.
type DirectCall struct {
	Table  *ProjectedTable
	Runner DirectCallRunner
}

func (c DirectCall) Resolve(ctx context.Context, artifact DemandedBodyArtifact, entry EntryBinding) (ClosedOutcome, error) {
	if c.Table == nil {
		return ClosedOutcome{}, fmt.Errorf("interproc: direct call has no projected table")
	}
	return c.Table.Resolve(ctx, artifact, entry, c.Runner)
}

// CacheMetrics reports table activity.  Misses and Executions count owner
// transactions; joins are equal-key callers that waited for that owner.
type CacheMetrics struct {
	Lookups, Misses, Hits, Joins, Executions, Failures, Evictions uint64
	Cells                                                         uint64
}

type tableCell struct {
	key         InstanceKey
	artifact    *artifactIndex
	done        chan struct{}
	closed      bool
	completed   bool
	invalidated bool
	outcome     ClosedOutcome
	err         error
	callees     []InstanceKey
}

// DependencySnapshotResolver resolves the current immutable identity for one
// dependency authority. It is deliberately content-addressed: a mutable
// generation may wake an owner, but cannot satisfy this check or enter a key.
type DependencySnapshotResolver interface {
	ResolveContentID(context.Context, Dependency) (ContentID, error)
}

// DependencySnapshotFunc adapts a content snapshot function for table setup.
type DependencySnapshotFunc func(context.Context, Dependency) (ContentID, error)

func (f DependencySnapshotFunc) ResolveContentID(ctx context.Context, dependency Dependency) (ContentID, error) {
	return f(ctx, dependency)
}

// DependencySnapshotMismatchError means a manifest snapshot no longer names
// the current content. The old result is not safe to publish or reuse.
type DependencySnapshotMismatchError struct {
	Dependency Dependency
	Current    ContentID
}

func (e *DependencySnapshotMismatchError) Error() string {
	return fmt.Sprintf("interproc: dependency %q changed content identity", e.Dependency.Kind)
}

// ErrInstanceInvalidated is returned to an owner or joiner whose in-flight
// cell was evicted before it could publish.
var ErrInstanceInvalidated = errors.New("interproc: instance invalidated before publication")

// artifactIndex is the second reverse-index hop: a manifest content ID names
// an artifact, and the artifact owns its exact instance cells. Canonical bytes
// remain with the index so a digest collision cannot merge artifacts.
type artifactIndex struct {
	id        ContentID
	canonical []byte
	deps      []Dependency
	instances map[ContentID][]*tableCell
}

// ProjectedTable holds exact instances.  Buckets are indexed by a content
// digest, but entries are always confirmed by canonical bytes before reuse.
type ProjectedTable struct {
	mu                  sync.Mutex
	buckets             map[ContentID][]*tableCell
	artifacts           map[ContentID][]*artifactIndex
	dependencyArtifacts map[ContentID][]*artifactIndex
	calleeCallers       map[ContentID][]*tableCell
	resolver            DependencySnapshotResolver
	metrics             CacheMetrics
	bucket              func([]byte) ContentID
}

func NewProjectedTable() *ProjectedTable { return newProjectedTable(contentID) }

// NewProjectedTableWithDependencyResolver enables live snapshot validation at
// lookup, commit, and explicit stale scans. A nil resolver is rejected by this
// constructor; callers with immutable, externally invalidated snapshots use
// NewProjectedTable and InvalidateDependency.
func NewProjectedTableWithDependencyResolver(resolver DependencySnapshotResolver) *ProjectedTable {
	if resolver == nil {
		return nil
	}
	table := newProjectedTable(contentID)
	table.resolver = resolver
	return table
}

func newProjectedTable(bucket func([]byte) ContentID) *ProjectedTable {
	if bucket == nil {
		bucket = contentID
	}
	return &ProjectedTable{
		buckets:             make(map[ContentID][]*tableCell),
		artifacts:           make(map[ContentID][]*artifactIndex),
		dependencyArtifacts: make(map[ContentID][]*artifactIndex),
		calleeCallers:       make(map[ContentID][]*tableCell),
		bucket:              bucket,
	}
}

// Metrics returns a stable snapshot; it never exposes cells or their private
// full entry bindings.
func (t *ProjectedTable) Metrics() CacheMetrics {
	if t == nil {
		return CacheMetrics{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	metrics := t.metrics
	for _, cells := range t.buckets {
		metrics.Cells += uint64(len(cells))
	}
	return metrics
}

// Resolve projects entry before lookup.  Missing projection coordinates and
// incomplete read certificates are terminal errors; neither is retried with a
// broader entry.  Only the elected owner sees entry and executes runner.
func (t *ProjectedTable) Resolve(ctx context.Context, artifact DemandedBodyArtifact, entry EntryBinding, runner DirectCallRunner) (ClosedOutcome, error) {
	if t == nil || runner == nil || ctx == nil {
		return ClosedOutcome{}, fmt.Errorf("interproc: invalid projected table resolution")
	}
	if err := ctx.Err(); err != nil {
		return ClosedOutcome{}, err
	}
	if err := t.validateDependencies(ctx, artifact.Dependencies()); err != nil {
		return ClosedOutcome{}, err
	}
	key, err := NewInstanceKey(artifact, entry)
	if err != nil {
		return ClosedOutcome{}, err
	}
	bucketID := t.bucket(key.CanonicalBytes())

	t.mu.Lock()
	t.metrics.Lookups++
	for _, cell := range t.buckets[bucketID] {
		if !cell.key.equal(key) {
			continue
		}
		if cell.closed {
			t.metrics.Hits++
			outcome := cell.outcome
			t.mu.Unlock()
			return outcome, nil
		}
		t.metrics.Joins++
		done := cell.done
		t.mu.Unlock()
		select {
		case <-done:
			if cell.err != nil {
				return ClosedOutcome{}, cell.err
			}
			if !cell.outcome.Valid() {
				return ClosedOutcome{}, fmt.Errorf("interproc: invalid joined portable outcome")
			}
			return cell.outcome, nil
		case <-ctx.Done():
			return ClosedOutcome{}, ctx.Err()
		}
	}

	indexedArtifact := t.indexArtifactLocked(artifact, key.artifactCanonical)
	cell := &tableCell{key: key, artifact: indexedArtifact, done: make(chan struct{})}
	t.buckets[bucketID] = append(t.buckets[bucketID], cell)
	indexedArtifact.instances[key.ContentID()] = append(indexedArtifact.instances[key.ContentID()], cell)
	t.metrics.Misses++
	t.metrics.Executions++
	t.mu.Unlock()

	outcome, observed, runErr := runner(ctx, artifact, entry)
	if runErr == nil {
		runErr = artifact.ReadCertificate().VerifyReadAudit(observed)
	}
	if runErr == nil && !outcome.Valid() {
		runErr = fmt.Errorf("interproc: runner returned an invalid portable closed outcome")
	}
	if runErr == nil {
		if err := ctx.Err(); err != nil {
			runErr = err
		}
	}
	if runErr == nil {
		runErr = t.validateDependencies(ctx, artifact.Dependencies())
	}

	t.mu.Lock()
	if cell.invalidated {
		t.mu.Unlock()
		return ClosedOutcome{}, ErrInstanceInvalidated
	}
	if runErr != nil {
		t.failCellLocked(bucketID, cell, runErr)
		t.metrics.Failures++
		t.mu.Unlock()
		return ClosedOutcome{}, runErr
	}
	cell.outcome = outcome
	cell.closed = true
	cell.completed = true
	close(cell.done)
	t.mu.Unlock()
	return outcome, nil
}

// InvalidateDependency evicts precisely the instances whose manifest contains
// id, then follows exact callee-to-caller edges transitively. It is a
// reclamation/promptness operation only; id is never part of a cache key.
func (t *ProjectedTable) InvalidateDependency(id ContentID) int {
	if t == nil || !id.Valid() {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.invalidateDependenciesLocked([]ContentID{id})
}

// InvalidateStale compares every indexed manifest snapshot with the current
// resolver and evicts all mismatches. Resolver failures also evict their
// dependent cells, because an unverified snapshot may not remain reusable.
func (t *ProjectedTable) InvalidateStale(ctx context.Context) (int, error) {
	if t == nil || t.resolver == nil || ctx == nil {
		return 0, fmt.Errorf("interproc: stale invalidation requires a dependency resolver and context")
	}
	t.mu.Lock()
	artifacts := make([]*artifactIndex, 0)
	for _, candidates := range t.artifacts {
		artifacts = append(artifacts, candidates...)
	}
	t.mu.Unlock()

	stale := make([]ContentID, 0)
	var firstErr error
	for _, artifact := range artifacts {
		for _, dependency := range artifact.deps {
			current, err := t.resolver.ResolveContentID(ctx, dependency)
			if err != nil || !current.Valid() || current != dependency.ID {
				stale = append(stale, dependency.ID)
				if firstErr == nil && err != nil {
					firstErr = fmt.Errorf("interproc: resolve dependency %q: %w", dependency.Kind, err)
				}
				if firstErr == nil && !current.Valid() {
					firstErr = fmt.Errorf("interproc: dependency resolver returned an invalid content identity for %q", dependency.Kind)
				}
			}
		}
	}
	t.mu.Lock()
	evicted := t.invalidateDependenciesLocked(stale)
	t.mu.Unlock()
	return evicted, firstErr
}

// LinkCallee records an exact instance dependency after both cells exist. The
// relation is metadata for invalidation; it never changes a portable outcome
// or permits a projection/content-ID-only merge.
func (t *ProjectedTable) LinkCallee(caller, callee InstanceKey) error {
	if t == nil || !caller.valid() || !callee.valid() {
		return fmt.Errorf("interproc: invalid callee link")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	callerCell := t.cellForKeyLocked(caller)
	if callerCell == nil || callerCell.invalidated || !callerCell.closed {
		return fmt.Errorf("interproc: caller instance is not committed")
	}
	if calleeCell := t.cellForKeyLocked(callee); calleeCell == nil || calleeCell.invalidated {
		return fmt.Errorf("interproc: callee instance is not indexed")
	}
	for _, existing := range callerCell.callees {
		if existing.equal(callee) {
			return nil
		}
	}
	callerCell.callees = append(callerCell.callees, callee)
	calleeID := callee.ContentID()
	t.calleeCallers[calleeID] = append(t.calleeCallers[calleeID], callerCell)
	return nil
}

func (t *ProjectedTable) validateDependencies(ctx context.Context, manifest DependencyManifest) error {
	if t.resolver == nil {
		return nil
	}
	for _, dependency := range manifest.Dependencies() {
		current, err := t.resolver.ResolveContentID(ctx, dependency)
		if err != nil {
			return fmt.Errorf("interproc: resolve dependency %q: %w", dependency.Kind, err)
		}
		if !current.Valid() || current != dependency.ID {
			return &DependencySnapshotMismatchError{Dependency: dependency, Current: current}
		}
	}
	return nil
}

func (t *ProjectedTable) indexArtifactLocked(artifact DemandedBodyArtifact, canonical []byte) *artifactIndex {
	id := artifact.ContentID()
	for _, candidate := range t.artifacts[id] {
		if bytes.Equal(candidate.canonical, canonical) {
			return candidate
		}
	}
	indexed := &artifactIndex{
		id:        id,
		canonical: append([]byte(nil), canonical...),
		deps:      artifact.Dependencies().Dependencies(),
		instances: make(map[ContentID][]*tableCell),
	}
	t.artifacts[id] = append(t.artifacts[id], indexed)
	for _, dependency := range indexed.deps {
		t.dependencyArtifacts[dependency.ID] = append(t.dependencyArtifacts[dependency.ID], indexed)
	}
	return indexed
}

func (t *ProjectedTable) cellForKeyLocked(key InstanceKey) *tableCell {
	for _, cell := range t.buckets[t.bucket(key.CanonicalBytes())] {
		if cell.key.equal(key) {
			return cell
		}
	}
	return nil
}

func (t *ProjectedTable) invalidateDependenciesLocked(ids []ContentID) int {
	queue := make([]*tableCell, 0)
	seen := make(map[*tableCell]struct{})
	for _, id := range ids {
		for _, artifact := range t.dependencyArtifacts[id] {
			for _, cells := range artifact.instances {
				for _, cell := range cells {
					if _, present := seen[cell]; !present {
						seen[cell] = struct{}{}
						queue = append(queue, cell)
					}
				}
			}
		}
	}
	for index := 0; index < len(queue); index++ {
		cell := queue[index]
		for _, caller := range t.calleeCallers[cell.key.ContentID()] {
			if caller.invalidated || !callerDependsOn(caller, cell.key) {
				continue
			}
			if _, present := seen[caller]; !present {
				seen[caller] = struct{}{}
				queue = append(queue, caller)
			}
		}
	}
	for _, cell := range queue {
		if cell.invalidated {
			continue
		}
		t.evictCellLocked(cell)
	}
	return len(queue)
}

func callerDependsOn(caller *tableCell, callee InstanceKey) bool {
	for _, candidate := range caller.callees {
		if candidate.equal(callee) {
			return true
		}
	}
	return false
}

func (t *ProjectedTable) evictCellLocked(cell *tableCell) {
	bucketID := t.bucket(cell.key.CanonicalBytes())
	t.remove(bucketID, cell)
	t.removeArtifactInstanceLocked(cell)
	t.removeCallerLinksLocked(cell)
	cell.invalidated = true
	if !cell.completed {
		cell.err = ErrInstanceInvalidated
		cell.completed = true
		close(cell.done)
	}
	t.metrics.Evictions++
}

func (t *ProjectedTable) failCellLocked(bucketID ContentID, cell *tableCell, err error) {
	cell.err = err
	cell.completed = true
	t.remove(bucketID, cell)
	t.removeArtifactInstanceLocked(cell)
	t.removeCallerLinksLocked(cell)
	close(cell.done)
}

func (t *ProjectedTable) removeArtifactInstanceLocked(cell *tableCell) {
	artifact := cell.artifact
	if artifact == nil {
		return
	}
	id := cell.key.ContentID()
	cells := artifact.instances[id]
	for index, candidate := range cells {
		if candidate != cell {
			continue
		}
		copy(cells[index:], cells[index+1:])
		cells[len(cells)-1] = nil
		cells = cells[:len(cells)-1]
		if len(cells) == 0 {
			delete(artifact.instances, id)
		} else {
			artifact.instances[id] = cells
		}
		break
	}
	if len(artifact.instances) != 0 {
		return
	}
	t.removeArtifactLocked(artifact)
}

func (t *ProjectedTable) removeArtifactLocked(want *artifactIndex) {
	removeArtifact := func(items []*artifactIndex) []*artifactIndex {
		for index, artifact := range items {
			if artifact != want {
				continue
			}
			copy(items[index:], items[index+1:])
			items[len(items)-1] = nil
			return items[:len(items)-1]
		}
		return items
	}
	items := removeArtifact(t.artifacts[want.id])
	if len(items) == 0 {
		delete(t.artifacts, want.id)
	} else {
		t.artifacts[want.id] = items
	}
	for _, dependency := range want.deps {
		items := removeArtifact(t.dependencyArtifacts[dependency.ID])
		if len(items) == 0 {
			delete(t.dependencyArtifacts, dependency.ID)
		} else {
			t.dependencyArtifacts[dependency.ID] = items
		}
	}
}

func (t *ProjectedTable) removeCallerLinksLocked(cell *tableCell) {
	for _, callee := range cell.callees {
		id := callee.ContentID()
		callers := t.calleeCallers[id]
		for index := 0; index < len(callers); index++ {
			if callers[index] != cell {
				continue
			}
			copy(callers[index:], callers[index+1:])
			callers[len(callers)-1] = nil
			callers = callers[:len(callers)-1]
			index--
		}
		if len(callers) == 0 {
			delete(t.calleeCallers, id)
		} else {
			t.calleeCallers[id] = callers
		}
	}
	cell.callees = nil
}

func (t *ProjectedTable) remove(bucketID ContentID, want *tableCell) {
	cells := t.buckets[bucketID]
	for index, cell := range cells {
		if cell != want {
			continue
		}
		copy(cells[index:], cells[index+1:])
		cells[len(cells)-1] = nil
		cells = cells[:len(cells)-1]
		if len(cells) == 0 {
			delete(t.buckets, bucketID)
		} else {
			t.buckets[bucketID] = cells
		}
		return
	}
}
