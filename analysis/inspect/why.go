package inspect

import (
	"strings"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/composite"
)

// formatWhy walks the provenance of one solved cell back through the Program
// declaration that produced it.
//
// The walk is the declaration's own: a rule's Program is a Candidate relation,
// an ordered Joins list, and a Fold. Naming which input rows a cell's value
// could have come from is therefore naming the declared joins of every rule
// whose Fold publishes into the cell's projection, each under the read form
// (Exact, Selected, Summary, Complete) it declares. Nothing here infers a data
// path the declaration does not state.
//
// Why a cell is not Concrete is the fold's own disposition: the row's class is
// one of the five declared reduction outcomes, and that key is printed
// verbatim. A cell whose payload was not admitted is named by the codec's
// refusal, also verbatim.
func formatWhy(session *Session, id identity.ContentID, scoped bool) string {
	var b strings.Builder
	writeDiagnostics(&b, "compile", session.compileDiag)
	writeDiagnostics(&b, "solve", session.solveDiag)
	if !scoped {
		writeWhyFamilies(&b, session)
		writeFindings(&b, session, identity.ContentID{}, false)
		return b.String()
	}
	writef(&b, "why.Identity=%s", id)
	record, ok := session.Lookup(id)
	if !ok {
		writef(&b, "session.Lookup(%s)=false", id)
		writeCoordinate(&b, session, id)
		writeFindings(&b, session, id, true)
		return b.String()
	}
	if record.kind != rowQuery {
		writef(&b, "session.Lookup(%s).Kind=%s", id, record.kind.String())
		writeFindings(&b, session, id, true)
		return b.String()
	}
	writeWhyQuery(&b, session, record)
	writeFindings(&b, session, id, true)
	return b.String()
}

// formatWhyKey walks provenance for one declaration key: a publication family
// answers with the rules that could write it; a factor axis answers with every
// family that reads it and every rule that publishes into it.
func formatWhyKey(session *Session, key schema.Key) string {
	var b strings.Builder
	writef(&b, "why.Key=%s", key)
	if !key.Available() {
		writef(&b, "schema.Key.Available=false")
		return b.String()
	}
	if _, familyOK := session.declared.QueryRegistration(key); familyOK {
		writeDeclaringRules(&b, session, key)
		return b.String()
	}
	found := false
	for _, query := range composite.QueryIssuance(session.compilation) {
		registration, registrationOK := session.declared.QueryRegistration(query.Family)
		if !registrationOK {
			continue
		}
		for subjectIndex := 0; subjectIndex < registration.SubjectCount(); subjectIndex++ {
			subject, subjectOK := registration.SubjectAt(subjectIndex)
			if !subjectOK || subject != key {
				continue
			}
			found = true
			writef(&b, "why[%s].ReadBy=%s", key, query.Family)
		}
	}
	if !found {
		writef(&b, "why[%s].ReadBy=none", key)
	}
	writef(&b, "why[%s].DeclaringRuleCount=%d", key, writeAxisProducers(&b, session, key, key))
	return b.String()
}

// writeWhyFamilies is the unscoped walk: every declared publication family and
// the rules whose folds could write it.
func writeWhyFamilies(b *strings.Builder, session *Session) {
	issued := composite.QueryIssuance(session.compilation)
	writef(b, "composite.QueryIssuance.Count=%d", len(issued))
	for _, query := range issued {
		writef(b, "composite.QueryIssuance[%d].Family=%s", query.Ordinal, query.Family)
		writef(b, "composite.QueryIssuance[%d].Population=%s", query.Ordinal, query.Population)
		writef(b, "composite.QueryIssuance[%d].Projection=%s", query.Ordinal, query.Projection)
		writeDeclaringRules(b, session, query.Family)
	}
}

func writeWhyQuery(b *strings.Builder, session *Session, record rowRecord) {
	solved := session.result
	if solved == nil {
		writef(b, "result=unavailable")
		return
	}
	family, familyOK := solved.FamilyAt(record.queryFamily)
	if !familyOK {
		return
	}
	prefix := queryPrefix(record.queryFamily, record.query)
	writef(b, "result.FamilyAt(%d).Key=%s", record.queryFamily, family.Key())
	writeCell(b, session, prefix, record.queryFamily, record.query)
	for _, query := range composite.QueryIssuance(session.compilation) {
		if query.Family != family.Key() {
			continue
		}
		writef(b, "composite.QueryIssuance[%d].Population=%s", query.Ordinal, query.Population)
		writef(b, "composite.QueryIssuance[%d].Projection=%s", query.Ordinal, query.Projection)
		break
	}
	writeDeclaringRules(b, session, family.Key())
}

// writeDeclaringRules names every rule whose declared Fold publishes a column
// on an axis the family's query registration reads, then walks that rule's
// Candidate and Joins. Those joins are the input rows the cell's value could
// have come from, each under the read form the declaration states.
func writeDeclaringRules(b *strings.Builder, session *Session, family schema.Key) {
	declared := session.declared
	registration, registrationOK := declared.QueryRegistration(family)
	if !registrationOK {
		writef(b, "why[%s].QueryRegistration=unavailable", family)
		return
	}
	writef(b, "why[%s].QueryRegistration.PopulationKind=%d", family, uint8(registration.PopulationKind()))
	writef(b, "why[%s].QueryRegistration.SubjectCount=%d", family, registration.SubjectCount())
	count := 0
	for subjectIndex := 0; subjectIndex < registration.SubjectCount(); subjectIndex++ {
		subject, subjectOK := registration.SubjectAt(subjectIndex)
		if !subjectOK {
			continue
		}
		writef(b, "why[%s].QueryRegistration.SubjectAt(%d)=%s", family, subjectIndex, subject)
		count += writeAxisProducers(b, session, family, subject)
	}
	writef(b, "why[%s].DeclaringRuleCount=%d", family, count)
}

// writeAxisProducers walks every rule that publishes into one subject axis.
func writeAxisProducers(b *strings.Builder, session *Session, family, axis schema.Key) int {
	declared := session.declared
	count := 0
	for position := 0; position < declared.RuleCount(); position++ {
		template, templateOK := declared.RuleAt(position)
		if !templateOK {
			continue
		}
		declaration := template.Program()
		if !declaration.Available() || !writesAxis(declaration, axis) {
			continue
		}
		count++
		key := template.Key()
		writef(b, "why[%s].Axis[%s].Rule=%s", family, axis, key)
		writef(b, "why[%s].Rule[%s].Owner=%s", family, key, template.Owner())
		writef(b, "why[%s].Rule[%s].Writes=%s", family, key, template.Writes())
		writef(b, "why[%s].Rule[%s].Program.Candidate.Axis=%s", family, key, declaration.Candidate.Axis.Key)
		writef(b, "why[%s].Rule[%s].Program.Candidate.Member=%s", family, key, declaration.Candidate.Member)
		writef(b, "why[%s].Rule[%s].Program.JoinCount=%d", family, key, declaration.JoinCount())
		for joinIndex := 0; joinIndex < declaration.JoinCount(); joinIndex++ {
			join, joinOK := declaration.JoinAt(joinIndex)
			if !joinOK {
				continue
			}
			label := "why[" + string(family) + "].Rule[" + string(key) + "].Program.Joins[" + decimal(uint64(joinIndex)) + "]"
			for sourceIndex, source := range join.Sources {
				writef(b, "%s.Sources[%d]=%s", label, sourceIndex, sourceSpelling(source))
			}
			writef(b, "%s.Relation.Axis=%s", label, join.Relation.Axis.Key)
			writef(b, "%s.Relation.Member=%s", label, join.Relation.Member)
			writef(b, "%s.Key.Axis=%s", label, join.Key.Axis.Key)
			writef(b, "%s.Key.Member=%s", label, join.Key.Member)
			if join.Predicate.Declared() {
				writef(b, "%s.Predicate.Member=%s", label, join.Predicate.Member)
			}
			writef(b, "%s.Read.Axis=%s", label, join.Read.Axis.EntryReference().Key)
			writef(b, "%s.Read.Form=%s", label, readFormSpelling(join.Read.Form))
			writef(b, "%s.Read.Contract.Order=%s", label, orderSpelling(join.Read.Contract.Order))
			writef(b, "%s.Read.Contract.Sparse=%s", label, sparseSpelling(join.Read.Contract.Sparse))
			writef(b, "%s.Read.Contract.OnOpaque=%s", label, onOpaqueSpelling(join.Read.Contract.OnOpaque))
			writef(b, "%s.Read.Contract.Multiplicity=%s", label, multiplicitySpelling(join.Read.Contract.Multiplicity))
			if join.Read.Contract.DenominatorRef.Declared() {
				writef(b, "%s.Read.Contract.DenominatorRef=%s", label, join.Read.Contract.DenominatorRef.EntryReference().Key)
			}
		}
		writef(b, "why[%s].Rule[%s].Program.Fold.Reducer.Axis=%s", family, key, declaration.Fold.Reducer.Axis.Key)
		writef(b, "why[%s].Rule[%s].Program.Fold.Reducer.Member=%s", family, key, declaration.Fold.Reducer.Member)
		for inputIndex, input := range declaration.Fold.Inputs {
			writef(b, "why[%s].Rule[%s].Program.Fold.Inputs[%d]=Joins[%d]", family, key, inputIndex, uint64(input))
		}
		for outputIndex, output := range declaration.Fold.Outputs {
			if output.Column.Axis.Key != axis {
				continue
			}
			writef(b, "why[%s].Rule[%s].Program.Fold.Outputs[%d].Column.Key=%s", family, key, outputIndex, output.Column.Key)
			writef(b, "why[%s].Rule[%s].Program.Fold.Outputs[%d].Destination.Member=%s", family, key, outputIndex, output.Destination.Member)
			writef(b, "why[%s].Rule[%s].Program.Fold.Outputs[%d].Mode=%s", family, key, outputIndex, outputModeSpelling(output.Mode))
			if output.RouteJoinPresent {
				writef(b, "why[%s].Rule[%s].Program.Fold.Outputs[%d].RouteJoin=Joins[%d]", family, key, outputIndex, uint64(output.RouteJoin))
			}
		}
	}
	return count
}

func writeFindings(b *strings.Builder, session *Session, id identity.ContentID, scoped bool) {
	report := session.report
	if report == nil || !report.Available() {
		writef(b, "report.Available=false")
		return
	}
	writef(b, "report.CollectionFailure=%d", uint8(report.CollectionFailure()))
	writef(b, "report.FindingCount=%d", report.FindingCount())
	for index := 0; index < report.FindingCount(); index++ {
		finding, ok := report.FindingAt(index)
		if !ok {
			continue
		}
		if scoped {
			match := false
			if findingID, idOK := finding.ID(); idOK && findingID == id {
				match = true
			}
			if subject, subjectOK := finding.SubjectID(); subjectOK && subject == id {
				match = true
			}
			if !match {
				continue
			}
		}
		writef(b, "report.FindingAt(%d).Code=%s", index, finding.Code())
		writef(b, "report.FindingAt(%d).Message=%s", index, finding.Message())
		writef(b, "report.FindingAt(%d).Render=%s", index, compactSpace(finding.Render()))
	}
}

func writeDiagnostics(b *strings.Builder, origin string, diagnostics anadiag.AnalyzeDiagnostics) {
	writef(b, "%s.AnalyzeDiagnostics.Phase=%s", origin, diagnostics.Phase.String())
	writef(b, "%s.AnalyzeDiagnostics.Reason=%s", origin, diagnostics.Reason.String())
	writef(b, "%s.AnalyzeDiagnostics.AssembleStage=%s", origin, diagnostics.AssembleStage.String())
	writef(b, "%s.AnalyzeDiagnostics.Construction=%s", origin, diagnostics.Construction.String())
	writef(b, "%s.AnalyzeDiagnostics.ObservationAttach=%s", origin, diagnostics.ObservationAttach.String())
	if diagnostics.Engine.Failure.Available() {
		writef(b, "%s.AnalyzeDiagnostics.Engine.Failure.Reason=%d", origin, uint8(diagnostics.Engine.Failure.Reason()))
		failure := diagnostics.Engine.Failure.Failure()
		if failure.Available() {
			writef(b, "%s.AnalyzeDiagnostics.Engine.Failure.Failure=%s", origin, failure.String())
		}
	}
}
