package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
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

func MaterializeStaticAllocation(reg *axis.Registry, typeValues *typevalue.Cache, ks *keyspace.KeySpace, point cfg.Point, template signature.ReturnAllocationTemplate, exactIdentities map[signature.AllocationTemplateID]identity.Term) (MaterializedStaticAllocation, bool) {
	if reg == nil || ks == nil || point == 0 {
		return MaterializedStaticAllocation{}, false
	}
	identities := allocationIdentityResolver{point: point, exact: exactIdentities}
	if exactIdentities != nil {
		if len(exactIdentities) != len(template.Objects) {
			return MaterializedStaticAllocation{}, false
		}
		for _, object := range template.Objects {
			if object.ID == "" {
				return MaterializedStaticAllocation{}, false
			}
			if _, concrete := exactIdentities[object.ID].Concrete(); !concrete {
				return MaterializedStaticAllocation{}, false
			}
		}
	}
	result, fn, ok := staticAllocationResult(reg, typeValues, template, identities)
	if !ok {
		return MaterializedStaticAllocation{}, false
	}
	effects := signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{template}}
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	objects := operationalHeapTableObjectsWithIdentities(ctx, typeValues, ks, fn, effects, identities)
	placements := operationalAllocationPlacementsWithIdentities(effects, identities)
	if len(objects) == 0 || len(placements) == 0 {
		return MaterializedStaticAllocation{}, false
	}
	return MaterializedStaticAllocation{Result: result, Objects: objects, Placements: placements, KeySpace: ks}, true
}

// StaticAllocationResult derives the relational return value of one allocation
// transaction without constructing its concrete heap graph. Allocation terms
// remain symbolic here; only MaterializeStaticAllocation may require concrete
// identities at a boundary authority.
func StaticAllocationResult(reg *axis.Registry, typeValues *typevalue.Cache, template signature.ReturnAllocationTemplate, exactIdentities map[signature.AllocationTemplateID]identity.Term) (product.Value, bool) {
	if reg == nil || len(exactIdentities) != len(template.Objects) {
		return product.Value{}, false
	}
	result, _, ok := staticAllocationResult(reg, typeValues, template, allocationIdentityResolver{exact: exactIdentities})
	return result, ok
}

func staticAllocationResult(reg *axis.Registry, typeValues *typevalue.Cache, template signature.ReturnAllocationTemplate, identities allocationIdentityResolver) (product.Value, *typ.Function, bool) {
	if reg == nil || template.Root == "" || len(template.Objects) == 0 {
		return product.Value{}, nil, false
	}
	var rootType typ.Type
	for _, object := range template.Objects {
		if object.ID == "" || !identities.term(object.ID).Valid() {
			return product.Value{}, nil, false
		}
		if object.ID == template.Root {
			rootType = object.Type
		}
	}
	if rootType == nil || typ.ContainsTypeParam(rootType) {
		return product.Value{}, nil, false
	}
	fn := typ.Func().Returns(rootType).Build()
	effects := signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{template}}
	result := returnValueFromSignatureTypeCached(reg, typeValues, fn, rootType)
	result = operationalReturnAllocationValueWithIdentities(reg, typeValues, &effects, fn, identities, template.ReturnIndex, result)
	return result, fn, true
}
