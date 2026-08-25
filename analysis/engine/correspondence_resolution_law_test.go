package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	program "github.com/wippyai/go-lua/analysis/schema/rule/program"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// directoryOwner is one axis's published candidate directory: for each
// (relation, mount, occurrence) it answers the rows that occurrence carries,
// in the owner's OWN order. Two directories built from the same occurrences
// therefore disagree about ordinals by construction, which is the situation
// the resolution exists for.
type directoryOwner struct {
	rows map[directoryAddress][]uint32
}

type directoryAddress struct {
	relation   uint32
	mount      identity.ContentID
	occurrence identity.ContentID
}

func (owner directoryOwner) CandidateCount(relation uint32, mount, occurrence identity.ContentID) (int, bool) {
	rows, ok := owner.rows[directoryAddress{relation: relation, mount: mount, occurrence: occurrence}]
	if !ok {
		return 0, false
	}
	return len(rows), true
}

func (owner directoryOwner) CandidateAt(relation uint32, mount, occurrence identity.ContentID, index int) (uint32, bool) {
	rows, ok := owner.rows[directoryAddress{relation: relation, mount: mount, occurrence: occurrence}]
	if !ok || index < 0 || index >= len(rows) {
		return 0, false
	}
	return rows[index], true
}

func (directoryOwner) MemberCount(uint32, uint32) (int, bool)         { return 0, false }
func (directoryOwner) MemberAt(uint32, uint32, int) (uint32, bool)    { return 0, false }
func (directoryOwner) KeyVectorCount(uint32, uint32) (int, bool)      { return 0, false }
func (directoryOwner) KeyVectorAt(uint32, uint32, int) (uint32, bool) { return 0, false }
func (directoryOwner) Project(uint32, uint32, uint32) (uint32, bool) {
	return 0, false
}

func contentID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

// TestACorrespondedRowIsResolvedByOccurrenceNotByOrdinal is the resolution law.
//
// Two directories that enumerate the same subjects number their rows
// independently. The rule resolved ordinal 3 in its own candidate directory;
// the foreign directory addressed by the SAME occurrence answers 7. Reusing 3
// would read whichever of the foreign owner's rows happens to sit at position
// 3, which is a different subject and a silently wrong answer, and no
// comparison of the two ordinals could detect it.
func TestACorrespondedRowIsResolvedByOccurrenceNotByOrdinal(t *testing.T) {
	occurrence := contentID(1)
	mount := contentID(9)
	owner := directoryOwner{rows: map[directoryAddress][]uint32{
		{relation: 4, mount: mount, occurrence: occurrence}: {7},
	}}
	resolved, ok := soleDirectoryCandidate(owner, 4, OperandCoords{Mount: mount, Occurrence: occurrence})
	if !ok || resolved != 7 {
		t.Fatalf("resolved foreign row = %d/%t, want the owner's own ordinal 7", resolved, ok)
	}
}

// TestOwnerOrdersArePermutedIndependently states why the ordinal can never be
// carried across. The same three occurrences are enumerated by two owners in
// unrelated orders; every occurrence resolves correctly through its own
// directory, and at no occurrence do the two orders agree.
func TestOwnerOrdersArePermutedIndependently(t *testing.T) {
	occurrences := []identity.ContentID{contentID(1), contentID(2), contentID(3)}
	mount := contentID(9)
	candidateRows := map[directoryAddress][]uint32{}
	foreignRows := map[directoryAddress][]uint32{}
	candidateOrder := []uint32{0, 1, 2}
	foreignOrder := []uint32{2, 0, 1}
	for index, occurrence := range occurrences {
		candidateRows[directoryAddress{relation: 0, mount: mount, occurrence: occurrence}] = []uint32{candidateOrder[index]}
		foreignRows[directoryAddress{relation: 4, mount: mount, occurrence: occurrence}] = []uint32{foreignOrder[index]}
	}
	candidateOwner := directoryOwner{rows: candidateRows}
	foreignOwner := directoryOwner{rows: foreignRows}
	for index, occurrence := range occurrences {
		coords := OperandCoords{Mount: mount, Occurrence: occurrence}
		own, ownOK := soleDirectoryCandidate(candidateOwner, 0, coords)
		foreign, foreignOK := soleDirectoryCandidate(foreignOwner, 4, coords)
		if !ownOK || !foreignOK {
			t.Fatalf("occurrence %d resolved own=%t foreign=%t", index, ownOK, foreignOK)
		}
		if own != candidateOrder[index] || foreign != foreignOrder[index] {
			t.Fatalf("occurrence %d resolved own=%d foreign=%d, want %d/%d", index, own, foreign, candidateOrder[index], foreignOrder[index])
		}
		if own == foreign {
			t.Fatalf("occurrence %d: the two owner orders agreed at %d, so this fixture proves nothing", index, own)
		}
	}
}

// TestOneOccurrenceResolvesADifferentRowUnderEachMount keeps the address
// complete. A mounted directory is addressed by (mount, occurrence): the same
// occurrence under a second mount is a different subject and a different row,
// so a resolution that dropped the mount would answer the first mount's row
// for every mount.
func TestOneOccurrenceResolvesADifferentRowUnderEachMount(t *testing.T) {
	occurrence := contentID(1)
	first, second := contentID(8), contentID(9)
	owner := directoryOwner{rows: map[directoryAddress][]uint32{
		{relation: 4, mount: first, occurrence: occurrence}:  {5},
		{relation: 4, mount: second, occurrence: occurrence}: {11},
	}}
	firstRow, firstOK := soleDirectoryCandidate(owner, 4, OperandCoords{Mount: first, Occurrence: occurrence})
	secondRow, secondOK := soleDirectoryCandidate(owner, 4, OperandCoords{Mount: second, Occurrence: occurrence})
	if !firstOK || !secondOK || firstRow != 5 || secondRow != 11 {
		t.Fatalf("mount-qualified rows = %d/%t and %d/%t, want 5 and 11", firstRow, firstOK, secondRow, secondOK)
	}
}

// TestADirectoryAnsweringNoRowOrManyRefusesTheIssuance keeps the resolution
// total. A read is one row of the directory it is addressed by: an occurrence
// the directory answers nothing for has no row to read, and one it answers a
// SET for has no single row either. Taking the zeroth of a set, or defaulting
// an absence to zero, would publish a fact about whichever subject happened to
// be first.
func TestADirectoryAnsweringNoRowOrManyRefusesTheIssuance(t *testing.T) {
	occurrence := contentID(1)
	mount := contentID(9)
	owner := directoryOwner{rows: map[directoryAddress][]uint32{
		{relation: 4, mount: mount, occurrence: occurrence}: {},
		{relation: 5, mount: mount, occurrence: occurrence}: {2, 3},
	}}
	if row, ok := soleDirectoryCandidate(owner, 4, OperandCoords{Mount: mount, Occurrence: occurrence}); ok {
		t.Fatalf("an occurrence with no row resolved %d", row)
	}
	if row, ok := soleDirectoryCandidate(owner, 5, OperandCoords{Mount: mount, Occurrence: occurrence}); ok {
		t.Fatalf("an occurrence carrying a candidate set resolved %d", row)
	}
	if row, ok := soleDirectoryCandidate(owner, 6, OperandCoords{Mount: mount, Occurrence: occurrence}); ok {
		t.Fatalf("an undeclared relation resolved %d", row)
	}
}

// TestAReadBorrowingTheCandidateDirectoryReusesTheResolvedOrdinal is the arm
// that must NOT translate. A read addressed by the rule's own candidate
// directory is already resolved; resolving it again would ask one directory
// the same question twice, and would refuse outright wherever the rule's
// candidate came from somewhere other than that directory's census.
func TestAReadBorrowingTheCandidateDirectoryReusesTheResolvedOrdinal(t *testing.T) {
	candidate := ruleplan.RelationAddr{Axis: 1, Member: 2}
	borrowed := generated.ReadPlan{
		Form:       ruleprogram.Exact,
		Addressing: candidate, AddressingPresent: true,
	}
	// A nil binding state is deliberate: this arm must reach no owner at all.
	resolved, ok := resolveGeneratedReadCandidate(nil, candidate, borrowed, OperandCoords{}, 3)
	if !ok || resolved != 3 {
		t.Fatalf("borrowed-directory read resolved %d/%t, want the rule's own 3", resolved, ok)
	}
	unaddressed := generated.ReadPlan{Form: ruleprogram.Selected}
	resolved, ok = resolveGeneratedReadCandidate(nil, candidate, unaddressed, OperandCoords{}, 3)
	if !ok || resolved != 3 {
		t.Fatalf("family-addressed read resolved %d/%t, want the rule's own 3", resolved, ok)
	}
}

// TestAForeignAddressedReadNeverResolvesWithoutItsOwner completes the same
// arm. A read the rule's directory does not issue cannot fall back to the
// rule's ordinal when its owner is unreachable: that fallback IS the defect,
// and it would be invisible because the wrong row reads perfectly well.
func TestAForeignAddressedReadNeverResolvesWithoutItsOwner(t *testing.T) {
	foreign := generated.ReadPlan{
		Form:       ruleprogram.Exact,
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 4}, AddressingPresent: true,
	}
	if resolved, ok := resolveGeneratedReadCandidate(nil, ruleplan.RelationAddr{Axis: 1, Member: 2}, foreign, OperandCoords{}, 3); ok {
		t.Fatalf("a foreign-addressed read resolved %d with no directory to resolve it in", resolved)
	}
}

const (
	correspondedRelationKey   schema.Key = "relation/generated-rule-law-corresponded"
	correspondedProjectionKey schema.Key = "projection/generated-rule-law-corresponded-key"
)

// newCorrespondedRuleLawFixture is the two-directory generated rule: the
// candidate is axis A's own occurrence relation, and the one join reads a
// relation of axis B that is addressed by B's OWN directory and declares that
// its order enumerates the same subjects as A's candidate order.
//
// This is the shape the resolution exists for. Nothing in it is exotic: it is
// the mounted-call shape - one axis owns the call directory, another owns the
// rows hanging off the same call - written at fixture scale.
func newCorrespondedRuleLawFixture(t testing.TB) generatedRuleLawFixture {
	t.Helper()
	axisReference := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: generatedRuleLawAxisKey}
	foreignReference := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: generatedRuleLawUnrelatedAxisKey}
	candidate := member.RelationRef{Axis: axisReference, Member: generatedRuleLawCandidate}
	corresponded := member.RelationRef{Axis: foreignReference, Member: correspondedRelationKey}

	declaration := program.Program{
		OperandRole: generatedRuleLawOperandRole,
		Candidate:   member.AxisRelationCandidate(candidate),
		Joins: []program.JoinDecl{{
			Sources:  []program.SourceRef{program.CandidateSource()},
			Relation: corresponded,
			Key:      member.ProjectionRef{Axis: foreignReference, Member: correspondedProjectionKey},
			Read: program.ReadDecl{
				PointBound: program.PointBound,
				Input:      0, Axis: program.AxisRef(foreignReference), Form: program.Exact,
				Contract: program.ReadContract{
					Order: program.OrderCanonical, Sparse: program.SparseExplicit,
					OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
				},
			},
		}},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: axisReference, Member: generatedRuleLawReducer},
			Inputs:  []program.JoinRef{0},
			Outputs: []program.OutputDecl{{
				Column:      axis.OutputRef{Axis: axisReference, Key: generatedRuleLawOutput},
				Destination: member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawDestination},
				Mode:        program.ModeExact, ValueSlot: 0,
			}},
		},
		Carry: &program.CarryDecl{Input: 1, Mode: program.CarryIdentity},
	}
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("corresponded generated Rule Program rejected: %+v", problem)
	}

	candidateCatalog, candidateCatalogOK := member.NewCatalog(
		[]member.Relation{
			{Key: generatedRuleLawCandidate, Subject: generatedRuleLawCandidateFact, CandidateProvider: member.AxisRelationCandidate(candidate)},
		},
		[]member.Projection{
			{Key: generatedRuleLawDestination, Relation: generatedRuleLawCandidate, Role: member.Destination, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(candidate)},
		},
		[]member.Reducer{{
			Key:     generatedRuleLawReducer,
			Inputs:  []member.ReducerInput{{Axis: foreignReference, Carrier: generatedRuleLawJoinFact, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne}},
			Outputs: []member.ReducerOutput{{Axis: axisReference, Carrier: generatedRuleLawJoinFact}},
		}}, nil,
	)
	if !candidateCatalogOK {
		t.Fatal("corresponded candidate member catalog")
	}
	foreignCatalog, foreignCatalogOK := member.NewCatalog(
		[]member.Relation{{
			Key: correspondedRelationKey, Subject: generatedRuleLawJoinFact,
			Inputs:            []member.Carrier{generatedRuleLawCandidateFact},
			CandidateProvider: member.AxisRelationCandidate(corresponded),
			Correspondences:   []member.RelationRef{candidate},
		}},
		[]member.Projection{{
			Key: correspondedProjectionKey, Relation: correspondedRelationKey, Role: member.Key,
			Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(corresponded),
		}}, nil, nil,
	)
	if !foreignCatalogOK {
		t.Fatal("corresponded foreign member catalog")
	}

	candidateTemplate, candidateAxisOK := axis.New(axis.Spec[struct{}]{
		Key: generatedRuleLawAxisKey, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse,
		Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: generatedRuleLawOutput, Writer: generatedRuleLawAxisKey}}},
		Catalog: candidateCatalog, Signature: axis.Signature{Key: generatedRuleLawKeyCarrier, Fact: generatedRuleLawJoinFact},
		Semantic: generatedRuleLawAxisRole,
	})
	foreignRole := generatedRuleLawUnrelatedRole(t)
	foreignTemplate, foreignAxisOK := axis.New(axis.Spec[struct{}]{
		Key: generatedRuleLawUnrelatedAxisKey, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse,
		Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: generatedRuleLawUnrelatedOutput, Writer: generatedRuleLawUnrelatedAxisKey}}},
		Catalog: foreignCatalog, Signature: axis.Signature{Key: generatedRuleLawKeyCarrier, Fact: generatedRuleLawJoinFact},
		Semantic: foreignRole,
	})
	if !candidateAxisOK || !foreignAxisOK {
		t.Fatalf("corresponded axes = %t/%t", candidateAxisOK, foreignAxisOK)
	}
	programRule, ruleOK := rule.New(rule.Spec{
		Key: generatedRuleLawProgram, Lane: rule.LaneLink, Writes: generatedRuleLawAxisKey, Owner: generatedRuleLawAxisKey,
		Semantic: generatedRuleLawRuleRole, Roles: []schema.Key{generatedRuleLawOperandRole}, Program: declaration,
	})
	absentRule, absentOK := rule.New(rule.Spec{
		Key: generatedRuleLawAbsent, Lane: rule.LaneLink, Writes: generatedRuleLawAxisKey, Owner: generatedRuleLawAxisKey,
		Semantic: generatedRuleLawAbsentRole,
	})
	if !ruleOK || !absentOK {
		t.Fatalf("corresponded rules = %t/%t", ruleOK, absentOK)
	}
	roles := make([]schema.Entry, 0, 5)
	for ordinal, role := range []schema.Key{
		generatedRuleLawAxisRole, generatedRuleLawRuleRole, generatedRuleLawOperandRole,
		generatedRuleLawAbsentRole, foreignRole,
	} {
		entry, entryOK := structure.New(structure.Spec{
			Key: role, Category: structure.CategorySemanticRole, Ordinal: uint16(ordinal + 1), Spelling: string(role), Accepted: true,
		})
		if !entryOK {
			t.Fatalf("corresponded role %q", role)
		}
		roles = append(roles, entry)
	}
	universe, universeOK := identity.DeriveContentID("go-lua/generated-rule-law/axis", []byte(generatedRuleLawAxisKey))
	if !universeOK {
		t.Fatal("corresponded denominator universe")
	}
	coordinate, coordinateOK := denominator.Coordinate(generatedRuleLawAxisKey, universe)
	if !coordinateOK {
		t.Fatal("corresponded denominator")
	}
	builder := seal.NewBuilder()
	register := func(surface seal.Surface) {
		if !builder.Register(surface) {
			t.Fatalf("corresponded surface %d", surface.Kind())
		}
	}
	register(generatedRuleLawSurface{kind: schema.SurfaceKindStructure, entries: roles})
	register(axis.NewSurface([]*axis.Template[struct{}]{candidateTemplate, foreignTemplate}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindIssuance})
	register(rule.NewSurface([]*rule.Template{programRule, absentRule}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindDiagnostic})
	register(generatedRuleLawSurface{kind: schema.SurfaceKindComposite})
	register(denominator.NewSurface([]*denominator.Entry{coordinate}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindQuery})
	register(generatedRuleLawSurface{kind: schema.SurfaceKindObservation})
	table, sealFailure := builder.Seal()
	if sealFailure.Available() || table == nil {
		t.Fatalf("corresponded schema seal: %+v", sealFailure)
	}
	catalog, compileFailure := ruleplan.Compile(table)
	if compileFailure.Available() || !catalog.Available() {
		t.Fatalf("corresponded plan compile: %+v", compileFailure)
	}
	return generatedRuleLawFixture{table: table, catalog: catalog}
}

// TestAnIssuedReadOfACorrespondedDirectoryProjectsTheForeignOwnersOwnRow is
// the wiring law: what the engine actually hands the owner when it mints a
// read surface.
//
// The rule resolves its candidate in axis A and gets row 1. The joined
// relation belongs to axis B, whose owner answers row 6 for the same
// occurrence and projects only that row. Issuing the read with A's ordinal
// asks B for a row it does not have; the surface is minted only because the
// engine resolves the join's own directory by occurrence first.
func TestAnIssuedReadOfACorrespondedDirectoryProjectsTheForeignOwnersOwnRow(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newCorrespondedRuleLawFixture(t))
	cell, cellOK := fixture.binding.state.rules[0].(*generatedRuleBindingCell)
	if !cellOK || cell == nil {
		t.Fatal("corresponded generated cell")
	}
	descriptor := cell.generated.program
	plan, planOK := descriptor.ReadAt(0)
	if !planOK || !plan.AddressingPresent {
		t.Fatalf("corresponded read carries no addressing directory: %+v/%t", plan, planOK)
	}
	candidate := descriptor.CandidateRelation()
	if plan.Addressing == candidate {
		t.Fatal("the corresponded fixture compiled the join as though the rule's own directory issued it")
	}

	candidateOwner := &generatedBindingLawOwner{
		relation: candidate.Member, candidate: 1, acceptCandidate: true,
		projections: map[[2]uint32]uint32{},
	}
	// B numbers its rows independently: the same occurrence is row 6 here.
	// Row 1 - the ordinal A resolved - is a real row of B's order too, and it
	// projects a DIFFERENT coordinate. That is the whole hazard: reusing A's
	// ordinal reads perfectly well and answers about another subject, so the
	// law below pins the coordinate rather than merely that the read issued.
	foreignOwner := &generatedBindingLawOwner{
		relation: plan.Addressing.Member, candidate: 6, acceptCandidate: true,
		projections: map[[2]uint32]uint32{},
		memberProjections: map[[3]uint32]uint32{
			{plan.Relation.Member, plan.Key.Member, 1}: 0,
			{plan.Relation.Member, plan.Key.Member, 6}: 1,
		},
	}
	if !BindRelationOwner(fixture.binding, fixture.factors[candidate.Axis], candidateOwner) ||
		!BindRelationOwner(fixture.binding, fixture.factors[plan.Relation.Axis], foreignOwner) {
		t.Fatal("corresponded relation owners")
	}
	if !fixture.binding.Seal() {
		t.Fatal("corresponded generated binding seal")
	}

	coords := OperandCoords{Mount: fixture.mount, Occurrence: fixture.occurrence}
	resolved, resolvedOK := resolveGeneratedReadCandidate(fixture.binding.state, candidate, plan, coords, 1)
	if !resolvedOK || resolved != 6 {
		t.Fatalf("resolved corresponded row = %d/%t, want the foreign owner's own 6", resolved, resolvedOK)
	}

	reads, _, readsOK := declareGeneratedReadSurfaces(fixture.binding.state, cell, descriptor,
		lawCanonicalRuleAnchor(t), generatedSummaryLawSemantic(t, cell), coords, 1)
	if !readsOK || len(reads) != 1 {
		t.Fatal("a read of a corresponded directory did not issue")
	}
	readFactor := fixture.binding.state.factors[plan.Factor]
	ownRow, ownRowOK := foreignOwner.Project(plan.Relation.Member, plan.Key.Member, 6)
	borrowedRow, borrowedRowOK := foreignOwner.Project(plan.Relation.Member, plan.Key.Member, 1)
	if !ownRowOK || !borrowedRowOK {
		t.Fatal("the foreign owner projects both of its rows")
	}
	want, wantOK := readFactor.schemaFactorExactRead(fixture.binding.state, fixture.binding.state.authority, uint64(ownRow))
	other, otherOK := readFactor.schemaFactorExactRead(fixture.binding.state, fixture.binding.state.authority, uint64(borrowedRow))
	if !wantOK || !otherOK || want.value == other.value {
		t.Fatal("the two rows of the foreign order must mint distinguishable coordinates for this law to say anything")
	}
	if reads[0].value != want.value {
		t.Fatalf("issued read coordinate = %+v, want the foreign owner's own row %+v", reads[0].value, want.value)
	}
	if reads[0].value == other.value {
		t.Fatal("the read was issued at the row the rule's own ordinal names in the foreign order")
	}
}

// TestACorrespondedReadRefusesWhenItsOwnDirectoryHasNoRow keeps the arm total
// at the boundary the resolution moved. The rule's candidate resolves
// perfectly well; the foreign directory answers nothing for this occurrence,
// and the read must refuse rather than fall back to the ordinal the rule
// already has - which would read row 1 of an order that never named it.
func TestACorrespondedReadRefusesWhenItsOwnDirectoryHasNoRow(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newCorrespondedRuleLawFixture(t))
	cell, cellOK := fixture.binding.state.rules[0].(*generatedRuleBindingCell)
	if !cellOK || cell == nil {
		t.Fatal("corresponded generated cell")
	}
	descriptor := cell.generated.program
	plan, planOK := descriptor.ReadAt(0)
	if !planOK {
		t.Fatal("corresponded read")
	}
	candidate := descriptor.CandidateRelation()
	candidateOwner := &generatedBindingLawOwner{
		relation: candidate.Member, candidate: 1, acceptCandidate: true,
		projections: map[[2]uint32]uint32{},
	}
	silentOwner := &generatedBindingLawOwner{
		relation:    plan.Addressing.Member,
		projections: map[[2]uint32]uint32{},
		memberProjections: map[[3]uint32]uint32{
			{plan.Relation.Member, plan.Key.Member, 1}: 0,
		},
	}
	if !BindRelationOwner(fixture.binding, fixture.factors[candidate.Axis], candidateOwner) ||
		!BindRelationOwner(fixture.binding, fixture.factors[plan.Relation.Axis], silentOwner) {
		t.Fatal("corresponded relation owners")
	}
	if !fixture.binding.Seal() {
		t.Fatal("corresponded generated binding seal")
	}
	coords := OperandCoords{Mount: fixture.mount, Occurrence: fixture.occurrence}
	if resolved, ok := resolveGeneratedReadCandidate(fixture.binding.state, candidate, plan, coords, 1); ok {
		t.Fatalf("a directory answering no row resolved %d", resolved)
	}
	if _, _, readsOK := declareGeneratedReadSurfaces(fixture.binding.state, cell, descriptor,
		lawCanonicalRuleAnchor(t), generatedSummaryLawSemantic(t, cell), coords, 1); readsOK {
		t.Fatal("a read issued against a directory that named no row for this occurrence")
	}
}

// TestACorrespondedRowIsResolvedAtTheOccurrenceTheCandidateNames is the
// addressing law for a directory whose rows do not all come from one
// occurrence family.
//
// A correspondence is resolved at the occurrence both directories are
// addressed by, and that is the candidate's own whenever the candidate row IS
// the subject. A row that NAMES a subject sealed elsewhere is not enumerated
// under its own occurrence in the corresponded directory: here the candidate's
// occurrence answers nothing at all, while the occurrence the row names
// answers 7. Asking under the candidate's own would refuse a row that exists,
// and a rule declaring such a read would issue a plan that resolves nothing.
func TestACorrespondedRowIsResolvedAtTheOccurrenceTheCandidateNames(t *testing.T) {
	named := contentID(1)
	own := contentID(2)
	mount := contentID(9)
	owner := directoryOwner{rows: map[directoryAddress][]uint32{
		{relation: 4, mount: mount, occurrence: named}: {7},
	}}
	if resolved, ok := soleDirectoryCandidateAt(owner, 4, mount, named); !ok || resolved != 7 {
		t.Fatalf("resolution at the named occurrence = %d/%t, want 7", resolved, ok)
	}
	if resolved, ok := soleDirectoryCandidate(owner, 4, OperandCoords{Mount: mount, Occurrence: own}); ok {
		t.Fatalf("the candidate's own occurrence resolved %d in a directory that never enumerated it", resolved)
	}
}

// TestANamedAddressIdentityNeverFallsBackToTheCandidateOccurrence completes the
// arm. A read whose declaration names an addressing identity cannot resolve
// without reading that identity off the candidate's row: falling back to the
// candidate's own occurrence IS the defect the declaration exists to prevent,
// and it would be invisible because the wrong row reads perfectly well.
func TestANamedAddressIdentityNeverFallsBackToTheCandidateOccurrence(t *testing.T) {
	candidate := ruleplan.RelationAddr{Axis: 1, Member: 2}
	named := generated.ReadPlan{
		Form:       ruleprogram.Exact,
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 4}, AddressingPresent: true,
		AddressIdentity: ruleplan.ProjectionAddr{Axis: 1, Member: 5}, AddressIdentityPresent: true,
	}
	// A nil binding state reaches no owner, so neither the foreign directory
	// nor the candidate's identity projection can answer.
	if resolved, ok := resolveGeneratedReadCandidate(nil, candidate, named, OperandCoords{}, 3); ok {
		t.Fatalf("a read naming an addressing identity resolved %d with no owner to read it from", resolved)
	}
}
