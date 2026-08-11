package engine

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// QueryInstance is one cold, one-shot binding of a declared Query to its
// complete typed observation surface.
type QueryInstance[R any] struct {
	query   *Query[R]
	declare func(*QueryBinding[R]) bool
	state   *queryInstanceState
	receipt *queryReceipt
}

type queryInstanceState struct{ used atomic.Bool }

// QueryReceipt is an opaque exact observation receipt. It is issued only by a
// successfully attached QueryInstance and becomes usable only after its
// owning Assembly has compiled a Solver revision. The equation query key,
// point, and result slot never cross this boundary.
type QueryReceipt[R any] struct {
	value *queryReceipt
	query *Query[R]
}

type queryReceipt struct {
	assembly  *Assembly
	solver    *Solver
	authority *queryAuthority
	key       composition.Key
	slot      int
}

func newQueryReceipt(assembly *Assembly, schema *querySchema) *queryReceipt {
	if assembly == nil || schema == nil {
		return nil
	}
	return &queryReceipt{assembly: assembly, authority: schema.authority, slot: -1}
}

// Receipt returns the exact structural observation issued by this one-shot
// QueryInstance.  It fails closed until the owning Assembly has compiled.
func (instance *QueryInstance[R]) Receipt() (QueryReceipt[R], bool) {
	if instance == nil || !validQueryReceipt(instance.receipt) || instance.query == nil || instance.receipt.authority == nil || instance.receipt.authority.schema != instance.query.schema {
		return QueryReceipt[R]{}, false
	}
	return QueryReceipt[R]{value: instance.receipt, query: instance.query}, true
}

// Available reports whether this receipt has been bound to a completed
// Assembly/Solver revision.  It exposes no equation identity or dense slot.
func (receipt QueryReceipt[R]) Available() bool {
	return validQueryReceipt(receipt.value) && receipt.query != nil && receipt.value.authority != nil && receipt.value.authority.schema == receipt.query.schema
}

func validQueryReceipt(receipt *queryReceipt) bool {
	if receipt == nil || receipt.assembly == nil || receipt.solver == nil || receipt.solver.assembly != receipt.assembly || receipt.solver.runtime == nil ||
		receipt.authority == nil || !validQueryAuthority(receipt.authority) || !receipt.key.Available() || receipt.slot < 0 || receipt.slot >= len(receipt.solver.runtime.queries) {
		return false
	}
	runtime := receipt.solver.runtime
	if runtime.graph == nil || receipt.slot >= runtime.graph.QueryCount() {
		return false
	}
	identity, ok := runtime.graph.QueryAt(receipt.slot)
	if !ok || !identity.Key().Available() || identity.Key() != receipt.key || !identity.Family().Available() || receipt.authority.schema.semantic.compositionKey() != identity.Family() {
		return false
	}
	row := runtime.queries[receipt.slot]
	if row == nil {
		return false
	}
	rowIdentity := row.query()
	return rowIdentity.Key().Available() && rowIdentity.Key() == receipt.key && rowIdentity.Family() == identity.Family() && row.queryAuthority() == receipt.authority
}

// queryReceiptBinding is the disposable join scratch for one equation query
// row and one Assembly observation.  Both sides are sorted by the complete
// observation tuple before they are merged, so binding is O(Q log Q) rather
// than a quadratic scan through every observation for every graph slot.
type queryReceiptBinding struct {
	identity    equation.Query
	observation *assemblyObservation
	slot        int
	family      composition.Key
	pointSite   equation.Site
	scope       composition.Key
	surfaces    []equation.Surface
}

func bindAssemblyQueryReceipts(assembly *Assembly, solver *Solver) bool {
	if assembly == nil || solver == nil || solver.runtime == nil || solver.runtime.graph == nil || len(assembly.observations) != solver.runtime.graph.QueryCount() {
		return false
	}
	count := solver.runtime.graph.QueryCount()
	graphRows := make([]queryReceiptBinding, count)
	for slot := 0; slot < count; slot++ {
		identity, ok := solver.runtime.graph.QueryAt(slot)
		if !ok || !identity.Key().Available() || !identity.Family().Available() || !identity.Point().Available() || !identity.Point().Site().Available() {
			return false
		}
		point := identity.Point()
		graphRows[slot] = queryReceiptBinding{
			identity:  identity,
			slot:      slot,
			family:    identity.Family(),
			pointSite: point.Site(),
			scope:     point.Scope().Key(),
			surfaces:  identity.Surfaces(),
		}
	}
	observations := make([]queryReceiptBinding, len(assembly.observations))
	for index, observation := range assembly.observations {
		if observation == nil || observation.point == nil || observation.query == nil || !observation.point.site.Available() || !observation.query.semantic.Available() {
			return false
		}
		observations[index] = queryReceiptBinding{
			observation: observation,
			slot:        index,
			family:      observation.query.semantic.compositionKey(),
			pointSite:   observation.point.site,
			scope:       observation.point.site.Scope().Key(),
			surfaces:    observation.surfaces,
		}
	}
	sort.Slice(graphRows, func(left, right int) bool { return compareQueryReceiptBinding(graphRows[left], graphRows[right]) < 0 })
	sort.Slice(observations, func(left, right int) bool {
		return compareQueryReceiptBinding(observations[left], observations[right]) < 0
	})
	for index := range graphRows {
		graphRow, observationRow := graphRows[index], observations[index]
		if compareQueryReceiptBinding(graphRow, observationRow) != 0 || !sameAssemblyObservation(graphRow, observationRow.observation) {
			return false
		}
		observation := observationRow.observation
		if observation.receipt == nil || observation.receipt.assembly != assembly || observation.receipt.authority == nil || observation.receipt.authority.schema != observation.query {
			return false
		}
		observation.receipt.solver = solver
		observation.receipt.key = graphRow.identity.Key()
		observation.receipt.slot = graphRow.slot
	}
	return true
}

func sameAssemblyObservation(binding queryReceiptBinding, observation *assemblyObservation) bool {
	identity := binding.identity
	if !identity.Key().Available() || observation == nil || observation.point == nil || observation.query == nil ||
		identity.Family() != observation.query.semantic.compositionKey() || !identity.Point().Available() || !identity.Point().Site().Same(observation.point.site) || len(binding.surfaces) != len(observation.surfaces) {
		return false
	}
	for index, surface := range binding.surfaces {
		if surface != observation.surfaces[index] {
			return false
		}
	}
	return true
}

func compareQueryReceiptBinding(left, right queryReceiptBinding) int {
	if comparison := compareQueryReceiptKey(left.family, right.family); comparison != 0 {
		return comparison
	}
	if comparison := compareQueryReceiptKey(left.pointSite.Key(), right.pointSite.Key()); comparison != 0 {
		return comparison
	}
	if comparison := compareQueryReceiptKey(left.scope, right.scope); comparison != 0 {
		return comparison
	}
	leftSurfaces, rightSurfaces := left.surfaces, right.surfaces
	for index := 0; index < len(leftSurfaces) && index < len(rightSurfaces); index++ {
		if comparison := compareQueryReceiptSurface(leftSurfaces[index], rightSurfaces[index]); comparison != 0 {
			return comparison
		}
	}
	if len(leftSurfaces) < len(rightSurfaces) {
		return -1
	}
	if len(leftSurfaces) > len(rightSurfaces) {
		return 1
	}
	return 0
}

func compareQueryReceiptKey(left, right composition.Key) int {
	for index := range left.ID {
		if left.ID[index] < right.ID[index] {
			return -1
		}
		if left.ID[index] > right.ID[index] {
			return 1
		}
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}

func compareQueryReceiptSurface(left, right equation.Surface) int {
	if comparison := compareQueryReceiptKey(left.Factor, right.Factor); comparison != 0 {
		return comparison
	}
	if left.Form < right.Form {
		return -1
	}
	if left.Form > right.Form {
		return 1
	}
	if left.Local < right.Local {
		return -1
	}
	if left.Local > right.Local {
		return 1
	}
	if comparison := compareQueryReceiptKey(left.Semantic, right.Semantic); comparison != 0 {
		return comparison
	}
	if comparison := compareQueryReceiptKey(left.Normalizer, right.Normalizer); comparison != 0 {
		return comparison
	}
	if left.Mode < right.Mode {
		return -1
	}
	if left.Mode > right.Mode {
		return 1
	}
	return 0
}

type QueryBinding[R any] struct {
	assembly    *Assembly
	observation *assemblyObservation
	query       *Query[R]
	gate        *coldGate
}

func NewQueryInstance[R any](query *Query[R], declare func(*QueryBinding[R]) bool) (*QueryInstance[R], bool) {
	if query == nil || query.schema == nil || query.composition == nil || !query.composition.Sealed() || !query.schema.bound || declare == nil {
		return nil, false
	}
	return &QueryInstance[R]{query: query, declare: declare, state: &queryInstanceState{}}, true
}

func InstanceQueryRead[R, S any, K ~uint32 | ~uint64](binding *QueryBinding[R], read QueryRead[S], ref Ref[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validQueryBinding(binding) || read.schema != binding.observation.query || read.index != binding.observation.at || read.resolve == nil {
		failQueryBinding(binding)
		return false
	}
	return queryExact(binding.assembly, binding.observation, ref)
}

func InstanceQuerySummaryRead[R, S, V any, K ~uint32 | ~uint64](binding *QueryBinding[R], read QueryRead[S], form ReadForm[V, S], refs *ClosedRefs[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validQueryBinding(binding) || read.schema != binding.observation.query || read.index != binding.observation.at || read.resolve == nil {
		failQueryBinding(binding)
		return false
	}
	summary := admitSummary(binding.assembly, form, refs)
	if summary == nil {
		return false
	}
	return querySummaryRead(binding.assembly, binding.observation, summary)
}

func validQueryBinding[R any](binding *QueryBinding[R]) bool {
	return binding != nil && binding.gate != nil && binding.assembly != nil && binding.observation != nil && binding.query != nil &&
		binding.observation.assembly == binding.assembly && binding.observation.query == binding.query.schema && validAssembly(binding.assembly)
}

func failQueryBinding[R any](binding *QueryBinding[R]) {
	if binding != nil {
		failAssembly(binding.assembly)
	}
}
