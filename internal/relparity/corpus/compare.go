package corpus

import "fmt"

// Class names how one fixture's two answers parted. It is the catalogue's
// grouping key: a histogram over classes is the honest statement of how far
// the new engine currently stands from the old one.
type Class string

const (
	// ClassValue is one site both engines answered, under different values.
	ClassValue Class = "value"
	// ClassOutcome is one site whose presence or admission differs.
	ClassOutcome Class = "outcome"
	// ClassDiagnostic is one site whose published diagnostic differs.
	ClassDiagnostic Class = "diagnostic"
	// ClassLineage is one site whose derivation differs.
	ClassLineage Class = "lineage"
	// ClassAbsentNew is a site the old engine published and the new did not.
	ClassAbsentNew Class = "absent-in-new"
	// ClassAbsentOld is a site the new engine published and the old did not.
	ClassAbsentOld Class = "absent-in-old"
	// ClassStatus is a whole-fixture disagreement about how the run ended,
	// including two refusals that refuse for different reasons.
	ClassStatus Class = "status"
	// ClassUnconstructed is the fixture the new engine could not be asked
	// about, because the production constructor that carries it there does
	// not exist yet. It is recorded, never skipped.
	ClassUnconstructed Class = "constructor-unavailable"
	// ClassUnreached is a fixture neither engine was asked about. It is not
	// a divergence and never enters the catalogue; it is the report facet
	// that keeps such a fixture out of the parity count.
	ClassUnreached Class = "unreached"
	// ClassTimeout is a fixture whose observation exhausted its bound.
	ClassTimeout Class = "timeout"
	// ClassProbeFailure is an observation process that produced no envelope.
	ClassProbeFailure Class = "probe-failure"
	// ClassProtocol is an envelope the driver could not open.
	ClassProtocol Class = "protocol"
)

// Divergence is one catalogued disagreement, addressed the way the report is
// read: which fixture, which published family, which query site.
//
// Family and Site are the whole-fixture wildcard "*" for a disagreement that
// is not about a single site, so every row in the catalogue has the same
// shape whatever it is about.
type Divergence struct {
	Fixture string `json:"fixture"`
	Family  string `json:"family"`
	Site    string `json:"site"`
	Class   Class  `json:"class"`
	Old     string `json:"old"`
	New     string `json:"new"`
	Rank    int    `json:"rank"`
}

// Wildcard addresses a divergence that is about the whole fixture.
const Wildcard = "*"

// String renders one divergence as the line a lane acts on.
func (divergence Divergence) String() string {
	return fmt.Sprintf("%s [%s] %s/%s\n  old: %s\n  new: %s",
		divergence.Fixture, divergence.Class, divergence.Family, divergence.Site,
		divergence.Old, divergence.New)
}

// Compare catalogues where one fixture's two answers part.
//
// Identical outcomes are parity, and that includes identical refusals: a side
// that declined has answered, and two sides declining for the same reason
// agree. An empty result is agreement over everything the comparison reached.
func Compare(envelope Envelope) []Divergence {
	old, oldHeld := envelope.Side(SideOld)
	fresh, newHeld := envelope.Side(SideNew)
	if !oldHeld || !newHeld {
		return []Divergence{{
			Fixture: envelope.Fixture, Family: Wildcard, Site: Wildcard,
			Class: ClassProtocol, Old: fmt.Sprintf("%t", oldHeld), New: fmt.Sprintf("%t", newHeld),
		}}
	}
	if old.Status != fresh.Status || old.Detail != fresh.Detail {
		class := ClassStatus
		if fresh.Status == StatusUnconstructed {
			class = ClassUnconstructed
		}
		return []Divergence{{
			Fixture: envelope.Fixture, Family: Wildcard, Site: Wildcard, Class: class,
			Old: render(old), New: render(fresh),
		}}
	}
	if old.Status != StatusSolved {
		// Both sides ended the same way for the same stated reason. That is
		// agreement; there are no rows on either side to compare.
		return nil
	}
	return compareRows(envelope.Fixture, old.Rows, fresh.Rows)
}

func render(answer Answer) string {
	if answer.Detail == "" {
		return string(answer.Status)
	}
	return string(answer.Status) + ": " + answer.Detail
}

// compareRows walks the old engine's sites in canonical order first, then the
// sites only the new engine published, so the catalogue's order is a fact
// about the two row sets rather than about map iteration.
func compareRows(fixture string, old, fresh []Row) []Divergence {
	freshByAddress := make(map[string]Row, len(fresh))
	for _, row := range fresh {
		freshByAddress[row.Address()] = row
	}
	oldByAddress := make(map[string]struct{}, len(old))
	for _, row := range old {
		oldByAddress[row.Address()] = struct{}{}
	}

	var divergences []Divergence
	for _, row := range old {
		other, held := freshByAddress[row.Address()]
		if !held {
			divergences = append(divergences, Divergence{
				Fixture: fixture, Family: row.Family, Site: row.Site,
				Class: ClassAbsentNew, Old: columns(row),
			})
			continue
		}
		for _, column := range []struct {
			class Class
			left  string
			right string
		}{
			{ClassValue, row.Value, other.Value},
			{ClassOutcome, row.Outcome, other.Outcome},
			{ClassDiagnostic, row.Diagnostic, other.Diagnostic},
			{ClassLineage, row.Lineage, other.Lineage},
		} {
			if column.left == column.right {
				continue
			}
			divergences = append(divergences, Divergence{
				Fixture: fixture, Family: row.Family, Site: row.Site,
				Class: column.class, Old: column.left, New: column.right,
			})
		}
	}
	for _, row := range fresh {
		if _, held := oldByAddress[row.Address()]; held {
			continue
		}
		divergences = append(divergences, Divergence{
			Fixture: fixture, Family: row.Family, Site: row.Site,
			Class: ClassAbsentOld, New: columns(row),
		})
	}
	return divergences
}

func columns(row Row) string {
	return fmt.Sprintf("value=%s outcome=%s diagnostic=%s lineage=%s",
		row.Value, row.Outcome, row.Diagnostic, row.Lineage)
}
