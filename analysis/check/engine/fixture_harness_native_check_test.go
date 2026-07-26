package engine_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/lint"
)

func nativeAssertionRows() []nativeFactRow {
	return []nativeFactRow{
		{Module: "main", Fact: engine.NativeFact{
			Lane: engine.NativeLaneValues, Family: "heap",
			Key: factkey.BuildKey(
				factkey.HeapTableClosed, []factkey.Part{factkey.IdentityPart([]byte("heap"))}, "op-00000003",
			).String(),
			Value: "closed", Occurrence: "op-00000003", Trust: engine.NativeTrustProven,
		}},
		{Module: "main", Fact: engine.NativeFact{Lane: engine.NativeLaneValues, Family: "value", Key: "value/path/sym2/op-00000003", Value: "scalar/number/7", Term: "path/sym2", Subject: "total", Occurrence: "op-00000003", Trust: engine.NativeTrustProven}},
		{Module: "main", Fact: engine.NativeFact{Lane: engine.NativeLaneValues, Family: "value", Key: "value/path/sym5/op-00000004", Value: "scalar/number/9", Term: "path/sym5", Subject: "scaled", Occurrence: "op-00000004", Trust: engine.NativeTrustProven}},
		{Module: "producer", Fact: engine.NativeFact{Lane: engine.NativeLaneOutcomes, Family: "return", Key: "return/arity", Value: "1"}},
	}
}

// nativeValidityRows carry the epoch intervals the closure publishes: one
// binding replaced by a later write, one binding never replaced, one claimed
// value, and one row whose validity was never published at all.
func nativeValidityRows() []nativeFactRow {
	return []nativeFactRow{
		{Module: "main", Fact: engine.NativeFact{
			Lane: engine.NativeLaneValues, Family: "value", Key: "value/path/sym2/op-00000004", Value: "scalar/number/0",
			Term: "path/sym2", Subject: "total", Occurrence: "op-00000004", Trust: engine.NativeTrustProven,
			Established: "op-00000004", Revoked: "op-00000009", Event: "environment-write"}},
		{Module: "main", Fact: engine.NativeFact{
			Lane: engine.NativeLaneValues, Family: "value", Key: "value/path/sym2/op-00000009", Value: "scalar/claim/claim-kind/1/claim-type/\"number\"",
			Term: "path/sym2", Subject: "total", Occurrence: "op-00000009", Trust: engine.NativeTrustClaimed,
			Established: "op-00000009"}},
		{Module: "main", Fact: engine.NativeFact{
			Lane: engine.NativeLaneValues, Family: "value", Key: "value/path/sym6/op-00000006", Value: "scalar/number/3",
			Term: "path/sym6", Subject: "limit", Occurrence: "op-00000006", Trust: engine.NativeTrustProven,
			Established: "op-00000006"}},
		{Module: "main", Fact: engine.NativeFact{
			Lane: engine.NativeLaneValues, Family: "cast-target", Key: "cast-target/path/sym8/op-00000008", Value: "shape/target/v1/AA",
			Term: "path/sym8", Subject: "checked", Occurrence: "op-00000008", Trust: engine.NativeTrustProven}},
	}
}

func TestFixtureNativeRejectsMalformedBlocks(t *testing.T) {
	for _, item := range []struct{ name, block, contains string }{
		{"unknown field", `{"min_fact": 1}`, "unknown field"},
		{"unknown row field", `{"facts": [{"familly": "heap", "min": 1}]}`, "unknown field"},
		{"not an object", `[]`, "cannot unmarshal"},
		{"asserts nothing", `{}`, "asserts nothing"},
		{"empty facts list", `{"facts": []}`, "asserts nothing"},
		{"row without a selector", `{"facts": [{"name": "x", "min": 1}]}`, "at least one selector"},
		{"row without a bound", `{"facts": [{"family": "heap"}]}`, "min must be positive or max must be set"},
		{"negative min", `{"facts": [{"family": "heap", "min": -1}]}`, "min must be non-negative"},
		{"negative max", `{"facts": [{"family": "heap", "max": -1}]}`, "max must be non-negative"},
		{"min above max", `{"facts": [{"key": "k", "min": 3, "max": 2}]}`, "min 3 exceeds max 2"},
		{"unknown lane", `{"facts": [{"lane": "guesses", "key": "k", "min": 1}]}`, `unknown lane "guesses"`},
		{"empty key assertion", `{"facts": [{"key_contains": [""], "min": 1}]}`, "key_contains contains an empty assertion"},
		{"empty value assertion", `{"facts": [{"value_contains": [""], "min": 1}]}`, "value_contains contains an empty assertion"},
		{"required row without content", `{"facts": [{"key_prefix": "heap/", "min": 1}]}`, "requires an exact key or a value assertion"},
		{"negative min_facts", `{"min_facts": -1}`, "min_facts must be non-negative"},
		{"negative max_facts", `{"max_facts": -1}`, "max_facts must be non-negative"},
		{"min_facts above max_facts", `{"min_facts": 5, "max_facts": 2}`, "min_facts 5 exceeds max_facts 2"},
		{"unknown trust", `{"facts": [{"trust": "hopeful", "min": 1}]}`, `unknown trust "hopeful"`},
		{"empty revocation entry", `{"facts": [{"key": "k", "min": 1, "revoked_by": [{}]}]}`, "at least one of established, revoked or event is required"},
		{"revocation of a withheld row", `{"facts": [{"key": "k", "max": 0, "revoked_by": [{"event": "apply"}]}]}`, "revoked_by requires min to be positive"},
		{"exhaustive without a set", `{"facts": [{"key": "k", "min": 1, "revoked_by_exhaustive": true}]}`, "revoked_by_exhaustive requires revoked_by"},
		{"revoked equal to established", `{"facts": [{"key": "k", "min": 1, "revoked_by": [{"established": "op-1", "revoked": "op-1"}]}]}`, `revoked "op-1" cannot equal established`},
		{"unknown invalidation field", `{"invalidation": [{"famly": "value", "max": 0}]}`, "unknown field"},
		{"invalidation without a selector", `{"invalidation": [{"name": "x", "max": 0}]}`, "at least one selector"},
		{"invalidation without a bound", `{"invalidation": [{"family": "value"}]}`, "min must be positive or max must be set"},
		{"invalidation min above max", `{"invalidation": [{"family": "value", "min": 3, "max": 2}]}`, "min 3 exceeds max 2"},
		{"invalidation with a degenerate revocation", `{"invalidation": [{"family": "value", "established": "op-1", "revoked": "op-1", "min": 1}]}`, `revoked "op-1" cannot equal established`},
	} {
		t.Run(item.name, func(t *testing.T) {
			misses := nativeExpectationMisses(json.RawMessage(item.block), nativeAssertionRows(), nil)
			if len(misses) != 1 || !strings.HasPrefix(misses[0], "malformed native block: ") || !strings.Contains(misses[0], item.contains) {
				t.Fatalf("misses = %#v, want one malformed report containing %q", misses, item.contains)
			}
		})
	}
}

func TestFixtureNativeAcceptsWellFormedAssertions(t *testing.T) {
	block := `{
		"min_facts": 4,
		"max_facts": 4,
		"facts": [
			{"name": "closed table", "family": "heap", "key_prefix": "heap/table-closed/", "value": "closed", "min": 1, "max": 1},
			{"name": "constant at a named subject", "lane": "values", "subject": "total", "value": "scalar/number/7", "min": 1, "max": 1},
			{"name": "anchored at an occurrence", "occurrence": "op-00000004", "value_prefix": "scalar/", "min": 1, "max": 1},
			{"name": "published by one module", "module": "producer", "key": "return/arity", "value": "1", "min": 1, "max": 1},
			{"name": "no diagnostic lane row", "lane": "diagnostics", "max": 0},
			{"name": "no sealed identity", "key_prefix": "heap/table-identity/", "max": 0}
		]
	}`
	if misses := nativeExpectationMisses(json.RawMessage(block), nativeAssertionRows(), nil); len(misses) != 0 {
		t.Fatalf("misses = %#v, want none", misses)
	}
}

func TestFixtureNativeRendersExpectedAgainstActualOnAValueMismatch(t *testing.T) {
	block := `{"facts": [{"name": "constant", "lane": "values", "subject": "total", "value": "scalar/number/8", "min": 1}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeAssertionRows(), nil)
	if len(misses) != 1 {
		t.Fatalf("misses = %#v, want one", misses)
	}
	for _, want := range []string{
		"constant {lane=values, subject=total, value=\"scalar/number/8\"}",
		"matched 0 rows, want at least 1",
		"published at the selected coordinate",
		"main [values/value subject=total trust=proven no-epoch-interval] value/path/sym2/op-00000003 = \"scalar/number/7\"",
	} {
		if !strings.Contains(misses[0], want) {
			t.Fatalf("miss %q does not report %q", misses[0], want)
		}
	}
}

func TestFixtureNativeRendersTheSelectorWhenNothingWasPublishedForIt(t *testing.T) {
	block := `{"facts": [{"family": "closure", "value": "{}", "min": 1}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeAssertionRows(), nil)
	if len(misses) != 1 || !strings.Contains(misses[0], "nothing published at the selected coordinate") {
		t.Fatalf("misses = %#v, want one report that the selector matched no published row", misses)
	}
}

func TestFixtureNativeRendersTheOffendingRowsWhenAWithholdingAssertionFails(t *testing.T) {
	block := `{"facts": [{"name": "no constants", "lane": "values", "value_prefix": "scalar/", "max": 0}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeAssertionRows(), nil)
	if len(misses) != 1 {
		t.Fatalf("misses = %#v, want one", misses)
	}
	for _, want := range []string{
		"matched 2 rows, want at most 0",
		"main [values/value subject=total trust=proven no-epoch-interval] value/path/sym2/op-00000003 = \"scalar/number/7\"",
		"main [values/value subject=scaled trust=proven no-epoch-interval] value/path/sym5/op-00000004 = \"scalar/number/9\"",
	} {
		if !strings.Contains(misses[0], want) {
			t.Fatalf("miss %q does not report %q", misses[0], want)
		}
	}
}

func TestFixtureNativeAcceptsRevocationAndSurvivalAssertions(t *testing.T) {
	block := `{
		"facts": [
			{"name": "the replaced constant names its own deopt point", "subject": "total", "value": "scalar/number/0", "min": 1, "max": 1,
			 "revoked_by": [{"revoked": "op-00000009", "event": "environment-write"}], "revoked_by_exhaustive": true},
			{"name": "the surviving constant is revoked by nothing", "subject": "limit", "value": "scalar/number/3", "min": 1, "max": 1},
			{"name": "the undischarged cast reaches the binding claimed", "subject": "total", "occurrence": "op-00000009", "trust": "claimed", "min": 1, "max": 1},
			{"name": "and never as a proof", "subject": "total", "occurrence": "op-00000009", "trust": "proven", "max": 0}
		],
		"invalidation": [
			{"name": "exactly one value of the accumulator is replaced", "subject": "total", "family": "value", "event": "environment-write", "min": 1, "max": 1},
			{"name": "the loop bound survives the write to the accumulator", "subject": "limit", "family": "value", "max": 0}
		]
	}`
	if misses := nativeExpectationMisses(json.RawMessage(block), nativeValidityRows(), nil); len(misses) != 0 {
		t.Fatalf("misses = %#v, want none", misses)
	}
}

// A fact whose validity the closure never published names no deopt point. It
// must fail a revocation assertion rather than read as "revoked by nothing".
func TestFixtureNativeRejectsRevocationAssertionsOnRowsWithoutAnEpochInterval(t *testing.T) {
	block := `{"facts": [{"name": "cast witness", "family": "cast-target", "subject": "checked", "value": "shape/target/v1/AA", "min": 1,
		"revoked_by": [{"event": "apply"}]}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeValidityRows(), nil)
	if len(misses) == 0 {
		t.Fatal("misses = none, want the missing epoch interval reported")
	}
	if !strings.Contains(misses[0], "matched a row with no published epoch interval") || !strings.Contains(misses[0], "no-epoch-interval") {
		t.Fatalf("miss %q does not report the row's missing interval", misses[0])
	}
}

// The precision half must be fail-closed for the same reason: "nothing revokes
// this" is only meaningful about a fact whose validity interval exists.
func TestFixtureNativeRejectsASurvivalAssertionOverAnUnpublishedInterval(t *testing.T) {
	block := `{"invalidation": [{"name": "the cast witness survives", "family": "cast-target", "subject": "checked", "max": 0}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeValidityRows(), nil)
	if len(misses) != 1 || !strings.Contains(misses[0], "selects no row with a published epoch interval") {
		t.Fatalf("misses = %#v, want the empty interval selection reported", misses)
	}
}

func TestFixtureNativeReportsAnUnlistedRevokingEvent(t *testing.T) {
	block := `{"facts": [{"name": "accumulator", "family": "value", "subject": "total", "value_prefix": "scalar/", "min": 2,
		"revoked_by": [{"event": "apply"}], "revoked_by_exhaustive": true}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeValidityRows(), nil)
	if len(misses) != 2 {
		t.Fatalf("misses = %#v, want the absent revocation and the unlisted one", misses)
	}
	for index, want := range []string{
		`publishes no revocation {event=apply}`,
		"is revoked by an unlisted event",
	} {
		if !strings.Contains(misses[index], want) {
			t.Fatalf("miss %q does not report %q", misses[index], want)
		}
	}
}

func TestFixtureNativeReportsAMissingRevocationCount(t *testing.T) {
	block := `{"invalidation": [{"name": "the accumulator is replaced twice", "family": "value", "subject": "total", "min": 2}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeValidityRows(), nil)
	if len(misses) != 1 {
		t.Fatalf("misses = %#v, want one", misses)
	}
	for _, want := range []string{
		"the accumulator is replaced twice {family=value, subject=total, revocation[any-revocation]}",
		"matched 1 revocations, want at least 2",
		"established=op-00000009 never-revoked",
	} {
		if !strings.Contains(misses[0], want) {
			t.Fatalf("miss %q does not report %q", misses[0], want)
		}
	}
}

// Trust is a value assertion: it pins whether a code generator may act on the
// row, so a row selected only by coordinate plus trust is a real specification.
func TestFixtureNativeReportsATrustMismatch(t *testing.T) {
	block := `{"facts": [{"name": "claimed accumulator", "family": "value", "subject": "total", "occurrence": "op-00000004", "trust": "claimed", "min": 1}]}`
	misses := nativeExpectationMisses(json.RawMessage(block), nativeValidityRows(), nil)
	if len(misses) != 1 {
		t.Fatalf("misses = %#v, want one", misses)
	}
	for _, want := range []string{"trust=claimed", "matched 0 rows, want at least 1", "trust=proven"} {
		if !strings.Contains(misses[0], want) {
			t.Fatalf("miss %q does not report %q", misses[0], want)
		}
	}
}

func TestFixtureNativeBoundsThePublishedRowTotal(t *testing.T) {
	rows := nativeAssertionRows()
	if misses := nativeExpectationMisses(json.RawMessage(`{"min_facts": 5}`), rows, nil); len(misses) != 1 || !strings.Contains(misses[0], "published facts = 4, want at least 5") {
		t.Fatalf("min_facts misses = %#v", misses)
	}
	if misses := nativeExpectationMisses(json.RawMessage(`{"max_facts": 3}`), rows, nil); len(misses) != 1 || !strings.Contains(misses[0], "published facts = 4, want at most 3") {
		t.Fatalf("max_facts misses = %#v", misses)
	}
}

// A withholding assertion must not pass because the module was never analysed.
func TestFixtureNativeReportsAModuleThatPublishedNoIndex(t *testing.T) {
	result := lint.ProjectResult{Entries: []lint.EntryResult{{Entry: lint.Entry{ModulePath: "main"}}}}
	rows, failures := fixtureNativeRows(result)
	if len(rows) != 0 || len(failures) != 1 || !strings.Contains(failures[0], "module main published no fact index") {
		t.Fatalf("rows = %#v, failures = %#v, want the missing index reported", rows, failures)
	}
	misses := nativeExpectationMisses(json.RawMessage(`{"facts": [{"key_prefix": "heap/", "max": 0}]}`), rows, failures)
	if len(misses) != 1 || !strings.Contains(misses[0], "module main published no fact index") {
		t.Fatalf("misses = %#v, want the missing index to fail the assertion", misses)
	}
}

func TestFixtureNativeAbsentBlockAssertsNothing(t *testing.T) {
	if misses := nativeExpectationMisses(nil, nativeAssertionRows(), nil); misses != nil {
		t.Fatalf("misses = %#v, want none for an absent block", misses)
	}
}

// The oracle's native rows must be a projection of one checking run, in a
// stable order, so an assertion is reproducible.
func TestFixtureNativeRowsAreStableAcrossRuns(t *testing.T) {
	suite := namedSuite{
		Name: "native/published-facts-sealed-record",
		Dir:  corpusRepositoryRoot(t) + "/testdata/fixtures/native/published-facts-sealed-record",
	}
	first, err := fixtureDiagnostics(suite)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := fixtureDiagnostics(suite)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(first.NativeFailures)+len(second.NativeFailures) != 0 {
		t.Fatalf("index failures %#v and %#v, want none", first.NativeFailures, second.NativeFailures)
	}
	if len(first.Native) == 0 || len(first.Native) != len(second.Native) {
		t.Fatalf("row counts %d and %d, want an equal non-empty projection", len(first.Native), len(second.Native))
	}
	for index := range first.Native {
		if !reflect.DeepEqual(first.Native[index], second.Native[index]) {
			t.Fatalf("row %d = %s and %s differ between runs", index, first.Native[index], second.Native[index])
		}
	}
}
