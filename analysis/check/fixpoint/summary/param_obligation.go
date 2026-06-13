package summary

// ParamMemberCallObligation records a pre-call obligation that depends on a
// callable member of another parameter. Call-boundary adaptation resolves the
// receiver argument's member type and materializes a plain ParamObligation for
// ArgParam.
type ParamMemberCallObligation struct {
	ReceiverParam    int
	Member           string
	ArgParam         int
	MemberParamIndex int
}
