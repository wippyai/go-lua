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
			{Name: "SiteRouteCarrier", Key: "carrier/site/route", Type: siteType("Route")},
			{Name: "SiteTagCarrier", Key: "carrier/site/route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Enumerations: []definition.Enumeration{
			{
				Name: "Alternatives", Over: "WireFactCarrier", Item: "WireSubjectCarrier",
				Count: definition.GoSymbol{PackagePath: sitePackage, Name: "AlternativeCount", ResultIndex: 0},
				At:    definition.GoSymbol{PackagePath: sitePackage, Name: "AlternativeAt", ResultIndex: 0},
			},
			{
				// Over is empty: the owner's whole directory, which is what a
				// widened answer is read out of.
				Name: "Directory", Item: "WireSubjectCarrier",
				Count: definition.GoSymbol{PackagePath: sitePackage, Name: "DirectoryCount", ResultIndex: 0},
				At:    definition.GoSymbol{PackagePath: sitePackage, Name: "DirectoryAt", ResultIndex: 0},
			},
			{
				// The same directory read as the owner's own keys: a widened
				// answer whose rows are not the items the source chain yields.
				Name: "KeyDirectory", Item: "SiteKeyCarrier",
				Count: definition.GoSymbol{PackagePath: sitePackage, Name: "KeyDirectoryCount", ResultIndex: 0},
				At:    definition.GoSymbol{PackagePath: sitePackage, Name: "KeyDirectoryAt", ResultIndex: 0},
			},
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
			{
				Name: "BodyRoutes", Key: "site/body-routes", Subject: "SiteRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "SiteMountedCarrier"},
					{Carrier: "WireFactCarrier"},
				},
				CandidateProvider: siteProvider(),
				Derivation:        declaredSiteDerivation(),
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
			{
				Name: "BodyRouteKey", Key: "site/body-route-key", Relation: "BodyRoutes",
				CandidateProvider: siteProvider(), Role: member.Key, Result: "SiteKeyCarrier",
				Accessor: siteMethod("Coordinate", "Route", false, -1),
			},
			{
				Name: "BodyRouteTag", Key: "site/body-route-tag", Relation: "BodyRoutes",
				CandidateProvider: siteProvider(), Role: member.Predicate, Result: "SiteTagCarrier",
				Accessor: siteMethod("Predicate", "Route", false, -1),
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

// selectionContribution is the fold of a conclusion over a derived selection:
// one many-valued selected input, handed the cells the member set observed.
func selectionContribution() definition.Contribution {
	return definition.Contribution{
		Axis: "site",
		Rule: "site-selection",
		Reducers: []definition.Reducer{{
			Name: "SelectionReducer", Key: "site/reducer/selection", Candidate: "SiteMountedCarrier",
			Inputs: []definition.ReducerInput{{
				Axis: siteAxis(), Carrier: "SiteFactCarrier",
				Form: member.ReadFormSelected, Multiplicity: member.MultiplicityMany,
				Tag: "SiteTagCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: siteAxis(), Carrier: "SiteFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: sitePackage, Name: "SelectionFold", ResultIndex: 0},
		}},
	}
}

func derivedRoster(t testing.TB) definition.Roster {
	t.Helper()
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "site", Name: "site", Base: siteBase(), Contributions: []definition.Contribution{siteContribution(), consumerContribution(), selectionContribution()}},
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
// itself still passes carrier values and nothing else. Each cell is named by
// the declared read that observed it, so one read's value can never reach the
// judgment under another read's name.
func TestADerivedFoldCallCarriesOnlyDeclaredCarriers(t *testing.T) {
	source := renderDerived(t, derivedTarget())
	if !strings.Contains(source, "fold.state.Fold(fold.candidate, cell0)") {
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

// selectionSpec is the derived exact fold with a second, SELECTED join added:
// the shape of a conclusion over a selection. The relation it names carries no
// derivation, which is one of the things the laws below refuse - a specimen
// that declared one would need a scheduled-death row of its own, and what
// these laws measure is the classification rather than the derivation.
func selectionSpec() rule.Spec {
	spec := derivedSpec()
	spec.Program.Joins = append(spec.Program.Joins, program.JoinDecl{
		Sources:  []program.SourceRef{program.CandidateSource(), program.PriorSource(0)},
		Relation: member.RelationRef{Axis: siteAxis(), Member: "site/candidates"},
		Key:      member.ProjectionRef{Axis: siteAxis(), Member: "site/coordinate"},
		Read: program.ReadDecl{
			PointBound: program.PointBound, Input: 0,
			Axis: program.AxisRef(siteAxis()), Form: program.Selected,
			Contract: program.ReadContract{
				Order: program.OrderByTag, Sparse: program.SparseExplicit,
				OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
				DenominatorRef: program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: "coordinates/site"},
			},
		},
	})
	spec.Program.Fold.Inputs = []program.JoinRef{1}
	return spec
}

// TestAnExactConclusionOverASelectionIsRefusedWhenItsMembersAreNotDerived
// fences the classification of the new shape. Each refusal names the clause
// that has no emitted form rather than falling back to the all-exact product,
// which would silently drop the selection.
func TestAnExactConclusionOverASelectionIsRefusedWhenItsMembersAreNotDerived(t *testing.T) {
	for _, probe := range []struct {
		name   string
		mutate func(*rule.Spec)
		clause string
	}{
		{
			name:   "the members come from no declared relation derivation",
			mutate: func(spec *rule.Spec) {},
			clause: "an exact conclusion over a selection with no relation derivation",
		},
		{
			name: "two selections have no declared correlation between them",
			mutate: func(spec *rule.Spec) {
				spec.Program.Joins = append(spec.Program.Joins, spec.Program.Joins[1])
				spec.Program.Fold.Inputs = []program.JoinRef{1}
			},
			clause: "an exact conclusion over 1 exact and 2 selected reads",
		},
		{
			name: "the selection observes a Factor the rule does not write",
			mutate: func(spec *rule.Spec) {
				spec.Program.Joins[1].Relation = member.RelationRef{Axis: wireAxis(), Member: "wire/facts"}
				spec.Program.Joins[1].Key = member.ProjectionRef{Axis: wireAxis(), Member: "wire/fact-key"}
				spec.Program.Joins[1].Read.Axis = program.AxisRef(wireAxis())
			},
			clause: "an exact conclusion over a foreign selection",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			spec := selectionSpec()
			probe.mutate(&spec)
			target := derivedTarget()
			target.Spec = spec
			source, err := Render(target, derivedRoster(t))
			if err == nil {
				t.Fatalf("an unexpressible conclusion emitted a family:\n%s", source)
			}
			refusal, named := err.(Unexpressible)
			if !named || !strings.Contains(refusal.Clause, probe.clause) {
				t.Fatalf("refusal is %v, want it to name %q", err, probe.clause)
			}
		})
	}
}

// declaredSiteDerivation is the site specimen's member set stated in the
// DECLARED form: what it reads its items out of, the one judgment that turns
// an item into a row, and the endpoint past which the answer is the owner's
// whole directory.
func declaredSiteDerivation() definition.RelationDerivation {
	return definition.RelationDerivation{
		StaticAxes:  []schema.EntryReference{siteAxis(), wireAxis()},
		Source:      []definition.EnumerationRef{{Axis: siteAxis(), Name: "Alternatives"}},
		Resolve:     definition.GoSymbol{PackagePath: sitePackage, Name: "ResolveRoute", ResultIndex: 0},
		InlineWidth: 4,
		Widen: definition.DerivationWiden{
			Predicate: definition.GoSymbol{PackagePath: sitePackage, Name: "IsTop", ResultIndex: 0},
			Source:    []definition.EnumerationRef{{Axis: siteAxis(), Name: "Directory"}},
		},
	}
}

// derivedSelectionSpec is the conclusion-over-a-selection declaration whose
// member set is DERIVED by the declared operators rather than by an authored
// Build. It is the emitter's whole consumer for the vocabulary.
func derivedSelectionSpec() rule.Spec {
	spec := selectionSpec()
	spec.Program.Joins[1].Relation = member.RelationRef{Axis: siteAxis(), Member: "site/body-routes"}
	spec.Program.Joins[1].Key = member.ProjectionRef{Axis: siteAxis(), Member: "site/body-route-key"}
	spec.Program.Joins[1].Predicate = member.ProjectionRef{Axis: siteAxis(), Member: "site/body-route-tag"}
	spec.Program.Joins[1].Sources = []program.SourceRef{program.CandidateSource(), program.PriorSource(0)}
	spec.Program.Fold.Reducer = member.ReducerRef{Axis: siteAxis(), Member: "site/reducer/selection"}
	return spec
}

func renderDerivedSelection(t testing.TB, spec rule.Spec) (string, error) {
	t.Helper()
	target := derivedTarget()
	target.Spec = spec
	source, err := Render(target, derivedRoster(t))
	return string(source), err
}

// TestAGeneratedMemberSetIsOrderedByTheKeyItsRelationDeclares is the order law,
// and the one the whole arm exists to make unbreakable.
//
// The engine canonicalizes a selection by the coordinate its cells are read
// at, so a member set has exactly one admissible order: ascending by the
// relation's OWN declared Key projection, normalized through the axis that
// numbers those coordinates. Every authored Build wrote that by hand. The
// generated one must write the same thing, and it must write it on BOTH paths
// - the ordinary answer and the widened one - because a widened set that came
// back in directory order would be just as wrong.
//
// The order is kept while the set is built. A member is normalized once, where
// it is resolved, and placed where it belongs; there is no sort afterwards to
// get wrong and no second normalization to disagree with the first.
func TestAGeneratedMemberSetIsOrderedByTheKeyItsRelationDeclares(t *testing.T) {
	source, err := renderDerivedSelection(t, derivedSelectionSpec())
	if err != nil {
		t.Fatalf("a declared derivation did not emit: %v", err)
	}
	build, buildFound := functionBody(source, "deriveDerived1Rows")
	if !buildFound {
		t.Fatalf("the emitted construction has no Build:\n%s", source)
	}
	// The declared Key projection, and nothing else, decides the order.
	if !strings.Contains(build, ".Coordinate()") {
		t.Fatalf("the construction does not read the relation's declared Key projection:\n%s", build)
	}
	// Normalized through the axis that numbers the coordinates, so the order is
	// the engine's own rather than whatever the carrier compares as.
	if !strings.Contains(build, ".KeyIndex(") {
		t.Fatalf("the construction does not normalize the key through its axis:\n%s", build)
	}
	if occurrences := strings.Count(build, "insertDerived1Row("); occurrences != 2 {
		t.Fatalf("the Build places its members %d times; both the ordinary and the widened path are ordered:\n%s", occurrences, build)
	}
	if strings.Contains(source, "SortFunc") {
		t.Fatalf("the construction sorts its answer; the order is kept while the set is built:\n%s", source)
	}
	insert, insertFound := functionBody(source, "insertDerived1Row")
	if !insertFound {
		t.Fatalf("the emitted construction has no canonical placement:\n%s", source)
	}
	if !strings.Contains(insert, "current.dense > dense") {
		t.Fatalf("placement does not scan for the position the coordinate belongs at:\n%s", insert)
	}
}

// TestAGeneratedMemberSetIsHeldByValue is the allocation law of the emitted
// shape. A derived set is a bounded inline prefix held BY VALUE, an explicit
// spill beyond it, and a count - the shape every authored Build converged on
// independently - so the ordinary answer never allocates a slice just to be
// returned, and the width past which it does is the one the relation declares.
func TestAGeneratedMemberSetIsHeldByValue(t *testing.T) {
	source, err := renderDerivedSelection(t, derivedSelectionSpec())
	if err != nil {
		t.Fatalf("a declared derivation did not emit: %v", err)
	}
	if !strings.Contains(source, "const derived1InlineWidth = 4") {
		t.Fatalf("the emitted set does not hold the width its relation declares:\n%s", source)
	}
	if !strings.Contains(source, "inline [derived1InlineWidth]derived1Member") {
		t.Fatalf("the emitted set has no inline prefix held by value:\n%s", source)
	}
	if !strings.Contains(source, "spill  []derived1Member") {
		t.Fatalf("the emitted set has no explicit spill:\n%s", source)
	}
	build, _ := functionBody(source, "deriveDerived1Rows")
	if strings.Contains(build, "rows []") {
		t.Fatalf("the Build is handed a row buffer; the set is its own answer and is returned by value:\n%s", build)
	}
	if strings.Contains(source, "append(built.rows") || strings.Contains(source, "state.rows") {
		t.Fatalf("the emitted set still answers through a slice of rows:\n%s", source)
	}
}

// TestAGeneratedMemberSetHoldsOneMemberPerAddress is the other half of the
// order. A member is reached at a coordinate AND at the tag its predicate
// answers: two items resolving to one address are one member named twice, and
// two items on one coordinate under different tags are two members where a
// selection carries one cell.
func TestAGeneratedMemberSetHoldsOneMemberPerAddress(t *testing.T) {
	source, err := renderDerivedSelection(t, derivedSelectionSpec())
	if err != nil {
		t.Fatalf("a declared derivation did not emit: %v", err)
	}
	insert, found := functionBody(source, "insertDerived1Row")
	if !found {
		t.Fatalf("the emitted construction has no canonical placement:\n%s", source)
	}
	if !strings.Contains(insert, "current.dense == dense") || !strings.Contains(insert, "current.tag != tag") {
		t.Fatalf("placement does not tell an alias of one member from two members on one coordinate:\n%s", insert)
	}
}

// TestAGeneratedMemberSetOwesTheLawsOfItsOwnConstruction states where the
// runtime half of the two laws above lives. Ordering and allocation are
// properties of the emitted code and of no fact any domain supplies, so the
// generator that writes the construction writes their laws beside it rather
// than leaving each migrated owner to restate them by hand.
func TestAGeneratedMemberSetOwesTheLawsOfItsOwnConstruction(t *testing.T) {
	target := derivedTarget()
	target.Spec = derivedSelectionSpec()
	source, err := RenderLaw(target, derivedRoster(t))
	if err != nil {
		t.Fatalf("a declared derivation emitted no law suite: %v", err)
	}
	suite := string(source)
	for _, law := range []string{
		"func TestDerived1RowsAnswersItsMembersInCoordinateOrder(",
		"func TestDerived1RowsHoldsOneMemberPerAddress(",
		"func TestDerived1RowsFillsItsDeclaredWidthWithoutAllocating(",
	} {
		if !strings.Contains(suite, law) {
			t.Fatalf("the emitted law suite is missing %q:\n%s", law, suite)
		}
	}
	if !strings.Contains(suite, "testing.AllocsPerRun") {
		t.Fatalf("the allocation law does not measure allocations:\n%s", suite)
	}
}

// TestAFamilyWithNoDerivedMemberSetOwesNoLawSuite keeps the emitted suite to
// what it can actually state. A family whose relations are authored has no
// generated construction, so there is nothing here for a law to hold, and an
// empty suite would be a file that only says the generator ran.
func TestAFamilyWithNoDerivedMemberSetOwesNoLawSuite(t *testing.T) {
	source, err := RenderLaw(derivedTarget(), derivedRoster(t))
	if err != nil {
		t.Fatalf("a family with no derived member set refused to answer: %v", err)
	}
	if source != nil {
		t.Fatalf("a family with no generated construction was given a law suite:\n%s", source)
	}
}

// TestADeclaredDerivationIsRefusedWhenItCannotOrderItsOwnRows is the refusal
// the order law rests on.
//
// The coordinates are numbered by one axis, and the generated Build is a free
// function - so that axis's schema reaches it only as a static axis it
// declared. A derivation that does not name it cannot normalize a key, and the
// only orders left would be the order items happened to come out in. That is
// refused by name here rather than emitted as a silent declaration order.
func TestADeclaredDerivationIsRefusedWhenItCannotOrderItsOwnRows(t *testing.T) {
	spec := derivedSelectionSpec()
	roster := derivedRosterWithDerivation(t, func(derivation *definition.RelationDerivation) {
		derivation.StaticAxes = []schema.EntryReference{wireAxis()}
	})
	target := derivedTarget()
	target.Spec = spec
	source, err := Render(target, roster)
	if err == nil {
		t.Fatalf("a derivation that cannot normalize its own key emitted a family:\n%s", source)
	}
	refusal, named := err.(Unexpressible)
	if !named || !strings.Contains(refusal.Clause, "a derived member set whose ordering axis it does not name") {
		t.Fatalf("refusal is %v, want it to name the missing ordering axis", err)
	}
}

// TestADeclaredDerivationReadsSomethingItWasGiven fences the source chain at
// its outer end. The first enumeration is read out of one of the relation's
// own declared inputs; one reading anything else would be handed a value the
// invocation never gives it.
func TestADeclaredDerivationReadsSomethingItWasGiven(t *testing.T) {
	roster := derivedRosterWithDerivation(t, func(derivation *definition.RelationDerivation) {
		derivation.Source = []definition.EnumerationRef{{Axis: siteAxis(), Name: "Directory"}}
	})
	target := derivedTarget()
	target.Spec = derivedSelectionSpec()
	source, err := Render(target, roster)
	if err == nil {
		t.Fatalf("a derivation reading something it was never given emitted a family:\n%s", source)
	}
	refusal, named := err.(Unexpressible)
	if !named || !strings.Contains(refusal.Clause, "a derived member set read out of a value its relation is not given") {
		t.Fatalf("refusal is %v, want it to name the unreachable source", err)
	}
}

// functionBody answers the body of the named top-level function in source.
func functionBody(source, name string) (string, bool) {
	marker := "\nfunc " + name + "("
	start := strings.Index(source, marker)
	if start < 0 {
		return "", false
	}
	rest := source[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// derivedRosterWithDerivation admits the site specimen with its declared
// derivation amended, so a law can state what an ill-formed one is refused for
// without restating the whole axis around it.
func derivedRosterWithDerivation(t testing.TB, amend func(*definition.RelationDerivation)) definition.Roster {
	t.Helper()
	base := siteBase()
	for index := range base.Relations {
		if base.Relations[index].Name != "BodyRoutes" {
			continue
		}
		derivation := declaredSiteDerivation()
		amend(&derivation)
		base.Relations[index].Derivation = derivation
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "site", Name: "site", Base: base, Contributions: []definition.Contribution{siteContribution(), consumerContribution(), selectionContribution()}},
		definition.Source{Package: "wire", Name: "wire", Base: wireBase()},
	)
	if !rosterOK {
		t.Fatal("the amended site roster is not admissible")
	}
	return roster
}

// derivedRosterWithEnumeration admits the site specimen with one of its
// declared enumerations amended, so a law can state what the SOURCE's own
// promises decide about the construction read out of it.
func derivedRosterWithEnumeration(t testing.TB, name string, amend func(*definition.Enumeration)) definition.Roster {
	t.Helper()
	base := siteBase()
	for index := range base.Enumerations {
		if base.Enumerations[index].Name == name {
			amend(&base.Enumerations[index])
		}
	}
	for index := range base.Relations {
		if base.Relations[index].Name == "BodyRoutes" {
			base.Relations[index].Derivation = declaredSiteDerivation()
		}
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "site", Name: "site", Base: base, Contributions: []definition.Contribution{siteContribution(), consumerContribution(), selectionContribution()}},
		definition.Source{Package: "wire", Name: "wire", Base: wireBase()},
	)
	if !rosterOK {
		t.Fatal("the amended site roster is not admissible")
	}
	return roster
}

// renderDerivedSelectionWith renders the derived selection against an amended
// roster.
func renderDerivedSelectionWith(t testing.TB, roster definition.Roster) string {
	t.Helper()
	target := derivedTarget()
	target.Spec = derivedSelectionSpec()
	source, err := Render(target, roster)
	if err != nil {
		t.Fatalf("a declared derivation did not emit: %v", err)
	}
	return string(source)
}

// TestAWidenedSetIsReadWhereItLiesWhenItsDirectoryYieldsInTheSameOrder is the
// widening half of the allocation bar.
//
// A set that reached its lattice endpoint is the owner's whole directory, and
// materializing one member per coordinate of a whole directory on a solve path
// is the copy the authored derivations were written to avoid: store's own
// widened plan "deliberately does not retain a copied allocation-root
// catalogue". When the directory hands its rows back in the order this
// relation is ordered by, there is nothing to place - the answer IS the
// directory - so the set records that it widened, keeps the inputs a member is
// resolved from, and answers each one on demand.
func TestAWidenedSetIsReadWhereItLiesWhenItsDirectoryYieldsInTheSameOrder(t *testing.T) {
	source := renderDerivedSelectionWith(t, derivedRosterWithEnumeration(t, "Directory", func(enumeration *definition.Enumeration) {
		enumeration.Order = siteAxis()
	}))
	build, found := functionBody(source, "deriveDerived1Rows")
	if !found {
		t.Fatalf("the emitted construction has no Build:\n%s", source)
	}
	if occurrences := strings.Count(build, "insertDerived1Row("); occurrences != 1 {
		t.Fatalf("the Build places members %d times; the widened arm copies nothing:\n%s", occurrences, build)
	}
	if !strings.Contains(build, "built.widened = true") {
		t.Fatalf("the widened arm does not record that it widened:\n%s", build)
	}
	if _, lazy := functionBody(source, "derived1WidenedAt"); !lazy {
		t.Fatalf("the widened answer has no on-demand accessor:\n%s", source)
	}
}

// TestALazilyWidenedSetIsHeldToTheOrderItsDirectoryPromises is what laziness
// costs. Reading a directory where it lies moves the ordering guarantee out of
// the construction and into the owner's promise, so the construction checks
// the promise it is resting on: the coordinates the directory yields are
// strictly ascending or the answer is refused. A directory that quietly stops
// being ordered can then never be answered in the wrong order.
func TestALazilyWidenedSetIsHeldToTheOrderItsDirectoryPromises(t *testing.T) {
	source := renderDerivedSelectionWith(t, derivedRosterWithEnumeration(t, "Directory", func(enumeration *definition.Enumeration) {
		enumeration.Order = siteAxis()
	}))
	build, _ := functionBody(source, "deriveDerived1Rows")
	if !strings.Contains(build, ".KeyIndex(") {
		t.Fatalf("the widened arm never normalizes a coordinate, so it cannot hold the directory to its order:\n%s", build)
	}
	if !strings.Contains(build, "dense <= widenPrevious") {
		t.Fatalf("the widened arm admits a directory that yields out of order:\n%s", build)
	}
}

// TestAWidenedSetIsPlacedWhenItsDirectoryIsNumberedByAnotherAxis is the other
// side of the same choice, and the one the migrated relation takes: Call's
// body directory yields in CALL's order while an Effect body route is ordered
// by Effect's, so two numberings meet and the answer has to be placed member
// by member exactly as the ordinary one is.
func TestAWidenedSetIsPlacedWhenItsDirectoryIsNumberedByAnotherAxis(t *testing.T) {
	source := renderDerivedSelectionWith(t, derivedRosterWithEnumeration(t, "Directory", func(enumeration *definition.Enumeration) {
		enumeration.Order = wireAxis()
	}))
	build, _ := functionBody(source, "deriveDerived1Rows")
	if occurrences := strings.Count(build, "insertDerived1Row("); occurrences != 2 {
		t.Fatalf("the Build places members %d times; a directory in another axis's order is placed:\n%s", occurrences, build)
	}
	if strings.Contains(build, "built.widened = true") {
		t.Fatalf("a directory in another axis's order was read where it lies:\n%s", build)
	}
}

// TestASourceMayBeFencedByItsOwnersSchema states the third shape an owner's
// accessor takes, and the reason the first two are not enough.
//
// How a value decomposes is its owner's answer, and an owner whose reads are
// fenced by its own schema answers with a method ON that schema taking the
// value - that is the normal case, not the exception: the fence is what makes
// the answer trustworthy. The declaration already says which shape a symbol
// is, because it says what the symbol's receiver is, so nothing here is
// configured.
func TestASourceMayBeFencedByItsOwnersSchema(t *testing.T) {
	source := renderDerivedSelectionWith(t, derivedRosterWithEnumeration(t, "Alternatives", func(enumeration *definition.Enumeration) {
		enumeration.Count = siteMethod("AlternativeCount", "Schema", true, 0)
		enumeration.At = siteMethod("AlternativeAt", "Schema", true, 0)
	}))
	build, found := functionBody(source, "deriveDerived1Rows")
	if !found {
		t.Fatalf("the emitted construction has no Build:\n%s", source)
	}
	if !strings.Contains(build, "siteSchema.AlternativeCount(given1)") {
		t.Fatalf("a source fenced by its owner's schema is not read through it:\n%s", build)
	}
	if !strings.Contains(build, "siteSchema.AlternativeAt(given1, cursor0)") {
		t.Fatalf("a fenced source's accessor is not handed the value it reads:\n%s", build)
	}
}

// TestAFencedSourceIsRefusedWhenItsDerivationDoesNotNameTheOwner is what that
// shape rests on. The generated Build is a free function, so an axis's schema
// reaches it only as a static axis the derivation declared; a source read
// through a schema the invocation is never handed has nothing to be read out
// of, and that is refused by name rather than emitted as a call to a variable
// that does not exist.
func TestAFencedSourceIsRefusedWhenItsDerivationDoesNotNameTheOwner(t *testing.T) {
	base := siteBase()
	for index := range base.Enumerations {
		if base.Enumerations[index].Name == "Alternatives" {
			base.Enumerations[index].Count = siteMethod("AlternativeCount", "Schema", true, 0)
			base.Enumerations[index].At = siteMethod("AlternativeAt", "Schema", true, 0)
		}
	}
	for index := range base.Relations {
		if base.Relations[index].Name != "BodyRoutes" {
			continue
		}
		derivation := declaredSiteDerivation()
		derivation.StaticAxes = []schema.EntryReference{wireAxis(), siteAxis()}
		base.Relations[index].Derivation = derivation
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "site", Name: "site", Base: base, Contributions: []definition.Contribution{siteContribution(), consumerContribution(), selectionContribution()}},
		definition.Source{Package: "wire", Name: "wire", Base: wireBase()},
	)
	if !rosterOK {
		t.Fatal("the amended site roster is not admissible")
	}
	target := derivedTarget()
	target.Spec = derivedSelectionSpec()
	if _, err := Render(target, roster); err != nil {
		t.Fatalf("a derivation naming its owner refused to emit: %v", err)
	}
}

// TestAWidenEndpointThatYieldsAnotherItemStatesItsOwnJudgment is the second
// half of the same reading: a judgment belongs to a source CHAIN, not to the
// derivation. The widened chain enumerates the owner's directory, whose rows
// are not necessarily the same thing as the items of the value that reached
// the endpoint, and one symbol cannot be handed both.
func TestAWidenEndpointThatYieldsAnotherItemStatesItsOwnJudgment(t *testing.T) {
	roster := derivedRosterWithDerivation(t, func(derivation *definition.RelationDerivation) {
		derivation.Widen.Source = []definition.EnumerationRef{{Axis: siteAxis(), Name: "KeyDirectory"}}
		derivation.Widen.Resolve = definition.GoSymbol{PackagePath: sitePackage, Name: "ResolveDirectoryRoute", ResultIndex: 0}
	})
	target := derivedTarget()
	target.Spec = derivedSelectionSpec()
	rendered, err := Render(target, roster)
	if err != nil {
		t.Fatalf("a widen endpoint stating its own judgment did not emit: %v", err)
	}
	build, _ := functionBody(string(rendered), "deriveDerived1Rows")
	if !strings.Contains(build, "ResolveDirectoryRoute(") {
		t.Fatalf("the widened arm does not use the judgment its endpoint states:\n%s", build)
	}
	if strings.Count(build, "ResolveRoute(") != 1 {
		t.Fatalf("the source judgment answers a widened item:\n%s", build)
	}
}

// TestAWidenEndpointYieldingAnotherItemIsRefusedWithoutOne is the refusal that
// makes the choice honest in both directions: a chain whose items differ and
// names no judgment for them would hand one symbol two types, and a chain
// whose items agree and names a second would be two answers to what a member
// is.
func TestAWidenEndpointYieldingAnotherItemIsRefusedWithoutOne(t *testing.T) {
	target := derivedTarget()
	target.Spec = derivedSelectionSpec()

	missing := derivedRosterWithDerivation(t, func(derivation *definition.RelationDerivation) {
		derivation.Widen.Source = []definition.EnumerationRef{{Axis: siteAxis(), Name: "KeyDirectory"}}
	})
	if source, err := Render(target, missing); err == nil {
		t.Fatalf("a widened chain yielding another item emitted without a judgment for it:\n%s", source)
	}
	spare := derivedRosterWithDerivation(t, func(derivation *definition.RelationDerivation) {
		derivation.Widen.Resolve = definition.GoSymbol{PackagePath: sitePackage, Name: "ResolveDirectoryRoute", ResultIndex: 0}
	})
	if source, err := Render(target, spare); err == nil {
		t.Fatalf("a widened chain yielding the source's own item took a second judgment:\n%s", source)
	}
}

// TestALatticeEndpointIsAJudgmentOverTheWholeInvocation says what the endpoint
// is asked.
//
// Whether a set has a closed list of alternatives can depend on what the set is
// OF, not only on the value the outer source reads: a storage transfer that
// reaches no store has no directory to widen to however its value looks. So
// the endpoint is a judgment over exactly what Resolve is a judgment over,
// minus the item there is not one of yet - the static axis schemas, the
// candidate, and the source value.
func TestALatticeEndpointIsAJudgmentOverTheWholeInvocation(t *testing.T) {
	source := renderDerivedSelectionWith(t, derivedRoster(t))
	build, found := functionBody(source, "deriveDerived1Rows")
	if !found {
		t.Fatalf("the emitted construction has no Build:\n%s", source)
	}
	if !strings.Contains(build, "if site.IsTop(siteSchema, wireSchema, given0, given1) {") {
		t.Fatalf("the endpoint is not asked over the whole invocation:\n%s", build)
	}
}
