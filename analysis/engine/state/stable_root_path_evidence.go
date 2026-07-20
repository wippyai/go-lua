package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// StableRootPathEvidenceMutation is an immutable ProductDomain-owned rewrite
// of stable/unversioned evidence for one lexical root. The caller decides
// which implications remain semantically valid; the path-evidence
// representation owns the one atomic storage rewrite.
type StableRootPathEvidenceMutation struct {
	seal        *productDomainSeal
	keys        *keyspace.KeySpace
	target      symbol.ID
	preserveAll bool
	preserveSet map[pathevidence.PathPresenceImplication]struct{}
}

// SealStableRootPathEvidenceMutation validates and freezes one stable-root
// rewrite. preserve must already be a canonical subset of the current must
// implication set; registration prevents another coordinate family from
// interpreting the mutation.
func (d ProductDomain) SealStableRootPathEvidenceMutation(
	keys *keyspace.KeySpace,
	target symbol.ID,
	preserveAll bool,
	preserve []pathevidence.PathPresenceImplication,
) (StableRootPathEvidenceMutation, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || target == 0 || preserveAll && len(preserve) != 0 {
		return StableRootPathEvidenceMutation{}, fmt.Errorf("state: invalid stable-root path-evidence mutation")
	}
	if !pathevidence.PathPresenceImplicationsCanonical(d.reg, keys, preserve) {
		return StableRootPathEvidenceMutation{}, fmt.Errorf("state: noncanonical stable-root implication preservation")
	}
	preserveSet := make(map[pathevidence.PathPresenceImplication]struct{}, len(preserve))
	for _, implication := range preserve {
		preserveSet[implication] = struct{}{}
	}
	return StableRootPathEvidenceMutation{
		seal: d.seal, keys: keys, target: target, preserveAll: preserveAll,
		preserveSet: preserveSet,
	}, nil
}

func (d ProductDomain) ownsStableRootPathEvidenceMutation(mutation StableRootPathEvidenceMutation) bool {
	return d.Valid() && mutation.seal == d.seal && mutation.keys != nil && mutation.keys.Valid() && mutation.target != 0 && !(mutation.preserveAll && len(mutation.preserveSet) != 0)
}

func stableRootImplicationPreserved(mutation StableRootPathEvidenceMutation, candidate pathevidence.PathPresenceImplication) bool {
	_, ok := mutation.preserveSet[candidate]
	return ok
}

func applyStableRootPathEvidenceMutation(lane pathevidence.Lane, mutation StableRootPathEvidenceMutation) pathevidence.Lane {
	if mutation.preserveAll {
		return lane.InvalidateStableSymbolPreservingAllImplications(mutation.target)
	}
	return lane.InvalidateStableSymbolPreservingImplications(mutation.target, func(candidate pathevidence.PathPresenceImplication) bool {
		return stableRootImplicationPreserved(mutation, candidate)
	})
}

// ApplyStableRootPathEvidenceMutation applies the same registered storage law
// used by the coordinate carrier to concrete State.
func (d ProductDomain) ApplyStableRootPathEvidenceMutation(mutation StableRootPathEvidenceMutation, current State) (State, error) {
	if !d.ownsStableRootPathEvidenceMutation(mutation) {
		return State{}, fmt.Errorf("state: foreign stable-root path-evidence mutation")
	}
	out := d.Normalize(current)
	if !out.laneEnabled(lanePathEvidenceBit) {
		return out, nil
	}
	next := applyStableRootPathEvidenceMutation(out.pathEvidence, mutation)
	if pathevidence.Domain(d.reg).Equal(out.pathEvidence, next) {
		return out, nil
	}
	out = out.reachable()
	out.pathEvidence = next
	return out, nil
}

// ApplyStableRootPathEvidence mutates an already-open coordinate carrier
// without composing a State or inspecting a coordinate-family name.
func (c *CoordinatePathEvidenceCarrier[K]) ApplyStableRootPathEvidence(mutation StableRootPathEvidenceMutation) (bool, bool) {
	if !c.Valid() || !c.domain.ownsStableRootPathEvidenceMutation(mutation) {
		return false, false
	}
	writeIndex, ok := c.coordinateWriteIndex()
	if !ok {
		return false, false
	}
	for _, entry := range c.baselineEntries {
		key := pathEvidenceCoordinateKey(entry.key)
		if !pathevidence.StableRootMutationRemovesCoordinate(key, mutation.target, mutation.preserveAll, func(candidate pathevidence.PathPresenceImplication) bool {
			return stableRootImplicationPreserved(mutation, candidate)
		}) {
			continue
		}
		if !c.coordinateWriteIndexContains(writeIndex, entry.key) {
			return false, false
		}
	}
	// Path-evidence lanes are persistent values. Once the finite affected
	// coordinate set has been authorized above, the package-owned rewrite is
	// atomic and cannot partially mutate on failure.
	return c.inner.ApplyStableRootMutation(mutation)
}
