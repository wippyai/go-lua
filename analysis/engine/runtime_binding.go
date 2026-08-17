// runtime_binding.go declares the read form kinds, the runtimeBinding container, its catalog freeze and the prepared composition.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
)

type readFormKind uint8

const (
	exactReadForm readFormKind = iota + 1
	summaryReadForm
)

// runtimeBinding is the one private binding cut from the equation graph's closed
// decision catalog to the carrier's dense guard universe.  It is deliberately
// graph-owned: a caller cannot choose atoms, their order, or a second Manager
// for a set of Factors.
type runtimeBinding struct {
	schema    *Schema
	state     *schemaBindingState
	authority *schemaBindingAuthority
	mode      runtimeBindingMode
	graph     *equation.Graph
	guards    *guard.Manager
	catalog   *graphBindingCatalog // cold only; sole compiler releases it after binding
	validated bool                 // newRuntimeBinding checked the dense atom catalog once
}

type runtimeBindingMode uint8

const (
	runtimeBindingReceipt runtimeBindingMode = iota + 1
)

// newRuntimeBinding derives dense atoms in the Graph catalog order.
// Atom numbers are implementation-local dense ranks; equation Decisions stay
// the only semantic identity.
func newRuntimeBinding(schema *Schema, graph *equation.Graph) (*runtimeBinding, bool) {
	if schema == nil || !schema.Available() || graph == nil || !graph.OwnsComposition(schema.cold) || graph.CompositionID() != schema.cold.ID() {
		return nil, false
	}
	catalog, catalogOK := buildGraphBindingCatalog(schema, graph)
	if !catalogOK || catalog == nil {
		return nil, false
	}
	atoms := make([]guard.Atom, graph.DecisionCount())
	for index := range atoms {
		decision, ok := graph.DecisionAt(index)
		if !ok || !decision.Available() || index > 0 {
			previous, previousOK := graph.DecisionAt(index - 1)
			if !ok || !decision.Available() || !previousOK || !previous.Available() || decision.Key() == previous.Key() {
				return nil, false
			}
		}
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil || manager == nil {
		return nil, false
	}
	return &runtimeBinding{schema: schema, mode: runtimeBindingReceipt, graph: graph, guards: manager, catalog: catalog, validated: true}, true
}

// newReceiptRuntimeBinding is the pre-fenced constructor for the callback-free
// Factor vertical.  It shares the same graph/catalog/runtime path as the
// constructor publishes the exact sealed SchemaBinding state and
// authority before any Factor implementation can be consumed.  A receipt can
// therefore never claim ownership of an unpinned runtime or mix bindings.
func newReceiptRuntimeBinding(binding *SchemaBinding, graph *equation.Graph) (*runtimeBinding, bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	schema, authority, sealed := state.schema, state.authority, state.phase == schemaBindingSealed && state.authority != nil
	state.mu.Unlock()
	if !sealed {
		return nil, false
	}
	runtime, ok := newRuntimeBinding(schema, graph)
	if !ok || runtime == nil {
		return nil, false
	}
	runtime.mode, runtime.state, runtime.authority = runtimeBindingReceipt, state, authority
	return runtime, true
}

func (binding *runtimeBinding) valid() bool {
	// The Graph and Manager are immutable after successful construction. The
	// complete decision/atom correspondence is proved above once; rewalking it
	// for every Factor would turn cold binding into F×Decision work.
	return binding != nil && binding.validated && binding.mode == runtimeBindingReceipt && binding.schema != nil && binding.schema.Available() && binding.graph != nil && binding.graph.CompositionID() == binding.schema.coldID() && binding.guards != nil && binding.state != nil && binding.authority != nil && binding.state.authority == binding.authority && binding.state.schema == binding.schema && binding.state.phase == schemaBindingSealed
}

func (binding *runtimeBinding) pinReceipt(receipt factorRuntimeReceipt) bool {
	if binding == nil || binding.mode != runtimeBindingReceipt || !receipt.valid() || receipt.schema != binding.schema {
		return false
	}
	return binding.state == receipt.state && binding.authority == receipt.authority
}

// takeFactorUses consumes one typed binder's cold graph partition. It is a
// one-shot compiler operation: a failed or duplicate bind cannot keep a
// second materialization route alive.
func (binding *runtimeBinding) takeFactorUses(key composition.Key) (graphFactorUses, bool) {
	if binding == nil || !binding.valid() || binding.catalog == nil || binding.catalog.factors == nil || !key.Available() {
		return graphFactorUses{}, false
	}
	uses, ok := binding.catalog.factors[key]
	if !ok || uses == nil {
		return graphFactorUses{}, false
	}
	delete(binding.catalog.factors, key)
	return *uses, true
}

// freezeCatalog is the compiler's release cut after every sealed Factor has
// bound. Runtime execution retains only concrete handles in members/queries;
// it must never retain graph lookup maps, vectors, or typed schema pointers.
func (binding *runtimeBinding) freezeCatalog() bool {
	if binding == nil || !binding.valid() || binding.catalog == nil || binding.catalog.factors == nil || len(binding.catalog.factors) != 0 {
		return false
	}
	binding.catalog.factors = nil
	binding.catalog = nil
	return true
}

// prepareRuntimeComposition establishes the one carrier composition for this
// graph.  A factor-free structural graph is the same composition with an
// empty root vector; its guard manager still comes from newRuntimeBinding's
// graph catalog and it proceeds through the ordinary Work, contribution, and
// publication protocol.
func prepareRuntimeComposition(factors []runtimeFactor, guards *guard.Manager) (*carrier.PreparedComposition, []runtimeFactor, bool) {
	if guards == nil {
		return nil, nil, false
	}
	if len(factors) == 0 {
		prepared, ok := carrier.PrepareComposition(nil, guards)
		if !ok || prepared == nil || prepared.Guards() != guards || prepared.Shape() == nil || prepared.Shape().Count() != 0 {
			return nil, nil, false
		}
		return prepared, nil, true
	}
	ordered := append([]runtimeFactor(nil), factors...)
	sort.Slice(ordered, func(left, right int) bool {
		return identity.CompareSemanticKey(ordered[left].semantic(), ordered[right].semantic()) < 0
	})
	operations := make([]carrier.FactorOperation, len(ordered))
	for index, factor := range ordered {
		if factor == nil || !factor.semantic().Available() || index > 0 && identity.CompareSemanticKey(ordered[index-1].semantic(), factor.semantic()) >= 0 {
			return nil, nil, false
		}
		operations[index] = factor.operation()
		if operations[index] == nil {
			return nil, nil, false
		}
	}
	prepared, ok := carrier.PrepareComposition(operations, guards)
	if !ok || prepared == nil || prepared.Guards() != guards || prepared.Shape() == nil || prepared.Shape().Count() != len(ordered) {
		return nil, nil, false
	}
	for index, factor := range ordered {
		bound, boundOK := factor.(interface{ bindRuntimeSlot(shape.Slot) bool })
		if !boundOK || !prepared.Shape().ValidSlot(shape.Slot(index)) || !bound.bindRuntimeSlot(shape.Slot(index)) {
			return nil, nil, false
		}
	}
	return prepared, ordered, true
}
