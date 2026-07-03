package judgment

import "testing"

func TestDiffShadowRecordsMatchesCodeSubjectAndSpan(t *testing.T) {
	span := SpanRef{File: "a.lua", StartLine: 10, StartCol: 4, EndLine: 10, EndCol: 8}
	oldRecords := []ShadowRecord{
		{Code: "E", SubjectKey: "call:1:arg:0", Span: span},
	}
	newRecords := []ShadowRecord{
		{Code: "E", SubjectKey: "call:1:arg:0", Span: span},
	}

	if got := DiffShadowRecords(oldRecords, newRecords); len(got) != 0 {
		t.Fatalf("diff = %#v, want none", got)
	}
}

func TestShadowRecordsFromJudgmentsUseStableSubjectAndPrimarySpan(t *testing.T) {
	span := SpanRef{File: "main.lua", StartLine: 3, StartCol: 7}
	got := ShadowRecordsFromJudgments([]Judgment{
		{
			Code: CodeCallArgType,
			Subject: NewSubjectRef(
				"fixture",
				SubjectCallArgument,
				"call:2:arg:0",
			),
			Spans: []SpanRef{span, {File: "main.lua", StartLine: 1}},
		},
	})
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	if got[0].Code != string(CodeCallArgType) || got[0].SubjectKey != "fixture|call_arg|call:2:arg:0" || got[0].Span != span {
		t.Fatalf("record = %#v, want code/stable subject/primary span", got[0])
	}
}

func TestShadowRecordsByCodeAndSpanDropsSubjectOnly(t *testing.T) {
	span := SpanRef{File: "main.lua", StartLine: 3, StartCol: 7}
	got := ShadowRecordsByCodeAndSpan([]ShadowRecord{
		{Code: "E", SubjectKey: "subject", Span: span},
	})
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	if got[0].Code != "E" || got[0].SubjectKey != "" || got[0].Span != span {
		t.Fatalf("record = %#v, want code/span with blank subject", got[0])
	}
}

func TestDiffShadowRecordsReportsDeterministicCountDeltas(t *testing.T) {
	shared := ShadowRecord{
		Code:       "E",
		SubjectKey: "call:1:arg:0",
		Span:       SpanRef{File: "a.lua", StartLine: 1, StartCol: 1},
	}
	oldOnly := ShadowRecord{
		Code:       "A",
		SubjectKey: "call:old",
		Span:       SpanRef{File: "a.lua", StartLine: 2, StartCol: 1},
	}
	newOnly := ShadowRecord{
		Code:       "Z",
		SubjectKey: "call:new",
		Span:       SpanRef{File: "z.lua", StartLine: 1, StartCol: 1},
	}

	got := DiffShadowRecords(
		[]ShadowRecord{shared, shared, oldOnly},
		[]ShadowRecord{shared, newOnly},
	)
	if len(got) != 3 {
		t.Fatalf("diff len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Kind != ShadowMissingNew || got[0].Record != oldOnly {
		t.Fatalf("first delta = %#v, want missing oldOnly", got[0])
	}
	if got[1].Kind != ShadowMissingNew || got[1].Record != shared {
		t.Fatalf("second delta = %#v, want missing shared duplicate", got[1])
	}
	if got[2].Kind != ShadowUnexpectedNew || got[2].Record != newOnly {
		t.Fatalf("third delta = %#v, want unexpected newOnly", got[2])
	}
}

func TestApplyAcceptedShadowDeltasRequiresExactReviewedCounts(t *testing.T) {
	deltas := []ShadowDelta{
		{Kind: ShadowUnexpectedNew, Record: ShadowRecord{Code: "E", Span: SpanRef{File: "suite/a.lua"}}},
		{Kind: ShadowUnexpectedNew, Record: ShadowRecord{Code: "E", Span: SpanRef{File: "suite/b.lua"}}},
		{Kind: ShadowMissingNew, Record: ShadowRecord{Code: "E", Span: SpanRef{File: "suite/c.lua"}}},
		{Kind: ShadowUnexpectedNew, Record: ShadowRecord{Code: "E", Span: SpanRef{File: "other/a.lua"}}},
	}
	rules := []AcceptedShadowDeltaRule{{
		Kind:       ShadowUnexpectedNew,
		Code:       "E",
		FilePrefix: "suite/",
		Count:      2,
		Reason:     "reviewed stricter obligation",
	}}

	remaining, problems := ApplyAcceptedShadowDeltas(deltas, rules)
	if len(problems) != 0 {
		t.Fatalf("problems = %#v, want none", problems)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining = %#v, want missing suite plus other unexpected", remaining)
	}
	if remaining[0].Kind != ShadowMissingNew || remaining[1].Record.Span.File != "other/a.lua" {
		t.Fatalf("remaining = %#v, want unaccepted deltas preserved", remaining)
	}

	rules[0].Count = 1
	_, problems = ApplyAcceptedShadowDeltas(deltas, rules)
	if len(problems) != 1 {
		t.Fatalf("problems = %#v, want exact-count mismatch", problems)
	}
}
