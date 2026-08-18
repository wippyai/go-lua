package analysis

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// compiledNativeArithmeticSummary is a short-lived projection of one
// ProgramArtifact arithmetic summary. It is deliberately not retained by the
// result geometry: native publication reads the immutable artifact column at
// detach time and immediately turns this value into publication rows.
type compiledNativeArithmeticSummary struct {
	mount, artifact, proof, occurrence, body, point, span identity.ContentID
	op                                                    flowkind.BinaryOp
	left, right, result                                   programartifact.NumericRepresentation
	divisor                                               programartifact.ArithmeticDivisorProperty
}

func (summary compiledNativeArithmeticSummary) valid() bool {
	return summary.mount.Available() && summary.artifact.Available() && summary.proof.Available() &&
		summary.occurrence.Available() && summary.body.Available() && summary.point.Available() && summary.span.Available() &&
		summary.proof != summary.occurrence && summary.op >= flowkind.BinaryAdd && summary.op <= flowkind.BinaryPow &&
		summary.left.Valid() && summary.right.Valid() && summary.result.Valid() && summary.divisor.Valid() &&
		(summary.divisor == programartifact.ArithmeticDivisorNone || summary.op == flowkind.BinaryIDiv)
}

type compiledNativeUnarySummary struct {
	mount, artifact, proof, occurrence, body, point, span identity.ContentID
	op                                                    flowkind.UnaryOp
	operand, result                                       programartifact.NumericRepresentation
}

func (summary compiledNativeUnarySummary) valid() bool {
	return summary.mount.Available() && summary.artifact.Available() && summary.proof.Available() &&
		summary.occurrence.Available() && summary.body.Available() && summary.point.Available() && summary.span.Available() &&
		summary.proof != summary.occurrence && summary.op == flowkind.UnaryNeg && summary.operand.Valid() &&
		summary.result.Valid() && summary.operand == summary.result
}

type compiledNativeScalarSourceKind uint8

const (
	compiledNativeScalarSourceProgramSummary compiledNativeScalarSourceKind = iota + 1
)

// compiledNativeScalarSource is a short-lived projection of one exact scalar
// summary. ProgramArtifact remains the owner of the summary; this value exists
// only while its publication rows are being assembled.
type compiledNativeScalarSource struct {
	kind       compiledNativeScalarSourceKind
	mount      identity.ContentID
	artifact   identity.ContentID
	proof      identity.ContentID
	occurrence identity.ContentID
	body       identity.ContentID
	point      identity.ContentID
	span       identity.ContentID
	family     keyspace.Family
	literal    keyspace.LiteralValue
}

func (source compiledNativeScalarSource) valid() bool {
	if !source.mount.Available() || !source.artifact.Available() || !source.proof.Available() ||
		!source.occurrence.Available() || !source.body.Available() || !source.point.Available() || !source.span.Available() {
		return false
	}
	switch source.kind {
	case compiledNativeScalarSourceProgramSummary:
		if source.proof == source.occurrence ||
			(source.literal.Kind != keyspace.LiteralInteger && source.literal.Kind != keyspace.LiteralFloat) {
			return false
		}
	default:
		return false
	}
	switch source.family {
	case keyspace.FamilyNil:
		return source.literal == (keyspace.LiteralValue{})
	case keyspace.FamilyBool:
		return source.literal.Kind == keyspace.LiteralBool
	case keyspace.FamilyInteger:
		return source.literal.Kind == keyspace.LiteralInteger
	case keyspace.FamilyFloat:
		return source.literal.Kind == keyspace.LiteralFloat
	case keyspace.FamilyString:
		return source.literal.Kind == keyspace.LiteralString
	default:
		return false
	}
}

// appendNativeArtifactSummaryRows reads the ProgramArtifact-owned native
// summary columns directly. The compiled values are never put in a receipt or
// another cache; each is consumed immediately by the native row projection.
func appendNativeArtifactSummaryRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, mounts []mountedProgramArtifact) bool {
	if rows == nil || seen == nil || len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() || !mount.artifact.ID().Available() {
			return false
		}
		for summaryIndex := 0; summaryIndex < mount.artifact.ExactScalarSummaryCount(); summaryIndex++ {
			summary, summaryOK := mount.artifact.ExactScalarSummaryAt(summaryIndex)
			literal, literalOK := summary.Literal()
			occurrence, occurrenceOK := mount.artifact.OccurrenceForID(programartifact.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !literalOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID {
				return false
			}
			point, pointOK := exactNativeScalarRulePoint(mount.artifact, summary.OccurrenceID())
			family := keyspace.FamilyInvalid
			switch literal.Kind {
			case keyspace.LiteralInteger:
				family = keyspace.FamilyInteger
			case keyspace.LiteralFloat:
				family = keyspace.FamilyFloat
			}
			source := compiledNativeScalarSource{
				kind: compiledNativeScalarSourceProgramSummary, mount: mount.moduleKey, artifact: mount.artifact.ID(),
				proof: summary.ID(), occurrence: summary.OccurrenceID(), body: bodyID, point: point, span: summary.SubjectID(),
				family: family, literal: literal,
			}
			if !pointOK || !source.valid() || !appendNativeStaticScalarRows(rows, seen, source) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < mount.artifact.ArithmeticSummaryCount(); summaryIndex++ {
			summary, summaryOK := mount.artifact.ArithmeticSummaryAt(summaryIndex)
			left, right, result, representationsOK := summary.Representations()
			occurrence, occurrenceOK := mount.artifact.OccurrenceForID(programartifact.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			point, pointOK := exactNativeScalarRulePoint(mount.artifact, summary.OccurrenceID())
			compiled := compiledNativeArithmeticSummary{
				mount: mount.moduleKey, artifact: mount.artifact.ID(), proof: summary.ID(), occurrence: summary.OccurrenceID(),
				body: bodyID, point: point, span: occurrence.ID(), op: summary.Operator(), left: left, right: right, result: result,
				divisor: summary.DivisorProperty(),
			}
			if !summaryOK || !representationsOK || !occurrenceOK || !bodyOK || !pointOK || summary.BodyPathID() != bodyID || !compiled.valid() ||
				!appendNativeArithmeticRows(rows, seen, compiled) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < mount.artifact.UnarySummaryCount(); summaryIndex++ {
			summary, summaryOK := mount.artifact.UnarySummaryAt(summaryIndex)
			operand, result, representationsOK := summary.Representations()
			occurrence, occurrenceOK := mount.artifact.OccurrenceForID(programartifact.OccurrenceUnary, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			compiled := compiledNativeUnarySummary{
				mount: mount.moduleKey, artifact: mount.artifact.ID(), proof: summary.ID(), occurrence: summary.OccurrenceID(),
				body: bodyID, point: summary.OutputPointID(), span: occurrence.ID(), op: summary.Operator(), operand: operand, result: result,
			}
			if !summaryOK || !representationsOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID || !compiled.valid() ||
				!appendNativeUnaryRows(rows, seen, compiled) {
				return false
			}
		}
	}
	return true
}

func exactNativeScalarRulePoint(artifact *programartifact.Artifact, occurrence identity.ContentID) (identity.ContentID, bool) {
	if artifact == nil || !artifact.Available() || !occurrence.Available() {
		return identity.ContentID{}, false
	}
	var point identity.ContentID
	found := false
	for index := 0; index < artifact.RulePlacementCount(); index++ {
		row, rowOK := artifact.RulePlacementAt(index)
		output, outputOK := row.OutputSemanticID()
		candidate, pointOK := row.PointAt(0)
		if !rowOK || !outputOK || !pointOK {
			continue
		}
		if row.ID() != occurrence || output != occurrence {
			continue
		}
		if found || row.Stage() != programartifact.RuleStageLocal {
			return identity.ContentID{}, false
		}
		point, found = candidate, true
	}
	return point, found && point.Available()
}
