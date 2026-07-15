package operationplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"math/bits"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ProjectionID is a stable selector understood by evaluated-root projection.
// IDs name narrow consumer products; there is deliberately no whole-State ID.
type ProjectionID string

const (
	ProjectionPointReachable ProjectionID = "body.point-reachable.v1"
	ProjectionEdgeNormal     ProjectionID = "body.edge-normal.v1"

	projectionBoundaryRootAssignment       ProjectionID = "body.boundary.root-assignment.v1"
	projectionBoundaryPathAssignment       ProjectionID = "body.boundary.path-assignment.v1"
	projectionBoundaryStaticMemberWrite    ProjectionID = "body.boundary.static-member-write.v1"
	projectionBoundaryDynamicIndexWrite    ProjectionID = "body.boundary.dynamic-index-write.v1"
	projectionBoundaryPathInvalidation     ProjectionID = "body.boundary.path-invalidation.v1"
	projectionBoundaryCovariantExposure    ProjectionID = "body.boundary.covariant-exposure.v1"
	projectionBoundaryNoNormalReturn       ProjectionID = "body.boundary.no-normal-return.v1"
	projectionBoundaryChannelSelect        ProjectionID = "body.boundary.channel-select.v1"
	projectionBoundaryPostcondition        ProjectionID = "body.boundary.postcondition.v1"
	projectionBoundaryPathRelation         ProjectionID = "body.boundary.postcondition-path-relation.v1"
	projectionBoundaryCallResult           ProjectionID = "body.boundary.call-result.v1"
	projectionBoundaryReturn               ProjectionID = "body.boundary.return.v1"
	projectionBoundaryCallProducer         ProjectionID = "body.boundary.call-producer.v1"
	projectionBoundaryCallOutcome          ProjectionID = "body.boundary.call-outcome.v1"
	ProjectionObservationAssignment        ProjectionID = "body.observation.assignment.v1"
	ProjectionObservationCallArgument      ProjectionID = "body.observation.call-argument.v1"
	ProjectionObservationCallResult        ProjectionID = "body.observation.call-result.v1"
	projectionObservationCallResultComplex ProjectionID = "body.observation.call-result-complex.v1"
	ProjectionObservationCallInvocation    ProjectionID = "body.observation.call-invocation.v1"
)

// RequirementStage identifies one compact read-model product. Values are
// stable schema tags.
type RequirementStage uint8

const (
	RequirementPoint RequirementStage = iota + 1
	RequirementBoundary
	RequirementEdge
	RequirementObservation
	RequirementRoute
)

type projectionDescriptor struct {
	id          ProjectionID
	stage       RequirementStage
	fact        Kind
	observation observation.Kind
}

// This is the registration boundary. Adding an observation consumer requires
// one stable selector here; no State lane switch or whole-State selector exists.
var observationProjectionDescriptors = [...]projectionDescriptor{
	{id: ProjectionPointReachable, stage: RequirementPoint},
	{id: ProjectionEdgeNormal, stage: RequirementEdge},
	{id: projectionBoundaryRootAssignment, stage: RequirementBoundary, fact: RootAssignment},
	{id: projectionBoundaryPathAssignment, stage: RequirementBoundary, fact: PathAssignment},
	{id: projectionBoundaryStaticMemberWrite, stage: RequirementBoundary, fact: PathStaticMemberWrite},
	{id: projectionBoundaryDynamicIndexWrite, stage: RequirementBoundary, fact: DynamicIndexWrite},
	{id: projectionBoundaryPathInvalidation, stage: RequirementBoundary, fact: PathDescendantInvalidation},
	{id: projectionBoundaryCovariantExposure, stage: RequirementBoundary, fact: CovariantExposure},
	{id: projectionBoundaryNoNormalReturn, stage: RequirementBoundary, fact: NoNormalReturn},
	{id: projectionBoundaryChannelSelect, stage: RequirementBoundary, fact: ChannelSelect},
	{id: projectionBoundaryPostcondition, stage: RequirementBoundary, fact: PostconditionRefinement},
	{id: projectionBoundaryPathRelation, stage: RequirementBoundary, fact: PostconditionPathRelation},
	{id: projectionBoundaryCallResult, stage: RequirementBoundary, fact: CallResultValue},
	{id: projectionBoundaryReturn, stage: RequirementBoundary, fact: Return},
	{id: projectionBoundaryCallProducer, stage: RequirementBoundary},
	{id: projectionBoundaryCallOutcome, stage: RequirementBoundary, fact: CallSite},
	{id: ProjectionObservationAssignment, stage: RequirementObservation, observation: observation.Assignment},
	{id: ProjectionObservationCallArgument, stage: RequirementObservation, observation: observation.CallArgument},
	{id: ProjectionObservationCallResult, stage: RequirementObservation, observation: observation.CallResult},
	{id: projectionObservationCallResultComplex, stage: RequirementObservation, observation: observation.CallResult},
	{id: ProjectionObservationCallInvocation, stage: RequirementRoute, observation: observation.CallInvocation},
}

const (
	selectorPointReachable uint8 = iota
	selectorEdgeNormal
	selectorBoundaryRootAssignment
	selectorBoundaryPathAssignment
	selectorBoundaryStaticMemberWrite
	selectorBoundaryDynamicIndexWrite
	selectorBoundaryPathInvalidation
	selectorBoundaryCovariantExposure
	selectorBoundaryNoNormalReturn
	selectorBoundaryChannelSelect
	selectorBoundaryPostcondition
	selectorBoundaryPathRelation
	selectorBoundaryCallResult
	selectorBoundaryReturn
	selectorBoundaryCallProducer
	selectorBoundaryCallOutcome
	selectorObservationAssignment
	selectorObservationCallArgument
	selectorObservationCallResult
	selectorObservationCallResultComplex
	selectorObservationCallInvocation
)

var boundaryProjectionSelectors = [...]uint8{
	RootAssignment:             selectorBoundaryRootAssignment + 1,
	PathAssignment:             selectorBoundaryPathAssignment + 1,
	PathStaticMemberWrite:      selectorBoundaryStaticMemberWrite + 1,
	DynamicIndexWrite:          selectorBoundaryDynamicIndexWrite + 1,
	PathDescendantInvalidation: selectorBoundaryPathInvalidation + 1,
	CovariantExposure:          selectorBoundaryCovariantExposure + 1,
	NoNormalReturn:             selectorBoundaryNoNormalReturn + 1,
	ChannelSelect:              selectorBoundaryChannelSelect + 1,
	PostconditionRefinement:    selectorBoundaryPostcondition + 1,
	PostconditionPathRelation:  selectorBoundaryPathRelation + 1,
	CallResultValue:            selectorBoundaryCallResult + 1,
	Return:                     selectorBoundaryReturn + 1,
}

// ObservationRequirement is a synthesized slot descriptor. Common slots are
// not retained as records: Cursor derives them from Plan's packed rows/cells,
// a reachability bitset, and packed branch edges.
type ObservationRequirement struct {
	selector uint8
	flags    uint8
	point    uint32
	to       uint32
	anchor   observation.Occurrence
}

const observationRequirementCallOutcome uint8 = 1

func (r ObservationRequirement) descriptor() (projectionDescriptor, bool) {
	if int(r.selector) >= len(observationProjectionDescriptors) {
		return projectionDescriptor{}, false
	}
	return observationProjectionDescriptors[r.selector], true
}
func (r ObservationRequirement) Projection() ProjectionID { d, _ := r.descriptor(); return d.id }
func (r ObservationRequirement) Stage() RequirementStage  { d, _ := r.descriptor(); return d.stage }

// FactKind returns the registered boundary fact that owns this selector.
// Selectors without a fact owner (point, edge, observation, and composite
// producer selectors) return false rather than manufacturing a sentinel fact.
func (r ObservationRequirement) FactKind() (Kind, bool) {
	d, ok := r.descriptor()
	return d.fact, ok && d.stage == RequirementBoundary && d.fact != 0
}

// ObservationKind returns the registered evidence family for observation and
// route selectors. Boundary/point/edge selectors return false.
func (r ObservationRequirement) ObservationKind() (observation.Kind, bool) {
	d, ok := r.descriptor()
	return d.observation, ok && (d.stage == RequirementObservation || d.stage == RequirementRoute) && d.observation != observation.Invalid
}
func (r ObservationRequirement) Point() cfg.Point { return cfg.Point(r.point) }
func (r ObservationRequirement) EdgeTarget() (cfg.Point, bool) {
	return cfg.Point(r.to), r.Stage() == RequirementEdge
}
func (r ObservationRequirement) Anchor() (observation.Occurrence, bool) {
	stage := r.Stage()
	return r.anchor, (stage == RequirementObservation || stage == RequirementRoute) && r.anchor.Valid()
}
func (r ObservationRequirement) RequiresCallOutcome() bool {
	return r.flags&observationRequirementCallOutcome != 0
}

// IsCallProducerBoundary reports the structural boundary whose values are the
// exact temporary return slots produced by one call. The selector remains
// private; consumers receive only this typed question rather than depending on
// its schema string.
func (r ObservationRequirement) IsCallProducerBoundary() bool {
	return r.selector == selectorBoundaryCallProducer && r.Stage() == RequirementBoundary
}

type ObservationSchemaID [sha256.Size]byte
type ObservationConsumerInventoryID [sha256.Size]byte

// ObservationRequirements shares Plan's immutable fact index and debug-point
// table. Its only per-body retained data is a reachable-point bitset and packed
// branch edges; no per-point State or per-common-slot record is retained.
type ObservationRequirements struct {
	schema     ObservationSchemaID
	inventory  ObservationConsumerInventoryID
	reachable  []uint64
	edges      []uint64
	rows       []row
	cells      []Cell
	facts      factflow.Facts
	points     []observationPoint
	pointCount int
	slotCount  int
	sealed     bool
}

func (r ObservationRequirements) Sealed() bool                  { return r.sealed }
func (r ObservationRequirements) SchemaID() ObservationSchemaID { return r.schema }
func (r ObservationRequirements) ConsumerInventoryID() ObservationConsumerInventoryID {
	return r.inventory
}
func (r ObservationRequirements) Entries(callOutcome bool) []ObservationRequirement {
	if !r.sealed {
		return nil
	}
	out := make([]ObservationRequirement, 0, r.slotCount)
	cursor := r.Cursor(callOutcome)
	for entry, ok := cursor.Next(); ok; entry, ok = cursor.Next() {
		out = append(out, entry)
	}
	return out
}
func (r ObservationRequirements) Cursor(callOutcome bool) ObservationRequirementCursor {
	if !r.sealed {
		return ObservationRequirementCursor{}
	}
	return ObservationRequirementCursor{requirements: &r, callOutcome: callOutcome}
}

func (p *Plan) ObservationRequirements() (ObservationRequirements, bool) {
	if p == nil || !p.observationRequirements.sealed {
		return ObservationRequirements{}, false
	}
	return p.observationRequirements, true
}

type observationRequirementPhase uint8

const (
	requirementCursorPoints observationRequirementPhase = iota
	requirementCursorBoundaries
	requirementCursorEdges
	requirementCursorObservations
	requirementCursorDone
)

// ObservationRequirementCursor synthesizes canonical slots without allocation.
type ObservationRequirementCursor struct {
	requirements    *ObservationRequirements
	callOutcome     bool
	phase           observationRequirementPhase
	point           int
	cell            uint32
	cellEnd         uint32
	boundaryTail    uint8
	edge            int
	observationTail uint8
	resultTarget    int
}

func (c *ObservationRequirementCursor) Next() (ObservationRequirement, bool) {
	if c == nil || c.requirements == nil {
		return ObservationRequirement{}, false
	}
	r := c.requirements
	for {
		switch c.phase {
		case requirementCursorPoints:
			if point, ok := c.nextReachable(); ok {
				return knownObservationRequirementSelector(selectorPointReachable, point, 0, observation.Occurrence{}, false), true
			}
			c.phase, c.point = requirementCursorBoundaries, 0
		case requirementCursorBoundaries:
			for c.point < r.pointCount {
				point := cfg.Point(c.point)
				if !r.reachableAt(point) {
					c.point++
					continue
				}
				if c.cellEnd == 0 && c.boundaryTail == 0 {
					row := r.rows[c.point]
					c.cell, c.cellEnd = row.start, row.end
				}
				for c.cell < c.cellEnd {
					cell := r.cells[c.cell]
					c.cell++
					if cell.Kind() == RootAssignment && !rootAssignmentHasReportableConsumer(r.facts, point) {
						continue
					}
					if selector, ok := boundaryProjectionSelectorForFact(cell.Kind()); ok {
						return knownObservationRequirementSelector(selector, point, 0, observation.Occurrence{}, false), true
					}
				}
				site, hasCall := r.facts.CallSiteView(point)
				if c.boundaryTail == 0 {
					c.boundaryTail = 1
					if hasCall && callproducer.HasView(site) {
						return knownObservationRequirementSelector(selectorBoundaryCallProducer, point, 0, observation.Occurrence{}, false), true
					}
				}
				if c.boundaryTail == 1 {
					c.boundaryTail = 2
					if hasCall && c.callOutcome {
						return knownObservationRequirementSelector(selectorBoundaryCallOutcome, point, 0, observation.Occurrence{}, true), true
					}
				}
				c.point++
				c.cell, c.cellEnd, c.boundaryTail = 0, 0, 0
			}
			c.phase = requirementCursorEdges
		case requirementCursorEdges:
			if c.edge < len(r.edges) {
				packed := r.edges[c.edge]
				c.edge++
				return knownObservationRequirementSelector(selectorEdgeNormal, cfg.Point(packed>>32), cfg.Point(uint32(packed)), observation.Occurrence{}, false), true
			}
			c.phase, c.point = requirementCursorObservations, 0
		case requirementCursorObservations:
			for c.point < r.pointCount {
				point := cfg.Point(c.point)
				if !r.reachableAt(point) {
					c.point++
					continue
				}
				ids := r.points[c.point]
				if c.observationTail == 0 {
					c.observationTail = 1
					if _, ok := r.facts.RootAssignment(point); ok && rootAssignmentHasReportableConsumer(r.facts, point) {
						anchor := observation.Occurrence{Point: ids.after, Kind: observation.Assignment}
						return knownObservationRequirementSelector(selectorObservationAssignment, point, 0, anchor, false), true
					}
				}
				if site, ok := r.facts.CallSiteView(point); ok {
					if c.observationTail == 1 {
						for c.resultTarget < site.ArgumentSourceCount() {
							index := c.resultTarget
							c.resultTarget++
							anchor := observation.Occurrence{Point: ids.call, Kind: observation.CallArgument, Slot: uint32(index)}
							return knownObservationRequirementSelector(selectorObservationCallArgument, point, 0, anchor, false), true
						}
						c.observationTail, c.resultTarget = 2, 0
					}
					if c.observationTail == 2 {
						c.observationTail = 3
						anchor := observation.Occurrence{Point: ids.call, Kind: observation.CallInvocation}
						return knownObservationRequirementSelector(selectorObservationCallInvocation, point, 0, anchor, false), true
					}
					for c.resultTarget < site.ResultTargetCount() {
						index := c.resultTarget
						c.resultTarget++
						target, found := site.ResultTargetAt(index)
						if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
							continue
						}
						projection := ProjectionObservationCallResult
						if target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 {
							projection = projectionObservationCallResultComplex
						}
						anchor := observation.Occurrence{Point: ids.call, Kind: observation.CallResult, Slot: uint32(index)}
						selector := selectorObservationCallResult
						if projection == projectionObservationCallResultComplex {
							selector = selectorObservationCallResultComplex
						}
						return knownObservationRequirementSelector(selector, point, 0, anchor, false), true
					}
				}
				c.point++
				c.observationTail, c.resultTarget = 0, 0
			}
			c.phase = requirementCursorDone
		case requirementCursorDone:
			return ObservationRequirement{}, false
		}
	}
}

func (c *ObservationRequirementCursor) nextReachable() (cfg.Point, bool) {
	r := c.requirements
	for c.point < r.pointCount {
		point := cfg.Point(c.point)
		c.point++
		if r.reachableAt(point) {
			return point, true
		}
	}
	return 0, false
}

func (r ObservationRequirements) reachableAt(point cfg.Point) bool {
	if uint64(point) >= uint64(r.pointCount) {
		return false
	}
	return r.reachable[uint(point)>>6]&(uint64(1)<<(uint(point)&63)) != 0
}

type observationRequirementBuilder struct {
	reachable      []uint64
	pointSelectors []uint64
	edges          []uint64
	records        []observationRequirementKey
	valid          bool
}

func newObservationRequirementBuilder(points int) observationRequirementBuilder {
	return observationRequirementBuilder{
		reachable:      make([]uint64, (points+63)/64),
		pointSelectors: make([]uint64, points),
		valid:          true,
	}
}

func (b *observationRequirementBuilder) addPoint(plan *Plan, graph cfg.Graph, body lexicalidentity.StableLexicalBodyID, point cfg.Point, ids observationPoint) {
	if !b.valid || body == (lexicalidentity.StableLexicalBodyID{}) || uint64(point) >= uint64(len(b.reachable)*64) || !ids.after.Valid() {
		b.valid = false
		return
	}
	b.emit(selectorPointReachable, point, 0, observation.Occurrence{}, false)
	row := plan.rows[point]
	for index := row.start; index < row.end; index++ {
		if plan.cells[index].Kind() == RootAssignment && !rootAssignmentHasReportableConsumer(plan.facts, point) {
			continue
		}
		if selector, ok := boundaryProjectionSelectorForFact(plan.cells[index].Kind()); ok {
			b.emit(selector, point, 0, observation.Occurrence{}, false)
		}
	}
	if rowContainsKind(plan.cells, row, RootAssignment) && rootAssignmentHasReportableConsumer(plan.facts, point) {
		b.emit(selectorObservationAssignment, point, 0, observation.Occurrence{Point: ids.after, Kind: observation.Assignment}, false)
	}
	if rowContainsKind(plan.cells, row, CallSite) {
		site, ok := plan.facts.CallSiteView(point)
		if !ok {
			b.valid = false
			return
		}
		if callproducer.HasView(site) {
			b.emit(selectorBoundaryCallProducer, point, 0, observation.Occurrence{}, false)
		}
		b.emit(selectorBoundaryCallOutcome, point, 0, observation.Occurrence{}, true)
		for index := 0; index < site.ArgumentSourceCount(); index++ {
			b.emit(selectorObservationCallArgument, point, 0, observation.Occurrence{Point: ids.call, Kind: observation.CallArgument, Slot: uint32(index)}, false)
		}
		b.emit(selectorObservationCallInvocation, point, 0, observation.Occurrence{Point: ids.call, Kind: observation.CallInvocation}, false)
		for index := 0; index < site.ResultTargetCount(); index++ {
			target, found := site.ResultTargetAt(index)
			if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
				continue
			}
			selector := selectorObservationCallResult
			if target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 {
				selector = selectorObservationCallResultComplex
			}
			b.emit(selector, point, 0, observation.Occurrence{Point: ids.call, Kind: observation.CallResult, Slot: uint32(index)}, false)
		}
	}
	if graph.IsBranch(point) {
		for _, to := range cfg.SuccessorsReadOnly(graph, point) {
			b.emit(selectorEdgeNormal, point, to, observation.Occurrence{}, false)
		}
	}
	for index := row.start; index < row.end; index++ {
		if plan.cells[index].Kind() == CallSite && !ids.call.Valid() {
			b.valid = false
			return
		}
	}
}

// rootAssignmentHasReportableConsumer distinguishes semantic assignment work
// from consumer-visible assignment evidence. A stable local function
// declaration still executes symbolically and is routed by the sealed lexical
// call surface, but its composite function type is not retained merely to
// publish an assignment observation when there is no declared target contract.
// Ordinary/export writes and annotated declarations remain reportable.
func rootAssignmentHasReportableConsumer(facts factflow.Facts, point cfg.Point) bool {
	assignment, ok := facts.RootAssignment(point)
	if !ok || assignment.Kind() != factflow.RootAssignmentLocalDeclaration {
		return ok
	}
	if _, declared := assignment.DeclaredAnnotationValue(); declared {
		return true
	}
	source := assignment.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || assignment.TargetSymbol() == 0 {
		return true
	}
	functionSymbol, function := facts.ExpressionFunction(source.ExprRef)
	_, valued := facts.ExpressionValue(source.ExprRef)
	return !function || functionSymbol == 0 || !valued
}

func rowContainsKind(cells []Cell, row row, kind Kind) bool {
	for index := row.start; index < row.end; index++ {
		if cells[index].Kind() == kind {
			return true
		}
	}
	return false
}

func (b *observationRequirementBuilder) emit(selector uint8, point, to cfg.Point, anchor observation.Occurrence, callOutcome bool) {
	if !b.valid {
		return
	}
	requirement, ok := makeObservationRequirementSelector(selector, point, to, anchor, callOutcome)
	if !ok {
		b.valid = false
		return
	}
	if int(requirement.point) >= len(b.pointSelectors) {
		b.valid = false
		return
	}
	switch requirement.Stage() {
	case RequirementPoint:
		b.reachable[uint(requirement.point)>>6] |= uint64(1) << (uint(requirement.point) & 63)
	case RequirementBoundary:
		if requirement.selector >= 64 {
			b.valid = false
			return
		}
		b.pointSelectors[requirement.point] |= uint64(1) << requirement.selector
	case RequirementEdge:
		if int(requirement.to) >= len(b.pointSelectors) {
			b.valid = false
			return
		}
		b.edges = append(b.edges, uint64(requirement.point)<<32|uint64(requirement.to))
	case RequirementObservation, RequirementRoute:
		b.records = append(b.records, encodeObservationRequirementKey(requirement))
	default:
		b.valid = false
	}
}

func (b *observationRequirementBuilder) freezeCanonical(plan *Plan, points []observationPoint) ObservationRequirements {
	if !b.valid || plan == nil || len(points) != len(plan.rows) {
		return ObservationRequirements{}
	}
	out := ObservationRequirements{
		schema: defaultObservationProjectionSchemaID, reachable: b.reachable, edges: b.edges,
		rows: plan.rows, cells: plan.cells, facts: plan.facts, points: points, pointCount: len(plan.rows), sealed: true,
	}
	var ok bool
	out.inventory, out.slotCount, ok = observationRequirementCompactInventoryID(
		out.schema, out.pointCount, b.reachable, b.pointSelectors, b.edges, b.records,
	)
	if !ok || out.inventory == (ObservationConsumerInventoryID{}) {
		return ObservationRequirements{}
	}
	return out
}

func boundaryProjectionForFact(kind Kind) (projectionDescriptor, bool) {
	selector, ok := boundaryProjectionSelectorForFact(kind)
	if !ok {
		return projectionDescriptor{}, false
	}
	return observationProjectionDescriptors[selector], true
}

func boundaryProjectionSelectorForFact(kind Kind) (uint8, bool) {
	if int(kind) >= len(boundaryProjectionSelectors) || boundaryProjectionSelectors[kind] == 0 {
		return 0, false
	}
	return boundaryProjectionSelectors[kind] - 1, true
}

func projectionSelector(id ProjectionID) (uint8, bool) {
	for index, descriptor := range observationProjectionDescriptors {
		if descriptor.id == id {
			return uint8(index), true
		}
	}
	return 0, false
}

func makeObservationRequirement(id ProjectionID, point, to cfg.Point, anchor observation.Occurrence, callOutcome bool) (ObservationRequirement, bool) {
	selector, ok := projectionSelector(id)
	if !ok {
		return ObservationRequirement{}, false
	}
	return makeObservationRequirementSelector(selector, point, to, anchor, callOutcome)
}

func makeObservationRequirementSelector(selector uint8, point, to cfg.Point, anchor observation.Occurrence, callOutcome bool) (ObservationRequirement, bool) {
	if int(selector) >= len(observationProjectionDescriptors) {
		return ObservationRequirement{}, false
	}
	requirement := ObservationRequirement{selector: selector, point: uint32(point), to: uint32(to), anchor: anchor}
	if callOutcome {
		requirement.flags |= observationRequirementCallOutcome
	}
	descriptor := observationProjectionDescriptors[selector]
	ok := false
	switch descriptor.stage {
	case RequirementPoint, RequirementBoundary:
		ok = to == 0 && anchor == (observation.Occurrence{})
	case RequirementEdge:
		ok = anchor == (observation.Occurrence{}) && point != to
	case RequirementObservation, RequirementRoute:
		ok = !callOutcome && to == 0 && anchor.Valid() && anchor.Kind == descriptor.observation
	default:
		ok = false
	}
	return requirement, ok
}

func knownObservationRequirementSelector(selector uint8, point, to cfg.Point, anchor observation.Occurrence, callOutcome bool) ObservationRequirement {
	requirement, ok := makeObservationRequirementSelector(selector, point, to, anchor, callOutcome)
	if !ok {
		panic("operationplan: registered observation selector rejected")
	}
	return requirement
}

var defaultObservationProjectionSchemaID = observationProjectionSchemaID(observationProjectionDescriptors[:])

func observationProjectionSchemaID(descriptors []projectionDescriptor) ObservationSchemaID {
	h := sha256.New()
	_, _ = io.WriteString(h, "go-lua.observation-projection-schema.v1")
	for _, descriptor := range descriptors {
		writeObservationString(h, string(descriptor.id))
		_, _ = h.Write([]byte{byte(descriptor.stage), byte(descriptor.fact), byte(descriptor.observation)})
	}
	var out ObservationSchemaID
	copy(out[:], h.Sum(nil))
	return out
}

func observationRequirementInventoryID(requirements ObservationRequirements) (ObservationConsumerInventoryID, int) {
	entries := requirements.Entries(true)
	reachable := make([]uint64, len(requirements.reachable))
	pointSelectors := make([]uint64, requirements.pointCount)
	edges := make([]uint64, 0)
	records := make([]observationRequirementKey, 0)
	for _, entry := range entries {
		if int(entry.point) >= requirements.pointCount {
			return ObservationConsumerInventoryID{}, 0
		}
		switch entry.Stage() {
		case RequirementPoint:
			reachable[uint(entry.point)>>6] |= uint64(1) << (uint(entry.point) & 63)
		case RequirementBoundary:
			if entry.selector >= 64 {
				return ObservationConsumerInventoryID{}, 0
			}
			pointSelectors[entry.point] |= uint64(1) << entry.selector
		case RequirementEdge:
			if int(entry.to) >= requirements.pointCount {
				return ObservationConsumerInventoryID{}, 0
			}
			edges = append(edges, uint64(entry.point)<<32|uint64(entry.to))
		case RequirementObservation, RequirementRoute:
			records = append(records, encodeObservationRequirementKey(entry))
		default:
			return ObservationConsumerInventoryID{}, 0
		}
	}
	id, count, ok := observationRequirementCompactInventoryID(
		requirements.schema, requirements.pointCount, reachable, pointSelectors, edges, records,
	)
	if !ok {
		return ObservationConsumerInventoryID{}, 0
	}
	return id, count
}

type observationRequirementKey [24]byte

func observationRequirementCompactInventoryID(
	schema ObservationSchemaID,
	pointCount int,
	reachable []uint64,
	pointSelectors []uint64,
	edges []uint64,
	records []observationRequirementKey,
) (ObservationConsumerInventoryID, int, bool) {
	if pointCount < 0 || uint64(pointCount) > uint64(^uint32(0)) || len(reachable) != (pointCount+63)/64 || len(pointSelectors) != pointCount {
		return ObservationConsumerInventoryID{}, 0, false
	}
	if pointCount > 0 && len(reachable) != 0 {
		lastBits := uint(pointCount & 63)
		if lastBits != 0 && reachable[len(reachable)-1]>>lastBits != 0 {
			return ObservationConsumerInventoryID{}, 0, false
		}
	}
	for point, selectors := range pointSelectors {
		if selectors != 0 && reachable[uint(point)>>6]&(uint64(1)<<(uint(point)&63)) == 0 {
			return ObservationConsumerInventoryID{}, 0, false
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i] < edges[j] })
	sort.Slice(records, func(i, j int) bool { return bytes.Compare(records[i][:], records[j][:]) < 0 })

	normalizedPointCount := 0
	for index := len(reachable) - 1; index >= 0; index-- {
		if reachable[index] != 0 {
			normalizedPointCount = index*64 + bits.Len64(reachable[index])
			break
		}
	}

	uniqueEdges := 0
	for index, edge := range edges {
		if index != 0 && edge == edges[index-1] {
			continue
		}
		from, to := uint32(edge>>32), uint32(edge)
		if int(from) >= pointCount || int(to) >= pointCount || from == to ||
			reachable[uint(from)>>6]&(uint64(1)<<(uint(from)&63)) == 0 {
			return ObservationConsumerInventoryID{}, 0, false
		}
		edges[uniqueEdges] = edge
		uniqueEdges++
		if referenced := int(from) + 1; referenced > normalizedPointCount {
			normalizedPointCount = referenced
		}
		if referenced := int(to) + 1; referenced > normalizedPointCount {
			normalizedPointCount = referenced
		}
	}

	uniqueRecords := 0
	for index, record := range records {
		if index != 0 && record == records[index-1] {
			continue
		}
		point := binary.BigEndian.Uint32(record[4:8])
		if int(point) >= pointCount || reachable[uint(point)>>6]&(uint64(1)<<(uint(point)&63)) == 0 {
			return ObservationConsumerInventoryID{}, 0, false
		}
		records[uniqueRecords] = record
		uniqueRecords++
		if referenced := int(point) + 1; referenced > normalizedPointCount {
			normalizedPointCount = referenced
		}
	}

	// Storage width is not part of the logical consumer set. Only the prefix
	// through the highest point named by a requirement participates in its ID;
	// ownership and body identity are bound separately by the certificate.
	normalizedWordCount := (normalizedPointCount + 63) / 64
	h := sha256.New()
	_, _ = io.WriteString(h, "go-lua.observation-consumer-inventory.v3")
	_, _ = h.Write(schema[:])
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], uint64(normalizedPointCount))
	_, _ = h.Write(raw[:])
	binary.BigEndian.PutUint64(raw[:], uint64(normalizedWordCount))
	_, _ = h.Write(raw[:])
	count := 0
	for _, word := range reachable[:normalizedWordCount] {
		binary.BigEndian.PutUint64(raw[:], word)
		_, _ = h.Write(raw[:])
		count += bits.OnesCount64(word)
	}
	binary.BigEndian.PutUint64(raw[:], uint64(normalizedPointCount))
	_, _ = h.Write(raw[:])
	for _, selectors := range pointSelectors[:normalizedPointCount] {
		binary.BigEndian.PutUint64(raw[:], selectors)
		_, _ = h.Write(raw[:])
		count += bits.OnesCount64(selectors)
	}
	binary.BigEndian.PutUint64(raw[:], uint64(uniqueEdges))
	_, _ = h.Write(raw[:])
	for _, edge := range edges[:uniqueEdges] {
		binary.BigEndian.PutUint64(raw[:], edge)
		_, _ = h.Write(raw[:])
	}
	count += uniqueEdges
	binary.BigEndian.PutUint64(raw[:], uint64(uniqueRecords))
	_, _ = h.Write(raw[:])
	for _, record := range records[:uniqueRecords] {
		_, _ = h.Write(record[:])
	}
	count += uniqueRecords

	var out ObservationConsumerInventoryID
	copy(out[:], h.Sum(nil))
	return out, count, true
}

func encodeObservationRequirementKey(entry ObservationRequirement) observationRequirementKey {
	var key observationRequirementKey
	encodeObservationRequirement(key[:], entry)
	return key
}

func encodeObservationRequirement(raw []byte, entry ObservationRequirement) {
	for i := range raw {
		raw[i] = 0
	}
	raw[0], raw[1], raw[2] = entry.selector, byte(entry.Stage()), entry.flags
	binary.BigEndian.PutUint32(raw[4:8], entry.point)
	binary.BigEndian.PutUint32(raw[8:12], entry.to)
	binary.BigEndian.PutUint32(raw[12:16], entry.anchor.Point.Ordinal)
	raw[16], raw[17] = byte(entry.anchor.Point.Phase), byte(entry.anchor.Kind)
	binary.BigEndian.PutUint32(raw[20:24], entry.anchor.Slot)
}

func writeObservationString(out hash.Hash, value string) {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	_, _ = out.Write(size[:])
	_, _ = io.WriteString(out, value)
}
