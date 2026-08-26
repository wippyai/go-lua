package plan

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// ExecutionSchema is the immutable, unchecked logical artifact identified by
// schemaID. It contains stable declarations, expression entries, dependency
// projections, and full semantic signatures; checking and execution are owned
// by higher layers.
type ExecutionSchema struct {
	schemaID      model.SchemaID
	relations     []model.RelationSchema
	columns       []model.ColumnSchema
	keys          []model.KeySchema
	scopes        []model.ScopeSchema
	expressions   []ExpressionRef
	dependencies  []Dependency
	sccs          []SCC
	signatures    []signature.Signature
	initials      []Initial
	capabilities  []model.TypeCapability
	observations  []algebra.ObservationContract
	contributions []output.ContributionSpec
	digest        identity.ContentID
}

func (schema ExecutionSchema) Available() bool            { return schema.digest.Available() }
func (schema ExecutionSchema) Digest() identity.ContentID { return schema.digest }

// SchemaID returns the owner-issued identity of this logical artifact.
func (schema ExecutionSchema) SchemaID() model.SchemaID { return schema.schemaID }
func (schema ExecutionSchema) Relations() []model.RelationSchema {
	return append([]model.RelationSchema(nil), schema.relations...)
}
func (schema ExecutionSchema) Columns() []model.ColumnSchema {
	return append([]model.ColumnSchema(nil), schema.columns...)
}
func (schema ExecutionSchema) Keys() []model.KeySchema {
	return append([]model.KeySchema(nil), schema.keys...)
}
func (schema ExecutionSchema) Scopes() []model.ScopeSchema {
	return append([]model.ScopeSchema(nil), schema.scopes...)
}
func (schema ExecutionSchema) Expressions() []ExpressionRef {
	return append([]ExpressionRef(nil), schema.expressions...)
}
func (schema ExecutionSchema) Dependencies() []Dependency {
	return append([]Dependency(nil), schema.dependencies...)
}
func (schema ExecutionSchema) SCCs() []SCC { return append([]SCC(nil), schema.sccs...) }

// Signatures returns the complete semantic contracts retained by the plan.
// The checker resolves expression operation identities against this registry.
func (schema ExecutionSchema) Signatures() []signature.Signature {
	return append([]signature.Signature(nil), schema.signatures...)
}

// Initials returns the complete immutable zero-input invocation catalogue.
// The rows are logical declaration data, not runtime requests.
func (schema ExecutionSchema) Initials() []Initial {
	return append([]Initial(nil), schema.initials...)
}

// TypeCapabilities returns the complete schema-sealed policy catalogue. A
// capability is the only declaration that can grant lattice ascent to a
// TypeID; callers must not infer it from a signature's Presence contract.
func (schema ExecutionSchema) TypeCapabilities() []model.TypeCapability {
	return append([]model.TypeCapability(nil), schema.capabilities...)
}

// Observations returns the complete schema-owned terminal observation
// descriptor catalogue. Descriptors are sealed with the execution schema and
// are not accepted from runtime callers after mount.
func (schema ExecutionSchema) Observations() []algebra.ObservationContract {
	return append([]algebra.ObservationContract(nil), schema.observations...)
}

// Contributions returns the complete schema-authored output contribution
// declarations. Each value is already sealed by the output schema package;
// checker admission still proves its port and capability against this exact
// schema's signatures and type registry.
func (schema ExecutionSchema) Contributions() []output.ContributionSpec {
	return append([]output.ContributionSpec(nil), schema.contributions...)
}

// Builder is mutable construction state. Add methods only retain declarations;
// they do not claim semantic validity or uniqueness. Build copies and
// canonicalizes the collections, then seals the builder.
type Builder struct {
	sealed        bool
	schemaID      model.SchemaID
	relations     []model.RelationSchema
	columns       []model.ColumnSchema
	keys          []model.KeySchema
	scopes        []model.ScopeSchema
	expressions   []ExpressionRef
	dependencies  []Dependency
	sccs          []SCC
	signatures    []signature.Signature
	initials      []Initial
	capabilities  []model.TypeCapability
	observations  []algebra.ObservationContract
	contributions []output.ContributionSpec
}

// NewBuilder begins an unchecked artifact for schemaID. The ID is retained
// exactly, including a zero or foreign value for the independent checker.
func NewBuilder(schemaID model.SchemaID) *Builder { return &Builder{schemaID: schemaID} }

func (builder *Builder) AddRelation(value model.RelationSchema) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.relations = append(builder.relations, value)
	return true
}

func (builder *Builder) AddColumn(value model.ColumnSchema) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.columns = append(builder.columns, value)
	return true
}

func (builder *Builder) AddKey(value model.KeySchema) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.keys = append(builder.keys, value)
	return true
}

func (builder *Builder) AddScope(value model.ScopeSchema) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.scopes = append(builder.scopes, value)
	return true
}

func (builder *Builder) AddExpression(value ExpressionRef) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.expressions = append(builder.expressions, value)
	return true
}

func (builder *Builder) AddDependency(value Dependency) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.dependencies = append(builder.dependencies, value)
	return true
}

func (builder *Builder) AddSCC(value SCC) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.sccs = append(builder.sccs, value)
	return true
}

func (builder *Builder) AddSignature(value signature.Signature) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.signatures = append(builder.signatures, value)
	return true
}

// AddInitial adds one owner declaration for a zero-input invocation. The
// independent checker owns validity and duplicate refusal; the builder only
// retains the immutable declaration for sealing.
func (builder *Builder) AddInitial(value Initial) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.initials = append(builder.initials, value)
	return true
}

// AddTypeCapability adds one explicit owner policy to the unchecked schema.
// Duplicate and cross-declaration validation belongs to the independent
// checker, just like the other logical declaration kinds.
func (builder *Builder) AddTypeCapability(value model.TypeCapability) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.capabilities = append(builder.capabilities, value)
	return true
}

// AddObservation adds one schema-owned terminal observation descriptor.
// Cross-reference and positional validation belongs to the independent
// checker, just like the other logical declaration kinds.
func (builder *Builder) AddObservation(value algebra.ObservationContract) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.observations = append(builder.observations, value)
	return true
}

// AddContribution retains one sealed output contribution declaration. The
// independent checker owns duplicate, signature-port, and capability
// validation; the builder only retains the authored declaration.
func (builder *Builder) AddContribution(value output.ContributionSpec) bool {
	if builder == nil || builder.sealed {
		return false
	}
	builder.contributions = append(builder.contributions, value)
	return true
}

// Build freezes one artifact. It intentionally performs no semantic,
// uniqueness, or cross-reference checking; that is the independent checker's
// authority.
func (builder *Builder) Build() (ExecutionSchema, bool) {
	if builder == nil || builder.sealed {
		return ExecutionSchema{}, false
	}
	schema := ExecutionSchema{
		schemaID:      builder.schemaID,
		relations:     append([]model.RelationSchema(nil), builder.relations...),
		columns:       append([]model.ColumnSchema(nil), builder.columns...),
		keys:          append([]model.KeySchema(nil), builder.keys...),
		scopes:        append([]model.ScopeSchema(nil), builder.scopes...),
		expressions:   append([]ExpressionRef(nil), builder.expressions...),
		dependencies:  append([]Dependency(nil), builder.dependencies...),
		sccs:          append([]SCC(nil), builder.sccs...),
		signatures:    append([]signature.Signature(nil), builder.signatures...),
		initials:      append([]Initial(nil), builder.initials...),
		capabilities:  append([]model.TypeCapability(nil), builder.capabilities...),
		observations:  append([]algebra.ObservationContract(nil), builder.observations...),
		contributions: append([]output.ContributionSpec(nil), builder.contributions...),
	}
	sort.Slice(schema.relations, func(left, right int) bool {
		leftValue, rightValue := schema.relations[left], schema.relations[right]
		if relationLessByID(leftValue.ID(), rightValue.ID()) {
			return true
		}
		if relationLessByID(rightValue.ID(), leftValue.ID()) {
			return false
		}
		return contentLess(digestRelationSchema(leftValue), digestRelationSchema(rightValue))
	})
	sort.Slice(schema.columns, func(left, right int) bool {
		leftValue, rightValue := schema.columns[left], schema.columns[right]
		if columnLessByID(leftValue.ID(), rightValue.ID()) {
			return true
		}
		if columnLessByID(rightValue.ID(), leftValue.ID()) {
			return false
		}
		return contentLess(digestColumnSchema(leftValue), digestColumnSchema(rightValue))
	})
	sort.Slice(schema.keys, func(left, right int) bool {
		leftValue, rightValue := schema.keys[left], schema.keys[right]
		if keyLessByID(leftValue.ID(), rightValue.ID()) {
			return true
		}
		if keyLessByID(rightValue.ID(), leftValue.ID()) {
			return false
		}
		return contentLess(digestKeySchema(leftValue), digestKeySchema(rightValue))
	})
	sort.Slice(schema.scopes, func(left, right int) bool {
		leftValue, rightValue := schema.scopes[left], schema.scopes[right]
		if scopeLessByID(leftValue.ID(), rightValue.ID()) {
			return true
		}
		if scopeLessByID(rightValue.ID(), leftValue.ID()) {
			return false
		}
		return contentLess(digestScopeSchema(leftValue), digestScopeSchema(rightValue))
	})
	sort.Slice(schema.expressions, func(left, right int) bool {
		leftValue, rightValue := schema.expressions[left], schema.expressions[right]
		if expressionIDLess(leftValue.ID(), rightValue.ID()) {
			return true
		}
		if expressionIDLess(rightValue.ID(), leftValue.ID()) {
			return false
		}
		return contentLess(leftValue.Digest(), rightValue.Digest())
	})
	sort.Slice(schema.dependencies, func(left, right int) bool {
		leftValue, rightValue := schema.dependencies[left], schema.dependencies[right]
		if dependencyIDLess(leftValue.ID(), rightValue.ID()) {
			return true
		}
		if dependencyIDLess(rightValue.ID(), leftValue.ID()) {
			return false
		}
		return contentLess(leftValue.Digest(), rightValue.Digest())
	})
	sort.Slice(schema.sccs, func(left, right int) bool {
		return contentLess(schema.sccs[left].Digest(), schema.sccs[right].Digest())
	})
	sort.Slice(schema.signatures, func(left, right int) bool {
		return contentLess(schema.signatures[left].Digest(), schema.signatures[right].Digest())
	})
	sort.Slice(schema.initials, func(left, right int) bool {
		return contentLess(schema.initials[left].Digest(), schema.initials[right].Digest())
	})
	sort.Slice(schema.capabilities, func(left, right int) bool {
		leftValue, rightValue := schema.capabilities[left], schema.capabilities[right]
		if typeIDLess(leftValue.Type(), rightValue.Type()) {
			return true
		}
		if typeIDLess(rightValue.Type(), leftValue.Type()) {
			return false
		}
		return contentLess(leftValue.Digest(), rightValue.Digest())
	})
	sort.Slice(schema.observations, func(left, right int) bool {
		return contentLess(schema.observations[left].Digest(), schema.observations[right].Digest())
	})
	sort.Slice(schema.contributions, func(left, right int) bool {
		return contentLess(schema.contributions[left].Digest(), schema.contributions[right].Digest())
	})
	schema.digest = digestExecutionSchema(schema)
	if !schema.digest.Available() {
		return ExecutionSchema{}, false
	}
	builder.sealed = true
	return schema, true
}
