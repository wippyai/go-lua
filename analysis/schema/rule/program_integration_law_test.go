package rule

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func programRuleReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func programRuleRelation(key string) member.RelationRef {
	return member.RelationRef{
		Axis:   programRuleReference("axis/program-owner"),
		Member: schema.Key(key),
	}
}

func programRuleProjection(key string) member.ProjectionRef {
	return member.ProjectionRef{
		Axis:   programRuleReference("axis/program-owner"),
		Member: schema.Key(key),
	}
}

func programRuleReducer(key string) member.ReducerRef {
	return member.ReducerRef{
		Axis:   programRuleReference("axis/program-owner"),
		Member: schema.Key(key),
	}
}

func programRuleColumn(key string) axis.OutputRef {
	return axis.OutputRef{Axis: programRuleReference("axis/program-owner"), Key: schema.Key(key)}
}

func TestRuleTemplateDigestIncludesOwnedProgram(t *testing.T) {
	base := Spec{
		Key:      "program-digest-rule",
		Lane:     LaneLink,
		Writes:   "axis/program-digest",
		Owner:    "axis/program-digest",
		Semantic: "semantic/rule/program-digest",
	}
	without, withoutOK := New(base)
	if !withoutOK {
		t.Fatal("declaration-only template")
	}
	base.Program = ruleprogram.Program{
		OperandRole: "semantic/operand/program-digest",
		Candidate:   ruleprogram.AxisRelationCandidate(programRuleRelation("candidate/program-digest")),
		Fold: ruleprogram.FoldDecl{
			Reducer: programRuleReducer("reducer/program-digest"),
			Outputs: []ruleprogram.OutputDecl{{
				Column:      programRuleColumn("column/program-digest"),
				Destination: programRuleProjection("destination/program-digest"),
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
	}
	base.Roles = []schema.Key{"semantic/operand/program-digest"}
	with, withOK := New(base)
	if !withOK {
		t.Fatal("program template")
	}
	if without.Digest() == with.Digest() {
		t.Fatal("Program declaration did not change Rule digest")
	}
	if with.Program().JoinCount() != 0 || !with.Program().Candidate.Available() {
		t.Fatal("template did not retain its owned Program")
	}
	refs := with.References()
	if len(refs) != 6 {
		t.Fatalf("references=%d, want owner/writes plus candidate/reducer/column/destination", len(refs))
	}
}

func TestRuleTemplateRejectsUnrecordedProgramOperandRole(t *testing.T) {
	spec := Spec{
		Key: "program-operand-fence", Lane: LaneLink,
		Writes: "axis/program-owner", Owner: "axis/program-owner",
		Semantic: "semantic/rule/program-operand-fence",
		Roles:    []schema.Key{"semantic/operand/declared"},
		Program: ruleprogram.Program{
			OperandRole: "semantic/operand/foreign",
			Candidate:   ruleprogram.AxisRelationCandidate(programRuleRelation("candidate/program-operand-fence")),
			Fold: ruleprogram.FoldDecl{
				Reducer: programRuleReducer("reducer/program-operand-fence"),
				Outputs: []ruleprogram.OutputDecl{{
					Column: programRuleColumn("column/program-operand-fence"), Destination: programRuleProjection("destination/program-operand-fence"),
					Mode: ruleprogram.ModeExact, ValueSlot: 0,
				}},
			},
		},
	}
	if template, ok := New(spec); ok || template != nil {
		t.Fatal("Program consumed an operand role absent from its Rule declaration")
	}
}

func TestRuleTemplateAllowsAbsentProgramForUnmigratedFamily(t *testing.T) {
	spec := Spec{
		Key:      "program-ratchet-rule",
		Lane:     LaneLink,
		Writes:   "axis/program-ratchet",
		Owner:    "axis/program-ratchet",
		Semantic: "semantic/rule/program-ratchet",
	}
	template, ok := New(spec)
	if !ok || template.Program().Available() {
		t.Fatal("unmigrated declaration was not retained as an absent Program")
	}
}
