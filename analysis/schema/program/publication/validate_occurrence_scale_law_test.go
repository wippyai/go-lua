package publication

import (
	"sort"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"

	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// The law this file states: seal validation resolves an occurrence by
// identity through the directory the seal pass built, never by scanning the
// occurrence family again.
//
// Program publishes no inverse index -- resolving an occurrence by identity
// is a documented cold scan -- so a phase that performs one such resolve per
// row of a joining family costs the product of the two family widths. The
// occurrence family is proportional to program size and so are the families
// that join to it, which makes that product quadratic in program size and
// puts a large single source file past every analysis budget.
//
// A cold row read is a computation, not an allocation: it derives the catalog
// address the row is published under and authenticates the row against its
// own identity. So the counter this law bounds is the work of one validation
// phase, measured as the cost it takes to run, and the quantity it is bounded
// by is the width of the families the phase walks.
//
// The measurement runs the row phase alone, with the occurrence directory
// already built, over two programs whose occurrence and arithmetic-summary
// families differ by a factor of four. A phase that reads each published row
// once grows by that same factor: every summary authenticates itself, and
// that is the honest linear floor. A phase that resolves its occurrence join
// by rescanning the occurrence family grows by the square of it. The law is
// the separation between the two, and it is measured as the best of several
// rounds so that scheduler noise can only ever make a passing run look worse.

const (
	occurrenceScaleSmall = 64
	occurrenceScaleLarge = 256
	// occurrenceScaleRounds is how many times each phase is run. The reported
	// cost is the best round, which is the one least disturbed by anything
	// else on the machine.
	occurrenceScaleRounds = 7
	// occurrenceScaleLinearHeadroom is how far above proportional growth the
	// larger program's cost may sit. The widths differ by a factor of four,
	// so a phase that reads every published row once lands at four and this
	// admits it with half again in slack. A phase that rescans the occurrence
	// family once per summary lands at sixteen.
	occurrenceScaleLinearHeadroom = 1.5
)

func occurrenceScaleID(t *testing.T, tag string, ordinal int) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID(
		"schema/program/occurrence-scale-law/"+tag,
		[]byte{byte(ordinal), byte(ordinal >> 8), byte(ordinal >> 16), byte(ordinal >> 24)},
	)
	if !ok {
		t.Fatalf("derive %s/%d", tag, ordinal)
	}
	return id
}

// occurrenceScaleFixture seals one program whose binary-arithmetic occurrence
// family and arithmetic-summary family both have the requested width, and
// returns the validator over it together with the state its row phase reads.
func occurrenceScaleFixture(t *testing.T, width int) (*validator, *validationState) {
	t.Helper()
	body := occurrenceScaleID(t, "body", 0)
	occurrences := make([]programschema.Occurrence, 0, width)
	inputs := make([]programschema.OccurrenceInput, 0, 2*width)
	summaries := make([]programschema.ArithmeticSummary, 0, width)
	for ordinal := 0; ordinal < width; ordinal++ {
		occurrence, ok := programschema.NewOccurrence(
			programschema.OccurrenceBinaryArithmetic,
			occurrenceScaleID(t, "occurrence", ordinal),
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
			input, inputOK := programschema.NewOccurrenceInput(occurrenceScaleID(t, "input", 2*ordinal+operand))
			if !inputOK {
				t.Fatalf("build occurrence input %d/%d", ordinal, operand)
			}
			inputs = append(inputs, input)
		}
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
	// The summary family is published in strict identity order, which the row
	// phase proves as it walks it.
	sort.Slice(summaries, func(left, right int) bool {
		return contentIDBefore(summaries[left].ID(), summaries[right].ID())
	})

	schemaID := occurrenceScaleID(t, "schema", width)
	catalog, catalogOK := programcatalog.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("catalog")
	}
	frozen, sealed := (Publication{
		Occurrences:         occurrences,
		OccurrenceInputs:    inputs,
		ArithmeticSummaries: summaries,
	}).Seal(catalog, identity.StoreID(7))
	if !sealed {
		t.Fatal("seal publication")
	}
	coldState, coldOK := programstate.New(frozen, catalog)
	if !coldOK {
		t.Fatal("cold state")
	}
	lifecycleView, lifecycleOK := lifecycle.NewView(coldState)
	if !lifecycleOK {
		t.Fatal("lifecycle view")
	}
	program := programschema.Program{
		Frozen:      frozen,
		ArtifactID:  occurrenceScaleID(t, "artifact", width),
		ProgramID:   occurrenceScaleID(t, "program", width),
		SchemaID:    schemaID,
		EntryBodyID: body,
	}
	if !program.Available() {
		t.Fatal("program is not available")
	}
	validator := &validator{program: program, state: coldState, frozen: frozen, catalog: catalog, lifecycle: lifecycleView}
	state := &validationState{bodyRows: map[identity.ContentID]programschema.Body{body: {}}}
	if !validator.validateSealOccurrences(state) {
		t.Fatal("occurrence directory")
	}
	return validator, state
}

func occurrenceScaleRowPhaseCost(t *testing.T, width int) time.Duration {
	t.Helper()
	validator, state := occurrenceScaleFixture(t, width)
	best := time.Duration(0)
	for round := 0; round < occurrenceScaleRounds; round++ {
		started := time.Now()
		if !validator.validateSealRows(state) {
			t.Fatalf("row phase refused a well-formed program of width %d", width)
		}
		if elapsed := time.Since(started); round == 0 || elapsed < best {
			best = elapsed
		}
	}
	return best
}

func TestSealValidationRowPhaseCostDoesNotFollowOccurrenceWidth(t *testing.T) {
	small := occurrenceScaleRowPhaseCost(t, occurrenceScaleSmall)
	large := occurrenceScaleRowPhaseCost(t, occurrenceScaleLarge)
	if small <= 0 {
		t.Fatalf("row phase over width %d measured no cost", occurrenceScaleSmall)
	}
	bound := occurrenceScaleLinearHeadroom * float64(occurrenceScaleLarge) / float64(occurrenceScaleSmall)
	if grown := float64(large) / float64(small); grown > bound {
		t.Fatalf(
			"row-phase cost grew faster than the occurrence family: width %d = %s, width %d = %s, growth %.2fx, linear bound %.2fx",
			occurrenceScaleSmall, small, occurrenceScaleLarge, large, grown, bound,
		)
	}
}
