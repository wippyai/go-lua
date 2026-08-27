package issuance

import (
	"github.com/wippyai/go-lua/analysis/schema"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

// CandidateSource is the owner-declared, callback-free redemption vocabulary
// for one issued Program relation. The relation key is the arm identity; the
// remaining fields are the typed result and one free source symbol the rule
// emitter spells and the compiler type-checks. No runtime registry or domain
// callback is involved.
type CandidateSource struct {
	Relation schema.Key
	Subject  memberdefinition.GoType
	Redeem   memberdefinition.GoSymbol
}

// Available proves that this source describes the exact direct free-call shape
// used by the generated installer. The function returns the typed candidate in
// result zero; its parameter signature is checked by Go at the generated call
// site rather than mirrored in schema metadata.
func (source CandidateSource) Available() bool {
	if !source.Relation.Available() || !source.Subject.Available() || !source.Redeem.Available() {
		return false
	}
	return source.Redeem.Receiver == (memberdefinition.GoType{}) && !source.Redeem.ReceiverPointer && source.Redeem.ResultIndex == 0
}

const lifecyclePackagePath = "github.com/wippyai/go-lua/analysis/schema/program/lifecycle"

// CandidateSourceFor resolves the owner-declared typed lowering for an
// issuance relation. The relation key is looked up in this owner's canonical
// Entries contribution, and the lowering is selected by that relation's
// canonical target row. No parallel relation registry or axis fallback is
// permitted.
func CandidateSourceFor(relation schema.Key) (CandidateSource, bool) {
	entries, entriesOK := Entries()
	if !entriesOK {
		return CandidateSource{}, false
	}
	declared, declaredOK := canonicalCandidateRelation(entries, relation)
	if !declaredOK {
		return CandidateSource{}, false
	}
	switch declared.Target() {
	case RowSubjectLivenessSpan:
		source := subjectLivenessCandidateSource(relation)
		return source, source.Available()
	default:
		return CandidateSource{}, false
	}
}

func canonicalCandidateRelation(entries []*schemaissuance.Entry, relation schema.Key) (*schemaissuance.Entry, bool) {
	var declared *schemaissuance.Entry
	for _, entry := range entries {
		if entry != nil && entry.Key() == relation && entry.Kind() == schemaissuance.KindRelation {
			declared = entry
			break
		}
	}
	if declared == nil || declared.Space() != RowOccurrence || declared.Target() != RowSubjectLivenessSpan ||
		declared.Cardinality() != schemaissuance.CardinalityOptional {
		return nil, false
	}
	if !canonicalSubjectLivenessJoin(declared.Joins()) || !canonicalTruePredicate(declared) {
		return nil, false
	}
	for _, entry := range entries {
		if entry != nil && entry.Kind() == schemaissuance.KindRowSpace && entry.Key() == declared.Target() {
			return declared, true
		}
	}
	return nil, false
}

func canonicalSubjectLivenessJoin(joins []schemaissuance.JoinField) bool {
	return len(joins) == 1 && joins[0] == (schemaissuance.JoinField{
		Source:  FieldOccurrenceID,
		Target:  FieldSubjectLivenessSpanID,
		Missing: schemaissuance.JoinMissingNoEdge,
	})
}

func canonicalTruePredicate(entry *schemaissuance.Entry) bool {
	if entry == nil || entry.ProgramLen() != 1 || entry.Result() != 1 {
		return false
	}
	instruction, instructionOK := entry.InstructionAt(0)
	return instructionOK && instruction.Op == schemaissuance.OpLiteral && instruction.Out == 1 &&
		instruction.Args == [6]uint16{} && instruction.Ref == "" && instruction.Aux == "" &&
		instruction.Type == schemaissuance.BoolType() && instruction.Literal == 1
}

func subjectLivenessCandidateSource(relation schema.Key) CandidateSource {
	source := CandidateSource{
		Relation: relation,
		Subject:  memberdefinition.GoType{PackagePath: lifecyclePackagePath, Name: "MountedSubjectLiveness"},
		Redeem: memberdefinition.GoSymbol{
			PackagePath: lifecyclePackagePath,
			Name:        "RedeemSubjectLiveness",
			ResultIndex: 0,
		},
	}
	return source
}
