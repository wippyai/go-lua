package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

func wideningThresholdsFromWIR(body *wir.Body) []int64 {
	thresholds := map[int64]struct{}{
		0: {},
		1: {},
	}
	if body == nil {
		return sortedThresholds(thresholds)
	}
	body.ForEachConst(func(c wir.Const) bool {
		if c.Kind == wir.ConstNumber {
			if value, ok := numparse.ParseIntegerLiteral(c.Number); ok {
				thresholds[value] = struct{}{}
			}
		}
		return true
	})
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpMakeTable {
			continue
		}
		for _, entry := range body.TableEntries(inst.TableEntries) {
			for _, seg := range entry.Suffix.Segments {
				if seg.Kind == segment.SegmentIndexInt && seg.Index > 0 {
					thresholds[int64(seg.Index)] = struct{}{}
				}
			}
		}
	}
	return sortedThresholds(thresholds)
}

func sortedThresholds(values map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
