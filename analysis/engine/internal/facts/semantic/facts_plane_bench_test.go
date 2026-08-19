package semantic

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// benchValue is a deliberately non-trivial lattice value: exact equality and
// fingerprinting both cost more than one machine word, which is what makes a
// value walk visible against an identity answer. Its tag is derived from its
// bits, so the lattice is an ordinary subset lattice over the bit set.
type benchValue struct {
	bits uint64
	tag  string
}

// benchEqualCalls and benchFingerprintCalls make the removed work visible
// independently of what one exact V comparison happens to cost: promotion is
// worth exactly the value comparisons and fingerprints it no longer performs.
var benchEqualCalls, benchFingerprintCalls int64

func benchEqual(left, right benchValue) bool {
	benchEqualCalls++
	return left.bits == right.bits && left.tag == right.tag
}

func benchFingerprint(value benchValue) uint64 {
	benchFingerprintCalls++
	hash := uint64(1469598103934665603)
	for index := 0; index < 8; index++ {
		hash = (hash ^ (value.bits>>(index*8))&0xff) * 1099511628211
	}
	for index := 0; index < len(value.tag); index++ {
		hash = (hash ^ uint64(value.tag[index])) * 1099511628211
	}
	return hash
}

func benchTerminal(bits uint64) benchValue {
	return benchValue{bits: bits, tag: "lattice-cell-" + strconv.FormatUint(bits, 10)}
}

func benchJoin(left, right benchValue) (benchValue, bool) {
	return benchTerminal(left.bits | right.bits), true
}

const (
	benchAtoms = 6
	benchKeys  = 32
)

type benchFixture struct {
	manager *guard.Manager
	values  *terminal.Arena[benchValue]
	facts   *diagram.Diagram[semanticFactor, semanticKey, benchValue]
	domain  *Domain[semanticFactor, semanticKey, benchValue]
	all     support.Mask
	on      support.Mask
	off     support.Mask
	ids     map[uint64]terminal.ID[benchValue]
}

// newBenchFixture pre-admits only the single-bit operand values. Every joined
// value is therefore genuinely new to the base generation and must be interned
// by the transaction that produces it, which is the exact shape a recurrence
// has.
func newBenchFixture(b *testing.B) benchFixture {
	b.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		b.Fatal(err)
	}
	setup := support.New(manager)
	all := setup.True()
	on, ok := setup.Literal(1, true)
	if !ok {
		b.Fatal("on")
	}
	off, ok := setup.Not(on)
	if !ok || !setup.Seal() {
		b.Fatal("off")
	}
	values, ok := terminal.New(terminal.Config[benchValue]{Equal: benchEqual, Fingerprint: benchFingerprint})
	if !ok {
		b.Fatal("arena")
	}
	ids := make(map[uint64]terminal.ID[benchValue])
	admit := func(bits uint64) {
		id, admitted := values.Admit(benchTerminal(bits))
		if !admitted {
			b.Fatal("admit")
		}
		ids[bits] = id
	}
	admit(0)
	for atom := 0; atom < benchAtoms; atom++ {
		admit(1 << atom)
	}
	if !values.Seal() {
		b.Fatal("seal")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, benchValue]{
		Factors: []semanticFactor{semanticColumn}, Terminals: values, Guards: manager,
	})
	if !ok {
		b.Fatal("diagram")
	}
	domain, ok := New(facts, values, Operations[benchValue]{
		Default:     benchTerminal(0),
		Equal:       benchEqual,
		Fingerprint: benchFingerprint,
		Join:        benchJoin,
		Widen:       benchJoin,
		Narrow:      func(_, right benchValue) (benchValue, bool) { return right, true },
		LessOrEq:    func(left, right benchValue) bool { return left.bits&right.bits == left.bits },
	})
	if !ok {
		b.Fatal("domain")
	}
	return benchFixture{manager: manager, values: values, facts: facts, domain: domain, all: all, on: on, off: off, ids: ids}
}

// plane writes one guarded column per key so both the sparse key zipper and
// the per-column decision walk are exercised.
func (fixture benchFixture) plane(b *testing.B, shift uint64) Plane[semanticFactor, semanticKey, benchValue] {
	b.Helper()
	builder := fixture.facts.Begin()
	root := fixture.facts.Empty()
	for key := 0; key < benchKeys; key++ {
		var written bool
		root, written = builder.Set(root, semanticColumn, semanticKey(key), fixture.on, fixture.ids[1<<((uint64(key)+shift)%benchAtoms)])
		if !written {
			b.Fatal("write on")
		}
		root, written = builder.Set(root, semanticColumn, semanticKey(key), fixture.off, fixture.ids[1<<((uint64(key)+shift+3)%benchAtoms)])
		if !written {
			b.Fatal("write off")
		}
	}
	sealed, ok := builder.Seal(root)
	if !ok {
		b.Fatal("plane seal")
	}
	plane, valid := fixture.domain.Plane(sealed)
	if !valid {
		b.Fatal("plane")
	}
	return plane
}

func (fixture benchFixture) join(b *testing.B, left, right Plane[semanticFactor, semanticKey, benchValue]) Plane[semanticFactor, semanticKey, benchValue] {
	b.Helper()
	plane, ok := fixture.domain.JoinContributions(left, right,
		diagram.NewSoleScratch[semanticKey, benchValue](), support.New(fixture.manager),
		func(semanticKey, support.Mask) bool { return true },
		func(semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
			return fixture.all, fixture.all, fixture.all, true
		})
	if !ok {
		b.Fatal("join")
	}
	return plane
}

// derived produces one plane through an ordinary join transaction, so its
// terminals are transaction-authored rather than pre-admitted base values.
func (fixture benchFixture) derived(b *testing.B) Plane[semanticFactor, semanticKey, benchValue] {
	b.Helper()
	return fixture.join(b, fixture.plane(b, 0), fixture.plane(b, 2))
}

// BenchmarkSameIndependentEqualPlanes measures whole-plane equality of two
// planes that were built by independent transactions over equal inputs. It is
// the publication-boundary wake decision.
func BenchmarkSameIndependentEqualPlanes(b *testing.B) {
	fixture := newBenchFixture(b)
	left, right := fixture.derived(b), fixture.derived(b)
	if !fixture.domain.Same(left, right) {
		b.Fatal("fixture planes are not equal")
	}
	b.ReportAllocs()
	benchEqualCalls, benchFingerprintCalls = 0, 0
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !fixture.domain.Same(left, right) {
			b.Fatal("equality regressed")
		}
	}
	reportValueWork(b)
}

// BenchmarkEqualUnderIndependentEqualPlanes measures the support-restricted
// recurrence comparison over the same two independently built planes.
func BenchmarkEqualUnderIndependentEqualPlanes(b *testing.B) {
	fixture := newBenchFixture(b)
	left, right := fixture.derived(b), fixture.derived(b)
	scratch := diagram.NewSoleScratch[semanticKey, benchValue]()
	if !fixture.domain.EqualUnder(left, right, fixture.all, scratch) {
		b.Fatal("fixture planes are not equal under the whole support")
	}
	b.ReportAllocs()
	benchEqualCalls, benchFingerprintCalls = 0, 0
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !fixture.domain.EqualUnder(left, right, fixture.all, scratch) {
			b.Fatal("equality regressed")
		}
	}
	reportValueWork(b)
}

// BenchmarkJoinContributionsManyFixedPoint measures the lifted point fold at a
// fixed point: every cell resolves to a value the sealed universe already
// holds, so the operation must reuse identities rather than mint them.
func BenchmarkJoinContributionsManyFixedPoint(b *testing.B) {
	fixture := newBenchFixture(b)
	inputs := []Plane[semanticFactor, semanticKey, benchValue]{fixture.plane(b, 0), fixture.plane(b, 2), fixture.plane(b, 4)}
	regions := support.New(fixture.manager)
	covers := func(_ semanticKey, output []support.Mask) bool {
		if len(output) != len(inputs) {
			return false
		}
		for index := range output {
			output[index] = fixture.all
		}
		return true
	}
	current := inputs[0]
	for round := 0; round < 4; round++ {
		var ok bool
		current, ok = fixture.domain.JoinContributionsMany(current, inputs, diagram.NewSoleScratch[semanticKey, benchValue](), regions, covers)
		if !ok {
			b.Fatal("warm fold")
		}
	}
	scratch := diagram.NewSoleScratch[semanticKey, benchValue]()
	b.ReportAllocs()
	benchEqualCalls, benchFingerprintCalls = 0, 0
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		next, ok := fixture.domain.JoinContributionsMany(current, inputs, scratch, regions, covers)
		if !ok {
			b.Fatal("fold")
		}
		current = next
	}
	reportValueWork(b)
}

// BenchmarkJoinContributionsFixedPoint measures the binary contribution join
// at a fixed point through the same recurrence shape.
func BenchmarkJoinContributionsFixedPoint(b *testing.B) {
	fixture := newBenchFixture(b)
	right := fixture.plane(b, 3)
	current := fixture.plane(b, 0)
	for round := 0; round < 4; round++ {
		current = fixture.join(b, current, right)
	}
	regions := support.New(fixture.manager)
	report := func(semanticKey, support.Mask) bool { return true }
	covers := func(semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
		return fixture.all, fixture.all, fixture.all, true
	}
	scratch := diagram.NewSoleScratch[semanticKey, benchValue]()
	b.ReportAllocs()
	benchEqualCalls, benchFingerprintCalls = 0, 0
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		next, ok := fixture.domain.JoinContributions(current, right, scratch, regions, report, covers)
		if !ok {
			b.Fatal("join")
		}
		current = next
	}
	reportValueWork(b)
}

// reportValueWork publishes the exact V-level work one operation performed.
func reportValueWork(b *testing.B) {
	b.StopTimer()
	b.ReportMetric(float64(benchEqualCalls)/float64(b.N), "valueEqual/op")
	b.ReportMetric(float64(benchFingerprintCalls)/float64(b.N), "fingerprint/op")
}
