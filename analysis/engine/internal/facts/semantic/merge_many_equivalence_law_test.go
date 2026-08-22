package semantic

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestJoinContributionsManyEqualsSequentialBinaryFoldOnRandomOperands is the
// meaning law of the many-way contribution fold. The fold is a join in the
// lattice of guarded contributions, so it must agree with folding the same
// operands one pair at a time under accumulated coverage - for every operand
// count, every coverage pattern and every distribution of repeated values.
// The engine canonicalizes contributions as a set while it traverses; this
// proves that canonicalization is meaning-preserving and not merely cheaper.
func TestJoinContributionsManyEqualsSequentialBinaryFoldOnRandomOperands(t *testing.T) {
	const atomCount = 4
	atoms := make([]guard.Atom, atomCount)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	literals := support.New(manager)
	if literals == nil {
		t.Fatal("literal work")
	}
	positive := make([]support.Mask, atomCount)
	negative := make([]support.Mask, atomCount)
	for index, atom := range atoms {
		high, highOK := literals.Literal(atom, true)
		low, lowOK := literals.Literal(atom, false)
		if !highOK || !lowOK {
			t.Fatal("literal")
		}
		positive[index], negative[index] = high, low
	}
	empty := literals.False()
	whole := literals.True()
	if !literals.Seal() {
		t.Fatal("literal seal")
	}

	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for value := uint8(0); value < 8; value++ {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("admit %d", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{Factors: []semanticFactor{semanticColumn}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}
	join := func(left, right uint8) (uint8, bool) { return left | right, true }
	domain, ok := New(facts, values, Operations[uint8]{
		Default: 0, Equal: func(left, right uint8) bool { return left == right }, Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join: join, Widen: join, Narrow: func(_, right uint8) (uint8, bool) { return right, true }, LessOrEq: func(left, right uint8) bool { return left&right == left },
	})
	if !ok {
		t.Fatal("domain")
	}

	for seed := int64(1); seed <= 24; seed++ {
		random := rand.New(rand.NewSource(seed))
		width := 2 + random.Intn(6)

		shell := support.New(manager)
		if shell == nil {
			t.Fatal("region shell")
		}
		coverage := make([]support.Mask, width)
		prefix := make([]support.Mask, width+1)
		prefix[0] = empty
		for operand := 0; operand < width; operand++ {
			region := empty
			for index := 0; index < atomCount; index++ {
				switch random.Intn(3) {
				case 0:
					region, ok = shell.Or(region, positive[index])
				case 1:
					region, ok = shell.Or(region, negative[index])
				default:
					continue
				}
				if !ok {
					t.Fatal("region union")
				}
			}
			coverage[operand] = region
			prefix[operand+1], ok = shell.Or(prefix[operand], region)
			if !ok {
				t.Fatal("prefix union")
			}
		}
		if !shell.Seal() {
			t.Fatal("region seal")
		}

		planes := make([]Plane[semanticFactor, semanticKey, uint8], width)
		for operand := range planes {
			builder := facts.Begin()
			if builder == nil {
				t.Fatal("plane builder")
			}
			// Two authored regions per operand keep every operand a real FDD
			// rather than a constant leaf, so the fold has to split on operand
			// structure as well as on coverage.
			root, written := builder.Set(facts.Empty(), semanticColumn, 7, positive[random.Intn(atomCount)], ids[uint8(1+random.Intn(3))])
			if !written {
				t.Fatal("first write")
			}
			root, written = builder.Set(root, semanticColumn, 7, negative[random.Intn(atomCount)], ids[uint8(1+random.Intn(3))])
			if !written {
				t.Fatal("second write")
			}
			root, written = builder.Seal(root)
			if !written {
				t.Fatal("plane seal")
			}
			planes[operand], written = domain.Plane(root)
			if !written {
				t.Fatal("plane")
			}
		}

		work := support.New(manager)
		if work == nil {
			t.Fatal("fold work")
		}
		many, folded := domain.JoinContributionsMany(planes[0], planes, diagram.NewSoleScratch[semanticKey, uint8](), work, func(key semanticKey, output []support.Mask) bool {
			if key != 7 || len(output) != width {
				return false
			}
			copy(output, coverage)
			return true
		})
		if !folded {
			t.Fatalf("seed %d many fold", seed)
		}

		sequential := planes[0]
		for operand := 1; operand < width; operand++ {
			left, right, reference := prefix[operand], coverage[operand], prefix[operand+1]
			var joined bool
			sequential, joined = domain.JoinContributions(sequential, planes[operand], diagram.NewSoleScratch[semanticKey, uint8](), work, func(semanticKey, support.Mask) bool { return true }, func(key semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
				return left, right, reference, key == 7
			})
			if !joined {
				t.Fatalf("seed %d sequential fold at %d", seed, operand)
			}
		}
		// Meaning is compared point by point over the whole valuation space and
		// then as a whole plane. Physical node sharing is not the law here: two
		// folds may reach the same meaning through different terminal leaves.
		for valuation := 0; valuation < 1<<atomCount; valuation++ {
			point := valuation
			read := func(plane Plane[semanticFactor, semanticKey, uint8]) (uint8, bool, bool) {
				id, present, valid := facts.At(plane.Root(), semanticColumn, 7, func(atom guard.Atom) bool {
					return point&(1<<(int(atom)-1)) != 0
				})
				if !valid || !present {
					return 0, present, valid
				}
				value, readable := values.Value(id)
				return value, present, readable
			}
			gotValue, gotPresent, gotValid := read(many)
			wantValue, wantPresent, wantValid := read(sequential)
			if gotValue != wantValue || gotPresent != wantPresent || gotValid != wantValid {
				t.Fatalf("seed %d width %d valuation %04b: many=%d/%t/%t sequential=%d/%t/%t", seed, width, point, gotValue, gotPresent, gotValid, wantValue, wantPresent, wantValid)
			}
		}
		if !domain.EqualUnder(many, sequential, whole, diagram.NewSoleScratch[semanticKey, uint8]()) {
			t.Fatalf("seed %d width %d: many-way fold and sequential binary fold disagree", seed, width)
		}
	}
}
