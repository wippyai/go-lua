// runtime_program_seal.go mints the Solver of one committed program. A
// committed program carries the rows its declaration stated - one member row
// per published member, one query row per published query - and sealing binds
// them against a freshly bound Factor plane, folds the published tables, and
// mints the Solver.
//
// Nothing is accumulated here and nothing is retained: the committed program
// is a finished value, so a second seal binds another plane from the same
// sealed rows. That is what lets one committed program answer several
// observation inventories without a second construction.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// programMemberBinding is one member row of a committed program: the identity
// the graph publishes it under and the sealed cell that mints its runtime row.
type programMemberBinding struct {
	member     identity.ContentID
	activation identity.ContentID
	operand    declaredRuleOperand
	binder     sealedRuleCell
	generated  *generatedMemberDeclaration
	activated  bool
}

// programQueryBinding is one query row of a committed program: the identity
// the graph publishes it under and the sealed query cell that mints it.
type programQueryBinding struct {
	id    identity.ContentID
	admit programQueryAdmit
}

// declaredQueryBindings states the query rows of one declaration as the
// committed program's own binding rows, in declaration order.
func declaredQueryBindings(queries []declaredQueryRow) []programQueryBinding {
	bindings := make([]programQueryBinding, 0, len(queries))
	for _, query := range queries {
		bindings = append(bindings, programQueryBinding{id: query.ID, admit: query.Admit})
	}
	return bindings
}

// Seal mints the Solver of this committed program under one observation
// inventory. It binds a Factor plane for the committed graph, mints one
// runtime row per declared member and query, folds the published query and
// observation tables, and mints the Solver from the assembled runtime.
//
// The committed program is unchanged by a seal: an inventory that binds is one
// Solver, and a later inventory over the same program binds its own plane.
func (committed *CommittedProgram) Seal(observations []ProgramObservationAdmission) (*Solver, SolveFailure, bool) {
	return committed.seal(observations)
}

func (committed *CommittedProgram) seal(observations []ProgramObservationAdmission) (*Solver, SolveFailure, bool) {
	if !committed.valid() {
		return nil, ProgramStageFailure(ProgramSealStageAdmission), false
	}
	plane, planeStage, planeOK := bindProgramPlane(committed.state, committed.graph)
	if !planeOK || plane == nil || plane.carrier == nil || plane.byKey == nil {
		if planeStage == ProgramSealStageNone {
			planeStage = ProgramSealStageFactorBind
		}
		return nil, ProgramStageFailure(planeStage), false
	}
	if !plane.attachQueryContext(committed) {
		return nil, ProgramStageFailure(ProgramSealStageAdmission), false
	}
	drafts, draftsOK := committed.bindMemberRows(plane)
	if !draftsOK {
		return nil, ProgramStageFailure(ProgramSealStageMemberBind), false
	}
	bound, boundOK := committed.bindQueryRows(plane)
	queries, queriesOK := bindProgramQueryTable(committed.addressed, committed.graph, bound)
	if !boundOK || !queriesOK {
		return nil, ProgramStageFailure(ProgramSealStageQueryAddress), false
	}
	observed, observationFailure := committed.bindObservationRows(plane, observations)
	if observationFailure != observationSealFailureNone {
		return nil, observationFailure.Failure(), false
	}
	runtime, refusal, assembled := assembleProgramRuntime(committed.state.schema, committed.graph, plane.carrier, plane.byKey, drafts, queries, observed, committed.contexts, committed.contextIndex, committed.contextLayout, committed.pointOwners, committed.pointTransitions, committed.artifactBacked)
	if !assembled || runtime == nil {
		// The assembly's own step travels out. A caller that received the bare
		// stage learned only that the program did not seal, which is the one
		// thing it already knew.
		if refusal.Available() {
			return nil, refusal.Failure(), false
		}
		return nil, ProgramStageFailure(ProgramSealStageProgramSeal), false
	}
	runtime.topology = committed.topology
	if !plane.releaseColdFactorBindings() {
		return nil, ProgramStageFailure(ProgramSealStageFactorBind), false
	}
	solver, failure, minted := mintProgramSolver(runtime)
	if !minted {
		if failure.Available() {
			return nil, failure, false
		}
		return nil, ProgramStageFailure(ProgramSealStageSolverMint), false
	}
	return solver, SolveFailure{}, true
}

// bindMemberRows mints one runtime member per declared member row. Each row is
// resolved against the published directory, so a bind addresses exactly the
// member the geometry published it under.
func (committed *CommittedProgram) bindMemberRows(plane *programPlane) ([]memberRow, bool) {
	drafts := make([]memberRow, 0, len(committed.members))
	bound := make(map[composition.Key]struct{}, len(committed.members))
	for _, declared := range committed.members {
		member, resolved := committed.declaredMember(declared)
		if !resolved || !member.Key().Available() {
			return nil, false
		}
		if _, duplicate := bound[member.Key()]; duplicate {
			return nil, false
		}
		if declared.generated != nil {
			generatedRow, generatedOK := bindGeneratedMember(plane, committed.topology, member, declared.generated)
			if !generatedOK || generatedRow == nil || generatedRow.member().Key() != member.Key() {
				return nil, false
			}
			bound[member.Key()] = struct{}{}
			drafts = append(drafts, memberRow{generated: generatedRow})
			continue
		}
		if declared.binder == nil || !declared.binder.schemaRuleComplete() {
			return nil, false
		}
		legacyRow, legacyOK := declared.binder.bindMember(plane, committed.topology, member, declared.operand)
		if !legacyOK || legacyRow == nil || legacyRow.member().Key() != member.Key() {
			return nil, false
		}
		bound[member.Key()] = struct{}{}
		drafts = append(drafts, memberRow{legacy: legacyRow})
	}
	return drafts, true
}

// declaredMember resolves one declared row to its published graph member. An
// activation row is addressed through its activation identity, which is the
// identity its trigger was published under.
func (committed *CommittedProgram) declaredMember(declared programMemberBinding) (equation.RuleMember, bool) {
	if declared.activated {
		member, ok := committed.lookupActivationMember(declared.activation)
		return member.member, ok
	}
	member, ok := committed.lookupRuleMember(declared.member)
	return member.member, ok
}

// bindQueryRows mints one runtime query per declared query row, keyed by the
// canonical query key the published table is addressed by.
func (committed *CommittedProgram) bindQueryRows(plane *programPlane) (map[composition.Key]queryRow, bool) {
	bound := make(map[composition.Key]queryRow, len(committed.queries))
	for _, declared := range committed.queries {
		if declared.admit == nil {
			return nil, false
		}
		query, resolved := committed.Query(declared.id)
		if !resolved || !query.identity.Key().Available() {
			return nil, false
		}
		if _, duplicate := bound[query.identity.Key()]; duplicate {
			return nil, false
		}
		row, ok := declared.admit.bindProgramQuery(plane, query.identity)
		if !ok || !row.valid() {
			return nil, false
		}
		bound[query.identity.Key()] = row
	}
	return bound, true
}

// bindObservationRows mints the optional observation rows of one inventory, in
// inventory order: the ordinal an observation answers on is its position here.
// The member-output point index is built once and only when an inventory asks
// for it.
func (committed *CommittedProgram) bindObservationRows(plane *programPlane, observations []ProgramObservationAdmission) ([]observationRow, observationSealFailure) {
	if len(observations) == 0 {
		return nil, observationSealFailureNone
	}
	points, indexed := indexObservationPoints(committed.graph)
	if !indexed {
		return nil, observationSealFailurePoint
	}
	rows := make([]observationRow, 0, len(observations))
	admitted := make(map[identity.ContentID]struct{}, len(observations))
	for _, declared := range observations {
		if declared.admit == nil || !declared.ID.Available() || !declared.Context.Available() || !declared.Mount.Available() {
			return nil, observationSealFailureArguments
		}
		if !committed.ownsObservationContext(declared.Context, declared.Mount) {
			return nil, observationSealFailureArguments
		}
		member, resolved := committed.MountedRuleMember(declared.Role, declared.Mount, declared.memberPoint, declared.Occurrence)
		if !resolved {
			return nil, observationSealFailurePoint
		}
		memberPoint, located := points[member.member.Key()]
		point := memberPoint
		pointOK := located && declared.Point == declared.memberPoint
		if declared.readPoint.Available() {
			if declared.Point != declared.readPoint {
				return nil, observationSealFailurePoint
			}
			point, pointOK = committed.lookupPoint(declared.Point)
		}
		if !located || !committed.graph.OwnsPoint(memberPoint) || !pointOK || !committed.graph.OwnsPoint(point) {
			return nil, observationSealFailurePoint
		}
		if _, duplicate := admitted[declared.ID]; duplicate {
			return nil, observationSealFailureDuplicate
		}
		row, ok := declared.admit.bindProgramObservation(plane, declared.ID, member.member, point, declared.Context, declared.exactSurface, declared.exactSurfaceOK)
		if !ok || !row.valid() {
			return nil, observationSealFailureFactor
		}
		admitted[declared.ID] = struct{}{}
		rows = append(rows, row)
	}
	return rows, observationSealFailureNone
}

// bindMember mints one ordinary Rule member's runtime row from the canonical
// operand issued by the same sealed cell. The owner resolver and content
// projector have already run before this boundary.
func (cell *schemaRuleBindingCellImpl[K, V, O]) bindMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, operand declaredRuleOperand) (runtimeMember, bool) {
	if cell == nil || topology == nil || plane == nil || plane.runtime == nil || !topology.OwnsGraph(plane.runtime.graph) {
		return nil, false
	}
	return bindSealedRuleCellMember(plane, cell, member, operand)
}

// bindMember mints one activation trigger's runtime row. An activation carries
// no operand: its row is compiled from the trigger member geometry and the
// sealed Factor plane.
func (cell *schemaActivationRuleBindingCell) bindMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, _ declaredRuleOperand) (runtimeMember, bool) {
	if cell == nil || topology == nil || plane == nil || !plane.frozen || plane.runtime == nil || plane.byKey == nil || !cell.schemaRuleComplete() || cell.state != plane.runtime.state || cell.state.authority != plane.runtime.authority {
		return nil, false
	}
	row, ok := bindActivationCellMember(member, cell, cell.ordinal, topology, member.Key(), plane.runtime.graph, plane.byKey)
	if !ok || row == nil {
		return nil, false
	}
	return row, true
}
