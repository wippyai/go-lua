package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// ReturnFlowKind classifies return-flow relations.
type ReturnFlowKind uint8

const (
	ReturnFlowInvalid ReturnFlowKind = iota
	ReturnFlowParam
	ReturnFlowParamMember
)

// ReturnFlow records a must relation from one return slot to a parameter or its
// static member path.
type ReturnFlow struct {
	ReturnIndex int
	Kind        ReturnFlowKind
	Param       int
	Path        []segment.Segment
}

type returnFlowKey struct {
	returnIndex int
	kind        ReturnFlowKind
	param       int
	path        pathdom.PathKey
}

func returnFlowParam(returnIndex, param int) ReturnFlow {
	return ReturnFlow{ReturnIndex: returnIndex, Kind: ReturnFlowParam, Param: param}
}

func returnFlowParamMember(returnIndex, param int, path []segment.Segment) ReturnFlow {
	return ReturnFlow{
		ReturnIndex: returnIndex,
		Kind:        ReturnFlowParamMember,
		Param:       param,
		Path:        append([]segment.Segment(nil), path...),
	}
}

var returnFlowLane returnFlowFactLane

type returnFlowFactLane struct{}

func (returnFlowFactLane) Normalize(in []ReturnFlow) []ReturnFlow {
	if len(in) == 0 {
		return nil
	}
	return normalizeReturnFlows(in, true)
}

func (returnFlowFactLane) NormalizeOwned(in []ReturnFlow) []ReturnFlow {
	if len(in) == 0 {
		return nil
	}
	return normalizeReturnFlows(in, false)
}

func (lane returnFlowFactLane) Equal(a, b []ReturnFlow) bool {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if returnFlowKeyOf(a[i]) != returnFlowKeyOf(b[i]) {
			return false
		}
	}
	return true
}

func (lane returnFlowFactLane) LessOrEq(a, b []ReturnFlow) bool {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(b) == 0 {
		return true
	}
	if len(a) == 0 {
		return false
	}
	byReturn := make(map[int]returnFlowKey, len(a))
	for _, flow := range a {
		key := returnFlowKeyOf(flow)
		byReturn[key.returnIndex] = key
	}
	for _, flow := range b {
		key := returnFlowKeyOf(flow)
		if got, ok := byReturn[key.returnIndex]; !ok || got != key {
			return false
		}
	}
	return true
}

func (lane returnFlowFactLane) Join(a, b []ReturnFlow) []ReturnFlow {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	right := make(map[int]returnFlowKey, len(b))
	for _, flow := range b {
		key := returnFlowKeyOf(flow)
		right[key.returnIndex] = key
	}
	var out []ReturnFlow
	for _, flow := range a {
		key := returnFlowKeyOf(flow)
		if got, ok := right[key.returnIndex]; ok && got == key {
			out = append(out, cloneReturnFlow(flow))
		}
	}
	return out
}

func (lane returnFlowFactLane) Widen(prev, next []ReturnFlow) []ReturnFlow {
	return lane.Join(prev, next)
}

func normalizeReturnFlows(in []ReturnFlow, clone bool) []ReturnFlow {
	byReturn := make(map[int]ReturnFlow, len(in))
	conflicts := make(map[int]struct{})
	for _, flow := range in {
		normalized, ok := normalizeReturnFlow(flow, clone)
		if !ok {
			continue
		}
		returnIndex := normalized.ReturnIndex
		if _, conflict := conflicts[returnIndex]; conflict {
			continue
		}
		if existing, ok := byReturn[returnIndex]; ok {
			if returnFlowKeyOf(existing) != returnFlowKeyOf(normalized) {
				delete(byReturn, returnIndex)
				conflicts[returnIndex] = struct{}{}
			}
			continue
		}
		byReturn[returnIndex] = normalized
	}
	if len(byReturn) == 0 {
		return nil
	}
	out := make([]ReturnFlow, 0, len(byReturn))
	for _, flow := range byReturn {
		out = append(out, flow)
	}
	sort.Slice(out, func(i, j int) bool {
		return returnFlowLess(out[i], out[j])
	})
	return out
}

func normalizeReturnFlow(flow ReturnFlow, clone bool) (ReturnFlow, bool) {
	if flow.ReturnIndex < 0 || flow.Param < 0 {
		return ReturnFlow{}, false
	}
	switch flow.Kind {
	case ReturnFlowParam:
		flow.Path = nil
	case ReturnFlowParamMember:
		if len(flow.Path) == 0 {
			return ReturnFlow{}, false
		}
		if _, ok := pathaddr.RelativeStaticMemberSuffixKey(flow.Path); !ok {
			return ReturnFlow{}, false
		}
		if clone {
			flow.Path = append([]segment.Segment(nil), flow.Path...)
		}
	default:
		return ReturnFlow{}, false
	}
	return flow, true
}

func cloneReturnFlow(flow ReturnFlow) ReturnFlow {
	flow.Path = append([]segment.Segment(nil), flow.Path...)
	return flow
}

func returnFlowKeyOf(flow ReturnFlow) returnFlowKey {
	return returnFlowKey{
		returnIndex: flow.ReturnIndex,
		kind:        flow.Kind,
		param:       flow.Param,
		path:        pathdom.Path{Segments: flow.Path}.Key(),
	}
}

func returnFlowLess(a, b ReturnFlow) bool {
	left := returnFlowKeyOf(a)
	right := returnFlowKeyOf(b)
	if left.returnIndex != right.returnIndex {
		return left.returnIndex < right.returnIndex
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	if left.param != right.param {
		return left.param < right.param
	}
	return left.path < right.path
}
