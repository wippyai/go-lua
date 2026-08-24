package inspect

// Gap names one inspectable fact the current public product surface does not
// expose. The inspector reports these instead of reaching past the surface or
// reconstructing them from private engine state.
type Gap struct {
	Layer    string
	Accessor string
}

// Gaps is the closed list of facts this inspector wants and the public Plan,
// Result, and Contract do not publish. It is the union of the per-layer lists
// below, in layer order.
func Gaps() []Gap {
	gaps := make([]Gap, 0, len(constructGapRows)+len(solvedGapRows)+len(transitionGapRows))
	gaps = append(gaps, constructGapRows...)
	gaps = append(gaps, solvedGapRows...)
	return append(gaps, transitionGapRows...)
}

func solvedGaps() []Gap { return solvedGapRows }

func transitionGaps() []Gap { return transitionGapRows }

// constructGapRows are the per-fixture instantiated construct-topology rows.
// The declared topology is reachable through composite.Table and
// composite.RulePlans; the rows the fixture actually instantiated live on
// engine.CommittedProgram, which analysis.Plan holds privately and publishes
// no accessor for. The lookups CommittedProgram does export are keyed by an
// identity the caller must already hold, so there is no enumeration either.
var constructGapRows = []Gap{
	{Layer: "construct", Accessor: "analysis.Plan.CommittedProgram"},
	{Layer: "construct", Accessor: "engine.CommittedProgram member enumeration (RuleMember is keyed lookup only)"},
	{Layer: "construct", Accessor: "engine.CommittedProgram activation enumeration (ActivationMember is keyed lookup only)"},
	{Layer: "construct", Accessor: "engine.CommittedProgram.MountedActivationCandidate rows"},
}

// solvedGapRows are the solver-state facts a published cell does not carry.
// The cell publishes the fold's conclusion; the pass and epoch it was written
// in, and the factor's live evidence between passes, are engine.Solver state
// that no public accessor reaches.
var solvedGapRows = []Gap{
	{Layer: "solved", Accessor: "engine.Solver factor value between passes"},
	{Layer: "solved", Accessor: "engine.Solver writing pass ordinal per cell"},
	{Layer: "solved", Accessor: "engine.Solver writing epoch ordinal per cell"},
	{Layer: "solved", Accessor: "analysis/result.Result point enumeration (Point is a key, not a table cursor)"},
}

// transitionGapRows are the solved program point transitions. The rows are a
// public engine type with public accessors, but they hang off
// engine.CommittedProgram and their Point coordinates are an engine-internal
// type, so no caller outside the engine can read one.
var transitionGapRows = []Gap{
	{Layer: "transition", Accessor: "engine.CommittedProgram.PointTransitions from analysis.Plan"},
	{Layer: "transition", Accessor: "engine.ProgramPointTransition.SourcePoint returns engine-internal equation.Point"},
	{Layer: "transition", Accessor: "engine.ProgramPointTransition.TargetPoint returns engine-internal equation.Point"},
}
