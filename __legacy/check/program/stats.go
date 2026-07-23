package program

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// FunctionalSummaryBodyStats measures the semantic work owned by one lexical
// function transformer. Cells and equations describe the body-owned fixed
// equation system; Evaluations counts executions of those equations. None of
// these counters may be multiplied by the number of callers.
type FunctionalSummaryBodyStats struct {
	Cells     int
	Equations int
}

// FunctionalSummaryStats is the caller-owned observation surface for the
// functional interprocedural solver. ApplyInstantiations is deliberately
// separate from body work: instantiation may scale with call sites, while a
// lexical body's cells, equations, and evaluations must not.
type FunctionalSummaryStats struct {
	ApplyInstantiations int
	LexicalBodies       int
	FormalEquations     int
	Bodies              map[lexicalidentity.StableLexicalBodyID]FunctionalSummaryBodyStats
}

// functionalSummaryWorkView is the minimal observation contract exported by
// the sole stabilized transformer solve.  It deliberately exposes no route,
// eligibility, fallback, or alternate executor: program knows the complete
// lexical forest and asks only how much body-owned work each member performed
// and how many cheap Apply substitutions were instantiated.
type functionalSummaryWorkView interface {
	FunctionalApplyInstantiations() int
	FunctionalSummaryBodyWork(lexicalidentity.StableLexicalBodyID) (cells int, equations int, ok bool)
	FormalEquationCount() int
}

func recordFunctionalSummaryStats(stats *Stats, prepared preparedBodies, view any) error {
	if stats == nil {
		return nil
	}
	work, ok := view.(functionalSummaryWorkView)
	if !ok {
		return fmt.Errorf("program: stabilized relation view has no functional-summary work authority")
	}
	bodyIDs := factoriesBodyIDs(prepared)
	bodies := make(map[lexicalidentity.StableLexicalBodyID]FunctionalSummaryBodyStats, len(bodyIDs))
	for bodyID := range bodyIDs {
		cells, equations, present := work.FunctionalSummaryBodyWork(bodyID)
		if !present {
			return fmt.Errorf("program: functional-summary work is absent for lexical body %s", bodyID)
		}
		if cells <= 0 || equations <= 0 {
			return fmt.Errorf("program: lexical body %s reported vacuous formal work %d/%d", bodyID, cells, equations)
		}
		bodies[bodyID] = FunctionalSummaryBodyStats{Cells: cells, Equations: equations}
	}
	instantiations := work.FunctionalApplyInstantiations()
	if instantiations < 0 {
		return fmt.Errorf("program: functional-summary Apply count is negative")
	}
	formalEquations := work.FormalEquationCount()
	if formalEquations <= 0 {
		return fmt.Errorf("program: formal equation inventory is empty")
	}
	stats.FunctionalSummary = FunctionalSummaryStats{
		ApplyInstantiations: instantiations, LexicalBodies: len(bodyIDs), FormalEquations: formalEquations, Bodies: bodies,
	}
	return nil
}

func recordProgramShape(stats *Stats, keys programKeys) {
	if stats != nil && len(keys.functions) > stats.MaxFunctionCount {
		stats.MaxFunctionCount = len(keys.functions)
	}
}
