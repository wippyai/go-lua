// runtime_program_construct.go holds the program-plane half of the production
// Solver constructor: the two published-table invariants a sealed program
// depends on, and the one mint of a Solver.
//
// The committed program is sealed before this file runs. What lives here is
// the part free of the construction workspace: given the addresses the sealed
// semantic directory published and the rows the bind produced, decide whether
// the published tables are total, then mint the Solver and the store its
// addresses are named in. Everything here takes runtime values, never a
// mutable construction handle.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// bindProgramQueryTable seals the published query table of one graph. addressed
// is the query set the sealed directory published, one canonical key per
// directory ordinal, in ordinal order. Resolving the table against it rather
// than against graph order alone is what establishes the two invariants graph
// order does not carry:
//
//   - every published address names a distinct graph query, so no two ordinals
//     publish one query and no ordinal publishes a query this graph does not own;
//   - every graph query carries a published address, so a solved query cannot
//     exist with no column to answer on.
//
// The table itself stays in graph order, because that is the order the assembled
// runtime reads it in.
func bindProgramQueryTable(addressed []composition.Key, graph *equation.Graph, bound map[composition.Key]runtimeQuery) ([]runtimeQuery, bool) {
	if graph == nil || bound == nil {
		return nil, false
	}
	count := graph.QueryCount()
	published := make(map[composition.Key]struct{}, len(addressed))
	for _, key := range addressed {
		if !key.Available() {
			return nil, false
		}
		if _, duplicate := published[key]; duplicate {
			return nil, false
		}
		published[key] = struct{}{}
	}
	if len(published) != count {
		return nil, false
	}
	queries := make([]runtimeQuery, count)
	for index := 0; index < count; index++ {
		declared, indexed := graph.QueryAt(index)
		if !indexed || !declared.Key().Available() {
			return nil, false
		}
		if _, covered := published[declared.Key()]; !covered {
			return nil, false
		}
		row, present := bound[declared.Key()]
		if !present || row == nil || row.query().Key() != declared.Key() {
			return nil, false
		}
		queries[index] = row
	}
	return queries, true
}

// bindProgramObservationTable seals the published observation table. The
// observation ordinal an issued observation handle carries is its position in this
// table, so the table and the issued ordinals are one sequence. published is how
// many distinct identities the construction admitted; requiring one row per
// admitted identity is what makes every issued ordinal address a row of the
// sealed program rather than a position past its end.
func bindProgramObservationTable(bound []runtimeObservation, published int) ([]runtimeObservation, bool) {
	if published < 0 || len(bound) != published {
		return nil, false
	}
	observations := append([]runtimeObservation(nil), bound...)
	for _, observation := range observations {
		if observation == nil {
			return nil, false
		}
	}
	return observations, true
}

// mintProgramSolver is the sole Solver constructor. It issues the store this
// Solver's addresses are named in and installs the relation the assembled
// runtime was built for. The store is issued exactly once, here, and never
// reused, so every address a completed State hands out names exactly this Solver
// and is meaningless in any other. A saturated store sequence mints nothing
// rather than aliasing a live store.
func mintProgramSolver(runtime *solverRuntime) (*Solver, SolveFailure, bool) {
	if runtime == nil || runtime.topology == nil {
		return nil, ProgramStageFailure(ProgramSealStageSolverMint), false
	}
	relation, relationOK := runtime.topology.InitialRelation()
	store, storeOK := solverStores.issue()
	if !relationOK || !storeOK {
		return nil, ProgramStageFailure(ProgramSealStageSolverMint), false
	}
	return &Solver{runtime: runtime, store: store, relation: relation}, SolveFailure{}, true
}
