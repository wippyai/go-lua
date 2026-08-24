package result

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"

	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

const (
	nativeOccurrenceScaleSmall          = 64
	nativeOccurrenceScaleLarge          = 256
	nativeOccurrenceScaleRounds         = 7
	nativeOccurrenceScaleLinearHeadroom = 1.5
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

func nativeArtifactSummaryJoinCost(t *testing.T, width int) time.Duration {
	t.Helper()
	program := nativeOccurrenceScaleProgram(t, width)
	count, published := program.ArithmeticSummaryCount()
	if !published {
		t.Fatal("arithmetic summaries")
	}
	best := time.Duration(0)
	for round := 0; round < nativeOccurrenceScaleRounds; round++ {
		started := time.Now()
		directory, ok := newNativeMountDirectory(program)
		if !ok {
			t.Fatalf("directory width %d", width)
		}
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
		if elapsed := time.Since(started); round == 0 || elapsed < best {
			best = elapsed
		}
	}
	return best
}

func TestNativeArtifactSummaryJoinCostDoesNotFollowOccurrenceWidth(t *testing.T) {
	small := nativeArtifactSummaryJoinCost(t, nativeOccurrenceScaleSmall)
	large := nativeArtifactSummaryJoinCost(t, nativeOccurrenceScaleLarge)
	if small <= 0 {
		t.Fatalf("summary join over width %d measured no cost", nativeOccurrenceScaleSmall)
	}
	bound := nativeOccurrenceScaleLinearHeadroom * float64(nativeOccurrenceScaleLarge) / float64(nativeOccurrenceScaleSmall)
	if grown := float64(large) / float64(small); grown > bound {
		t.Fatalf(
			"summary-join cost grew faster than the occurrence family: width %d = %s, width %d = %s, growth %.2fx, linear bound %.2fx",
			nativeOccurrenceScaleSmall, small, nativeOccurrenceScaleLarge, large, grown, bound,
		)
	}
}
