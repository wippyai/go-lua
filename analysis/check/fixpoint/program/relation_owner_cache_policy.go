package program

import (
	"encoding/binary"
	"hash/fnv"
)

// relationActiveV1ResolutionMarker is the semantic-version fence for summary
// owners whose calls are served by the frozen relation resolver. It is a
// value, rather than process-local identity, so independently prepared equal
// programs derive the same cache key.
//
// This policy is intentionally not wired into production solving yet. The
// resolver must be installed atomically with this fence and the cache bypass.
const relationActiveV1ResolutionMarker uint64 = 0x72656c2d61637431 // "rel-act1"

// composeRelationActiveResolutionDigest keeps legacy call routing in the key
// while fencing it from results produced without relation resolution.
func composeRelationActiveResolutionDigest(legacy uint64) uint64 {
	h := fnv.New64a()
	var encoded [16]byte
	binary.LittleEndian.PutUint64(encoded[:8], legacy)
	binary.LittleEndian.PutUint64(encoded[8:], relationActiveV1ResolutionMarker)
	_, _ = h.Write(encoded[:])
	return h.Sum64()
}

// relationOwnerCachePolicy is the inactive activation policy prepared from a
// run-local consumer catalog. It centralizes three inseparable decisions for
// an active consumer: fence its resolution identity, bypass the cross-unit
// summary cache, and create no retained summary-application owner.
//
// Keeping this as a helper (with no production callers) lets activation later
// install one policy at the query-owner construction seam instead of growing
// independent branches in the solve and materialization paths.
type relationOwnerCachePolicy struct {
	consumers relationConsumerPolicy
}

func newRelationOwnerCachePolicy(consumers relationConsumerPolicy) relationOwnerCachePolicy {
	return relationOwnerCachePolicy{consumers: consumers}
}

func (p relationOwnerCachePolicy) active(owner relationConsumerIdentity) bool {
	return p.consumers.Active(owner)
}

func (p relationOwnerCachePolicy) summaryCache(
	owner relationConsumerIdentity,
	legacy *SummarySolveCache,
) *SummarySolveCache {
	if p.active(owner) {
		return nil
	}
	return legacy
}

func (p relationOwnerCachePolicy) retainedOwner(
	owner relationConsumerIdentity,
	run *retainedSummaryApplicationRun,
) *retainedSummaryApplicationOwner {
	if p.active(owner) || run == nil {
		return nil
	}
	return run.newOwner(owner.Summary)
}

func (p relationOwnerCachePolicy) resolutionDigest(owner relationConsumerIdentity, legacy uint64) uint64 {
	if !p.active(owner) {
		return legacy
	}
	return composeRelationActiveResolutionDigest(legacy)
}
