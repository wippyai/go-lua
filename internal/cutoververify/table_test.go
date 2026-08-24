package cutoververify

import "testing"

func TestFormatTableAlignment(t *testing.T) {
	results := []Result{
		{Name: "INDEX", Status: StatusPass, Note: "no staged files"},
		{Name: "PROTOCOL-ZERO", Status: StatusWarn, Note: "3 legacy token reference(s) found"},
		{Name: "TESTS", Status: StatusFail, Note: "go test -count=1 domain/x/... failed"},
	}
	got := FormatTable(results)
	want := "" +
		"CHECK          STATUS  NOTE\n" +
		"-------------  ------  ----\n" +
		"INDEX          PASS    no staged files\n" +
		"PROTOCOL-ZERO  WARN    3 legacy token reference(s) found\n" +
		"TESTS          FAIL    go test -count=1 domain/x/... failed\n"
	if got != want {
		t.Fatalf("FormatTable mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatTableEmpty(t *testing.T) {
	got := FormatTable(nil)
	want := "CHECK  STATUS  NOTE\n-----  ------  ----\n"
	if got != want {
		t.Fatalf("FormatTable(nil) mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestOverallPassRequiresEveryCheck(t *testing.T) {
	pass, line := Overall([]Result{
		{Name: "A", Status: StatusPass},
		{Name: "B", Status: StatusSkip},
	}, false)
	if !pass || line != "RESULT: PASS" {
		t.Fatalf("got pass=%v line=%q, want pass=true line=RESULT: PASS", pass, line)
	}

	pass, line = Overall([]Result{
		{Name: "A", Status: StatusPass},
		{Name: "B", Status: StatusFail},
	}, false)
	if pass || line != "RESULT: FAIL" {
		t.Fatalf("got pass=%v line=%q, want pass=false line=RESULT: FAIL", pass, line)
	}
}

func TestOverallWarnTreatment(t *testing.T) {
	results := []Result{
		{Name: "A", Status: StatusPass},
		{Name: "PROTOCOL-ZERO", Status: StatusWarn},
	}

	if pass, line := Overall(results, false); !pass || line != "RESULT: PASS" {
		t.Fatalf("warn should pass when treatWarnAsFail=false, got pass=%v line=%q", pass, line)
	}
	if pass, line := Overall(results, true); pass || line != "RESULT: FAIL" {
		t.Fatalf("warn should fail when treatWarnAsFail=true, got pass=%v line=%q", pass, line)
	}
}
