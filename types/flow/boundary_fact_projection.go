package flow

import (
	"math"

	"github.com/wippyai/go-lua/types/flow/numeric"
)

// BoundaryFactProjectionInput is the point-local proof state that can be
// published across a function boundary once its stable addresses are rebased.
type BoundaryFactProjectionInput struct {
	KeyPresence   KeyPresenceFacts
	StaticMembers StaticMemberFacts
	Num           *numeric.State
	IndexWrites   IndexWriteAdmissionFacts
	PathAliases   PathAliasFacts
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
	var lenUpper []BoundaryLengthUpperBound
	ForEachNumericLenBoundAddress(in.Num, func(targetAddr StableAddress, lower, upper int64) bool {
		targets := paths.fromAddress(targetAddr)
		if len(targets) == 0 {
			return true
		}
		if lower > 0 {
			for _, target := range targets {
				lenLower = append(lenLower, BoundaryLengthLowerBound{Target: target, Lower: lower})
			}
		}
		if upper < math.MaxInt64 {
			for _, target := range targets {
				lenUpper = append(lenUpper, BoundaryLengthUpperBound{Target: target, Upper: upper})
			}
		}
		return true
	})

	var indexWrites []BoundaryIndexWriteFact
	in.IndexWrites.ForEachAddress(func(fact IndexWriteAdmissionAddressFact) bool {
		if fact.Key.IsZero() || fact.Value.IsZero() {
			return true
		}
		tables := paths.fromAddress(fact.Target)
		if len(tables) == 0 {
			return true
		}
		keys := boundaryIndexWriteKeyPaths(fact, paths, in.PathAliases)
		var valuePaths []BoundaryPath
		if fact.HasValuePath {
			valuePaths = paths.fromAddress(fact.ValuePath)
		}
		for _, table := range tables {
			if len(keys) == 0 {
				base := BoundaryIndexWriteFact{
					Table:    table,
					KeyValue: fact.Key,
					Value:    fact.Value,
				}
				if len(valuePaths) == 0 {
					indexWrites = append(indexWrites, base)
					continue
				}
				for _, valuePath := range valuePaths {
					next := base
					next.ValuePath = valuePath
					next.HasValuePath = true
					indexWrites = append(indexWrites, next)
				}
				continue
			}
			for _, key := range keys {
				base := BoundaryIndexWriteFact{
					Table:      table,
					KeyPath:    key,
					HasKeyPath: true,
					KeyValue:   fact.Key,
					Value:      fact.Value,
				}
				if len(valuePaths) == 0 {
					indexWrites = append(indexWrites, base)
					continue
				}
				for _, valuePath := range valuePaths {
					next := base
					next.ValuePath = valuePath
					next.HasValuePath = true
					indexWrites = append(indexWrites, next)
				}
			}
		}
		return true
	})

	parts := keyFacts.Parts()
	parts.LengthLower = append(parts.LengthLower, lenLower...)
	parts.LengthUpper = append(parts.LengthUpper, lenUpper...)
	parts.IndexWrites = append(parts.IndexWrites, indexWrites...)
	parts.StaticMembers = append(parts.StaticMembers, staticMembers...)
	return BoundaryFactsFromParts(parts)
}

func boundaryIndexWriteKeyPaths(
	fact IndexWriteAdmissionAddressFact,
	paths boundaryAddressPathCache,
	aliases PathAliasFacts,
) []BoundaryPath {
	if !fact.HasKeyPath {
		return nil
	}
	var out []BoundaryPath
	out = append(out, paths.fromAddress(fact.KeyPath)...)
	for _, alias := range aliases.Entries() {
		source, ok := alias.SourceAddress()
		if !ok {
			continue
		}
		remainder, ok := fact.KeyPath.RemainderAfterPrefix(source)
		if !ok {
			continue
		}
		value, ok := alias.ValueAddress()
		if !ok {
			continue
		}
		rebased, ok := value.Append(remainder)
		if !ok {
			continue
		}
		out = appendBoundaryPathsUnique(out, paths.fromAddress(rebased)...)
	}
	return out
}

func appendBoundaryPathsUnique(out []BoundaryPath, paths ...BoundaryPath) []BoundaryPath {
	for _, path := range paths {
		seen := false
		for _, existing := range out {
			if compareBoundaryPath(existing, path) == 0 {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, path)
		}
	}
	return out
}
