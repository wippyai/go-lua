package wire

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
	"github.com/wippyai/go-lua/types/signature"
)

// ScopedErrorType returns this module's canonical error type carried into the
// module's own namespace, the same namespace ScopeSignature carries a declared
// callable into. A correlation is decided by comparing the two, so both sides
// must be read through one scoping act.
func (m *Manifest) ScopedErrorType() typ.Type {
	if m == nil || m.ErrorType == nil {
		return nil
	}
	return m.ScopeType(m.ErrorType)
}

// CorrelatesValueAndError reports whether a callable answers the value/error
// pair the module's error type declares: a trailing Optional(error) result
// preceded by at least one value result. A single trailing optional error
// correlates nothing, because there is no value slot for the error to exclude.
func CorrelatesValueAndError(errorType typ.Type, sig signature.Function) bool {
	_, ok := valueErrorReturns(errorType, sig)
	return ok
}

// ValueErrorOperation derives the two correlated normal arms of a fallible
// member from the module's declared error type. The success arm answers the
// declared values with no error; the failure arm answers nil for every value
// slot with the error present.
//
// The derivation states only the correlation. A value result crosses onto the
// success arm exactly as the provider declared it, so a member that may answer
// nil on success keeps that nil; only the failure arm adds one, and it adds it
// because a member that reports an error has no value to answer beside it.
func ValueErrorOperation(errorType typ.Type, sig signature.Function) (Operation, bool) {
	returns, ok := valueErrorReturns(errorType, sig)
	if !ok {
		return Operation{}, false
	}
	failure := returns[len(returns)-1]
	present := make([]typ.Type, 0, len(returns))
	absent := make([]typ.Type, 0, len(returns))
	for _, value := range returns[:len(returns)-1] {
		present = append(present, value)
		absent = append(absent, typ.Nil)
	}
	present = append(present, typ.Nil)
	absent = append(absent, failure)
	return Operation{
		ReplaceNormalSet: true,
		ReplaceNormal: []Values{
			{Fixed: present, Tail: ValuesClosed},
			{Fixed: absent, Tail: ValuesClosed},
		},
	}, true
}

// AuthorsNormalArms reports whether a provider law already states the normal
// outcome set of its callable. A derived correlation replaces that set, so it
// applies only where the provider left it to the signature.
func (o Operation) AuthorsNormalArms() bool {
	return o.Replace || o.ReplaceNormalSet || len(o.AppendNormal) != 0 || len(o.Outcomes) != 0
}

// AddressesOutcomes reports whether a provider law addresses outcomes by
// ordinal. Such a law reads the outcome set it was written against, so the
// outcome set may not be derived underneath it.
func (o Operation) AddressesOutcomes() bool {
	if len(o.OutcomeAmendments) != 0 || len(o.OutcomeTailTypes) != 0 || len(o.Acquisitions) != 0 {
		return true
	}
	if len(o.Suspensions) != 0 || len(o.Spawns) != 0 || len(o.Resumes) != 0 || o.SubedgeRelation != nil {
		return true
	}
	if o.Behavior != nil && (len(o.Behavior.Results) != 0 || len(o.Behavior.Predicates) != 0) {
		return true
	}
	for _, transfer := range o.Transfers {
		if len(transfer.Outcomes) != 0 {
			return true
		}
	}
	return false
}

// valueErrorReturns returns the callable's declared results with the trailing
// optional error replaced by the error type it wraps, when the callable
// answers the module's value/error pair.
func valueErrorReturns(errorType typ.Type, sig signature.Function) ([]typ.Type, bool) {
	if errorType == nil || sig.Type == nil {
		return nil, false
	}
	// A tail or suffix result makes the return geometry open, so the last
	// declared result is not the last result the call answers.
	if sig.ResultTail != nil || len(sig.ResultSuffix) != 0 {
		return nil, false
	}
	returns := sig.Type.Returns
	if len(returns) < 2 {
		return nil, false
	}
	failure, ok := optionalMember(returns[len(returns)-1])
	if !ok || !typ.TypeEquals(failure, errorType) {
		return nil, false
	}
	out := make([]typ.Type, len(returns))
	copy(out, returns)
	out[len(out)-1] = failure
	return out, true
}

// optionalMember returns the single non-nil member of an optional type. It
// accepts both spellings the type expressions produce for one nullable
// declaration: an Optional node and a two-member union with nil.
func optionalMember(t typ.Type) (typ.Type, bool) {
	switch value := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		if value.Inner == nil || value.Inner.Kind() == typ.Nil.Kind() {
			return nil, false
		}
		return value.Inner, true
	case *typ.Union:
		var member typ.Type
		nilable := false
		for _, candidate := range value.Members {
			if candidate == nil {
				return nil, false
			}
			if candidate.Kind() == typ.Nil.Kind() {
				nilable = true
				continue
			}
			if member != nil {
				return nil, false
			}
			member = candidate
		}
		if !nilable || member == nil {
			return nil, false
		}
		return member, true
	}
	return nil, false
}
