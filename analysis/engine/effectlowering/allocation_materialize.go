package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// MaterializedStaticAllocation is the canonical point-specialized projection
// of one closed allocation template. It is produced during Relation
// specialization, never retained by operationplan.Plan.
type MaterializedStaticAllocation struct {
	Result     product.Value
	Objects    map[identity.ID]heapidentity.TableObject
	Placements map[identity.ID]placement.Value
	KeySpace   *keyspace.KeySpace
}

func MaterializeStaticAllocation(reg *axis.Registry, typeValues *typevalue.Cache, ks *keyspace.KeySpace, point cfg.Point, template signature.ReturnAllocationTemplate) (MaterializedStaticAllocation, bool) {
	if reg == nil || ks == nil || point == 0 || template.Root == "" || len(template.Objects) == 0 {
		return MaterializedStaticAllocation{}, false
	}
	var rootType typ.Type
	for _, object := range template.Objects {
		if object.ID == template.Root {
			rootType = object.Type
			break
		}
	}
	if rootType == nil || typ.ContainsTypeParam(rootType) {
		return MaterializedStaticAllocation{}, false
	}
	fn := typ.Func().Returns(rootType).Build()
	effects := signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{template}}
	result := returnValueFromSignatureTypeCached(reg, typeValues, fn, rootType)
	result = operationalReturnAllocationValue(reg, typeValues, &effects, fn, point, template.ReturnIndex, result)
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	objects := operationalHeapTableObjects(ctx, typeValues, ks, fn, effects)
	placements := operationalAllocationPlacements(point, effects)
	if len(objects) == 0 || len(placements) == 0 {
		return MaterializedStaticAllocation{}, false
	}
	return MaterializedStaticAllocation{Result: result, Objects: objects, Placements: placements, KeySpace: ks}, true
}
