package flow

import "github.com/wippyai/go-lua/types/constraint"

// sourceRoute is the canonical address view of a provenance source plus the
// suffix consumed below the originating value.
type sourceRoute struct {
	source    StableAddress
	remainder []constraint.Segment
}

func canonicalSourceRoute(sourceKey constraint.PathKey, remainder []constraint.Segment) (sourceRoute, bool) {
	source, ok := StableAddressFromCanonicalKey(sourceKey)
	if !ok {
		return sourceRoute{}, false
	}
	return sourceRoute{
		source:    source,
		remainder: cloneAddressSegments(remainder),
	}, true
}

func appendCanonicalSourceRoute(out []sourceRoute, sourceKey constraint.PathKey, remainder []constraint.Segment) []sourceRoute {
	route, ok := canonicalSourceRoute(sourceKey, remainder)
	if !ok {
		return out
	}
	return append(out, route)
}
