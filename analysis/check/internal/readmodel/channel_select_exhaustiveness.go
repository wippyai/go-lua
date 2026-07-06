package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
)

// ForEachChannelSelectExhaustiveness visits channel.select elseif chains that
// do not handle every selectable case and do not have a select default.
func (r Reader) ForEachChannelSelectExhaustiveness(visit func(ChannelSelectExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	graph := r.result.Graph()
	selects := r.channelSelectInfos(graph)
	if len(selects) == 0 {
		return false
	}
	cases := newReadmodelChannelSelectCaseIndex(selects)
	reachability := cfg.NewReachability(graph)
	visited := false
	for _, chain := range r.result.IfBranchChains() {
		if !chain.HasElseIf() {
			continue
		}
		item, ok := r.channelSelectChainExhaustiveness(graph, reachability, chain, selects, cases)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

type readmodelSelectInfo struct {
	point      cfg.Point
	result     path.Path
	cases      []readmodelSelectCase
	hasDefault bool
}

type readmodelSelectCase struct {
	path path.Path
	name string
}

type readmodelChannelSelectCaseIndex map[readmodelChannelSelectCaseKey][]readmodelChannelSelectCaseMatch

type readmodelChannelSelectCaseKey struct {
	resultChannel path.PathKey
	channel       path.PathKey
}

type readmodelChannelSelectCaseMatch struct {
	selectIndex int
	caseIndex   int
}

func (r Reader) channelSelectInfos(graph cfg.Graph) []readmodelSelectInfo {
	var out []readmodelSelectInfo
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		out = append(out, r.channelSelectInfosAt(point)...)
	}
	return out
}

func (r Reader) channelSelectInfosAt(point cfg.Point) []readmodelSelectInfo {
	events := r.result.ChannelSelects(point)
	if len(events) == 0 {
		return nil
	}
	byID := make(map[factflow.ChannelSelectID]*readmodelSelectInfo)
	var order []factflow.ChannelSelectID
	for _, event := range events {
		id := event.SelectID()
		if id == "" {
			continue
		}
		info := byID[id]
		if info == nil {
			info = &readmodelSelectInfo{point: point}
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
			name := casePath.DisplayRoot(r.result.SymbolName)
			if name == "" {
				name = casePath.String()
			}
			info.cases = append(info.cases, readmodelSelectCase{
				path: casePath,
				name: name,
			})
		}
	}
	out := make([]readmodelSelectInfo, 0, len(order))
	for _, id := range order {
		info := byID[id]
		if info == nil || info.result.IsEmpty() || len(info.cases) == 0 {
			continue
		}
		out = append(out, *info)
	}
	return out
}

func (r Reader) channelSelectChainExhaustiveness(
	graph cfg.Graph,
	reachability *cfg.Reachability,
	chain body.IfBranchChain,
	selects []readmodelSelectInfo,
	cases readmodelChannelSelectCaseIndex,
) (ChannelSelectExhaustiveness, bool) {
	handledBySelect := make(map[int]map[int]bool)
	for _, branch := range chain.Branches {
		if branch.Check.Kind != branchcond.CheckPathEqual {
			continue
		}
		for _, match := range cases.matchesForCheck(branch.Check) {
			if match.selectIndex < 0 || match.selectIndex >= len(selects) {
				continue
			}
			if !readmodelSelectCanReachBranch(selects[match.selectIndex], chain.Head.Point, graph, reachability) {
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
	selected, handled, ok := readmodelBestChannelSelectCandidate(handledBySelect)
	if !ok {
		return ChannelSelectExhaustiveness{}, false
	}
	info := selects[selected]
	if info.hasDefault {
		return ChannelSelectExhaustiveness{}, false
	}
	if len(handled) >= len(info.cases) {
		return ChannelSelectExhaustiveness{}, false
	}
	var handledNames []string
	var missing []string
	for i, c := range info.cases {
		if handled[i] {
			handledNames = appendUniqueReadmodelString(handledNames, c.name)
		} else {
			missing = appendUniqueReadmodelString(missing, c.name)
		}
	}
	if len(missing) == 0 {
		return ChannelSelectExhaustiveness{}, false
	}
	return ChannelSelectExhaustiveness{
		Point:         chain.Head.Point,
		Span:          sourceSpanFromBody(chain.Head.ConditionSpan),
		ResultChannel: info.result.Field(channelselect.ResultChannelField).String(),
		Handled:       handledNames,
		Missing:       missing,
		HasDefault:    info.hasDefault,
	}, true
}

func readmodelSelectCanReachBranch(info readmodelSelectInfo, branchPoint cfg.Point, graph cfg.Graph, reachability *cfg.Reachability) bool {
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

func readmodelBestChannelSelectCandidate(handledBySelect map[int]map[int]bool) (int, map[int]bool, bool) {
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

func newReadmodelChannelSelectCaseIndex(selects []readmodelSelectInfo) readmodelChannelSelectCaseIndex {
	out := make(readmodelChannelSelectCaseIndex)
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
			key := readmodelChannelSelectCaseKey{resultChannel: resultKey, channel: channelKey}
			out[key] = append(out[key], readmodelChannelSelectCaseMatch{
				selectIndex: selectIndex,
				caseIndex:   caseIndex,
			})
		}
	}
	return out
}

func (idx readmodelChannelSelectCaseIndex) matchesForCheck(check branchcond.Check) []readmodelChannelSelectCaseMatch {
	matches := idx[readmodelChannelSelectCaseKey{resultChannel: check.Path.Key(), channel: check.OtherPath.Key()}]
	if len(matches) == 0 {
		matches = idx[readmodelChannelSelectCaseKey{resultChannel: check.OtherPath.Key(), channel: check.Path.Key()}]
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func appendUniqueReadmodelString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
