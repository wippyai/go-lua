package inspect

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestInspectCommandsNonemptyAndStableOnFibonacci(t *testing.T) {
	session := openFixture(t, "bench/fibonacci")
	for _, name := range []string{"target", "rows", "publish", "why"} {
		first, err := session.Command(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.TrimSpace(first) == "" {
			t.Fatalf("%s produced no output", name)
		}
		second, err := session.Command(name)
		if err != nil {
			t.Fatalf("%s second: %v", name, err)
		}
		if first != second {
			t.Fatalf("%s output is not stable", name)
		}
	}
	target, err := session.Command("target")
	if err != nil {
		t.Fatal(err)
	}
	id := firstOperationID(t, target)
	rendered, err := session.Command("row", id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(rendered) == "" {
		t.Fatal("row produced no output")
	}
	if !strings.Contains(rendered, "contract.OperationContentID=") {
		t.Fatalf("row did not name contract.OperationContentID: %s", rendered)
	}
	if session.Compilation().Available() {
		if session.CompileStatus() != analysis.CompileComplete {
			t.Fatalf("compile complete required: status=%v diagnostics=%+v", session.CompileStatus(), session.CompileDiagnostics())
		}
		if session.SolveStatus() != analysis.AnalyzeComplete || session.Result() == nil {
			t.Fatalf("solve complete required: status=%v result=%t diagnostics=%+v", session.SolveStatus(), session.Result() != nil, session.SolveDiagnostics())
		}
		rows, err := session.Command("rows")
		if err != nil {
			t.Fatal(err)
		}
		body := firstBodyID(t, rows)
		bodyRow, err := session.Command("row", body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(bodyRow, "result.BodyAt(") {
			t.Fatalf("row did not name result.BodyAt: %s", bodyRow)
		}
	}
}

func TestInspectSelfDiffEmpty(t *testing.T) {
	session := openFixture(t, "bench/fibonacci")
	diff, err := session.Command("diff", "bench/fibonacci")
	if err != nil {
		t.Fatal(err)
	}
	if diff != "" {
		t.Fatalf("self diff = %q", diff)
	}
}

func TestInspectRowLookupAllocsZero(t *testing.T) {
	session := openFixture(t, "bench/fibonacci")
	target, err := session.Command("target")
	if err != nil {
		t.Fatal(err)
	}
	id, ok := ParseContentID(firstOperationID(t, target))
	if !ok {
		t.Fatal("operation identity")
	}
	if _, ok := session.Lookup(id); !ok {
		t.Fatal("lookup")
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := session.Lookup(id); !ok {
			t.Fatal("lookup")
		}
	})
	if allocs != 0 {
		t.Fatalf("Lookup allocs = %v, want 0", allocs)
	}
}

func TestInspectReportsUnexposedAccessors(t *testing.T) {
	session := openFixture(t, "bench/fibonacci")
	rows, err := session.Command("rows")
	if err != nil {
		t.Fatal(err)
	}
	if len(Gaps()) == 0 {
		t.Fatal("gaps")
	}
	for _, gap := range Gaps() {
		line := "unexposed." + gap.Layer + "=" + gap.Accessor
		if !strings.Contains(rows, line) {
			t.Fatalf("rows missing %q", line)
		}
	}
}

func openFixture(t *testing.T, name string) *Session {
	t.Helper()
	root, err := testfixture.RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	session, err := Open(root, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !session.Close() {
			t.Error("close session")
		}
	})
	return session
}

func firstBodyID(t *testing.T, rows string) string {
	t.Helper()
	return firstPrefixed(t, rows, "result.BodyAt(0).ID=")
}

func firstOperationID(t *testing.T, target string) string {
	t.Helper()
	for _, line := range strings.Split(target, "\n") {
		if strings.HasPrefix(line, "contract.OperationContentID(") {
			_, value, ok := strings.Cut(line, "=")
			if !ok || value == "" {
				break
			}
			return value
		}
	}
	t.Fatal("no contract.OperationContentID in target")
	return ""
}

func firstPrefixed(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("no %s", prefix)
	return ""
}
