// Package query owns Call's observation-only exact query.
//
// The family reads the already-issued Call Factor at one exact point and
// projects its one output cell into dispatch.CalleeSet. It is deliberately an
// observation population: construction must attach it only where a caller
// explicitly asks for the observation, never enumerate it as a selected-point
// result family.
package query

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

// CalleeSetResultFamily is the one spelling of Call's exact observation
// family. Its payload codec is intentionally not implemented in this slice;
// the query is first sealed as a typed producer and is not admitted to the
// composite result table until the central result-plane owner is ready.
const CalleeSetResultFamily schema.Key = "call-callee-set"

const (
	querySemantic = "semantic/query/call-callee-set"
	queryCodec    = "semantic/query-result/call-callee-set"
	queryContract = "semantic/fold-contract/call-callee-set"
)

// StructureSpecs contributes only Call's authored semantic roles. Generic
// population and projection roles belong to analysis/schema/query and are
// contributed by the schema composition root, not copied into this domain.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("query/call-callee-set", "query-result/call-callee-set", "fold-contract/call-callee-set")
}

// ExactQueryImplementation is the sealed implementation of Call's exact
// observation producer.
type ExactQueryImplementation = engine.ExactQueryImplementation[calldomain.Value, calldispatch.CalleeSet]

// ExactQueryFragment is the cold query declaration: one exact Call read and
// one result slot. It carries no runtime algebra or result bytes.
type ExactQueryFragment struct {
	slot    *engine.QuerySlot[calldispatch.CalleeSet]
	read    engine.SchemaReadForm[calldomain.Value]
	freezer identity.SemanticKey
}

func (fragment *ExactQueryFragment) Available() bool {
	return fragment != nil && fragment.slot != nil && fragment.freezer.Available()
}

// QuerySpec declares the family as an exact, observation-only Call query.
// FoldGeneral is required because the projection is a single exact Call cell;
// it cannot be reconstructed from independently folded fragments.
func QuerySpec() queryschema.Spec {
	return queryschema.Spec{
		Family:     CalleeSetResultFamily,
		Semantic:   querySemantic,
		Codec:      queryCodec,
		Fold:       queryschema.FoldGeneral,
		Contract:   queryContract,
		Subjects:   []schema.Key{"call"},
		Population: queryschema.PopulationObservation,
		Projection: queryschema.ProjectionExact,
	}
}

// DeclareQuery opens Call's exact observation slot from the Call owner's
// already-declared exact read. No Call vector or summary form is introduced.
func DeclareQuery(builder *engine.SchemaBuilder, context queryschema.Declaration) (*ExactQueryFragment, bool) {
	cell, cellOK := context.Subjects.At("call")
	declared, declaredOK := axis.Payload[*callowner.SchemaFragment](cell)
	if !cellOK || !declaredOK {
		return nil, false
	}
	read := declared.ExactRead()
	if read.Schema() != nil {
		return nil, false
	}
	slot, slotOK := engine.NewQuerySlot[calldispatch.CalleeSet](builder, engine.SchemaQuerySpec{Semantic: context.Semantic, Freezer: context.Freezer})
	if !slotOK || !engine.SchemaQueryRead(slot, read) {
		return nil, false
	}
	fragment := &ExactQueryFragment{slot: slot, read: read, freezer: context.Freezer}
	return fragment, fragment.Available()
}

// BindQuery installs Call's exact observation fold on the owner that issued
// the Call Factor. The owner method keeps the private Factor slot behind the
// Call boundary.
func BindQuery(binding *engine.SchemaBinding, context queryschema.Binding[*ExactQueryFragment]) bool {
	cell, cellOK := context.Subjects.At("call")
	owner, ownerOK := axis.Payload[*callowner.HotOwner](cell)
	if !cellOK || !ownerOK || !context.Fragment.Available() || binding == nil || !owner.MatchesBinding(binding) || context.Fragment.read.Schema() == nil {
		return false
	}
	return callowner.BindExactQuery(owner, context.Fragment.slot, exactQuerySpec(context.Fragment.freezer))
}

// RecoverQuery recovers the sealed exact observation implementation. The
// implementation is still typed; no erased result or codec is created here.
func RecoverQuery(binding *engine.SchemaBinding, context queryschema.Sealed[*ExactQueryFragment]) (*ExactQueryImplementation, bool) {
	if !context.Fragment.Available() {
		return nil, false
	}
	return engine.ExactQueryImplementationAt[calldomain.Value, calldispatch.CalleeSet](binding, context.Fragment.slot)
}

// exactQuerySpec projects exactly one Call output cell. An unwritten cell or
// Call Bottom is absence; no zero-cardinality CalleeSet is fabricated. Any
// other value is classified only through dispatch's owner-issued classifier.
func exactQuerySpec(freezer identity.SemanticKey) engine.HotExactQuerySpec[calldomain.Value, calldispatch.CalleeSet] {
	return engine.HotExactQuerySpec[calldomain.Value, calldispatch.CalleeSet]{
		Fold: engine.QueryFold[engine.OrderedCells[calldomain.Value], calldispatch.CalleeSet]{
			Begin:          func() calldispatch.CalleeSet { return calldispatch.CalleeSet{} },
			BorrowIssued:   true,
			TransferResult: true,
			Accumulate: func(_ calldispatch.CalleeSet, cells engine.OrderedCells[calldomain.Value]) (calldispatch.CalleeSet, bool) {
				if cells.Count() != 1 {
					return calldispatch.CalleeSet{}, false
				}
				value, present, available := cells.At(0)
				if !available {
					return calldispatch.CalleeSet{}, false
				}
				if !present || value.IsEmpty() {
					return calldispatch.CalleeSet{}, true
				}
				return calldispatch.ClassifyCalleeSet(value)
			},
		},
		Result: engine.FrozenResult[calldispatch.CalleeSet]{
			Semantic:    freezer,
			Freeze:      func(value calldispatch.CalleeSet) calldispatch.CalleeSet { return value },
			Clone:       func(value calldispatch.CalleeSet) calldispatch.CalleeSet { return value },
			Equal:       equalCalleeSet,
			Fingerprint: fingerprintCalleeSet,
			Present:     func(value calldispatch.CalleeSet) bool { return value.Available() },
		},
	}
}

func equalCalleeSet(left, right calldispatch.CalleeSet) bool {
	if left.Available() != right.Available() {
		return false
	}
	if !left.Available() {
		return true
	}
	if left.Completeness() != right.Completeness() {
		return false
	}
	leftCardinality, leftFinite := left.Cardinality()
	rightCardinality, rightFinite := right.Cardinality()
	return leftFinite == rightFinite && leftCardinality == rightCardinality
}

func fingerprintCalleeSet(value calldispatch.CalleeSet) uint64 {
	if !value.Available() {
		return 0
	}
	cardinality, _ := value.Cardinality()
	return uint64(value.Completeness())<<32 | uint64(cardinality)
}
