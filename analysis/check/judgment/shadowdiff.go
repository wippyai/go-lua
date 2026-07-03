package judgment

import (
	"fmt"
	"sort"
	"strings"
)

// ShadowRecord is the stable comparison unit used while migrating legacy
// diagnostics producers onto judgment renderers. Adapters should populate it
// from old diagnostics and from new judgments without carrying messages into
// the semantic match key.
type ShadowRecord struct {
	Code       string
	SubjectKey string
	Span       SpanRef
}

// ShadowRecordsFromJudgments converts judgments to migration comparison
// records. The first span is the primary span; judgments without spans still
// match by code and subject.
func ShadowRecordsFromJudgments(items []Judgment) []ShadowRecord {
	if len(items) == 0 {
		return nil
	}
	out := make([]ShadowRecord, 0, len(items))
	for _, item := range items {
		out = append(out, ShadowRecordFromJudgment(item))
	}
	return out
}

// ShadowRecordFromJudgment converts one semantic judgment to its shadow key.
func ShadowRecordFromJudgment(item Judgment) ShadowRecord {
	var span SpanRef
	if len(item.Spans) > 0 {
		span = item.Spans[0]
	}
	return ShadowRecord{
		Code:       string(item.Code),
		SubjectKey: item.Subject.StableKey(),
		Span:       span,
	}
}

// ShadowRecordsByCodeAndSpan drops subject identity for legacy rendered
// baselines that predate SubjectRef. Use it only at migration boundaries; new
// judgment-to-judgment comparisons should keep strict subject keys.
func ShadowRecordsByCodeAndSpan(records []ShadowRecord) []ShadowRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]ShadowRecord, len(records))
	for i, record := range records {
		record.SubjectKey = ""
		out[i] = record
	}
	return out
}

// ShadowDeltaKind classifies one shadow-diff mismatch.
type ShadowDeltaKind uint8

const (
	ShadowMissingNew ShadowDeltaKind = iota + 1
	ShadowUnexpectedNew
)

// ShadowDelta describes one unmatched old/new record.
type ShadowDelta struct {
	Kind   ShadowDeltaKind
	Record ShadowRecord
}

// AcceptedShadowDeltaRule records a reviewed migration delta class. Count is
// part of the contract: if future output adds or removes deltas under the same
// prefix, the review no longer matches and the shadow gate stays red.
type AcceptedShadowDeltaRule struct {
	Kind       ShadowDeltaKind
	Code       string
	FilePrefix string
	Count      int
	Reason     string
}

// ApplyAcceptedShadowDeltas removes reviewed deltas and reports rule-count
// mismatches. It is intentionally separate from DiffShadowRecords so semantic
// diffing stays policy-free.
func ApplyAcceptedShadowDeltas(deltas []ShadowDelta, rules []AcceptedShadowDeltaRule) ([]ShadowDelta, []string) {
	if len(deltas) == 0 || len(rules) == 0 {
		return deltas, nil
	}
	actual := make([]int, len(rules))
	remaining := make([]ShadowDelta, 0, len(deltas))
	for _, delta := range deltas {
		matched := false
		for i, rule := range rules {
			if rule.matches(delta) {
				actual[i]++
				matched = true
				break
			}
		}
		if !matched {
			remaining = append(remaining, delta)
		}
	}
	var problems []string
	for i, rule := range rules {
		if actual[i] == rule.Count {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"accepted shadow delta %s %s %q matched %d, want %d (%s)",
			shadowDeltaKindString(rule.Kind),
			rule.Code,
			rule.FilePrefix,
			actual[i],
			rule.Count,
			rule.Reason,
		))
	}
	return remaining, problems
}

func (r AcceptedShadowDeltaRule) matches(delta ShadowDelta) bool {
	if r.Kind != 0 && delta.Kind != r.Kind {
		return false
	}
	if r.Code != "" && delta.Record.Code != r.Code {
		return false
	}
	if r.FilePrefix != "" && !strings.HasPrefix(delta.Record.Span.File, r.FilePrefix) {
		return false
	}
	return true
}

func shadowDeltaKindString(kind ShadowDeltaKind) string {
	switch kind {
	case ShadowMissingNew:
		return "missing-new"
	case ShadowUnexpectedNew:
		return "unexpected-new"
	default:
		return "unknown"
	}
}

// DiffShadowRecords compares old producer output with new judgment output by
// code + subject + primary span. The result is deterministic and contains no
// policy classification; callers must review each delta as an engine precision
// change, old-diagnostics bug, or migration bug.
func DiffShadowRecords(oldRecords, newRecords []ShadowRecord) []ShadowDelta {
	oldCounts := shadowCounts(oldRecords)
	newCounts := shadowCounts(newRecords)

	keys := make([]shadowKey, 0, len(oldCounts)+len(newCounts))
	seen := make(map[shadowKey]struct{}, len(oldCounts)+len(newCounts))
	for key := range oldCounts {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range newCounts {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return shadowKeyLess(keys[i], keys[j])
	})

	var out []ShadowDelta
	for _, key := range keys {
		oldCount := oldCounts[key]
		newCount := newCounts[key]
		switch {
		case oldCount > newCount:
			for i := 0; i < oldCount-newCount; i++ {
				out = append(out, ShadowDelta{Kind: ShadowMissingNew, Record: key.record})
			}
		case newCount > oldCount:
			for i := 0; i < newCount-oldCount; i++ {
				out = append(out, ShadowDelta{Kind: ShadowUnexpectedNew, Record: key.record})
			}
		}
	}
	return out
}

type shadowKey struct {
	record ShadowRecord
}

func shadowCounts(records []ShadowRecord) map[shadowKey]int {
	counts := make(map[shadowKey]int, len(records))
	for _, record := range records {
		counts[shadowKey{record: record}]++
	}
	return counts
}

func shadowKeyLess(a, b shadowKey) bool {
	if a.record.Code != b.record.Code {
		return a.record.Code < b.record.Code
	}
	if a.record.SubjectKey != b.record.SubjectKey {
		return a.record.SubjectKey < b.record.SubjectKey
	}
	return spanLess(a.record.Span, b.record.Span)
}

func spanLess(a, b SpanRef) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	if a.StartLine != b.StartLine {
		return a.StartLine < b.StartLine
	}
	if a.StartCol != b.StartCol {
		return a.StartCol < b.StartCol
	}
	if a.EndLine != b.EndLine {
		return a.EndLine < b.EndLine
	}
	return a.EndCol < b.EndCol
}
