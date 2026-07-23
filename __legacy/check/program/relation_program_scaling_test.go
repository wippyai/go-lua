package program

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func functionalSummaryCallerFixture(callers int) string {
	var source strings.Builder
	source.WriteString("local function helper(value: number): number\n")
	source.WriteString("    return value + 1\n")
	source.WriteString("end\n")
	source.WriteString("return ")
	for caller := 1; caller <= callers; caller++ {
		if caller != 1 {
			source.WriteString(" + ")
		}
		source.WriteString("helper(")
		source.WriteString(strconv.Itoa(caller))
		source.WriteByte(')')
	}
	source.WriteByte('\n')
	return source.String()
}

func runFunctionalSummaryCallerFixture(t *testing.T, callers int) (FunctionalSummaryBodyStats, int) {
	t.Helper()
	statements := parseRelationProgramInputChunk(t, functionalSummaryCallerFixture(callers))
	bindings := bind.BindChunk(statements, bind.Options{})
	registry := standard.Registry()
	key := rootKey(summary.SummaryKey{})
	keys := collectKeys(bindings, key, registry, nil, body.Config{}.ModuleExports, statements)
	prepared, err := prepareBoundChunkBodies(statements, bindings, body.Config{Registry: registry}, keys)
	if err != nil {
		t.Fatalf("prepare %d-caller functional-summary fixture: %v", callers, err)
	}
	if len(prepared.functions) != 1 {
		t.Fatalf("%d-caller fixture prepared %d lexical functions, want helper only", callers, len(prepared.functions))
	}
	var helper lexicalidentity.StableLexicalBodyID
	for _, static := range prepared.functions {
		helper = static.StableLexicalBodyID()
	}
	if helper == (lexicalidentity.StableLexicalBodyID{}) {
		t.Fatal("functional-summary helper has no stable lexical identity")
	}

	stats := &Stats{}
	config := Config{Check: body.Config{Registry: registry}, Stats: stats}
	if _, err := RunBoundChunk(statements, bindings, config); err != nil {
		t.Fatalf("run %d-caller functional-summary fixture: %v", callers, err)
	}
	if stats.FunctionalSummary.ApplyInstantiations != callers {
		t.Fatalf("%d callers produced %d Apply instantiations, want exactly %d", callers, stats.FunctionalSummary.ApplyInstantiations, callers)
	}
	bodyStats, present := stats.FunctionalSummary.Bodies[helper]
	if !present {
		t.Fatalf("%d callers published no functional-summary counters for helper %s", callers, helper)
	}
	if bodyStats.Cells <= 0 || bodyStats.Equations <= 0 {
		t.Fatalf("%d callers published vacuous helper work: cells=%d equations=%d", callers, bodyStats.Cells, bodyStats.Equations)
	}
	return bodyStats, stats.FunctionalSummary.ApplyInstantiations
}

// A lexical function is solved as one parametric transformer. Adding callers
// may add cheap substitutions, but must not add callee CFG cells, equations,
// equations.
func TestRelationProgramFunctionalSummaryCallerScaling(t *testing.T) {
	var baseline FunctionalSummaryBodyStats
	for index, callers := range []int{1, 10, 100} {
		bodyStats, instantiations := runFunctionalSummaryCallerFixture(t, callers)
		if index == 0 {
			baseline = bodyStats
			continue
		}
		if bodyStats != baseline {
			t.Fatalf("helper work scaled with %d callers: got cells/equations=%d/%d, one-caller baseline=%d/%d; Apply instantiations=%d",
				callers, bodyStats.Cells, bodyStats.Equations,
				baseline.Cells, baseline.Equations, instantiations)
		}
	}
}
