package interproc

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

// FastInstanceKey is the fixed-width primary index used by RuntimeCache.  It
// is only an index: every matching slot still confirms the two canonical byte
// witnesses before its result can be reused.
type FastInstanceKey struct {
	ArtifactID   ContentID
	ProjectionID ContentID
}

// SummaryInstanceRef is the actor-facing reference to a shared summary
// instance. It is deliberately three scalar words: actors never retain a Go
// pointer to an artifact, cache cell, closure, or callback. Artifact and
// instance handles are resolved through their owning slabs, while Generation
// carries invalidation/lease state.
type SummaryInstanceRef struct {
	ArtifactHandle uint32
	InstanceHandle uint32
	Generation     uint32
}

// SummaryInstanceSlab retains scalar actor-to-instance references in one
// contiguous backing array. Its capacity is fixed at creation: exhaustion is
// visible to the owner instead of silently allocating a pointer-bearing graph.
type SummaryInstanceSlab struct {
	references []SummaryInstanceRef
	requested  uint64
	highWater  uint64
}

// NewSummaryInstanceSlab reserves capacity for actor references. Negative
// capacities are treated as zero so callers can safely propagate a measured
// configuration value.
func NewSummaryInstanceSlab(capacity int) *SummaryInstanceSlab {
	if capacity < 0 {
		capacity = 0
	}
	return &SummaryInstanceSlab{references: make([]SummaryInstanceRef, 0, capacity)}
}

// Retain appends one scalar reference and returns false when the preallocated
// slab is full. It never grows the slab.
func (s *SummaryInstanceSlab) Retain(reference SummaryInstanceRef) bool {
	if s == nil || len(s.references) == cap(s.references) {
		return false
	}
	s.references = append(s.references, reference)
	bytes := uint64(len(s.references)) * uint64(unsafe.Sizeof(SummaryInstanceRef{}))
	s.requested = bytes
	if bytes > s.highWater {
		s.highWater = bytes
	}
	return true
}

// Len reports retained actor references.
func (s *SummaryInstanceSlab) Len() int {
	if s == nil {
		return 0
	}
	return len(s.references)
}

// RequestedBytes reports currently retained scalar reference storage.
func (s *SummaryInstanceSlab) RequestedBytes() uint64 {
	if s == nil {
		return 0
	}
	return s.requested
}

// HighWaterBytes reports the largest scalar reference payload retained by the
// slab since construction.
func (s *SummaryInstanceSlab) HighWaterBytes() uint64 {
	if s == nil {
		return 0
	}
	return s.highWater
}

// Valid reports whether both halves of a primary key are available.
func (k FastInstanceKey) Valid() bool { return k.ArtifactID.Valid() && k.ProjectionID.Valid() }

// NewFastInstanceKey derives the fixed key from complete canonical witnesses.
func NewFastInstanceKey(artifactCanonical, projectionCanonical []byte) FastInstanceKey {
	return FastInstanceKey{
		ArtifactID:   ContentIDFromCanonicalBytes(artifactCanonical),
		ProjectionID: ContentIDFromCanonicalBytes(projectionCanonical),
	}
}

// EvaluatorScratch is a worker-owned scalar projection arena. Projection
// bytes are valid until the next Reset or Pop. It intentionally stores no
// entry, artifact, or outcome pointers, so Reset cannot retain a caller graph.
type EvaluatorScratch struct {
	projection []byte
	used       uint32
	frames     []uint32
	overflows  uint64
}

// NewEvaluatorScratch reserves normal-path projection and re-entrant frame
// capacity at worker start. Zero capacities are accepted for callers that
// deliberately exercise the metered overflow path.
func NewEvaluatorScratch(projectionCapacity, frameCapacity int) *EvaluatorScratch {
	if projectionCapacity < 0 {
		projectionCapacity = 0
	}
	if frameCapacity < 0 {
		frameCapacity = 0
	}
	return &EvaluatorScratch{
		projection: make([]byte, projectionCapacity),
		frames:     make([]uint32, 0, frameCapacity),
	}
}

// Reset rewinds the scalar arena. There are no pointer-bearing temporaries to
// retain; the byte arena is deliberately reused.
func (s *EvaluatorScratch) Reset() {
	if s == nil {
		return
	}
	s.used = 0
	s.frames = s.frames[:0]
}

// Push starts a nested scratch frame. It returns false rather than growing the
// frame stack, making re-entrant capacity exhaustion a visible cold-path event.
func (s *EvaluatorScratch) Push() bool {
	if s == nil || len(s.frames) == cap(s.frames) {
		if s != nil {
			s.overflows++
		}
		return false
	}
	s.frames = append(s.frames, s.used)
	return true
}

// Pop discards the current nested frame.
func (s *EvaluatorScratch) Pop() bool {
	if s == nil || len(s.frames) == 0 {
		return false
	}
	last := len(s.frames) - 1
	s.used = s.frames[last]
	s.frames = s.frames[:last]
	return true
}

// OverflowCount reports explicit normal-capacity failures.
func (s *EvaluatorScratch) OverflowCount() uint64 {
	if s == nil {
		return 0
	}
	return s.overflows
}

var ErrProjectionScratchOverflow = errors.New("interproc: projection scratch capacity exceeded")

func (s *EvaluatorScratch) reserve(length int) ([]byte, error) {
	if s == nil || length < 0 || uint64(s.used)+uint64(length) > uint64(len(s.projection)) {
		if s != nil {
			s.overflows++
		}
		return nil, ErrProjectionScratchOverflow
	}
	start := s.used
	s.used += uint32(length)
	return s.projection[start:s.used], nil
}

// ScratchProjectionEncoder is admission-built from a frozen artifact. It
// writes exactly the certificate-selected canonical projection into a caller
// supplied worker scratch without maps, clones, or interface conversions.
type ScratchProjectionEncoder struct {
	artifactID        ContentID
	artifactCanonical []byte
	selectors         []EntrySelector
	demand            DemandKey
	initialBytes      int
}

// NewScratchProjectionEncoder freezes the artifact witness and sorted selector
// list once. This is an admission/cold operation; Encode is the hot operation.
func NewScratchProjectionEncoder(artifact DemandedBodyArtifact) (*ScratchProjectionEncoder, error) {
	canonical := artifact.CanonicalBytes()
	id := artifact.ContentID()
	if canonical == nil || !id.Valid() {
		return nil, fmt.Errorf("interproc: malformed demanded body artifact")
	}
	selectors := artifact.ReadCertificate().Selectors()
	for i, selector := range selectors {
		if !selector.valid() || i != 0 && selectors[i-1] >= selector {
			return nil, fmt.Errorf("interproc: malformed read certificate")
		}
	}
	return &ScratchProjectionEncoder{
		artifactID:        id,
		artifactCanonical: append([]byte(nil), canonical...),
		selectors:         selectors,
		demand:            artifact.DemandKey(),
		initialBytes:      8 + len("interproc-entry-projection/content-v1") + 8,
	}, nil
}

func (e *ScratchProjectionEncoder) ArtifactID() ContentID {
	if e == nil {
		return ContentID{}
	}
	return e.artifactID
}

// ArtifactCanonicalBytes returns a defensive copy. Runtime callers should use
// EncodeWithArtifactWitness when they need the allocation-free internal view.
func (e *ScratchProjectionEncoder) ArtifactCanonicalBytes() []byte {
	if e == nil {
		return nil
	}
	return append([]byte(nil), e.artifactCanonical...)
}

// Encode writes the canonical projection into scratch and returns its fixed
// primary key. The returned bytes alias scratch and must be consumed before the
// next scratch reset/pop. Missing values remain fail-closed.
func (e *ScratchProjectionEncoder) Encode(scratch *EvaluatorScratch, entry EntryBinding) (FastInstanceKey, []byte, error) {
	if e == nil || !e.artifactID.Valid() {
		return FastInstanceKey{}, nil, fmt.Errorf("interproc: nil scratch projection encoder")
	}
	if scratch == nil {
		return FastInstanceKey{}, nil, ErrProjectionScratchOverflow
	}
	start := scratch.used
	needed := e.initialBytes
	valueAt := 0
	for _, selector := range e.selectors {
		for valueAt < len(entry.values) && entry.values[valueAt].Selector < selector {
			valueAt++
		}
		if valueAt == len(entry.values) || entry.values[valueAt].Selector != selector || entry.values[valueAt].Encoding == nil {
			return FastInstanceKey{}, nil, &IncompleteReadCertificateError{Demand: e.demand, Selector: selector}
		}
		needed += 8 + len(selector) + 8 + len(entry.values[valueAt].Encoding)
	}
	buf, err := scratch.reserve(needed)
	if err != nil {
		return FastInstanceKey{}, nil, err
	}
	write := 0
	writeProjectionBytes(buf, &write, "interproc-entry-projection/content-v1")
	writeU64Fixed(buf, &write, uint64(len(e.selectors)))
	valueAt = 0
	for _, selector := range e.selectors {
		for entry.values[valueAt].Selector < selector {
			valueAt++
		}
		value := entry.values[valueAt]
		writeProjectionBytes(buf, &write, string(selector))
		writeProjectionBytesRaw(buf, &write, value.Encoding)
	}
	projection := scratch.projection[start:scratch.used]
	return FastInstanceKey{ArtifactID: e.artifactID, ProjectionID: contentID(projection)}, projection, nil
}

func writeU64Fixed(dst []byte, at *int, value uint64) {
	dst[*at+0] = byte(value >> 56)
	dst[*at+1] = byte(value >> 48)
	dst[*at+2] = byte(value >> 40)
	dst[*at+3] = byte(value >> 32)
	dst[*at+4] = byte(value >> 24)
	dst[*at+5] = byte(value >> 16)
	dst[*at+6] = byte(value >> 8)
	dst[*at+7] = byte(value)
	*at += 8
}

func writeProjectionBytes(dst []byte, at *int, value string) {
	writeU64Fixed(dst, at, uint64(len(value)))
	*at += copy(dst[*at:], value)
}

func writeProjectionBytesRaw(dst []byte, at *int, value []byte) {
	writeU64Fixed(dst, at, uint64(len(value)))
	*at += copy(dst[*at:], value)
}

// RuntimeCacheMetrics separates hot hits from cold publication and joins.
type RuntimeCacheMetrics struct {
	Lookups, Hits, Misses, Joins, Publications, Invalidations uint64
}

type runtimeCacheMetrics struct {
	lookups, hits, misses, joins, publications, invalidations atomic.Uint64
}

type runtimeCacheSlot struct {
	key                                             FastInstanceKey
	artifactOffset, projectionOffset, outcomeOffset uint32
	artifactLength, projectionLength, outcomeLength uint32
	occupied                                        bool
}

// runtimeCacheSnapshot is immutable after publication. Slots contain scalar
// offsets into arena rather than pointers to per-instance object graphs.
type runtimeCacheSnapshot struct {
	slots []runtimeCacheSlot
	arena []byte
	used  uint32
}

type runtimeCacheFlight struct {
	done       chan struct{}
	artifact   []byte
	projection []byte
	err        error
}

type runtimeCacheShard struct {
	mu       sync.Mutex // cold publication, resize, invalidation, and flights only
	snapshot atomic.Pointer[runtimeCacheSnapshot]
	flights  map[FastInstanceKey][]*runtimeCacheFlight
}

// RuntimeCache is a sharded scalar-slot cache. Its hot Load path takes no
// table mutex and performs only snapshot lookup plus canonical confirmation.
type RuntimeCache struct {
	shards  []runtimeCacheShard
	mask    uint64
	metrics runtimeCacheMetrics
}

// NewRuntimeCache constructs a cache with power-of-two shard count. Slot
// growth is cold and snapshots are atomically replaced, so warmed reads never
// observe map growth or a partially published cell.
func NewRuntimeCache(shardCount, initialSlots int) *RuntimeCache {
	shardCount = powerOfTwoAtLeast(shardCount, 1)
	initialSlots = powerOfTwoAtLeast(initialSlots, 4)
	c := &RuntimeCache{shards: make([]runtimeCacheShard, shardCount), mask: uint64(shardCount - 1)}
	for i := range c.shards {
		c.shards[i].snapshot.Store(&runtimeCacheSnapshot{slots: make([]runtimeCacheSlot, initialSlots)})
	}
	return c
}

func powerOfTwoAtLeast(value, minimum int) int {
	if value < minimum {
		value = minimum
	}
	value--
	for shift := 1; shift < 64; shift <<= 1 {
		value |= value >> shift
	}
	return value + 1
}

func (c *RuntimeCache) shardFor(key FastInstanceKey) *runtimeCacheShard {
	if c == nil || len(c.shards) == 0 {
		return nil
	}
	return &c.shards[fastInstanceHash(key)&c.mask]
}

func fastInstanceHash(key FastInstanceKey) uint64 {
	// Both IDs participate; this is probe distribution, not identity.
	x := uint64(key.ArtifactID[0])<<56 | uint64(key.ArtifactID[7])<<48 | uint64(key.ArtifactID[15])<<40 | uint64(key.ArtifactID[23])<<32 |
		uint64(key.ProjectionID[0])<<24 | uint64(key.ProjectionID[9])<<16 | uint64(key.ProjectionID[19])<<8 | uint64(key.ProjectionID[31])
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ x>>31
}

// Load returns a retained immutable outcome only after confirming both stored
// canonical witnesses byte-for-byte. It is allocation-free after warmup.
func (c *RuntimeCache) Load(key FastInstanceKey, artifactCanonical, projection []byte) (ClosedOutcome, bool) {
	if c == nil || !key.Valid() {
		return ClosedOutcome{}, false
	}
	c.metrics.lookups.Add(1)
	shard := c.shardFor(key)
	snapshot := shard.snapshot.Load()
	for i := range snapshot.slots {
		slot := &snapshot.slots[i]
		if !slot.occupied || slot.key != key {
			continue
		}
		if !slotWitnessEqual(snapshot, *slot, artifactCanonical, projection) {
			continue
		}
		outcome := snapshot.arena[slot.outcomeOffset : slot.outcomeOffset+slot.outcomeLength]
		c.metrics.hits.Add(1)
		return ClosedOutcome{canonical: outcome, id: contentID(outcome)}, true
	}
	c.metrics.misses.Add(1)
	return ClosedOutcome{}, false
}

func slotWitnessEqual(snapshot *runtimeCacheSnapshot, slot runtimeCacheSlot, artifactCanonical, projection []byte) bool {
	artifact := snapshot.arena[slot.artifactOffset : slot.artifactOffset+slot.artifactLength]
	storedProjection := snapshot.arena[slot.projectionOffset : slot.projectionOffset+slot.projectionLength]
	return bytes.Equal(artifact, artifactCanonical) && bytes.Equal(storedProjection, projection)
}

// Store publishes a closed outcome. Witness/key mismatch is rejected before
// publication, which keeps synthetic or corrupted inputs from becoming cells.
func (c *RuntimeCache) Store(key FastInstanceKey, artifactCanonical, projection []byte, outcome ClosedOutcome) error {
	if c == nil || !key.Valid() || !outcome.Valid() ||
		ContentIDFromCanonicalBytes(artifactCanonical) != key.ArtifactID ||
		ContentIDFromCanonicalBytes(projection) != key.ProjectionID {
		return fmt.Errorf("interproc: invalid runtime cache publication")
	}
	return c.storeCold(key, artifactCanonical, projection, outcome.canonical)
}

func (c *RuntimeCache) storeCold(key FastInstanceKey, artifactCanonical, projection, outcome []byte) error {
	shard := c.shardFor(key)
	if shard == nil {
		return fmt.Errorf("interproc: invalid runtime cache")
	}
	shard.mu.Lock()
	defer shard.mu.Unlock()
	old := shard.snapshot.Load()
	for i := range old.slots {
		if old.slots[i].occupied && old.slots[i].key == key && slotWitnessEqual(old, old.slots[i], artifactCanonical, projection) {
			return nil
		}
	}
	count := 1
	for i := range old.slots {
		if old.slots[i].occupied {
			count++
		}
	}
	size := len(old.slots)
	if count*10 >= size*7 {
		size *= 2
	}
	next := &runtimeCacheSnapshot{slots: make([]runtimeCacheSlot, size), arena: make([]byte, 0, int(old.used)+len(artifactCanonical)+len(projection)+len(outcome))}
	for i := range old.slots {
		if old.slots[i].occupied {
			copySlot(next, old, old.slots[i])
		}
	}
	insertSlot(next, key, artifactCanonical, projection, outcome)
	shard.snapshot.Store(next)
	c.metrics.publications.Add(1)
	return nil
}

func copySlot(dst, src *runtimeCacheSnapshot, slot runtimeCacheSlot) {
	insertSlot(dst, slot.key,
		src.arena[slot.artifactOffset:slot.artifactOffset+slot.artifactLength],
		src.arena[slot.projectionOffset:slot.projectionOffset+slot.projectionLength],
		src.arena[slot.outcomeOffset:slot.outcomeOffset+slot.outcomeLength])
}

func insertSlot(snapshot *runtimeCacheSnapshot, key FastInstanceKey, artifactCanonical, projection, outcome []byte) {
	index := int(fastInstanceHash(key) & uint64(len(snapshot.slots)-1))
	for snapshot.slots[index].occupied {
		index = (index + 1) & (len(snapshot.slots) - 1)
	}
	slot := &snapshot.slots[index]
	slot.key = key
	slot.occupied = true
	slot.artifactOffset = snapshot.used
	slot.artifactLength = uint32(len(artifactCanonical))
	snapshot.arena = append(snapshot.arena, artifactCanonical...)
	snapshot.used += uint32(len(artifactCanonical))
	slot.projectionOffset = snapshot.used
	slot.projectionLength = uint32(len(projection))
	snapshot.arena = append(snapshot.arena, projection...)
	snapshot.used += uint32(len(projection))
	slot.outcomeOffset = snapshot.used
	slot.outcomeLength = uint32(len(outcome))
	snapshot.arena = append(snapshot.arena, outcome...)
	snapshot.used += uint32(len(outcome))
}

// Invalidate removes only the exact witness-confirmed cell and returns whether
// a published result was removed. A digest match alone cannot evict a peer.
func (c *RuntimeCache) Invalidate(key FastInstanceKey, artifactCanonical, projection []byte) bool {
	if c == nil || !key.Valid() {
		return false
	}
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	old := shard.snapshot.Load()
	found := false
	for i := range old.slots {
		if old.slots[i].occupied && old.slots[i].key == key && slotWitnessEqual(old, old.slots[i], artifactCanonical, projection) {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	next := &runtimeCacheSnapshot{slots: make([]runtimeCacheSlot, len(old.slots)), arena: make([]byte, 0, old.used)}
	for i := range old.slots {
		slot := old.slots[i]
		if slot.occupied && !(slot.key == key && slotWitnessEqual(old, slot, artifactCanonical, projection)) {
			copySlot(next, old, slot)
		}
	}
	shard.snapshot.Store(next)
	c.metrics.invalidations.Add(1)
	return true
}

// LoadOrCompute gives cache clients single-flight ownership without placing a
// channel or lock on a closed hit. compute is invoked by exactly one owner for
// each exact canonical witness pair; waiters re-read the immutable snapshot.
func (c *RuntimeCache) LoadOrCompute(key FastInstanceKey, artifactCanonical, projection []byte, compute func() (ClosedOutcome, error)) (ClosedOutcome, error) {
	if outcome, ok := c.Load(key, artifactCanonical, projection); ok {
		return outcome, nil
	}
	if c == nil || compute == nil || !key.Valid() {
		return ClosedOutcome{}, fmt.Errorf("interproc: invalid runtime cache resolution")
	}
	shard := c.shardFor(key)
	shard.mu.Lock()
	if outcome, ok := c.loadSnapshot(shard.snapshot.Load(), key, artifactCanonical, projection); ok {
		shard.mu.Unlock()
		return outcome, nil
	}
	if shard.flights == nil {
		shard.flights = make(map[FastInstanceKey][]*runtimeCacheFlight)
	}
	for _, flight := range shard.flights[key] {
		// The fixed primary key may collide. Only the exact witness pair joins
		// an existing owner; collision candidates remain independent owners.
		if !bytes.Equal(flight.artifact, artifactCanonical) || !bytes.Equal(flight.projection, projection) {
			continue
		}
		c.metrics.joins.Add(1)
		done := flight.done
		shard.mu.Unlock()
		<-done
		if flight.err != nil {
			return ClosedOutcome{}, flight.err
		}
		if outcome, ok := c.loadSnapshot(shard.snapshot.Load(), key, artifactCanonical, projection); ok {
			return outcome, nil
		}
		return ClosedOutcome{}, fmt.Errorf("interproc: runtime cache owner finished without publication")
	}
	flight := &runtimeCacheFlight{
		done:       make(chan struct{}),
		artifact:   append([]byte(nil), artifactCanonical...),
		projection: append([]byte(nil), projection...),
	}
	shard.flights[key] = append(shard.flights[key], flight)
	shard.mu.Unlock()

	outcome, err := compute()
	if err == nil {
		err = c.Store(key, artifactCanonical, projection, outcome)
	}
	shard.mu.Lock()
	flight.err = err
	flights := shard.flights[key]
	for i, candidate := range flights {
		if candidate == flight {
			copy(flights[i:], flights[i+1:])
			flights[len(flights)-1] = nil
			flights = flights[:len(flights)-1]
			break
		}
	}
	if len(flights) == 0 {
		delete(shard.flights, key)
	} else {
		shard.flights[key] = flights
	}
	close(flight.done)
	shard.mu.Unlock()
	return outcome, err
}

func (c *RuntimeCache) loadSnapshot(snapshot *runtimeCacheSnapshot, key FastInstanceKey, artifactCanonical, projection []byte) (ClosedOutcome, bool) {
	for i := range snapshot.slots {
		slot := &snapshot.slots[i]
		if slot.occupied && slot.key == key && slotWitnessEqual(snapshot, *slot, artifactCanonical, projection) {
			outcome := snapshot.arena[slot.outcomeOffset : slot.outcomeOffset+slot.outcomeLength]
			return ClosedOutcome{canonical: outcome, id: contentID(outcome)}, true
		}
	}
	return ClosedOutcome{}, false
}

// Metrics returns atomically collected cache activity.
func (c *RuntimeCache) Metrics() RuntimeCacheMetrics {
	if c == nil {
		return RuntimeCacheMetrics{}
	}
	return RuntimeCacheMetrics{
		Lookups: c.metrics.lookups.Load(), Hits: c.metrics.hits.Load(), Misses: c.metrics.misses.Load(),
		Joins: c.metrics.joins.Load(), Publications: c.metrics.publications.Load(), Invalidations: c.metrics.invalidations.Load(),
	}
}
