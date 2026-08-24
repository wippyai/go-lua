package inspect

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// CellView opens the published answer of one query as an immutable reading
// over the payload the solve detached. It is the only path this package takes
// into a cell: plane.Admit is the codec the encoder wrote through, so the
// inspector holds no second declaration of the wire and materializes no row.
//
// The second result is the codec's own refusal name for a payload it did not
// admit; the third is whether the query carries a cell at all.
func (session *Session) CellView(familyIndex, queryIndex int) (plane.View, plane.Refusal, bool) {
	if session == nil || session.result == nil || session.plan == nil {
		return plane.View{}, plane.RefusalNone, false
	}
	family, familyOK := session.result.FamilyAt(familyIndex)
	if !familyOK {
		return plane.View{}, plane.RefusalNone, false
	}
	query, queryOK := family.QueryAt(queryIndex)
	if !queryOK {
		return plane.View{}, plane.RefusalNone, false
	}
	cell, cellOK := query.Cell()
	if !cellOK {
		return plane.View{}, plane.RefusalNone, false
	}
	layout, layoutOK := session.plan.QueryResultLayout(family.Key())
	if !layoutOK {
		return plane.View{}, plane.RefusalLayout, false
	}
	view, refusal := plane.Admit(layout, cell.Present(), cell.RowCount(), cell.Payload())
	if refusal.Available() {
		return plane.View{}, refusal, true
	}
	return view, plane.RefusalNone, true
}

// CellRow resolves one coordinate of a published answer. The coordinate plane
// ascends, so the lookup is a bisection over the admitted payload and reads
// nothing into the heap.
func (session *Session) CellRow(familyIndex, queryIndex int, coordinate identity.ContentID) (plane.Row, bool) {
	view, refusal, ok := session.CellView(familyIndex, queryIndex)
	if !ok || refusal.Available() {
		return plane.Row{}, false
	}
	return view.Lookup(coordinate)
}

// writeCell renders one query's whole solved answer under the accessor names
// that produced it.
func writeCell(b *strings.Builder, session *Session, prefix string, familyIndex, queryIndex int) {
	solved := session.result
	if solved == nil {
		writef(b, "%s.Cell=unavailable", prefix)
		return
	}
	family, familyOK := solved.FamilyAt(familyIndex)
	if !familyOK {
		writef(b, "%s.Cell=unavailable", prefix)
		return
	}
	query, queryOK := family.QueryAt(queryIndex)
	if !queryOK {
		writef(b, "%s.Cell=unavailable", prefix)
		return
	}
	writef(b, "%s.Status=%s", prefix, queryStatusSpelling(query.Status()))
	cell, cellOK := query.Cell()
	if !cellOK {
		writef(b, "%s.Cell=unavailable", prefix)
		return
	}
	writef(b, "%s.Cell.Present=%t", prefix, cell.Present())
	writef(b, "%s.Cell.RowCount=%d", prefix, cell.RowCount())
	writef(b, "%s.Cell.ContentID=%s", prefix, cell.ContentID())
	writef(b, "%s.Cell.ContractID=%s", prefix, cell.ContractID())

	layout, layoutOK := session.plan.QueryResultLayout(family.Key())
	if !layoutOK {
		writef(b, "%s.QueryResultLayout=unavailable", prefix)
		return
	}
	writef(b, "%s.QueryResultLayout.Digest=%s", prefix, layout.Digest())
	view, refusal, viewOK := session.CellView(familyIndex, queryIndex)
	if !viewOK {
		writef(b, "%s.plane.Admit=unavailable", prefix)
		return
	}
	if refusal.Available() {
		writef(b, "%s.plane.Admit.Refusal=%s", prefix, refusal)
		return
	}
	writef(b, "%s.plane.Admit.Refusal=%s", prefix, plane.RefusalNone)
	writef(b, "%s.plane.View.Owner=%s", prefix, view.Owner())
	writef(b, "%s.plane.View.RowCount=%d", prefix, view.RowCount())
	for rowIndex := 0; rowIndex < view.RowCount(); rowIndex++ {
		row, rowOK := view.At(rowIndex)
		if !rowOK {
			continue
		}
		writeRow(b, layout, prefix, rowIndex, row)
	}
}

// writeRow renders one coordinate row: its class, then every declared column
// under the carrier it was admitted over.
func writeRow(b *strings.Builder, layout *plane.Sealed, prefix string, rowIndex int, row plane.Row) {
	writef(b, "%s.plane.View.At(%d).ID=%s", prefix, rowIndex, row.ID())
	writef(b, "%s.plane.View.At(%d).Written=%t", prefix, rowIndex, row.Written())
	class, classOK := row.Class()
	if classOK {
		writef(b, "%s.plane.View.At(%d).Class=%s", prefix, rowIndex, class)
	} else {
		writef(b, "%s.plane.View.At(%d).Class=unwritten", prefix, rowIndex)
	}
	for column := 0; column < layout.ColumnCount(); column++ {
		declared, declaredOK := layout.ColumnAt(column)
		if !declaredOK {
			continue
		}
		name := prefix + ".plane.View.At(" + decimal(uint64(rowIndex)) + ")." + string(declared.Key)
		switch declared.Carrier {
		case plane.CarrierMember:
			if member, decided := row.Member(column); decided {
				writef(b, "%s.Member=%s", name, member)
			} else {
				writef(b, "%s.Member=undecided", name)
			}
		case plane.CarrierEvidence:
			writef(b, "%s.Evidence=%s", name, evidenceSpelling(row.Evidence(column)))
		case plane.CarrierFlag:
			writef(b, "%s.Flag=%t", name, row.Flag(column))
		case plane.CarrierOrdinal:
			if value, present := row.Ordinal(column); present {
				writef(b, "%s.Ordinal=%d", name, value)
			} else {
				writef(b, "%s.Ordinal=absent", name)
			}
		case plane.CarrierIdentity:
			if value, present := row.Identity(column); present {
				writef(b, "%s.Identity=%s", name, value)
			} else {
				writef(b, "%s.Identity=absent", name)
			}
		case plane.CarrierWords:
			writef(b, "%s.Count=%d", name, row.Count())
			for index := 0; index < row.Count(); index++ {
				if word, wordOK := row.WordAt(index); wordOK {
					writef(b, "%s.WordAt(%d)=%d", name, index, word)
				}
			}
		case plane.CarrierAtoms:
			writef(b, "%s.Count=%d", name, row.Count())
			for index := 0; index < row.Count(); index++ {
				if atom, atomOK := row.AtomAt(index); atomOK {
					writef(b, "%s.AtomAt(%d)=%s", name, index, atom)
				}
			}
		}
	}
}

func evidenceSpelling(state plane.Evidence) string {
	switch state {
	case plane.EvidenceAbsent:
		return "EvidenceAbsent"
	case plane.EvidenceUnknown:
		return "EvidenceUnknown"
	case plane.EvidenceRefuted:
		return "EvidenceRefuted"
	case plane.EvidenceProven:
		return "EvidenceProven"
	default:
		return "EvidenceInvalid"
	}
}

func queryStatusSpelling(status result.QueryStatus) string {
	switch status {
	case result.QueryHit:
		return "QueryHit"
	case result.QueryProvenAbsent:
		return "QueryProvenAbsent"
	default:
		return "QueryInvalid"
	}
}
