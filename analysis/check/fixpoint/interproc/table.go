package interproc

import (
	"bytes"
	"context"
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
	Lookups, Misses, Hits, Joins, Executions, Failures uint64
	Cells                                              uint64
}

type tableCell struct {
	key     InstanceKey
	done    chan struct{}
	closed  bool
	outcome ClosedOutcome
	err     error
}

// ProjectedTable holds exact instances.  Buckets are indexed by a content
// digest, but entries are always confirmed by canonical bytes before reuse.
type ProjectedTable struct {
	mu      sync.Mutex
	buckets map[ContentID][]*tableCell
	metrics CacheMetrics
	bucket  func([]byte) ContentID
}

func NewProjectedTable() *ProjectedTable { return newProjectedTable(contentID) }

func newProjectedTable(bucket func([]byte) ContentID) *ProjectedTable {
	if bucket == nil {
		bucket = contentID
	}
	return &ProjectedTable{buckets: make(map[ContentID][]*tableCell), bucket: bucket}
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

	cell := &tableCell{key: key, done: make(chan struct{})}
	t.buckets[bucketID] = append(t.buckets[bucketID], cell)
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

	t.mu.Lock()
	if runErr != nil {
		cell.err = runErr
		t.remove(bucketID, cell)
		t.metrics.Failures++
		close(cell.done)
		t.mu.Unlock()
		return ClosedOutcome{}, runErr
	}
	cell.outcome = outcome
	cell.closed = true
	close(cell.done)
	t.mu.Unlock()
	return outcome, nil
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
