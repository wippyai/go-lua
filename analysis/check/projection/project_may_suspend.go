package projection

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type pointMaySuspendReader interface {
	PointMaySuspend(cfg.Point) bool
}

type pointNormallyReachableReader interface {
	PointNormallyReachable(cfg.Point) bool
}

type channelSelectsReader interface {
	ChannelSelects(cfg.Point) []factflow.ChannelSelect
}

func projectMaySuspend(result ResultReader) bool {
	if result == nil {
		return false
	}
	graph := result.Graph()
	if graph == nil {
		return false
	}
	if reader, ok := result.(pointMaySuspendReader); ok {
		for _, point := range graph.RPO() {
			if !summaryPointNormallyReachable(result, point) {
				continue
			}
			if reader.PointMaySuspend(point) {
				return true
			}
		}
		return false
	}
	for _, point := range graph.RPO() {
		if !summaryPointNormallyReachable(result, point) {
			continue
		}
		if resultPointMaySuspendFallback(result, point) {
			return true
		}
	}
	return false
}

func summaryPointNormallyReachable(result ResultReader, point cfg.Point) bool {
	if reader, ok := result.(pointNormallyReachableReader); ok {
		return reader.PointNormallyReachable(point)
	}
	if reader, ok := result.(stateAtReader); ok {
		_, ok := reader.StateAt(point)
		return ok
	}
	return true
}

func resultPointMaySuspendFallback(result ResultReader, point cfg.Point) bool {
	if reader, ok := result.(channelSelectsReader); ok && len(reader.ChannelSelects(point)) != 0 {
		return true
	}
	if !hasCallSiteView(result) {
		return false
	}
	if _, ok := callSiteViewAt(result, point); !ok {
		return false
	}
	reader, ok := result.(callOutcomeAtReader)
	if !ok {
		return true
	}
	outcome, ok := reader.CallOutcomeAt(point)
	if !ok || !outcome.SuspensionKnown {
		return true
	}
	return outcome.MaySuspend
}
