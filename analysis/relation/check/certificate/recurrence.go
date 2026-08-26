package certificate

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// RecurrenceData is the certificate-owned logical recurrence projection.
// It contains only owner-issued identities and canonical relation sets; it
// does not leak a schema, registry, checker, scheduler, or physical object.
type RecurrenceData struct {
	projections   []RecurrenceProjection
	edges         []RecurrenceEdge
	components    []RecurrenceComponent
	wideningHeads []RecurrenceHead
	completeUses  []CompleteUse
	valid         bool
}

// RecurrenceProjection is the proved relation footprint of one dependency.
// Expression lets a physical binder associate the dependency with its sealed
// expression entry without reopening the declaration registry.
type RecurrenceProjection struct {
	dependency   model.DependencyID
	expression   model.ExpressionID
	reads        []model.RelationID
	writes       []model.RelationID
	completeUses []CompleteUse
}

func (projection RecurrenceProjection) Dependency() model.DependencyID { return projection.dependency }
func (projection RecurrenceProjection) Expression() model.ExpressionID { return projection.expression }
func (projection RecurrenceProjection) Reads() []model.RelationID {
	return append([]model.RelationID(nil), projection.reads...)
}
func (projection RecurrenceProjection) Writes() []model.RelationID {
	return append([]model.RelationID(nil), projection.writes...)
}
func (projection RecurrenceProjection) CompleteUses() []CompleteUse {
	return cloneCompleteUses(projection.completeUses)
}

// CompleteWriter is immutable writer/cold-stratification evidence copied from
// the recurrence checker into the certificate namespace.
type CompleteWriter struct {
	dependency model.DependencyID
	component  uint32
	earlier    bool
}

func (writer CompleteWriter) Dependency() model.DependencyID { return writer.dependency }
func (writer CompleteWriter) Component() uint32              { return writer.component }
func (writer CompleteWriter) Earlier() bool                  { return writer.earlier }
func (writer CompleteWriter) Available() bool                { return writer.dependency.Available() && writer.earlier }
func (writer CompleteWriter) IsEarlier() bool                { return writer.earlier }

// CompleteUse is one checked Complete occurrence owned by a dependency. Its
// occurrence/path identity is stable logical evidence; no physical ordinal or
// mutable epoch is retained. Cold is true exactly when Writers is empty.
type CompleteUse struct {
	dependency  model.DependencyID
	expression  model.ExpressionID
	path        string
	occurrence  identity.ContentID
	child       model.RelationID
	denominator model.DenominatorRef
	writers     []CompleteWriter
	cold        bool
}

func (use CompleteUse) Dependency() model.DependencyID  { return use.dependency }
func (use CompleteUse) Expression() model.ExpressionID  { return use.expression }
func (use CompleteUse) Path() string                    { return use.path }
func (use CompleteUse) Occurrence() identity.ContentID  { return use.occurrence }
func (use CompleteUse) Child() model.RelationID         { return use.child }
func (use CompleteUse) ChildRelation() model.RelationID { return use.child }
func (use CompleteUse) Relation() model.RelationID      { return use.child }
func (use CompleteUse) Denominator() model.DenominatorRef {
	return use.denominator
}
func (use CompleteUse) Cold() bool      { return use.cold }
func (use CompleteUse) SolveCold() bool { return use.cold }
func (use CompleteUse) IsCold() bool    { return use.cold }
func (use CompleteUse) Writers() []CompleteWriter {
	return append([]CompleteWriter(nil), use.writers...)
}
func (use CompleteUse) WriterProof() []CompleteWriter        { return use.Writers() }
func (use CompleteUse) OccurrenceID() identity.ContentID     { return use.occurrence }
func (use CompleteUse) DenominatorRef() model.DenominatorRef { return use.denominator }
func (use CompleteUse) Available() bool {
	if !use.dependency.Available() || !use.expression.Available() || use.path == "" || !use.occurrence.Available() || !use.child.Available() || !use.denominator.Available() || use.denominator.Relation() != use.child || use.cold != (len(use.writers) == 0) {
		return false
	}
	seen := make(map[model.DependencyID]struct{}, len(use.writers))
	for _, writer := range use.writers {
		if !writer.Available() {
			return false
		}
		if _, duplicate := seen[writer.dependency]; duplicate {
			return false
		}
		seen[writer.dependency] = struct{}{}
	}
	return true
}
func (use CompleteUse) WriterIDs() []model.DependencyID {
	result := make([]model.DependencyID, len(use.writers))
	for index, writer := range use.writers {
		result[index] = writer.dependency
	}
	return result
}

// Digest includes every CompleteUse field and its strict writer proof. It is
// used by certificate construction so evidence changes cannot preserve a
// mounted identity accidentally.
func (use CompleteUse) Digest() identity.ContentID {
	if !use.dependency.Available() || !use.expression.Available() || use.path == "" || !use.occurrence.Available() || !use.child.Available() || !use.denominator.Available() || use.denominator.Relation() != use.child || use.cold != (len(use.writers) == 0) {
		return identity.ContentID{}
	}
	parts := [][]byte{
		contentBytes(use.dependency.Owner().Content()), contentBytes(use.dependency.Content()),
		contentBytes(use.expression.Owner().Content()), contentBytes(use.expression.Content()),
		[]byte(use.path), contentBytes(use.occurrence),
		contentBytes(use.child.Owner().Content()), contentBytes(use.child.Content()),
		contentBytes(use.denominator.Relation().Owner().Content()), contentBytes(use.denominator.Relation().Content()),
		contentBytes(use.denominator.Key().Owner().Content()), contentBytes(use.denominator.Key().Content()),
		[]byte{boolByte(use.cold)},
	}
	for _, writer := range use.writers {
		if !writer.dependency.Available() || !writer.earlier {
			return identity.ContentID{}
		}
		parts = append(parts, contentBytes(writer.dependency.Owner().Content()), contentBytes(writer.dependency.Content()), uint32Bytes(writer.component), []byte{boolByte(writer.earlier)})
	}
	value, ok := identity.DeriveContentID("analysis/relation/check/recurrence/complete-use/v1", parts...)
	if !ok {
		return identity.ContentID{}
	}
	return value
}

func contentBytes(value identity.ContentID) []byte { return append([]byte(nil), value[:]...) }
func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
func uint32Bytes(value uint32) []byte {
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}

func cloneCompleteUses(values []CompleteUse) []CompleteUse {
	if values == nil {
		return nil
	}
	result := make([]CompleteUse, len(values))
	for index, value := range values {
		result[index] = value
		result[index].writers = append([]CompleteWriter(nil), value.writers...)
	}
	return result
}

// RecurrenceEdge is a canonical dependency graph edge. It uses identities
// rather than plan declarations so physical consumers cannot reopen schema.
type RecurrenceEdge struct {
	from model.DependencyID
	to   model.DependencyID
}

func (edge RecurrenceEdge) From() model.DependencyID { return edge.from }
func (edge RecurrenceEdge) To() model.DependencyID   { return edge.to }

// RecurrenceComponent is a proved SCC projection. Consumers must use Cyclic
// rather than infer recurrence by re-walking the graph.
type RecurrenceComponent struct {
	members []model.DependencyID
	edges   []RecurrenceEdge
	cyclic  bool
}

func (component RecurrenceComponent) Members() []model.DependencyID {
	return append([]model.DependencyID(nil), component.members...)
}
func (component RecurrenceComponent) Edges() []RecurrenceEdge {
	return append([]RecurrenceEdge(nil), component.edges...)
}
func (component RecurrenceComponent) Cyclic() bool { return component.cyclic }

// RecurrenceHead is a validated widening permission for one dependency and
// relation. It is an identity pair, not a plan declaration.
type RecurrenceHead struct {
	dependency model.DependencyID
	relation   model.RelationID
}

func (head RecurrenceHead) Dependency() model.DependencyID { return head.dependency }
func (head RecurrenceHead) Relation() model.RelationID     { return head.relation }

// Available reports whether Check produced a complete recurrence projection.
// A valid empty projection is distinct from the zero value.
func (recurrence RecurrenceData) Available() bool { return recurrence.valid }

// Projections returns defensive copies in canonical dependency order.
func (recurrence RecurrenceData) Projections() []RecurrenceProjection {
	result := make([]RecurrenceProjection, len(recurrence.projections))
	for index, value := range recurrence.projections {
		result[index] = RecurrenceProjection{
			dependency:   value.dependency,
			expression:   value.expression,
			reads:        append([]model.RelationID(nil), value.reads...),
			writes:       append([]model.RelationID(nil), value.writes...),
			completeUses: cloneCompleteUses(value.completeUses),
		}
	}
	return result
}

// CompleteUses returns every immutable Complete occurrence in canonical
// dependency/occurrence order.
func (recurrence RecurrenceData) CompleteUses() []CompleteUse {
	return cloneCompleteUses(recurrence.completeUses)
}

// Edges returns defensive copies in canonical producer/consumer order.
func (recurrence RecurrenceData) Edges() []RecurrenceEdge {
	return append([]RecurrenceEdge(nil), recurrence.edges...)
}

// Components returns defensive copies in canonical member-set order.
func (recurrence RecurrenceData) Components() []RecurrenceComponent {
	result := make([]RecurrenceComponent, len(recurrence.components))
	for index, value := range recurrence.components {
		result[index] = RecurrenceComponent{
			members: append([]model.DependencyID(nil), value.members...),
			edges:   append([]RecurrenceEdge(nil), value.edges...),
			cyclic:  value.cyclic,
		}
	}
	return result
}

// WideningHeads returns defensive copies in canonical dependency/relation
// order.
func (recurrence RecurrenceData) WideningHeads() []RecurrenceHead {
	return append([]RecurrenceHead(nil), recurrence.wideningHeads...)
}
