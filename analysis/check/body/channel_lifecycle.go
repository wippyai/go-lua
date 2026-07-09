package body

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/ambient"
)

// ChannelLifecycleOperation classifies a runtime channel operation that is
// invalid when the receiver is proven closed.
type ChannelLifecycleOperation string

const (
	ChannelLifecycleSend  ChannelLifecycleOperation = "send"
	ChannelLifecycleClose ChannelLifecycleOperation = "close"
)

// ChannelLifecycleMisuseProof records a channel operation whose receiver is
// provably closed at the call entry.
type ChannelLifecycleMisuseProof struct {
	Point     cfg.Point
	Operation ChannelLifecycleOperation
	Channel   string
	State     string
	Span      SourceSpan
}

// ChannelLifecycleMisuseProofs returns send/close calls whose channel receiver
// is provably closed at call entry. Unknown, joined, or escaped channel state is
// intentionally silent.
func (r *Result) ChannelLifecycleMisuseProofs() []ChannelLifecycleMisuseProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	receiverType := channelMethodReceiverTypeProvider(r.registry, r.facts, r.visibility, r.sources, r.typeValues)
	var out []ChannelLifecycleMisuseProof
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		site, ok := r.CallSiteView(point)
		if !ok {
			continue
		}
		operation, ok := channelLifecycleOperation(site.MethodName())
		if !ok {
			continue
		}
		receiver, ok := site.ReceiverPath()
		if !ok || receiver.IsEmpty() {
			continue
		}
		in, ok := r.StateAt(point)
		if !ok {
			continue
		}
		ctx := transfer.NodeContext{
			Graph:    r.Graph(),
			Registry: r.registry,
			Point:    point,
			Read:     r.boundaryRead,
		}
		if ctx.Graph != nil {
			ctx.Node = ctx.Graph.Node(point)
		}
		receiverStaticType, ok := receiverType(ctx, site, in, r.boundaryRead)
		if !ok {
			continue
		}
		if _, ok := ambient.ChannelPayloadType(receiverStaticType); !ok {
			continue
		}
		resource, ok := r.TypestateResourceAtCallEntry(point, receiver, effectlowering.ChannelLifecycleProtocol)
		if !ok {
			continue
		}
		slot, ok := in.TypestateSlot(resource)
		if !channelSlotProvablyClosed(slot, ok) {
			continue
		}
		out = append(out, ChannelLifecycleMisuseProof{
			Point:     point,
			Operation: operation,
			Channel:   r.DisplayPath(receiver),
			State:     string(slot.Current),
			Span:      r.callSpanAt(point),
		})
	}
	return out
}

func channelLifecycleOperation(method string) (ChannelLifecycleOperation, bool) {
	switch method {
	case "send":
		return ChannelLifecycleSend, true
	case "close":
		return ChannelLifecycleClose, true
	default:
		return "", false
	}
}

func channelSlotProvablyClosed(slot typestate.Slot, ok bool) bool {
	if !ok || slot.Current != effectlowering.ChannelStateClosed {
		return false
	}
	return slot.Locality != typestate.LocalityUnknown &&
		slot.Locality != typestate.LocalityEscaped &&
		slot.Locality != typestate.LocalityBottom
}
