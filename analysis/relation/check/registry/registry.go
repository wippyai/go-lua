package registry

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
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
)

func (code Code) String() string {
	names := [...]string{
		"Invalid", "SchemaUnavailable", "SchemaIdentityUnavailable",
		"RelationUnavailable", "RelationDuplicate", "ColumnUnavailable",
		"ColumnDuplicate", "KeyUnavailable", "KeyDuplicate", "ScopeUnavailable",
		"ScopeDuplicate", "ExpressionUnavailable", "ExpressionDuplicate",
		"ExpressionNil", "ExpressionDigest", "DependencyUnavailable",
		"DependencyDuplicate", "SignatureUnavailable", "SignatureDuplicate",
		"SCCUnavailable",
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
	schema       plan.ExecutionSchema
	relations    map[model.RelationID]model.RelationSchema
	columns      map[model.ColumnID]model.ColumnSchema
	keys         map[model.KeyID]model.KeySchema
	scopes       map[model.ScopeID]model.ScopeSchema
	expressions  map[model.ExpressionID]plan.ExpressionRef
	dependencies map[model.DependencyID]plan.Dependency
	signatures   map[signature.Identity]signature.Signature
	sccs         []plan.SCC
	issues       []Issue
}

// Build indexes schema exactly once.  Invalid entries remain represented in
// the view where possible so a proof pass can continue to report its nearest
// law, while the structural defect itself is emitted only here.
func Build(schema plan.ExecutionSchema) *View {
	view := &View{
		schema:       schema,
		relations:    make(map[model.RelationID]model.RelationSchema),
		columns:      make(map[model.ColumnID]model.ColumnSchema),
		keys:         make(map[model.KeyID]model.KeySchema),
		scopes:       make(map[model.ScopeID]model.ScopeSchema),
		expressions:  make(map[model.ExpressionID]plan.ExpressionRef),
		dependencies: make(map[model.DependencyID]plan.Dependency),
		signatures:   make(map[signature.Identity]signature.Signature),
		sccs:         schema.SCCs(),
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

func (view *View) Relations() []model.RelationSchema {
	if view == nil {
		return nil
	}
	values := make([]model.RelationSchema, 0, len(view.relations))
	for _, value := range view.relations {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool { return RelationPath(values[left].ID()) < RelationPath(values[right].ID()) })
	return values
}

func (view *View) Columns() []model.ColumnSchema {
	if view == nil {
		return nil
	}
	values := make([]model.ColumnSchema, 0, len(view.columns))
	for _, value := range view.columns {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool { return ColumnPath(values[left].ID()) < ColumnPath(values[right].ID()) })
	return values
}

func (view *View) Keys() []model.KeySchema {
	if view == nil {
		return nil
	}
	values := make([]model.KeySchema, 0, len(view.keys))
	for _, value := range view.keys {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool { return KeyPath(values[left].ID()) < KeyPath(values[right].ID()) })
	return values
}

func (view *View) Scopes() []model.ScopeSchema {
	if view == nil {
		return nil
	}
	values := make([]model.ScopeSchema, 0, len(view.scopes))
	for _, value := range view.scopes {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool { return ScopePath(values[left].ID()) < ScopePath(values[right].ID()) })
	return values
}

func (view *View) Expressions() []plan.ExpressionRef {
	if view == nil {
		return nil
	}
	values := make([]plan.ExpressionRef, 0, len(view.expressions))
	for _, value := range view.expressions {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool {
		return ExpressionPath(values[left].ID()) < ExpressionPath(values[right].ID())
	})
	return values
}

func (view *View) Dependencies() []plan.Dependency {
	if view == nil {
		return nil
	}
	values := make([]plan.Dependency, 0, len(view.dependencies))
	for _, value := range view.dependencies {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool {
		return DependencyPath(values[left].ID()) < DependencyPath(values[right].ID())
	})
	return values
}

func (view *View) Signatures() []signature.Signature {
	if view == nil {
		return nil
	}
	values := make([]signature.Signature, 0, len(view.signatures))
	for _, value := range view.signatures {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool {
		return SignaturePath(values[left].Identity()) < SignaturePath(values[right].Identity())
	})
	return values
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
