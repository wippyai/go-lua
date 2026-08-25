package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// The derived exact fold's specimen is two axes, because the form exists for
// the shape one axis cannot state alone: a rule whose candidate and whose
// publication both belong to the axis it writes, reading one exact fact from
// another axis, and folding the two through a judgment that needs both axes'
// sealed cold schemas.
const (
	sitePackage = "example/site"
	wirePackage = "example/wire"
)

func siteType(name string) definition.GoType {
	return definition.GoType{PackagePath: sitePackage, Name: name}
}

func wireType(name string) definition.GoType {
	return definition.GoType{PackagePath: wirePackage, Name: name}
}

func siteMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: sitePackage, Name: name,
		Receiver: siteType(receiver), ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func wireMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: wirePackage, Name: name,
		Receiver: wireType(receiver), ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func siteAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "site"}
}

func wireAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "wire"}
}

func siteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{Axis: siteAxis(), Member: "site/candidates"})
}

func wireProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{Axis: wireAxis(), Member: "wire/candidates"})
}

// siteBase is the written axis: it owns the candidate directory the rule draws
// from and the destination projection its publication is addressed by, so the
// whole exact publication is one axis's own statement.
func siteBase() definition.Definition {
	return definition.Definition{
		Name:       "Site",
		Axis:       "site",
		ImportPath: sitePackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "SiteKeyCarrier",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: siteMethod("KeyIndex", "Schema", true, 0),
		}},
		Signature: definition.Signature{Key: "SiteKeyCarrier", Fact: "SiteFactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "SiteKeyCarrier", Key: "carrier/site/key", Type: siteType("Key")},
			{Name: "SiteFactCarrier", Key: "carrier/site/fact", Type: siteType("Fact")},
			{Name: "SiteMountedCarrier", Key: "carrier/site/mounted", Type: siteType("Mounted")},
			{Name: "WireFactCarrier", Key: "carrier/wire/fact", Type: wireType("Value")},
			{Name: "WireSubjectCarrier", Key: "carrier/wire/coordinate", Type: wireType("Coordinate")},
		},
		Relations: []definition.Relation{
			{
				Name: "Candidates", Key: "site/candidates", Subject: "SiteMountedCarrier",
				CandidateProvider: siteProvider(),
				CandidateResolver: siteMethod("MountedForOccurrence", "Schema", true, 0),
				CandidateOrdinal:  siteMethod("MountedOrdinal", "Schema", true, 0),
				CandidateAt:       siteMethod("MountedAt", "Schema", true, 0),
			},
			{
				Name: "WireBirths", Key: "site/wire-births", Subject: "WireSubjectCarrier",
				CandidateProvider: wireProvider(),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "Coordinate", Key: "site/coordinate", Relation: "Candidates",
				CandidateProvider: siteProvider(), Role: member.Destination, Result: "SiteKeyCarrier",
				Accessor: siteMethod("Root", "Mounted", false, -1),
			},
			{
				Name: "WireBirthDestination", Key: "site/wire-birth-destination", Relation: "WireBirths",
				CandidateProvider: wireProvider(), Role: member.Destination, Result: "SiteKeyCarrier",
				Accessor: wireMethod("Root", "Coordinate", false, -1),
			},
		},
	}
}

// wireBase is the foreign axis the one exact read is taken from. Its own
// candidate directory is addressed by the same occurrence the site directory
// is, which is what lets the engine resolve this read off a site candidate.
func wireBase() definition.Definition {
	return definition.Definition{
		Name:       "Wire",
		Axis:       "wire",
		ImportPath: wirePackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "WireKeyCarrier",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: wireMethod("KeyIndex", "Schema", true, 0),
		}},
		Signature: definition.Signature{Key: "WireKeyCarrier", Fact: "WireValueCarrier"},
		Carriers: []definition.Carrier{
			{Name: "WireKeyCarrier", Key: "carrier/wire/key", Type: wireType("Key")},
			{Name: "WireValueCarrier", Key: "carrier/wire/fact", Type: wireType("Value")},
			{Name: "WireCoordinateCarrier", Key: "carrier/wire/coordinate", Type: wireType("Coordinate")},
		},
		Relations: []definition.Relation{
			{
				Name: "Candidates", Key: "wire/candidates", Subject: "WireCoordinateCarrier",
				CandidateProvider: wireProvider(),
				CandidateResolver: wireMethod("CoordinateForOccurrence", "Schema", true, 0),
				CandidateOrdinal:  wireMethod("CoordinateOrdinal", "Schema", true, 0),
				CandidateAt:       wireMethod("CoordinateAt", "Schema", true, 0),
				Correspondences:   []member.RelationRef{{Axis: siteAxis(), Member: "site/candidates"}},
			},
			{
				Name: "Facts", Key: "wire/facts", Subject: "WireValueCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "WireCoordinateCarrier"}},
				CandidateProvider: wireProvider(),
			},
		},
		Projections: []definition.Projection{{
			Name: "FactKey", Key: "wire/fact-key", Relation: "Facts",
			CandidateProvider: wireProvider(), Role: member.Key, Result: "WireKeyCarrier",
			Accessor: wireMethod("Key", "Coordinate", false, -1),
		}},
	}
}

// siteContribution is the rule's own reducer. Its judgment needs both axes'
// sealed cold schemas, so it declares the state those schemas are sealed into
// once at install; the fold itself remains a call over carriers.
func siteContribution() definition.Contribution {
	return definition.Contribution{
		Axis: "site",
		Rule: "site-derived",
		Reducers: []definition.Reducer{{
			Name: "DerivedReducer", Key: "site/reducer/derived", Candidate: "SiteMountedCarrier",
			Inputs: []definition.ReducerInput{{
				Axis: wireAxis(), Carrier: "WireFactCarrier",
				Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{Axis: siteAxis(), Carrier: "SiteFactCarrier"}},
			Derivation: definition.ReducerDerivation{
				State:      siteType("Judgment"),
				Build:      definition.GoSymbol{PackagePath: sitePackage, Name: "DeriveJudgment", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{siteAxis(), wireAxis()},
			},
			Implementation: siteMethod("Fold", "Judgment", false, 0),
		}},
	}
}

// consumerContribution is the heterogeneous rule beside it: its candidate
// belongs to the read axis, so it publishes into the written axis at a
// destination the emitted installer projects, and its fold rests on carriers
// alone.
func consumerContribution() definition.Contribution {
	return definition.Contribution{
		Axis: "site",
		Rule: "site-consumer",
		Reducers: []definition.Reducer{{
			Name: "ConsumerReducer", Key: "site/reducer/consumer", Candidate: "WireSubjectCarrier",
			Inputs: []definition.ReducerInput{{
				Axis: wireAxis(), Carrier: "WireFactCarrier",
				Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
			}},
			Outputs:        []definition.ReducerOutput{{Axis: siteAxis(), Carrier: "SiteFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: sitePackage, Name: "ConsumerFold", ResultIndex: 0},
		}},
	}
}

func derivedRoster(t testing.TB) definition.Roster {
	t.Helper()
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "site", Name: "site", Base: siteBase(), Contributions: []definition.Contribution{siteContribution(), consumerContribution()}},
		definition.Source{Package: "wire", Name: "wire", Base: wireBase()},
	)
	if !rosterOK {
		t.Fatal("derived exact fold member roster is not admissible")
	}
	return roster
}

// derivedSpec is the declaration the form is named for: candidate on the
// written axis, one exact foreign read, one exact publication at the
// candidate's own destination projection, and no carry at all.
func derivedSpec() rule.Spec {
	return rule.Spec{
		Key:      "site-derived",
		Writes:   "site",
		Owner:    "site",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/site", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/site/derived",
		Roles:    []schema.Key{"semantic/operand/site/derived"},
		Program: program.Program{
			OperandRole: "semantic/operand/site/derived",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: siteAxis(), Member: "site/candidates"}),
			Joins: []program.JoinDecl{{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: wireAxis(), Member: "wire/facts"},
				Key:      member.ProjectionRef{Axis: wireAxis(), Member: "wire/fact-key"},
				Read: program.ReadDecl{
					PointBound: program.PointBound, Input: 0,
					Axis: program.AxisRef(wireAxis()), Form: program.Exact,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
					},
				},
			}},
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: siteAxis(), Member: "site/reducer/derived"},
				Inputs:  []program.JoinRef{0},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: siteAxis(), Key: "site/facts"},
					Destination: member.ProjectionRef{Axis: siteAxis(), Member: "site/coordinate"},
					Mode:        program.ModeExact, ValueSlot: 0,
				}},
			},
		},
	}
}

func derivedTarget() Target {
	return Target{PackagePath: "example/rule/derived", PackageName: "derived", Spec: derivedSpec()}
}

func renderDerived(t testing.TB, target Target) string {
	t.Helper()
	source, err := Render(target, derivedRoster(t))
	if err != nil {
		t.Fatalf("the derived exact fold declaration did not emit: %v", err)
	}
	return string(source)
}

// TestADerivedExactFoldPublishesAtItsOwnCandidatesCoordinate is the form's
// defining law. A rule whose candidate directory belongs to the axis it writes
// publishes at the destination that candidate projects, and the emitted family
// seals that write through the plane's exact write. Refusing it as "outside
// the foreign consumer normal form" was the emitter holding one consumer shape
// as if it were the only exact publication there is.
func TestADerivedExactFoldPublishesAtItsOwnCandidatesCoordinate(t *testing.T) {
	source := renderDerived(t, derivedTarget())
	if !strings.Contains(source, "execution.FormExact") {
		t.Fatalf("the emitted family does not claim the exact execution form:\n%s", source)
	}
	if !strings.Contains(source, "plane.ExactWrite(planRow.Target, uint16(output.Slot))") {
		t.Fatalf("the emitted family does not seal an exact write at the row's own target:\n%s", source)
	}
}

// TestADerivedExactFoldProjectsNoForeignDestination states which owner answers
// the destination. The candidate and the publication belong to one axis, so
// that axis's own relation owner projects the row; the construction-time
// projector exists for the heterogeneous rule that has no such owner, and
// emitting one here would be a second answer to a question already answered.
func TestADerivedExactFoldProjectsNoForeignDestination(t *testing.T) {
	source := renderDerived(t, derivedTarget())
	if strings.Contains(source, "ProjectExactDestination") {
		t.Fatalf("the emitted installer projects a destination its candidate's own owner already answers:\n%s", source)
	}
}

// TestADerivedExactFoldFencesTheAbsenceOfACarry holds the emitted fence to the
// declaration. This rule declares no carry, and its engine seam admits none,
// so a plan row that arrived carrying one is a plan that no longer matches the
// declaration this family was emitted from.
func TestADerivedExactFoldFencesTheAbsenceOfACarry(t *testing.T) {
	source := renderDerived(t, derivedTarget())
	if !strings.Contains(source, "carryMode, carryPresent := planRow.Rule.CarryMode()") {
		t.Fatalf("the emitted installer states nothing about the row's carry:\n%s", source)
	}
	if !strings.Contains(source, "if carryPresent || carryMode != 0 {") {
		t.Fatalf("the emitted installer does not refuse a carried row:\n%s", source)
	}
}

// TestADerivedFoldSealsItsJudgmentOnceFromTheDeclaredStaticAxes is the sealed
// state law. A judgment that needs its axes' cold schemas gets them as the
// family's sealed state, built once when the family is installed - never as a
// fold parameter, which would grow the call shape with plumbing, and never per
// invocation, which would allocate on a warm path.
func TestADerivedFoldSealsItsJudgmentOnceFromTheDeclaredStaticAxes(t *testing.T) {
	source := renderDerived(t, derivedTarget())
	call, found := callArguments(source, "DeriveJudgment")
	if !found {
		t.Fatalf("the emitted installer never seals the declared fold state:\n%s", source)
	}
	if len(call) != 2 || call[0] != "install.siteSchema" || call[1] != "install.wireSchema" {
		t.Fatalf("the fold state is sealed from %v, the declaration names the site and wire schemas", call)
	}
	family, familyFound := typeBody(source, "sealedFamily")
	if !familyFound || !strings.Contains(family, "state site.Judgment") {
		t.Fatalf("the emitted family does not hold the sealed judgment:\n%s", family)
	}
	if occurrences := strings.Count(source, "DeriveJudgment("); occurrences != 1 {
		t.Fatalf("the emitted family seals its judgment %d times; it is sealed once, at install", occurrences)
	}
}

// TestADerivedFoldCallCarriesOnlyDeclaredCarriers is the reducer contract at
// this form. Sealed state reaches the judgment as its receiver, so the call
// itself still passes carrier values and nothing else.
func TestADerivedFoldCallCarriesOnlyDeclaredCarriers(t *testing.T) {
	source := renderDerived(t, derivedTarget())
	if !strings.Contains(source, "fold.state.Fold(fold.candidate, cell)") {
		t.Fatalf("the emitted fold call is not the declared judgment over its carriers:\n%s", source)
	}
}

// consumerSpec is the heterogeneous declaration: candidate on the read axis,
// publication into the written axis at a destination the installer projects,
// and the identity carry that retains the written Factor at a coordinate
// belonging to no directory of its own.
func consumerSpec() rule.Spec {
	spec := derivedSpec()
	spec.Key = "site-consumer"
	spec.Semantic = "semantic/rule/site/consumer"
	spec.Roles = []schema.Key{"semantic/operand/site/consumer"}
	spec.Program.OperandRole = "semantic/operand/site/consumer"
	spec.Program.Candidate = member.AxisRelationCandidate(member.RelationRef{Axis: wireAxis(), Member: "wire/candidates"})
	spec.Program.Fold.Reducer = member.ReducerRef{Axis: siteAxis(), Member: "site/reducer/consumer"}
	spec.Program.Fold.Outputs[0].Destination = member.ProjectionRef{Axis: siteAxis(), Member: "site/wire-birth-destination"}
	spec.Program.Carry = &program.CarryDecl{Input: 0, Mode: program.CarryIdentity}
	return spec
}

func consumerTarget() Target {
	return Target{PackagePath: "example/rule/consumer", PackageName: "consumer", Spec: consumerSpec()}
}

// TestAHeterogeneousExactFoldStillRequiresItsIdentityCarry is the law the
// carry-form specimen used to state through its own declaration. It is
// restated here, against the shape it actually governs: a rule whose candidate
// belongs to another axis publishes at a coordinate no directory of the
// written axis enumerates, and the written Factor reaches that coordinate only
// through the declared identity carry. Admitting one without it would drop
// that Factor's carry closure at exactly the rows this rule mints.
func TestAHeterogeneousExactFoldStillRequiresItsIdentityCarry(t *testing.T) {
	if _, err := Render(consumerTarget(), derivedRoster(t)); err != nil {
		t.Fatalf("the heterogeneous consumer declaration did not emit: %v", err)
	}
	spec := consumerSpec()
	spec.Program.Carry = nil
	target := consumerTarget()
	target.Spec = spec
	source, err := Render(target, derivedRoster(t))
	if err == nil {
		t.Fatalf("a heterogeneous exact fold emitted without its identity carry:\n%s", source)
	}
	refusal, named := err.(Unexpressible)
	if !named || !strings.Contains(refusal.Clause, "an authored exact output with no identity carry") {
		t.Fatalf("refusal does not name the missing identity carry: %v", err)
	}
}

// TestAHeterogeneousExactFoldProjectsItsOwnDestination is the other half of
// that split. No owner holds both the read axis's candidate directory and the
// written axis's key, so the installer - the one sealed object holding both
// schemas - answers the destination itself.
func TestAHeterogeneousExactFoldProjectsItsOwnDestination(t *testing.T) {
	source, err := Render(consumerTarget(), derivedRoster(t))
	if err != nil {
		t.Fatalf("the heterogeneous consumer declaration did not emit: %v", err)
	}
	if !strings.Contains(string(source), "func (install familyInstaller) ProjectExactDestination(ordinal uint32) (uint64, bool) {") {
		t.Fatalf("the emitted installer answers no construction-time destination:\n%s", source)
	}
}

// TestAnExactFoldFenceRestatesTheDeclaredCarry states what the emitted fence
// is for. It is not a requirement the emitter holds over declarations - the
// carry disposition is the declaration's own - it is the restatement that
// catches a plan row whose descriptor no longer agrees with the declaration
// the family was emitted from. A row carrying nothing and a row carrying the
// written Factor are different plans, and each family refuses the other.
func TestAnExactFoldFenceRestatesTheDeclaredCarry(t *testing.T) {
	uncarried := renderDerived(t, derivedTarget())
	if !strings.Contains(uncarried, "if carryPresent || carryMode != 0 {") {
		t.Fatalf("the uncarried family does not refuse a carried row:\n%s", uncarried)
	}
	if strings.Contains(uncarried, "CarryIdentity") {
		t.Fatalf("the uncarried family fences a carry its declaration never names:\n%s", uncarried)
	}
	carried, err := Render(consumerTarget(), derivedRoster(t))
	if err != nil {
		t.Fatalf("the heterogeneous consumer declaration did not emit: %v", err)
	}
	if !strings.Contains(string(carried), "if !carryPresent || carryMode != program.CarryIdentity {") {
		t.Fatalf("the carried family does not hold its row to the declared identity carry:\n%s", carried)
	}
}

// TestADerivedFoldStateIsRefusedWhenItsAxesAreNotTheDeclarationsOwn keeps the
// sealed state inside the declaration. A fold state built from an axis the
// rule never names would reach a schema the declaration does not admit it to.
func TestADerivedFoldStateIsRefusedWhenItsAxesAreNotTheDeclarationsOwn(t *testing.T) {
	contribution := siteContribution()
	contribution.Reducers[0].Derivation.StaticAxes = []schema.EntryReference{
		{Surface: schema.SurfaceKindAxis, Key: "absent"},
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "site", Name: "site", Base: siteBase(), Contributions: []definition.Contribution{contribution}},
		definition.Source{Package: "wire", Name: "wire", Base: wireBase()},
	)
	if !rosterOK {
		t.Fatal("the probe roster is not admissible")
	}
	source, err := Render(derivedTarget(), roster)
	if err == nil {
		t.Fatalf("a fold state sealed against an unrostered axis emitted:\n%s", source)
	}
	if _, named := err.(Unexpressible); !named {
		t.Fatalf("refusal is not named as unexpressible: %v", err)
	}
}

// TestADerivedExactFoldIsAFunctionOfItsDeclaration is the freshness premise at
// this form, for the same reason the carry form has one.
func TestADerivedExactFoldIsAFunctionOfItsDeclaration(t *testing.T) {
	first := renderDerived(t, derivedTarget())
	for attempt := 0; attempt < 8; attempt++ {
		if again := renderDerived(t, derivedTarget()); again != first {
			t.Fatal("two renders of one derived exact fold declaration produced different families")
		}
	}
}
