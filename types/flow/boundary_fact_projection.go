package flow

import "github.com/wippyai/go-lua/types/flow/numeric"

// BoundaryFactProjectionInput is the point-local proof state that can be
// published across a function boundary once its stable addresses are rebased.
type BoundaryFactProjectionInput struct {
	KeyPresence   KeyPresenceFacts
	StaticMembers StaticMemberFacts
	Num           *numeric.State
	IndexWrites   IndexWriteAdmissionFacts
}

// BoundaryFactProjectionPolicy configures cross-boundary fact projection. The
// key-presence options stay nested because key-array pending history is a
// key-presence lane policy, not a generic boundary-fact law.
type BoundaryFactProjectionPolicy struct {
	KeyPresence KeyPresenceBoundaryProjection
}

// ProjectBoundaryFacts projects point-local proof facts into boundary-relative
// facts using projector as the sole address rebasing authority.
func ProjectBoundaryFacts(
	in BoundaryFactProjectionInput,
	projector BoundaryAddressProjector,
	policy BoundaryFactProjectionPolicy,
) BoundaryFacts {
	if projector == nil {
		return BoundaryFactsDomain.Top()
	}
	paths := newBoundaryAddressPathCache(projector)
	keyFacts := projectKeyPresenceBoundaryFactsWithPaths(in.KeyPresence, paths, policy.KeyPresence)

	var staticMembers []BoundaryStaticMemberFact
	for _, fact := range in.StaticMembers.Entries() {
		addr, ok := StableAddressFromCanonicalKey(fact.Path)
		if !ok || fact.Value.IsZero() {
			continue
		}
		for _, target := range paths.fromAddress(addr) {
			staticMembers = append(staticMembers, BoundaryStaticMemberFact{
				Target: target,
				Value:  fact.Value,
			})
		}
	}

	var lenLower []BoundaryLengthLowerBound
	ForEachNumericLenBoundAddress(in.Num, func(targetAddr StableAddress, lower, _ int64) bool {
		if lower <= 0 {
			return true
		}
		for _, target := range paths.fromAddress(targetAddr) {
			lenLower = append(lenLower, BoundaryLengthLowerBound{Target: target, Lower: lower})
		}
		return true
	})

	var indexWrites []BoundaryIndexWriteFact
	in.IndexWrites.ForEachAddress(func(fact IndexWriteAdmissionAddressFact) bool {
		if !fact.HasKeyPath || fact.Value.IsZero() {
			return true
		}
		tables := paths.fromAddress(fact.Target)
		if len(tables) == 0 {
			return true
		}
		keys := paths.fromAddress(fact.KeyPath)
		if len(keys) == 0 {
			return true
		}
		for _, table := range tables {
			for _, key := range keys {
				indexWrites = append(indexWrites, BoundaryIndexWriteFact{
					Table: table,
					Key:   key,
					Value: fact.Value,
				})
			}
		}
		return true
	})

	return BoundaryFactsOf(
		keyFacts.KeyPresence(),
		keyFacts.KeyArrays(),
		keyFacts.KeyArrayValues(),
		keyFacts.AppendKeys(),
		lenLower,
		indexWrites,
	).WithAppendElementFieldOrigins(keyFacts.AppendElementFieldOrigins()).
		WithStaticMembers(staticMembers)
}
