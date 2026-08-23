package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
)

// runtimeInputProjection is the sealed address of one dense Group input port.
// The source point, source Input identity, source provenance and composition
// generation are all issued by the equation Graph at seal.  The read factor
// is copied from the generated CompiledRule read plan when that port is used
// by a generated member.  Runtime therefore consumes one owner-issued source
// address and one already-lowered carrier transport; it cannot rediscover a
// point, factor, or generation from the live graph.
//
// The projection intentionally does not retain a PointState. State is
// contextual and changes per epoch. The executor combines this immutable
// source point with the epoch's one state address, then applies the same
// sealed transport. There is one point plane and one transport boundary, not
// a second generated-input state representation.
type runtimeInputProjection struct {
	sourcePoint       int
	sourceKey         composition.Key
	sourceInputKey    composition.Key
	sourceProvenance  composition.Key
	sourceComposition composition.ID
	sourceGeneration  identity.Generation
	readFactor        uint32
	readFactorKey     composition.Key
	readFactorSlot    shape.Slot
	readFactorPresent bool
	transport         runtimeInput
}

type runtimeInputFactor struct {
	ordinal uint32
	key     composition.Key
	slot    shape.Slot
}

// sealRuntimeInputFactors projects the read-factor requirements of every
// generated member in one Group. A dense port may have one required factor or
// none (for a legacy-only port); two distinct factors on the same port are an
// ambiguous second authority and refuse. The factor record is checked against
// the already sealed runtime program, so this function never manufactures a
// factor from a slot or from an occurrence surface.
func sealRuntimeInputFactors(program *runtimeProgram, span memberSpan, inputCount int) ([]runtimeInputFactor, []bool, bool) {
	if program == nil || !program.valid() || inputCount < 0 || span.count() < 0 {
		return nil, nil, false
	}
	rows := program.memberRows(span)
	if len(rows) != span.count() {
		return nil, nil, false
	}
	factors := make([]runtimeInputFactor, inputCount)
	present := make([]bool, inputCount)
	for _, row := range rows {
		if !row.valid() {
			return nil, nil, false
		}
		if row.generated == nil {
			continue
		}
		descriptor, descriptorOK := program.generatedProgramAt(row.generated.rule)
		if !descriptorOK || descriptor.InputCount() != inputCount {
			return nil, nil, false
		}
		for readIndex := 0; readIndex < descriptor.ReadCount(); readIndex++ {
			read, readOK := descriptor.ReadAt(readIndex)
			if !readOK || int(read.Input) < 0 || int(read.Input) >= inputCount || read.Factor == ^uint32(0) {
				return nil, nil, false
			}
			record, recordOK := program.factorRecordAt(int(read.Factor))
			if !recordOK || !record.valid() {
				// A read factor is an independent sealed coordinate. The
				// CompiledRule read plan, not the output row or a runtime slot,
				// is the sole authority for this port's required factor.
				return nil, nil, false
			}
			requirement := runtimeInputFactor{ordinal: read.Factor, key: record.key, slot: record.slot}
			input := int(read.Input)
			if present[input] && factors[input] != requirement {
				return nil, nil, false
			}
			factors[input], present[input] = requirement, true
		}
	}
	return factors, present, true
}

// sealRuntimeInputProjection converts the already constructed Group input
// rows into dense graph-point ordinals and joins them to the CompiledRule
// factor requirements. The Group row is the owner-issued source geometry;
// this function resolves its address exactly once in the committed graph and
// attaches the transport assembled for that same row.
//
// No input is omitted, reordered, or recovered by a fallback. Arity and
// ownership are closed at this boundary, so a foreign or holey relation
// refuses during assembly instead of being rediscovered at runtime.
func sealRuntimeInputProjection(graph *equation.Graph, program *runtimeProgram, plan *linkexecutionplan.LinkExecutionPlan, span memberSpan, group equation.GroupNode, transports []runtimeInput) ([]runtimeInputProjection, bool) {
	if graph == nil || program == nil || !program.valid() || !graph.OwnsGroup(group) || !group.Key().Available() || group.InputCount() < 0 || len(transports) != group.InputCount() || !graph.CompositionID().Available() {
		return nil, false
	}
	generation := identity.Generation(0)
	if plan != nil {
		if !plan.Available() || plan.Graph() != graph || !plan.Generation().Available() {
			return nil, false
		}
		generation = plan.Generation()
	}
	factors, factorPresent, factorsOK := sealRuntimeInputFactors(program, span, group.InputCount())
	if !factorsOK {
		return nil, false
	}
	projection := make([]runtimeInputProjection, group.InputCount())
	for index := range projection {
		input, inputOK := group.InputAt(index)
		if !inputOK || !input.Point().Available() || !input.Key().Available() || !input.Provenance().Available() || !transports[index].valid() || transports[index].key != input.Key() || transports[index].provenance != input.Provenance() {
			return nil, false
		}
		pointIndex, indexed := graph.PointIndex(input.Point())
		if !indexed || pointIndex < 0 || pointIndex >= graph.PointCount() {
			return nil, false
		}
		point, pointOK := graph.PointAt(schedule.Node(pointIndex))
		if !pointOK || !graph.OwnsPoint(point) || !point.Available() || point.Key() != input.Point().Key() {
			return nil, false
		}
		projection[index] = runtimeInputProjection{
			sourcePoint:       pointIndex,
			sourceKey:         point.Key(),
			sourceInputKey:    input.Key(),
			sourceProvenance:  input.Provenance(),
			sourceComposition: graph.CompositionID(),
			sourceGeneration:  generation,
			readFactorPresent: factorPresent[index],
			transport:         transports[index],
		}
		if factorPresent[index] {
			projection[index].readFactor = factors[index].ordinal
			projection[index].readFactorKey = factors[index].key
			projection[index].readFactorSlot = factors[index].slot
		}
		if !projection[index].validFor(graph, generation) {
			return nil, false
		}
	}
	return projection, true
}

// validFor authenticates the immutable source projection against the exact
// graph and generation that issued it. A copied projection from a different
// mounted revision is therefore refused before transport, even when its
// point ordinal and semantic key happen to match.
func (projection runtimeInputProjection) validFor(graph *equation.Graph, generation identity.Generation) bool {
	if graph == nil || projection.sourcePoint < 0 || projection.sourcePoint >= graph.PointCount() || !projection.sourceKey.Available() || !projection.sourceInputKey.Available() || !projection.sourceProvenance.Available() || projection.sourceComposition != graph.CompositionID() || !projection.validTransport() {
		return false
	}
	if projection.sourceGeneration != generation {
		return false
	}
	point, pointOK := graph.PointAt(schedule.Node(projection.sourcePoint))
	return pointOK && graph.OwnsPoint(point) && point.Available() && point.Key() == projection.sourceKey
}

// validTransport is the post-seal, graph-free runtime check. It checks only
// immutable handles and the already-lowered carrier boundary; no graph point,
// source row, generation, or factor directory is looked up here.
func (projection runtimeInputProjection) validTransport() bool {
	if !projection.sourceKey.Available() || !projection.sourceInputKey.Available() || !projection.sourceProvenance.Available() || !projection.transport.valid() || projection.transport.key != projection.sourceInputKey || projection.transport.provenance != projection.sourceProvenance {
		return false
	}
	if projection.readFactorPresent {
		return projection.readFactor != ^uint32(0) && projection.readFactorKey.Available() && projection.readFactorSlot >= 0
	}
	return projection.readFactor == 0 && projection.readFactorKey == (composition.Key{}) && projection.readFactorSlot == 0
}

// factorOwnedBy authenticates the sealed read factor against the runtime
// program that issued the projection. It is deliberately a table check, not
// a graph lookup or a factor-owner rediscovery: the read plan's dense ordinal,
// canonical key, and carrier slot must remain one tuple.
func (projection runtimeInputProjection) factorOwnedBy(program *runtimeProgram) bool {
	if !projection.validTransport() {
		return false
	}
	if !projection.readFactorPresent {
		return true
	}
	record, ok := program.factorRecordAt(int(projection.readFactor))
	return ok && record.valid() && record.key == projection.readFactorKey && record.slot == projection.readFactorSlot
}

// factorRootPresent checks the required factor's opaque carrier root without
// opening a typed domain plane. TransportPointState carries the complete root
// vector, so checking this slot before and after transport proves the sealed
// factor is not silently dropped while leaving value interpretation to the
// Factor owner.
func (projection runtimeInputProjection) factorRootPresent(state carrier.PointState) bool {
	if !projection.readFactorPresent {
		return true
	}
	_, ok := state.HandleAt(projection.readFactorSlot)
	return ok
}

func (producer *runtimeProducer) inputProjectionAt(index int) (runtimeInputProjection, bool) {
	if producer == nil || index < 0 || index >= len(producer.inputProjection) {
		return runtimeInputProjection{}, false
	}
	projection := producer.inputProjection[index]
	if !projection.validTransport() {
		return runtimeInputProjection{}, false
	}
	return projection, true
}
