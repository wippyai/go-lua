package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ForEachHoistableLoad visits codegen licenses projected from the same
// body-owned occurrence stream consumed by invariant-loop-read advice. Keep
// the raw-load witness check at this codegen boundary even though the body
// stream also filters it: absence of that witness must never become a license.
func (r Reader) ForEachHoistableLoad(visit func(HoistableLoad) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	r.result.ForEachInvariantLoopReadOccurrence(func(occ body.InvariantLoopReadOccurrence) bool {
		if !occ.RawLoadWitness {
			return true
		}
		visited = true
		return visit(hoistableLoadFromBody(r, occ))
	})
	return visited
}

func hoistableLoadFromBody(r Reader, occ body.InvariantLoopReadOccurrence) HoistableLoad {
	var bodyID lexicalidentity.StableLexicalBodyID
	if r.result != nil {
		bodyID = r.result.StableLexicalBodyID()
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
