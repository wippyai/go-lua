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

// ParamMemberReturnSlot records that a function return slot delegates to a
// result slot from a callable member of one parameter. Call-boundary adaptation
// can then rehydrate effect-bearing return-presence relations from the concrete
// receiver argument's imported signature.
type ParamMemberReturnSlot struct {
	ReceiverParam     int
	Member            string
	ReturnIndex       int
	MemberResultIndex int
}
