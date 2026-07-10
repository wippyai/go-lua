package diffreport

import (
	"reflect"
	"sort"
	"strings"
)

// Report is the classified delta between a baseline diagnostic snapshot and a
// current diagnostic snapshot.
type Report struct {
	New     []Record
	Removed []Record
	Changed []Change
}

// Change is a same-identity diagnostic whose visible payload changed.
type Change struct {
	Baseline Record
	Current  Record
}

// Empty reports whether there are no diagnostic deltas.
func (r Report) Empty() bool {
	return len(r.New) == 0 && len(r.Removed) == 0 && len(r.Changed) == 0
}

// Compare classifies diagnostics that are new, removed, or changed at the same
// identity. It matches exact identities first, then pairs remaining records by
// scope/file/code/message while ignoring spans so pure line-number drift is not
// reported as new plus removed.
func Compare(baseline, current []Record) Report {
	base := diagnosticRecords(baseline)
	cur := diagnosticRecords(current)
	sortRecords(base)
	sortRecords(cur)

	baseMatched := make([]bool, len(base))
	curMatched := make([]bool, len(cur))
	var report Report

	matchAnchored(base, cur, baseMatched, curMatched, &report)
	matchLegacyExact(base, cur, baseMatched, curMatched, &report)
	matchLegacyLineDrift(base, cur, baseMatched, curMatched, &report)

	for i, record := range base {
		if !baseMatched[i] {
			report.Removed = append(report.Removed, record)
		}
	}
	for i, record := range cur {
		if !curMatched[i] {
			report.New = append(report.New, record)
		}
	}
	sortReport(&report)
	return report
}

func diagnosticRecords(records []Record) []Record {
	out := make([]Record, 0, len(records))
	for _, record := range records {
		if diagnosticRecord(record) {
			out = append(out, record)
		}
	}
	return out
}

func matchAnchored(base, cur []Record, baseMatched, curMatched []bool, report *Report) {
	baseBuckets := make(map[anchoredIdentity][]int)
	curBuckets := make(map[anchoredIdentity][]int)
	for i, record := range base {
		if key, ok := anchoredKey(record); ok {
			baseBuckets[key] = append(baseBuckets[key], i)
		}
	}
	for i, record := range cur {
		if key, ok := anchoredKey(record); ok {
			curBuckets[key] = append(curBuckets[key], i)
		}
	}
	for _, key := range sortedAnchoredKeys(baseBuckets, curBuckets) {
		baseIndexes := baseBuckets[key]
		curIndexes := curBuckets[key]
		n := min(len(baseIndexes), len(curIndexes))
		for i := 0; i < n; i++ {
			baseIndex := baseIndexes[i]
			curIndex := curIndexes[i]
			baseMatched[baseIndex] = true
			curMatched[curIndex] = true
			if !recordsEquivalentIgnoringSpans(base[baseIndex], cur[curIndex]) {
				report.Changed = append(report.Changed, Change{
					Baseline: base[baseIndex],
					Current:  cur[curIndex],
				})
			}
		}
	}
}

func matchLegacyExact(base, cur []Record, baseMatched, curMatched []bool, report *Report) {
	baseBuckets := make(map[Identity][]int)
	curBuckets := make(map[Identity][]int)
	for i, record := range base {
		if baseMatched[i] {
			continue
		}
		baseBuckets[Key(record)] = append(baseBuckets[Key(record)], i)
	}
	for i, record := range cur {
		if curMatched[i] {
			continue
		}
		curBuckets[Key(record)] = append(curBuckets[Key(record)], i)
	}
	for _, key := range sortedIdentityKeys(baseBuckets, curBuckets) {
		baseIndexes := baseBuckets[key]
		curIndexes := curBuckets[key]
		for _, baseIndex := range baseIndexes {
			if baseMatched[baseIndex] {
				continue
			}
			curIndex, ok := nextLegacyCandidate(base[baseIndex], cur, curIndexes, curMatched)
			if !ok {
				continue
			}
			baseMatched[baseIndex] = true
			curMatched[curIndex] = true
			equivalent := recordsEquivalent(base[baseIndex], cur[curIndex])
			if base[baseIndex].SubjectAnchor != "" || cur[curIndex].SubjectAnchor != "" {
				equivalent = recordsEquivalentIgnoringAnchors(base[baseIndex], cur[curIndex])
			}
			if !equivalent {
				report.Changed = append(report.Changed, Change{
					Baseline: base[baseIndex],
					Current:  cur[curIndex],
				})
			}
		}
	}
}

func matchLegacyLineDrift(base, cur []Record, baseMatched, curMatched []bool, report *Report) {
	baseBuckets := make(map[driftIdentity][]int)
	curBuckets := make(map[driftIdentity][]int)
	for i, record := range base {
		if !baseMatched[i] {
			baseBuckets[driftKey(record)] = append(baseBuckets[driftKey(record)], i)
		}
	}
	for i, record := range cur {
		if !curMatched[i] {
			curBuckets[driftKey(record)] = append(curBuckets[driftKey(record)], i)
		}
	}
	for _, key := range sortedDriftKeys(baseBuckets, curBuckets) {
		baseIndexes := baseBuckets[key]
		curIndexes := curBuckets[key]
		for _, baseIndex := range baseIndexes {
			if baseMatched[baseIndex] {
				continue
			}
			curIndex, ok := nextLegacyCandidate(base[baseIndex], cur, curIndexes, curMatched)
			if !ok {
				continue
			}
			baseMatched[baseIndex] = true
			curMatched[curIndex] = true
			if !recordsEquivalentIgnoringSpans(base[baseIndex], cur[curIndex]) {
				report.Changed = append(report.Changed, Change{
					Baseline: base[baseIndex],
					Current:  cur[curIndex],
				})
			}
		}
	}
}

func nextLegacyCandidate(base Record, cur []Record, curIndexes []int, curMatched []bool) (int, bool) {
	for _, curIndex := range curIndexes {
		if curMatched[curIndex] || bothAnchored(base, cur[curIndex]) {
			continue
		}
		return curIndex, true
	}
	return 0, false
}

func bothAnchored(a, b Record) bool {
	return strings.TrimSpace(a.SubjectAnchor) != "" && strings.TrimSpace(b.SubjectAnchor) != ""
}

func sortedIdentityKeys(base, cur map[Identity][]int) []Identity {
	keys := make([]Identity, 0, len(base))
	for key := range base {
		if _, ok := cur[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return identityLess(keys[i], keys[j])
	})
	return keys
}

func sortedDriftKeys(base, cur map[driftIdentity][]int) []driftIdentity {
	keys := make([]driftIdentity, 0, len(base))
	for key := range base {
		if _, ok := cur[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return driftLess(keys[i], keys[j])
	})
	return keys
}

func sortedAnchoredKeys(base, cur map[anchoredIdentity][]int) []anchoredIdentity {
	keys := make([]anchoredIdentity, 0, len(base))
	for key := range base {
		if _, ok := cur[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return anchoredLess(keys[i], keys[j])
	})
	return keys
}

func recordsEquivalent(a, b Record) bool {
	return reflect.DeepEqual(canonicalRecord(a, false), canonicalRecord(b, false))
}

func recordsEquivalentIgnoringSpans(a, b Record) bool {
	return reflect.DeepEqual(canonicalRecord(a, true), canonicalRecord(b, true))
}

func recordsEquivalentIgnoringAnchors(a, b Record) bool {
	left := canonicalRecord(a, false)
	right := canonicalRecord(b, false)
	left.SubjectAnchor = ""
	right.SubjectAnchor = ""
	return reflect.DeepEqual(left, right)
}

type comparableRecord struct {
	Kind          string
	Scope         string
	Entry         string
	Code          string
	Severity      string
	SubjectAnchor string
	File          string
	Span          Span
	Message       string
	Help          string
	Evidence      []Fact
	Labels        []Label
}

func canonicalRecord(r Record, ignoreSpans bool) comparableRecord {
	span := primarySpan(r)
	evidence := append([]Fact(nil), r.Evidence...)
	labels := append([]Label(nil), r.Labels...)
	if ignoreSpans {
		span = Span{}
		for i := range evidence {
			evidence[i].Span = Span{}
		}
		for i := range labels {
			labels[i].Span = Span{}
		}
	}
	return comparableRecord{
		Kind:          normalizedKind(r),
		Scope:         scopeKey(r),
		Entry:         entryKey(r),
		Code:          r.Code,
		Severity:      r.Severity,
		SubjectAnchor: canonicalSubjectAnchor(r, ignoreSpans),
		File:          fileKey(r),
		Span:          span,
		Message:       r.Message,
		Help:          r.Help,
		Evidence:      evidence,
		Labels:        labels,
	}
}

func canonicalSubjectAnchor(r Record, ignoreSpans bool) string {
	if ignoreSpans {
		return ""
	}
	return strings.TrimSpace(r.SubjectAnchor)
}

func sortReport(report *Report) {
	sortRecords(report.New)
	sortRecords(report.Removed)
	sort.Slice(report.Changed, func(i, j int) bool {
		return changeLess(report.Changed[i], report.Changed[j])
	})
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		return recordLess(records[i], records[j])
	})
}

func recordLess(a, b Record) bool {
	aKey := Key(a)
	bKey := Key(b)
	if !identityEqual(aKey, bKey) {
		return identityLess(aKey, bKey)
	}
	if a.Severity != b.Severity {
		return a.Severity < b.Severity
	}
	if a.Message != b.Message {
		return a.Message < b.Message
	}
	if a.Help != b.Help {
		return a.Help < b.Help
	}
	return false
}

func changeLess(a, b Change) bool {
	aKey := Key(a.Baseline)
	bKey := Key(b.Baseline)
	if !identityEqual(aKey, bKey) {
		return identityLess(aKey, bKey)
	}
	if a.Baseline.Message != b.Baseline.Message {
		return a.Baseline.Message < b.Baseline.Message
	}
	return a.Current.Message < b.Current.Message
}

func identityLess(a, b Identity) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if a.Entry != b.Entry {
		return a.Entry < b.Entry
	}
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	if a.Column != b.Column {
		return a.Column < b.Column
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Subject < b.Subject
}

func identityEqual(a, b Identity) bool {
	return a.Scope == b.Scope &&
		a.Entry == b.Entry &&
		a.File == b.File &&
		a.Code == b.Code &&
		a.Line == b.Line &&
		a.Column == b.Column &&
		a.Subject == b.Subject
}

func driftLess(a, b driftIdentity) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if a.Entry != b.Entry {
		return a.Entry < b.Entry
	}
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Message < b.Message
}

func anchoredLess(a, b anchoredIdentity) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if a.Entry != b.Entry {
		return a.Entry < b.Entry
	}
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.SubjectAnchor < b.SubjectAnchor
}
