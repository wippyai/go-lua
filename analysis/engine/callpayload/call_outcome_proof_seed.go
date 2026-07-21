package callpayload

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// CallOutcomeProofSeed is one portable, provider-leaf declaration for a
// proof-producing normal-return fact. Path remains in boundary syntax; it is
// substituted only through the exact ExternalCall occurrence while freezing.
type CallOutcomeProofSeed struct {
	Path     pathdom.Path
	Presence presence.Value
}

// NormalReturnPathPresenceProofSeed declares one exact normal-return presence
// proof a provider leaf may publish.
func NormalReturnPathPresenceProofSeed(path pathdom.Path, value presence.Value) CallOutcomeProofSeed {
	return CallOutcomeProofSeed{Path: path.Clone(), Presence: value}
}

func (s CallOutcomeProofSeed) valid() bool {
	return s.Path.IsPlaceholder() &&
		(presence.Equal(s.Presence, presence.Present()) || presence.Equal(s.Presence, presence.Absent()))
}

func proofSeedLess(left, right CallOutcomeProofSeed) bool {
	if !left.Path.Equal(right.Path) {
		return left.Path.Less(right.Path)
	}
	return left.Presence < right.Presence
}

func proofSeedEqual(left, right CallOutcomeProofSeed) bool {
	return left.Path.Equal(right.Path) && presence.Equal(left.Presence, right.Presence)
}

func canonicalProofSeeds(in []CallOutcomeProofSeed) ([]CallOutcomeProofSeed, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := append([]CallOutcomeProofSeed(nil), in...)
	for index := range out {
		if !out[index].valid() {
			return nil, fmt.Errorf("callpayload: invalid normal-return proof seed %d", index)
		}
	}
	sort.Slice(out, func(i, j int) bool { return proofSeedLess(out[i], out[j]) })
	write := 0
	for _, seed := range out {
		if write != 0 && proofSeedEqual(out[write-1], seed) {
			continue
		}
		out[write], write = seed, write+1
	}
	return out[:write], nil
}

func proofSeedsContain(seeds []CallOutcomeProofSeed, path pathdom.Path, value presence.Value) bool {
	index := sort.Search(len(seeds), func(index int) bool {
		if !seeds[index].Path.Equal(path) {
			return !seeds[index].Path.Less(path)
		}
		return seeds[index].Presence >= value
	})
	return index < len(seeds) && seeds[index].Path.Equal(path) && presence.Equal(seeds[index].Presence, value)
}
