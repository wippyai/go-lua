package relcompile

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Name is the authored address of one declared entry: the surface entry that
// issued it and the member key that entry declared it under. A surface entry
// names itself with an absent member, so an axis, a rule, a query, a
// denominator and a nested relation are all addressed the same way.
type Name struct {
	Entry  schema.EntryReference
	Member schema.Key
}

// NewName addresses one nested member of a surface entry.
func NewName(entry schema.EntryReference, member schema.Key) Name {
	return Name{Entry: entry, Member: member}
}

// EntryName addresses one surface entry itself.
func EntryName(surface schema.SurfaceKind, key schema.Key) Name {
	return Name{Entry: schema.EntryReference{Surface: surface, Key: key}}
}

// Available reports whether the name addresses a declared surface entry.
func (name Name) Available() bool { return name.Entry.Available() }

// Owner returns the surface entry that issued this name.
func (name Name) Owner() Name { return Name{Entry: name.Entry} }

// String renders the authored address for a refusal.
func (name Name) String() string {
	if name.Member == "" {
		return fmt.Sprintf("%d:%s", name.Entry.Surface, name.Entry.Key)
	}
	return fmt.Sprintf("%d:%s/%s", name.Entry.Surface, name.Entry.Key, name.Member)
}

// Site is the declaration position one resolution was requested from: the rule
// that named the entry and the authored path inside that rule's declaration.
type Site struct {
	Rule schema.Key
	Path string
}

// String renders the declaration position for a refusal.
func (site Site) String() string {
	if site.Rule == "" {
		return site.Path
	}
	return fmt.Sprintf("%s.%s", site.Rule, site.Path)
}

// ReasonKind is the closed refusal vocabulary of the canonical identity
// registry. Every refusal is one of these; none of them is recoverable by a
// default, a substitution, or a locally minted identity.
type ReasonKind uint8

const (
	// ReasonInvalid is the unavailable zero value.
	ReasonInvalid ReasonKind = iota
	// ReasonUnavailable means the authored name or the owner-issued token is
	// the zero value.
	ReasonUnavailable
	// ReasonUnknown means no owner installed an identity under this name.
	ReasonUnknown
	// ReasonDuplicateName means the name already carries an identity.
	ReasonDuplicateName
	// ReasonDuplicateIdentity means the owner-issued token already names a
	// different entry.
	ReasonDuplicateIdentity
	// ReasonForeign means the resolved entry belongs to an owner or relation
	// other than the one this site requires.
	ReasonForeign
	// ReasonUndeclared means the authored declaration states one side of a
	// relational fact and not the other, so there is nothing to resolve
	// against. It is a declaration-surface finding, never a compensation site.
	ReasonUndeclared
	// ReasonUnlowered means the authored fact is complete and this lowering
	// has no relational counterpart for it yet. Compiling would drop a
	// declared fact, so the resolution refuses instead.
	ReasonUnlowered
)

// String returns the canonical reason label.
func (reason ReasonKind) String() string {
	switch reason {
	case ReasonUnavailable:
		return "unavailable"
	case ReasonUnknown:
		return "unknown"
	case ReasonDuplicateName:
		return "duplicate name"
	case ReasonDuplicateIdentity:
		return "duplicate identity"
	case ReasonForeign:
		return "foreign"
	case ReasonUndeclared:
		return "undeclared"
	case ReasonUnlowered:
		return "unlowered"
	default:
		return "invalid"
	}
}

// EntryKind is the closed vocabulary of what the canonical registry holds. A
// refusal names it so the report says which owner statement is missing, not
// merely that something did not resolve.
type EntryKind uint8

const (
	// KindInvalid is the unavailable zero value.
	KindInvalid EntryKind = iota
	// KindOwner is a declaration-surface entry that issues members.
	KindOwner
	// KindScope is one decision-scope schema.
	KindScope
	// KindRelation is one logical relation.
	KindRelation
	// KindColumn is one relation column.
	KindColumn
	// KindKey is one ordered key vector.
	KindKey
	// KindType is one semantic type.
	KindType
	// KindOperation is one semantic operation.
	KindOperation
	// KindSignature is one sealed semantic operation signature.
	KindSignature
	// KindExpression is one logical expression root.
	KindExpression
	// KindDependency is one logical dependency.
	KindDependency
	// KindDenominator is one authenticated relation/key universe.
	KindDenominator
	// KindAddress is the column a relation's rows are addressed by.
	KindAddress
	// KindPublicationKey is the key a relation's rows are published under.
	KindPublicationKey
)

// String returns the canonical entry-kind label.
func (kind EntryKind) String() string {
	switch kind {
	case KindOwner:
		return "owner"
	case KindScope:
		return "scope"
	case KindRelation:
		return "relation"
	case KindColumn:
		return "column"
	case KindKey:
		return "key"
	case KindType:
		return "type"
	case KindOperation:
		return "operation"
	case KindSignature:
		return "signature"
	case KindExpression:
		return "expression"
	case KindDependency:
		return "dependency"
	case KindDenominator:
		return "denominator"
	case KindAddress:
		return "address"
	case KindPublicationKey:
		return "publication key"
	default:
		return "invalid"
	}
}

// Refusal is the one refusal shape of name resolution. It names the rule and
// the declaration site so an unresolved reference is reported where it is
// authored rather than as an anonymous compile failure.
type Refusal struct {
	Site   Site
	Name   Name
	Kind   EntryKind
	Reason ReasonKind
}

// Error implements error.
func (refusal Refusal) Error() string {
	return fmt.Sprintf("relcompile: %s %s at %s: %s", refusal.Reason, refusal.Kind, refusal.Site, refusal.Name)
}

func refuse(site Site, name Name, kind EntryKind, reason ReasonKind) error {
	return Refusal{Site: site, Name: name, Kind: kind, Reason: reason}
}

// relationEntry accumulates the columns and keys installed under one relation
// so the immutable RelationSchema is built once, in install order.
type relationEntry struct {
	id          model.RelationID
	scope       model.ScopeID
	coordinates map[Coordinate]model.ColumnID
	publish     model.KeyID
	columns     []model.ColumnID
	keys        []model.KeyID
}

// Registry is the one canonical identity registry of the declaration
// compiler. It resolves an authored name to the identity its owner issued and
// never derives one: an identity enters only through an Install call that
// carries the owner-issued token, and a name nobody installed refuses.
//
// The registry is also the sole accumulator of the relation, column, key and
// scope schemas the compiler emits, so a resolved identity and the schema row
// that declares it can never disagree.
type Registry struct {
	owners map[schema.EntryReference]model.OwnerID

	relations     map[Name]*relationEntry
	relationOrder []Name

	columns     map[Name]model.ColumnID
	columnType  map[Name]model.TypeID
	columnOwner map[Name]Name
	columnOrder []Name

	keys       map[Name]model.KeyID
	keyColumns map[Name][]model.ColumnID
	keyOrder   []Name

	scopes     map[Name]model.ScopeID
	scopeDims  map[Name][]model.ColumnID
	scopeOrder []Name

	types        map[Name]model.TypeID
	operations   map[Name]model.OperationID
	expressions  map[Name]model.ExpressionID
	dependencies map[Name]model.DependencyID

	denominators map[Name]model.DenominatorRef

	signatures     map[Name]signature.Signature
	signatureOrder []Name

	issued map[identity.ContentID]Name
}

// NewRegistry returns an empty canonical identity registry.
func NewRegistry() *Registry {
	return &Registry{
		owners:       map[schema.EntryReference]model.OwnerID{},
		relations:    map[Name]*relationEntry{},
		columns:      map[Name]model.ColumnID{},
		columnType:   map[Name]model.TypeID{},
		columnOwner:  map[Name]Name{},
		keys:         map[Name]model.KeyID{},
		keyColumns:   map[Name][]model.ColumnID{},
		scopes:       map[Name]model.ScopeID{},
		scopeDims:    map[Name][]model.ColumnID{},
		types:        map[Name]model.TypeID{},
		operations:   map[Name]model.OperationID{},
		expressions:  map[Name]model.ExpressionID{},
		dependencies: map[Name]model.DependencyID{},
		denominators: map[Name]model.DenominatorRef{},
		signatures:   map[Name]signature.Signature{},
		issued:       map[identity.ContentID]Name{},
	}
}

// claim records that name holds the owner-issued token. One token names one
// entry, so a second entry claiming it refuses.
func (registry *Registry) claim(site Site, name Name, kind EntryKind, issued identity.ContentID) error {
	if !name.Available() || !issued.Available() {
		return refuse(site, name, kind, ReasonUnavailable)
	}
	if holder, taken := registry.issued[issued]; taken {
		if holder == name {
			return refuse(site, name, kind, ReasonDuplicateName)
		}
		return refuse(site, name, kind, ReasonDuplicateIdentity)
	}
	registry.issued[issued] = name
	return nil
}

// InstallOwner adopts the identity a declaration surface issued for one entry
// that owns relations: the axis, query, observation or diagnostic entry whose
// members the following installs are declared under.
func (registry *Registry) InstallOwner(entry schema.EntryReference, issued identity.ContentID) error {
	name := Name{Entry: entry}
	site := Site{Path: "registry.owner"}
	if _, exists := registry.owners[entry]; exists {
		return refuse(site, name, KindOwner, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindOwner, issued); err != nil {
		return err
	}
	owner, ok := model.IssueOwnerID(issued)
	if !ok {
		return refuse(site, name, KindOwner, ReasonUnavailable)
	}
	registry.owners[entry] = owner
	return nil
}

// InstallScope adopts the identity an owner issued for one decision-scope
// schema. Dimensions are declared separately, once their columns exist.
func (registry *Registry) InstallScope(name Name, issued identity.ContentID) error {
	site := Site{Path: "registry.scope"}
	owner, ok := registry.owners[name.Entry]
	if !ok {
		return refuse(site, name.Owner(), KindOwner, ReasonUnknown)
	}
	if _, exists := registry.scopes[name]; exists {
		return refuse(site, name, KindScope, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindScope, issued); err != nil {
		return err
	}
	scope, ok := model.IssueScopeID(owner, issued)
	if !ok {
		return refuse(site, name, KindScope, ReasonUnavailable)
	}
	registry.scopes[name] = scope
	registry.scopeOrder = append(registry.scopeOrder, name)
	return nil
}

// InstallType adopts the identity an owner issued for one semantic type.
func (registry *Registry) InstallType(name Name, issued identity.ContentID) error {
	site := Site{Path: "registry.type"}
	owner, ok := registry.owners[name.Entry]
	if !ok {
		return refuse(site, name.Owner(), KindOwner, ReasonUnknown)
	}
	if _, exists := registry.types[name]; exists {
		return refuse(site, name, KindType, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindType, issued); err != nil {
		return err
	}
	typeID, ok := model.IssueTypeID(owner, issued)
	if !ok {
		return refuse(site, name, KindType, ReasonUnavailable)
	}
	registry.types[name] = typeID
	return nil
}

// InstallRelation adopts the identity an owner issued for one relation and
// binds it to an already installed decision scope.
func (registry *Registry) InstallRelation(name Name, issued identity.ContentID, scope Name) error {
	site := Site{Path: "registry.relation"}
	owner, ok := registry.owners[name.Entry]
	if !ok {
		return refuse(site, name.Owner(), KindOwner, ReasonUnknown)
	}
	scopeID, ok := registry.scopes[scope]
	if !ok {
		return refuse(site, scope, KindScope, ReasonUnknown)
	}
	if _, exists := registry.relations[name]; exists {
		return refuse(site, name, KindRelation, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindRelation, issued); err != nil {
		return err
	}
	relation, ok := model.IssueRelationID(owner, issued)
	if !ok {
		return refuse(site, name, KindRelation, ReasonUnavailable)
	}
	registry.relations[name] = &relationEntry{
		id: relation, scope: scopeID, coordinates: map[Coordinate]model.ColumnID{},
	}
	registry.relationOrder = append(registry.relationOrder, name)
	return nil
}

// InstallColumn adopts the identity a relation's owner issued for one column
// and records the semantic type the column carries.
func (registry *Registry) InstallColumn(name Name, issued identity.ContentID, relation Name, columnType Name) error {
	site := Site{Path: "registry.column"}
	entry, ok := registry.relations[relation]
	if !ok {
		return refuse(site, relation, KindRelation, ReasonUnknown)
	}
	typeID, ok := registry.types[columnType]
	if !ok {
		return refuse(site, columnType, KindType, ReasonUnknown)
	}
	if _, exists := registry.columns[name]; exists {
		return refuse(site, name, KindColumn, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindColumn, issued); err != nil {
		return err
	}
	column, ok := model.IssueColumnID(entry.id, issued)
	if !ok {
		return refuse(site, name, KindColumn, ReasonUnavailable)
	}
	registry.columns[name] = column
	registry.columnType[name] = typeID
	registry.columnOwner[name] = relation
	registry.columnOrder = append(registry.columnOrder, name)
	entry.columns = append(entry.columns, column)
	return nil
}

// InstallKey adopts the identity a relation's owner issued for one ordered key
// vector over that relation's own columns.
func (registry *Registry) InstallKey(name Name, issued identity.ContentID, relation Name, columns ...Name) error {
	site := Site{Path: "registry.key"}
	entry, ok := registry.relations[relation]
	if !ok {
		return refuse(site, relation, KindRelation, ReasonUnknown)
	}
	vector := make([]model.ColumnID, 0, len(columns))
	for _, column := range columns {
		id, known := registry.columns[column]
		if !known {
			return refuse(site, column, KindColumn, ReasonUnknown)
		}
		if id.Relation() != entry.id {
			return refuse(site, column, KindColumn, ReasonForeign)
		}
		vector = append(vector, id)
	}
	if _, exists := registry.keys[name]; exists {
		return refuse(site, name, KindKey, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindKey, issued); err != nil {
		return err
	}
	key, ok := model.IssueKeyID(entry.id, issued)
	if !ok {
		return refuse(site, name, KindKey, ReasonUnavailable)
	}
	registry.keys[name] = key
	registry.keyColumns[name] = vector
	registry.keyOrder = append(registry.keyOrder, name)
	entry.keys = append(entry.keys, key)
	return nil
}

// InstallOperation adopts the identity an owner issued for one semantic
// operation.
func (registry *Registry) InstallOperation(name Name, issued identity.ContentID) error {
	site := Site{Path: "registry.operation"}
	owner, ok := registry.owners[name.Entry]
	if !ok {
		return refuse(site, name.Owner(), KindOwner, ReasonUnknown)
	}
	if _, exists := registry.operations[name]; exists {
		return refuse(site, name, KindOperation, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindOperation, issued); err != nil {
		return err
	}
	operation, ok := model.IssueOperationID(owner, issued)
	if !ok {
		return refuse(site, name, KindOperation, ReasonUnavailable)
	}
	registry.operations[name] = operation
	return nil
}

// InstallExpression adopts the identity an owner issued for one logical
// expression root.
func (registry *Registry) InstallExpression(name Name, issued identity.ContentID) error {
	site := Site{Path: "registry.expression"}
	owner, ok := registry.owners[name.Entry]
	if !ok {
		return refuse(site, name.Owner(), KindOwner, ReasonUnknown)
	}
	if _, exists := registry.expressions[name]; exists {
		return refuse(site, name, KindExpression, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindExpression, issued); err != nil {
		return err
	}
	expression, ok := model.IssueExpressionID(owner, issued)
	if !ok {
		return refuse(site, name, KindExpression, ReasonUnavailable)
	}
	registry.expressions[name] = expression
	return nil
}

// InstallDependency adopts the identity an owner issued for one logical
// dependency.
func (registry *Registry) InstallDependency(name Name, issued identity.ContentID) error {
	site := Site{Path: "registry.dependency"}
	owner, ok := registry.owners[name.Entry]
	if !ok {
		return refuse(site, name.Owner(), KindOwner, ReasonUnknown)
	}
	if _, exists := registry.dependencies[name]; exists {
		return refuse(site, name, KindDependency, ReasonDuplicateName)
	}
	if err := registry.claim(site, name, KindDependency, issued); err != nil {
		return err
	}
	dependency, ok := model.IssueDependencyID(owner, issued)
	if !ok {
		return refuse(site, name, KindDependency, ReasonUnavailable)
	}
	registry.dependencies[name] = dependency
	return nil
}

// InstallDenominator pairs an authored denominator entry with the relation
// and key that close it. The denominator surface names one entry; the relation
// and key it denominates are the owner's own statement of what that entry is.
func (registry *Registry) InstallDenominator(name Name, relation Name, key Name) error {
	site := Site{Path: "registry.denominator"}
	if !name.Available() {
		return refuse(site, name, KindDenominator, ReasonUnavailable)
	}
	entry, ok := registry.relations[relation]
	if !ok {
		return refuse(site, relation, KindRelation, ReasonUnknown)
	}
	keyID, known := registry.keys[key]
	if !known {
		return refuse(site, key, KindKey, ReasonUnknown)
	}
	if _, exists := registry.denominators[name]; exists {
		return refuse(site, name, KindDenominator, ReasonDuplicateName)
	}
	reference, ok := model.NewDenominatorRef(entry.id, keyID)
	if !ok {
		return refuse(site, key, KindKey, ReasonForeign)
	}
	registry.denominators[name] = reference
	return nil
}

// InstallSignature adopts one sealed semantic signature under the operation
// name its owner declared it for. The signature carries the operation identity
// and version, so neither is minted here.
func (registry *Registry) InstallSignature(name Name, value signature.Signature) error {
	site := Site{Path: "registry.signature"}
	operation, ok := registry.operations[name]
	if !ok {
		return refuse(site, name, KindOperation, ReasonUnknown)
	}
	if !value.Available() {
		return refuse(site, name, KindSignature, ReasonUnavailable)
	}
	if value.Identity().Operation != operation {
		return refuse(site, name, KindSignature, ReasonForeign)
	}
	if _, exists := registry.signatures[name]; exists {
		return refuse(site, name, KindSignature, ReasonDuplicateName)
	}
	registry.signatures[name] = value
	registry.signatureOrder = append(registry.signatureOrder, name)
	return nil
}

// DeclarePublicationKey records the key a relation's rows are published under.
func (registry *Registry) DeclarePublicationKey(relation Name, key Name) error {
	site := Site{Path: "registry.publication"}
	entry, ok := registry.relations[relation]
	if !ok {
		return refuse(site, relation, KindRelation, ReasonUnknown)
	}
	keyID, known := registry.keys[key]
	if !known {
		return refuse(site, key, KindKey, ReasonUnknown)
	}
	if keyID.Relation() != entry.id {
		return refuse(site, key, KindKey, ReasonForeign)
	}
	if entry.publish.Available() {
		return refuse(site, relation, KindPublicationKey, ReasonDuplicateName)
	}
	entry.publish = keyID
	return nil
}

// Coordinate is the closed set of addressing coordinates a relation may
// publish. It mirrors what the declaration surface lets a relation say about
// its own rows, so the compiler never resolves a coordinate an owner has no
// way to declare.
type Coordinate uint8

const (
	// CoordinateInvalid is the unavailable zero value.
	CoordinateInvalid Coordinate = iota
	// CoordinateAddress is the column one row of the relation is identified
	// by, and the side an oriented equijoin onto the relation pairs against.
	CoordinateAddress
	// CoordinateParent is the column carrying the address of the parent row
	// this row hangs off.
	CoordinateParent
	// CoordinateOrdinal is the column keying a row within its parent's set.
	CoordinateOrdinal
	// CoordinateTag is the column a selection correlates a returned row with
	// the source row that selected it.
	CoordinateTag
	// CoordinateOccurrence is the column naming the occurrence family a row is
	// enumerated under.
	CoordinateOccurrence
)

// String returns the canonical coordinate label.
func (coordinate Coordinate) String() string {
	switch coordinate {
	case CoordinateAddress:
		return "address"
	case CoordinateParent:
		return "parent"
	case CoordinateOrdinal:
		return "ordinal"
	case CoordinateTag:
		return "tag"
	case CoordinateOccurrence:
		return "occurrence"
	default:
		return "invalid"
	}
}

// DeclareCoordinate records one column a relation's rows are addressed by. It
// is the relation owner's own statement, not a derivation: an equijoin onto
// this relation pairs against a coordinate declared here, and a relation that
// publishes none for what a read needs cannot be joined that way.
func (registry *Registry) DeclareCoordinate(relation Name, coordinate Coordinate, column Name) error {
	site := Site{Path: "registry.coordinate"}
	if coordinate == CoordinateInvalid {
		return refuse(site, relation, KindAddress, ReasonUnavailable)
	}
	entry, ok := registry.relations[relation]
	if !ok {
		return refuse(site, relation, KindRelation, ReasonUnknown)
	}
	id, known := registry.columns[column]
	if !known {
		return refuse(site, column, KindColumn, ReasonUnknown)
	}
	if id.Relation() != entry.id {
		return refuse(site, column, KindColumn, ReasonForeign)
	}
	if _, declared := entry.coordinates[coordinate]; declared {
		return refuse(site, relation, KindAddress, ReasonDuplicateName)
	}
	for declared, existing := range entry.coordinates {
		if existing == id {
			_ = declared
			return refuse(site, column, KindColumn, ReasonDuplicateName)
		}
	}
	entry.coordinates[coordinate] = id
	return nil
}

// DeclareScopeDimensions records the columns one decision scope is a
// conjunction over. Dimensions are declared after their columns exist so a
// scope never has to be built from a column that does not yet have an
// identity.
func (registry *Registry) DeclareScopeDimensions(scope Name, dimensions ...Name) error {
	site := Site{Path: "registry.scope.dimensions"}
	if _, ok := registry.scopes[scope]; !ok {
		return refuse(site, scope, KindScope, ReasonUnknown)
	}
	if len(registry.scopeDims[scope]) != 0 {
		return refuse(site, scope, KindScope, ReasonDuplicateName)
	}
	vector := make([]model.ColumnID, 0, len(dimensions))
	for _, dimension := range dimensions {
		id, known := registry.columns[dimension]
		if !known {
			return refuse(site, dimension, KindColumn, ReasonUnknown)
		}
		vector = append(vector, id)
	}
	registry.scopeDims[scope] = vector
	return nil
}

// Owner resolves one surface entry to the identity it issued.
func (registry *Registry) Owner(site Site, entry schema.EntryReference) (model.OwnerID, error) {
	name := Name{Entry: entry}
	if !name.Available() {
		return model.OwnerID{}, refuse(site, name, KindOwner, ReasonUnavailable)
	}
	owner, ok := registry.owners[entry]
	if !ok {
		return model.OwnerID{}, refuse(site, name, KindOwner, ReasonUnknown)
	}
	return owner, nil
}

// Relation resolves one authored relation name.
func (registry *Registry) Relation(site Site, name Name) (model.RelationID, error) {
	if !name.Available() {
		return model.RelationID{}, refuse(site, name, KindRelation, ReasonUnavailable)
	}
	entry, ok := registry.relations[name]
	if !ok {
		return model.RelationID{}, refuse(site, name, KindRelation, ReasonUnknown)
	}
	return entry.id, nil
}

// Addressed resolves one coordinate a relation publishes about its own rows.
// A coordinate the relation does not publish refuses as undeclared, naming the
// relation and the coordinate, because the other side of that equijoin is the
// owner's statement to make and never the compiler's to guess.
func (registry *Registry) Addressed(site Site, name Name, coordinate Coordinate) (model.ColumnID, error) {
	if !name.Available() {
		return model.ColumnID{}, refuse(site, name, KindRelation, ReasonUnavailable)
	}
	entry, ok := registry.relations[name]
	if !ok {
		return model.ColumnID{}, refuse(site, name, KindRelation, ReasonUnknown)
	}
	column, declared := entry.coordinates[coordinate]
	if !declared {
		return model.ColumnID{}, Refusal{
			Site: Site{Rule: site.Rule, Path: site.Path + "#" + coordinate.String()},
			Name: name, Kind: KindAddress, Reason: ReasonUndeclared,
		}
	}
	return column, nil
}

// Column resolves one authored column name.
func (registry *Registry) Column(site Site, name Name) (model.ColumnID, error) {
	if !name.Available() {
		return model.ColumnID{}, refuse(site, name, KindColumn, ReasonUnavailable)
	}
	column, ok := registry.columns[name]
	if !ok {
		return model.ColumnID{}, refuse(site, name, KindColumn, ReasonUnknown)
	}
	return column, nil
}

// ColumnType resolves the semantic type one column carries. A caller that
// holds a column asks the registry what it is typed as rather than rebuilding
// the type's name, which only the installing owner knows.
func (registry *Registry) ColumnType(site Site, name Name) (model.TypeID, error) {
	if !name.Available() {
		return model.TypeID{}, refuse(site, name, KindColumn, ReasonUnavailable)
	}
	if _, known := registry.columns[name]; !known {
		return model.TypeID{}, refuse(site, name, KindColumn, ReasonUnknown)
	}
	typeID, ok := registry.columnType[name]
	if !ok {
		return model.TypeID{}, refuse(site, name, KindType, ReasonUndeclared)
	}
	return typeID, nil
}

// SealedSignature resolves the whole sealed signature installed under one
// operation name.
func (registry *Registry) SealedSignature(site Site, name Name) (signature.Signature, error) {
	if !name.Available() {
		return signature.Signature{}, refuse(site, name, KindSignature, ReasonUnavailable)
	}
	value, ok := registry.signatures[name]
	if !ok {
		return signature.Signature{}, refuse(site, name, KindSignature, ReasonUnknown)
	}
	return value, nil
}

// RelationKeyOf resolves the key one relation's rows are published under, by
// the relation identity rather than by the name it was installed under.
func (registry *Registry) RelationKeyOf(site Site, relation model.RelationID) (model.KeyID, error) {
	for name, entry := range registry.relations {
		if entry.id != relation {
			continue
		}
		if !entry.publish.Available() {
			return model.KeyID{}, refuse(site, name, KindPublicationKey, ReasonUndeclared)
		}
		return entry.publish, nil
	}
	return model.KeyID{}, refuse(site, Name{}, KindRelation, ReasonUnknown)
}

// RelationColumns returns the columns one relation declares, in the order its
// owner installed them.
func (registry *Registry) RelationColumns(site Site, relation model.RelationID) ([]model.ColumnID, error) {
	for _, entry := range registry.relations {
		if entry.id != relation {
			continue
		}
		return append([]model.ColumnID(nil), entry.columns...), nil
	}
	return nil, refuse(site, Name{}, KindRelation, ReasonUnknown)
}

// Key resolves one authored key name.
func (registry *Registry) Key(site Site, name Name) (model.KeyID, error) {
	if !name.Available() {
		return model.KeyID{}, refuse(site, name, KindKey, ReasonUnavailable)
	}
	key, ok := registry.keys[name]
	if !ok {
		return model.KeyID{}, refuse(site, name, KindKey, ReasonUnknown)
	}
	return key, nil
}

// Scope resolves one authored decision-scope name.
func (registry *Registry) Scope(site Site, name Name) (model.ScopeID, error) {
	if !name.Available() {
		return model.ScopeID{}, refuse(site, name, KindScope, ReasonUnavailable)
	}
	scope, ok := registry.scopes[name]
	if !ok {
		return model.ScopeID{}, refuse(site, name, KindScope, ReasonUnknown)
	}
	return scope, nil
}

// Type resolves one authored semantic type name.
func (registry *Registry) Type(site Site, name Name) (model.TypeID, error) {
	if !name.Available() {
		return model.TypeID{}, refuse(site, name, KindType, ReasonUnavailable)
	}
	typeID, ok := registry.types[name]
	if !ok {
		return model.TypeID{}, refuse(site, name, KindType, ReasonUnknown)
	}
	return typeID, nil
}

// Operation resolves one authored semantic operation name.
func (registry *Registry) Operation(site Site, name Name) (model.OperationID, error) {
	if !name.Available() {
		return model.OperationID{}, refuse(site, name, KindOperation, ReasonUnavailable)
	}
	operation, ok := registry.operations[name]
	if !ok {
		return model.OperationID{}, refuse(site, name, KindOperation, ReasonUnknown)
	}
	return operation, nil
}

// Expression resolves one authored expression name.
func (registry *Registry) Expression(site Site, name Name) (model.ExpressionID, error) {
	if !name.Available() {
		return model.ExpressionID{}, refuse(site, name, KindExpression, ReasonUnavailable)
	}
	expression, ok := registry.expressions[name]
	if !ok {
		return model.ExpressionID{}, refuse(site, name, KindExpression, ReasonUnknown)
	}
	return expression, nil
}

// Dependency resolves one authored dependency name.
func (registry *Registry) Dependency(site Site, name Name) (model.DependencyID, error) {
	if !name.Available() {
		return model.DependencyID{}, refuse(site, name, KindDependency, ReasonUnavailable)
	}
	dependency, ok := registry.dependencies[name]
	if !ok {
		return model.DependencyID{}, refuse(site, name, KindDependency, ReasonUnknown)
	}
	return dependency, nil
}

// Denominator resolves one authored denominator entry.
func (registry *Registry) Denominator(site Site, name Name) (model.DenominatorRef, error) {
	if !name.Available() {
		return model.DenominatorRef{}, refuse(site, name, KindDenominator, ReasonUnavailable)
	}
	reference, ok := registry.denominators[name]
	if !ok {
		return model.DenominatorRef{}, refuse(site, name, KindDenominator, ReasonUnknown)
	}
	return reference, nil
}

// Signature resolves the sealed operation identity installed under one
// authored reducer or operation name.
func (registry *Registry) Signature(site Site, name Name) (signature.Identity, error) {
	if !name.Available() {
		return signature.Identity{}, refuse(site, name, KindSignature, ReasonUnavailable)
	}
	value, ok := registry.signatures[name]
	if !ok {
		return signature.Identity{}, refuse(site, name, KindSignature, ReasonUnknown)
	}
	return value.Identity(), nil
}

// PublicationKeyOf resolves the key one relation's rows are published under.
func (registry *Registry) PublicationKeyOf(site Site, relation model.RelationID) (model.KeyID, error) {
	for name, entry := range registry.relations {
		if entry.id != relation {
			continue
		}
		if !entry.publish.Available() {
			return model.KeyID{}, refuse(site, name, KindPublicationKey, ReasonUndeclared)
		}
		return entry.publish, nil
	}
	return model.KeyID{}, refuse(site, Name{}, KindRelation, ReasonUnknown)
}

// PublicationKey resolves the key of the relation that owns one authored
// destination column.
func (registry *Registry) PublicationKey(site Site, column Name) (model.KeyID, error) {
	if !column.Available() {
		return model.KeyID{}, refuse(site, column, KindColumn, ReasonUnavailable)
	}
	owner, ok := registry.columnOwner[column]
	if !ok {
		return model.KeyID{}, refuse(site, column, KindColumn, ReasonUnknown)
	}
	entry := registry.relations[owner]
	if !entry.publish.Available() {
		return model.KeyID{}, refuse(site, owner, KindPublicationKey, ReasonUndeclared)
	}
	return entry.publish, nil
}

// RelationPublicationKey resolves the key one relation's rows are published
// under. It is the same statement PublicationKey answers through a column of
// that relation, asked of the relation directly: a producer publishes into a
// relation, and the column it stamps is one of that relation's own.
func (registry *Registry) RelationPublicationKey(site Site, relation Name) (model.KeyID, error) {
	if !relation.Available() {
		return model.KeyID{}, refuse(site, relation, KindRelation, ReasonUnavailable)
	}
	entry, ok := registry.relations[relation]
	if !ok {
		return model.KeyID{}, refuse(site, relation, KindRelation, ReasonUnknown)
	}
	if !entry.publish.Available() {
		return model.KeyID{}, refuse(site, relation, KindPublicationKey, ReasonUndeclared)
	}
	return entry.publish, nil
}

// Declaration projects the installed registries into the resolved shape the
// compiler consumes. Relation, column, key and scope order is install order,
// which is the authored declaration order.
func (registry *Registry) Declaration(schemaID model.SchemaID) Declaration {
	declaration := Declaration{SchemaID: schemaID}
	for _, name := range registry.relationOrder {
		entry := registry.relations[name]
		declaration.Relations = append(declaration.Relations,
			model.DefineRelationSchema(entry.id, entry.columns, entry.keys, entry.scope))
	}
	for _, name := range registry.columnOrder {
		declaration.Columns = append(declaration.Columns,
			model.DefineColumnSchema(registry.columns[name], registry.columnType[name]))
	}
	for _, name := range registry.keyOrder {
		declaration.Keys = append(declaration.Keys,
			model.DefineKeySchema(registry.keys[name], registry.keyColumns[name]))
	}
	for _, name := range registry.scopeOrder {
		declaration.Scopes = append(declaration.Scopes,
			model.DefineScopeSchema(registry.scopes[name], registry.scopeDims[name]))
	}
	for _, name := range registry.signatureOrder {
		declaration.Signatures = append(declaration.Signatures, registry.signatures[name])
	}
	return declaration
}
