package axis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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

// TestAxisSurfaceSealsCompleteInventory is the baseline: a complete axis
// declaration is admitted, indexed, and sealed with no verdict.
func TestAxisSurfaceSealsCompleteInventory(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
		mustTemplate(t, scratchSpec("heap", heapRole)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("complete axis inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// scratchForeignSurface contributes rows of a foreign record under this
// surface's own kind and states this surface's laws over them. It is how a
// contribution that is not an axis declaration reaches the axis surface through
// the public seal path.
type scratchForeignSurface struct{}

func (scratchForeignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindAxis }

func (scratchForeignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchRuleEntry{key: "scratch-foreign"}}
}

func (scratchForeignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface[scratchInputs]{}.Seal(view, sealed)
}

// TestAxisSurfaceRejectsAForeignRow states that the axis surface reads axis
// declarations and nothing else. A row of another record type carries none of
// the declared data every axis law is stated over, so it is rejected as the
// wrong shape rather than read as a partially declared axis.
func TestAxisSurfaceRejectsAForeignRow(t *testing.T) {
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindAxis {
			builder.Register(scratchForeignSurface{})
			continue
		}
		builder.Register(scratchRuleSurface{kind: kind})
	}
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a foreign row was admitted into the axis surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign row rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindAxis {
		t.Fatalf("shape verdict named surface %d, not the axis surface", failure.Contributor)
	}
}

// TestAxisIdentityIsThisSurfaceDerivation states that an axis carries this
// surface's own derivation of its key. An entry identity minted for another
// surface names another entry, so it may not travel here.
func TestAxisIdentityIsThisSurfaceDerivation(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	template.id = schema.NewEntryID(schema.SurfaceKindRule, template.key)
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawAxisIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxisKeyIsUnique states that two axes cannot share one authored
// identity. The entry identity is derived from the key, so the declaration
// root rejects the duplicate before any axis law is reached.
func TestAxisKeyIsUnique(t *testing.T) {
	first := mustTemplate(t, scratchSpec("value", valueRole))
	second := mustTemplate(t, scratchSpec("value", heapRole))
	failure := sealTemplates(t, []*Template[scratchInputs]{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate axis key sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxisSemanticIsDeclared states that an axis resolves to one canonical
// engine identity in the vocabulary it is sealed against.
func TestAxisSemanticIsDeclared(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	template.semantic = absentRole
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawSemanticIdentity || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("axis without a canonical identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxisRequiredFieldsAreComplete states that every closure field of an axis
// entry is present. An axis whose bind thunk is missing cannot be bound, and
// the table says so at seal rather than at bind.
func TestAxisRequiredFieldsAreComplete(t *testing.T) {
	for _, missing := range []string{"semantic", "declare", "bind"} {
		template := mustTemplate(t, scratchSpec("value", valueRole))
		switch missing {
		case "semantic":
			template.semantic = ""
		case "declare":
			template.declare = nil
		case "bind":
			template.bind = nil
		}
		failure := sealTemplates(t, []*Template[scratchInputs]{template})
		if failure.Law != LawFieldComplete || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("axis without %s sealed: law=%d disposition=%s", missing, failure.Law, failure.Disposition)
		}
	}
}

// TestAxisMetadataIsComplete states the declared half of the same
// completeness: an axis identity alone is not a declaration.
func TestAxisMetadataIsComplete(t *testing.T) {
	for _, missing := range []string{"storage", "cardinality", "lifetime", "mutability", "concurrency"} {
		template := mustTemplate(t, scratchSpec("value", valueRole))
		switch missing {
		case "storage":
			template.storage = StorageInvalid
		case "cardinality":
			template.cardinality = CardinalityInvalid
		case "lifetime":
			template.lifetime = LifetimeInvalid
		case "mutability":
			template.mutability = MutabilityInvalid
		case "concurrency":
			template.concurrency = ConcurrencyInvalid
		}
		failure := sealTemplates(t, []*Template[scratchInputs]{template})
		if failure.Law != LawMetadataComplete || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("axis without %s sealed: law=%d disposition=%s", missing, failure.Law, failure.Disposition)
		}
	}
}

// TestAxisDependencyEdgesResolve states that a declared dependency names an
// axis in this table.
func TestAxisDependencyEdgesResolve(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Dependencies = []schema.Key{"absent"}
	template := mustTemplate(t, spec)
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawDependencyResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved dependency sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	resolved := scratchSpec("heap", heapRole)
	resolved.Dependencies = []schema.Key{"value"}
	if failure = sealTemplates(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
		mustTemplate(t, resolved),
	}); failure.Available() {
		t.Fatalf("resolved dependency rejected: law=%d", failure.Law)
	}
}

// TestAxisSemanticIsUnique states that two axes cannot claim one canonical
// engine identity.
func TestAxisSemanticIsUnique(t *testing.T) {
	first := mustTemplate(t, scratchSpec("value", valueRole))
	second := mustTemplate(t, scratchSpec("heap", valueRole))
	failure := sealTemplates(t, []*Template[scratchInputs]{first, second})
	if failure.Law != LawSemanticUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("shared axis semantic sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxesBindBeforeRules states the root's phase law over the axis surface.
// The declaration catalog order is the bind phase order, so a table that
// registers the rule surface before the axis surface is rejected by the root.
func TestAxesBindBeforeRules(t *testing.T) {
	builder := schema.NewBuilder()
	builder.Register(scratchRuleSurface{kind: schema.SurfaceKindRule})
	builder.Register(NewSurface([]*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
	}))
	_, failure := builder.Seal()
	if failure.Law != schema.LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("rule surface registered before the axis surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestNewRejectsIncompleteSpec states the constructor half of completeness: a
// spec missing any required field yields no template at all.
func TestNewRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]){
		"key":     func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) { spec.Key = "" },
		"storage": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) { spec.Storage = StorageInvalid },
		"cardinality": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) {
			spec.Cardinality = CardinalityInvalid
		},
		"lifetime": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) {
			spec.Lifetime = LifetimeInvalid
		},
		"mutability": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) {
			spec.Mutability = MutabilityInvalid
		},
		"concurrency": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) {
			spec.Concurrency = ConcurrencyInvalid
		},
		"semantic": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) { spec.Semantic = "" },
		"declare":  func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) { spec.Declare = nil },
		"bind":     func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) { spec.Bind = nil },
		"algebra":  func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) { spec.Algebra = nil },
		"self-edge": func(spec *Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]) {
			spec.Dependencies = []schema.Key{"value"}
		},
	}
	for missing, damage := range cases {
		spec := scratchSpec("value", valueRole)
		damage(&spec)
		if template, ok := New(spec); ok || template != nil {
			t.Fatalf("spec without %s admitted", missing)
		}
	}
}

// TestBoundAxisPublishesItsAlgebra states that binding an axis and publishing
// the algebra of that binding are one step: a bound axis without a complete
// algebra is not published.
func TestBoundAxisPublishesItsAlgebra(t *testing.T) {
	rows, rowsOK := structure.Collect(vocabulary.RoleSpecs("factor/value", "factor/heap", "factor/pack"))
	roles, rolesOK := vocabulary.NewRoles(rows)
	if !rowsOK || !rolesOK {
		t.Fatal("scratch semantic role vocabulary")
	}
	valueFactor, valueFactorOK := roles.Key(valueRole)
	if !valueFactorOK {
		t.Fatal("scratch value role did not resolve")
	}
	spec := scratchSpec("value", valueRole)
	template := mustTemplate(t, spec)
	builder := engine.NewSchema()
	fragment, declared := template.Declare(Declaration{Builder: builder, Roles: roles})
	if !declared || !fragment.Available() {
		t.Fatal("scratch axis did not declare")
	}
	// The bind thunk needs one open engine binding to carry; the scratch axis
	// declares no engine slot of its own, so the schema below is the smallest
	// sealable one.
	if _, slotOK := engine.NewFactorSlot[uint64](builder, valueFactor); !slotOK {
		t.Fatal("scratch factor slot")
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK {
		t.Fatal("scratch schema did not seal")
	}
	binding := engine.NewSchemaBinding(sealed)
	if binding == nil {
		t.Fatal("scratch binding")
	}
	bound, boundOK := template.Bind(binding, scratchInputs{ready: true}, fragment)
	if !boundOK || !bound.AlgebraAvailable() {
		t.Fatal("bound axis published no algebra")
	}
	algebra, algebraOK := AlgebraOf[uint64](bound)
	if !algebraOK || algebra.KeyEnd != 4 || !algebra.AdmitAt(3, 1) || algebra.AdmitAt(4, 1) {
		t.Fatal("published algebra does not answer for the axis key space")
	}
	if _, wrongType := AlgebraOf[uint32](bound); wrongType {
		t.Fatal("published algebra answered at a foreign fact type")
	}
	if _, rejected := template.Bind(binding, scratchInputs{}, fragment); rejected {
		t.Fatal("axis published a cell for a rejected binding")
	}
	incomplete := scratchSpec("heap", heapRole)
	incomplete.Algebra = func(*scratchAxis) (Algebra[uint64], bool) { return Algebra[uint64]{}, true }
	if _, published := mustTemplate(t, incomplete).Bind(binding, scratchInputs{ready: true}, fragment); published {
		t.Fatal("axis published an incomplete algebra")
	}
}

// TestAdoptProjectsOneEngineAlgebra states the single conversion law: the
// surface's ordinal view of an algebra is the engine spec the owner binds,
// projected, and a key outside the declared space is rejected rather than
// truncated into a neighbour.
func TestAdoptProjectsOneEngineAlgebra(t *testing.T) {
	type carrier uint32
	admitted := map[carrier]bool{0: true, 1: true}
	spec := engine.HotFactorSpec[carrier, uint64]{
		KeyEnd:      2,
		Lattice:     scratchLattice(),
		Default:     0,
		AdmitAt:     func(key carrier, value uint64) bool { return admitted[key] },
		Fingerprint: func(value uint64) uint64 { return value ^ 7 },
		WidenRank:   engine.Measure[carrier, uint64]{Width: 2, At: func(key carrier, value uint64, component int) uint64 { return uint64(key) + uint64(component) }},
	}
	algebra, ok := Adopt(spec)
	if !ok || !algebra.Available() {
		t.Fatal("complete engine spec was not projected")
	}
	if algebra.KeyEnd != 2 || algebra.Fingerprint(1) != spec.Fingerprint(1) || algebra.Default != spec.Default {
		t.Fatal("projection lost the declared algebra")
	}
	if !algebra.AdmitAt(1, 0) || algebra.AdmitAt(2, 0) {
		t.Fatal("projected admission does not fence the declared key space")
	}
	if algebra.Widen.Width != 2 || algebra.Widen.At(1, 0, 1) != 2 || algebra.Widen.At(2, 0, 1) != 0 {
		t.Fatal("projected widen rank does not fence the declared key space")
	}
	if !algebra.Narrow.Absent() || algebra.Narrow.Available() {
		t.Fatal("undeclared narrow rank was projected as declared")
	}
	spec.AdmitAt = nil
	if _, admitted := Adopt(spec); admitted {
		t.Fatal("engine spec without an admission predicate was projected")
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same axes and declare different coordinate spaces are two tables.
// The storage an axis's facts live in is read by every consumer of the space,
// so moving it moves the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealTable(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
	})
	if failure.Available() {
		t.Fatalf("scratch axis inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	spec := scratchSpec("value", valueRole)
	spec.Storage = StorageStatic
	shifted, failure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, spec)})
	if failure.Available() {
		t.Fatalf("axis with a shifted storage rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("an axis's declared storage left the table digest unchanged")
	}
}

// TestTableDigestCoversSemanticIdentity is the identity half of the same drift
// law. An axis's canonical identity is the role it selects from the closed
// vocabulary, and every engine binding of the axis is made under that identity,
// so two catalogs whose axes name the same key and select different roles are
// two tables. The two inventories below differ in the selected role and in
// nothing else.
func TestTableDigestCoversSemanticIdentity(t *testing.T) {
	declared, failure := sealTable(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
	})
	if failure.Available() {
		t.Fatalf("scratch axis inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	shifted, failure := sealTable(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", heapRole)),
	})
	if failure.Available() {
		t.Fatalf("axis with a shifted semantic role rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("an axis's selected semantic role left the table digest unchanged")
	}
}
