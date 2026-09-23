package regression

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func nonConvergenceWarnings(diagnostics []diag.Diagnostic) []string {
	var out []string
	for _, d := range diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "type inference did not converge") {
			out = append(out, d.Message)
		}
	}
	return out
}

func TestInference_ConvergesOnTruthyNarrowedSelfEmbedding(t *testing.T) {
	code := `
		local function read(view: string, source: any): any
			local value = nil
			local err: string? = nil
			if view == "brief" then
				value, err = source.brief()
				if value then value = { brief = value } end
			elseif view == "tasks" then
				value, err = source.tasks()
				if value then value = { tasks = value } end
			elseif view == "changes" then
				value, err = source.changes()
				if value then value = { changes = value } end
			end
			if err then return nil end
			return value
		end
		return read
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if warnings := nonConvergenceWarnings(result.Diagnostics); len(warnings) > 0 {
		t.Fatalf("unexpected non-convergence warnings: %v", warnings)
	}
}

func TestInference_ConvergesOnJournalReadViews(t *testing.T) {
	source, err := os.ReadFile("../../../../testdata/fixtures/realworld/journal-read-views/main.lua")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	result := testutil.Check(string(source), testutil.WithStdlib())
	if warnings := nonConvergenceWarnings(result.Diagnostics); len(warnings) > 0 {
		t.Fatalf("unexpected non-convergence warnings: %v", warnings)
	}
}
