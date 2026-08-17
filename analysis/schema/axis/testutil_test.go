package axis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// The semantic roles the scratch axes are declared under. An axis names a role
// by the key it is declared under and the surface resolves it against the
// structural vocabulary sealed below, so the scratch table declares the rows
// these keys name.
const (
	valueRole  schema.Key = "semantic/factor/value"
	heapRole   schema.Key = "semantic/factor/heap"
	packRole   schema.Key = "semantic/factor/pack"
	absentRole schema.Key = "semantic/factor/absent"
)

// scratchInputs is a stand-in for a composition's Link input record. The
// surface is blind to it, so a scratch record proves the same laws the
// analyzer's own record does.
type scratchInputs struct{ ready bool }

type scratchFragment struct{ semantic identity.SemanticKey }

type scratchAxis struct{ fragment *scratchFragment }

// scratchRuleSurface stands in for one sibling surface. The declaration root
// requires every catalog member to be registered, so an axis law is stated
// against a complete table rather than a half-registered one.
type scratchRuleSurface struct{ kind schema.SurfaceKind }

type scratchRuleEntry struct{ key schema.Key }

func (entry scratchRuleEntry) Key() schema.Key { return entry.key }

func (entry scratchRuleEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchRuleEntry) EntryContent(*framing.Writer) error { return nil }

func (surface scratchRuleSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface scratchRuleSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchRuleEntry{key: "scratch-rule"}}
}

func (surface scratchRuleSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// scratchStructureSurface stands in for the structural vocabulary. It carries
// real semantic role rows, because an axis names its identity by reference and
// the axis surface resolves that reference against this view; the vocabulary's
// own totality laws are its package's and are not restated here.
type scratchStructureSurface struct{}

func (surface scratchStructureSurface) Kind() schema.SurfaceKind {
	return schema.SurfaceKindStructure
}

func (surface scratchStructureSurface) Entries() []schema.Entry {
	rows, ok := structure.Collect(vocabulary.RoleSpecs("factor/value", "factor/heap", "factor/pack"))
	if !ok {
		return nil
	}
	entries := make([]schema.Entry, len(rows))
	for index, row := range rows {
		entries[index] = row
	}
	return entries
}

func (surface scratchStructureSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func scratchLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return ^uint64(0) },
		Equal:    func(a, b uint64) bool { return a == b },
		LessOrEq: func(a, b uint64) bool { return a <= b },
		Join: func(a, b uint64) uint64 {
			if a > b {
				return a
			}
			return b
		},
		Widen: func(prev, next uint64) uint64 {
			if prev > next {
				return prev
			}
			return next
		},
	}
}

func scratchAlgebra() Algebra[uint64] {
	return Algebra[uint64]{
		KeyEnd:      4,
		Lattice:     scratchLattice(),
		Default:     0,
		AdmitAt:     func(key uint64, value uint64) bool { return key < 4 },
		Fingerprint: func(value uint64) uint64 { return value },
		Widen:       Rank[uint64]{Width: 1, At: func(key uint64, value uint64, component int) uint64 { return value }},
	}
}

// scratchSpec is one complete axis declaration. Each law test starts from this
// record and removes exactly the field the law is about.
func scratchSpec(key, semantic schema.Key) Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64] {
	return Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]{
		Key:         key,
		Storage:     StorageFactor,
		Cardinality: CardinalityDense,
		Lifetime:    LifetimeLink,
		Mutability:  MutabilitySolve,
		Concurrency: ConcurrencySingleWriter,
		Semantic:    semantic,
		Declare: func(context Declaration) (*scratchFragment, bool) {
			resolved, ok := context.Roles.Key(semantic)
			return &scratchFragment{semantic: resolved}, ok
		},
		Bind: func(context Binding[scratchInputs, *scratchFragment]) (*scratchAxis, bool) {
			return &scratchAxis{fragment: context.Fragment}, context.Inputs.ready
		},
		Algebra: func(bound *scratchAxis) (Algebra[uint64], bool) { return scratchAlgebra(), true },
	}
}

// sealTemplates seals one axis inventory into a complete declaration table.
// The catalog is walked rather than listed, so the surfaces the declaration
// root settles on do not change what these laws assert.
func sealTemplates(t *testing.T, templates []*Template[scratchInputs]) schema.SealFailure {
	t.Helper()
	_, failure := sealTable(t, templates)
	return failure
}

// sealTable is the same seal, read for the table it produces rather than for
// the verdict alone.
func sealTable(t *testing.T, templates []*Template[scratchInputs]) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindAxis:
			builder.Register(NewSurface(templates))
		case schema.SurfaceKindStructure:
			builder.Register(scratchStructureSurface{})
		default:
			builder.Register(scratchRuleSurface{kind: kind})
		}
	}
	return builder.Seal()
}

func mustTemplate(t *testing.T, spec Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) *Template[scratchInputs] {
	t.Helper()
	template, ok := New(spec)
	if !ok || template == nil {
		t.Fatalf("scratch axis %q rejected by construction", spec.Key)
	}
	return template
}
