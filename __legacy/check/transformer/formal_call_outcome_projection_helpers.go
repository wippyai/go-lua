package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
)

func denseCallResults(reg *axis.Registry, results []callpayload.CallResult) []product.Value {
	width := 0
	for _, result := range results {
		if result.Index >= width {
			width = result.Index + 1
		}
	}
	out := make([]product.Value, width)
	for index := range out {
		out[index] = product.Absent(reg)
	}
	for _, result := range results {
		if result.Index >= 0 {
			out[result.Index] = result.Value
		}
	}
	return out
}

func declaredBoundaryReturns(body *relationProgramBody) []product.Value {
	if body == nil || body.relation.descriptors == nil {
		return nil
	}
	handler, ok := body.relation.descriptors.handlers[DescriptorReturn].(returnHandler)
	if !ok {
		return nil
	}
	return append([]product.Value(nil), handler.declared...)
}

func projectReturnPresenceTransaction(transaction factapply.CallResultTransaction) []callpayload.CallReturnPresenceRelation {
	out := make([]callpayload.CallReturnPresenceRelation, 0)
	for index := 0; index < transaction.Len(); index++ {
		step, present := transaction.Step(index)
		if !present {
			continue
		}
		relation, publication := step.ReturnPresenceRelation()
		if !publication {
			continue
		}
		out = append(out, callpayload.CallReturnPresenceRelation{
			TriggerIndex: relation.TriggerIndex(), TriggerPresence: relation.TriggerPresence(),
			TargetIndex: relation.TargetIndex(), TargetPresence: relation.TargetPresence(),
		})
	}
	return out
}

func appendUniqueReturnPresence(out []callpayload.CallReturnPresenceRelation, candidate callpayload.CallReturnPresenceRelation) []callpayload.CallReturnPresenceRelation {
	for _, prior := range out {
		if equalReturnPresence(prior, candidate) {
			return out
		}
	}
	return append(out, candidate)
}

func equalReturnPresence(a, b callpayload.CallReturnPresenceRelation) bool {
	return a.TriggerIndex == b.TriggerIndex && a.TargetIndex == b.TargetIndex &&
		presence.Equal(a.TriggerPresence, b.TriggerPresence) && presence.Equal(a.TargetPresence, b.TargetPresence)
}
