package program

import "github.com/wippyai/go-lua/analysis/check/fixpoint/summary"

// materializedSolveRegion is the exact owner closure whose body solve may be
// invalid after a proof round. It deliberately covers only solve dependencies;
// context discovery and result attachment require their own evidence before a
// caller may use this to skip the full materialization traversal.
type materializedSolveRegion struct {
	owners map[summary.SummaryKey]struct{}
}

func (r materializedSolveRegion) contains(key summary.SummaryKey) bool {
	_, ok := r.owners[key]
	return ok
}

// materializedAffectedSolveRegion constructs reverse tracked-summary-read
// closure. It fails closed unless every expected owner has exactly one cached
// solve with complete dependency evidence and unchanged routing/resolution.
// Entry-state equality remains validated by materializedSolveCache.read when an
// owner is actually reused.
func materializedAffectedSolveRegion(
	cache *materializedSolveCache,
	changes materializedSummaryChanges,
	keys programKeys,
	expected map[summary.SummaryKey]struct{},
) (materializedSolveRegion, bool) {
	if cache == nil || len(expected) == 0 {
		return materializedSolveRegion{}, false
	}
	byOwner := make(map[summary.SummaryKey]materializedSolveCacheEntry, len(cache.entries))
	for key, entry := range cache.entries {
		if _, wanted := expected[key.owner]; !wanted {
			continue
		}
		if _, duplicate := byOwner[key.owner]; duplicate {
			return materializedSolveRegion{}, false
		}
		if len(entry.deps) == 0 && !entry.noDepUniverseKnown {
			return materializedSolveRegion{}, false
		}
		if entry.routing != materializedOwnerRoutingDigest(keys, key.owner) ||
			entry.resolution != summaryOwnerResolutionDigest(keys, key.owner) {
			return materializedSolveRegion{}, false
		}
		byOwner[key.owner] = entry
	}
	if len(byOwner) != len(expected) {
		return materializedSolveRegion{}, false
	}
	for owner := range expected {
		if _, ok := byOwner[owner]; !ok {
			return materializedSolveRegion{}, false
		}
	}

	reverse := make(map[summary.SummaryKey][]summary.SummaryKey)
	for owner, entry := range byOwner {
		for dependency := range entry.deps {
			reverse[dependency] = append(reverse[dependency], owner)
		}
	}
	affected := make(map[summary.SummaryKey]struct{})
	queue := make([]summary.SummaryKey, 0, len(changes.Presentation))
	for changed := range changes.Presentation {
		queue = append(queue, changed)
		if _, owned := expected[changed]; owned {
			affected[changed] = struct{}{}
		}
	}
	seenChange := make(map[summary.SummaryKey]struct{}, len(queue))
	for len(queue) != 0 {
		changed := queue[0]
		queue = queue[1:]
		if _, seen := seenChange[changed]; seen {
			continue
		}
		seenChange[changed] = struct{}{}
		for _, owner := range reverse[changed] {
			if _, seen := affected[owner]; seen {
				continue
			}
			affected[owner] = struct{}{}
			// A changed owner may change its published summary, so propagate to
			// callers. Recursive SCCs terminate through affected membership.
			queue = append(queue, owner)
		}
	}
	return materializedSolveRegion{owners: affected}, true
}
