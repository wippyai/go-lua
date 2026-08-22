package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// lawSummaryRange is deliberately a scalar source. It lets the laws exercise
// the same opaque range seam as a domain operand without giving the test a
// slice that appendDeclaredSummary could accidentally retain or copy.
type lawSummaryRange struct {
	keys  []uint64
	calls uint64
}

func (source *lawSummaryRange) SummaryKeyCount() int {
	if source == nil {
		return -1
	}
	return len(source.keys)
}

func (source *lawSummaryRange) SummaryKeyAt(index int) (uint64, bool) {
	if source == nil || index < 0 || index >= len(source.keys) {
		return 0, false
	}
	source.calls++
	return source.keys[index], true
}

func summaryAdmissionLawFixture(t testing.TB, base, keyEnd uint64, source summaryKeySource) (*SchemaBinding, *ruleSummaryMapping) {
	t.Helper()
	schema, factor := factorOnlySlotSchema(t, coldKey(base))
	spec := hotUintFactorSpec()
	spec.KeyEnd = keyEnd
	binding := NewSchemaBinding(schema)
	if factor == nil || binding == nil || !BindFactor(binding, factor, spec) || !binding.Seal() {
		t.Fatal("summary admission Factor fixture")
	}
	factorKey := compositionKeyOf(coldKey(base))
	normalizer := compositionKeyOf(coldKey(base + 1))
	surface := equation.Surface{
		Factor: factorKey, Form: equation.SurfaceReadSummary,
		Content: [32]byte{1}, Semantic: normalizer, Normalizer: normalizer,
	}
	mapping := &ruleSummaryMapping{
		state: binding.state, authority: binding.state.authority,
		factor: factorKey, normalizer: normalizer,
		surface: surface, keys: source,
	}
	return binding, mapping
}

func TestSummaryAdmissionRejectsMalformedScalarRanges(t *testing.T) {
	const (
		base   = uint64(998_010)
		keyEnd = uint64(4)
	)
	cases := map[string][]uint64{
		"empty-nonempty-factor": nil,
		"unsorted":              {0, 2, 1},
		"out-of-range":          {0, keyEnd},
	}
	nextBase := base
	for name, keys := range cases {
		t.Run(name, func(t *testing.T) {
			source := &lawSummaryRange{keys: keys}
			binding, mapping := summaryAdmissionLawFixture(t, nextBase, keyEnd, source)
			if _, accepted := appendDeclaredSummary(nil, mapping, binding.state, binding.state.authority); accepted {
				t.Fatal("malformed scalar summary range was admitted")
			}
		})
		nextBase++
	}

	negative := &lawSummaryRange{keys: []uint64{0}}
	binding, mapping := summaryAdmissionLawFixture(t, base, keyEnd, negative)
	// A source that reports an invalid count must fail closed even when its
	// backing data would otherwise be a valid one-key range.
	invalid := &invalidSummaryRange{}
	mapping.keys = invalid
	if _, accepted := appendDeclaredSummary(nil, mapping, binding.state, binding.state.authority); accepted {
		t.Fatal("negative scalar summary count was admitted")
	}

	shortBinding, shortMapping := summaryAdmissionLawFixture(t, base+1, keyEnd, shortSummaryRange{})
	if _, accepted := appendDeclaredSummary(nil, shortMapping, shortBinding.state, shortBinding.state.authority); accepted {
		t.Fatal("scalar summary range with a missing coordinate was admitted")
	}
}

type invalidSummaryRange struct{}

func (invalidSummaryRange) SummaryKeyCount() int { return -1 }

func (invalidSummaryRange) SummaryKeyAt(int) (uint64, bool) { return 0, false }

type shortSummaryRange struct{}

func (shortSummaryRange) SummaryKeyCount() int { return 2 }

func (shortSummaryRange) SummaryKeyAt(index int) (uint64, bool) { return 0, index == 0 }

func TestRepeatedSummaryAdmissionsMaterializeOnlyFirstSurface(t *testing.T) {
	const (
		base       = uint64(998_020)
		keyEnd     = uint64(256)
		admissions = 256
	)
	source := &lawSummaryRange{keys: make([]uint64, keyEnd)}
	for index := range source.keys {
		source.keys[index] = uint64(index)
	}
	binding, mapping := summaryAdmissionLawFixture(t, base, keyEnd, source)
	summaries, accepted := appendDeclaredSummary(nil, mapping, binding.state, binding.state.authority)
	if !accepted || len(summaries) != 1 || len(summaries[0].Keys) != int(keyEnd) {
		t.Fatal("first scalar summary admission did not materialize its complete key range")
	}
	firstCalls := source.calls

	// Every later mounted-point admission names the same surface and retains
	// only the opaque source. The duplicate fold compares Count/At and must not
	// allocate another full-width equation key vector for any point.
	allocations := testing.AllocsPerRun(64, func() {
		for index := 0; index < admissions; index++ {
			folded, foldedOK := appendDeclaredSummary(summaries, mapping, binding.state, binding.state.authority)
			if !foldedOK || len(folded) != 1 || &folded[0].Keys[0] != &summaries[0].Keys[0] {
				panic("duplicate scalar summary admission changed the materialized surface")
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("repeated identical summary admissions allocated %v times per run", allocations)
	}
	if source.calls <= firstCalls {
		t.Fatal("duplicate scalar summary admissions did not validate their source range")
	}
	if len(summaries) != 1 || summaries[0].Keys[0] != 0 || summaries[0].Keys[keyEnd-1] != keyEnd-1 {
		t.Fatal("duplicate scalar summary admissions changed the canonical materialization")
	}
}
