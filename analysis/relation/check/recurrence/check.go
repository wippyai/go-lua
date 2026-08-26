package recurrence

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	checkregistry "github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Code identifies one deterministic recurrence refusal.  The values are
// intentionally closed: callers can classify a refusal without parsing its
// text, while Error.Error remains useful in diagnostics.
type Code uint8

const (
	CodeInvalid Code = iota
	CodeUnavailableSchema
	CodeDuplicateExpression
	CodeInvalidExpression
	CodeDuplicateSignature
	CodeInvalidSignature
	CodeMissingSignature
	CodeDuplicateDependency
	CodeInvalidDependency
	CodeMissingExpression
	CodeInvalidRelation
	CodeDuplicateProjection
	CodeProjectionMismatch
	CodeDuplicateEdge
	CodeMissingEdge
	CodeSpuriousEdge
	CodeDuplicateComponent
	CodeMissingComponent
	CodeSpuriousComponent
	CodeRecurrencePolicy
	CodeInvalidWideningHead
	CodeDuplicateWideningHead
	// CodeCompleteStratification is returned when a Complete occurrence can
	// observe a relation writer in its own SCC or in a component that is not a
	// proved strict predecessor.  Complete is a closed-world read and cannot
	// be solved against a same-component (or later) writer.
	CodeCompleteStratification
)

// CodeCompleteSameComponent and CodeInvalidCompleteStratification are
// compatibility aliases for callers that classify the more specific
// Complete refusal.  The checker deliberately has one refusal code: both a
// same-component writer and a non-predecessor writer violate the same strict
// stratification law.
const (
	CodeCompleteSameComponent         = CodeCompleteStratification
	CodeInvalidCompleteStratification = CodeCompleteStratification
)

// Error is the first canonical refusal in the checker ordering.  The checker
// sorts every input before validating it and never returns a map iteration's
// arbitrary failure, so equal malformed artifacts fail with equal Code and
// stable context.
type Error struct {
	Code       Code
	Dependency model.DependencyID
	Other      model.DependencyID
	Relation   model.RelationID
	Occurrence identity.ContentID
	Detail     string
}

func (err *Error) Error() string {
	if err == nil {
		return "relation/check/recurrence: <nil>"
	}
	name := codeName(err.Code)
	if err.Detail == "" {
		return "relation/check/recurrence: " + name
	}
	return "relation/check/recurrence: " + name + ": " + err.Detail
}

func codeName(code Code) string {
	switch code {
	case CodeUnavailableSchema:
		return "schema unavailable"
	case CodeDuplicateExpression:
		return "duplicate expression"
	case CodeInvalidExpression:
		return "invalid expression"
	case CodeDuplicateSignature:
		return "duplicate signature"
	case CodeInvalidSignature:
		return "invalid signature"
	case CodeMissingSignature:
		return "missing signature"
	case CodeDuplicateDependency:
		return "duplicate dependency"
	case CodeInvalidDependency:
		return "invalid dependency"
	case CodeMissingExpression:
		return "missing dependency expression"
	case CodeInvalidRelation:
		return "invalid relation reference"
	case CodeDuplicateProjection:
		return "duplicate relation projection"
	case CodeProjectionMismatch:
		return "relation projection mismatch"
	case CodeDuplicateEdge:
		return "duplicate dependency edge"
	case CodeMissingEdge:
		return "missing dependency edge"
	case CodeSpuriousEdge:
		return "spurious dependency edge"
	case CodeDuplicateComponent:
		return "duplicate SCC component"
	case CodeMissingComponent:
		return "missing SCC component"
	case CodeSpuriousComponent:
		return "spurious SCC component"
	case CodeRecurrencePolicy:
		return "invalid recurrence policy"
	case CodeInvalidWideningHead:
		return "invalid widening head"
	case CodeDuplicateWideningHead:
		return "duplicate widening head"
	case CodeCompleteStratification:
		return "invalid Complete stratification"
	default:
		return "invalid recurrence declaration"
	}
}

// CompleteWriter is one immutable writer-side proof for a Complete use. The
// component is the canonical topological SCC rank redeemed by the checker;
// Earlier is retained as evidence rather than inferred by a physical
// scheduler.  A writer is never emitted unless it writes the Complete child
// relation.
type CompleteWriter struct {
	Dependency model.DependencyID
	Component  uint32
	Earlier    bool
}

// CompleteUse is the checker-owned evidence for one Complete occurrence.
// Path and Occurrence identify the occurrence under the sealed expression
// root; neither is a physical address.  Cold is true exactly when Writers is
// empty, proving that the denominator has no solve writer.
type CompleteUse struct {
	Dependency  model.DependencyID
	Expression  model.ExpressionID
	Path        string
	Occurrence  identity.ContentID
	Child       model.RelationID
	Denominator model.DenominatorRef
	Writers     []CompleteWriter
	Cold        bool
}

// Digest is a stable identity of the complete evidence, including the
// writer/cold proof. It is intentionally derived here rather than from a
// declaration ordinal so certificate and mount identities cannot omit this
// semantic evidence.
func (use CompleteUse) Digest() identity.ContentID {
	if !use.Dependency.Available() || !use.Expression.Available() || !use.Occurrence.Available() || !use.Child.Available() || !use.Denominator.Available() {
		return identity.ContentID{}
	}
	parts := [][]byte{
		contentBytes(use.Dependency.Owner().Content()), contentBytes(use.Dependency.Content()),
		contentBytes(use.Expression.Owner().Content()), contentBytes(use.Expression.Content()),
		contentBytes(use.Occurrence), []byte(use.Path),
		contentBytes(use.Child.Owner().Content()), contentBytes(use.Child.Content()),
		contentBytes(use.Denominator.Relation().Owner().Content()), contentBytes(use.Denominator.Relation().Content()),
		contentBytes(use.Denominator.Key().Owner().Content()), contentBytes(use.Denominator.Key().Content()),
		[]byte{boolByte(use.Cold)},
	}
	for _, writer := range use.Writers {
		if !writer.Dependency.Available() || !writer.Earlier {
			return identity.ContentID{}
		}
		parts = append(parts,
			contentBytes(writer.Dependency.Owner().Content()), contentBytes(writer.Dependency.Content()),
			uint32Bytes(writer.Component), []byte{boolByte(writer.Earlier)})
	}
	return deriveContentID("analysis/relation/check/recurrence/complete-use/v1", parts...)
}

func contentBytes(value identity.ContentID) []byte { return append([]byte(nil), value[:]...) }

func uint32Bytes(value uint32) []byte {
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

func deriveContentID(domain string, parts ...[]byte) identity.ContentID {
	value, ok := identity.DeriveContentID(domain, parts...)
	if !ok {
		return identity.ContentID{}
	}
	return value
}

// WriterIDs returns defensive writer identities in canonical dependency
// order.  CompleteUse itself is copied by Proof.CompleteUses.
func (use CompleteUse) WriterIDs() []model.DependencyID {
	result := make([]model.DependencyID, len(use.Writers))
	for index, writer := range use.Writers {
		result[index] = writer.Dependency
	}
	return result
}

func (writer CompleteWriter) Available() bool {
	return writer.Dependency.Available() && writer.Earlier
}
func (writer CompleteWriter) IsEarlier() bool { return writer.Earlier }

func (use CompleteUse) ChildRelation() model.RelationID      { return use.Child }
func (use CompleteUse) Relation() model.RelationID           { return use.Child }
func (use CompleteUse) DenominatorRef() model.DenominatorRef { return use.Denominator }
func (use CompleteUse) SolveCold() bool                      { return use.Cold }
func (use CompleteUse) IsCold() bool                         { return use.Cold }
func (use CompleteUse) WriterProof() []CompleteWriter {
	return append([]CompleteWriter(nil), use.Writers...)
}

func (use CompleteUse) Available() bool {
	if !use.Dependency.Available() || !use.Expression.Available() || use.Path == "" || !use.Occurrence.Available() || !use.Child.Available() || !use.Denominator.Available() || use.Denominator.Relation() != use.Child {
		return false
	}
	seen := make(map[model.DependencyID]struct{}, len(use.Writers))
	for _, writer := range use.Writers {
		if !writer.Available() {
			return false
		}
		if _, duplicate := seen[writer.Dependency]; duplicate {
			return false
		}
		seen[writer.Dependency] = struct{}{}
	}
	return use.Cold == (len(use.Writers) == 0)
}

// Projection is the canonical relation dependency projection for one
// dependency.  Reads and Writes are immutable sorted sets represented as
// slices because the checker and the later certificate both need stable
// content-addressed order, not a runtime map.
type Projection struct {
	Dependency   model.DependencyID
	Expression   model.ExpressionID
	Reads        []plan.RelationRef
	Writes       []plan.RelationRef
	CompleteUses []CompleteUse
}

// Component is a proved SCC projection.  Members and Edges are canonical
// sorted sets; Cyclic is true for a multi-member component or a singleton
// self-loop.  No physical order is retained here.
type Component struct {
	Members []plan.DependencyRef
	Edges   []plan.DependencyEdge
	Cyclic  bool
}

// Proof is the immutable recurrence proof returned by Check.  It contains
// only logical projections needed by the independent certificate layer.  It
// deliberately has no scheduler, physical ordinal, or WTO order.
type Proof struct {
	projections   []Projection
	edges         []plan.DependencyEdge
	components    []Component
	wideningHeads []plan.WideningHead
	completeUses  []CompleteUse
	valid         bool
}

// Available reports whether Check produced this proof.  An empty valid
// schema is distinct from the zero proof and remains a legitimate result.
func (proof Proof) Available() bool { return proof.valid }

// Projections returns defensive copies in dependency identity order.
func (proof Proof) Projections() []Projection {
	result := make([]Projection, len(proof.projections))
	for index, value := range proof.projections {
		result[index] = Projection{
			Dependency:   value.Dependency,
			Expression:   value.Expression,
			Reads:        append([]plan.RelationRef(nil), value.Reads...),
			Writes:       append([]plan.RelationRef(nil), value.Writes...),
			CompleteUses: cloneCompleteUses(value.CompleteUses),
		}
	}
	return result
}

// CompleteUses returns defensive copies of every checked Complete occurrence
// in canonical dependency/occurrence order.  The proof owns the original
// evidence; callers cannot mutate its writer or cold proof.
func (proof Proof) CompleteUses() []CompleteUse {
	return cloneCompleteUses(proof.completeUses)
}

func cloneCompleteUses(values []CompleteUse) []CompleteUse {
	if values == nil {
		return nil
	}
	result := make([]CompleteUse, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Writers = append([]CompleteWriter(nil), value.Writers...)
	}
	return result
}

// Edges returns defensive copies in producer/consumer identity order.
func (proof Proof) Edges() []plan.DependencyEdge {
	return append([]plan.DependencyEdge(nil), proof.edges...)
}

// Components returns defensive copies in canonical member-set order.
func (proof Proof) Components() []Component {
	result := make([]Component, len(proof.components))
	for index, value := range proof.components {
		result[index] = Component{
			Members: append([]plan.DependencyRef(nil), value.Members...),
			Edges:   append([]plan.DependencyEdge(nil), value.Edges...),
			Cyclic:  value.Cyclic,
		}
	}
	return result
}

// WideningHeads returns the canonical set of recurrence heads that passed
// SCC membership, positivity, uniqueness, and output-relation validation.
// The returned slice is defensive; mount consumes this projection directly
// and never correlates raw SCC declarations.
func (proof Proof) WideningHeads() []plan.WideningHead {
	return append([]plan.WideningHead(nil), proof.wideningHeads...)
}

// Check derives and verifies the complete logical recurrence projection.
// The plan is an unchecked artifact by design; every declaration and
// cross-reference is therefore checked here.  On any refusal the returned
// proof is zero and the deterministic first Error is non-nil.
func Check(schema plan.ExecutionSchema) (Proof, error) {
	indexed := checkregistry.Build(schema)
	if err := structuralRefusal(indexed); err != nil {
		return Proof{}, err
	}
	return CheckView(indexed)
}

// CheckView runs recurrence proof construction against an already indexed
// schema. It is the composition surface used by certificate construction so
// dependency projections and graph edges have exactly one registry owner.
func CheckView(indexed *checkregistry.View) (Proof, error) {
	if indexed == nil {
		indexed = checkregistry.Build(plan.ExecutionSchema{})
	}
	// Structural findings are owned by registry/certificate. A direct caller
	// that bypasses that gate receives no recurrence refusal and, importantly,
	// no usable proof for an invalid view.
	if !indexed.Valid() {
		return Proof{}, nil
	}
	ids := indexed.DependencyIDs()
	projections := make([]Projection, 0, len(ids))
	derivedByID := make(map[model.DependencyID]Projection, len(ids))
	for _, id := range ids {
		dependency, ok := indexed.Dependency(id)
		if !ok {
			return Proof{}, refusalFor(CodeInvalidDependency, id, "dependency is absent from the shared registry")
		}
		entry, ok := indexed.Expression(dependency.Expression())
		if !ok {
			return Proof{}, refusalFor(CodeMissingExpression, id, "dependency expression is absent from the expression registry")
		}
		projection, walkErr := deriveExpression(entry.Expression(), dependency.Expression(), indexed)
		if walkErr != nil {
			walkErr.Dependency = id
			return Proof{}, walkErr
		}
		projection.Dependency = id
		projection.Expression = dependency.Expression()
		if err := validateDeclaredProjection(dependency, projection); err != nil {
			return Proof{}, err
		}
		projections = append(projections, projection)
		derivedByID[id] = projection
	}

	derivedEdges := deriveEdges(ids, derivedByID)
	wideningHeads, err := validateComponents(indexed.SCCs(), ids, indexed, derivedByID, derivedEdges)
	if err != nil {
		return Proof{}, err
	}
	components, componentErr := canonicalComponents(indexed.SCCs(), ids, derivedEdges)
	if componentErr != nil {
		return Proof{}, componentErr
	}
	completeUses, completeErr := validateCompleteUses(ids, derivedByID, components, derivedEdges)
	if completeErr != nil {
		return Proof{}, completeErr
	}
	for index := range projections {
		projections[index].CompleteUses = completeUsesForDependency(completeUses, projections[index].Dependency)
		derivedByID[projections[index].Dependency] = projections[index]
	}
	return Proof{projections: projections, edges: derivedEdges, components: components, wideningHeads: wideningHeads, completeUses: completeUses, valid: true}, nil
}

func structuralRefusal(indexed *checkregistry.View) *Error {
	for _, issue := range indexed.Issues() {
		switch issue.Code {
		case checkregistry.CodeSchemaUnavailable, checkregistry.CodeSchemaIdentityUnavailable:
			return refusal(CodeUnavailableSchema, issue.Detail)
		case checkregistry.CodeExpressionDuplicate:
			return refusal(CodeDuplicateExpression, issue.Detail)
		case checkregistry.CodeExpressionUnavailable, checkregistry.CodeExpressionNil, checkregistry.CodeExpressionDigest:
			return refusal(CodeInvalidExpression, issue.Detail)
		case checkregistry.CodeSignatureDuplicate:
			return refusal(CodeDuplicateSignature, issue.Detail)
		case checkregistry.CodeSignatureUnavailable:
			return refusal(CodeInvalidSignature, issue.Detail)
		case checkregistry.CodeDependencyDuplicate:
			return refusal(CodeDuplicateDependency, issue.Detail)
		case checkregistry.CodeDependencyUnavailable:
			return refusal(CodeInvalidDependency, issue.Detail)
		case checkregistry.CodeSCCUnavailable:
			return refusal(CodeInvalidDependency, issue.Detail)
		case checkregistry.CodeRelationUnavailable, checkregistry.CodeColumnUnavailable,
			checkregistry.CodeKeyUnavailable, checkregistry.CodeScopeUnavailable,
			checkregistry.CodeRelationDuplicate, checkregistry.CodeColumnDuplicate,
			checkregistry.CodeKeyDuplicate, checkregistry.CodeScopeDuplicate:
			return refusal(CodeInvalidRelation, issue.Detail)
		}
	}
	return nil
}

// Validate is the narrow convenience surface used by admission gates that
// need only a Boolean.  Check remains the sole proof-producing authority.
func Validate(schema plan.ExecutionSchema) bool {
	_, err := Check(schema)
	return err == nil
}

func refusal(code Code, detail string) *Error {
	return &Error{Code: code, Detail: detail}
}

func refusalFor(code Code, dependency model.DependencyID, detail string) *Error {
	return &Error{Code: code, Dependency: dependency, Detail: detail}
}

type relationSet map[model.RelationID]struct{}

type expressionProjection struct {
	reads  relationSet
	writes relationSet
}

func emptyProjection() expressionProjection {
	return expressionProjection{reads: make(relationSet), writes: make(relationSet)}
}

// deriveCompleteUses performs the occurrence walk separately from the
// read/write projection.  Keeping this identity walk independent means the
// existing relation projection remains a set while Complete occurrences keep
// their authored child path and denominator evidence.
func deriveCompleteUses(expression algebra.Expression, root model.ExpressionID, indexed *checkregistry.View) ([]CompleteUse, *Error) {
	if expression == nil {
		return nil, refusal(CodeInvalidExpression, "nil expression child")
	}
	entryRoot := root
	uses := make([]CompleteUse, 0)
	var walk func(algebra.Expression, string) *Error
	walk = func(node algebra.Expression, path string) *Error {
		if node == nil {
			return refusal(CodeInvalidExpression, "nil expression child")
		}
		switch value := node.(type) {
		case algebra.Complete:
			child, ok := expressionRelation(value.Child(), indexed)
			if !ok {
				return refusal(CodeInvalidRelation, "Complete child relation is unavailable")
			}
			denominator := value.Denominator()
			if !denominator.Available() || !denominator.Relation().Available() || denominator.Relation() != child {
				return refusal(CodeInvalidRelation, "Complete denominator does not identify its child relation")
			}
			uses = append(uses, CompleteUse{
				Expression:  entryRoot,
				Path:        path,
				Occurrence:  occurrenceIdentity(entryRoot, path),
				Child:       child,
				Denominator: denominator,
			})
			return walk(value.Child(), path+".child")
		case *algebra.Complete:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil complete expression")
			}
			return walk(*value, path)
		case algebra.Select:
			return walk(value.Child(), path+".child")
		case *algebra.Select:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil select expression")
			}
			return walk(value.Child(), path+".child")
		case algebra.Project:
			return walk(value.Child(), path+".child")
		case *algebra.Project:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil project expression")
			}
			return walk(value.Child(), path+".child")
		case algebra.ColumnProject:
			return walk(value.Child(), path+".child")
		case *algebra.ColumnProject:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil column-project expression")
			}
			return walk(value.Child(), path+".child")
		case algebra.Join:
			if err := walk(value.Left(), path+".left"); err != nil {
				return err
			}
			return walk(value.Right(), path+".right")
		case *algebra.Join:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil join expression")
			}
			return walk(*value, path)
		case algebra.Expand:
			return walk(value.Child(), path+".child")
		case *algebra.Expand:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil expand expression")
			}
			return walk(value.Child(), path+".child")
		case algebra.Merge:
			for index, childExpression := range value.Inputs() {
				if err := walk(childExpression, fmt.Sprintf("%s.input[%d]", path, index)); err != nil {
					return err
				}
			}
			return nil
		case *algebra.Merge:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil merge expression")
			}
			return walk(*value, path)
		case algebra.Group:
			return walk(value.Child(), path+".child")
		case *algebra.Group:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil group expression")
			}
			return walk(value.Child(), path+".child")
		case algebra.Apply:
			for index, childExpression := range value.Inputs() {
				if err := walk(childExpression, fmt.Sprintf("%s.child[%d]", path, index)); err != nil {
					return err
				}
			}
			return nil
		case *algebra.Apply:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil apply expression")
			}
			return walk(*value, path)
		case algebra.Publish:
			return walk(value.Child(), path+".child")
		case *algebra.Publish:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil publish expression")
			}
			return walk(*value, path)
		case algebra.Input:
			return nil
		case *algebra.Input:
			if value == nil {
				return refusal(CodeInvalidExpression, "nil input expression")
			}
			return nil
		default:
			return refusal(CodeInvalidExpression, fmt.Sprintf("unrecognized logical expression kind %d", node.Kind()))
		}
	}
	if err := walk(expression, "root"); err != nil {
		return nil, err
	}
	return uses, nil
}

// expressionRelation is the checker-owned nominal output relation walk used
// only for Complete's child relation. It mirrors the closed algebra shape:
// Project/Publish/Apply establish their declared destination, Join keeps its
// left relation, and row-preserving operators retain their child relation.
func expressionRelation(expression algebra.Expression, indexed *checkregistry.View) (model.RelationID, bool) {
	if expression == nil {
		return model.RelationID{}, false
	}
	switch value := expression.(type) {
	case algebra.Input:
		return value.Relation(), value.Relation().Available()
	case *algebra.Input:
		if value == nil {
			return model.RelationID{}, false
		}
		return value.Relation(), value.Relation().Available()
	case algebra.Select:
		return expressionRelation(value.Child(), indexed)
	case *algebra.Select:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(value.Child(), indexed)
	case algebra.Project:
		return value.Contract().Target(), value.Contract().Target().Available()
	case *algebra.Project:
		if value == nil {
			return model.RelationID{}, false
		}
		return value.Contract().Target(), value.Contract().Target().Available()
	case algebra.ColumnProject:
		return expressionRelation(value.Child(), indexed)
	case *algebra.ColumnProject:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(value.Child(), indexed)
	case algebra.Join:
		return expressionRelation(value.Left(), indexed)
	case *algebra.Join:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(value.Left(), indexed)
	case algebra.Expand:
		return expressionRelation(value.Child(), indexed)
	case *algebra.Expand:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(value.Child(), indexed)
	case algebra.Merge:
		inputs := value.Inputs()
		if len(inputs) == 0 {
			return model.RelationID{}, false
		}
		return expressionRelation(inputs[0], indexed)
	case *algebra.Merge:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(*value, indexed)
	case algebra.Group:
		return expressionRelation(value.Child(), indexed)
	case *algebra.Group:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(value.Child(), indexed)
	case algebra.Complete:
		return expressionRelation(value.Child(), indexed)
	case *algebra.Complete:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(value.Child(), indexed)
	case algebra.Apply:
		operation, ok := indexed.Signature(value.Contract().Operation())
		if !ok || len(operation.Outputs()) == 0 {
			return model.RelationID{}, false
		}
		return operation.Outputs()[0].Relation, operation.Outputs()[0].Relation.Available()
	case *algebra.Apply:
		if value == nil {
			return model.RelationID{}, false
		}
		return expressionRelation(*value, indexed)
	case algebra.Publish:
		return value.Contract().Destination(), value.Contract().Destination().Available()
	case *algebra.Publish:
		if value == nil {
			return model.RelationID{}, false
		}
		return value.Contract().Destination(), value.Contract().Destination().Available()
	default:
		return model.RelationID{}, false
	}
}

func occurrenceIdentity(expression model.ExpressionID, path string) identity.ContentID {
	if !expression.Available() || path == "" {
		return identity.ContentID{}
	}
	return deriveContentID("analysis/relation/check/recurrence/complete-occurrence/v1",
		contentBytes(expression.Owner().Content()), contentBytes(expression.Content()), []byte(path))
}

func deriveExpression(expression algebra.Expression, root model.ExpressionID, indexed *checkregistry.View) (Projection, *Error) {
	projection, err := deriveNode(expression, indexed)
	if err != nil {
		return Projection{}, err
	}
	result := Projection{
		Reads:  relationRefs(projection.reads),
		Writes: relationRefs(projection.writes),
	}
	uses, useErr := deriveCompleteUses(expression, root, indexed)
	if useErr != nil {
		return Projection{}, useErr
	}
	result.CompleteUses = uses
	return result, nil
}

func deriveNode(expression algebra.Expression, indexed *checkregistry.View) (expressionProjection, *Error) {
	if expression == nil {
		return expressionProjection{}, refusal(CodeInvalidExpression, "nil expression child")
	}
	result := emptyProjection()
	switch value := expression.(type) {
	case algebra.Input:
		if !value.Relation().Available() {
			return result, refusal(CodeInvalidRelation, "input relation is unavailable")
		}
		result.reads[value.Relation()] = struct{}{}
		return result, nil
	case *algebra.Input:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil input expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Select:
		return deriveNode(value.Child(), indexed)
	case *algebra.Select:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil select expression")
		}
		return deriveNode(value.Child(), indexed)
	case algebra.Project:
		result, err := deriveNode(value.Child(), indexed)
		if err != nil {
			return result, err
		}
		target := value.Contract().Target()
		if !target.Available() {
			return result, refusal(CodeInvalidRelation, "project target relation is unavailable")
		}
		// Project reads the target relation's keyed destination to construct a
		// row-shaped value; it does not publish state. Publish is the sole
		// logical write boundary in this algebra.
		result.reads[target] = struct{}{}
		return result, nil
	case *algebra.Project:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil project expression")
		}
		return deriveNode(*value, indexed)
	case algebra.ColumnProject:
		// ColumnProject only retains cells from its child row. It establishes no
		// new read or write relation; Publish remains the sole write boundary.
		return deriveNode(value.Child(), indexed)
	case *algebra.ColumnProject:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil column-project expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Join:
		return deriveChildren([]algebra.Expression{value.Left(), value.Right()}, indexed)
	case *algebra.Join:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil join expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Expand:
		result, err := deriveNode(value.Child(), indexed)
		if err != nil {
			return result, err
		}
		contract := value.Contract()
		if !contract.Available() {
			return result, refusal(CodeInvalidRelation, "expand contract is unavailable")
		}
		// Expand's runtime footprint is the child candidate and the hot reader
		// only.  Publisher is cold owner evidence authenticated by the mount
		// catalogue; it must not become a recurrence edge or runtime wake.
		for _, relation := range []model.RelationID{contract.Candidate(), contract.Reader()} {
			if !relation.Available() {
				return result, refusal(CodeInvalidRelation, "expand dependency relation is unavailable")
			}
			result.reads[relation] = struct{}{}
		}
		return result, nil
	case *algebra.Expand:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil expand expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Merge:
		return deriveChildren(value.Inputs(), indexed)
	case *algebra.Merge:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil merge expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Group:
		return deriveNode(value.Child(), indexed)
	case *algebra.Group:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil group expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Complete:
		result, err := deriveNode(value.Child(), indexed)
		if err != nil {
			return result, err
		}
		denominator := value.Denominator()
		if !denominator.Available() || !denominator.Relation().Available() {
			return result, refusal(CodeInvalidRelation, "complete denominator relation is unavailable")
		}
		result.reads[denominator.Relation()] = struct{}{}
		return result, nil
	case *algebra.Complete:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil complete expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Apply:
		return deriveApply(value.Inputs(), value.Contract().Operation(), indexed)
	case *algebra.Apply:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil apply expression")
		}
		return deriveNode(*value, indexed)
	case algebra.Publish:
		result, err := deriveNode(value.Child(), indexed)
		if err != nil {
			return result, err
		}
		destination := value.Contract().Destination()
		if !destination.Available() {
			return result, refusal(CodeInvalidRelation, "publish destination relation is unavailable")
		}
		result.writes[destination] = struct{}{}
		return result, nil
	case *algebra.Publish:
		if value == nil {
			return result, refusal(CodeInvalidExpression, "nil publish expression")
		}
		return deriveNode(*value, indexed)
	default:
		return result, refusal(CodeInvalidExpression, fmt.Sprintf("unrecognized logical expression kind %d", expression.Kind()))
	}
}

func deriveChildren(children []algebra.Expression, indexed *checkregistry.View) (expressionProjection, *Error) {
	result := emptyProjection()
	for _, child := range children {
		projection, err := deriveNode(child, indexed)
		if err != nil {
			return result, err
		}
		union(result.reads, projection.reads)
		union(result.writes, projection.writes)
	}
	return result, nil
}

func deriveApply(children []algebra.Expression, operation signature.Identity, indexed *checkregistry.View) (expressionProjection, *Error) {
	result, err := deriveChildren(children, indexed)
	if err != nil {
		return result, err
	}
	_, ok := indexed.Signature(operation)
	if !ok {
		return result, refusal(CodeMissingSignature, "apply operation is absent from the signature registry")
	}
	// Apply is a semantic computation over its physical child expressions. The
	// signature is checked for validity above, but its input/output relations
	// are not additional state edges: child expressions are the only reads and
	// Publish is the sole state-write boundary. Treating signature outputs or
	// its publication authority as edges makes every publishing Apply appear
	// self-dependent and forces needless recurrence work.
	return result, nil
}

func union(target, source relationSet) {
	for value := range source {
		target[value] = struct{}{}
	}
}

func relationRefs(values relationSet) []plan.RelationRef {
	ids := make([]model.RelationID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return relationIDLess(ids[left], ids[right]) })
	result := make([]plan.RelationRef, 0, len(ids))
	for _, id := range ids {
		ref, ok := plan.NewRelationRef(id)
		if ok {
			result = append(result, ref)
		}
	}
	return result
}

func validateDeclaredProjection(dependency plan.Dependency, derived Projection) *Error {
	reads, err := declaredRelationSet(dependency.Reads(), dependency.ID())
	if err != nil {
		return err
	}
	writes, err := declaredRelationSet(dependency.Writes(), dependency.ID())
	if err != nil {
		return err
	}
	if !sameRelationSet(reads, derived.Reads) || !sameRelationSet(writes, derived.Writes) {
		return refusalFor(CodeProjectionMismatch, dependency.ID(), "declared read/write projection differs from the expression registry")
	}
	return nil
}

func declaredRelationSet(values []plan.RelationRef, dependency model.DependencyID) (relationSet, *Error) {
	result := make(relationSet, len(values))
	for _, ref := range values {
		id := ref.ID()
		if !ref.Available() || !id.Available() {
			return nil, refusalFor(CodeInvalidRelation, dependency, "declared relation reference is unavailable")
		}
		if _, exists := result[id]; exists {
			return nil, refusalFor(CodeDuplicateProjection, dependency, "declared relation projection contains a duplicate")
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func sameRelationSet(left relationSet, right []plan.RelationRef) bool {
	if len(left) != len(right) {
		return false
	}
	for _, ref := range right {
		if _, ok := left[ref.ID()]; !ok {
			return false
		}
	}
	return true
}

func deriveEdges(ids []model.DependencyID, projections map[model.DependencyID]Projection) []plan.DependencyEdge {
	edges := make([]plan.DependencyEdge, 0)
	for _, producer := range ids {
		writes := relationSetFromRefs(projections[producer].Writes)
		for _, consumer := range ids {
			reads := relationSetFromRefs(projections[consumer].Reads)
			if intersects(writes, reads) {
				from := plan.DefineDependencyRef(producer)
				to := plan.DefineDependencyRef(consumer)
				edges = append(edges, plan.DefineDependencyEdge(from, to))
			}
		}
	}
	sort.Slice(edges, func(left, right int) bool { return edgeLess(edges[left], edges[right]) })
	return edges
}

func relationSetFromRefs(values []plan.RelationRef) relationSet {
	result := make(relationSet, len(values))
	for _, value := range values {
		result[value.ID()] = struct{}{}
	}
	return result
}

func intersects(left, right relationSet) bool {
	if len(left) > len(right) {
		left, right = right, left
	}
	for id := range left {
		if _, ok := right[id]; ok {
			return true
		}
	}
	return false
}

func validateComponents(declared []plan.SCC, ids []model.DependencyID, registry *checkregistry.View, projections map[model.DependencyID]Projection, edges []plan.DependencyEdge) ([]plan.WideningHead, *Error) {
	expected := stronglyConnected(ids, edges)
	seen := make(map[string]struct{}, len(declared))
	ordered := append([]plan.SCC(nil), declared...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return componentKey(ordered[left].Members()) < componentKey(ordered[right].Members())
	})
	wideningHeads := make([]plan.WideningHead, 0)
	for _, component := range ordered {
		key := componentKey(component.Members())
		if _, duplicate := seen[key]; duplicate {
			return nil, refusal(CodeDuplicateComponent, "SCC member set occurs more than once")
		}
		seen[key] = struct{}{}
		members, memberErr := validateMembers(component.Members(), registry)
		if memberErr != nil {
			return nil, memberErr
		}
		expectedComponent, ok := expected[key]
		if !ok {
			return nil, refusal(CodeSpuriousComponent, "declared SCC is not a component of the derived dependency graph")
		}
		if !sameDependencySet(members, expectedComponent.members) {
			return nil, refusal(CodeSpuriousComponent, "declared SCC membership differs from derived component")
		}
		if err := validateComponentEdges(component, members, expectedComponent.edges); err != nil {
			return nil, err
		}
		if err := validateRecurrence(component, members, expectedComponent.cyclic, projections); err != nil {
			return nil, err
		}
		wideningHeads = append(wideningHeads, component.Recurrence().Heads()...)
	}
	if len(seen) != len(expected) {
		return nil, refusal(CodeMissingComponent, "dependency graph has an undeclared SCC component")
	}
	sort.SliceStable(wideningHeads, func(left, right int) bool {
		leftDigest, rightDigest := wideningHeads[left].Digest(), wideningHeads[right].Digest()
		if comparison := bytes.Compare(leftDigest[:], rightDigest[:]); comparison != 0 {
			return comparison < 0
		}
		return wideningHeadKey(wideningHeads[left]) < wideningHeadKey(wideningHeads[right])
	})
	return wideningHeads, nil
}

type derivedComponent struct {
	members []plan.DependencyRef
	edges   []plan.DependencyEdge
	cyclic  bool
}

func stronglyConnected(ids []model.DependencyID, edges []plan.DependencyEdge) map[string]derivedComponent {
	adjacency := make(map[model.DependencyID][]model.DependencyID, len(ids))
	reverse := make(map[model.DependencyID][]model.DependencyID, len(ids))
	for _, id := range ids {
		adjacency[id] = nil
		reverse[id] = nil
	}
	for _, edge := range edges {
		from, to := edge.From().ID(), edge.To().ID()
		adjacency[from] = append(adjacency[from], to)
		reverse[to] = append(reverse[to], from)
	}
	for id := range adjacency {
		sort.Slice(adjacency[id], func(left, right int) bool { return dependencyIDLess(adjacency[id][left], adjacency[id][right]) })
		sort.Slice(reverse[id], func(left, right int) bool { return dependencyIDLess(reverse[id][left], reverse[id][right]) })
	}
	visited := make(map[model.DependencyID]bool, len(ids))
	finish := make([]model.DependencyID, 0, len(ids))
	for _, start := range ids {
		if visited[start] {
			continue
		}
		visited[start] = true
		stack := []walkFrame{{id: start}}
		for len(stack) > 0 {
			last := len(stack) - 1
			frame := &stack[last]
			if frame.next < len(adjacency[frame.id]) {
				next := adjacency[frame.id][frame.next]
				frame.next++
				if !visited[next] {
					visited[next] = true
					stack = append(stack, walkFrame{id: next})
				}
				continue
			}
			finish = append(finish, frame.id)
			stack = stack[:last]
		}
	}
	assigned := make(map[model.DependencyID]bool, len(ids))
	result := make(map[string]derivedComponent, len(ids))
	for index := len(finish) - 1; index >= 0; index-- {
		start := finish[index]
		if assigned[start] {
			continue
		}
		members := make([]model.DependencyID, 0)
		stack := []model.DependencyID{start}
		assigned[start] = true
		for len(stack) > 0 {
			last := len(stack) - 1
			id := stack[last]
			stack = stack[:last]
			members = append(members, id)
			for _, previous := range reverse[id] {
				if !assigned[previous] {
					assigned[previous] = true
					stack = append(stack, previous)
				}
			}
		}
		sort.Slice(members, func(left, right int) bool { return dependencyIDLess(members[left], members[right]) })
		memberRefs := make([]plan.DependencyRef, 0, len(members))
		memberSet := make(map[model.DependencyID]struct{}, len(members))
		for _, id := range members {
			memberRefs = append(memberRefs, plan.DefineDependencyRef(id))
			memberSet[id] = struct{}{}
		}
		componentEdges := make([]plan.DependencyEdge, 0)
		selfLoop := false
		for _, edge := range edges {
			from, to := edge.From().ID(), edge.To().ID()
			if _, fromOK := memberSet[from]; fromOK {
				if _, toOK := memberSet[to]; toOK {
					componentEdges = append(componentEdges, edge)
					selfLoop = selfLoop || from == to
				}
			}
		}
		sort.Slice(componentEdges, func(left, right int) bool { return edgeLess(componentEdges[left], componentEdges[right]) })
		result[componentKey(memberRefs)] = derivedComponent{members: memberRefs, edges: componentEdges, cyclic: len(members) > 1 || selfLoop}
	}
	return result
}

type walkFrame struct {
	id   model.DependencyID
	next int
}

func validateMembers(values []plan.DependencyRef, registry *checkregistry.View) ([]plan.DependencyRef, *Error) {
	seen := make(map[model.DependencyID]struct{}, len(values))
	result := make([]plan.DependencyRef, 0, len(values))
	for _, value := range values {
		id := value.ID()
		if !value.Available() || !id.Available() {
			return nil, refusal(CodeInvalidDependency, "SCC member identity is unavailable")
		}
		if _, ok := registry.Dependency(id); !ok {
			return nil, refusal(CodeSpuriousComponent, "SCC member is not a declared dependency")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, refusal(CodeDuplicateComponent, "SCC contains a duplicate member")
		}
		seen[id] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return dependencyRefLess(result[left], result[right]) })
	return result, nil
}

func validateComponentEdges(declared plan.SCC, members []plan.DependencyRef, expected []plan.DependencyEdge) *Error {
	memberSet := make(map[model.DependencyID]struct{}, len(members))
	for _, member := range members {
		memberSet[member.ID()] = struct{}{}
	}
	seen := make(map[string]struct{}, len(declared.Edges()))
	actual := make([]plan.DependencyEdge, 0, len(declared.Edges()))
	for _, edge := range declared.Edges() {
		from, to := edge.From().ID(), edge.To().ID()
		if !edge.Available() || !edge.From().Available() || !edge.To().Available() {
			return refusal(CodeSpuriousEdge, "SCC edge endpoint is unavailable")
		}
		if _, ok := memberSet[from]; !ok {
			return refusal(CodeSpuriousEdge, "SCC edge source is outside its component")
		}
		if _, ok := memberSet[to]; !ok {
			return refusal(CodeSpuriousEdge, "SCC edge destination is outside its component")
		}
		key := edgeKey(edge)
		if _, duplicate := seen[key]; duplicate {
			return refusal(CodeDuplicateEdge, "SCC contains a duplicate edge")
		}
		seen[key] = struct{}{}
		actual = append(actual, edge)
	}
	sort.Slice(actual, func(left, right int) bool { return edgeLess(actual[left], actual[right]) })
	if len(actual) != len(expected) {
		if len(actual) < len(expected) {
			return refusal(CodeMissingEdge, "SCC omits a derived internal edge")
		}
		return refusal(CodeSpuriousEdge, "SCC declares an edge absent from the derived graph")
	}
	for index := range expected {
		if edgeKey(actual[index]) != edgeKey(expected[index]) {
			return refusal(CodeSpuriousEdge, "SCC edge set differs from the derived graph")
		}
	}
	return nil
}

func validateRecurrence(component plan.SCC, members []plan.DependencyRef, cyclic bool, projections map[model.DependencyID]Projection) *Error {
	recurrence := component.Recurrence()
	if !recurrence.Available() {
		return refusal(CodeRecurrencePolicy, "recurrence kind is unavailable")
	}
	if cyclic && recurrence.Kind() != plan.Positive {
		return refusal(CodeRecurrencePolicy, "cyclic component is not declared positive")
	}
	if !cyclic && recurrence.Kind() != plan.Acyclic {
		return refusal(CodeRecurrencePolicy, "acyclic component carries a positive recurrence policy")
	}
	memberSet := make(map[model.DependencyID]struct{}, len(members))
	for _, member := range members {
		memberSet[member.ID()] = struct{}{}
	}
	seenHeads := make(map[string]struct{}, len(recurrence.Heads()))
	for _, head := range recurrence.Heads() {
		if !head.Available() || !head.Dependency().Available() || !head.Relation().Available() {
			return refusal(CodeInvalidWideningHead, "widening head identity is unavailable")
		}
		dependency := head.Dependency().ID()
		if _, ok := memberSet[dependency]; !ok {
			return refusal(CodeInvalidWideningHead, "widening head dependency is outside its SCC")
		}
		key := wideningHeadKey(head)
		if _, duplicate := seenHeads[key]; duplicate {
			return refusal(CodeDuplicateWideningHead, "widening head occurs more than once")
		}
		seenHeads[key] = struct{}{}
		if !containsRelation(projections[dependency].Writes, head.Relation().ID()) {
			return refusal(CodeInvalidWideningHead, "widening head relation is not an output relation of its dependency")
		}
	}
	if !cyclic && len(recurrence.Heads()) != 0 {
		return refusal(CodeInvalidWideningHead, "acyclic component declares a widening head")
	}
	return nil
}

func containsRelation(values []plan.RelationRef, target model.RelationID) bool {
	for _, value := range values {
		if value.ID() == target {
			return true
		}
	}
	return false
}

func canonicalComponents(declared []plan.SCC, ids []model.DependencyID, edges []plan.DependencyEdge) ([]Component, error) {
	_ = ids
	_ = edges
	ordered := append([]plan.SCC(nil), declared...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return componentKey(ordered[left].Members()) < componentKey(ordered[right].Members())
	})
	result := make([]Component, 0, len(ordered))
	for _, value := range ordered {
		members := append([]plan.DependencyRef(nil), value.Members()...)
		edges := append([]plan.DependencyEdge(nil), value.Edges()...)
		sort.Slice(members, func(left, right int) bool { return dependencyRefLess(members[left], members[right]) })
		sort.Slice(edges, func(left, right int) bool { return edgeLess(edges[left], edges[right]) })
		cyclic := len(members) > 1
		for _, edge := range edges {
			if edge.From().ID() == edge.To().ID() {
				cyclic = true
			}
		}
		result = append(result, Component{Members: members, Edges: edges, Cyclic: cyclic})
	}
	return result, nil
}

// validateCompleteUses closes the Complete evidence over the derived
// dependency graph.  Writers are computed from the checked write projection,
// never from declarations supplied by a physical consumer.  The component
// ranks are the same deterministic topological order used by mount's cold
// schedule; independent components are ordered by their canonical member key.
func validateCompleteUses(ids []model.DependencyID, projections map[model.DependencyID]Projection, components []Component, edges []plan.DependencyEdge) ([]CompleteUse, *Error) {
	ranks, componentByDependency, ok := componentRanks(components, edges)
	if !ok {
		return nil, refusal(CodeRecurrencePolicy, "SCC condensation has no strict topological order")
	}
	result := make([]CompleteUse, 0)
	for _, dependency := range ids {
		projection, projectionOK := projections[dependency]
		if !projectionOK {
			return nil, refusalFor(CodeInvalidDependency, dependency, "Complete projection owner is absent")
		}
		ownerComponent, ownerOK := componentByDependency[dependency]
		ownerRank, ownerRankOK := ranks[ownerComponent]
		if !ownerOK || !ownerRankOK {
			return nil, refusalFor(CodeInvalidDependency, dependency, "Complete projection owner has no SCC component")
		}
		for _, declared := range projection.CompleteUses {
			use := declared
			use.Dependency = dependency
			if !use.Expression.Available() || use.Expression != projection.Expression {
				return nil, refusalFor(CodeInvalidExpression, dependency, "Complete occurrence expression identity is unavailable")
			}
			if use.Path == "" || !use.Occurrence.Available() || !use.Child.Available() || !use.Denominator.Available() || use.Denominator.Relation() != use.Child {
				return nil, refusalFor(CodeInvalidRelation, dependency, "Complete occurrence evidence is unavailable")
			}
			writers := make([]CompleteWriter, 0)
			for _, writerID := range ids {
				writerProjection := projections[writerID]
				if !containsRelation(writerProjection.Writes, use.Child) {
					continue
				}
				writerComponent, writerOK := componentByDependency[writerID]
				writerRank, writerRankOK := ranks[writerComponent]
				if !writerOK || !writerRankOK {
					return nil, refusalFor(CodeInvalidDependency, dependency, "Complete writer has no SCC component")
				}
				earlier := writerRank < ownerRank
				writers = append(writers, CompleteWriter{Dependency: writerID, Component: writerRank, Earlier: earlier})
				if !earlier {
					return nil, &Error{
						Code:       CodeCompleteStratification,
						Dependency: dependency,
						Other:      writerID,
						Relation:   use.Child,
						Occurrence: use.Occurrence,
						Detail:     "Complete writer is not in a strictly earlier SCC component",
					}
				}
			}
			sort.Slice(writers, func(left, right int) bool {
				return dependencyIDLess(writers[left].Dependency, writers[right].Dependency)
			})
			use.Writers = writers
			use.Cold = len(writers) == 0
			if !use.Available() || !use.Digest().Available() {
				return nil, refusalFor(CodeInvalidRelation, dependency, "Complete occurrence proof is not canonical")
			}
			result = append(result, use)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Dependency != result[right].Dependency {
			return dependencyIDLess(result[left].Dependency, result[right].Dependency)
		}
		if bytes.Compare(result[left].Occurrence[:], result[right].Occurrence[:]) != 0 {
			return bytes.Compare(result[left].Occurrence[:], result[right].Occurrence[:]) < 0
		}
		return result[left].Path < result[right].Path
	})
	return result, nil
}

func completeUsesForDependency(values []CompleteUse, dependency model.DependencyID) []CompleteUse {
	result := make([]CompleteUse, 0)
	for _, value := range values {
		if value.Dependency == dependency {
			result = append(result, value)
		}
	}
	return cloneCompleteUses(result)
}

func componentRanks(components []Component, edges []plan.DependencyEdge) (map[int]uint32, map[model.DependencyID]int, bool) {
	componentByDependency := make(map[model.DependencyID]int)
	keys := make([]string, len(components))
	for index, component := range components {
		members := component.Members
		refs := append([]plan.DependencyRef(nil), members...)
		keys[index] = componentKey(refs)
		for _, member := range members {
			if !member.Available() {
				return nil, nil, false
			}
			if _, duplicate := componentByDependency[member.ID()]; duplicate {
				return nil, nil, false
			}
			componentByDependency[member.ID()] = index
		}
	}
	adjacency := make(map[int]map[int]struct{}, len(components))
	indegree := make([]int, len(components))
	for index := range components {
		adjacency[index] = make(map[int]struct{})
	}
	for _, edge := range edges {
		from, fromOK := componentByDependency[edge.From().ID()]
		to, toOK := componentByDependency[edge.To().ID()]
		if !fromOK || !toOK {
			return nil, nil, false
		}
		if from == to {
			continue
		}
		if _, duplicate := adjacency[from][to]; duplicate {
			continue
		}
		adjacency[from][to] = struct{}{}
		indegree[to]++
	}
	ready := make([]int, 0, len(components))
	for index, degree := range indegree {
		if degree == 0 {
			ready = append(ready, index)
		}
	}
	less := func(left, right int) bool { return keys[left] < keys[right] }
	result := make(map[int]uint32, len(components))
	for len(ready) != 0 {
		sort.Slice(ready, func(left, right int) bool { return less(ready[left], ready[right]) })
		current := ready[0]
		ready = ready[1:]
		result[current] = uint32(len(result))
		for next := range adjacency[current] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	return result, componentByDependency, len(result) == len(components)
}

func sameDependencySet(left, right []plan.DependencyRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID() != right[index].ID() {
			return false
		}
	}
	return true
}

func relationIDLess(left, right model.RelationID) bool {
	if compareContent(left.Owner().Content(), right.Owner().Content()) != 0 {
		return compareContent(left.Owner().Content(), right.Owner().Content()) < 0
	}
	return compareContent(left.Content(), right.Content()) < 0
}

func dependencyIDLess(left, right model.DependencyID) bool {
	if compareContent(left.Owner().Content(), right.Owner().Content()) != 0 {
		return compareContent(left.Owner().Content(), right.Owner().Content()) < 0
	}
	return compareContent(left.Content(), right.Content()) < 0
}

func dependencyRefLess(left, right plan.DependencyRef) bool {
	return dependencyIDLess(left.ID(), right.ID())
}

func edgeLess(left, right plan.DependencyEdge) bool {
	if left.From().ID() != right.From().ID() {
		return dependencyIDLess(left.From().ID(), right.From().ID())
	}
	return dependencyIDLess(left.To().ID(), right.To().ID())
}

func edgeKey(value plan.DependencyEdge) string {
	var result bytes.Buffer
	fromOwner := value.From().ID().Owner().Content()
	fromContent := value.From().ID().Content()
	toOwner := value.To().ID().Owner().Content()
	toContent := value.To().ID().Content()
	result.Write(fromOwner[:])
	result.Write(fromContent[:])
	result.Write(toOwner[:])
	result.Write(toContent[:])
	return result.String()
}

func componentKey(values []plan.DependencyRef) string {
	ordered := append([]plan.DependencyRef(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return dependencyRefLess(ordered[left], ordered[right]) })
	var result bytes.Buffer
	for _, value := range ordered {
		owner := value.ID().Owner().Content()
		content := value.ID().Content()
		result.Write(owner[:])
		result.Write(content[:])
	}
	return result.String()
}

func wideningHeadKey(value plan.WideningHead) string {
	var result bytes.Buffer
	owner := value.Dependency().ID().Owner().Content()
	content := value.Dependency().ID().Content()
	relationOwner := value.Relation().ID().Owner().Content()
	relationContent := value.Relation().ID().Content()
	result.Write(owner[:])
	result.Write(content[:])
	result.Write(relationOwner[:])
	result.Write(relationContent[:])
	return result.String()
}

func compareContent(left, right [32]byte) int { return bytes.Compare(left[:], right[:]) }
