package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/type/indexproof"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// DynamicReadRangeQuery is one exact visible index/array address pair. It is
// present only for a root table path and a uniquely visible key path; keeping
// the pair intact prevents combining a proof for one index with another
// index's numeric floor.
type DynamicReadRangeQuery struct {
	Shape              indexform.IndexShape
	IndexStateKey      pathaddr.StateKey
	ArrayStateKey      pathaddr.StateKey
	IndexProofStateKey pathaddr.StateKey
	ArrayProofStateKey pathaddr.StateKey
	ModuloInteger      bool
}

// DynamicReadQuery is one correlated table/key terminal query. A plan is
// deliberately built after the operand decision diagrams have been
// correlated: joining unrelated operand supports would destroy both precision
// and sparsity.
type DynamicReadQuery struct {
	KeySpace          *keyspace.KeySpace
	TableKeys         []pathaddr.StateKey
	KeyKeys           []pathaddr.StateKey
	TableValue        product.Value
	TablePath         keyspace.Key
	OwnerPath         keyspace.Key
	OwnerValue        product.Value
	HasOwnerValue     bool
	ProjectPath       bool
	KeyValue          product.Value
	TypeValues        *typevalue.Cache
	RangeContainer    product.Value
	HasRangeContainer bool
	Range             DynamicReadRangeQuery
	HasRange          bool
}

// DynamicReadEvidence is the complete immutable observation consumed by the
// source-value dynamic-read algebra. It contains no State and no whole
// coordinate family. Path and heap observations are exactly those demanded by
// the query's finite structural binder.
type DynamicReadEvidence struct {
	domain              ProductDomain
	query               DynamicReadQuery
	Value               product.Value
	HasValue            bool
	KeyMembershipProven bool
	HeapObject          heapidentity.TableObject
	HasHeapObject       bool
	ProjectedTable      product.Value
	ProjectedSegments   int
	pathValues          map[keyspace.Key]product.Value
	rangeProof          bool
	rangeBranchProof    bool
	rangeIndexFloor     int64
	hasRangeIndexFloor  bool
	rangeIndexCeil      int64
	hasRangeIndexCeil   bool
	rangeArrayLenFloor  int64
	hasRangeArrayFloor  bool
	rangeDiffProof      bool
	rangeRelations      []RelConstraint
	rangeNumFloors      map[pathaddr.StateKey]int64
}

// Domain returns the sealed product authority which produced this evidence.
func (e DynamicReadEvidence) Domain() ProductDomain { return e.domain }

// InRangeIndexEvidence returns the exact paired must-proof and numeric lower
// bound observed for this query. proof is false unless the registered path
// coordinate exists; floorOK is false unless the matching index coordinate
// carries a finite floor.
func (e DynamicReadEvidence) InRangeIndexEvidence() bool {
	return e.rangeProof
}

// MatchesQuery reports whether evidence was produced for this exact
// correlated terminal query. This prevents a guarded cofactor from being
// paired with evidence from a sibling table/key region.
func (e DynamicReadEvidence) MatchesQuery(query DynamicReadQuery) bool {
	if !e.domain.Valid() || e.query.KeySpace != query.KeySpace || e.query.TablePath != query.TablePath ||
		e.query.OwnerPath != query.OwnerPath || e.query.ProjectPath != query.ProjectPath ||
		e.query.HasOwnerValue != query.HasOwnerValue || len(e.query.TableKeys) != len(query.TableKeys) ||
		len(e.query.KeyKeys) != len(query.KeyKeys) ||
		e.query.HasRangeContainer != query.HasRangeContainer ||
		e.query.HasRange != query.HasRange || e.query.Range != query.Range ||
		!product.Equal(e.domain.reg, e.query.TableValue, query.TableValue) ||
		!product.Equal(e.domain.reg, e.query.KeyValue, query.KeyValue) ||
		e.query.HasOwnerValue && !product.Equal(e.domain.reg, e.query.OwnerValue, query.OwnerValue) ||
		e.query.HasRangeContainer && !product.Equal(e.domain.reg, e.query.RangeContainer, query.RangeContainer) {
		return false
	}
	for i := range e.query.TableKeys {
		if e.query.TableKeys[i] != query.TableKeys[i] {
			return false
		}
	}
	for i := range e.query.KeyKeys {
		if e.query.KeyKeys[i] != query.KeyKeys[i] {
			return false
		}
	}
	return true
}

// PathValue returns one exact flow-sensitive path observation.
func (e DynamicReadEvidence) PathValue(path keyspace.Key) (product.Value, bool) {
	value, ok := e.pathValues[path]
	return value, ok
}

// laneDynamicReadPolicy is the explicit registration law for an ordinary
// State lane. The invalid zero value is rejected during product construction.
type laneDynamicReadPolicy struct {
	declared   bool
	phase      uint8
	demand     func(DynamicReadQuery) bool
	project    func(ProductDomain, DynamicReadQueryPlan, laneFactorPayload) (laneFactorPayload, error)
	observe    func(laneFactorPayload, *dynamicReadBuilder) error
	contribute func(laneFactorPayload, DynamicReadQuery, *dynamicReadProjectionEvidence)
}

type dynamicReadProjectionEvidence struct {
	keyMembershipProven bool
}

func dynamicReadIndependent() laneDynamicReadPolicy {
	return laneDynamicReadPolicy{declared: true}
}

func dynamicReadOrdinary(
	phase uint8,
	demand func(DynamicReadQuery) bool,
	project func(ProductDomain, DynamicReadQueryPlan, laneFactorPayload) (laneFactorPayload, error),
	observe func(laneFactorPayload, *dynamicReadBuilder) error,
) laneDynamicReadPolicy {
	return laneDynamicReadPolicy{declared: true, phase: phase, demand: demand, project: project, observe: observe}
}

func dynamicReadAlways(DynamicReadQuery) bool { return true }

func dynamicReadFacts() laneDynamicReadPolicy {
	return dynamicReadOrdinary(1, dynamicReadAlways, projectDynamicReadFacts, func(payload laneFactorPayload, out *dynamicReadBuilder) error {
		if out.hasFacts {
			return fmt.Errorf("%w: duplicate dynamic-read fact producer", ErrInvalidLaneFactor)
		}
		out.facts, out.hasFacts = typedLaneFactorValue[dynamicIndexLane](payload), true
		return nil
	})
}

func dynamicReadMemberships() laneDynamicReadPolicy {
	policy := dynamicReadOrdinary(0, dynamicReadAlways, projectDynamicReadMemberships, func(payload laneFactorPayload, out *dynamicReadBuilder) error {
		if out.hasMemberships {
			return fmt.Errorf("%w: duplicate dynamic-read membership producer", ErrInvalidLaneFactor)
		}
		out.memberships, out.hasMemberships = typedLaneFactorValue[keyMembershipLane](payload), true
		return nil
	})
	policy.contribute = func(payload laneFactorPayload, _ DynamicReadQuery, out *dynamicReadProjectionEvidence) {
		lane := typedLaneFactorValue[keyMembershipLane](payload)
		out.keyMembershipProven = !lane.bottom && len(lane.path) != 0
	}
	return policy
}

// coordinateDynamicReadPolicy is the registration-owned sparse query law for
// one transposed family. seed declares the exact first demand; advance may add
// further exact slots after observing prior scalar values. Demands are
// monotone, finite and structurally terminating; no iteration budget exists.
type coordinateDynamicReadPolicy struct {
	declared         bool
	participates     bool
	rangeEvidence    bool
	demand           func(DynamicReadQuery) bool
	topologyRequired bool
	seed             func(*dynamicReadPlanner, *dynamicReadCoordinateBinding) error
	advance          func(*dynamicReadPlanner, *dynamicReadCoordinateBinding) error
}

func dynamicReadCoordinateIndependent() coordinateDynamicReadPolicy {
	return coordinateDynamicReadPolicy{declared: true}
}

func dynamicReadCoordinateParticipant(
	seed func(*dynamicReadPlanner, *dynamicReadCoordinateBinding) error,
	advance func(*dynamicReadPlanner, *dynamicReadCoordinateBinding) error,
) coordinateDynamicReadPolicy {
	return dynamicReadCoordinateParticipantWhen(dynamicReadAlways, seed, advance)
}

func dynamicReadCoordinateParticipantWhen(
	demand func(DynamicReadQuery) bool,
	seed func(*dynamicReadPlanner, *dynamicReadCoordinateBinding) error,
	advance func(*dynamicReadPlanner, *dynamicReadCoordinateBinding) error,
) coordinateDynamicReadPolicy {
	return coordinateDynamicReadPolicy{declared: true, participates: true, demand: demand, topologyRequired: true, seed: seed, advance: advance}
}

// DynamicReadCoordinateDemand identifies one exact family demand round. A
// skeleton is a single family-global quotient root; Slots are ordinary scalar
// roots. Both are validated against their sealed domain and keyspace when a
// batch is advanced.
type DynamicReadCoordinateDemand struct {
	family                    CoordinateFamily
	needSkeleton              bool
	scalarDefaultsUseTopology bool
	slots                     []CoordinateSlot
}

func (d DynamicReadCoordinateDemand) Family() CoordinateFamily { return d.family }
func (d DynamicReadCoordinateDemand) NeedsSkeleton() bool      { return d.needSkeleton }

// ScalarDefaultsUseTopology reports whether an omitted demanded scalar is
// derived from the family's skeleton. It is independent of NeedsSkeleton:
// later demand rounds reuse the already-observed skeleton without requesting
// it again, while their scalar defaults remain topology-dependent.
func (d DynamicReadCoordinateDemand) ScalarDefaultsUseTopology() bool {
	return d.scalarDefaultsUseTopology
}
func (d DynamicReadCoordinateDemand) Slots() []CoordinateSlot {
	return append([]CoordinateSlot(nil), d.slots...)
}

// DynamicReadDemandSet is the detached exact demand of one binder round.
type DynamicReadDemandSet struct {
	ordinary   []ProductLane
	coordinate []DynamicReadCoordinateDemand
}

// DynamicReadPotentialLanes returns the registration-owned residual lane
// envelope of the dynamic-read binder. Exact scalar coordinate demands remain
// query dependent, but no lane outside this set can ever be observed. Catalog
// growth is therefore reflected here without an operation-side axis list.
func (d ProductDomain) DynamicReadPotentialLanes() (LaneSet, error) {
	if !d.Valid() {
		return LaneSet{}, fmt.Errorf("%w: invalid dynamic-read domain", ErrInvalidLaneFactor)
	}
	lanes := make([]LaneID, 0, len(d.factorLanes))
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		participates := runtime.dynamicRead.demand != nil
		for j := range runtime.coordinates {
			participates = participates || runtime.coordinates[j].dynamicRead.participates
		}
		if participates && runtime.lane.ID() != LaneValues {
			lanes = append(lanes, runtime.lane.ID())
		}
	}
	return NewLaneSet(lanes...), nil
}

func (d DynamicReadDemandSet) OrdinaryLanes() []ProductLane {
	return append([]ProductLane(nil), d.ordinary...)
}
func (d DynamicReadDemandSet) CoordinateDemands() []DynamicReadCoordinateDemand {
	out := make([]DynamicReadCoordinateDemand, len(d.coordinate))
	for i, demand := range d.coordinate {
		out[i] = DynamicReadCoordinateDemand{
			family: demand.family, needSkeleton: demand.needSkeleton,
			scalarDefaultsUseTopology: demand.scalarDefaultsUseTopology,
			slots:                     append([]CoordinateSlot(nil), demand.slots...),
		}
	}
	return out
}
func (d DynamicReadDemandSet) Empty() bool { return len(d.ordinary) == 0 && len(d.coordinate) == 0 }

// DynamicReadCoordinateBatch answers one exact coordinate demand. Family is
// explicit so guarded evaluators can preserve identity without reconstructing
// a LaneFactor. Skeleton is meaningful exactly when HasSkeleton is true.
type DynamicReadCoordinateBatch struct {
	Family      CoordinateFamily
	Skeleton    CoordinateFamilySkeleton
	HasSkeleton bool
	Scalars     []CoordinateScalarFactor
}

// DynamicReadEvidenceBatch answers one demand round in demand order.
type DynamicReadEvidenceBatch struct {
	Ordinary   []LaneFactor
	Coordinate []DynamicReadCoordinateBatch
}

// DynamicReadFactorProjectionPlan is the registration-sealed factor topology
// shared by every correlated query over one evaluator frame. Query values and
// demanded scalar roots remain runtime data; lane/family discovery does not.
type DynamicReadFactorProjectionPlan struct {
	seal       *productDomainSeal
	keys       *keyspace.KeySpace
	ordinary   []ProductLane
	coordinate []CoordinateFamily
}

// ValidFor reports whether this plan belongs to the exact product/keyspace.
func (p DynamicReadFactorProjectionPlan) ValidFor(d ProductDomain, keys *keyspace.KeySpace) bool {
	return d.Valid() && p.seal == d.seal && p.keys == keys && keys != nil && keys.Valid()
}

type dynamicReadProjectedCoordinateFamily struct {
	family          CoordinateFamily
	factor          LaneFactor
	bound           bool
	prepared        bool
	direct          bool
	skeleton        CoordinateFamilySkeleton
	scalars         []CoordinateScalarFactor
	byHash          map[uint64][]int
	authorityByHash map[uint64][]CoordinateSlot
}

// DynamicReadCoordinateFactorSource is an explicitly bounded direct-family
// source. Inventory distinguishes an omitted authorized scalar (whose default
// is meaningful) from a demand outside the sparse carrier's authority.
type DynamicReadCoordinateFactorSource struct {
	factor    CoordinateFamilyFactor
	inventory CoordinateFactorInventory
}

// SealDynamicReadCoordinateFactorSource binds a direct coordinate factor to
// its complete declared scalar authority.
func (d ProductDomain) SealDynamicReadCoordinateFactorSource(
	inventory CoordinateFactorInventory,
	factor CoordinateFamilyFactor,
) (DynamicReadCoordinateFactorSource, error) {
	if !inventory.ValidFor(d, inventory.KeySpace()) || factor.Family().seal != d.seal ||
		factor.Skeleton().keys != inventory.KeySpace() {
		return DynamicReadCoordinateFactorSource{}, fmt.Errorf("%w: invalid direct dynamic-read coordinate source", ErrInvalidLaneFactor)
	}
	if _, err := d.SealCoordinateFamilyFactor(factor.Skeleton(), factor.Scalars()); err != nil {
		return DynamicReadCoordinateFactorSource{}, err
	}
	for _, scalar := range factor.Scalars() {
		owned := false
		for _, slot := range inventory.Slots() {
			equal, err := d.CoordinateSlotEqual(slot, scalar.slot)
			if err != nil {
				return DynamicReadCoordinateFactorSource{}, err
			}
			owned = owned || equal
		}
		if !owned {
			return DynamicReadCoordinateFactorSource{}, fmt.Errorf("%w: direct dynamic-read scalar is outside inventory", ErrInvalidLaneFactor)
		}
	}
	return DynamicReadCoordinateFactorSource{factor: factor, inventory: inventory}, nil
}

// DynamicReadFactorProjection is one evaluator-frame binding of a sealed
// topology. A query-disabled family is never decomposed; the first demand for
// an enabled family prepares its scalar index exactly once for this frame.
type DynamicReadFactorProjection struct {
	plan       DynamicReadFactorProjectionPlan
	byOrdinal  map[LaneOrdinal]LaneFactor
	coordinate []dynamicReadProjectedCoordinateFamily
}

// SealDynamicReadFactorProjectionPlan freezes the complete registered
// DynamicRead factor envelope without observing any runtime query or factor.
func (d ProductDomain) SealDynamicReadFactorProjectionPlan(keys *keyspace.KeySpace) (DynamicReadFactorProjectionPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return DynamicReadFactorProjectionPlan{}, fmt.Errorf("%w: invalid dynamic-read projection topology", ErrInvalidLaneFactor)
	}
	out := DynamicReadFactorProjectionPlan{seal: d.seal, keys: keys}
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		if runtime.dynamicRead.demand != nil {
			out.ordinary = append(out.ordinary, runtime.lane)
		}
		for j := range runtime.coordinates {
			coordinate := runtime.coordinates[j]
			if coordinate.dynamicRead.participates {
				out.coordinate = append(out.coordinate, coordinate.family)
			}
		}
	}
	return out, nil
}

// BindDynamicReadFactorProjection transposes one evaluator frame into its
// presealed query topology. Extra supplied lanes are permitted because the
// same frame may serve other semantic operations; every planned participant
// must be present exactly once.
func (d ProductDomain) BindDynamicReadFactorProjection(
	plan DynamicReadFactorProjectionPlan,
	supplied []LaneFactor,
	direct ...DynamicReadCoordinateFactorSource,
) (DynamicReadFactorProjection, error) {
	if !plan.ValidFor(d, plan.keys) {
		return DynamicReadFactorProjection{}, fmt.Errorf("%w: foreign dynamic-read projection topology", ErrInvalidLaneFactor)
	}
	out := DynamicReadFactorProjection{
		plan: plan, byOrdinal: make(map[LaneOrdinal]LaneFactor, len(supplied)),
		coordinate: make([]dynamicReadProjectedCoordinateFamily, len(plan.coordinate)),
	}
	for _, factor := range supplied {
		runtime, err := d.validateFactor(factor)
		if err != nil {
			return DynamicReadFactorProjection{}, err
		}
		if _, duplicate := out.byOrdinal[runtime.lane.ordinal]; duplicate {
			return DynamicReadFactorProjection{}, fmt.Errorf("%w: duplicate dynamic-read projection factor", ErrIncompleteLaneFactors)
		}
		out.byOrdinal[runtime.lane.ordinal] = factor
	}
	for _, lane := range plan.ordinary {
		if _, present := out.byOrdinal[lane.ordinal]; !present {
			return DynamicReadFactorProjection{}, fmt.Errorf("%w: missing dynamic-read projection lane %q", ErrIncompleteLaneFactors, lane.id)
		}
	}
	for index, family := range plan.coordinate {
		factor, present := out.byOrdinal[family.lane.ordinal]
		out.coordinate[index] = dynamicReadProjectedCoordinateFamily{family: family, factor: factor, bound: present}
	}
	for _, source := range direct {
		if !source.inventory.ValidFor(d, plan.keys) || source.factor.Family().seal != d.seal {
			return DynamicReadFactorProjection{}, fmt.Errorf("%w: foreign direct dynamic-read coordinate source", ErrInvalidLaneFactor)
		}
		matched := false
		for index := range out.coordinate {
			prepared := &out.coordinate[index]
			if prepared.family != source.factor.Family() {
				continue
			}
			if prepared.bound {
				return DynamicReadFactorProjection{}, fmt.Errorf("%w: duplicate whole-lane and direct dynamic-read family %q", ErrIncompleteLaneFactors, prepared.family.id)
			}
			prepared.bound, prepared.prepared, prepared.direct = true, true, true
			prepared.skeleton, prepared.scalars = source.factor.Skeleton(), source.factor.Scalars()
			prepared.byHash = make(map[uint64][]int, len(prepared.scalars))
			prepared.authorityByHash = make(map[uint64][]CoordinateSlot, source.inventory.Len())
			for _, slot := range source.inventory.Slots() {
				if slot.family != prepared.family {
					continue
				}
				hash, hashErr := d.CoordinateSlotHash(slot)
				if hashErr != nil {
					return DynamicReadFactorProjection{}, hashErr
				}
				prepared.authorityByHash[hash] = append(prepared.authorityByHash[hash], slot)
			}
			for scalarIndex, scalar := range prepared.scalars {
				hash, hashErr := d.CoordinateSlotHash(scalar.slot)
				if hashErr != nil {
					return DynamicReadFactorProjection{}, hashErr
				}
				prepared.byHash[hash] = append(prepared.byHash[hash], scalarIndex)
			}
			matched = true
			break
		}
		if !matched {
			return DynamicReadFactorProjection{}, fmt.Errorf("%w: unplanned direct dynamic-read coordinate family", ErrInvalidLaneFactor)
		}
	}
	return out, nil
}

// DynamicReadQueryPlan is an immutable, ProductDomain-owned binder state.
// Callers can only advance it with evidence for the exact current demand.
type DynamicReadQueryPlan struct {
	seal       *productDomainSeal
	query      DynamicReadQuery
	selection  DynamicReadSelection
	ordinary   []dynamicReadOrdinaryBinding
	coordinate []dynamicReadCoordinateBinding
	projection dynamicReadProjectionEvidence
	complete   bool
	evidence   DynamicReadEvidence
}

// Query returns the immutable correlated terminal query owned by this plan.
func (p DynamicReadQueryPlan) Query() DynamicReadQuery {
	out := p.query
	out.TableKeys = append([]pathaddr.StateKey(nil), p.query.TableKeys...)
	out.KeyKeys = append([]pathaddr.StateKey(nil), p.query.KeyKeys...)
	return out
}

type dynamicReadOrdinaryBinding struct {
	lane     ProductLane
	policy   laneDynamicReadPolicy
	observed bool
	factor   LaneFactor
}

type dynamicReadCoordinateBinding struct {
	family        CoordinateFamily
	policy        coordinateDynamicReadPolicy
	skeleton      CoordinateFamilySkeleton
	hasSkeleton   bool
	slots         []CoordinateSlot
	scalars       []CoordinateScalarFactor
	pathAuthority CoordinatePathEvidenceAuthority[statekey.Value]
}

// DynamicReadAdvance is one binder state together with its next exact demand,
// or the complete evidence when Complete is true.
type DynamicReadAdvance struct {
	Plan     DynamicReadQueryPlan
	Demands  DynamicReadDemandSet
	Evidence DynamicReadEvidence
	Complete bool
}

// DynamicReadDemandCursor is the borrowed execution form of the registered
// dynamic-read demand law.  It deliberately owns no selection policy: every
// round is still produced by PlanDynamicRead/AdvanceDynamicRead.  Consumers
// keep the cursor on their execution scratch and answer the current demand
// synchronously, which makes a guarded evaluator able to refine its carrier
// before asking the binder to resume.
//
// The cursor has no iteration budget.  The registration contract guarantees
// that seed/advance demands are monotone, finite, and structurally terminating.
type DynamicReadDemandCursor struct {
	domain  ProductDomain
	advance DynamicReadAdvance
}

// BeginDynamicReadDemandCursor starts one exact registered demand transaction.
// The returned cursor borrows the immutable ProductDomain; callers must not
// substitute an independently reconstructed query plan.
func (d ProductDomain) BeginDynamicReadDemandCursor(query DynamicReadQuery) (DynamicReadDemandCursor, error) {
	advance, err := d.PlanDynamicRead(query)
	if err != nil {
		return DynamicReadDemandCursor{}, err
	}
	return DynamicReadDemandCursor{domain: d, advance: advance}, nil
}

// Complete reports whether the registered binder has accepted every demanded
// observation and produced final evidence.
func (c DynamicReadDemandCursor) Complete() bool { return c.advance.Complete }

// Demands borrows the exact current demand set.  It is meaningful only until
// the next Resume call; callers must answer it directly instead of retaining
// detached per-region demand envelopes.
func (c DynamicReadDemandCursor) Demands() DynamicReadDemandSet { return c.advance.Demands }

// Evidence returns the final registered evidence after Complete becomes true.
func (c DynamicReadDemandCursor) Evidence() (DynamicReadEvidence, bool) {
	if !c.advance.Complete {
		return DynamicReadEvidence{}, false
	}
	return c.advance.Evidence, true
}

// Resume consumes exactly the cursor's current demand and advances the
// registration-owned state machine.  A caller that omits a demanded slot gets
// the same hard error as the direct AdvanceDynamicRead API; omission can never
// be interpreted as a coordinate default.
func (c *DynamicReadDemandCursor) Resume(batch DynamicReadEvidenceBatch) error {
	if c == nil || !c.domain.Valid() || c.advance.Complete || c.advance.Demands.Empty() {
		return fmt.Errorf("%w: invalid dynamic-read demand cursor", ErrInvalidLaneFactor)
	}
	next, err := c.domain.AdvanceDynamicRead(c.advance.Plan, batch)
	if err != nil {
		return err
	}
	c.advance = next
	return nil
}

// ProjectOrdinaryDemand applies the cursor's own sealed projection to one
// currently demanded ordinary lane.  Consumers must use this instead of
// reconstructing a projection plan while answering sparse cursor rounds.
func (c DynamicReadDemandCursor) ProjectOrdinaryDemand(lane ProductLane, factor LaneFactor) (LaneFactor, error) {
	if !c.domain.Valid() || c.advance.Complete || c.advance.Demands.Empty() {
		return LaneFactor{}, fmt.Errorf("%w: invalid dynamic-read demand cursor", ErrInvalidLaneFactor)
	}
	return c.domain.ProjectDynamicReadLane(c.advance.Plan, lane, factor)
}

type dynamicReadBuilder struct {
	facts          dynamicIndexLane
	hasFacts       bool
	memberships    keyMembershipLane
	hasMemberships bool
	evidence       DynamicReadEvidence
}

type dynamicReadPlanner struct {
	domain  ProductDomain
	plan    *DynamicReadQueryPlan
	builder *dynamicReadBuilder
}

// PlanDynamicRead starts the deterministic finite demand protocol.
func (d ProductDomain) PlanDynamicRead(query DynamicReadQuery) (DynamicReadAdvance, error) {
	if !d.Valid() || query.KeySpace == nil || !query.KeySpace.Valid() ||
		!product.BelongsToRegistry(d.reg, query.TableValue) || !product.BelongsToRegistry(d.reg, query.KeyValue) ||
		query.HasOwnerValue && !product.BelongsToRegistry(d.reg, query.OwnerValue) ||
		query.HasRangeContainer && !product.BelongsToRegistry(d.reg, query.RangeContainer) {
		return DynamicReadAdvance{}, fmt.Errorf("%w: invalid dynamic-read query", ErrInvalidLaneFactor)
	}
	if query.HasRange && (!query.Range.Shape.Valid() || query.Range.ArrayStateKey == "") {
		return DynamicReadAdvance{}, fmt.Errorf("%w: invalid dynamic-read range pair", ErrInvalidLaneFactor)
	}
	if query.HasRange && query.Range.Shape.Kind() == indexform.IndexFormAffine && query.Range.IndexStateKey == "" {
		return DynamicReadAdvance{}, fmt.Errorf("%w: missing dynamic-read affine address", ErrInvalidLaneFactor)
	}
	query.TableKeys = append([]pathaddr.StateKey(nil), query.TableKeys...)
	query.KeyKeys = append([]pathaddr.StateKey(nil), query.KeyKeys...)
	selection, err := d.PrepareDynamicReadSelection(query)
	if err != nil {
		return DynamicReadAdvance{}, err
	}
	plan := DynamicReadQueryPlan{seal: d.seal, query: query, selection: selection}
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		if runtime.dynamicRead.demand != nil && runtime.dynamicRead.demand(query) {
			plan.ordinary = append(plan.ordinary, dynamicReadOrdinaryBinding{lane: runtime.lane, policy: runtime.dynamicRead})
		}
		for j := range runtime.coordinates {
			coordinate := runtime.coordinates[j]
			if coordinate.dynamicRead.participates && (coordinate.dynamicRead.demand == nil || coordinate.dynamicRead.demand(query)) {
				plan.coordinate = append(plan.coordinate, dynamicReadCoordinateBinding{family: coordinate.family, policy: coordinate.dynamicRead})
			}
		}
	}
	planner := dynamicReadPlanner{domain: d, plan: &plan}
	for i := range plan.coordinate {
		binding := &plan.coordinate[i]
		binding.hasSkeleton = false
		if err := binding.policy.seed(&planner, binding); err != nil {
			return DynamicReadAdvance{}, err
		}
	}
	pathFamily, hasPathFamily := d.PathEvidenceCoordinateFamily()
	if hasPathFamily {
		for i := range plan.coordinate {
			binding := &plan.coordinate[i]
			if binding.family != pathFamily {
				continue
			}
			reads, inventoryErr := d.SealCoordinateFactorInventory(query.KeySpace, binding.slots)
			if inventoryErr != nil {
				return DynamicReadAdvance{}, inventoryErr
			}
			empty, inventoryErr := d.SealCoordinateFactorInventory(query.KeySpace, nil)
			if inventoryErr != nil {
				return DynamicReadAdvance{}, inventoryErr
			}
			binding.pathAuthority, inventoryErr = SealCoordinatePathEvidenceAuthority(
				d, query.KeySpace, nil, nil, reads, empty, false, false,
				func(slot statekey.Value) bool { return slot != 0 },
			)
			if inventoryErr != nil {
				return DynamicReadAdvance{}, inventoryErr
			}
		}
	}
	return d.dynamicReadAdvance(plan)
}

func dynamicReadNumBoundCoordinates(direction numbound.Direction) coordinateDynamicReadPolicy {
	policy := dynamicReadCoordinateParticipantWhen(
		func(query DynamicReadQuery) bool {
			return query.HasRange && query.Range.Shape.Kind() == indexform.IndexFormAffine
		},
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			path, ok := planner.plan.query.KeySpace.InternStateKey(planner.plan.query.Range.IndexStateKey)
			if !ok {
				return fmt.Errorf("%w: dynamic-read numeric floor address", ErrInvalidLaneFactor)
			}
			return planner.addSlot(binding, CoordinateSlot{family: binding.family, keys: planner.plan.query.KeySpace, key: wrapNumBoundCoordinateKey(path)})
		},
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			if planner.builder == nil {
				return nil
			}
			if direction == numbound.Lower && planner.builder.evidence.rangeNumFloors == nil {
				planner.builder.evidence.rangeNumFloors = make(map[pathaddr.StateKey]int64)
			}
			for _, scalar := range binding.scalars {
				omitted, present, err := planner.domain.CoordinateReachableDefault(scalar.Slot())
				if err != nil {
					return err
				}
				isOmitted := false
				if present {
					isOmitted, err = planner.domain.CoordinateScalarEqual(omitted, scalar)
					if err != nil {
						return err
					}
				}
				bound := numBoundCoordinateScalarValue(scalar.payload)
				path := numBoundCoordinateKeyValue(scalar.slot.key).path
				stateKey, keyOK := pathaddr.StateKeyFromPathKey(planner.plan.query.KeySpace.FormatReadOnly(path))
				if isOmitted || !bound.present || !keyOK {
					continue
				}
				if direction == numbound.Lower {
					planner.builder.evidence.rangeNumFloors[stateKey] = bound.value
					if stateKey == planner.plan.query.Range.IndexStateKey {
						planner.builder.evidence.rangeIndexFloor = bound.value
						planner.builder.evidence.hasRangeIndexFloor = true
					}
				} else if stateKey == planner.plan.query.Range.IndexStateKey {
					planner.builder.evidence.rangeIndexCeil = bound.value
					planner.builder.evidence.hasRangeIndexCeil = true
				}
			}
			return nil
		},
	)
	policy.rangeEvidence = true
	return policy
}

func dynamicReadNumFloorCoordinates() coordinateDynamicReadPolicy {
	return dynamicReadNumBoundCoordinates(numbound.Lower)
}

func dynamicReadNumCeilCoordinates() coordinateDynamicReadPolicy {
	return dynamicReadNumBoundCoordinates(numbound.Upper)
}

func dynamicReadLenFloorCoordinates() coordinateDynamicReadPolicy {
	policy := dynamicReadCoordinateParticipantWhen(
		func(query DynamicReadQuery) bool { return query.HasRange },
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			path, ok := planner.plan.query.KeySpace.InternStateKey(planner.plan.query.Range.ArrayStateKey)
			if !ok {
				return fmt.Errorf("%w: dynamic-read array length address", ErrInvalidLaneFactor)
			}
			return planner.addSlot(binding, CoordinateSlot{family: binding.family, keys: planner.plan.query.KeySpace, key: wrapLenFloorCoordinateKey(path)})
		},
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			if planner.builder == nil || len(binding.scalars) != 1 {
				return nil
			}
			floor := lenFloorCoordinateScalarValue(binding.scalars[0].payload).floor.Lo
			if floor > 0 && floor != lenbound.BottomFloor().Lo {
				planner.builder.evidence.rangeArrayLenFloor = floor
				planner.builder.evidence.hasRangeArrayFloor = true
			}
			return nil
		},
	)
	policy.rangeEvidence = true
	return policy
}

func dynamicReadDiffRelationCoordinates() coordinateDynamicReadPolicy {
	policy := dynamicReadCoordinateParticipantWhen(
		func(query DynamicReadQuery) bool {
			return query.HasRange && query.Range.Shape.Kind() == indexform.IndexFormAffine
		},
		// The first round deliberately requests only the static skeleton. Shape
		// component closure must precede scalar/DD alignment.
		func(_ *dynamicReadPlanner, _ *dynamicReadCoordinateBinding) error { return nil },
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			if planner.builder == nil {
				return nil
			}
			query := planner.plan.query
			component, err := planner.domain.DiffRelationShapeComponent(binding.skeleton, []RelOperand{
				RelValueOperand(query.Range.IndexStateKey), RelLengthOperand(query.Range.ArrayStateKey),
			})
			if err != nil {
				return fmt.Errorf("dynamic-read relation component: %w", err)
			}
			priorSlots := len(binding.slots)
			for _, slot := range component {
				if err := planner.addSlot(binding, slot); err != nil {
					return err
				}
			}
			if len(binding.slots) != priorSlots {
				return nil
			}
			coeff, offset, ok := query.Range.Shape.Affine()
			if !ok {
				return fmt.Errorf("%w: dynamic-read affine form", ErrInvalidLaneFactor)
			}
			constraints := make(map[RelConstraint]struct{})
			operands := make(map[RelOperand]struct{})
			for _, scalar := range binding.scalars {
				incident, _, err := planner.domain.DiffRelationShapeConstraints(scalar)
				if err != nil {
					return fmt.Errorf("dynamic-read relation shape: %w", err)
				}
				for _, relation := range incident {
					constraints[relation] = struct{}{}
					for _, operand := range relation.AppendOperands(nil) {
						operands[operand] = struct{}{}
					}
				}
			}
			floorBinding := dynamicReadBindingByFamilyID(planner.plan, numFloorCoordinateFamilyID)
			if floorBinding == nil {
				return fmt.Errorf("%w: dynamic-read numeric floor family", ErrIncompleteLaneFactors)
			}
			priorFloorSlots := len(floorBinding.slots)
			for operand := range operands {
				if operand.Kind == RelOperandValue {
					path, pathOK := query.KeySpace.InternStateKey(operand.Key)
					if !pathOK {
						return fmt.Errorf("%w: dynamic-read relation operand", ErrInvalidLaneFactor)
					}
					if err := planner.addSlot(floorBinding, CoordinateSlot{family: floorBinding.family, keys: query.KeySpace, key: wrapNumBoundCoordinateKey(path)}); err != nil {
						return err
					}
				}
			}
			if len(floorBinding.slots) != priorFloorSlots {
				return nil
			}
			planner.builder.evidence.rangeRelations = planner.builder.evidence.rangeRelations[:0]
			for relation := range constraints {
				planner.builder.evidence.rangeRelations = append(planner.builder.evidence.rangeRelations, relation)
			}
			sort.Slice(planner.builder.evidence.rangeRelations, func(i, j int) bool {
				return relConstraintLess(planner.builder.evidence.rangeRelations[i], planner.builder.evidence.rangeRelations[j])
			})
			asserted := make([]numeric.NumericConstraint, 0, len(constraints)+len(planner.builder.evidence.rangeNumFloors))
			for _, relation := range planner.builder.evidence.rangeRelations {
				asserted = append(asserted, relation.NumericConstraint())
			}
			floorKeys := make([]pathaddr.StateKey, 0, len(planner.builder.evidence.rangeNumFloors))
			for key := range planner.builder.evidence.rangeNumFloors {
				floorKeys = append(floorKeys, key)
			}
			sort.Slice(floorKeys, func(i, j int) bool { return floorKeys[i] < floorKeys[j] })
			for _, key := range floorKeys {
				asserted = append(asserted, numeric.GeConst{X: key.PathKey(), C: planner.builder.evidence.rangeNumFloors[key]})
			}
			goal := numeric.NewScaledLe(coeff, RelValueOperand(query.Range.IndexStateKey).NumericKey(), 0, "", RelLengthOperand(query.Range.ArrayStateKey).NumericKey(), -offset)
			planner.builder.evidence.rangeDiffProof = solver.DefaultPortfolio().Entails(asserted, goal) == decision.Valid
			return nil
		},
	)
	policy.rangeEvidence = true
	return policy
}

func dynamicReadBindingByFamilyID(plan *DynamicReadQueryPlan, id CoordinateFamilyID) *dynamicReadCoordinateBinding {
	if plan == nil {
		return nil
	}
	for index := range plan.coordinate {
		if plan.coordinate[index].family.id == id {
			return &plan.coordinate[index]
		}
	}
	return nil
}

// ProjectDynamicReadLane applies one registration-owned semantic quotient to
// an ordinary lane before guarded tuple alignment. The output remains a valid
// factor of the same lane and projection is idempotent.
func (d ProductDomain) ProjectDynamicReadLane(plan DynamicReadQueryPlan, lane ProductLane, factor LaneFactor) (LaneFactor, error) {
	if !d.Valid() || plan.seal != d.seal {
		return LaneFactor{}, fmt.Errorf("%w: foreign dynamic-read projection plan", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane != lane {
		return LaneFactor{}, fmt.Errorf("%w: dynamic-read projection lane", ErrInvalidLaneFactor)
	}
	var policy laneDynamicReadPolicy
	found := false
	for i := range plan.ordinary {
		if plan.ordinary[i].lane == lane {
			policy, found = plan.ordinary[i].policy, true
			break
		}
	}
	if !found || policy.project == nil {
		return LaneFactor{}, fmt.Errorf("%w: lane has no dynamic-read projection", ErrInvalidLaneFactor)
	}
	payload, err := policy.project(d, plan, factor.payload)
	if err != nil || payload == nil {
		if err == nil {
			err = ErrInvalidLaneFactor
		}
		return LaneFactor{}, err
	}
	projected := LaneFactor{lane: lane, payload: payload}
	if _, err := d.validateFactorFor(runtime, projected); err != nil {
		return LaneFactor{}, err
	}
	return projected, nil
}

func projectDynamicReadMemberships(_ ProductDomain, plan DynamicReadQueryPlan, payload laneFactorPayload) (laneFactorPayload, error) {
	source := typedLaneFactorValue[keyMembershipLane](payload)
	if source.bottom {
		return typedLaneFactorPayload[keyMembershipLane]{value: keyMembershipLane{bottom: true}}, nil
	}
	projected := keyMembershipLane{}
	for _, key := range plan.query.KeyKeys {
		for _, table := range plan.query.TableKeys {
			membership := PathKeyMembership(key, table)
			if source.hasPathKeyMembership(key, table) {
				projected, _ = projected.add(membership)
			}
		}
	}
	return typedLaneFactorPayload[keyMembershipLane]{value: projected}, nil
}

func projectDynamicReadFacts(d ProductDomain, plan DynamicReadQueryPlan, payload laneFactorPayload) (laneFactorPayload, error) {
	source := typedLaneFactorValue[dynamicIndexLane](payload)
	projected := dynamicIndexLane{}
	if source.top {
		return typedLaneFactorPayload[dynamicIndexLane]{value: projected}, nil
	}
	values := make(map[dynamicindex.Key]dynamicindex.Fact)
	for factKey, fact := range source.values {
		if dynamicReadFactRelevant(d, plan.selection, plan.projection.keyMembershipProven, factKey, fact) {
			values[factKey] = fact
		}
	}
	projected.values = values
	return typedLaneFactorPayload[dynamicIndexLane]{value: projected}, nil
}

func dynamicReadFactRelevant(d ProductDomain, selection DynamicReadSelection, membershipProven bool, factKey dynamicindex.Key, fact dynamicindex.Fact) bool {
	return selection.FactRelevant(d, membershipProven, factKey, fact)
}

// AdvanceDynamicRead validates and consumes exactly one outstanding demand.
func (d ProductDomain) AdvanceDynamicRead(plan DynamicReadQueryPlan, batch DynamicReadEvidenceBatch) (DynamicReadAdvance, error) {
	if !d.Valid() || plan.seal != d.seal || plan.complete {
		return DynamicReadAdvance{}, fmt.Errorf("%w: invalid dynamic-read plan", ErrInvalidLaneFactor)
	}
	next := cloneDynamicReadPlan(plan)
	demand := d.dynamicReadDemands(next)
	if len(batch.Ordinary) != len(demand.ordinary) || len(batch.Coordinate) != len(demand.coordinate) {
		return DynamicReadAdvance{}, fmt.Errorf("%w: incomplete dynamic-read evidence batch", ErrIncompleteLaneFactors)
	}
	for ordinaryIndex, lane := range demand.ordinary {
		var binding *dynamicReadOrdinaryBinding
		for i := range next.ordinary {
			if !next.ordinary[i].observed && next.ordinary[i].lane == lane {
				binding = &next.ordinary[i]
				break
			}
		}
		if binding == nil {
			return DynamicReadAdvance{}, fmt.Errorf("%w: dynamic-read ordinary demand", ErrInvalidLaneFactor)
		}
		factor := batch.Ordinary[ordinaryIndex]
		runtime, laneErr := d.validateLane(binding.lane)
		if laneErr != nil {
			return DynamicReadAdvance{}, laneErr
		}
		if _, err := d.validateFactorFor(runtime, factor); err != nil {
			return DynamicReadAdvance{}, fmt.Errorf("%w: dynamic-read ordinary factor", err)
		}
		projected, projectErr := d.ProjectDynamicReadLane(next, binding.lane, factor)
		if projectErr != nil {
			return DynamicReadAdvance{}, projectErr
		}
		equal, equalErr := d.LaneEqual(factor, projected)
		if equalErr != nil || !equal {
			return DynamicReadAdvance{}, fmt.Errorf("%w: unprojected dynamic-read ordinary factor", ErrInvalidLaneFactor)
		}
		binding.factor, binding.observed = factor, true
		if binding.policy.contribute != nil {
			binding.policy.contribute(factor.payload, next.query, &next.projection)
		}
	}
	for i, answer := range batch.Coordinate {
		want := demand.coordinate[i]
		if answer.Family != want.family || answer.HasSkeleton != want.needSkeleton || len(answer.Scalars) != len(want.slots) {
			return DynamicReadAdvance{}, fmt.Errorf("%w: dynamic-read coordinate demand mismatch", ErrInvalidLaneFactor)
		}
		binding := dynamicReadBinding(&next, want.family)
		if binding == nil {
			return DynamicReadAdvance{}, fmt.Errorf("%w: dynamic-read coordinate family", ErrInvalidLaneFactor)
		}
		coordinate, err := d.validateCoordinateFamily(want.family)
		if err != nil {
			return DynamicReadAdvance{}, err
		}
		if want.needSkeleton {
			if answer.Skeleton.payload == nil {
				return DynamicReadAdvance{}, fmt.Errorf("%w: empty dynamic-read skeleton answer for %q", ErrInvalidLaneFactor, want.family.id)
			}
			if err := d.validateCoordinateSkeletonFor(coordinate, answer.Skeleton, next.query.KeySpace); err != nil {
				return DynamicReadAdvance{}, err
			}
			binding.skeleton, binding.hasSkeleton = answer.Skeleton, true
		}
		for scalarIndex, scalar := range answer.Scalars {
			if err := d.validateCoordinateFactorFor(coordinate, scalar, next.query.KeySpace); err != nil {
				return DynamicReadAdvance{}, err
			}
			equal, err := d.CoordinateSlotEqual(scalar.slot, want.slots[scalarIndex])
			if err != nil || !equal {
				return DynamicReadAdvance{}, fmt.Errorf("%w: dynamic-read scalar order", ErrInvalidLaneFactor)
			}
			binding.scalars = append(binding.scalars, scalar)
		}
	}
	return d.dynamicReadAdvance(next)
}

func (d ProductDomain) dynamicReadAdvance(plan DynamicReadQueryPlan) (DynamicReadAdvance, error) {
	demands := d.dynamicReadDemands(plan)
	if !demands.Empty() {
		return DynamicReadAdvance{Plan: plan, Demands: demands}, nil
	}
	builder := dynamicReadBuilder{}
	builder.evidence.domain = d
	builder.evidence.query = plan.query
	for i := range plan.ordinary {
		binding := plan.ordinary[i]
		if !binding.observed || binding.policy.observe == nil {
			return DynamicReadAdvance{}, fmt.Errorf("%w: missing dynamic-read ordinary evidence", ErrIncompleteLaneFactors)
		}
		if err := binding.policy.observe(binding.factor.payload, &builder); err != nil {
			return DynamicReadAdvance{}, err
		}
	}
	planner := dynamicReadPlanner{domain: d, plan: &plan, builder: &builder}
	for {
		priorSlots := dynamicReadSlotCount(plan)
		priorEvidence := cloneDynamicReadEvidence(builder.evidence)
		for i := range plan.coordinate {
			binding := &plan.coordinate[i]
			if binding.policy.topologyRequired && !binding.hasSkeleton || binding.policy.advance == nil {
				return DynamicReadAdvance{}, fmt.Errorf("%w: missing dynamic-read coordinate evidence", ErrIncompleteLaneFactors)
			}
			if err := binding.policy.advance(&planner, binding); err != nil {
				return DynamicReadAdvance{}, err
			}
		}
		finalizeDynamicReadRangeProof(plan.query, &builder.evidence)
		demands = d.dynamicReadDemands(plan)
		if !demands.Empty() {
			if dynamicReadSlotCount(plan) <= priorSlots {
				return DynamicReadAdvance{}, fmt.Errorf("%w: non-progressing dynamic-read demand", ErrInvalidLaneFactor)
			}
			return DynamicReadAdvance{Plan: plan, Demands: demands}, nil
		}
		if dynamicReadEvidenceEqual(d.reg, priorEvidence, builder.evidence) {
			break
		}
	}
	if err := resolveDynamicReadOrdinaryEvidence(d, plan.selection, &builder); err != nil {
		return DynamicReadAdvance{}, err
	}
	plan.complete = true
	plan.evidence = cloneDynamicReadEvidence(builder.evidence)
	return DynamicReadAdvance{Plan: plan, Evidence: cloneDynamicReadEvidence(plan.evidence), Complete: true}, nil
}

func (d ProductDomain) dynamicReadDemands(plan DynamicReadQueryPlan) DynamicReadDemandSet {
	out := DynamicReadDemandSet{}
	var ordinaryPhase uint8
	hasOrdinaryPhase := false
	for i := range plan.ordinary {
		if plan.ordinary[i].observed {
			continue
		}
		phase := plan.ordinary[i].policy.phase
		if !hasOrdinaryPhase || phase < ordinaryPhase {
			ordinaryPhase, hasOrdinaryPhase = phase, true
		}
	}
	for i := range plan.ordinary {
		if !plan.ordinary[i].observed && hasOrdinaryPhase && plan.ordinary[i].policy.phase == ordinaryPhase {
			out.ordinary = append(out.ordinary, plan.ordinary[i].lane)
		}
	}
	for i := range plan.coordinate {
		binding := &plan.coordinate[i]
		demand := DynamicReadCoordinateDemand{
			family:                    binding.family,
			needSkeleton:              binding.policy.topologyRequired && !binding.hasSkeleton,
			scalarDefaultsUseTopology: binding.policy.topologyRequired,
		}
		for _, slot := range binding.slots {
			if !dynamicReadHasScalar(d, binding, slot) {
				demand.slots = append(demand.slots, slot)
			}
		}
		if demand.needSkeleton || len(demand.slots) != 0 {
			out.coordinate = append(out.coordinate, demand)
		}
	}
	return out
}

// ProjectDynamicReadEvidence is the concrete adapter for the same sparse
// binder. It performs no semantic read of its own: every round projects only
// the plan's exact registered demands from State, then calls Advance.
func (d ProductDomain) ProjectDynamicReadEvidence(query DynamicReadQuery, input State) (DynamicReadEvidence, error) {
	return d.projectDynamicReadEvidence(query, input, input, false)
}

// ProjectDynamicReadEvidenceFactors is the factor-native adapter for the same
// finite demand binder used by ProjectDynamicReadEvidence.  supplied must
// contain exactly one factor for every lane demanded by query; coordinate
// families are decomposed directly from those factors.  No State is composed.
func (d ProductDomain) ProjectDynamicReadEvidenceFactors(query DynamicReadQuery, supplied []LaneFactor) (DynamicReadEvidence, error) {
	plan, err := d.SealDynamicReadFactorProjectionPlan(query.KeySpace)
	if err != nil {
		return DynamicReadEvidence{}, err
	}
	projection, err := d.BindDynamicReadFactorProjection(plan, supplied)
	if err != nil {
		return DynamicReadEvidence{}, err
	}
	return d.ProjectDynamicReadEvidenceFromFactorProjection(query, &projection)
}

// ProjectDynamicReadEvidenceFromFactorProjection executes the canonical
// demand protocol against one prebound evaluator frame. Only demanded scalar
// fibers are read; no coordinate family is decomposed or sorted here.
func (d ProductDomain) ProjectDynamicReadEvidenceFromFactorProjection(
	query DynamicReadQuery,
	projection *DynamicReadFactorProjection,
) (DynamicReadEvidence, error) {
	if projection == nil || !projection.plan.ValidFor(d, query.KeySpace) {
		return DynamicReadEvidence{}, fmt.Errorf("%w: foreign dynamic-read factor projection", ErrInvalidLaneFactor)
	}
	cursor, err := d.BeginDynamicReadDemandCursor(query)
	if err != nil {
		return DynamicReadEvidence{}, err
	}
	for !cursor.Complete() {
		demands := cursor.Demands()
		if demands.Empty() {
			return DynamicReadEvidence{}, fmt.Errorf("%w: empty incomplete dynamic-read demand", ErrIncompleteLaneFactors)
		}
		batch := DynamicReadEvidenceBatch{}
		for _, lane := range demands.OrdinaryLanes() {
			factor, ok := projection.byOrdinal[lane.ordinal]
			if !ok {
				return DynamicReadEvidence{}, fmt.Errorf("%w: missing dynamic-read lane %q", ErrIncompleteLaneFactors, lane.id)
			}
			projected, projectErr := d.ProjectDynamicReadLane(cursor.advance.Plan, lane, factor)
			if projectErr != nil {
				return DynamicReadEvidence{}, projectErr
			}
			batch.Ordinary = append(batch.Ordinary, projected)
		}
		for _, demand := range demands.CoordinateDemands() {
			var prepared *dynamicReadProjectedCoordinateFamily
			for index := range projection.coordinate {
				if projection.coordinate[index].family == demand.family {
					prepared = &projection.coordinate[index]
					break
				}
			}
			if prepared == nil {
				return DynamicReadEvidence{}, fmt.Errorf("%w: missing dynamic-read coordinate lane %q", ErrIncompleteLaneFactors, demand.family.lane.id)
			}
			if !prepared.bound {
				return DynamicReadEvidence{}, fmt.Errorf("%w: missing dynamic-read coordinate lane %q", ErrIncompleteLaneFactors, demand.family.lane.id)
			}
			if !prepared.prepared {
				skeleton, scalars, decomposeErr := d.DecomposeCoordinateFamily(prepared.factor, demand.family, query.KeySpace)
				if decomposeErr != nil {
					return DynamicReadEvidence{}, decomposeErr
				}
				prepared.skeleton, prepared.scalars = skeleton, scalars
				prepared.byHash = make(map[uint64][]int, len(scalars))
				for scalarIndex, scalar := range scalars {
					hash, hashErr := d.CoordinateSlotHash(scalar.slot)
					if hashErr != nil {
						return DynamicReadEvidence{}, hashErr
					}
					prepared.byHash[hash] = append(prepared.byHash[hash], scalarIndex)
				}
				prepared.prepared = true
			}
			answer := DynamicReadCoordinateBatch{Family: demand.family, HasSkeleton: demand.needSkeleton}
			if demand.needSkeleton {
				answer.Skeleton = prepared.skeleton
			}
			for _, slot := range demand.slots {
				hash, findErr := d.CoordinateSlotHash(slot)
				if findErr != nil {
					return DynamicReadEvidence{}, findErr
				}
				var scalar CoordinateScalarFactor
				found := false
				if prepared.direct {
					authorized := false
					for _, candidate := range prepared.authorityByHash[hash] {
						equal, equalErr := d.CoordinateSlotEqual(candidate, slot)
						if equalErr != nil {
							return DynamicReadEvidence{}, equalErr
						}
						authorized = authorized || equal
					}
					if !authorized {
						return DynamicReadEvidence{}, fmt.Errorf("%w: dynamic-read demand is outside direct coordinate authority", ErrIncompleteLaneFactors)
					}
				}
				for _, scalarIndex := range prepared.byHash[hash] {
					candidate := prepared.scalars[scalarIndex]
					equal, equalErr := d.CoordinateSlotEqual(candidate.slot, slot)
					if equalErr != nil {
						return DynamicReadEvidence{}, equalErr
					}
					if equal {
						scalar, found = candidate, true
						break
					}
				}
				if !found {
					scalar, findErr = d.CoordinateDefault(prepared.skeleton, slot)
					if findErr != nil {
						return DynamicReadEvidence{}, findErr
					}
				}
				answer.Scalars = append(answer.Scalars, scalar)
			}
			batch.Coordinate = append(batch.Coordinate, answer)
		}
		if err := cursor.Resume(batch); err != nil {
			return DynamicReadEvidence{}, err
		}
	}
	evidence, complete := cursor.Evidence()
	if !complete {
		return DynamicReadEvidence{}, fmt.Errorf("%w: incomplete dynamic-read demand cursor", ErrIncompleteLaneFactors)
	}
	return evidence, nil
}

// ProjectDynamicReadEvidenceWithProof projects value evidence from input and
// range evidence from proofInput through one registered demand transaction.
// This is the boundary-read law: values may be observed immediately before an
// operation while its exact proof facts are published by that operation. No
// proof Boolean or post-refinement bypasses the binder.
func (d ProductDomain) ProjectDynamicReadEvidenceWithProof(query DynamicReadQuery, input, proofInput State) (DynamicReadEvidence, error) {
	return d.projectDynamicReadEvidence(query, input, proofInput, true)
}

func (d ProductDomain) projectDynamicReadEvidence(query DynamicReadQuery, input, proofInput State, splitProof bool) (DynamicReadEvidence, error) {
	advance, err := d.PlanDynamicRead(query)
	if err != nil {
		return DynamicReadEvidence{}, err
	}
	for !advance.Complete {
		if advance.Demands.Empty() {
			return DynamicReadEvidence{}, fmt.Errorf("%w: empty incomplete dynamic-read demand", ErrIncompleteLaneFactors)
		}
		batch := DynamicReadEvidenceBatch{}
		if lanes := advance.Demands.OrdinaryLanes(); len(lanes) != 0 {
			batch.Ordinary, err = d.DecomposeLanes(input, lanes)
			if err != nil {
				return DynamicReadEvidence{}, err
			}
			for index, lane := range lanes {
				batch.Ordinary[index], err = d.ProjectDynamicReadLane(advance.Plan, lane, batch.Ordinary[index])
				if err != nil {
					return DynamicReadEvidence{}, err
				}
			}
		}
		for _, demand := range advance.Demands.CoordinateDemands() {
			binding := dynamicReadBinding(&advance.Plan, demand.family)
			if binding == nil {
				return DynamicReadEvidence{}, fmt.Errorf("%w: dynamic-read coordinate binding", ErrInvalidLaneFactor)
			}
			coordinateInput := input
			if splitProof && binding.policy.rangeEvidence {
				coordinateInput = proofInput
			}
			factors, decomposeErr := d.DecomposeLanes(coordinateInput, []ProductLane{demand.family.Lane()})
			if decomposeErr != nil || len(factors) != 1 {
				return DynamicReadEvidence{}, fmt.Errorf("%w: dynamic-read coordinate projection", ErrInvalidLaneFactor)
			}
			skeleton, scalars, decomposeErr := d.DecomposeCoordinateFamily(factors[0], demand.family, query.KeySpace)
			if decomposeErr != nil {
				return DynamicReadEvidence{}, decomposeErr
			}
			var proofSkeleton CoordinateFamilySkeleton
			var proofScalars []CoordinateScalarFactor
			mixedPathProof := splitProof && demand.family.id == pathEvidenceCoordinateFamilyID &&
				query.HasRange && query.Range.IndexProofStateKey != "" && query.Range.ArrayProofStateKey != ""
			if mixedPathProof {
				proofFactors, proofErr := d.DecomposeLanes(proofInput, []ProductLane{demand.family.Lane()})
				if proofErr != nil || len(proofFactors) != 1 {
					return DynamicReadEvidence{}, fmt.Errorf("%w: dynamic-read proof coordinate projection", ErrInvalidLaneFactor)
				}
				proofSkeleton, proofScalars, proofErr = d.DecomposeCoordinateFamily(proofFactors[0], demand.family, query.KeySpace)
				if proofErr != nil {
					return DynamicReadEvidence{}, proofErr
				}
				if demand.needSkeleton {
					skeleton, proofErr = d.CoordinateSkeletonJoin(skeleton, proofSkeleton)
					if proofErr != nil {
						return DynamicReadEvidence{}, proofErr
					}
				}
			}
			answer := DynamicReadCoordinateBatch{Family: demand.family, HasSkeleton: demand.needSkeleton}
			if demand.needSkeleton {
				answer.Skeleton = skeleton
			}
			proofSlot, hasProofSlot, proofSlotErr := dynamicReadRangeBranchProofSlot(d, query, demand.family)
			if proofSlotErr != nil {
				return DynamicReadEvidence{}, proofSlotErr
			}
			for _, slot := range demand.slots {
				scalarSource := scalars
				defaultSkeleton := skeleton
				if mixedPathProof && hasProofSlot {
					isProofSlot, equalErr := d.CoordinateSlotEqual(slot, proofSlot)
					if equalErr != nil {
						return DynamicReadEvidence{}, equalErr
					}
					if isProofSlot {
						scalarSource = proofScalars
						defaultSkeleton = proofSkeleton
					}
				}
				scalar, found, findErr := findDynamicReadScalar(d, scalarSource, slot)
				if findErr != nil {
					return DynamicReadEvidence{}, findErr
				}
				if !found {
					scalar, findErr = d.CoordinateDefault(defaultSkeleton, slot)
					if findErr != nil {
						return DynamicReadEvidence{}, findErr
					}
				}
				answer.Scalars = append(answer.Scalars, scalar)
			}
			batch.Coordinate = append(batch.Coordinate, answer)
		}
		advance, err = d.AdvanceDynamicRead(advance.Plan, batch)
		if err != nil {
			return DynamicReadEvidence{}, err
		}
	}
	return advance.Evidence, nil
}

func dynamicReadRangeBranchProofSlot(d ProductDomain, query DynamicReadQuery, family CoordinateFamily) (CoordinateSlot, bool, error) {
	if family.id != pathEvidenceCoordinateFamilyID || !query.HasRange ||
		query.Range.IndexProofStateKey == "" || query.Range.ArrayProofStateKey == "" {
		return CoordinateSlot{}, false, nil
	}
	index, indexOK := query.KeySpace.InternStateKey(query.Range.IndexProofStateKey)
	array, arrayOK := query.KeySpace.InternStateKey(query.Range.ArrayProofStateKey)
	if !indexOK || !arrayOK {
		return CoordinateSlot{}, false, fmt.Errorf("%w: dynamic-read range address", ErrInvalidLaneFactor)
	}
	slot, err := d.PathBranchProofCoordinateSlot(query.KeySpace, pathevidence.BranchProof{
		Kind: pathevidence.BranchProofIndexInRange, Path: index, Other: array,
	})
	return slot, err == nil, err
}

func resolveDynamicReadOrdinaryEvidence(d ProductDomain, selection DynamicReadSelection, out *dynamicReadBuilder) error {
	if !out.hasFacts || !out.hasMemberships {
		return fmt.Errorf("%w: incomplete registered dynamic-read producers", ErrIncompleteLaneFactors)
	}
	if !selection.validFor(d) {
		return fmt.Errorf("%w: invalid dynamic-read selection", ErrInvalidLaneFactor)
	}
	request := selection.query
	membershipProven := false
	if !out.memberships.bottom {
		for _, key := range request.KeyKeys {
			for _, table := range request.TableKeys {
				if out.memberships.has(PathKeyMembership(key, table)) {
					membershipProven = true
					break
				}
			}
			if membershipProven {
				break
			}
		}
	}
	out.evidence.KeyMembershipProven = membershipProven
	if out.facts.top || len(selection.tables) == 0 {
		return nil
	}
	domain := product.Domain(d.reg)
	for factKey, fact := range out.facts.values {
		if !dynamicReadFactRelevant(d, selection, membershipProven, factKey, fact) {
			continue
		}
		if !out.evidence.HasValue {
			out.evidence.Value, out.evidence.HasValue = fact.Value, true
		} else {
			out.evidence.Value = domain.Join(out.evidence.Value, fact.Value)
		}
	}
	return nil
}

func dynamicReadHeapCoordinates() coordinateDynamicReadPolicy {
	return dynamicReadCoordinateParticipant(
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			if id, ok := product.Get(planner.domain.reg, planner.plan.query.TableValue, identity.Key).ID(); ok {
				return planner.addHeapSlot(binding, heapCoordinateKey{kind: heapCoordinateRoot, id: identity.ConcreteTerm(id)})
			}
			return nil
		},
		advanceDynamicReadHeap,
	)
}

func dynamicReadPathCoordinates() coordinateDynamicReadPolicy {
	return dynamicReadCoordinateParticipant(
		func(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
			query := planner.plan.query
			if query.HasRange && query.Range.IndexProofStateKey != "" && query.Range.ArrayProofStateKey != "" {
				index, indexOK := query.KeySpace.InternStateKey(query.Range.IndexProofStateKey)
				array, arrayOK := query.KeySpace.InternStateKey(query.Range.ArrayProofStateKey)
				if !indexOK || !arrayOK {
					return fmt.Errorf("%w: dynamic-read range address", ErrInvalidLaneFactor)
				}
				slot, err := planner.domain.PathBranchProofCoordinateSlot(query.KeySpace, pathevidence.BranchProof{
					Kind: pathevidence.BranchProofIndexInRange, Path: index, Other: array,
				})
				if err != nil {
					return err
				}
				if err := planner.addSlot(binding, slot); err != nil {
					return err
				}
			}
			for _, path := range []keyspace.Key{query.OwnerPath, query.TablePath} {
				if path.Kind != keyspace.KindInvalid {
					slot, err := planner.domain.PresenceImplicationRefinementCoordinateSlot(query.KeySpace, path)
					if err != nil {
						return err
					}
					if err := planner.addSlot(binding, slot); err != nil {
						return err
					}
				}
			}
			for _, candidate := range planner.plan.selection.PathMembers() {
				slot, err := planner.domain.PresenceImplicationRefinementCoordinateSlot(query.KeySpace, candidate)
				if err != nil {
					return err
				}
				if err := planner.addSlot(binding, slot); err != nil {
					return err
				}
			}
			return nil
		},
		advanceDynamicReadPath,
	)
}

func advanceDynamicReadPath(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
	if planner.builder == nil {
		return nil
	}
	present := make([]CoordinateScalarFactor, 0, len(binding.scalars))
	for _, scalar := range binding.scalars {
		isDefault, err := planner.domain.CoordinateScalarIsOmitted(binding.skeleton, scalar)
		if err != nil {
			return err
		}
		if !isDefault {
			present = append(present, scalar)
		}
	}
	carrier, err := planner.domain.OpenCoordinatePathEvidenceCarrier(
		binding.skeleton, present, ValueLaneFactor{}, true, binding.pathAuthority, PathDescendantMutationFactors{},
	)
	if err != nil {
		return err
	}
	if planner.builder.evidence.pathValues == nil {
		planner.builder.evidence.pathValues = make(map[keyspace.Key]product.Value)
	}
	for _, slot := range binding.slots {
		_ = slot
	}
	query := planner.plan.query
	if query.HasRange && query.Range.IndexProofStateKey != "" && query.Range.ArrayProofStateKey != "" {
		index, indexOK := query.KeySpace.InternStateKey(query.Range.IndexProofStateKey)
		array, arrayOK := query.KeySpace.InternStateKey(query.Range.ArrayProofStateKey)
		if indexOK && arrayOK {
			planner.builder.evidence.rangeBranchProof = carrier.HasProof(pathevidence.BranchProof{
				Kind: pathevidence.BranchProofIndexInRange, Path: index, Other: array,
			})
		}
	}
	paths := []keyspace.Key{query.OwnerPath, query.TablePath}
	paths = append(paths, planner.plan.selection.PathMembers()...)
	for _, path := range paths {
		if path.Kind == keyspace.KindInvalid {
			continue
		}
		if value, ok := carrier.ReadPath(path); ok && !product.Equal(planner.domain.reg, value, product.Bottom(planner.domain.reg)) {
			planner.builder.evidence.pathValues[path] = value
		}
	}
	return nil
}

func finalizeDynamicReadRangeProof(query DynamicReadQuery, evidence *DynamicReadEvidence) {
	if evidence == nil {
		return
	}
	evidence.rangeProof = false
	if !query.HasRange {
		return
	}
	shape := query.Range.Shape
	lengthAtLeast := func(floor int64) bool {
		if floor <= 0 || evidence.hasRangeArrayFloor && evidence.rangeArrayLenFloor >= floor {
			return true
		}
		if !query.HasRangeContainer || query.TypeValues == nil || !evidence.domain.Valid() {
			return false
		}
		containerType, ok := query.TypeValues.TypeOf(evidence.domain.reg, query.RangeContainer)
		return ok && indexproof.SequenceLengthKnownAtLeast(containerType, floor)
	}
	switch shape.Kind() {
	case indexform.IndexFormArrayLength:
		evidence.rangeProof = lengthAtLeast(1)
	case indexform.IndexFormModuloLength:
		evidence.rangeProof = query.Range.ModuloInteger && lengthAtLeast(1)
	case indexform.IndexFormConstant:
		constant, _ := shape.Constant()
		evidence.rangeProof = constant >= 1 && lengthAtLeast(constant)
	case indexform.IndexFormAffine:
		coeff, offset, ok := shape.Affine()
		if !ok {
			return
		}
		if coeff == 1 && offset == 0 && evidence.rangeBranchProof {
			evidence.rangeProof = true
			return
		}
		if !evidence.hasRangeIndexFloor {
			return
		}
		minimum, ok := indexform.CheckedMulInt64(coeff, evidence.rangeIndexFloor)
		if !ok {
			return
		}
		minimum, ok = indexform.CheckedAddInt64(minimum, offset)
		if !ok || minimum < 1 {
			return
		}
		if evidence.rangeDiffProof {
			evidence.rangeProof = true
			return
		}
		if evidence.hasRangeIndexCeil {
			maximum, maximumOK := indexform.CheckedMulInt64(coeff, evidence.rangeIndexCeil)
			if maximumOK {
				maximum, maximumOK = indexform.CheckedAddInt64(maximum, offset)
			}
			if maximumOK && maximum >= 1 && lengthAtLeast(maximum) {
				evidence.rangeProof = true
				return
			}
		}
	}
}

func advanceDynamicReadHeap(planner *dynamicReadPlanner, binding *dynamicReadCoordinateBinding) error {
	query := planner.plan.query
	skeleton := heapCoordinateSkeletonValue(binding.skeleton.payload)
	if skeleton.top {
		return nil
	}
	table := query.TableValue
	pathProjected := false
	if query.ProjectPath && planner.builder != nil && query.TablePath.Kind != keyspace.KindInvalid {
		if value, ok := planner.builder.evidence.pathValues[query.TablePath]; ok &&
			!product.Equal(planner.domain.reg, value, product.Bottom(planner.domain.reg)) {
			table, pathProjected = value, true
		}
	}
	projected := 0
	if query.ProjectPath && !pathProjected && query.TablePath.Kind != keyspace.KindInvalid {
		segments, ok := query.KeySpace.SegmentsView(query.TablePath)
		if ok {
			for _, suffix := range segments {
				id, identified := product.Get(planner.domain.reg, table, identity.Key).ID()
				term := identity.ConcreteTerm(id)
				object, exists := skeleton.objects[term]
				if !identified || !exists || object.bottom {
					break
				}
				root, rootOK, err := planner.heapScalar(binding, heapCoordinateKey{kind: heapCoordinateRoot, id: term})
				if err != nil {
					return err
				}
				if !rootOK {
					return planner.addHeapSlot(binding, heapCoordinateKey{kind: heapCoordinateRoot, id: term})
				}
				rootID, rootIdentified := product.Get(planner.domain.reg, root, identity.Key).ID()
				if !rootIdentified || rootID != id || product.Equal(planner.domain.reg, product.Meet(planner.domain.reg, root, table), product.Bottom(planner.domain.reg)) {
					break
				}
				memberKey, memberOK := query.KeySpace.FromRootlessSuffix([]segment.Segment{suffix})
				if !memberOK || !sortedHeapKeyContains(query.KeySpace, object.staticKeys, memberKey) {
					break
				}
				member, memberPresent, memberErr := planner.heapScalar(binding, heapCoordinateKey{kind: heapCoordinateMember, id: term, key: memberKey})
				if memberErr != nil {
					return memberErr
				}
				if !memberPresent {
					return planner.addHeapSlot(binding, heapCoordinateKey{kind: heapCoordinateMember, id: term, key: memberKey})
				}
				table = member
				projected++
			}
		}
	}
	if planner.builder != nil && !pathProjected && projected != 0 {
		planner.builder.evidence.ProjectedTable = table
		planner.builder.evidence.ProjectedSegments = projected
	}
	id, identified := product.Get(planner.domain.reg, table, identity.Key).ID()
	term := identity.ConcreteTerm(id)
	object, exists := skeleton.objects[term]
	if !identified || !exists || object.bottom {
		return nil
	}
	if _, ok, err := planner.heapScalar(binding, heapCoordinateKey{kind: heapCoordinateRoot, id: term}); err != nil {
		return err
	} else if !ok {
		return planner.addHeapSlot(binding, heapCoordinateKey{kind: heapCoordinateRoot, id: term})
	}
	memberKeys := planner.plan.selection.HeapMembers(object.staticKeys)
	addedMemberDemand := false
	for _, member := range memberKeys {
		if _, ok, err := planner.heapScalar(binding, heapCoordinateKey{kind: heapCoordinateMember, id: term, key: member}); err != nil {
			return err
		} else if !ok {
			if err := planner.addHeapSlot(binding, heapCoordinateKey{kind: heapCoordinateMember, id: term, key: member}); err != nil {
				return err
			}
			addedMemberDemand = true
		}
	}
	if addedMemberDemand {
		return nil
	}
	if planner.builder == nil {
		return nil
	}
	root, _, _ := planner.heapScalar(binding, heapCoordinateKey{kind: heapCoordinateRoot, id: term})
	members := make(map[keyspace.Key]product.Value, len(memberKeys))
	for _, key := range memberKeys {
		value, ok, _ := planner.heapScalar(binding, heapCoordinateKey{kind: heapCoordinateMember, id: term, key: key})
		if ok {
			members[key] = value
		}
	}
	if object.dynamicIndexFactsTop {
		return nil
	}
	planner.builder.evidence.HeapObject = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: root, StaticMembers: members, DynamicIndexFacts: object.dynamicIndexFacts,
		StableShape: object.stableShape, PrefixStableShape: object.prefixStableShape,
	})
	planner.builder.evidence.HasHeapObject = true
	return nil
}

func (p *dynamicReadPlanner) addHeapSlot(binding *dynamicReadCoordinateBinding, key heapCoordinateKey) error {
	return p.addSlot(binding, CoordinateSlot{family: binding.family, keys: p.plan.query.KeySpace, key: wrapHeapCoordinateKey(key)})
}

func (p *dynamicReadPlanner) addSlot(binding *dynamicReadCoordinateBinding, slot CoordinateSlot) error {
	coordinate, err := p.domain.validateCoordinateFamily(binding.family)
	if err != nil || p.domain.validateCoordinateSlotFor(coordinate, slot, p.plan.query.KeySpace) != nil {
		return fmt.Errorf("%w: dynamic-read coordinate slot", ErrInvalidLaneFactor)
	}
	for _, existing := range binding.slots {
		equal, equalErr := p.domain.CoordinateSlotEqual(existing, slot)
		if equalErr != nil {
			return equalErr
		}
		if equal {
			return nil
		}
	}
	binding.slots = append(binding.slots, slot)
	sort.SliceStable(binding.slots, func(i, j int) bool {
		less, _ := p.domain.CoordinateSlotLess(binding.slots[i], binding.slots[j])
		return less
	})
	return nil
}

func (p *dynamicReadPlanner) heapScalar(binding *dynamicReadCoordinateBinding, key heapCoordinateKey) (product.Value, bool, error) {
	slot := CoordinateSlot{family: binding.family, keys: p.plan.query.KeySpace, key: wrapHeapCoordinateKey(key)}
	for _, scalar := range binding.scalars {
		equal, err := p.domain.CoordinateSlotEqual(scalar.slot, slot)
		if err != nil {
			return product.Value{}, false, err
		}
		if equal {
			return heapCoordinateScalarValue(scalar.payload).value, true, nil
		}
	}
	return product.Value{}, false, nil
}

func dynamicReadCanonicalPaths(keys *keyspace.KeySpace, path keyspace.Key) []keyspace.Key {
	out := []keyspace.Key{path}
	if canonical, ok := keys.FieldCanonical(path); ok && canonical != path {
		out = append(out, canonical)
	}
	return out
}

func dynamicReadBinding(plan *DynamicReadQueryPlan, family CoordinateFamily) *dynamicReadCoordinateBinding {
	for i := range plan.coordinate {
		if plan.coordinate[i].family == family {
			return &plan.coordinate[i]
		}
	}
	return nil
}

func dynamicReadHasScalar(d ProductDomain, binding *dynamicReadCoordinateBinding, slot CoordinateSlot) bool {
	for _, scalar := range binding.scalars {
		equal, err := d.CoordinateSlotEqual(scalar.slot, slot)
		if err == nil && equal {
			return true
		}
	}
	return false
}

func findDynamicReadScalar(d ProductDomain, scalars []CoordinateScalarFactor, slot CoordinateSlot) (CoordinateScalarFactor, bool, error) {
	for _, scalar := range scalars {
		equal, err := d.CoordinateSlotEqual(scalar.slot, slot)
		if err != nil {
			return CoordinateScalarFactor{}, false, err
		}
		if equal {
			return scalar, true, nil
		}
	}
	return CoordinateScalarFactor{}, false, nil
}

func dynamicReadSlotCount(plan DynamicReadQueryPlan) int {
	count := 0
	for i := range plan.coordinate {
		count += len(plan.coordinate[i].slots)
	}
	return count
}

func cloneDynamicReadPlan(plan DynamicReadQueryPlan) DynamicReadQueryPlan {
	out := plan
	out.query.TableKeys = append([]pathaddr.StateKey(nil), plan.query.TableKeys...)
	out.query.KeyKeys = append([]pathaddr.StateKey(nil), plan.query.KeyKeys...)
	out.ordinary = append([]dynamicReadOrdinaryBinding(nil), plan.ordinary...)
	out.coordinate = make([]dynamicReadCoordinateBinding, len(plan.coordinate))
	for i, binding := range plan.coordinate {
		out.coordinate[i] = binding
		out.coordinate[i].slots = append([]CoordinateSlot(nil), binding.slots...)
		out.coordinate[i].scalars = append([]CoordinateScalarFactor(nil), binding.scalars...)
	}
	out.evidence = cloneDynamicReadEvidence(plan.evidence)
	return out
}

func cloneDynamicReadEvidence(e DynamicReadEvidence) DynamicReadEvidence {
	out := e
	out.rangeRelations = append([]RelConstraint(nil), e.rangeRelations...)
	if e.rangeNumFloors != nil {
		out.rangeNumFloors = make(map[pathaddr.StateKey]int64, len(e.rangeNumFloors))
		for key, floor := range e.rangeNumFloors {
			out.rangeNumFloors[key] = floor
		}
	}
	if e.pathValues != nil {
		out.pathValues = make(map[keyspace.Key]product.Value, len(e.pathValues))
		for key, value := range e.pathValues {
			out.pathValues[key] = value
		}
	}
	return out
}

func dynamicReadEvidenceEqual(reg *axis.Registry, left, right DynamicReadEvidence) bool {
	if left.HasValue != right.HasValue || left.KeyMembershipProven != right.KeyMembershipProven ||
		left.HasHeapObject != right.HasHeapObject || left.ProjectedSegments != right.ProjectedSegments ||
		left.rangeProof != right.rangeProof || left.rangeBranchProof != right.rangeBranchProof ||
		left.rangeDiffProof != right.rangeDiffProof || len(left.rangeRelations) != len(right.rangeRelations) ||
		len(left.rangeNumFloors) != len(right.rangeNumFloors) ||
		left.hasRangeIndexFloor != right.hasRangeIndexFloor || left.rangeIndexFloor != right.rangeIndexFloor ||
		left.hasRangeIndexCeil != right.hasRangeIndexCeil || left.rangeIndexCeil != right.rangeIndexCeil ||
		left.hasRangeArrayFloor != right.hasRangeArrayFloor || left.rangeArrayLenFloor != right.rangeArrayLenFloor ||
		left.HasValue && !product.Equal(reg, left.Value, right.Value) ||
		left.ProjectedSegments != 0 && !product.Equal(reg, left.ProjectedTable, right.ProjectedTable) ||
		len(left.pathValues) != len(right.pathValues) {
		return false
	}
	for index := range left.rangeRelations {
		if left.rangeRelations[index] != right.rangeRelations[index] {
			return false
		}
	}
	for key, floor := range left.rangeNumFloors {
		if other, ok := right.rangeNumFloors[key]; !ok || other != floor {
			return false
		}
	}
	for key, value := range left.pathValues {
		other, ok := right.pathValues[key]
		if !ok || !product.Equal(reg, value, other) {
			return false
		}
	}
	if left.HasHeapObject {
		return heapidentity.ObjectDomain(reg).Equal(left.HeapObject, right.HeapObject)
	}
	return true
}
