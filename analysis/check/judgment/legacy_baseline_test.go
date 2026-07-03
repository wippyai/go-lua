package judgment

import (
	"strings"
	"testing"
)

func TestReadLegacyBaselineShadowRecordsMapsDiagnosticRecordsOnly(t *testing.T) {
	input := strings.NewReader(`
{"kind":"suite","suite":"x","status":"pass"}
{"kind":"diagnostic","code":"type.call.direct.argument_type","file":"main.lua","span":{"start_line":4,"start_col":8,"end_line":4,"end_col":10}}
{"kind":"diagnostic","code":"type.assignment","file":"main.lua","span":{"start_line":9,"start_col":1,"end_line":9,"end_col":2}}
`)
	got, err := ReadLegacyBaselineShadowRecords(input, func(code string) (string, bool) {
		if code != "type.call.direct.argument_type" {
			return "", false
		}
		return string(CodeCallArgType), true
	})
	if err != nil {
		t.Fatalf("ReadLegacyBaselineShadowRecords: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1: %#v", len(got), got)
	}
	want := ShadowRecord{
		Code: string(CodeCallArgType),
		Span: SpanRef{File: "main.lua", StartLine: 4, StartCol: 8, EndLine: 4, EndCol: 10},
	}
	if got[0] != want {
		t.Fatalf("record = %#v, want %#v", got[0], want)
	}
}

func TestReadLegacyBaselineShadowRecordsReportsBadJSON(t *testing.T) {
	_, err := ReadLegacyBaselineShadowRecords(strings.NewReader("{"), nil)
	if err == nil {
		t.Fatal("ReadLegacyBaselineShadowRecords accepted malformed JSON")
	}
}
