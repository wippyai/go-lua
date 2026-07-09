package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ForEachHoistableLoad visits codegen licenses projected from the same
// body-owned occurrence stream consumed by invariant-loop-read advice.
func (r Reader) ForEachHoistableLoad(visit func(HoistableLoad) bool) bool {
	return projectBodyOccurrences(r, visit, (*body.Result).ForEachInvariantLoopReadOccurrence, hoistableLoadFromBody)
}

func hoistableLoadFromBody(r Reader, occ body.InvariantLoopReadOccurrence) HoistableLoad {
	var bodyID uint64
	if r.result != nil && r.result.Graph() != nil {
		bodyID = r.result.Graph().ID()
	}
	return HoistableLoad{
		SchemaVersion: readapi.HoistableLoadSchemaVersion,
		BodyID:        bodyID,
		Point:         occ.Point,
		ReadPath:      occ.ReadPath,
		LoopHead:      occ.LoopHead,
		LoopSpan:      sourceSpanFromBody(occ.LoopSpan),
	}
}
