package inspect

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/composite"
)

// formatPublish renders the publication layer: the summary columns each
// family declares, every query's published answer, the declared activation
// transitions the topology carries, and the diagnostics the oracle renders.
func formatPublish(session *Session) string {
	var b strings.Builder
	writef(&b, "session.Fixture=%s", session.fixture)
	// The summary columns are a declaration: every family publishes its
	// answers under one sealed layout, whether or not this fixture reached a
	// solve. They are rendered from the issuance inventory first, so the
	// column layer is readable at a refused solve too.
	issued := composite.QueryIssuance(session.compilation)
	writef(&b, "composite.QueryIssuance.Count=%d", len(issued))
	for _, query := range issued {
		writef(&b, "composite.QueryIssuance[%d].Family=%s", query.Ordinal, query.Family)
		writef(&b, "composite.QueryIssuance[%d].Population=%s", query.Ordinal, query.Population)
		writef(&b, "composite.QueryIssuance[%d].Projection=%s", query.Ordinal, query.Projection)
		writeLayout(&b, session, query.Family)
	}
	solved := session.result
	if solved == nil {
		writef(&b, "result=unavailable")
	} else {
		writef(&b, "result.SourceID=%s", solved.SourceID())
		writef(&b, "result.ContentID=%s", solved.ContentID())
		writef(&b, "result.FamilyCount=%d", solved.FamilyCount())
		for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
			family, familyOK := solved.FamilyAt(familyIndex)
			if !familyOK {
				continue
			}
			writef(&b, "result.FamilyAt(%d).Key=%s", familyIndex, family.Key())
			writef(&b, "result.FamilyAt(%d).ID=%s", familyIndex, family.ID())
			writef(&b, "result.FamilyAt(%d).Codec=%s", familyIndex, semanticSpelling(family.Codec()))
			writef(&b, "result.FamilyAt(%d).ContractID=%s", familyIndex, family.ContractID())
			writef(&b, "result.FamilyAt(%d).QueryCount=%d", familyIndex, family.QueryCount())
			for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
				query, queryOK := family.QueryAt(queryIndex)
				if !queryOK {
					continue
				}
				prefix := queryPrefix(familyIndex, queryIndex)
				if site, siteOK := query.SiteID(); siteOK {
					writef(&b, "%s.SiteID=%s", prefix, site)
				}
				writeCell(&b, session, prefix, familyIndex, queryIndex)
			}
		}
		writef(&b, "result.NativePublicationAvailable=%t", solved.NativePublicationAvailable())
		writef(&b, "result.NativePublicationCount=%d", solved.NativePublicationCount())
		for index := 0; index < solved.NativePublicationCount(); index++ {
			row, ok := solved.NativePublicationAt(index)
			if !ok {
				continue
			}
			if id, idOK := row.ID(); idOK {
				writef(&b, "result.NativePublicationAt(%d).ID=%s", index, id)
			}
			writef(&b, "result.NativePublicationAt(%d).Kind=%s", index, row.Kind().String())
			writef(&b, "result.NativePublicationAt(%d).Lane=%s", index, row.Lane().String())
			writef(&b, "result.NativePublicationAt(%d).Trust=%s", index, row.Trust().String())
			writef(&b, "result.NativePublicationAt(%d).Family=%s", index, row.Family())
			writef(&b, "result.NativePublicationAt(%d).Exact=%t", index, row.Exact())
			if provenance, provenanceOK := row.Provenance(); provenanceOK {
				writef(&b, "result.NativePublicationAt(%d).Provenance.MountID=%s", index, provenance.MountID())
				writef(&b, "result.NativePublicationAt(%d).Provenance.PointID=%s", index, provenance.PointID())
				writef(&b, "result.NativePublicationAt(%d).Provenance.BodyID=%s", index, provenance.BodyID())
			}
		}
	}
	writeTransitions(&b, session)
	writeFindings(&b, session, identity.ContentID{}, false)
	return b.String()
}

// writeLayout names one family's declared publication columns: the row-state
// vocabulary a written row is classified under, and every column with the
// carrier its bytes are admitted over. These are the summary columns every
// answer of the family is published through.
func writeLayout(b *strings.Builder, session *Session, family schema.Key) {
	if session.plan == nil {
		writef(b, "plan.QueryResultLayout(%s).Available=false", family)
		return
	}
	layout, layoutOK := session.plan.QueryResultLayout(family)
	writef(b, "plan.QueryResultLayout(%s).Available=%t", family, layoutOK)
	if !layoutOK {
		// A family whose answers are detached through a codec other than the
		// publication plane declares no plane layout. That is the family's
		// declaration, not a missing accessor.
		return
	}
	writef(b, "plan.QueryResultLayout(%s).Digest=%s", family, layout.Digest())
	writef(b, "plan.QueryResultLayout(%s).RowWidth=%d", family, layout.RowWidth())
	for rank, state := range layout.States() {
		writef(b, "plan.QueryResultLayout(%s).States[%d]=%s", family, rank, state)
	}
	writef(b, "plan.QueryResultLayout(%s).ColumnCount=%d", family, layout.ColumnCount())
	for index := 0; index < layout.ColumnCount(); index++ {
		column, columnOK := layout.ColumnAt(index)
		if !columnOK {
			continue
		}
		writef(b, "plan.QueryResultLayout(%s).ColumnAt(%d).Key=%s", family, index, column.Key)
		writef(b, "plan.QueryResultLayout(%s).ColumnAt(%d).Carrier=%s", family, index, carrierSpelling(column.Carrier))
	}
}

// writeTransitions renders the declared activation transitions: the transport
// vector one rule's activation candidate instantiates when its route crosses a
// transition, and whether the mounted body carries that axis back out.
//
// The solved point-transition rows live on engine.CommittedProgram, which the
// compiled Plan does not publish; Gaps names that accessor.
func writeTransitions(b *strings.Builder, session *Session) {
	declared := session.declared
	rows := 0
	for position := 0; position < declared.RuleCount(); position++ {
		template, templateOK := declared.RuleAt(position)
		if !templateOK {
			continue
		}
		declaration := template.Program()
		if !declaration.Available() || declaration.TransportCount() == 0 {
			continue
		}
		key := template.Key()
		for index := 0; index < declaration.TransportCount(); index++ {
			transport, transportOK := declaration.TransportAt(index)
			if !transportOK {
				continue
			}
			rows++
			writef(b, "transition[%s].Transport[%d].Axis=%s", key, index, transport.Axis.EntryReference().Key)
			writef(b, "transition[%s].Transport[%d].Exported=%t", key, index, transport.Exported)
		}
	}
	writef(b, "transition.DeclaredTransportRows=%d", rows)
	for _, gap := range transitionGaps() {
		writef(b, "unexposed.%s=%s", gap.Layer, gap.Accessor)
	}
}

func carrierSpelling(carrier plane.Carrier) string {
	switch carrier {
	case plane.CarrierMember:
		return "CarrierMember"
	case plane.CarrierEvidence:
		return "CarrierEvidence"
	case plane.CarrierFlag:
		return "CarrierFlag"
	case plane.CarrierOrdinal:
		return "CarrierOrdinal"
	case plane.CarrierIdentity:
		return "CarrierIdentity"
	case plane.CarrierWords:
		return "CarrierWords"
	case plane.CarrierAtoms:
		return "CarrierAtoms"
	default:
		return "CarrierInvalid"
	}
}
