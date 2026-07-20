package factapply

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// AllocationTemplateFresh is one object in a closed allocation graph. ID is
// the exact identity also used by the graph's heap object and placement.
type AllocationTemplateFresh struct {
	ID        identity.ID
	Placement placement.Value
}

// AllocationTemplateTransaction is the immutable callback-free execution
// payload for one EffectAllocationTemplate. The returned value, heap object
// keys, placement keys, and freshness inventory are validated as one closed
// identity graph before the payload can enter the boundary program.
type AllocationTemplateTransaction struct {
	result     product.Value
	objects    map[identity.ID]heapidentity.TableObject
	placements map[identity.ID]placement.Value
	fresh      []AllocationTemplateFresh
	keys       *keyspace.KeySpace
}

// AllocationTemplateMaterialization is the engine-neutral constructor input.
// The producer owns signature/type lowering; factapply owns only validation
// and execution of the resulting closed semantic graph.
type AllocationTemplateMaterialization struct {
	Result     product.Value
	Objects    map[identity.ID]heapidentity.TableObject
	Placements map[identity.ID]placement.Value
	KeySpace   *keyspace.KeySpace
}

// NewAllocationTemplateTransaction freezes one canonical materialization into
// a typed transaction. The materializer's keyspace remains owned by the heap
// objects; execution does no structural rekeying and therefore requires the
// same prepared-body keyspace at construction time.
func NewAllocationTemplateTransaction(reg *axis.Registry, materialized AllocationTemplateMaterialization) (AllocationTemplateTransaction, error) {
	if reg == nil || materialized.KeySpace == nil || !materialized.KeySpace.Valid() ||
		len(materialized.Objects) == 0 || len(materialized.Objects) != len(materialized.Placements) ||
		!product.BelongsToRegistry(reg, materialized.Result) {
		return AllocationTemplateTransaction{}, fmt.Errorf("factapply: allocation transaction is unowned or incomplete")
	}
	root, exact := identityvalue.ExactID(reg, materialized.Result)
	if !exact {
		return AllocationTemplateTransaction{}, fmt.Errorf("factapply: allocation transaction result has no exact identity")
	}
	out := AllocationTemplateTransaction{
		result:     materialized.Result,
		objects:    make(map[identity.ID]heapidentity.TableObject, len(materialized.Objects)),
		placements: make(map[identity.ID]placement.Value, len(materialized.Placements)),
		fresh:      make([]AllocationTemplateFresh, 0, len(materialized.Objects)),
		keys:       materialized.KeySpace,
	}
	objectDomain := heapidentity.ObjectDomain(reg)
	for id, object := range materialized.Objects {
		p, hasPlacement := materialized.Placements[id]
		objectID, hasObjectID := identityvalue.ExactID(reg, object.Root())
		if id == (identity.ID{}) || !hasPlacement || p == placement.Bottom ||
			objectDomain.Equal(object, objectDomain.Bottom()) || !hasObjectID || objectID != id {
			return AllocationTemplateTransaction{}, fmt.Errorf("factapply: allocation transaction identity graph is inconsistent")
		}
		out.objects[id] = heapidentity.CloneObject(object)
		out.placements[id] = p
		out.fresh = append(out.fresh, AllocationTemplateFresh{ID: id, Placement: p})
	}
	if _, ok := out.objects[root]; !ok {
		return AllocationTemplateTransaction{}, fmt.Errorf("factapply: allocation transaction result is not a graph root")
	}
	for id := range materialized.Placements {
		if _, ok := out.objects[id]; !ok {
			return AllocationTemplateTransaction{}, fmt.Errorf("factapply: allocation placement has no heap object")
		}
	}
	sort.Slice(out.fresh, func(i, j int) bool { return allocationTransactionIdentityLess(out.fresh[i].ID, out.fresh[j].ID) })
	return out, nil
}

func (t AllocationTemplateTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil || t.keys == nil || !t.keys.Valid() || len(t.fresh) == 0 || len(t.fresh) != len(t.objects) || len(t.objects) != len(t.placements) ||
		!product.BelongsToRegistry(reg, t.result) {
		return false
	}
	root, exact := identityvalue.ExactID(reg, t.result)
	if !exact {
		return false
	}
	_, rootPresent := t.objects[root]
	return rootPresent
}

func (t AllocationTemplateTransaction) Result() product.Value { return t.result }

func (t AllocationTemplateTransaction) Len() int { return len(t.fresh) }

// Fresh returns one detached freshness record in deterministic identity order.
func (t AllocationTemplateTransaction) Fresh(index int) (AllocationTemplateFresh, bool) {
	if index < 0 || index >= len(t.fresh) {
		return AllocationTemplateFresh{}, false
	}
	return t.fresh[index], true
}

// Object returns a detached heap object for one transaction identity.
func (t AllocationTemplateTransaction) Object(id identity.ID) (heapidentity.TableObject, bool) {
	object, ok := t.objects[id]
	return heapidentity.CloneObject(object), ok
}

// ApplyAllocationTemplateTransaction atomically materializes the heap and
// placement lanes. Each identity is joined with an already-present recursive
// route contribution. Cancellation or malformed input returns the exact input
// state, so no transaction prefix can be published.
func ApplyAllocationTemplateTransaction(ctx context.Context, reg *axis.Registry, transaction AllocationTemplateTransaction, input state.State) (state.State, error) {
	if ctx == nil || reg == nil || !transaction.Valid(reg) {
		return input, fmt.Errorf("factapply: invalid allocation transaction")
	}
	if err := ctx.Err(); err != nil {
		return input, err
	}
	mutations := make([]state.ObjectGraphMutation, len(transaction.fresh))
	for index, fresh := range transaction.fresh {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return input, err
			}
		}
		mutations[index] = state.ObjectGraphMutation{
			Identity: identity.ConcreteTerm(fresh.ID), Object: transaction.objects[fresh.ID], Placement: fresh.Placement,
		}
	}
	domain := state.RegisteredProductDomain(reg)
	plan, err := domain.PrepareObjectGraphJoinPlan(transaction.keys, mutations)
	if err != nil {
		return input, err
	}
	out, err := domain.ApplyObjectGraphMutation(plan, input)
	if err != nil {
		return input, err
	}
	if err := ctx.Err(); err != nil {
		return input, err
	}
	return out, nil
}

func allocationTransactionIdentityLess(left, right identity.ID) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Site != right.Site {
		return left.Site < right.Site
	}
	return left.Index < right.Index
}
