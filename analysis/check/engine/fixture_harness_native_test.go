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
	// Invalidation asserts over the epoch intervals the closure published for
	// the selected rows: where a fact's validity begins, which operation ends
	// it, and what kind of event that operation is. A row bounded by max 0 is
	// the precision half — the selected fact must survive the named event.
	Invalidation []fixtureNativeInvalidation `json:"invalidation,omitempty"`
}

// fixtureNativeSelector selects published rows. Every selector is an exact
// match against published data; there is deliberately no selector that matches
// a row merely because it exists.
type fixtureNativeSelector struct {
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
	// Trust selects the row's published proof provenance. A native code
	// generator may elide a guard only for a proven row; a claimed row is a
	// source assertion the checker could not discharge and stays guarded at
	// runtime, so the two are never interchangeable in an assertion.
	Trust string `json:"trust,omitempty"`
}

// fixtureNativeRevocation selects the epoch interval published for a row: the
// epoch its validity begins at, the epoch that ends it, and the artifact's
// occurrence kind of the ending operation. An entry with no field set selects
// any revocation.
type fixtureNativeRevocation struct {
	Established string `json:"established,omitempty"`
	Revoked     string `json:"revoked,omitempty"`
	Event       string `json:"event,omitempty"`
}

// fixtureNativeFact selects published rows and bounds their count.
type fixtureNativeFact struct {
	// Name labels the assertion in failure output. It selects nothing.
	Name string `json:"name,omitempty"`

	fixtureNativeSelector

	// RevokedBy demands that every listed revocation is published for the
	// matched rows. It also demands that every matched row carries an epoch
	// interval at all: a fact whose validity the closure never published
	// cannot be consumed speculatively, so silence fails the assertion.
	RevokedBy []fixtureNativeRevocation `json:"revoked_by,omitempty"`
	// RevokedByExhaustive demands the converse: every revocation published for
	// the matched rows is one of the listed entries, so no unnamed event ends
	// the fact.
	RevokedByExhaustive bool `json:"revoked_by_exhaustive,omitempty"`

	Min int  `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// fixtureNativeInvalidation bounds how many of the selected rows carry a
// revocation matching the revocation selector. It requires the selection to
// name at least one row with a published epoch interval, so "nothing revokes
// this" can never pass because the closure published no validity at all.
type fixtureNativeInvalidation struct {
	Name string `json:"name,omitempty"`

	fixtureNativeSelector
	fixtureNativeRevocation

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
	trust := r.Fact.Trust
	if trust == "" {
		trust = "-"
	}
	return fmt.Sprintf("%s [%s/%s subject=%s trust=%s%s] %s = %q",
		r.Module, r.Fact.Lane, r.Fact.Family, subject, trust, r.validity(), r.Fact.Key, r.Fact.Value)
}

// validity renders the published epoch interval so a revocation failure shows
// what the closure did publish, including that it published nothing.
func (r nativeFactRow) validity() string {
	switch {
	case r.Fact.Established == "":
		return " no-epoch-interval"
	case r.Fact.Revoked == "":
		return " established=" + r.Fact.Established + " never-revoked"
	case r.Fact.Event == "":
		return " established=" + r.Fact.Established + " revoked=" + r.Fact.Revoked
	default:
		return " established=" + r.Fact.Established + " revoked=" + r.Fact.Revoked + " event=" + r.Fact.Event
	}
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

func validNativeTrust(trust string) bool {
	switch trust {
	case engine.NativeTrustProven, engine.NativeTrustClaimed, engine.NativeTrustUnknown:
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
	if len(expect.Facts) == 0 && len(expect.Invalidation) == 0 && expect.MinFacts == 0 && expect.MaxFacts == nil {
		return fmt.Errorf("the native block asserts nothing: set facts, invalidation, min_facts, or max_facts")
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
	for index, invalidation := range expect.Invalidation {
		if err := validateFixtureNativeInvalidation(invalidation); err != nil {
			return fmt.Errorf("invalidation[%d]: %w", index, err)
		}
	}
	return nil
}

func validateFixtureNativeFact(exp fixtureNativeFact) error {
	if err := exp.validate(); err != nil {
		return err
	}
	if err := validateNativeBounds(exp.Min, exp.Max); err != nil {
		return err
	}
	// A required row must pin what the engine published. Asserting that some
	// row exists under a key prefix is not a specification of a fact.
	if exp.Min > 0 && !exp.assertsContent() {
		return fmt.Errorf("min %d requires an exact key or a value assertion", exp.Min)
	}
	for index, revocation := range exp.RevokedBy {
		if err := revocation.validate(); err != nil {
			return fmt.Errorf("revoked_by[%d]: %w", index, err)
		}
		if revocation.empty() {
			return fmt.Errorf("revoked_by[%d]: at least one of established, revoked or event is required", index)
		}
	}
	// A revocation set describes rows that exist. Attaching one to an assertion
	// that demands no row would report a revocation nothing has to publish.
	if len(exp.RevokedBy) > 0 && exp.Min == 0 {
		return fmt.Errorf("revoked_by requires min to be positive")
	}
	if exp.RevokedByExhaustive && len(exp.RevokedBy) == 0 {
		return fmt.Errorf("revoked_by_exhaustive requires revoked_by")
	}
	return nil
}

func validateFixtureNativeInvalidation(exp fixtureNativeInvalidation) error {
	if err := exp.fixtureNativeSelector.validate(); err != nil {
		return err
	}
	if err := exp.fixtureNativeRevocation.validate(); err != nil {
		return err
	}
	return validateNativeBounds(exp.Min, exp.Max)
}

func validateNativeBounds(min int, max *int) error {
	if min < 0 {
		return fmt.Errorf("min must be non-negative")
	}
	if max != nil && *max < 0 {
		return fmt.Errorf("max must be non-negative")
	}
	if min == 0 && max == nil {
		return fmt.Errorf("min must be positive or max must be set")
	}
	if max != nil && min > *max {
		return fmt.Errorf("min %d exceeds max %d", min, *max)
	}
	return nil
}

func (exp fixtureNativeSelector) validate() error {
	if !exp.selects() {
		return fmt.Errorf("at least one selector is required")
	}
	if exp.Lane != "" && !validNativeLane(exp.Lane) {
		return fmt.Errorf("unknown lane %q", exp.Lane)
	}
	if exp.Trust != "" && !validNativeTrust(exp.Trust) {
		return fmt.Errorf("unknown trust %q", exp.Trust)
	}
	if err := validateContains("key_contains", exp.KeyContains, false); err != nil {
		return err
	}
	return validateContains("value_contains", exp.ValueContains, false)
}

func (exp fixtureNativeRevocation) validate() error {
	if exp.Revoked != "" && exp.Established != "" && exp.Revoked == exp.Established {
		return fmt.Errorf("revoked %q cannot equal established", exp.Revoked)
	}
	return nil
}

func (exp fixtureNativeRevocation) empty() bool {
	return exp.Established == "" && exp.Revoked == "" && exp.Event == ""
}

func (exp fixtureNativeSelector) selects() bool {
	return exp.Lane != "" || exp.Module != "" || exp.Family != "" || exp.Key != "" || exp.KeyPrefix != "" ||
		exp.KeySuffix != "" || len(exp.KeyContains) > 0 || exp.Subject != "" || exp.Term != "" ||
		exp.Occurrence != "" || exp.Value != nil || exp.ValuePrefix != "" || len(exp.ValueContains) > 0 ||
		exp.Trust != ""
}

// assertsContent reports whether the selector pins published content rather
// than merely a coordinate. Proof provenance is content: it is the difference
// between a row a code generator may act on and one it may not.
func (exp fixtureNativeSelector) assertsContent() bool {
	return exp.Key != "" || exp.Value != nil || exp.ValuePrefix != "" || len(exp.ValueContains) > 0 || exp.Trust != ""
}

// selectsKey is the identity half of the selector. It is separated from the
// value half so a failed assertion can render the rows the engine did publish
// at the selected coordinate against the value the fixture demanded.
func (exp fixtureNativeSelector) selectsKey(row nativeFactRow) bool {
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

func (exp fixtureNativeSelector) selectsValue(row nativeFactRow) bool {
	value := row.Fact.Value
	if exp.Value != nil && value != *exp.Value {
		return false
	}
	if exp.ValuePrefix != "" && !strings.HasPrefix(value, exp.ValuePrefix) {
		return false
	}
	if exp.Trust != "" && row.Fact.Trust != exp.Trust {
		return false
	}
	return containsAllNativeValue(value, exp.ValueContains)
}

// Bare native selectors are atoms, not arbitrary substrings: `incomplete`
// must never satisfy an assertion about `complete` merely because the latter
// is its suffix. Structured selectors retain ordinary substring matching for
// backward-compatible opaque fact content.
func containsAllNativeValue(value string, needles []string) bool {
	for _, needle := range needles {
		if nativeBareSelector(needle) {
			if !nativeValueToken(value, needle) {
				return false
			}
			continue
		}
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}

func nativeBareSelector(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func nativeValueToken(value, token string) bool {
	for offset := 0; ; {
		index := strings.Index(value[offset:], token)
		if index < 0 {
			return false
		}
		index += offset
		// A bare selector names an atomic *value*, not a structured field name.
		// It therefore matches `state=complete` but not `complete=false` as a
		// positive complete verdict. Structured selectors such as
		// `complete=false` retain their exact substring contract above.
		before := index == 0 || value[index-1] == '=' || !nativeTokenByte(value[index-1])
		afterIndex := index + len(token)
		after := afterIndex == len(value) || !nativeTokenByte(value[afterIndex])
		// A structured boolean key is selected by its positive verdict only:
		// `exhaustive` means `exhaustive=true`, while the exact structured
		// selector `exhaustive=false` remains available for the negative fact.
		positiveBoolean := afterIndex+5 <= len(value) && value[afterIndex:afterIndex+5] == "=true"
		if before && after && (afterIndex == len(value) || value[afterIndex] != '=' || positiveBoolean) {
			return true
		}
		offset = afterIndex
	}
}

func nativeTokenByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

// selectsRevocation matches the epoch interval the closure published for a
// row. A row the closure never revoked matches no revocation selector, so a
// revocation assertion can never be satisfied by an unrevoked row.
func (exp fixtureNativeRevocation) selectsRevocation(fact engine.NativeFact) bool {
	for _, revocation := range nativeFactRevocations(fact) {
		if exp.Established != "" && revocation.Established != exp.Established {
			continue
		}
		if exp.Revoked != "" && revocation.Revoked != exp.Revoked {
			continue
		}
		if exp.Event == "" || revocation.Event == exp.Event {
			return true
		}
	}
	return false
}

func nativeFactRevocations(fact engine.NativeFact) []engine.NativeRevocation {
	if len(fact.Revocations) != 0 {
		return fact.Revocations
	}
	if fact.Revoked == "" {
		return nil
	}
	return []engine.NativeRevocation{{Established: fact.Established, Revoked: fact.Revoked, Event: fact.Event}}
}

func (exp fixtureNativeSelector) describe() []string {
	parts := make([]string, 0, 14)
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
	add("trust", exp.Trust)
	return parts
}

func (exp fixtureNativeRevocation) describe() []string {
	parts := make([]string, 0, 3)
	add := func(name, value string) {
		if value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	add("established", exp.Established)
	add("revoked", exp.Revoked)
	add("event", exp.Event)
	if len(parts) == 0 {
		return []string{"any-revocation"}
	}
	return parts
}

func describeNativeAssertion(name string, parts []string) string {
	selector := "{" + strings.Join(parts, ", ") + "}"
	if name != "" {
		return name + " " + selector
	}
	return selector
}

func describeFixtureNativeFact(exp fixtureNativeFact) string {
	parts := exp.describe()
	for _, revocation := range exp.RevokedBy {
		parts = append(parts, "revoked_by["+strings.Join(revocation.describe(), " ")+"]")
	}
	if exp.RevokedByExhaustive {
		parts = append(parts, "revoked_by_exhaustive")
	}
	return describeNativeAssertion(exp.Name, parts)
}

func describeFixtureNativeInvalidation(exp fixtureNativeInvalidation) string {
	return describeNativeAssertion(exp.Name,
		append(exp.fixtureNativeSelector.describe(), "revocation["+strings.Join(exp.fixtureNativeRevocation.describe(), " ")+"]"))
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
	for _, invalidation := range expect.Invalidation {
		misses = append(misses, nativeInvalidationExpectationMisses(invalidation, rows)...)
	}
	return misses
}

// selectRows splits the published rows into those the whole selector matched
// and those that matched its identity half, so a value or trust mismatch can
// be rendered against what the closure did publish at that coordinate.
func (exp fixtureNativeSelector) selectRows(rows []nativeFactRow) (matched, keyed []nativeFactRow) {
	for _, row := range rows {
		if !exp.selectsKey(row) {
			continue
		}
		keyed = append(keyed, row)
		if exp.selectsValue(row) {
			matched = append(matched, row)
		}
	}
	return matched, keyed
}

func nativeFactExpectationMisses(exp fixtureNativeFact, rows []nativeFactRow) []string {
	matched, keyed := exp.selectRows(rows)
	var misses []string
	if len(matched) < exp.Min {
		misses = append(misses, fmt.Sprintf("%s matched %d rows, want at least %d%s",
			describeFixtureNativeFact(exp), len(matched), exp.Min, renderNativeSamples("published at the selected coordinate", keyed)))
	}
	if exp.Max != nil && len(matched) > *exp.Max {
		misses = append(misses, fmt.Sprintf("%s matched %d rows, want at most %d%s",
			describeFixtureNativeFact(exp), len(matched), *exp.Max, renderNativeSamples("matched", matched)))
	}
	return append(misses, nativeRevocationSetMisses(exp, matched)...)
}

// nativeRevocationSetMisses checks the revocation set of the matched rows
// against the fixture's. A matched row with no published epoch interval fails
// first: a fact whose validity was never published names no deopt point, and
// treating that silence as "revoked by nothing" is exactly the unsound reading.
func nativeRevocationSetMisses(exp fixtureNativeFact, matched []nativeFactRow) []string {
	if len(exp.RevokedBy) == 0 {
		return nil
	}
	var misses []string
	for _, row := range matched {
		if row.Fact.Established == "" && len(row.Fact.Revocations) == 0 {
			misses = append(misses, fmt.Sprintf("%s matched a row with no published epoch interval; %s",
				describeFixtureNativeFact(exp), row))
		}
	}
	for _, revocation := range exp.RevokedBy {
		found := false
		for _, row := range matched {
			found = found || revocation.selectsRevocation(row.Fact)
		}
		if !found {
			misses = append(misses, fmt.Sprintf("%s publishes no revocation {%s}%s",
				describeFixtureNativeFact(exp), strings.Join(revocation.describe(), " "), renderNativeSamples("matched", matched)))
		}
	}
	if !exp.RevokedByExhaustive {
		return misses
	}
	for _, row := range matched {
		for _, actual := range nativeFactRevocations(row.Fact) {
			listed := false
			for _, revocation := range exp.RevokedBy {
				listed = listed || (revocation.Established == "" || revocation.Established == actual.Established) &&
					(revocation.Revoked == "" || revocation.Revoked == actual.Revoked) &&
					(revocation.Event == "" || revocation.Event == actual.Event)
			}
			if !listed {
				misses = append(misses, fmt.Sprintf("%s is revoked by an unlisted event; %s", describeFixtureNativeFact(exp), row))
			}
		}
	}
	return misses
}

func nativeInvalidationExpectationMisses(exp fixtureNativeInvalidation, rows []nativeFactRow) []string {
	matched, keyed := exp.selectRows(rows)
	var intervals, revoked []nativeFactRow
	for _, row := range matched {
		if row.Fact.Established == "" {
			continue
		}
		intervals = append(intervals, row)
		if exp.selectsRevocation(row.Fact) {
			revoked = append(revoked, row)
		}
	}
	// Bounding revocations of rows whose validity was never published would
	// pass on silence, which is the one reading a speculative consumer must
	// never be given.
	if len(intervals) == 0 {
		return []string{fmt.Sprintf("%s selects no row with a published epoch interval%s",
			describeFixtureNativeInvalidation(exp), renderNativeSamples("published at the selected coordinate", keyed))}
	}
	var misses []string
	if len(revoked) < exp.Min {
		misses = append(misses, fmt.Sprintf("%s matched %d revocations, want at least %d%s",
			describeFixtureNativeInvalidation(exp), len(revoked), exp.Min, renderNativeSamples("selected", intervals)))
	}
	if exp.Max != nil && len(revoked) > *exp.Max {
		misses = append(misses, fmt.Sprintf("%s matched %d revocations, want at most %d%s",
			describeFixtureNativeInvalidation(exp), len(revoked), *exp.Max, renderNativeSamples("revoked", revoked)))
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
