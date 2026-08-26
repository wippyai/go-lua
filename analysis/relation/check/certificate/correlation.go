package certificate

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// CorrelationPartition is one certificate-owned child partition reference.
// Population is the closed Q universe, Child is that child's own Complete
// denominator, and Projection is the owner-issued coordinate used to redeem
// the partition for one Q row. Apply and Ordinal retain the sealed occurrence
// that owns this tuple of authorities; without them two equal-looking child
// ranges from different Applies could be conflated.
type CorrelationPartition struct {
	apply      identity.ContentID
	ordinal    uint32
	population model.DenominatorRef
	child      model.DenominatorRef
	projection model.ColumnID
}

func (partition CorrelationPartition) Available() bool {
	return partition.apply.Available() && partition.population.Available() && partition.child.Available() && partition.projection.Available() && partition.child.Relation() == partition.projection.Relation()
}

// Apply returns the sealed Apply expression identity owning this partition.
func (partition CorrelationPartition) Apply() identity.ContentID { return partition.apply }

// Ordinal returns the declaration-order child ordinal within Apply.
func (partition CorrelationPartition) Ordinal() uint32 { return partition.ordinal }

// Population returns the independent Q denominator.
func (partition CorrelationPartition) Population() model.DenominatorRef { return partition.population }

// Child returns the child's exact Complete denominator.
func (partition CorrelationPartition) Child() model.DenominatorRef { return partition.child }

// Projection returns the child's owner-issued Q coordinate column.
func (partition CorrelationPartition) Projection() model.ColumnID { return partition.projection }

// Digest returns the sealed logical identity of this partition authority.
// The digest is derived solely from the certificate-owned fields; no runtime
// directory or physical arrangement participates in this identity.
func (partition CorrelationPartition) Digest() identity.ContentID {
	if !partition.Available() {
		return identity.ContentID{}
	}
	parts := [][]byte{
		appendCorrelationContent(partition.apply),
		appendCorrelationDenominator(partition.population),
		appendCorrelationDenominator(partition.child),
		appendCorrelationColumn(partition.projection),
		{byte(partition.ordinal >> 24), byte(partition.ordinal >> 16), byte(partition.ordinal >> 8), byte(partition.ordinal)},
	}
	value, _ := identity.DeriveContentID("analysis/relation/check/certificate/correlation-partition/v1", parts...)
	return value
}

// CorrelationDenominators projects the independent Q populations declared by
// checked Apply correlations. The projection is derived from the one sealed
// expression registry; it is not a second relation catalogue and it never
// reconstructs a population from a child range.
//
// A checked certificate cannot contain a malformed correlation. The explicit
// refusal path here is still required so a future registry implementation
// cannot silently turn a malformed/foreign population into an empty runtime
// inventory.
func (certificate Certificate) CorrelationDenominators() []model.DenominatorRef {
	if certificate.registry == nil {
		return nil
	}
	seen := make(map[model.DenominatorRef]struct{})
	result := make([]model.DenominatorRef, 0)
	for _, entry := range certificate.registry.Expressions() {
		if !entry.Available() || entry.Expression() == nil {
			return nil
		}
		if !collectCorrelationDenominators(entry.Expression(), seen, &result) {
			return nil
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return denominatorOrder(result[left], result[right]) < 0
	})
	return result
}

// CorrelationPartitions projects the exact child partition authorities for
// every checked partitioned correlated Apply. A global Complete witness is
// insufficient for a coordinate-projected child: it proves only the union of
// child rows, not the authenticated empty posting for one Q coordinate. A
// sealed shared Complete child is the deliberate exception: its empty
// projection broadcasts one globally authenticated Complete denominator and
// has no partition directory. This projection therefore retains one child
// denominator and projection per partitioned child and never asks runtime to
// infer a partition from a non-empty tuple.
func (certificate Certificate) CorrelationPartitions() []CorrelationPartition {
	if certificate.registry == nil {
		return nil
	}
	seen := make(map[CorrelationPartition]struct{})
	result := make([]CorrelationPartition, 0)
	for _, entry := range certificate.registry.Expressions() {
		if !entry.Available() || entry.Expression() == nil {
			return nil
		}
		if !collectCorrelationPartitions(entry.Expression(), seen, &result) {
			return nil
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].apply != result[right].apply {
			return correlationContentLess(result[left].apply, result[right].apply)
		}
		return result[left].ordinal < result[right].ordinal
	})
	return result
}

func collectCorrelationDenominators(expression algebra.Expression, seen map[model.DenominatorRef]struct{}, result *[]model.DenominatorRef) bool {
	if expression == nil {
		return false
	}
	visit := func(children ...algebra.Expression) bool {
		for _, child := range children {
			if !collectCorrelationDenominators(child, seen, result) {
				return false
			}
		}
		return true
	}
	switch value := expression.(type) {
	case algebra.Input:
		return true
	case *algebra.Input:
		return value != nil
	case algebra.Select:
		return visit(value.Child())
	case *algebra.Select:
		return value != nil && visit(value.Child())
	case algebra.Project:
		return visit(value.Child())
	case *algebra.Project:
		return value != nil && visit(value.Child())
	case algebra.Join:
		return visit(value.Left(), value.Right())
	case *algebra.Join:
		return value != nil && visit(value.Left(), value.Right())
	case algebra.Merge:
		return visit(value.Inputs()...)
	case *algebra.Merge:
		return value != nil && visit(value.Inputs()...)
	case algebra.Group:
		return visit(value.Child())
	case *algebra.Group:
		return value != nil && visit(value.Child())
	case algebra.Complete:
		return visit(value.Child())
	case *algebra.Complete:
		return value != nil && visit(value.Child())
	case algebra.Apply:
		correlation := value.Contract().Correlation()
		if correlation.Specified() {
			if !correlation.Available() || !correlation.Population().Available() {
				return false
			}
			if _, exists := seen[correlation.Population()]; !exists {
				seen[correlation.Population()] = struct{}{}
				*result = append(*result, correlation.Population())
			}
		}
		return visit(value.Inputs()...)
	case *algebra.Apply:
		if value == nil {
			return false
		}
		correlation := value.Contract().Correlation()
		if correlation.Specified() {
			if !correlation.Available() || !correlation.Population().Available() {
				return false
			}
			if _, exists := seen[correlation.Population()]; !exists {
				seen[correlation.Population()] = struct{}{}
				*result = append(*result, correlation.Population())
			}
		}
		return visit(value.Inputs()...)
	case algebra.Publish:
		return visit(value.Child())
	case *algebra.Publish:
		return value != nil && visit(value.Child())
	case algebra.ColumnProject:
		return visit(value.Child())
	case *algebra.ColumnProject:
		return value != nil && visit(value.Child())
	case algebra.Expand:
		return visit(value.Child())
	case *algebra.Expand:
		return value != nil && visit(value.Child())
	default:
		return false
	}
}

func collectCorrelationPartitions(expression algebra.Expression, seen map[CorrelationPartition]struct{}, result *[]CorrelationPartition) bool {
	if expression == nil {
		return false
	}
	visit := func(children ...algebra.Expression) bool {
		for _, child := range children {
			if !collectCorrelationPartitions(child, seen, result) {
				return false
			}
		}
		return true
	}
	switch value := expression.(type) {
	case algebra.Input:
		return true
	case *algebra.Input:
		return value != nil
	case algebra.Select:
		return visit(value.Child())
	case *algebra.Select:
		return value != nil && visit(value.Child())
	case algebra.Project:
		return visit(value.Child())
	case *algebra.Project:
		return value != nil && visit(value.Child())
	case algebra.Join:
		return visit(value.Left(), value.Right())
	case *algebra.Join:
		return value != nil && visit(value.Left(), value.Right())
	case algebra.Merge:
		return visit(value.Inputs()...)
	case *algebra.Merge:
		return value != nil && visit(value.Inputs()...)
	case algebra.Group:
		return visit(value.Child())
	case *algebra.Group:
		return value != nil && visit(value.Child())
	case algebra.Complete:
		return visit(value.Child())
	case *algebra.Complete:
		return value != nil && visit(value.Child())
	case algebra.ColumnProject:
		return visit(value.Child())
	case *algebra.ColumnProject:
		return value != nil && visit(value.Child())
	case algebra.Expand:
		return visit(value.Child())
	case *algebra.Expand:
		return value != nil && visit(value.Child())
	case algebra.Publish:
		return visit(value.Child())
	case *algebra.Publish:
		return value != nil && visit(value.Child())
	case algebra.Apply:
		if correlation := value.Contract().Correlation(); correlation.Specified() {
			if !correlation.Available() {
				return false
			}
			for ordinal, child := range value.Inputs() {
				// The mixed population ABI has one direct Input child at
				// ordinal zero.  That child is the independent Q driver and
				// therefore has no Complete denominator or posting directory;
				// its scalar SlotSource is redeemed from the population row at
				// runtime.  Certificate partition collection must not ask the
				// scalar child for a posting witness.  The typing/mount passes
				// already prove the complete mixed shape before this projection
				// is consumed, so this only removes the non-posting child from
				// the partition inventory.
				if isPopulationInputChild(child, correlation, ordinal) {
					continue
				}
				projection, projectionOK := correlation.ProjectionAt(ordinal)
				if !projectionOK {
					return false
				}
				if len(projection) == 0 {
					// A shared child has no Q-row posting. Keep its global
					// Complete witness in CompleteDenominators and deliberately
					// omit it from this partition projection. Refuse a malformed
					// registry entry rather than silently treating an arbitrary
					// child as shared.
					if _, sharedOK := exactSharedCompleteDenominator(child); !sharedOK {
						return false
					}
					continue
				}
				if len(projection) != 1 {
					return false
				}
				childDenominator, childOK := completePartitionDenominator(child)
				if !childOK || !childDenominator.Available() || projection[0].Relation() != childDenominator.Relation() {
					return false
				}
				partition := CorrelationPartition{apply: value.Digest(), ordinal: uint32(ordinal), population: correlation.Population(), child: childDenominator, projection: projection[0]}
				if !partition.Available() {
					return false
				}
				if _, duplicate := seen[partition]; !duplicate {
					seen[partition] = struct{}{}
					*result = append(*result, partition)
				}
			}
		}
		return visit(value.Inputs()...)
	case *algebra.Apply:
		if value == nil {
			return false
		}
		return collectCorrelationPartitions(*value, seen, result)
	default:
		return false
	}
}

// exactSharedCompleteDenominator recognizes the only certificate shape that
// may omit a partition authority: Complete(Select(Input)) over the Complete
// node's own relation. This mirrors the checker law and keeps a malformed
// foreign registry from converting an arbitrary child into a broadcast.
func exactSharedCompleteDenominator(expression algebra.Expression) (model.DenominatorRef, bool) {
	complete, ok := expression.(algebra.Complete)
	if !ok {
		pointer, pointerOK := expression.(*algebra.Complete)
		if !pointerOK || pointer == nil {
			return model.DenominatorRef{}, false
		}
		complete = *pointer
	}
	denominator := complete.Denominator()
	if !denominator.Available() {
		return model.DenominatorRef{}, false
	}
	selectExpression, ok := complete.Child().(algebra.Select)
	if !ok {
		pointer, pointerOK := complete.Child().(*algebra.Select)
		if !pointerOK || pointer == nil {
			return model.DenominatorRef{}, false
		}
		selectExpression = *pointer
	}
	input, ok := selectExpression.Child().(algebra.Input)
	if !ok {
		pointer, pointerOK := selectExpression.Child().(*algebra.Input)
		if !pointerOK || pointer == nil {
			return model.DenominatorRef{}, false
		}
		input = *pointer
	}
	if !input.Relation().Available() || input.Relation() != denominator.Relation() {
		return model.DenominatorRef{}, false
	}
	return denominator, true
}

// isPopulationInputChild recognizes only the structural identity of the
// mixed Apply population child: the first child is a direct Input of the
// correlation population relation and projects the population coordinate.
// The checker proves the authored scalar SlotSource and delivery shape; this
// helper intentionally does not inspect a signature or infer a child from
// runtime cardinality.  All-complete Applies retain their historical path
// because child zero is a Complete node and therefore cannot match.
func isPopulationInputChild(expression algebra.Expression, correlation algebra.ApplyCorrelation, ordinal int) bool {
	if ordinal != 0 || !correlation.Available() {
		return false
	}
	projection, projectionOK := correlation.ProjectionAt(ordinal)
	if !projectionOK || len(projection) != 1 || projection[0] != correlation.Coordinate() {
		return false
	}
	var relation model.RelationID
	switch value := expression.(type) {
	case algebra.Input:
		relation = value.Relation()
	case *algebra.Input:
		if value == nil {
			return false
		}
		relation = value.Relation()
	default:
		return false
	}
	return relation.Available() && relation == correlation.Population().Relation()
}

// completePartitionDenominator returns the child authority issued by its
// Complete node. The child expression may be richer than the first replay
// implementation's Complete(Select(Input)) shape; that implementation may
// refuse such a shape at mount, but the schema certificate must still expose
// the exact child denominator instead of silently treating the partition as
// absent. The Complete node is the authority; no child scan or inferred
// denominator is allowed here.
func completePartitionDenominator(expression algebra.Expression) (model.DenominatorRef, bool) {
	complete, ok := expression.(algebra.Complete)
	if !ok {
		if pointer, pointerOK := expression.(*algebra.Complete); pointerOK && pointer != nil {
			complete = *pointer
		} else {
			return model.DenominatorRef{}, false
		}
	}
	return complete.Denominator(), complete.Denominator().Available()
}

func correlationContentLess(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func appendCorrelationContent(value identity.ContentID) []byte {
	return append([]byte(nil), value[:]...)
}

func appendCorrelationDenominator(value model.DenominatorRef) []byte {
	relation := value.Relation()
	key := value.Key()
	relationOwner, relationContent := relation.Owner().Content(), relation.Content()
	keyOwner, keyContent := key.Owner().Content(), key.Content()
	result := appendCorrelationContent(relationOwner)
	result = append(result, relationContent[:]...)
	result = append(result, keyOwner[:]...)
	return append(result, keyContent[:]...)
}

func appendCorrelationColumn(value model.ColumnID) []byte {
	relation := value.Relation()
	relationOwner, relationContent, columnContent := relation.Owner().Content(), relation.Content(), value.Content()
	result := appendCorrelationContent(relationOwner)
	result = append(result, relationContent[:]...)
	return append(result, columnContent[:]...)
}

func denominatorOrder(left, right model.DenominatorRef) int {
	leftRelation := left.Relation().Owner().Content().String() + "/" + left.Relation().Content().String()
	rightRelation := right.Relation().Owner().Content().String() + "/" + right.Relation().Content().String()
	if leftRelation < rightRelation {
		return -1
	}
	if leftRelation > rightRelation {
		return 1
	}
	leftKey := left.Key().Owner().Content().String() + "/" + left.Key().Content().String()
	rightKey := right.Key().Owner().Content().String() + "/" + right.Key().Content().String()
	if leftKey < rightKey {
		return -1
	}
	if leftKey > rightKey {
		return 1
	}
	return 0
}
