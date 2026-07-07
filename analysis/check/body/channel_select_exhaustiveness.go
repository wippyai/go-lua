package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
)

// ChannelSelectExhaustivenessProof records an elseif chain that handles only a
// subset of a channel.select result's possible cases and has no select default.
type ChannelSelectExhaustivenessProof struct {
	Point         cfg.Point
	Span          SourceSpan
	ResultChannel string
	Handled       []string
	Missing       []string
	HasDefault    bool
}

type channelSelectInfo struct {
	selectID   factflow.ChannelSelectID
	point      cfg.Point
	result     path.Path
	cases      []channelSelectCase
	hasDefault bool
}

type channelSelectCase struct {
	path  path.Path
	name  string
	index int
}

type channelSelectCaseIndex map[channelSelectCaseKey][]channelSelectCaseMatch

type channelSelectCaseKey struct {
	resultChannel path.PathKey
	channel       path.PathKey
}

type channelSelectCaseMatch struct {
	selectIndex int
	caseIndex   int
}

// ChannelSelectExhaustivenessProofs returns channel.select elseif chains whose
// handled channel cases do not cover every still-possible select case.
func (r *Result) ChannelSelectExhaustivenessProofs() []ChannelSelectExhaustivenessProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	graph := r.Graph()
	selects := r.channelSelectInfos(graph)
	if len(selects) == 0 {
		return nil
	}
	cases := newChannelSelectCaseIndex(selects)
	reachability := cfg.NewReachability(graph)
	var out []ChannelSelectExhaustivenessProof
	for _, chain := range r.IfBranchChains() {
		if !chain.HasElseIf() {
			continue
		}
		item, ok := r.channelSelectChainExhaustiveness(graph, reachability, chain, selects, cases)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func (r *Result) channelSelectInfos(graph cfg.Graph) []channelSelectInfo {
	var out []channelSelectInfo
	for _, point := range graph.RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		out = append(out, r.channelSelectInfosAt(point)...)
	}
	return out
}

func (r *Result) channelSelectInfosAt(point cfg.Point) []channelSelectInfo {
	events := r.ChannelSelects(point)
	if len(events) == 0 {
		return nil
	}
	byID := make(map[factflow.ChannelSelectID]*channelSelectInfo)
	var order []factflow.ChannelSelectID
	for _, event := range events {
		id := event.SelectID()
		if id == "" {
			continue
		}
		info := byID[id]
		if info == nil {
			info = &channelSelectInfo{selectID: id, point: point}
			byID[id] = info
			order = append(order, id)
		}
		switch event.Kind() {
		case factflow.ChannelSelectSelect:
			resultPath, ok := event.ResultPath()
			if !ok || resultPath.IsEmpty() {
				continue
			}
			info.result = resultPath
			info.hasDefault = event.HasDefault()
		case factflow.ChannelSelectCase:
			casePath, ok := event.CasePath()
			if !ok || casePath.IsEmpty() {
				continue
			}
			name := casePath.DisplayRoot(r.SymbolName)
			if name == "" {
				name = casePath.String()
			}
			info.cases = append(info.cases, channelSelectCase{
				path:  casePath,
				name:  name,
				index: event.Index(),
			})
		}
	}
	out := make([]channelSelectInfo, 0, len(order))
	for _, id := range order {
		info := byID[id]
		if info == nil || info.result.IsEmpty() || len(info.cases) == 0 {
			continue
		}
		out = append(out, *info)
	}
	return out
}

func (r *Result) channelSelectChainExhaustiveness(
	graph cfg.Graph,
	reachability *cfg.Reachability,
	chain IfBranchChain,
	selects []channelSelectInfo,
	cases channelSelectCaseIndex,
) (ChannelSelectExhaustivenessProof, bool) {
	handledBySelect := make(map[int]map[int]bool)
	for _, branch := range chain.Branches {
		if branch.Check.Kind != branchcond.CheckPathEqual {
			continue
		}
		for _, match := range cases.matchesForCheck(branch.Check) {
			if match.selectIndex < 0 || match.selectIndex >= len(selects) {
				continue
			}
			if !channelSelectCanReachBranch(selects[match.selectIndex], chain.Head.Point, graph, reachability) {
				continue
			}
			handled := handledBySelect[match.selectIndex]
			if handled == nil {
				handled = make(map[int]bool)
				handledBySelect[match.selectIndex] = handled
			}
			handled[match.caseIndex] = true
		}
	}
	selected, handled, ok := bestChannelSelectCandidate(handledBySelect)
	if !ok {
		return ChannelSelectExhaustivenessProof{}, false
	}
	info := selects[selected]
	if info.hasDefault {
		return ChannelSelectExhaustivenessProof{}, false
	}
	if len(handled) >= len(info.cases) {
		return ChannelSelectExhaustivenessProof{}, false
	}
	var handledNames []string
	var missing []string
	for i, c := range info.cases {
		if !r.channelSelectCaseStillPossibleAt(chain.Head.Point, info, c) {
			continue
		}
		if handled[i] {
			handledNames = appendUniqueChannelSelectString(handledNames, c.name)
		} else {
			missing = appendUniqueChannelSelectString(missing, c.name)
		}
	}
	if len(missing) == 0 {
		return ChannelSelectExhaustivenessProof{}, false
	}
	return ChannelSelectExhaustivenessProof{
		Point:         chain.Head.Point,
		Span:          chain.Head.ConditionSpan,
		ResultChannel: info.result.Field(channelselect.ResultChannelField).String(),
		Handled:       handledNames,
		Missing:       missing,
		HasDefault:    info.hasDefault,
	}, true
}

func (r *Result) channelSelectCaseStillPossibleAt(point cfg.Point, info channelSelectInfo, c channelSelectCase) bool {
	if r == nil || info.selectID == "" || info.result.IsEmpty() {
		return true
	}
	value, ok := r.PathValueAtBoundary(point, info.result)
	if !ok {
		return true
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok || !channelselect.ResultHasSelectID(t, string(info.selectID)) {
		return true
	}
	_, ok = channelselect.ResultCaseTypeFromValue(t, string(info.selectID), c.index)
	return ok
}

func channelSelectCanReachBranch(info channelSelectInfo, branchPoint cfg.Point, graph cfg.Graph, reachability *cfg.Reachability) bool {
	if graph == nil {
		return true
	}
	if info.point == branchPoint {
		return true
	}
	if reachability != nil {
		return reachability.CanReach(info.point, branchPoint)
	}
	return cfg.PointCanReach(graph, info.point, branchPoint)
}

func bestChannelSelectCandidate(handledBySelect map[int]map[int]bool) (int, map[int]bool, bool) {
	selected := -1
	var selectedHandled map[int]bool
	for selectIndex, handled := range handledBySelect {
		if len(handled) == 0 {
			continue
		}
		if selected == -1 || len(handled) > len(selectedHandled) || len(handled) == len(selectedHandled) && selectIndex > selected {
			selected = selectIndex
			selectedHandled = handled
		}
	}
	return selected, selectedHandled, selected != -1
}

func newChannelSelectCaseIndex(selects []channelSelectInfo) channelSelectCaseIndex {
	out := make(channelSelectCaseIndex)
	for selectIndex, info := range selects {
		resultChannel := info.result.Field(channelselect.ResultChannelField)
		resultKey := resultChannel.Key()
		if resultKey == "" {
			continue
		}
		for caseIndex, c := range info.cases {
			channelKey := c.path.Key()
			if channelKey == "" {
				continue
			}
			key := channelSelectCaseKey{resultChannel: resultKey, channel: channelKey}
			out[key] = append(out[key], channelSelectCaseMatch{
				selectIndex: selectIndex,
				caseIndex:   caseIndex,
			})
		}
	}
	return out
}

func (idx channelSelectCaseIndex) matchesForCheck(check branchcond.Check) []channelSelectCaseMatch {
	matches := idx[channelSelectCaseKey{resultChannel: check.Path.Key(), channel: check.OtherPath.Key()}]
	if len(matches) == 0 {
		matches = idx[channelSelectCaseKey{resultChannel: check.OtherPath.Key(), channel: check.Path.Key()}]
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func appendUniqueChannelSelectString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
