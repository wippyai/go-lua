package result

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"

	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// The scale law this file states: the native summary join resolves an
// occurrence through the directory it built once for the mount, never by
// rescanning the occurrence family for each joining summary.
//
// Program publishes no inverse index -- OccurrenceForID is a documented cold
// scan -- so a join that performs one such resolve per summary costs the
// product of the occurrence family and the summary family. Both are
// proportional to program size, which makes that product quadratic in program
// size and puts a large single source file past every analysis budget.
//
// The quantity the law bounds is the Program row read: deriving the row's
// catalog address and authenticating the row against its own identity. That
// is a count of operations, not an elapsed time, so the law states the same
// separation whatever else the machine is doing. The directory build reads
// each published occurrence once, each published rule occurrence once, and
// each rule's parent occurrence once; the summary join then reads nothing,
// because the directory holds no Program and can address no row. A join that
// resolved by rescanning would instead read the occurrence family once per
// summary.
const (
	nativeOccurrenceScaleSmall = 64
	nativeOccurrenceScaleLarge = 256
	// nativeOccurrenceScaleBuildReadsPerOccurrence is how many Program rows
	// the directory build reads for each published occurrence: the occurrence
	// row, the rule occurrence row published against it, and that rule's
	// parent occurrence row.
	nativeOccurrenceScaleBuildReadsPerOccurrence = 3
)

func nativeOccurrenceScaleID(t *testing.T, tag string, ordinal int) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID(
		"analysis/result/native-artifact-scale/"+tag,
		[]byte{byte(ordinal), byte(ordinal >> 8), byte(ordinal >> 16), byte(ordinal >> 24)},
	)
	if !ok {
		t.Fatalf("derive %s/%d", tag, ordinal)
	}
	return id
}

func nativeOccurrenceScaleProgram(t *testing.T, width int) programschema.Program {
	t.Helper()
	body := nativeOccurrenceScaleID(t, "body", 0)
	occurrences := make([]programschema.Occurrence, 0, width)
	inputs := make([]programschema.OccurrenceInput, 0, 2*width)
	rules := make([]programschema.RuleOccurrence, 0, width)
	summaries := make([]programschema.ArithmeticSummary, 0, width)
	for ordinal := 0; ordinal < width; ordinal++ {
		occurrence, ok := programschema.NewOccurrence(
			programschema.OccurrenceBinaryArithmetic,
			nativeOccurrenceScaleID(t, "occurrence", ordinal),
			body,
			1,
			0, 0,
			uint32(2*ordinal), 2,
			keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
		)
		if !ok {
			t.Fatalf("build occurrence %d", ordinal)
		}
		occurrences = append(occurrences, occurrence)
		for operand := 0; operand < 2; operand++ {
			input, inputOK := programschema.NewOccurrenceInput(nativeOccurrenceScaleID(t, "input", 2*ordinal+operand))
			if !inputOK {
				t.Fatalf("build occurrence input %d/%d", ordinal, operand)
			}
			inputs = append(inputs, input)
		}
		point := nativeOccurrenceScaleID(t, "point", ordinal)
		rule, ruleOK := programschema.NewRuleOccurrenceWithInputs(
			schema.Key("native-artifact-scale"),
			schema.Key("native-artifact-scale-axis"),
			uint32(ordinal),
			point,
			[]identity.ContentID{nativeOccurrenceScaleID(t, "rule-input", ordinal)},
			programissuance.StageComputation,
			programissuance.InputPreviousStage,
			programschema.RuleOccurrenceRoute{},
			true,
			programschema.RuleOccurrenceSource{},
		)
		if !ruleOK {
			t.Fatalf("build rule occurrence %d", ordinal)
		}
		rules = append(rules, rule)
		summary, summaryOK := programschema.NewArithmeticSummary(
			occurrence.ID(), body, 1,
			programschema.NumericRepresentationInteger,
			programschema.NumericRepresentationInteger,
			programschema.NumericRepresentationInteger,
			programschema.ArithmeticDivisorNone,
		)
		if !summaryOK {
			t.Fatalf("build arithmetic summary %d", ordinal)
		}
		summaries = append(summaries, summary)
	}
	schemaID := nativeOccurrenceScaleID(t, "schema", width)
	catalog, catalogOK := programcatalog.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("catalog")
	}
	frozen, sealed := (publication.Publication{
		Occurrences:         occurrences,
		OccurrenceInputs:    inputs,
		RuleOccurrences:     rules,
		ArithmeticSummaries: summaries,
	}).Seal(catalog, identity.StoreID(7))
	if !sealed {
		t.Fatal("seal publication")
	}
	program := programschema.Program{
		Frozen:      frozen,
		ArtifactID:  nativeOccurrenceScaleID(t, "artifact", width),
		ProgramID:   nativeOccurrenceScaleID(t, "program", width),
		SchemaID:    schemaID,
		EntryBodyID: body,
	}
	if !program.Available() {
		t.Fatal("program is not available")
	}
	return program
}

func TestNativeMountDirectoryAgreesWithOccurrenceForID(t *testing.T) {
	program := nativeOccurrenceScaleProgram(t, 8)
	directory, ok := newNativeMountDirectory(program)
	if !ok {
		t.Fatal("directory")
	}
	count, published := program.ArithmeticSummaryCount()
	if !published {
		t.Fatal("arithmetic summaries")
	}
	for index := 0; index < count; index++ {
		summary, summaryOK := program.ArithmeticSummaryAt(index)
		scanned, scannedOK := program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
		looked, lookedOK := directory.occurrence(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
		if !summaryOK || !scannedOK || !lookedOK || looked.ID() != scanned.ID() {
			t.Fatalf("directory occurrence %d diverged from OccurrenceForID", index)
		}
		point, pointOK := directory.computationPoint(summary.OccurrenceID())
		if !pointOK || !point.Available() {
			t.Fatalf("directory computation point %d", index)
		}
	}
}

// nativeArtifactSummaryJoinRowReads runs the native summary join over one
// program of the requested width and returns the Program rows it read while
// building the mount directory and the rows it read while joining the summary
// family to it.
func nativeArtifactSummaryJoinRowReads(t *testing.T, width int) (build, join uint64) {
	t.Helper()
	program := nativeOccurrenceScaleProgram(t, width)
	count, published := program.ArithmeticSummaryCount()
	if !published {
		t.Fatal("arithmetic summaries")
	}
	dbgNativeJoinRowReadReset()
	directory, ok := newNativeMountDirectory(program)
	if !ok {
		t.Fatalf("directory width %d", width)
	}
	build = dbgNativeJoinRowReadCount()
	for index := 0; index < count; index++ {
		summary, summaryOK := program.ArithmeticSummaryAt(index)
		occurrence, occurrenceOK := directory.occurrence(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
		_, pointOK := directory.computationPoint(summary.OccurrenceID())
		if !summaryOK || !occurrenceOK || !pointOK || summary.BodyPathID() == (identity.ContentID{}) {
			t.Fatalf("summary join refused a well-formed program of width %d", width)
		}
		if _, bodyOK := occurrence.BodyID(); !bodyOK {
			t.Fatalf("summary join lost occurrence body at width %d", width)
		}
	}
	return build, dbgNativeJoinRowReadCount() - build
}

// nativeArtifactSummaryScanRowReads resolves the same summary family without a
// directory, the way Program's own surface forces: walk the occurrence family
// until the summary's occurrence answers. It returns the Program rows that
// walk reads, which is the cost the directory exists to avoid.
func nativeArtifactSummaryScanRowReads(t *testing.T, width int) uint64 {
	t.Helper()
	program := nativeOccurrenceScaleProgram(t, width)
	summaryCount, published := program.ArithmeticSummaryCount()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !published || !occurrencesPublished {
		t.Fatal("published families")
	}
	reads := uint64(0)
	for index := 0; index < summaryCount; index++ {
		summary, summaryOK := program.ArithmeticSummaryAt(index)
		if !summaryOK {
			t.Fatalf("arithmetic summary %d", index)
		}
		resolved := false
		for ordinal := 0; ordinal < occurrenceCount && !resolved; ordinal++ {
			reads++
			row, rowOK := program.OccurrenceAt(ordinal)
			resolved = rowOK && row.Kind() == programschema.OccurrenceBinaryArithmetic && row.ID() == summary.OccurrenceID()
		}
		if !resolved {
			t.Fatalf("scan lost the occurrence of summary %d", index)
		}
	}
	return reads
}

func TestNativeArtifactSummaryJoinCostDoesNotFollowOccurrenceWidth(t *testing.T) {
	smallBuild, smallJoin := nativeArtifactSummaryJoinRowReads(t, nativeOccurrenceScaleSmall)
	largeBuild, largeJoin := nativeArtifactSummaryJoinRowReads(t, nativeOccurrenceScaleLarge)

	// The directory is built by reading each published row once. Anything
	// more is a scan nested inside the build.
	for _, width := range []struct {
		occurrences int
		reads       uint64
	}{
		{nativeOccurrenceScaleSmall, smallBuild},
		{nativeOccurrenceScaleLarge, largeBuild},
	} {
		want := uint64(nativeOccurrenceScaleBuildReadsPerOccurrence * width.occurrences)
		if width.reads != want {
			t.Fatalf(
				"mount directory over %d occurrences read %d Program rows, want exactly %d",
				width.occurrences, width.reads, want,
			)
		}
	}

	// Every summary is answered by the directory. A join that reaches back to
	// the Program for even one summary is the quadratic join this law refuses.
	if smallJoin != 0 || largeJoin != 0 {
		t.Fatalf(
			"summary join read Program rows: width %d = %d rows, width %d = %d rows, want none",
			nativeOccurrenceScaleSmall, smallJoin, nativeOccurrenceScaleLarge, largeJoin,
		)
	}

	// The join's whole read volume therefore follows the occurrence family and
	// nothing else: four times the occurrences, four times the rows.
	small, large := smallBuild+smallJoin, largeBuild+largeJoin
	if want := small * uint64(nativeOccurrenceScaleLarge/nativeOccurrenceScaleSmall); large != want {
		t.Fatalf(
			"summary-join reads grew faster than the occurrence family: width %d = %d rows, width %d = %d rows, want %d",
			nativeOccurrenceScaleSmall, small, nativeOccurrenceScaleLarge, large, want,
		)
	}

	// What the directory is worth, in the same unit. Resolving each summary by
	// walking the occurrence family reads the two families' product; the law
	// above is the separation between that and the linear count.
	smallScan, largeScan := nativeArtifactSummaryScanRowReads(t, nativeOccurrenceScaleSmall), nativeArtifactSummaryScanRowReads(t, nativeOccurrenceScaleLarge)
	for _, walked := range []struct {
		occurrences int
		reads       uint64
	}{
		{nativeOccurrenceScaleSmall, smallScan},
		{nativeOccurrenceScaleLarge, largeScan},
	} {
		width := uint64(walked.occurrences)
		if want := width * (width + 1) / 2; walked.reads != want {
			t.Fatalf(
				"scanning resolve over %d occurrences read %d Program rows, want the family product %d",
				walked.occurrences, walked.reads, want,
			)
		}
	}
	if smallScan <= small || largeScan/smallScan <= largeBuild/smallBuild {
		t.Fatalf(
			"the directory bought no separation: join %d -> %d rows, scan %d -> %d rows",
			small, large, smallScan, largeScan,
		)
	}
}
