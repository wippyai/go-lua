package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Result is the one closed evaluator output ABI. Relation-producing nodes
// carry ordered cofiber Batches; Apply retains its typed semantic results;
// Publish retains both the child Apply extents and its ordered settlements.
// It is intentionally not a second fact store or a generic callback result.
type Result struct {
	dependency   model.DependencyID
	expression   model.ExpressionID
	node         identity.ContentID
	kind         algebra.Kind
	batches      []tuple.Batch
	applications []apply.Results
	settlements  []publish.Settlement
	sealed       bool
}

// nodeValue is the transient value produced while recursively interpreting a
// sealed expression tree. Child nodes do not have dependency identities: the
// dependency schedule identifies only the mounted root work item. Keeping
// that distinction explicit prevents the evaluator from inventing child
// identities or a second result address book.
type nodeValue struct {
	node         identity.ContentID
	kind         algebra.Kind
	batches      []tuple.Batch
	applications []apply.Results
	settlements  []publish.Settlement
}

func relationNode(node identity.ContentID, kind algebra.Kind, batches []tuple.Batch) (nodeValue, bool) {
	if !node.Available() || !relationKind(kind) || batches == nil {
		return nodeValue{}, false
	}
	for _, batch := range batches {
		if !batch.Available() {
			return nodeValue{}, false
		}
	}
	value := nodeValue{node: node, kind: kind, batches: append([]tuple.Batch{}, batches...), applications: []apply.Results{}, settlements: []publish.Settlement{}}
	return value, value.available()
}

// composedRelationNode carries a relation node whose sealed algebra also
// preserves Apply applications. Merge is the vertical alternative boundary:
// an existing carried relation needs no new proposal, while an Apply sibling
// still owns the proposal lease that Publish must redeem. ColumnProject may
// preserve the same sidecar when it keeps the Apply's complete output vector.
func composedRelationNode(node identity.ContentID, kind algebra.Kind, batches []tuple.Batch, applications []apply.Results) (nodeValue, bool) {
	if !node.Available() || !composedRelationKind(kind) || batches == nil || applications == nil {
		return nodeValue{}, false
	}
	for _, batch := range batches {
		if !batch.Available() {
			return nodeValue{}, false
		}
	}
	if !applicationResultsAvailable(applications) {
		return nodeValue{}, false
	}
	value := nodeValue{node: node, kind: kind, batches: append([]tuple.Batch{}, batches...), applications: append([]apply.Results{}, applications...), settlements: []publish.Settlement{}}
	return value, value.available()
}

func applyNode(node identity.ContentID, values []apply.Results) (nodeValue, bool) {
	if !node.Available() || values == nil || !applicationResultsAvailable(values) {
		return nodeValue{}, false
	}
	result := nodeValue{node: node, kind: algebra.KindApply, batches: []tuple.Batch{}, applications: append([]apply.Results{}, values...), settlements: []publish.Settlement{}}
	return result, result.available()
}

func applicationResultsAvailable(values []apply.Results) bool {
	for _, value := range values {
		if !value.Available() {
			return false
		}
	}
	return true
}

func publishNode(node identity.ContentID, values []publish.Settlement, applications []apply.Results) (nodeValue, bool) {
	if !node.Available() || values == nil || applications == nil || !applicationResultsAvailable(applications) {
		return nodeValue{}, false
	}
	for _, value := range values {
		if !value.Available() {
			return nodeValue{}, false
		}
	}
	result := nodeValue{node: node, kind: algebra.KindPublish, batches: []tuple.Batch{}, applications: append([]apply.Results{}, applications...), settlements: append([]publish.Settlement{}, values...)}
	return result, result.available()
}

func (value nodeValue) available() bool {
	if !value.node.Available() || !value.kindAvailable() || value.batches == nil || value.applications == nil || value.settlements == nil {
		return false
	}
	switch value.kind {
	case algebra.KindApply:
		return len(value.batches) == 0 && len(value.settlements) == 0 && applicationResultsAvailable(value.applications)
	case algebra.KindPublish:
		return len(value.batches) == 0 && applicationResultsAvailable(value.applications)
	default:
		return relationKind(value.kind) && (len(value.applications) == 0 || composedRelationKind(value.kind)) && applicationResultsAvailable(value.applications) && len(value.settlements) == 0
	}
}

func (value nodeValue) kindAvailable() bool {
	for _, kind := range algebra.Kinds() {
		if value.kind == kind {
			return true
		}
	}
	return false
}

func sealNodeResult(dependency model.DependencyID, expression model.ExpressionID, value nodeValue) (Result, bool) {
	if !dependency.Available() || !expression.Available() || !value.available() {
		return Result{}, false
	}
	result := Result{dependency: dependency, expression: expression, node: value.node, kind: value.kind, batches: append([]tuple.Batch{}, value.batches...), applications: append([]apply.Results{}, value.applications...), settlements: append([]publish.Settlement{}, value.settlements...), sealed: true}
	return result, result.Available()
}

func relationResult(dependency model.DependencyID, expression model.ExpressionID, node identity.ContentID, kind algebra.Kind, batches []tuple.Batch) (Result, bool) {
	value, ok := relationNode(node, kind, batches)
	if !ok {
		return Result{}, false
	}
	return sealNodeResult(dependency, expression, value)
}

func applyResult(dependency model.DependencyID, expression model.ExpressionID, node identity.ContentID, values []apply.Results) (Result, bool) {
	value, ok := applyNode(node, values)
	if !ok {
		return Result{}, false
	}
	return sealNodeResult(dependency, expression, value)
}

func publishResult(dependency model.DependencyID, expression model.ExpressionID, node identity.ContentID, values []publish.Settlement) (Result, bool) {
	value, ok := publishNode(node, values, []apply.Results{})
	if !ok {
		return Result{}, false
	}
	return sealNodeResult(dependency, expression, value)
}

// Available is O(1): constructors validate nested values once before sealing.
func (result Result) Available() bool {
	if !result.sealed || !result.dependency.Available() || !result.expression.Available() || !result.node.Available() || !result.kindAvailable() || result.batches == nil || result.applications == nil || result.settlements == nil {
		return false
	}
	switch result.kind {
	case algebra.KindApply:
		return len(result.batches) == 0 && len(result.settlements) == 0 && applicationResultsAvailable(result.applications)
	case algebra.KindPublish:
		return len(result.batches) == 0 && applicationResultsAvailable(result.applications)
	default:
		return relationKind(result.kind) && (len(result.applications) == 0 || composedRelationKind(result.kind)) && applicationResultsAvailable(result.applications) && len(result.settlements) == 0
	}
}

func (result Result) kindAvailable() bool {
	for _, kind := range algebra.Kinds() {
		if result.kind == kind {
			return true
		}
	}
	return false
}

func relationKind(kind algebra.Kind) bool {
	switch kind {
	case algebra.KindInput, algebra.KindSelect, algebra.KindProject, algebra.KindColumnProject, algebra.KindJoin, algebra.KindMerge, algebra.KindGroup, algebra.KindComplete, algebra.KindExpand:
		return true
	default:
		return false
	}
}

func composedRelationKind(kind algebra.Kind) bool {
	return kind == algebra.KindColumnProject || kind == algebra.KindMerge
}

func (result Result) Dependency() model.DependencyID {
	if !result.Available() {
		return model.DependencyID{}
	}
	return result.dependency
}

// Expression returns the exact compiler-issued root expression whose sealed
// physical node produced this result. Keeping it beside the dependency stops
// terminal consumers from pairing a valid dependency with a foreign result.
func (result Result) Expression() model.ExpressionID {
	if !result.Available() {
		return model.ExpressionID{}
	}
	return result.expression
}

func (result Result) Node() identity.ContentID {
	if !result.Available() {
		return identity.ContentID{}
	}
	return result.node
}

func (result Result) Kind() algebra.Kind {
	if !result.Available() {
		return algebra.KindInvalid
	}
	return result.kind
}

// Batches returns an ordered defensive copy only for relation-producing
// nodes. An empty slice is a valid no-selection relation result.
func (result Result) Batches() []tuple.Batch {
	if !result.Available() || !relationKind(result.kind) {
		return nil
	}
	values := make([]tuple.Batch, len(result.batches))
	copy(values, result.batches)
	return values
}

// Applications returns the typed result vector produced by Apply, or carried
// unchanged through a sealed ColumnProject/Merge composition on its way to a
// generic Publish boundary.
func (result Result) Applications() []apply.Results {
	if !result.Available() || (result.kind != algebra.KindApply && result.kind != algebra.KindPublish && !composedRelationKind(result.kind)) {
		return nil
	}
	values := make([]apply.Results, len(result.applications))
	copy(values, result.applications)
	return values
}

// Settlements returns one publication settlement per application in order.
func (result Result) Settlements() []publish.Settlement {
	if !result.Available() || result.kind != algebra.KindPublish {
		return nil
	}
	values := make([]publish.Settlement, len(result.settlements))
	copy(values, result.settlements)
	return values
}
