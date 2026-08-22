package diagnostic

import (
	"strings"
	"testing"
)

// The witness rendering laws.
//
// A judgment is often established somewhere other than the place it is
// reported at: the guard that proved a value, the write that overwrote one,
// the birth site of a table. A proof line that names such a place has to show
// it, or a reader is told a fact and left to find where it came from. These
// laws state that showing, and the silence that is its other half: a line
// established at the finding's own position repeats nothing, so every row the
// analyzer published before witnesses existed renders exactly as it did.

const witnessRenderLawFile = "main.lua"

const witnessRenderLawSource = `local typed: string = read_name()
local unrelated = 1
local narrowed = type_cast(typed)
`

// witnessRenderLawFinding is one redundant-claim finding: a primary position,
// and one located witness the row's proof line is anchored at.
func witnessRenderLawFinding(t *testing.T, fixture diagnosticTestFixture, primary, witness DiagnosticLocation) Finding {
	t.Helper()
	subject, subjectOK := NewSemanticName("typed")
	target, targetOK := NewTargetType("string")
	if !subjectOK || !targetOK {
		t.Fatal("redundant-claim payload unavailable")
	}
	report := NewReport(reportLawID(91), reportLawID(92), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	report.AppendFinding(NewFindingRow(reportLawID(93), reportLawID(94), DiagnosticCodeRedundantClaim, FindingSeverityHint, primary,
		NewTemplateData(subject, target, TypeCastClaim(), witness)))
	finding, findingOK := report.FindingAt(0)
	if !findingOK {
		t.Fatal("redundant-claim finding unavailable")
	}
	return finding
}

func witnessRenderLawLocation(t *testing.T, line, column uint32) DiagnosticLocation {
	t.Helper()
	location, ok := NewLocation(witnessRenderLawFile, line, column, 0, 0)
	if !ok {
		t.Fatalf("location %s:%d:%d unavailable", witnessRenderLawFile, line, column)
	}
	return location
}

// orderedContains reports whether every part appears in order.
func orderedContains(rendered string, parts []string) bool {
	rest := rendered
	for _, part := range parts {
		index := strings.Index(rest, part)
		if index < 0 {
			return false
		}
		rest = rest[index+len(part):]
	}
	return true
}

// TestWitnessAnchoredEvidenceShowsItsOwnPlaceLaw states that a proof line
// established at a located witness renders that place and the source there,
// immediately under the line it belongs to.
func TestWitnessAnchoredEvidenceShowsItsOwnPlaceLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	primary := witnessRenderLawLocation(t, 3, 18)
	witness := witnessRenderLawLocation(t, 1, 7)
	finding := witnessRenderLawFinding(t, fixture, primary, witness)

	rendered, ok := finding.RenderSource(witnessRenderLawFile, witnessRenderLawSource)
	if !ok {
		t.Fatal("source render unavailable")
	}
	want := []string{
		"hint[advice.redundant_claim]: type cast call is redundant; value is already string",
		"--> main.lua:3:18",
		"3 | local narrowed = type_cast(typed)",
		"because:",
		"1. proven: typed is proven to be string before the claim",
		"--> main.lua:1:7",
		"1 | local typed: string = read_name()",
		"2. proven: claim checks string at this site",
		"help: Remove the runtime type claim when the proven source type is sufficient.",
	}
	if !orderedContains(rendered, want) {
		t.Fatalf("witness-anchored render lost its ordered contract:\n%s", rendered)
	}
	// The second proof line is established at the finding's own position, so it
	// names no place of its own. Exactly two locations are shown: the finding's
	// and the witness's.
	if count := strings.Count(rendered, "--> "); count != 2 {
		t.Fatalf("render shows %d locations, want the finding's and its one witness:\n%s", count, rendered)
	}
	if strings.Contains(rendered, "where:") {
		t.Fatalf("row declaring no context witness rendered a where section:\n%s", rendered)
	}
}

// TestPrimaryAnchoredEvidenceShowsNoPlaceLaw states the silence half: a row
// whose every proof line is established at the finding's own position renders
// one location and no repetition of it. This is the shape every row published
// before witnesses existed has, so it is also the statement that adding
// witnesses moved none of them.
func TestPrimaryAnchoredEvidenceShowsNoPlaceLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	primary := witnessRenderLawLocation(t, 3, 18)
	report := NewReport(reportLawID(191), reportLawID(192), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	report.AppendFinding(NewFindingRow(reportLawID(193), reportLawID(194), DiagnosticCodeAlwaysTrueGuard, FindingSeverityHint, primary, EmptyTemplateData()))
	finding, findingOK := report.FindingAt(0)
	if !findingOK {
		t.Fatal("guard finding unavailable")
	}
	rendered, ok := finding.RenderSource(witnessRenderLawFile, witnessRenderLawSource)
	if !ok {
		t.Fatal("source render unavailable")
	}
	if count := strings.Count(rendered, "--> "); count != 1 {
		t.Fatalf("primary-anchored render shows %d locations, want only the finding's:\n%s", count, rendered)
	}
	want := []string{
		"hint[advice.always_true_guard]: condition is proven always true",
		"--> main.lua:3:18",
		"3 | local narrowed = type_cast(typed)",
		"because:",
		"1. proven: condition is proven to be true on every reachable path",
		"help: Remove the guard or move the guarded code out of the branch.",
	}
	if !orderedContains(rendered, want) {
		t.Fatalf("primary-anchored render lost its ordered contract:\n%s", rendered)
	}
}

// TestWitnessAtTheFindingPositionShowsNoPlaceLaw states that the silence is
// decided by position rather than by anchor: a witness a producer happens to
// locate at the finding's own position adds nothing to read, so nothing is
// shown for it.
func TestWitnessAtTheFindingPositionShowsNoPlaceLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	primary := witnessRenderLawLocation(t, 3, 18)
	finding := witnessRenderLawFinding(t, fixture, primary, primary)
	rendered, ok := finding.RenderSource(witnessRenderLawFile, witnessRenderLawSource)
	if !ok {
		t.Fatal("source render unavailable")
	}
	if count := strings.Count(rendered, "--> "); count != 1 {
		t.Fatalf("witness at the finding position shows %d locations, want only the finding's:\n%s", count, rendered)
	}
}

// TestRenderWithoutSourceShowsPlacesWithoutLinesLaw states that the coordinates
// a reader needs do not depend on the caller owning the source text: a render
// with no source shows every place and no source line.
func TestRenderWithoutSourceShowsPlacesWithoutLinesLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	primary := witnessRenderLawLocation(t, 3, 18)
	witness := witnessRenderLawLocation(t, 1, 7)
	rendered := witnessRenderLawFinding(t, fixture, primary, witness).Render()
	if count := strings.Count(rendered, "--> "); count != 2 {
		t.Fatalf("sourceless render shows %d locations, want the finding's and its one witness:\n%s", count, rendered)
	}
	if strings.Contains(rendered, " | ") {
		t.Fatalf("sourceless render showed a source line:\n%s", rendered)
	}
}

// TestContextSectionShowsTheSurroundingPlaceLaw states what the "where"
// section renders: the coordinates of one located witness and the source line
// there, under a heading that separates the place a judgment ranges over from
// the places its proof lines come from.
func TestContextSectionShowsTheSurroundingPlaceLaw(t *testing.T) {
	var rendered strings.Builder
	rendered.WriteString("where:\n")
	renderLocated(&rendered, witnessRenderLawLocation(t, 2, 7), witnessRenderLawSource, true)
	want := []string{"where:", "--> main.lua:2:7", "2 | local unrelated = 1"}
	if !orderedContains(rendered.String(), want) {
		t.Fatalf("context section lost its ordered contract:\n%s", rendered.String())
	}
}

// TestLocatedLineOutsideTheSourceShowsNoFrameLaw states that a coordinate the
// caller's text does not hold shows its place and no source line, rather than
// an empty numbered frame a reader would read as a blank line of program.
func TestLocatedLineOutsideTheSourceShowsNoFrameLaw(t *testing.T) {
	var rendered strings.Builder
	renderLocated(&rendered, witnessRenderLawLocation(t, 99, 1), witnessRenderLawSource, true)
	if got := rendered.String(); got != "--> main.lua:99:1\n" {
		t.Fatalf("out-of-range located line rendered %q", got)
	}
}
