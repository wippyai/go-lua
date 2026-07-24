package engine_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/lint"
)

// check.native is the fixture oracle for non-diagnostic published facts. It is
// the same assertion idiom as check.placement — minimum and maximum counts over
// a projection of one engine publication — but it is generic over fact
// families: a row selects published rows by lane, module, family, key, subject
// and value, and constrains how many the engine must publish. A new fact family
// therefore needs a new fixture, never a new harness rule.
//
// A count maximum of zero is first class: the soundness-critical half of the
// corpus asserts that a fact is withheld when its proof does not hold, and that
// adding a possible target or use never strengthens a published grant.
type fixtureNative struct {
	// MinFacts and MaxFacts bound the whole published row set.
	MinFacts int  `json:"min_facts,omitempty"`
	MaxFacts *int `json:"max_facts,omitempty"`
	// Facts are the per-family row assertions.
	Facts []fixtureNativeFact `json:"facts,omitempty"`
}

// fixtureNativeFact selects published rows and bounds their count. Every
// selector is an exact match against published data; there is deliberately no
// selector that matches a row merely because it exists.
type fixtureNativeFact struct {
	// Name labels the assertion in failure output. It selects nothing.
	Name string `json:"name,omitempty"`

	Lane        string   `json:"lane,omitempty"`
	Module      string   `json:"module,omitempty"`
	Family      string   `json:"family,omitempty"`
	Key         string   `json:"key,omitempty"`
	KeyPrefix   string   `json:"key_prefix,omitempty"`
	KeySuffix   string   `json:"key_suffix,omitempty"`
	KeyContains []string `json:"key_contains,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	Term        string   `json:"term,omitempty"`
	Occurrence  string   `json:"occurrence,omitempty"`

	Value         *string  `json:"value,omitempty"`
	ValuePrefix   string   `json:"value_prefix,omitempty"`
	ValueContains []string `json:"value_contains,omitempty"`

	Min int  `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// nativeFactRow is one published row joined to the module that published it.
type nativeFactRow struct {
	Module string
	Fact   engine.NativeFact
}

func (r nativeFactRow) String() string {
	subject := r.Fact.Subject
	if subject == "" {
		subject = "-"
	}
	return fmt.Sprintf("%s [%s/%s subject=%s] %s = %q", r.Module, r.Fact.Lane, r.Fact.Family, subject, r.Fact.Key, r.Fact.Value)
}

const nativeFailureSamples = 4

func validNativeLane(lane string) bool {
	switch lane {
	case engine.NativeLaneValues, engine.NativeLaneOutcomes, engine.NativeLaneDiagnostics:
		return true
	default:
		return false
	}
}

// parseFixtureNative decodes the block strictly. An unknown or misspelled field
// is a malformed assertion, not an assertion that silently passes.
func parseFixtureNative(raw json.RawMessage) (*fixtureNative, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	expect := &fixtureNative{}
	if err := decoder.Decode(expect); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, fmt.Errorf("trailing content after the native block")
	}
	if err := validateFixtureNative(expect); err != nil {
		return nil, err
	}
	return expect, nil
}

func validateFixtureNative(expect *fixtureNative) error {
	if len(expect.Facts) == 0 && expect.MinFacts == 0 && expect.MaxFacts == nil {
		return fmt.Errorf("the native block asserts nothing: set facts, min_facts, or max_facts")
	}
	if expect.MinFacts < 0 {
		return fmt.Errorf("min_facts must be non-negative")
	}
	if expect.MaxFacts != nil {
		if *expect.MaxFacts < 0 {
			return fmt.Errorf("max_facts must be non-negative")
		}
		if expect.MinFacts > *expect.MaxFacts {
			return fmt.Errorf("min_facts %d exceeds max_facts %d", expect.MinFacts, *expect.MaxFacts)
		}
	}
	for index, fact := range expect.Facts {
		if err := validateFixtureNativeFact(fact); err != nil {
			return fmt.Errorf("facts[%d]: %w", index, err)
		}
	}
	return nil
}

func validateFixtureNativeFact(exp fixtureNativeFact) error {
	if !exp.selects() {
		return fmt.Errorf("at least one selector is required")
	}
	if exp.Lane != "" && !validNativeLane(exp.Lane) {
		return fmt.Errorf("unknown lane %q", exp.Lane)
	}
	if err := validateContains("key_contains", exp.KeyContains, false); err != nil {
		return err
	}
	if err := validateContains("value_contains", exp.ValueContains, false); err != nil {
		return err
	}
	if exp.Min < 0 {
		return fmt.Errorf("min must be non-negative")
	}
	if exp.Max != nil && *exp.Max < 0 {
		return fmt.Errorf("max must be non-negative")
	}
	if exp.Min == 0 && exp.Max == nil {
		return fmt.Errorf("min must be positive or max must be set")
	}
	if exp.Max != nil && exp.Min > *exp.Max {
		return fmt.Errorf("min %d exceeds max %d", exp.Min, *exp.Max)
	}
	// A required row must pin what the engine published. Asserting that some
	// row exists under a key prefix is not a specification of a fact.
	if exp.Min > 0 && exp.Key == "" && exp.Value == nil && exp.ValuePrefix == "" && len(exp.ValueContains) == 0 {
		return fmt.Errorf("min %d requires an exact key or a value assertion", exp.Min)
	}
	return nil
}

func (exp fixtureNativeFact) selects() bool {
	return exp.Lane != "" || exp.Module != "" || exp.Family != "" || exp.Key != "" || exp.KeyPrefix != "" ||
		exp.KeySuffix != "" || len(exp.KeyContains) > 0 || exp.Subject != "" || exp.Term != "" ||
		exp.Occurrence != "" || exp.Value != nil || exp.ValuePrefix != "" || len(exp.ValueContains) > 0
}

// selectsKey is the identity half of the selector. It is separated from the
// value half so a failed assertion can render the rows the engine did publish
// at the selected coordinate against the value the fixture demanded.
func (exp fixtureNativeFact) selectsKey(row nativeFactRow) bool {
	fact := row.Fact
	if exp.Lane != "" && fact.Lane != exp.Lane {
		return false
	}
	if exp.Module != "" && row.Module != exp.Module {
		return false
	}
	if exp.Family != "" && fact.Family != exp.Family {
		return false
	}
	if exp.Key != "" && fact.Key != exp.Key {
		return false
	}
	if exp.KeyPrefix != "" && !strings.HasPrefix(fact.Key, exp.KeyPrefix) {
		return false
	}
	if exp.KeySuffix != "" && !strings.HasSuffix(fact.Key, exp.KeySuffix) {
		return false
	}
	if !containsAll(fact.Key, exp.KeyContains) {
		return false
	}
	if exp.Subject != "" && fact.Subject != exp.Subject {
		return false
	}
	if exp.Term != "" && fact.Term != exp.Term {
		return false
	}
	return exp.Occurrence == "" || fact.Occurrence == exp.Occurrence
}

func (exp fixtureNativeFact) selectsValue(row nativeFactRow) bool {
	value := row.Fact.Value
	if exp.Value != nil && value != *exp.Value {
		return false
	}
	if exp.ValuePrefix != "" && !strings.HasPrefix(value, exp.ValuePrefix) {
		return false
	}
	return containsAll(value, exp.ValueContains)
}

func describeFixtureNativeFact(exp fixtureNativeFact) string {
	parts := make([]string, 0, 12)
	add := func(name, value string) {
		if value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	add("lane", exp.Lane)
	add("module", exp.Module)
	add("family", exp.Family)
	add("key", exp.Key)
	add("key_prefix", exp.KeyPrefix)
	add("key_suffix", exp.KeySuffix)
	if len(exp.KeyContains) > 0 {
		add("key_contains", strings.Join(exp.KeyContains, "|"))
	}
	add("subject", exp.Subject)
	add("term", exp.Term)
	add("occurrence", exp.Occurrence)
	if exp.Value != nil {
		parts = append(parts, fmt.Sprintf("value=%q", *exp.Value))
	}
	add("value_prefix", exp.ValuePrefix)
	if len(exp.ValueContains) > 0 {
		add("value_contains", strings.Join(exp.ValueContains, "|"))
	}
	selector := strings.Join(parts, ", ")
	if exp.Name != "" {
		return fmt.Sprintf("%s {%s}", exp.Name, selector)
	}
	return "{" + selector + "}"
}

// fixtureNativeRows joins every module's published fact index. A checked module
// whose body was rejected before publication has no index; that is reported
// rather than read as an empty publication, so a withholding assertion cannot
// pass because nothing was analysed.
func fixtureNativeRows(result lint.ProjectResult) ([]nativeFactRow, []string) {
	var rows []nativeFactRow
	var failures []string
	for _, entry := range result.Entries {
		if entry.Engine.Native == nil {
			failures = append(failures, fmt.Sprintf("module %s published no fact index", entry.Entry.ModulePath))
			continue
		}
		for _, fact := range entry.Engine.Native.Facts() {
			rows = append(rows, nativeFactRow{Module: entry.Entry.ModulePath, Fact: fact})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Module != rows[j].Module {
			return rows[i].Module < rows[j].Module
		}
		if rows[i].Fact.Lane != rows[j].Fact.Lane {
			return rows[i].Fact.Lane < rows[j].Fact.Lane
		}
		if rows[i].Fact.Key != rows[j].Fact.Key {
			return rows[i].Fact.Key < rows[j].Fact.Key
		}
		return rows[i].Fact.Value < rows[j].Fact.Value
	})
	return rows, failures
}

func nativeExpectationMisses(raw json.RawMessage, rows []nativeFactRow, indexFailures []string) []string {
	expect, err := parseFixtureNative(raw)
	if err != nil {
		return []string{"malformed native block: " + err.Error()}
	}
	if expect == nil {
		return nil
	}
	misses := append([]string(nil), indexFailures...)
	if len(rows) < expect.MinFacts {
		misses = append(misses, fmt.Sprintf("published facts = %d, want at least %d", len(rows), expect.MinFacts))
	}
	if expect.MaxFacts != nil && len(rows) > *expect.MaxFacts {
		misses = append(misses, fmt.Sprintf("published facts = %d, want at most %d", len(rows), *expect.MaxFacts))
	}
	for _, fact := range expect.Facts {
		misses = append(misses, nativeFactExpectationMisses(fact, rows)...)
	}
	return misses
}

func nativeFactExpectationMisses(exp fixtureNativeFact, rows []nativeFactRow) []string {
	var matched, keyed []nativeFactRow
	for _, row := range rows {
		if !exp.selectsKey(row) {
			continue
		}
		keyed = append(keyed, row)
		if exp.selectsValue(row) {
			matched = append(matched, row)
		}
	}
	var misses []string
	if len(matched) < exp.Min {
		misses = append(misses, fmt.Sprintf("%s matched %d rows, want at least %d%s",
			describeFixtureNativeFact(exp), len(matched), exp.Min, renderNativeSamples("published at the selected coordinate", keyed)))
	}
	if exp.Max != nil && len(matched) > *exp.Max {
		misses = append(misses, fmt.Sprintf("%s matched %d rows, want at most %d%s",
			describeFixtureNativeFact(exp), len(matched), *exp.Max, renderNativeSamples("matched", matched)))
	}
	return misses
}

func renderNativeSamples(label string, rows []nativeFactRow) string {
	if len(rows) == 0 {
		return "; nothing " + label
	}
	sample := rows
	suffix := ""
	if len(sample) > nativeFailureSamples {
		sample, suffix = sample[:nativeFailureSamples], fmt.Sprintf(" (+%d more)", len(rows)-nativeFailureSamples)
	}
	parts := make([]string, 0, len(sample))
	for _, row := range sample {
		parts = append(parts, row.String())
	}
	return "; " + label + ": " + strings.Join(parts, " | ") + suffix
}
