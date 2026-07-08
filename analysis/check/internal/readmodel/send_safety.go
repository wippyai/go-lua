package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r Reader) sendSafetyReports(point cfg.Point, args []CallArgument) []SendSafety {
	if r.result == nil {
		return nil
	}
	occurrences := r.result.SendSafetyOccurrences(point)
	if len(occurrences) == 0 {
		return nil
	}
	var out []SendSafety
	for _, occ := range occurrences {
		arg, ok := callArgumentByIndex(args, occ.ArgumentIndex)
		if !ok {
			continue
		}
		if occ.HasArgumentValue {
			arg.Value = occ.ArgumentValue
			arg.ValueHash = r.ValueHash(occ.ArgumentValue)
			arg.TypeWithPresence, _ = r.ValueTypeWithPresence(occ.ArgumentValue)
		}
		out = append(out, SendSafety{
			Point:               occ.Point,
			Argument:            arg,
			Target:              occ.Target.Clone(),
			Recursive:           occ.Recursive,
			Verdict:             sendSafetyVerdictFromBody(occ.Verdict),
			Reason:              occ.Reason,
			Identity:            occ.Identity,
			HasIdentity:         occ.HasIdentity,
			BirthSpan:           sourceSpanFromBody(occ.BirthSpan),
			HasBirthSpan:        occ.HasBirthSpan,
			Placement:           occ.Placement,
			HasPlacement:        occ.HasPlacement,
			Frozen:              occ.Frozen,
			DirectObjectLiteral: occ.DirectObjectLiteral,
			GraphHasChildID:     occ.GraphHasChildID,
			GraphUnknown:        occ.GraphUnknown,
		})
	}
	return out
}

func callArgumentByIndex(args []CallArgument, index int) (CallArgument, bool) {
	for _, arg := range args {
		if arg.Index == index {
			return arg, true
		}
	}
	return CallArgument{}, false
}

func sendSafetyVerdictFromBody(verdict body.SendSafetyVerdict) SendSafetyVerdict {
	switch verdict {
	case body.SendSafetyProvenIsolated:
		return SendSafetyProvenIsolated
	case body.SendSafetyProvenImmutable:
		return SendSafetyProvenImmutable
	default:
		return SendSafetyUnknown
	}
}
