package inspect

// Gap names one inspectable fact the current public product surface does not
// expose. The inspector does not reconstruct these from private engine state.
type Gap struct {
	Layer    string
	Accessor string
}

// Gaps is the closed list of construct-topology and solved-plane facts the
// detached Result, Plan, and Contract do not publish. The inspector reports
// them instead of compensating.
func Gaps() []Gap {
	return []Gap{
		{Layer: "construct", Accessor: "engine.CommittedProgram.RuleMember"},
		{Layer: "construct", Accessor: "engine.CommittedProgram.ActivationMember"},
		{Layer: "construct", Accessor: "engine.CommittedProgram owner fence"},
		{Layer: "solved", Accessor: "engine.State factor value"},
		{Layer: "solved", Accessor: "engine.State four-state evidence"},
		{Layer: "solved", Accessor: "engine.State writing pass/epoch"},
		{Layer: "solved", Accessor: "analysis/result.Result.PointAt"},
		{Layer: "publication", Accessor: "plane.View of CanonicalResultCell.Payload"},
	}
}
