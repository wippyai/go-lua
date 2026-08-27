package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

const specimenPackage = "example/specimen"

func specimenType(name string) definition.GoType {
	return definition.GoType{PackagePath: specimenPackage, Name: name}
}

func specimenMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: specimenPackage, Name: name,
		Receiver: specimenType(receiver), ResultIndex: resultIndex,
	}
}

func specimenAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "specimen"}
}

// specimenRoster is one axis owner's whole authored vocabulary: a candidate
// directory, the predecessor relation a carry folds over, the transition it
// carries with, and the fold itself. It is the smallest declaration a carry
// family can be emitted from.
func specimenRoster(t testing.TB) definition.Roster {
	t.Helper()
	provider := member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}
	base := definition.Definition{
		Name:       "Specimen",
		Axis:       "specimen",
		ImportPath: specimenPackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "KeyCarrier",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "Schema", 0),
		}},
		Signature: definition.Signature{Key: "KeyCarrier", Fact: "FactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "KeyCarrier", Key: "carrier/specimen/key", Type: specimenType("Key")},
			{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")},
		},
		Relations: []definition.Relation{
			{
				Name: "Candidates", Key: "specimen/candidates", Subject: "KeyCarrier",
				CandidateProvider: member.AxisRelationCandidate(provider),
				CandidateResolver: specimenMethod("CandidateForOccurrence", "Schema", 0),
				CandidateOrdinal:  specimenMethod("CandidateOrdinal", "Schema", 0),
				CandidateAt:       specimenMethod("CandidateAt", "Schema", 0),
			},
			{
				Name: "Predecessors", Key: "specimen/predecessors", Subject: "FactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "KeyCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "PredecessorKey", Key: "specimen/predecessor-key", Relation: "Predecessors",
				CandidateProvider: member.AxisRelationCandidate(provider), Role: member.Key, Result: "KeyCarrier",
				Accessor: specimenMethod("Coordinate", "Key", -1),
			},
			{
				Name: "Coordinate", Key: "specimen/coordinate", Relation: "Candidates",
				CandidateProvider: member.AxisRelationCandidate(provider), Role: member.Destination, Result: "KeyCarrier",
				Accessor: specimenMethod("Coordinate", "Key", -1),
			},
		},
		CarryTransforms: []definition.CarryTransform{{
			Name: "Transition", Key: "specimen/transition", Candidate: "KeyCarrier",
			Input: "FactCarrier", Output: "FactCarrier",
			Implementation: specimenMethod("Age", "Key", 0),
		}},
	}
	contribution := definition.Contribution{
		Axis: "specimen",
		Rule: "specimen-carry",
		Reducers: []definition.Reducer{{
			Name: "CarryReducer", Key: "specimen/reducer/carry", Candidate: "KeyCarrier",
			Inputs: []definition.ReducerInput{{
				Axis: specimenAxis(), Carrier: "FactCarrier",
				Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
			}},
			Outputs:        []definition.ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: specimenPackage, Name: "CarryFold", ResultIndex: 0},
		}},
	}
	roster, rosterOK := definition.NewRoster(definition.Source{
		Package: "specimen", Name: "specimen", Base: base,
		Contributions: []definition.Contribution{contribution},
	})
	if !rosterOK {
		t.Fatal("specimen member roster is not admissible")
	}
	return roster
}

func specimenSpec() rule.Spec {
	return rule.Spec{
		Key:      "specimen-carry",
		Writes:   "specimen",
		Owner:    "specimen",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/specimen", Requirement: "program-requirement/unrestricted", Form: "program-form/local-finish"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/specimen",
		Roles:    []schema.Key{"semantic/operand/specimen"},
		Program: program.Program{
			OperandRole: "semantic/operand/specimen",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}),
			Joins: []program.JoinDecl{{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: specimenAxis(), Member: "specimen/predecessors"},
				Key:      member.ProjectionRef{Axis: specimenAxis(), Member: "specimen/predecessor-key"},
				Read: program.ReadDecl{
					PointBound: program.PointBound, Input: 0,
					Axis: program.AxisRef(specimenAxis()), Form: program.Exact,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
					},
				},
			}},
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: specimenAxis(), Member: "specimen/reducer/carry"},
				Inputs:  []program.JoinRef{0},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: specimenAxis(), Key: "specimen/facts"},
					Destination: member.ProjectionRef{Axis: specimenAxis(), Member: "specimen/coordinate"},
					Mode:        program.ModeExact, ValueSlot: 0,
				}},
			},
			Carry: &program.CarryDecl{
				Input: 0, Mode: program.CarryTransform,
				Transform: member.CarryTransformRef{Axis: specimenAxis(), Member: "specimen/transition"},
				Output:    algebra.ScalarSource(algebra.NewSlotSource(0, 0)),
			},
		},
	}
}

func specimenTarget() Target {
	return Target{PackagePath: "example/rule/specimen", PackageName: "specimen", Spec: specimenSpec()}
}

func renderSpecimen(t testing.TB, target Target) string {
	t.Helper()
	source, err := Render(target, specimenRoster(t))
	if err != nil {
		t.Fatalf("specimen declaration did not emit: %v", err)
	}
	return string(source)
}

// TestEmittedFoldCallCarriesOnlyDeclaredCarriers is the call-shape law at the
// emission site. The one call an emitted family makes into its owner's fold
// passes the carriers the declaration derives and nothing beside them: no axis
// schema, no derived plan, no projection, no dense ordinal. Those are the
// sealed state of the installed family, and a fold that reached for one would
// be taking plumbing the declaration never gave it.
func TestEmittedFoldCallCarriesOnlyDeclaredCarriers(t *testing.T) {
	source := renderSpecimen(t, specimenTarget())
	call, found := callArguments(source, "CarryFold")
	if !found {
		t.Fatalf("the emitted family makes no call to the declared fold:\n%s", source)
	}
	permitted := map[string]struct{}{"fold.candidate": {}, "cell": {}}
	for _, argument := range call {
		if _, allowed := permitted[argument]; !allowed {
			t.Fatalf("the emitted fold call passes %q, which is not a carrier the declaration derives", argument)
		}
	}
	if len(call) != 2 {
		t.Fatalf("the emitted fold call passes %d arguments, the declaration derives 2", len(call))
	}
}

// TestEmittedFamilyHoldsNoSchemaItNeverReads states the family/installer
// split. An axis schema the declaration reaches only to resolve a candidate is
// the installer's; a family that retained it would carry owner state no
// invocation path reads, which is how a sealed family grows back into a
// plumbing carrier.
func TestEmittedFamilyHoldsNoSchemaItNeverReads(t *testing.T) {
	source := renderSpecimen(t, specimenTarget())
	body, found := typeBody(source, "sealedFamily")
	if !found {
		t.Fatalf("the emitted source declares no family type:\n%s", source)
	}
	if strings.Contains(body, "Schema") {
		t.Fatalf("the emitted family retains an axis schema its worker never reads:\n%s", body)
	}
	installer, installerFound := typeBody(source, "familyInstaller")
	if !installerFound || !strings.Contains(installer, "specimenSchema") {
		t.Fatalf("the emitted installer does not hold the candidate directory's schema:\n%s", installer)
	}
}

// TestEmittedInputCapacityIsTheDeclaredPortCount holds the family to the
// declaration's own port geometry. Two reads that share one input port are one
// port, and a family that answered its read count instead would size the run's
// port vector by a coincidence of how many joins were authored.
func TestEmittedInputCapacityIsTheDeclaredPortCount(t *testing.T) {
	source := renderSpecimen(t, specimenTarget())
	if !strings.Contains(source, "func (*sealedFamily) InputCapacity() int  { return 1 }") {
		t.Fatalf("the emitted family does not answer the declaration's one input port:\n%s", source)
	}
	if !strings.Contains(source, "func (*sealedFamily) OutputCapacity() int { return 1 }") {
		t.Fatalf("the emitted family does not answer the declaration's one output column:\n%s", source)
	}
}

// TestEmissionIsAFunctionOfTheDeclaration is the freshness law's premise: two
// renders of one declaration are one file. A generator whose output depended
// on map order could not have a freshness law at all.
func TestEmissionIsAFunctionOfTheDeclaration(t *testing.T) {
	first := renderSpecimen(t, specimenTarget())
	for attempt := 0; attempt < 8; attempt++ {
		if again := renderSpecimen(t, specimenTarget()); again != first {
			t.Fatal("two renders of one declaration produced different families")
		}
	}
}

// TestAnUnexpressibleDeclarationIsRefusedByName is the refusal law. A shape
// the emitted vocabulary has no form for is named - the rule, the clause, and
// why - rather than silently omitted or half emitted. A generator that emitted
// a partial family would hand the engine a rule whose declaration and
// execution disagree.
func TestAnUnexpressibleDeclarationIsRefusedByName(t *testing.T) {
	for _, probe := range []struct {
		name   string
		mutate func(*rule.Spec)
		clause string
	}{
		// The specimen's candidate belongs to the axis it writes, so dropping
		// its carry states the derived exact fold rather than a shape with no
		// form. The identity-carry law belongs to the heterogeneous consumer,
		// where the written Factor reaches a coordinate no directory of its
		// own enumerates, and is stated there.
		{
			name: "a summary read the product cannot span",
			mutate: func(spec *rule.Spec) {
				spec.Program.Joins[0].Read.Form = program.Summary
				spec.Program.Joins[0].Predicate = member.ProjectionRef{Axis: specimenAxis(), Member: "specimen/coordinate"}
				spec.Program.Joins[0].Read.Contract.DenominatorRef = program.DenominatorRef{
					Surface: schema.SurfaceKindDenominator, Key: "coordinates/specimen",
				}
			},
			// A vector delivered by a Factor's own cursor declares no span, so
			// the product that would fold it has no width for its cells. The
			// refusal names that, which is the reason, rather than naming the
			// carry beside it.
			clause: "an exact fold over a vector with no span",
		},
		// A structural publication DOES have an emitted family now. What is
		// refused by name is a structural row that has not said what it
		// mounts, or one that claims a coordinate it does not publish into.
		{
			name: "a structural publication that names no branch vocabulary",
			mutate: func(spec *rule.Spec) {
				spec.Program.Fold.Outputs[0].Mode = program.ModeStructural
				spec.Program.Carry = nil
			},
			clause: "a structural output with no branch vocabulary",
		},
		{
			name:   "a structural publication beside a carry",
			mutate: func(spec *rule.Spec) { spec.Program.Fold.Outputs[0].Mode = program.ModeStructural },
			clause: "a structural output beside a carry",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			spec := specimenSpec()
			probe.mutate(&spec)
			target := specimenTarget()
			target.Spec = spec
			source, err := Render(target, specimenRoster(t))
			if err == nil {
				t.Fatalf("an unexpressible declaration emitted a family:\n%s", source)
			}
			refusal, named := err.(Unexpressible)
			if !named {
				t.Fatalf("refusal is not named as unexpressible: %v", err)
			}
			if refusal.Rule != spec.Key {
				t.Fatalf("refusal names rule %q, the declaration is %q", string(refusal.Rule), string(spec.Key))
			}
			if !strings.Contains(refusal.Clause, probe.clause) {
				t.Fatalf("refusal clause is %q, want it to name %q", refusal.Clause, probe.clause)
			}
			if refusal.Detail == "" {
				t.Fatal("refusal names a clause with no reason")
			}
		})
	}
}

// TestAFamilyIsEmittedForOneRuleOnly fences the target. An emitter that
// accepted a declaration with no key would write a file no declaration owns.
func TestAFamilyIsEmittedForOneRuleOnly(t *testing.T) {
	for _, target := range []Target{
		{PackagePath: "", PackageName: "specimen", Spec: specimenSpec()},
		{PackagePath: "example/rule/specimen", PackageName: "", Spec: specimenSpec()},
		{PackagePath: "example/rule/specimen", PackageName: "specimen"},
	} {
		if source, err := Render(target, specimenRoster(t)); err == nil {
			t.Fatalf("an incomplete target emitted a family:\n%s", source)
		}
	}
}

// callArguments answers the argument list of the first call to name in source.
func callArguments(source, name string) ([]string, bool) {
	marker := name + "("
	start := strings.Index(source, marker)
	if start < 0 {
		return nil, false
	}
	open := start + len(marker)
	depth := 1
	for cursor := open; cursor < len(source); cursor++ {
		switch source[cursor] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				body := strings.TrimSpace(source[open:cursor])
				if body == "" {
					return nil, true
				}
				parts := strings.Split(body, ",")
				for index := range parts {
					parts[index] = strings.TrimSpace(parts[index])
				}
				return parts, true
			}
		}
	}
	return nil, false
}

// typeBody answers the field block of the named type declaration.
func typeBody(source, name string) (string, bool) {
	marker := "\ntype " + name + " struct {"
	start := strings.Index(source, marker)
	if start < 0 {
		return "", false
	}
	rest := source[start+len(marker):]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}
