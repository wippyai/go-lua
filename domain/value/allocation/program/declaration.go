// Package program owns the allocation result's callback-free rule declaration.
//
// It names Value's sealed allocation-receipt directory as its candidate, the
// exact Value publication at the coordinate that receipt was issued with, and
// the owner-issued transition every carried coordinate passes through. It
// declares no read: the receipt is the whole of the evidence the judgment rests
// on. It contains no engine slot, runtime callback, or compatibility path; the
// judgment itself stays in domain/value/allocation.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The allocation family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "value"
	OutputKey   schema.Key = "value/facts"
	RuleKey     schema.Key = "value-allocation"
	RuleRole               = "rule/value/allocation"
	OperandRole            = "operand/value/allocation"

	// Value owns the receipt directory, the coordinate it publishes at, the
	// transition its carry applies, and the fold that answers from the receipt.
	AllocationResults          schema.Key = valuedomain.AllocationResults
	AllocationResultCoordinate schema.Key = valuedomain.AllocationResultCoordinate
	AllocationCarryTransform   schema.Key = valuedomain.AllocationCarryTransform
	Reducer                    schema.Key = valuedomain.AllocationResultReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleIssues is this rule's mounted issuance geometry: one allocation
// occurrence, entered where the constructor is written.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/allocation",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/local-entry",
	}}
}

// RuleEntry is the canonical callback-free allocation rule declaration. The
// family is installed through the generated RuleFamily seam; this value is what
// Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  AllocationResult(),
	}
}

// StructureSpecs contributes this rule's own semantic roles. The Value factor
// role is the axis owner's declaration and is therefore not re-authored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// AllocationResult returns the immutable allocation rule declaration.
//
// The candidate is Value's sealed allocation receipt. There is no join: the
// receipt already carries the Recent fact its constructor mints, so no cell of
// any Factor is narrower evidence for it, and the publication holds over the
// invocation's own support. It is exact at the coordinate the receipt was
// issued with, so Value's relation owner projects it, and the carry ages every
// other carried coordinate through the receipt's own Recent-to-Summary
// transition in the same patch.
func AllocationResult() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   valueAxis,
			Member: AllocationResults,
		}),
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: Reducer},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: valueAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: AllocationResultCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{
			Input:     0,
			Mode:      ruleprogram.CarryTransform,
			Transform: member.CarryTransformRef{Axis: valueAxis, Member: AllocationCarryTransform},
		},
	}
}
