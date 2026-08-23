package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func wiringSpec(withProgram bool) rule.Spec {
	valueAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
	spec := rule.Spec{
		Key:      "wiring-law-specimen",
		Lane:     rule.LaneMounted,
		Writes:   "value",
		Owner:    "value",
		Semantic: "semantic/rule/value/source",
		Issues: []rule.Issuance{{
			Occurrence:  "occurrence/value-source",
			Requirement: "program-requirement/unrestricted",
			Form:        "program-form/base-none",
		}},
	}
	if !withProgram {
		return spec
	}
	spec.Roles = []schema.Key{"semantic/operand/value/source"}
	spec.Program = ruleprogram.Program{
		OperandRole: "semantic/operand/value/source",
		Candidate:   member.RelationRef{Axis: valueAxis, Member: valuedomain.SourceSeeds},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: valuedomain.SourceReducer},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: valueAxis, Key: "value/facts"},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: valuedomain.SourceCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
	}
	return spec
}

func handWiring(spec rule.Spec) (*rule.Template, RuleContributor[principals, authorities], bool) {
	return WireRule(
		spec,
		func(*engine.SchemaBuilder, rule.Declaration[principals]) (struct{}, bool) { return struct{}{}, true },
		func(*engine.SchemaBinding, rule.Registration[struct{}]) (engine.RuleSlotCapability, bool) {
			return engine.RuleSlotCapability{}, true
		},
		nil,
		func(*engine.SchemaBinding, rule.Binding[authorities, struct{}]) (struct{}, bool) {
			return struct{}{}, true
		},
		nil, nil, nil,
	)
}

// TestProgramCarryingTemplateIsRefusedByNameWhenHandWired states the wiring
// law both ways round. A row's declaration and its wiring are authored in
// different packages, and only one of them can be executed: a Program is
// lowered by the composition, a hook set is run by its domain. A row that says
// both, or neither, is refused where the two halves meet, under a name that
// says which rule and which half.
func TestProgramCarryingTemplateIsRefusedByNameWhenHandWired(t *testing.T) {
	template, contributor, ok := handWiring(wiringSpec(true))
	if ok {
		t.Fatal("a Program-carrying template was admitted through the hand-wired path")
	}
	if refusal := contributor.complete(template); refusal != WiringProgramWiredByHand {
		t.Fatalf("refusal = %s, want %s", refusal, WiringProgramWiredByHand)
	}
}

// TestGeneratedWiringIsRefusedByNameWithoutAProgram is the converse: the
// composition has nothing to lower for a row wired generated whose declaration
// states no Program.
func TestGeneratedWiringIsRefusedByNameWithoutAProgram(t *testing.T) {
	template, ok := rule.New(wiringSpec(false))
	if !ok {
		t.Fatal("Program-less specimen declaration rejected")
	}
	contributor := RuleContributor[principals, authorities]{generated: true}
	if refusal := contributor.complete(template); refusal != WiringGeneratedWithoutProgram {
		t.Fatalf("refusal = %s, want %s", refusal, WiringGeneratedWithoutProgram)
	}
	if _, _, admitted := WireGeneratedRule[principals, authorities](wiringSpec(false)); admitted {
		t.Fatal("a Program-less template was admitted through the generated path")
	}
}

// TestMatchedWiringIsAdmitted is the positive: each half admitted against its
// own declaration.
func TestMatchedWiringIsAdmitted(t *testing.T) {
	generatedTemplate, generatedContributor, generatedOK := WireGeneratedRule[principals, authorities](wiringSpec(true))
	if !generatedOK {
		t.Fatal("a Program-carrying template was refused by the generated path")
	}
	if refusal := generatedContributor.complete(generatedTemplate); refusal != WiringAdmitted {
		t.Fatalf("generated refusal = %s", refusal)
	}
	handTemplate, handContributor, handOK := handWiring(wiringSpec(false))
	if !handOK {
		t.Fatal("a Program-less template was refused by the hand-wired path")
	}
	if refusal := handContributor.complete(handTemplate); refusal != WiringAdmitted {
		t.Fatalf("hand-wired refusal = %s", refusal)
	}
}

// TestRuleCatalogNamesItsFirstWiringRefusal states that the catalog's verdict
// is readable: when the roster does not admit, the failure names the rule and
// the mismatch rather than leaving a slot ordinal to be walked back.
func TestRuleCatalogNamesItsFirstWiringRefusal(t *testing.T) {
	_, _, ok := RuleTemplates[principals, authorities]()
	failure := RuleTemplateRefusal[principals, authorities]()
	if ok != !failure.Available() {
		t.Fatalf("catalog verdict %t disagrees with named failure %s", ok, failure)
	}
	if failure.Available() && !failure.Rule.Available() {
		t.Fatalf("wiring failure %s names no rule", failure)
	}
}
