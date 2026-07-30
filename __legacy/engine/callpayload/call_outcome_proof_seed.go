package callpayload

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// CallOutcomeProductionProgramKind identifies the typed normal-return program
// a provider leaf is entitled to publish. It is deliberately distinct from
// the resulting proof coordinates: those are derived by the registered
// normal-return program at the consuming ExternalCall occurrence.
type CallOutcomeProductionProgramKind uint8

const (
	CallOutcomeProductionProgramInvalid CallOutcomeProductionProgramKind = iota
	CallOutcomeProductionProgramPathRefinement
)

// CallOutcomeProductionProgram is the provider-leaf program declaration kept
// in boundary syntax until an ExternalCall occurrence freezes it.
type CallOutcomeProductionProgram struct {
	kind     CallOutcomeProductionProgramKind
	path     pathdom.Path
	presence presence.Value
}

// NormalReturnPathRefinement reports the exact N3 input this production
// program supplies. The returned path is detached from the declaration.
func (p CallOutcomeProductionProgram) NormalReturnPathRefinement() (pathdom.Path, presence.Value, bool) {
	if p.kind != CallOutcomeProductionProgramPathRefinement || p.path.IsEmpty() ||
		(!presence.Equal(p.presence, presence.Present()) && !presence.Equal(p.presence, presence.Absent())) {
		return pathdom.Path{}, presence.Top(), false
	}
	return p.path.Clone(), p.presence, true
}

func (p CallOutcomeProductionProgram) valid() bool {
	_, _, ok := p.NormalReturnPathRefinement()
	return ok
}

// CallOutcomeProofSeed is one portable, provider-leaf declaration for a
// proof-producing normal-return fact. Path remains in boundary syntax for the
// slice-1 authority tuple; Production retains the actual typed producer shape.
type CallOutcomeProofSeed struct {
	Path       pathdom.Path
	Presence   presence.Value
	production CallOutcomeProductionProgram
}

// NormalReturnPathPresenceProofSeed declares one exact normal-return presence
// proof a provider leaf may publish.
func NormalReturnPathPresenceProofSeed(path pathdom.Path, value presence.Value) CallOutcomeProofSeed {
	return CallOutcomeProofSeed{
		Path: path.Clone(), Presence: value,
		production: CallOutcomeProductionProgram{
			kind: CallOutcomeProductionProgramPathRefinement,
			path: path.Clone(), presence: value,
		},
	}
}

// ProductionProgram returns this seed's typed leaf production program.
func (s CallOutcomeProofSeed) ProductionProgram() CallOutcomeProductionProgram {
	return CallOutcomeProductionProgram{kind: s.production.kind, path: s.production.path.Clone(), presence: s.production.presence}
}

func (s CallOutcomeProofSeed) valid() bool {
	return s.Path.IsPlaceholder() &&
		(presence.Equal(s.Presence, presence.Present()) || presence.Equal(s.Presence, presence.Absent())) &&
		s.production.valid() && s.production.path.Equal(s.Path) && presence.Equal(s.production.presence, s.Presence)
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
