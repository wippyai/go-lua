package diffreport

import (
	"bytes"
	"os"
	"testing"
)

func TestCompareMatchesExactIdentity(t *testing.T) {
	baseline := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is 42, not string")}
	current := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is 42, not string")}

	report := Compare(baseline, current)
	assertReportCounts(t, report, 0, 0, 0)
}

func TestCompareSuppressesPureLineShift(t *testing.T) {
	baseline := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is 42, not string")}
	current := []Record{testRecord("suite-a", "main.lua", 12, 13, "type.call", "argument 1 is 42, not string")}

	report := Compare(baseline, current)
	assertReportCounts(t, report, 0, 0, 0)
}

func TestCompareClassifiesSameSiteMessageChange(t *testing.T) {
	baseline := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is 42, not string")}
	current := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is integer, not string")}

	report := Compare(baseline, current)
	assertReportCounts(t, report, 0, 0, 1)
	if got, want := report.Changed[0].Baseline.Message, "argument 1 is 42, not string"; got != want {
		t.Fatalf("baseline message = %q, want %q", got, want)
	}
	if got, want := report.Changed[0].Current.Message, "argument 1 is integer, not string"; got != want {
		t.Fatalf("current message = %q, want %q", got, want)
	}
}

func TestCompareClassifiesNewRecord(t *testing.T) {
	current := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is 42, not string")}

	report := Compare(nil, current)
	assertReportCounts(t, report, 1, 0, 0)
}

func TestCompareClassifiesRemovedRecord(t *testing.T) {
	baseline := []Record{testRecord("suite-a", "main.lua", 10, 13, "type.call", "argument 1 is 42, not string")}

	report := Compare(baseline, nil)
	assertReportCounts(t, report, 0, 1, 0)
}

func TestCompareMatchesExactBeforeLineDrift(t *testing.T) {
	message := "cannot assign value because assigned value is number, not string"
	baseline := []Record{
		testRecord("suite-a", "main.lua", 10, 13, "type.assignment", message),
		testRecord("suite-a", "main.lua", 20, 13, "type.assignment", message),
	}
	current := []Record{
		testRecord("suite-a", "main.lua", 10, 13, "type.assignment", message),
		testRecord("suite-a", "main.lua", 30, 13, "type.assignment", message),
	}

	report := Compare(baseline, current)
	assertReportCounts(t, report, 0, 0, 0)
}

func TestReadJSONLAcceptsExternalHarnessRows(t *testing.T) {
	input := bytes.NewBufferString(`{"target":"framework/src/views","entry_id":"wippy.views:renderer","code":"E0000","severity":"error","line":53,"column":37,"message":"argument 1 (page) comes from any/unknown; no proof shows it is {id: number | string}"}` + "\n")

	records, err := ReadJSONL(input)
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	key := Key(records[0])
	if key.Scope != "framework/src/views" || key.Entry != "wippy.views:renderer" || key.File != "wippy.views:renderer" || key.Line != 53 || key.Column != 37 {
		t.Fatalf("unexpected key: %#v", key)
	}
	if key.Subject != "argument 1" {
		t.Fatalf("subject = %q, want argument 1", key.Subject)
	}
}

func TestWriteJSONLGoldenDeterministic(t *testing.T) {
	baseline := []Record{
		testRecord("suite-z", "main.lua", 7, 2, "type.assignment", "cannot assign gone because assigned value is number, not string"),
		testRecord("suite-a", "main.lua", 4, 5, "type.call", "argument 1 is number, not string"),
	}
	current := []Record{
		testRecord("suite-b", "worker.lua", 2, 1, "type.return", "returned value 2 comes from any/unknown; no proof shows it satisfies declared return type string?"),
		testRecord("suite-a", "main.lua", 4, 5, "type.call", "argument 1 is integer, not string"),
	}

	var got bytes.Buffer
	if err := WriteJSONL(&got, Compare(baseline, current)); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	want, err := os.ReadFile("testdata/jsonl_report.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got.String() != string(want) {
		t.Fatalf("JSONL report mismatch\n--- got ---\n%s--- want ---\n%s", got.String(), string(want))
	}
}

func testRecord(suite, file string, line, col int, code, message string) Record {
	return Record{
		Kind:     "diagnostic",
		Suite:    suite,
		Code:     code,
		Severity: "error",
		File:     file,
		Span: Span{
			StartLine: line,
			StartCol:  col,
		},
		Message: message,
	}
}

func assertReportCounts(t *testing.T, report Report, newCount, removedCount, changedCount int) {
	t.Helper()
	if len(report.New) != newCount || len(report.Removed) != removedCount || len(report.Changed) != changedCount {
		t.Fatalf("counts new/removed/changed = %d/%d/%d, want %d/%d/%d",
			len(report.New), len(report.Removed), len(report.Changed),
			newCount, removedCount, changedCount)
	}
}
