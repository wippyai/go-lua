package composite

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// scratchEntry is a stand-in row for a sibling surface. The composite surface
// resolves an axis by deriving the axis surface's identity for a key and asking
// the sealed view, so a scratch axis inventory proves the same laws the
// analyzer's own axis records do.
type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(*framing.Writer) error { return nil }

// scratchSurface stands in for one sibling surface of the catalog. The
// declaration root requires every catalog member to be populated, so a
// composite law is stated against a complete table rather than a half
// registered one.
type scratchSurface struct {
	kind schema.SurfaceKind
	keys []schema.Key
}

func (contribution scratchSurface) Kind() schema.SurfaceKind { return contribution.kind }

func (contribution scratchSurface) Entries() []schema.Entry {
	keys := contribution.keys
	if len(keys) == 0 {
		keys = []schema.Key{"scratch"}
	}
	entries := make([]schema.Entry, len(keys))
	for index, key := range keys {
		entries[index] = scratchEntry{key: key}
	}
	return entries
}

func (contribution scratchSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// scratchAxes is the axis inventory every composite law below is sealed
// against: two membership axes, one output axis, and one intermediate.
func scratchAxes() []schema.Key {
	return []schema.Key{"container", "member", "reachable", "frontier"}
}

func contractID(role string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(role)))
}

// sealEntries seals one composite inventory into a complete declaration table.
// The catalog is walked rather than listed, so the surfaces the declaration
// root settles on do not change what these laws assert, and the axis surface a
// composite resolves its membership against carries a real inventory.
func sealEntries(t *testing.T, entries []*Entry) schema.SealFailure {
	t.Helper()
	_, failure := sealTable(t, entries)
	return failure
}

// sealTable is the same seal, read for the table it produces rather than for
// the verdict alone.
func sealTable(t *testing.T, entries []*Entry) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	return sealSurface(t, NewSurface(entries))
}

// sealSurface seals one composite contribution into a complete declaration
// table. It is the same table sealTable builds, stated over the contribution
// rather than over the inventory, so a contribution that is not this package's
// own surface is sealed under exactly the laws above.
func sealSurface(t *testing.T, contribution schema.Surface) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindComposite:
			builder.Register(contribution)
		case schema.SurfaceKindAxis:
			builder.Register(scratchSurface{kind: kind, keys: scratchAxes()})
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	return builder.Seal()
}

// foreignSurface contributes a row that is not this surface's entry type, under
// this surface's own kind, and states this surface's laws over it.
type foreignSurface struct{}

func (foreignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindComposite }

func (foreignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: "foreign"}}
}

func (foreignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

// containmentSpec is the toy relation this surface is proved against: a
// containment-shaped composite over two axes. The whole is read exactly; the
// parts are the point set demanded by that read; the relation folds onto a
// third axis and routes through a declared frontier.
func containmentSpec(key schema.Key) Spec {
	return Spec{
		Key: key,
		Roles: []Role{
			{Key: "whole", Axis: "container", Cone: Cone{Form: ConeExact}},
			{Key: "part", Axis: "member", Cone: Cone{Form: ConeDemand, Source: "whole"}},
		},
		Ordering: OrderingOrdered,
		Output: Output{
			Kind:    OutputReducer,
			Reducer: Reducer{Axis: "reachable", Descent: []uint16{0, 2}},
		},
		Discipline: Discipline{
			Determinism:  DeterminismDeterministic,
			Monotonicity: MonotonicityMonotone,
			Reentrancy:   ReentrancyExclusive,
		},
		Intermediates: []schema.Key{"frontier"},
	}
}

// overlapSpec is the commutative toy: two indistinguishable roles over one
// axis, handed to the store as a capability rather than folded.
func overlapSpec(key schema.Key) Spec {
	return Spec{
		Key: key,
		Roles: []Role{
			{Key: "left", Axis: "member", Cone: Cone{Form: ConeSummary}},
			{Key: "right", Axis: "member", Cone: Cone{Form: ConeSummary}},
		},
		Ordering: OrderingCommutative,
		Output: Output{
			Kind: OutputCapability,
			Capability: Capability{
				Patches: []Patch{
					{Role: "left", Contract: contractID("patch/left")},
					{Role: "right", Contract: contractID("patch/right")},
				},
				Closure: contractID("closure/overlap"),
				Commit:  contractID("commit/overlap"),
			},
		},
		Discipline: Discipline{
			Determinism:  DeterminismDeterministic,
			Monotonicity: MonotonicityMonotone,
			Reentrancy:   ReentrancyReentrant,
		},
	}
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok || entry == nil {
		t.Fatalf("scratch composite %q rejected by construction", spec.Key)
	}
	return entry
}

// TestCompositeSurfaceSealsCompleteInventory is the baseline: a complete
// composite inventory is admitted, indexed, and sealed with no verdict.
func TestCompositeSurfaceSealsCompleteInventory(t *testing.T) {
	entries := []*Entry{
		mustEntry(t, containmentSpec("containment")),
		mustEntry(t, overlapSpec("overlap")),
	}
	if failure := sealEntries(t, entries); failure.Available() {
		t.Fatalf("complete composite inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestToyCompositeModelsAContainmentRelation is the modeling proof: the
// surface holds a real relation - membership by role over two axes, an exact
// read on one side, the point set demanded by that read on the other, a fold
// onto a third axis, and a declared intermediate - and reads it back
// unchanged.
func TestToyCompositeModelsAContainmentRelation(t *testing.T) {
	spec := containmentSpec("containment")
	entry := mustEntry(t, spec)
	if failure := sealEntries(t, []*Entry{entry}); failure.Available() {
		t.Fatalf("containment composite rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if entry.RoleCount() != 2 || entry.Ordering() != OrderingOrdered {
		t.Fatalf("membership lost: roles=%d ordering=%d", entry.RoleCount(), entry.Ordering())
	}
	whole, wholeOK := entry.Role("whole")
	if !wholeOK || whole.Axis != "container" || whole.Cone.Form != ConeExact || whole.Cone.Source.Available() {
		t.Fatal("the containing side is not an exact read over its own axis")
	}
	part, partOK := entry.Role("part")
	if !partOK || part.Axis != "member" || part.Cone.Form != ConeDemand || part.Cone.Source != "whole" {
		t.Fatal("the contained side is not the point set demanded by the containing read")
	}
	output := entry.Output()
	if output.Kind != OutputReducer || output.Reducer.Axis != "reachable" {
		t.Fatalf("relation does not fold onto its declared output axis: kind=%d", output.Kind)
	}
	if len(output.Reducer.Descent) != 2 || output.Reducer.Descent[0] != 0 || output.Reducer.Descent[1] != 2 {
		t.Fatal("declared rank descent lost")
	}
	if !output.Capability.Absent() {
		t.Fatal("the unselected case of the output union is populated")
	}
	intermediate, intermediateOK := entry.IntermediateAt(0)
	if entry.IntermediateCount() != 1 || !intermediateOK || intermediate != "frontier" {
		t.Fatal("declared intermediate axis lost")
	}
	discipline := entry.Discipline()
	if discipline.Determinism != DeterminismDeterministic || discipline.Monotonicity != MonotonicityMonotone ||
		discipline.Reentrancy != ReentrancyExclusive {
		t.Fatal("declared solver discipline lost")
	}
	// The declaration is a value: the entry was built from this very spec, so
	// rewriting the authored slices now must not reach what was sealed.
	spec.Roles[0].Axis = "member"
	spec.Output.Reducer.Descent[0] = 9
	if again, ok := entry.RoleAt(0); !ok || again.Axis != "container" {
		t.Fatal("the sealed membership aliases its authored spec")
	}
	if entry.Output().Reducer.Descent[0] != 0 {
		t.Fatal("the sealed reducer aliases its authored spec")
	}
	// The other half of the same law: the reducer descent read back above is a
	// collection, so a reader that rewrites it must not reach the entry either.
	output.Reducer.Descent[0] = 9
	if entry.Output().Reducer.Descent[0] != 0 {
		t.Fatal("the sealed reducer aliases the descent it handed back")
	}
}

// TestSealedCapabilityDoesNotAliasItsReader states the same value law over the
// other case of the output union: the patch contracts a capability hands back
// are a collection, and rewriting them may not reach the sealed entry.
func TestSealedCapabilityDoesNotAliasItsReader(t *testing.T) {
	spec := overlapSpec("overlap")
	entry := mustEntry(t, spec)
	if failure := sealEntries(t, []*Entry{entry}); failure.Available() {
		t.Fatalf("overlap composite rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	authored := entry.Output().Capability.Patches[0].Contract
	spec.Output.Capability.Patches[0].Contract = contractID("patch/rewritten")
	if entry.Output().Capability.Patches[0].Contract != authored {
		t.Fatal("the sealed capability aliases its authored spec")
	}
	entry.Output().Capability.Patches[0].Contract = contractID("patch/rewritten")
	if entry.Output().Capability.Patches[0].Contract != authored {
		t.Fatal("the sealed capability aliases the patch list it handed back")
	}
}

// TestCompositeIdentityIsThisSurfaceDerivation states that a composite carries
// this surface's own derivation of its key. An entry identity minted for
// another surface names another entry, so it may not travel here.
func TestCompositeIdentityIsThisSurfaceDerivation(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.id = schema.NewEntryID(schema.SurfaceKindAxis, entry.key)
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawCompositeIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeKeyIsUnique states that two composites cannot share one
// authored identity.
func TestCompositeKeyIsUnique(t *testing.T) {
	first := mustEntry(t, containmentSpec("containment"))
	second := mustEntry(t, overlapSpec("containment"))
	failure := sealEntries(t, []*Entry{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate composite key sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeMembershipIsDeclared states that a relation with no membership
// is not a relation.
func TestCompositeMembershipIsDeclared(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.roles = nil
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawMembershipDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("composite without membership sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	entry = mustEntry(t, containmentSpec("containment"))
	entry.roles[1].Axis = ""
	if failure = sealEntries(t, []*Entry{entry}); failure.Law != LawMembershipDeclared ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("role without an axis sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeRoleKeysAreUnique states that a composite distinguishes its own
// members: two roles under one name are one role.
func TestCompositeRoleKeysAreUnique(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.roles[1].Key = entry.roles[0].Key
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawRoleUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate role key sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeMembershipResolvesAgainstSealedAxes states that a role ranges
// over a coordinate space that exists in the same table.
func TestCompositeMembershipResolvesAgainstSealedAxes(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.roles[0].Axis = "absent"
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawMembershipResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("membership over an undeclared axis sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeConeFormsAreWellFormed states that every role declares exactly
// one read shape, and that a source is declared by the demand form alone.
func TestCompositeConeFormsAreWellFormed(t *testing.T) {
	cases := map[string]Cone{
		"undeclared form":        {Form: ConeInvalid},
		"source without demand":  {Form: ConeExact, Source: "whole"},
		"demand without source":  {Form: ConeDemand},
		"out of catalog ordinal": {Form: ConeDemand + 1, Source: "whole"},
	}
	for name, cone := range cases {
		entry := mustEntry(t, containmentSpec("containment"))
		entry.roles[1].Cone = cone
		failure := sealEntries(t, []*Entry{entry})
		if failure.Law != LawConeForm || failure.Disposition != schema.DispositionMalformed {
			t.Fatalf("cone with %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestCompositeDemandConeNamesAnotherRole states that a demand cone derives its
// point set from a read this composite actually performs, and never from
// itself.
func TestCompositeDemandConeNamesAnotherRole(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.roles[1].Cone.Source = entry.roles[1].Key
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawDemandSource || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("self-sourced demand cone sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	entry = mustEntry(t, containmentSpec("containment"))
	entry.roles[1].Cone.Source = "absent"
	if failure = sealEntries(t, []*Entry{entry}); failure.Law != LawDemandSource ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("demand cone sourced from a role outside the composite sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeDemandConeRequiresMonotonicity states that a composite whose
// read set is derived from its own reads declares the monotonicity that
// fixpoint needs.
func TestCompositeDemandConeRequiresMonotonicity(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.discipline.Monotonicity = MonotonicityNonMonotone
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawDemandMonotone || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("non-monotone demand composite sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeOutputDisciplineIsExactlyOneCase states the closed union: a
// composite that populates both cases, or neither, is not a union.
func TestCompositeOutputDisciplineIsExactlyOneCase(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.output.Kind = OutputInvalid
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawOutputDiscipline || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("composite without an output discipline sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	both := mustEntry(t, containmentSpec("containment"))
	both.output.Capability = overlapSpec("overlap").Output.Capability
	if failure = sealEntries(t, []*Entry{both}); failure.Law != LawOutputDiscipline ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("composite carrying both output cases sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	neither := mustEntry(t, containmentSpec("containment"))
	neither.output.Reducer = Reducer{}
	if failure = sealEntries(t, []*Entry{neither}); failure.Law != LawOutputDiscipline ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("composite carrying no output case sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	crossed := mustEntry(t, overlapSpec("overlap"))
	crossed.output.Kind = OutputReducer
	if failure = sealEntries(t, []*Entry{crossed}); failure.Law != LawOutputDiscipline ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("composite whose discriminant names the unpopulated case sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeReducerOutputAxisIsDeclaredAndNotRead states that a fold names a
// declared axis, and that the axis it writes is not one it reads.
func TestCompositeReducerOutputAxisIsDeclaredAndNotRead(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.output.Reducer.Axis = "absent"
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawReducerOutputAxis || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("fold onto an undeclared axis sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	reading := mustEntry(t, containmentSpec("containment"))
	reading.output.Reducer.Axis = "container"
	if failure = sealEntries(t, []*Entry{reading}); failure.Law != LawReducerOutputAxis ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("fold onto an axis the composite reads sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositeReducerDeclaresARankDescent states the termination half of the
// fold: the descending components are declared, and their order is
// unambiguous.
func TestCompositeReducerDeclaresARankDescent(t *testing.T) {
	for name, descent := range map[string][]uint16{
		"no components":       nil,
		"repeated component":  {1, 1},
		"unordered component": {2, 0},
	} {
		entry := mustEntry(t, containmentSpec("containment"))
		entry.output.Reducer.Descent = descent
		failure := sealEntries(t, []*Entry{entry})
		if failure.Law != LawReducerDescent || failure.Disposition != schema.DispositionMalformed {
			t.Fatalf("reducer with %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestCompositeCapabilityCoversEveryRole states the store-handoff half: every
// role has exactly one declared patch contract, and the closure and commit
// identities are declared.
func TestCompositeCapabilityCoversEveryRole(t *testing.T) {
	entry := mustEntry(t, overlapSpec("overlap"))
	entry.output.Capability.Patches = entry.output.Capability.Patches[:1]
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawCapabilityContract || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("capability missing a role's patch sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	repeated := mustEntry(t, overlapSpec("overlap"))
	repeated.output.Capability.Patches[1].Role = repeated.output.Capability.Patches[0].Role
	if failure = sealEntries(t, []*Entry{repeated}); failure.Law != LawCapabilityContract ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("capability patching one role twice sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	for name, damage := range map[string]func(*Entry){
		"closure":  func(entry *Entry) { entry.output.Capability.Closure = identity.ContentID{} },
		"commit":   func(entry *Entry) { entry.output.Capability.Commit = identity.ContentID{} },
		"contract": func(entry *Entry) { entry.output.Capability.Patches[0].Contract = identity.ContentID{} },
	} {
		damaged := mustEntry(t, overlapSpec("overlap"))
		damage(damaged)
		if failure = sealEntries(t, []*Entry{damaged}); failure.Law != LawCapabilityContract ||
			failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("capability without a %s identity sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestCompositeDisciplineIsDeclared states that a composite says how it behaves
// under the solver before it is admitted to one.
func TestCompositeDisciplineIsDeclared(t *testing.T) {
	for name, damage := range map[string]func(*Entry){
		"determinism":  func(entry *Entry) { entry.discipline.Determinism = DeterminismInvalid },
		"monotonicity": func(entry *Entry) { entry.discipline.Monotonicity = MonotonicityInvalid },
		"reentrancy":   func(entry *Entry) { entry.discipline.Reentrancy = ReentrancyInvalid },
		"ordering":     func(entry *Entry) { entry.ordering = OrderingInvalid },
	} {
		// The overlap toy declares no demand cone, so an undeclared
		// monotonicity reaches the discipline law rather than the demand law.
		entry := mustEntry(t, overlapSpec("overlap"))
		damage(entry)
		failure := sealEntries(t, []*Entry{entry})
		if failure.Law != LawDisciplineDeclared || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("composite without a declared %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestCompositeDeclaresNoHiddenState states that every coordinate space a
// composite routes through is a declared axis entry, and that an intermediate
// is not a second name for a space the composite already declared.
func TestCompositeDeclaresNoHiddenState(t *testing.T) {
	entry := mustEntry(t, containmentSpec("containment"))
	entry.intermediates = []schema.Key{"absent"}
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawNoHiddenState || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("intermediate over an undeclared axis sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	unnamed := mustEntry(t, containmentSpec("containment"))
	unnamed.intermediates = []schema.Key{""}
	if failure = sealEntries(t, []*Entry{unnamed}); failure.Law != LawNoHiddenState ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unnamed intermediate sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	for name, key := range map[string]schema.Key{"role axis": "container", "output axis": "reachable"} {
		repeated := mustEntry(t, containmentSpec("containment"))
		repeated.intermediates = []schema.Key{key}
		if failure = sealEntries(t, []*Entry{repeated}); failure.Law != LawNoHiddenState ||
			failure.Disposition != schema.DispositionDuplicate {
			t.Fatalf("intermediate repeating the %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestCompositeCommutativityMatchesRoleSymmetry states the biconditional. A
// declaration is data: roles the declaration cannot tell apart are
// interchangeable, so commutativity must be declared exactly when they are, and
// never when the declaration does distinguish them.
func TestCompositeCommutativityMatchesRoleSymmetry(t *testing.T) {
	overclaimed := mustEntry(t, containmentSpec("containment"))
	overclaimed.ordering = OrderingCommutative
	failure := sealEntries(t, []*Entry{overclaimed})
	if failure.Law != LawCommutativity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("commutativity declared over distinguishable roles sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	withheld := mustEntry(t, overlapSpec("overlap"))
	withheld.ordering = OrderingOrdered
	if failure = sealEntries(t, []*Entry{withheld}); failure.Law != LawCommutativity ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("indistinguishable roles sealed without commutativity: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	// Giving one of the two roles distinguishing structure is what makes an
	// asymmetric relation over a single axis declarable.
	distinguished := overlapSpec("overlap")
	distinguished.Roles[1].Cone = Cone{Form: ConeDemand, Source: "left"}
	distinguished.Ordering = OrderingOrdered
	if _, ok := New(distinguished); !ok {
		t.Fatal("an asymmetric relation over one axis is not declarable")
	}
}

// TestCompositeDependencyEdgesResolve states that a declared dependency names a
// composite in this table.
func TestCompositeDependencyEdgesResolve(t *testing.T) {
	spec := containmentSpec("containment")
	spec.Dependencies = []schema.Key{"absent"}
	failure := sealEntries(t, []*Entry{mustEntry(t, spec)})
	if failure.Law != LawDependencyResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved dependency sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	resolved := overlapSpec("overlap")
	resolved.Dependencies = []schema.Key{"containment"}
	if failure = sealEntries(t, []*Entry{
		mustEntry(t, containmentSpec("containment")),
		mustEntry(t, resolved),
	}); failure.Available() {
		t.Fatalf("resolved dependency rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	self := mustEntry(t, containmentSpec("containment"))
	self.dependencies = []schema.Key{"containment"}
	if failure = sealEntries(t, []*Entry{self}); failure.Law != LawDependencyResolves ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("self-edge sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositesSealAfterAxes is the phase law. A composite resolves its
// membership against the axis inventory, so the axis surface is sealed below
// it and a table registered the other way round is rejected by the root.
func TestCompositesSealAfterAxes(t *testing.T) {
	builder := schema.NewBuilder()
	builder.Register(NewSurface([]*Entry{mustEntry(t, containmentSpec("containment"))}))
	builder.Register(scratchSurface{kind: schema.SurfaceKindAxis, keys: scratchAxes()})
	_, failure := builder.Seal()
	if failure.Law != schema.LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("axis surface registered after the composite surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCompositesRequireASealedAxisSurface is the other half of the phase law,
// stated by this surface rather than by the root. Registration order is only
// strictly increasing, so a table may skip the axis surface entirely; a
// composite that cannot reach the axis inventory cannot resolve one role, and
// says so instead of sealing a membership over nothing.
func TestCompositesRequireASealedAxisSurface(t *testing.T) {
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindAxis:
		case schema.SurfaceKindComposite:
			builder.Register(NewSurface([]*Entry{mustEntry(t, containmentSpec("containment"))}))
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a composite surface sealed without the axis inventory it resolves against")
	}
	if failure.Law != LawAxisPhase || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("law=%d disposition=%s want axis-phase/incomplete", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindComposite {
		t.Fatalf("verdict attributed to surface %d, not the composite surface", failure.Contributor)
	}
}

// TestCompositeForeignRowIsRejected states the entry-shape law: this surface's
// laws are stated over its own record, so a row admitted under the composite
// kind that is not a composite entry is rejected rather than read as one.
func TestCompositeForeignRowIsRejected(t *testing.T) {
	sealed, failure := sealSurface(t, foreignSurface{})
	if sealed != nil {
		t.Fatal("a foreign row was admitted into the composite surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("law=%d disposition=%s want entry-shape/malformed", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindComposite {
		t.Fatalf("verdict attributed to surface %d, not the composite surface", failure.Contributor)
	}
}

// TestNewRejectsIncompleteSpec states the constructor half of every law above:
// a spec that violates one yields no entry at all.
func TestNewRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*Spec){
		"key":            func(spec *Spec) { spec.Key = "" },
		"membership":     func(spec *Spec) { spec.Roles = nil },
		"role key":       func(spec *Spec) { spec.Roles[0].Key = "" },
		"role axis":      func(spec *Spec) { spec.Roles[0].Axis = "" },
		"cone form":      func(spec *Spec) { spec.Roles[0].Cone = Cone{} },
		"duplicate role": func(spec *Spec) { spec.Roles[1].Key = spec.Roles[0].Key },
		"demand source":  func(spec *Spec) { spec.Roles[1].Cone.Source = "absent" },
		"self demand":    func(spec *Spec) { spec.Roles[1].Cone.Source = spec.Roles[1].Key },
		"demand monotonicity": func(spec *Spec) {
			spec.Discipline.Monotonicity = MonotonicityNonMonotone
		},
		"ordering":    func(spec *Spec) { spec.Ordering = OrderingInvalid },
		"determinism": func(spec *Spec) { spec.Discipline.Determinism = DeterminismInvalid },
		"reentrancy":  func(spec *Spec) { spec.Discipline.Reentrancy = ReentrancyInvalid },
		"output kind": func(spec *Spec) { spec.Output.Kind = OutputInvalid },
		"output case": func(spec *Spec) { spec.Output.Reducer = Reducer{} },
		"output axis": func(spec *Spec) { spec.Output.Reducer.Axis = "container" },
		"descent":     func(spec *Spec) { spec.Output.Reducer.Descent = nil },
		"descent order": func(spec *Spec) {
			spec.Output.Reducer.Descent = []uint16{3, 1}
		},
		"intermediate":        func(spec *Spec) { spec.Intermediates = []schema.Key{""} },
		"intermediate repeat": func(spec *Spec) { spec.Intermediates = []schema.Key{"container"} },
		"self edge":           func(spec *Spec) { spec.Dependencies = []schema.Key{"containment"} },
		"unnamed edge":        func(spec *Spec) { spec.Dependencies = []schema.Key{""} },
		"commutativity":       func(spec *Spec) { spec.Ordering = OrderingCommutative },
	}
	for name, damage := range cases {
		spec := containmentSpec("containment")
		damage(&spec)
		if entry, ok := New(spec); ok || entry != nil {
			t.Fatalf("spec with a rejected %s admitted", name)
		}
	}
	capabilityCases := map[string]func(*Spec){
		"patch coverage": func(spec *Spec) {
			spec.Output.Capability.Patches = spec.Output.Capability.Patches[:1]
		},
		"patch contract": func(spec *Spec) {
			spec.Output.Capability.Patches[0].Contract = identity.ContentID{}
		},
		"unknown patch role": func(spec *Spec) { spec.Output.Capability.Patches[0].Role = "absent" },
		"closure":            func(spec *Spec) { spec.Output.Capability.Closure = identity.ContentID{} },
		"commit":             func(spec *Spec) { spec.Output.Capability.Commit = identity.ContentID{} },
	}
	for name, damage := range capabilityCases {
		spec := overlapSpec("overlap")
		damage(&spec)
		if entry, ok := New(spec); ok || entry != nil {
			t.Fatalf("capability spec with a rejected %s admitted", name)
		}
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same composites and declare different relations are two tables. A
// cone form is the read a role performs, so moving one moves the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealTable(t, []*Entry{mustEntry(t, containmentSpec("containment"))})
	if failure.Available() {
		t.Fatalf("toy composite rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	spec := containmentSpec("containment")
	spec.Roles[0].Cone = Cone{Form: ConeSummary}
	shifted, failure := sealTable(t, []*Entry{mustEntry(t, spec)})
	if failure.Available() {
		t.Fatalf("composite with a shifted cone form rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a composite's declared cone form left the table digest unchanged")
	}
}
