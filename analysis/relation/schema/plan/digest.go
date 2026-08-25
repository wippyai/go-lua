package plan

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func digestExecutionSchema(schema ExecutionSchema) identity.ContentID {
	sections := [][]byte{
		contentBytes(derive("relation/schema/plan/schema-id/v1", schemaIDBytes(schema.schemaID))),
		contentBytes(sectionIDs("relations", relationSchemaIDs(schema.relations))),
		contentBytes(sectionIDs("columns", columnSchemaIDs(schema.columns))),
		contentBytes(sectionIDs("keys", keySchemaIDs(schema.keys))),
		contentBytes(sectionIDs("scopes", scopeSchemaIDs(schema.scopes))),
		contentBytes(sectionIDs("expressions", expressionIDs(schema.expressions))),
		contentBytes(sectionIDs("dependencies", dependencyIDs(schema.dependencies))),
		contentBytes(sectionIDs("sccs", sccIDs(schema.sccs))),
		contentBytes(sectionIDs("signatures", signatureIDs(schema.signatures))),
	}
	return derive("relation/schema/plan/execution-schema/v1", sections...)
}

func sectionIDs(name string, ids []identity.ContentID) identity.ContentID {
	parts := make([][]byte, 0, len(ids)+1)
	parts = append(parts, []byte(name))
	for _, id := range ids {
		parts = append(parts, contentBytes(id))
	}
	return derive("relation/schema/plan/section/v1", parts...)
}

func relationSchemaIDs(values []model.RelationSchema) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, digestRelationSchema(value))
	}
	return ids
}

func columnSchemaIDs(values []model.ColumnSchema) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, digestColumnSchema(value))
	}
	return ids
}

func keySchemaIDs(values []model.KeySchema) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, digestKeySchema(value))
	}
	return ids
}

func scopeSchemaIDs(values []model.ScopeSchema) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, digestScopeSchema(value))
	}
	return ids
}

func expressionIDs(values []ExpressionRef) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, digestExpressionRef(value))
	}
	return ids
}

func dependencyIDs(values []Dependency) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.Digest())
	}
	return ids
}

func sccIDs(values []SCC) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.Digest())
	}
	return ids
}

func signatureIDs(values []signature.Signature) []identity.ContentID {
	ids := make([]identity.ContentID, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.Digest())
	}
	return ids
}

func digestDependency(value Dependency) identity.ContentID {
	parts := [][]byte{dependencyIDBytes(value.id), expressionIDBytes(value.expression)}
	for _, relation := range value.reads {
		parts = append(parts, relationBytes(relation))
	}
	for _, relation := range value.writes {
		parts = append(parts, relationBytes(relation))
	}
	return derive("relation/schema/plan/dependency/v1", parts...)
}

func digestEdge(value DependencyEdge) identity.ContentID {
	return derive("relation/schema/plan/edge/v1", dependencyIDBytes(value.from.ID()), dependencyIDBytes(value.to.ID()))
}

func digestWideningHead(value WideningHead) identity.ContentID {
	return derive("relation/schema/plan/widening-head/v1", dependencyIDBytes(value.dependency.ID()), relationBytes(value.relation))
}

func digestRecurrence(value Recurrence) identity.ContentID {
	parts := [][]byte{uint64Bytes(uint64(value.kind))}
	for _, head := range value.heads {
		parts = append(parts, contentBytes(head.Digest()))
	}
	return derive("relation/schema/plan/recurrence/v1", parts...)
}

func digestSCC(value SCC) identity.ContentID {
	parts := [][]byte{contentBytes(value.recurrence.Digest())}
	for _, member := range value.members {
		parts = append(parts, dependencyIDBytes(member.ID()))
	}
	for _, edge := range value.edges {
		parts = append(parts, contentBytes(edge.Digest()))
	}
	return derive("relation/schema/plan/scc/v1", parts...)
}

func digestRelationSchema(value model.RelationSchema) identity.ContentID {
	// Scope dimensions are digested by the complete ScopeSchema registry;
	// relations retain only the nominal scope reference.
	parts := [][]byte{relationBytesOfID(value.ID()), scopeBytes(value.Scope())}
	for _, column := range value.Columns() {
		parts = append(parts, columnBytes(column))
	}
	for _, key := range value.Keys() {
		parts = append(parts, keyBytes(key))
	}
	return derive("relation/schema/plan/relation/v1", parts...)
}

func digestColumnSchema(value model.ColumnSchema) identity.ContentID {
	return derive("relation/schema/plan/column/v1", columnBytes(value.ID()), typeBytes(value.Type()))
}

func digestKeySchema(value model.KeySchema) identity.ContentID {
	parts := [][]byte{keyBytes(value.ID())}
	for _, column := range value.Columns() {
		parts = append(parts, columnBytes(column))
	}
	return derive("relation/schema/plan/key/v1", parts...)
}

func digestScopeSchema(value model.ScopeSchema) identity.ContentID {
	parts := [][]byte{scopeBytes(value.ID())}
	for _, dimension := range value.Dimensions() {
		parts = append(parts, columnBytes(dimension))
	}
	return derive("relation/schema/plan/scope/v1", parts...)
}

func relationBytes(ref RelationRef) []byte { return relationBytesOfID(ref.ID()) }

func digestExpressionRef(value ExpressionRef) identity.ContentID {
	return derive("relation/schema/plan/expression-entry/v1", expressionIDBytes(value.ID()), contentBytes(value.Digest()))
}

func digestDependencyRef(value DependencyRef) identity.ContentID {
	return derive("relation/schema/plan/dependency-ref/v1", dependencyIDBytes(value.ID()))
}

func expressionIDBytes(value model.ExpressionID) []byte {
	return append(contentBytes(value.Owner().Content()), contentBytes(value.Content())...)
}

func schemaIDBytes(value model.SchemaID) []byte {
	return append(contentBytes(value.Owner().Content()), contentBytes(value.Content())...)
}

func dependencyIDBytes(value model.DependencyID) []byte {
	return append(contentBytes(value.Owner().Content()), contentBytes(value.Content())...)
}

func expressionIDLess(left, right model.ExpressionID) bool {
	if contentLess(left.Owner().Content(), right.Owner().Content()) {
		return true
	}
	return left.Owner() == right.Owner() && contentLess(left.Content(), right.Content())
}

func dependencyIDLess(left, right model.DependencyID) bool {
	if contentLess(left.Owner().Content(), right.Owner().Content()) {
		return true
	}
	return left.Owner() == right.Owner() && contentLess(left.Content(), right.Content())
}

func relationBytesOfID(value model.RelationID) []byte {
	return append(contentBytes(value.Owner().Content()), contentBytes(value.Content())...)
}

func scopeBytes(value model.ScopeID) []byte {
	return append(contentBytes(value.Owner().Content()), contentBytes(value.Content())...)
}

func columnBytes(value model.ColumnID) []byte {
	return append(relationBytesOfID(value.Relation()), contentBytes(value.Content())...)
}

func keyBytes(value model.KeyID) []byte {
	return append(relationBytesOfID(value.Relation()), contentBytes(value.Content())...)
}

func typeBytes(value model.TypeID) []byte {
	return append(contentBytes(value.Owner().Content()), contentBytes(value.Content())...)
}

func relationLess(left, right RelationRef) bool { return relationLessByID(left.ID(), right.ID()) }

func relationLessByID(left, right model.RelationID) bool {
	if contentLess(left.Owner().Content(), right.Owner().Content()) {
		return true
	}
	return left.Owner() == right.Owner() && contentLess(left.Content(), right.Content())
}

func columnLessByID(left, right model.ColumnID) bool {
	if relationLessByID(left.Relation(), right.Relation()) {
		return true
	}
	return left.Relation() == right.Relation() && contentLess(left.Content(), right.Content())
}

func keyLessByID(left, right model.KeyID) bool {
	if relationLessByID(left.Relation(), right.Relation()) {
		return true
	}
	return left.Relation() == right.Relation() && contentLess(left.Content(), right.Content())
}

func scopeLessByID(left, right model.ScopeID) bool {
	if contentLess(left.Owner().Content(), right.Owner().Content()) {
		return true
	}
	return left.Owner() == right.Owner() && contentLess(left.Content(), right.Content())
}

func contentLess(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func derive(tag string, parts ...[]byte) identity.ContentID {
	value, ok := identity.DeriveContentID(tag, parts...)
	if !ok {
		return identity.ContentID{}
	}
	return value
}

func uint64Bytes(value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return encoded[:]
}

func contentBytes(value identity.ContentID) []byte { return append([]byte(nil), value[:]...) }
