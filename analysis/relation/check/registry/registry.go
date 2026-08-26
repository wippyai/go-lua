package registry

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Code identifies a structural defect found while indexing one execution
// schema.  The proof passes map these few shared defects into their own
// diagnostic vocabularies; declaration and semantic laws remain owned by the
// pass that proves them.
type Code uint8

const (
	CodeSchemaUnavailable Code = iota + 1
	CodeSchemaIdentityUnavailable
	CodeRelationUnavailable
	CodeRelationDuplicate
	CodeColumnUnavailable
	CodeColumnDuplicate
	CodeKeyUnavailable
	CodeKeyDuplicate
	CodeScopeUnavailable
	CodeScopeDuplicate
	CodeExpressionUnavailable
	CodeExpressionDuplicate
	CodeExpressionNil
	CodeExpressionDigest
	CodeDependencyUnavailable
	CodeDependencyDuplicate
	CodeSignatureUnavailable
	CodeSignatureDuplicate
	CodeSCCUnavailable
	CodeTypeCapabilityUnavailable
	CodeTypeCapabilityDuplicate
	CodeObservationUnavailable
	CodeObservationDuplicate
	CodeInitialUnavailable
	CodeInitialDuplicate
	CodeContributionUnavailable
	CodeContributionDuplicate
	CodeContributionPort
	CodeContributionCapability
	CodeContributionPresence
)

func (code Code) String() string {
	names := [...]string{
		"Invalid", "SchemaUnavailable", "SchemaIdentityUnavailable",
		"RelationUnavailable", "RelationDuplicate", "ColumnUnavailable",
		"ColumnDuplicate", "KeyUnavailable", "KeyDuplicate", "ScopeUnavailable",
		"ScopeDuplicate", "ExpressionUnavailable", "ExpressionDuplicate",
		"ExpressionNil", "ExpressionDigest", "DependencyUnavailable",
		"DependencyDuplicate", "SignatureUnavailable", "SignatureDuplicate",
		"SCCUnavailable", "TypeCapabilityUnavailable", "TypeCapabilityDuplicate",
		"ObservationUnavailable", "ObservationDuplicate", "InitialUnavailable", "InitialDuplicate",
		"ContributionUnavailable", "ContributionDuplicate", "ContributionPort", "ContributionCapability", "ContributionPresence",
	}
	if int(code) < len(names) {
		return names[code]
	}
	return "Unknown"
}

// Issue is one shared structural defect.  Path uses the canonical identity
// vocabulary below and never depends on the source slice ordinal.
type Issue struct {
	Code   Code
	Path   string
	Detail string
}

// View is an immutable index over one unchecked ExecutionSchema.  The maps
// are private and every accessor returns a value or a defensive copy, so all
// checker passes observe the same declarations and canonical traversal.
type View struct {
	schema        plan.ExecutionSchema
	relations     map[model.RelationID]model.RelationSchema
	columns       map[model.ColumnID]model.ColumnSchema
	keys          map[model.KeyID]model.KeySchema
	scopes        map[model.ScopeID]model.ScopeSchema
	expressions   map[model.ExpressionID]plan.ExpressionRef
	dependencies  map[model.DependencyID]plan.Dependency
	signatures    map[signature.Identity]signature.Signature
	capabilities  map[model.TypeID]model.TypeCapability
	observations  map[identity.ContentID]algebra.ObservationContract
	initials      map[plan.Initial]plan.Initial
	contributions map[output.OutputPort]output.ContributionSpec
	sccs          []plan.SCC
	issues        []Issue
}

// Build indexes schema exactly once.  Invalid entries remain represented in
// the view where possible so a proof pass can continue to report its nearest
// law, while the structural defect itself is emitted only here.
func Build(schema plan.ExecutionSchema) *View {
	view := &View{
		schema:        schema,
		relations:     make(map[model.RelationID]model.RelationSchema),
		columns:       make(map[model.ColumnID]model.ColumnSchema),
		keys:          make(map[model.KeyID]model.KeySchema),
		scopes:        make(map[model.ScopeID]model.ScopeSchema),
		expressions:   make(map[model.ExpressionID]plan.ExpressionRef),
		dependencies:  make(map[model.DependencyID]plan.Dependency),
		signatures:    make(map[signature.Identity]signature.Signature),
		capabilities:  make(map[model.TypeID]model.TypeCapability),
		observations:  make(map[identity.ContentID]algebra.ObservationContract),
		initials:      make(map[plan.Initial]plan.Initial),
		contributions: make(map[output.OutputPort]output.ContributionSpec),
		sccs:          schema.SCCs(),
	}
	if !schema.Available() {
		view.add(CodeSchemaUnavailable, "schema", "execution schema digest is unavailable")
	}
	if !schema.SchemaID().Available() {
		view.add(CodeSchemaIdentityUnavailable, "schema", "schema identity is unavailable")
	}

	for _, value := range schema.Relations() {
		addDeclaration(view, CodeRelationUnavailable, CodeRelationDuplicate, RelationPath(value.ID()), value.Available(), value.ID(), view.relations, value)
	}
	for _, value := range schema.Columns() {
		addDeclaration(view, CodeColumnUnavailable, CodeColumnDuplicate, ColumnPath(value.ID()), value.Available(), value.ID(), view.columns, value)
	}
	for _, value := range schema.Keys() {
		addDeclaration(view, CodeKeyUnavailable, CodeKeyDuplicate, KeyPath(value.ID()), value.Available(), value.ID(), view.keys, value)
	}
	for _, value := range schema.Scopes() {
		addDeclaration(view, CodeScopeUnavailable, CodeScopeDuplicate, ScopePath(value.ID()), value.Available(), value.ID(), view.scopes, value)
	}
	for _, value := range schema.Expressions() {
		path := ExpressionPath(value.ID())
		if !value.Available() {
			view.add(CodeExpressionUnavailable, path, "expression registry entry is unavailable")
		} else if value.Expression() == nil {
			view.add(CodeExpressionNil, path, "expression node is nil")
		} else if value.Digest() != value.Expression().Digest() {
			view.add(CodeExpressionDigest, path, "expression registry digest does not match node")
		}
		if _, exists := view.expressions[value.ID()]; exists {
			view.add(CodeExpressionDuplicate, path, "duplicate expression identity")
		} else {
			view.expressions[value.ID()] = value
		}
	}
	for _, value := range schema.Dependencies() {
		path := DependencyPath(value.ID())
		if !value.Available() || !value.ID().Available() || !value.Expression().Available() {
			view.add(CodeDependencyUnavailable, path, "dependency declaration is unavailable")
		}
		if _, exists := view.dependencies[value.ID()]; exists {
			view.add(CodeDependencyDuplicate, path, "duplicate dependency identity")
		} else {
			view.dependencies[value.ID()] = value
		}
	}
	for _, value := range schema.Signatures() {
		path := SignaturePath(value.Identity())
		if !value.Available() || !value.Identity().Available() {
			view.add(CodeSignatureUnavailable, path, "semantic signature is unavailable")
		}
		if _, exists := view.signatures[value.Identity()]; exists {
			view.add(CodeSignatureDuplicate, path, "duplicate semantic signature identity")
		} else {
			view.signatures[value.Identity()] = value
		}
	}
	for _, value := range schema.Initials() {
		path := InitialPath(value)
		if !value.Available() {
			view.add(CodeInitialUnavailable, path, "initial declaration is unavailable")
		}
		if _, exists := view.initials[value]; exists {
			view.add(CodeInitialDuplicate, path, "duplicate initial declaration")
		} else {
			view.initials[value] = value
		}
	}
	for _, value := range schema.TypeCapabilities() {
		path := TypeCapabilityPath(value.Type())
		if !value.Available() {
			view.add(CodeTypeCapabilityUnavailable, path, "type capability is unavailable")
		}
		if _, exists := view.capabilities[value.Type()]; exists {
			view.add(CodeTypeCapabilityDuplicate, path, "duplicate type capability")
		} else {
			view.capabilities[value.Type()] = value
		}
	}
	for _, value := range schema.Observations() {
		path := ObservationPath(value.Digest())
		if !value.Available() {
			view.add(CodeObservationUnavailable, path, "observation descriptor is unavailable")
		}
		if _, exists := view.observations[value.Digest()]; exists {
			view.add(CodeObservationDuplicate, path, "duplicate observation descriptor identity")
		} else {
			view.observations[value.Digest()] = value
		}
	}
	for _, value := range schema.Contributions() {
		path := ContributionPath(value.Port())
		if !value.Available() {
			view.add(CodeContributionUnavailable, path, "contribution declaration is unavailable")
		}
		if _, exists := view.contributions[value.Port()]; exists {
			view.add(CodeContributionDuplicate, path, "duplicate contribution output port")
		} else {
			view.contributions[value.Port()] = value
		}
		if !value.Available() {
			continue
		}
		signatureValue, signatureOK := view.signatures[value.Port().Operation]
		if !signatureOK || !signatureValue.Available() {
			view.add(CodeContributionPort, path, "contribution port is not declared by a sealed signature")
			continue
		}
		declared, declaredOK := signatureValue.OutputFor(value.Port().Column.Relation(), value.Port().Column)
		if !declaredOK || !declared.Available() || declared.Type != value.ValueType() || declared.Presence != value.Presence() {
			view.add(CodeContributionPort, path, "contribution port, value type, or presence does not match its signature output")
		}
		if value.Presence() == signature.ProduceOptional || value.Presence() == signature.ProduceAbsent {
			view.add(CodeContributionPresence, path, "optional or absent contribution requires a closed-world producer denominator")
		}
		capability, capabilityOK := view.capabilities[value.ValueType()]
		if !capabilityOK || !capability.Available() || !capability.Equal(value.Algebra()) {
			view.add(CodeContributionCapability, path, "contribution capability does not match the schema type capability")
		}
	}
	for index, value := range view.sccs {
		if !value.Available() {
			view.add(CodeSCCUnavailable, fmt.Sprintf("scc[%d]", index), "SCC declaration is unavailable")
		}
	}
	view.sortIssues()
	return view
}

// Issues returns the deterministic structural findings produced by Build.
func (view *View) Issues() []Issue {
	if view == nil {
		return nil
	}
	return append([]Issue(nil), view.issues...)
}

// Valid reports whether the indexed schema has no shared structural defect.
// Proof passes may still reject a structurally valid view for their own laws.
func (view *View) Valid() bool { return view != nil && len(view.issues) == 0 }

func (view *View) add(code Code, path, detail string) {
	view.issues = append(view.issues, Issue{Code: code, Path: path, Detail: detail})
}

// addDeclaration is intentionally generic: declaration kinds differ only in
// their structural issue code and identity map.  Semantic membership remains
// in typing/authority and is not smuggled into this shared index.
func addDeclaration[ID comparable, Value any](view *View, unavailable, duplicate Code, path string, available bool, id ID, target map[ID]Value, value Value) {
	if !available {
		view.add(unavailable, path, "declaration is unavailable")
	}
	if _, exists := target[id]; exists {
		view.add(duplicate, path, "duplicate declaration identity")
		return
	}
	target[id] = value
}

// Schema returns the immutable source artifact.
func (view *View) Schema() plan.ExecutionSchema {
	if view == nil {
		return plan.ExecutionSchema{}
	}
	return view.schema
}

func (view *View) Relation(id model.RelationID) (model.RelationSchema, bool) {
	if view == nil {
		return model.RelationSchema{}, false
	}
	value, ok := view.relations[id]
	return value, ok
}

func (view *View) Column(id model.ColumnID) (model.ColumnSchema, bool) {
	if view == nil {
		return model.ColumnSchema{}, false
	}
	value, ok := view.columns[id]
	return value, ok
}

func (view *View) Key(id model.KeyID) (model.KeySchema, bool) {
	if view == nil {
		return model.KeySchema{}, false
	}
	value, ok := view.keys[id]
	return value, ok
}

// Denominator resolves one relation/key universe from the shared registry.
// Keeping this lookup here makes every checker pass use the same declaration
// index for source, operation, and observation destinations; no pass invents
// a second denominator catalogue.
func (view *View) Denominator(ref model.DenominatorRef) (model.RelationSchema, model.KeySchema, bool) {
	if view == nil || !ref.Available() {
		return model.RelationSchema{}, model.KeySchema{}, false
	}
	relation, relationOK := view.relations[ref.Relation()]
	key, keyOK := view.keys[ref.Key()]
	if !relationOK || !keyOK || !relation.Available() || !key.Available() || key.Relation() != ref.Relation() || !relation.HasKey(ref.Key()) {
		return model.RelationSchema{}, model.KeySchema{}, false
	}
	return relation, key, true
}

func (view *View) Scope(id model.ScopeID) (model.ScopeSchema, bool) {
	if view == nil {
		return model.ScopeSchema{}, false
	}
	value, ok := view.scopes[id]
	return value, ok
}

func (view *View) Expression(id model.ExpressionID) (plan.ExpressionRef, bool) {
	if view == nil {
		return plan.ExpressionRef{}, false
	}
	value, ok := view.expressions[id]
	return value, ok
}

func (view *View) Dependency(id model.DependencyID) (plan.Dependency, bool) {
	if view == nil {
		return plan.Dependency{}, false
	}
	value, ok := view.dependencies[id]
	return value, ok
}

func (view *View) Signature(id signature.Identity) (signature.Signature, bool) {
	if view == nil {
		return signature.Signature{}, false
	}
	value, ok := view.signatures[id]
	return value, ok
}

// TypeCapability resolves the explicit sealed policy for one TypeID.
func (view *View) TypeCapability(id model.TypeID) (model.TypeCapability, bool) {
	if view == nil {
		return model.TypeCapability{}, false
	}
	value, ok := view.capabilities[id]
	return value, ok
}

// TypeCapabilities returns the canonical capability catalogue.
func (view *View) TypeCapabilities() []model.TypeCapability {
	if view == nil {
		return nil
	}
	return view.schema.TypeCapabilities()
}

// Observation resolves one schema-sealed terminal descriptor by content
// identity. Runtime callers must use this mount-projected catalogue rather
// than supplying a fresh descriptor after seal.
func (view *View) Observation(id identity.ContentID) (algebra.ObservationContract, bool) {
	if view == nil || !id.Available() {
		return algebra.ObservationContract{}, false
	}
	value, ok := view.observations[id]
	return value, ok && value.Available() && value.Digest() == id
}

// Observations returns the canonical descriptor catalogue in digest order.
func (view *View) Observations() []algebra.ObservationContract {
	if view == nil {
		return nil
	}
	return view.schema.Observations()
}

// Initial resolves one schema-sealed zero-input invocation declaration.
func (view *View) Initial(value plan.Initial) (plan.Initial, bool) {
	if view == nil || !value.Available() {
		return plan.Initial{}, false
	}
	initial, ok := view.initials[value]
	return initial, ok && initial.Available()
}

// Initials returns the canonical immutable initial-invocation catalogue.
func (view *View) Initials() []plan.Initial {
	if view == nil {
		return nil
	}
	return view.schema.Initials()
}

// Contribution resolves one exact schema-authored output port.
func (view *View) Contribution(port output.OutputPort) (output.ContributionSpec, bool) {
	if view == nil || !port.Available() {
		return output.ContributionSpec{}, false
	}
	value, ok := view.contributions[port]
	return value, ok && value.Available()
}

// Contributions returns the canonical declaration vector retained by the
// indexed schema.
func (view *View) Contributions() []output.ContributionSpec {
	if view == nil {
		return nil
	}
	return view.schema.Contributions()
}

func (view *View) Relations() []model.RelationSchema {
	if view == nil {
		return nil
	}
	// ExecutionSchema.Build already sealed this catalogue in the canonical
	// relation order. Preserve that order here; rebuilding it from the map via
	// diagnostic paths would lose the owner/relation axes that plan sealing
	// intentionally orders first.
	return view.schema.Relations()
}

func (view *View) Columns() []model.ColumnSchema {
	if view == nil {
		return nil
	}
	return view.schema.Columns()
}

func (view *View) Keys() []model.KeySchema {
	if view == nil {
		return nil
	}
	return view.schema.Keys()
}

func (view *View) Scopes() []model.ScopeSchema {
	if view == nil {
		return nil
	}
	return view.schema.Scopes()
}

func (view *View) Expressions() []plan.ExpressionRef {
	if view == nil {
		return nil
	}
	return view.schema.Expressions()
}

func (view *View) Dependencies() []plan.Dependency {
	if view == nil {
		return nil
	}
	return view.schema.Dependencies()
}

func (view *View) Signatures() []signature.Signature {
	if view == nil {
		return nil
	}
	return view.schema.Signatures()
}

func (view *View) SCCs() []plan.SCC {
	if view == nil {
		return nil
	}
	return append([]plan.SCC(nil), view.sccs...)
}

func (view *View) RelationIDs() []model.RelationID {
	values := view.Relations()
	result := make([]model.RelationID, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID())
	}
	return result
}

func (view *View) ExpressionIDs() []model.ExpressionID {
	values := view.Expressions()
	result := make([]model.ExpressionID, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID())
	}
	return result
}

func (view *View) DependencyIDs() []model.DependencyID {
	values := view.Dependencies()
	result := make([]model.DependencyID, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID())
	}
	return result
}

func (view *View) SignatureIdentities() []signature.Identity {
	values := view.Signatures()
	result := make([]signature.Identity, 0, len(values))
	for _, value := range values {
		result = append(result, value.Identity())
	}
	return result
}

func (view *View) sortIssues() {
	sort.SliceStable(view.issues, func(left, right int) bool {
		if view.issues[left].Path != view.issues[right].Path {
			return view.issues[left].Path < view.issues[right].Path
		}
		if view.issues[left].Code != view.issues[right].Code {
			return view.issues[left].Code < view.issues[right].Code
		}
		return view.issues[left].Detail < view.issues[right].Detail
	})
}

func RelationPath(id model.RelationID) string { return fmt.Sprintf("relation[%x]", id.Content()) }
func ColumnPath(id model.ColumnID) string     { return fmt.Sprintf("column[%x]", id.Content()) }
func KeyPath(id model.KeyID) string           { return fmt.Sprintf("key[%x]", id.Content()) }
func ScopePath(id model.ScopeID) string       { return fmt.Sprintf("scope[%x]", id.Content()) }
func ExpressionPath(id model.ExpressionID) string {
	return fmt.Sprintf("expression[%x]", id.Content())
}
func DependencyPath(id model.DependencyID) string {
	return fmt.Sprintf("dependency[%x]", id.Content())
}
func SignaturePath(value signature.Identity) string {
	return fmt.Sprintf("signature[%x/%d]", value.Operation.Content(), value.Version)
}

func ContributionPath(value output.OutputPort) string {
	return fmt.Sprintf("contribution[%x/%d@%x/%x]", value.Operation.Operation.Content(), value.Operation.Version, value.Column.Relation().Content(), value.Column.Content())
}

func InitialPath(value plan.Initial) string {
	operation := value.Operation()
	return fmt.Sprintf("initial[%x/%d@%x]", operation.Operation.Content(), operation.Version, value.Scope().Content())
}

func TypeCapabilityPath(value model.TypeID) string {
	return fmt.Sprintf("type-capability[%x]", value.Content())
}

func ObservationPath(value identity.ContentID) string {
	return fmt.Sprintf("observation[%x]", value)
}
