package authorityprojection

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// TypeNameResolver is the deliberately narrow bridge needed for a column
// whose type authority is owned by another sealed surface. It resolves the
// complete nominal TypeID to the exact name under which that type was already
// installed in the target Registry.
//
// The resolver is consumed only during Project and is never retained.
type TypeNameResolver func(model.TypeID) (relcompile.Name, bool)

// Project installs one sealed authority Catalog into an existing canonical
// Registry. The Registry must already contain the Catalog owner: this
// function verifies that exact owner fence and intentionally never calls
// InstallOwner, since accepting InstallOwner's duplicate refusal would hide a
// caller ordering error.
//
// Installation follows the dependency order scopes, relations, columns,
// keys, scope dimensions, denominators, coordinates, and publication keys.
// Every token is taken verbatim from the Catalog and every resulting nominal
// identity is resolved and compared with the Catalog's identity before the
// next dependency tier is installed.
func Project(registry *relcompile.Registry, catalog authority.Catalog, resolver TypeNameResolver) error {
	if registry == nil {
		return fmt.Errorf("authorityprojection: nil registry")
	}
	if !catalog.Available() {
		return refuse(relcompile.Name{}, relcompile.KindOwner, relcompile.ReasonUnavailable)
	}

	owner := catalog.Owner()
	ownerName := relcompile.Name{Entry: owner.Entry}
	ownerID := catalog.OwnerID()
	if !owner.Available() || !ownerID.Available() || owner.ID() != ownerID {
		return refuse(ownerName, relcompile.KindOwner, relcompile.ReasonForeign)
	}
	installedOwner, ownerErr := registry.Owner(site("owner"), owner.Entry)
	if ownerErr != nil {
		// In particular, an uninstalled owner is not adopted here. The caller
		// receives the Registry's exact unknown-owner refusal.
		return ownerErr
	}
	if installedOwner != ownerID {
		return refuse(ownerName, relcompile.KindOwner, relcompile.ReasonForeign)
	}

	if resolver == nil {
		return refuse(ownerName, relcompile.KindType, relcompile.ReasonUnavailable)
	}
	state, err := prepare(catalog, owner.Entry, ownerID, registry, resolver)
	if err != nil {
		return err
	}

	// Scope identities are owner-fenced and relation installation depends on
	// their names, so scopes are always the first installed rows.
	for _, item := range state.scopes {
		if err := registry.InstallScope(item.name, item.scope.Token(), item.scope.Region()); err != nil {
			return err
		}
		if resolved, err := registry.Scope(site("scope"), item.name); err != nil {
			return err
		} else if resolved != item.scope.ID() {
			return refuse(item.name, relcompile.KindScope, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.relations {
		if err := registry.InstallRelation(item.name, item.relation.Token(), item.scopeName); err != nil {
			return err
		}
		if resolved, err := registry.Relation(site("relation"), item.name); err != nil {
			return err
		} else if resolved != item.relation.ID() {
			return refuse(item.name, relcompile.KindRelation, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.columns {
		// Resolve and check the exact TypeID immediately before crossing the
		// Registry InstallColumn boundary. This protects against a resolver
		// that returns a same-shaped or same-named but different type.
		resolvedType, err := registry.Type(site("column.type"), item.typeName)
		if err != nil {
			return err
		}
		if resolvedType != item.column.Type() {
			return refuse(item.typeName, relcompile.KindType, relcompile.ReasonForeign)
		}
		if err := registry.InstallColumn(item.name, item.column.Token(), item.relationName, item.typeName); err != nil {
			return err
		}
		if resolved, err := registry.Column(site("column"), item.name); err != nil {
			return err
		} else if resolved != item.column.ID() {
			return refuse(item.name, relcompile.KindColumn, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.keys {
		if err := registry.InstallKey(item.name, item.key.Token(), item.relationName, item.columnNames...); err != nil {
			return err
		}
		if resolved, err := registry.Key(site("key"), item.name); err != nil {
			return err
		} else if resolved != item.key.ID() {
			return refuse(item.name, relcompile.KindKey, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.scopes {
		if err := registry.DeclareScopeDimensions(item.name, item.dimensionNames...); err != nil {
			return err
		}
		if !scopeDimensionsEqual(registry, item.name, item.scope.ID(), item.dimensionIDs) {
			return refuse(item.name, relcompile.KindScope, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.denominators {
		if err := registry.InstallDenominator(item.name, item.relationName, item.keyName); err != nil {
			return err
		}
		if resolved, err := registry.Denominator(site("denominator"), item.name); err != nil {
			return err
		} else if resolved != item.denominator.Reference() {
			return refuse(item.name, relcompile.KindDenominator, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.coordinates {
		if err := registry.DeclareCoordinate(item.relationName, item.coordinate, item.columnName); err != nil {
			return err
		}
		if resolved, err := registry.Addressed(site("coordinate"), item.relationName, item.coordinate); err != nil {
			return err
		} else if resolved != item.column.ID() {
			return refuse(item.relationName, relcompile.KindAddress, relcompile.ReasonForeign)
		}
	}

	for _, item := range state.publications {
		if err := registry.DeclarePublicationKey(item.relationName, item.keyName); err != nil {
			return err
		}
		if resolved, err := registry.RelationPublicationKey(site("publication"), item.relationName); err != nil {
			return err
		} else if resolved != item.key.ID() {
			return refuse(item.relationName, relcompile.KindPublicationKey, relcompile.ReasonForeign)
		}
	}
	return nil
}

type scopeItem struct {
	name           relcompile.Name
	scope          authority.Scope
	dimensionNames []relcompile.Name
	dimensionIDs   []model.ColumnID
}

type relationItem struct {
	name      relcompile.Name
	relation  authority.Relation
	scopeName relcompile.Name
}

type columnItem struct {
	name         relcompile.Name
	column       authority.Column
	relationName relcompile.Name
	typeName     relcompile.Name
}

type keyItem struct {
	name         relcompile.Name
	key          authority.Key
	relationName relcompile.Name
	columnNames  []relcompile.Name
}

type denominatorItem struct {
	name         relcompile.Name
	denominator  authority.Denominator
	relationName relcompile.Name
	keyName      relcompile.Name
}

type coordinateItem struct {
	relationName relcompile.Name
	coordinate   relcompile.Coordinate
	columnName   relcompile.Name
	column       authority.Column
}

type publicationItem struct {
	relationName relcompile.Name
	keyName      relcompile.Name
	key          authority.Key
}

// projectionState is cold, call-local data. In particular it intentionally
// does not contain the source Catalog or any map/index that survives Project.
type projectionState struct {
	scopes       []scopeItem
	relations    []relationItem
	columns      []columnItem
	keys         []keyItem
	denominators []denominatorItem
	coordinates  []coordinateItem
	publications []publicationItem
}

func prepare(catalog authority.Catalog, owner schema.EntryReference, ownerID model.OwnerID, registry *relcompile.Registry, resolver TypeNameResolver) (projectionState, error) {
	state := projectionState{}

	scopes := catalog.Scopes()
	relations := catalog.Relations()
	columns := catalog.Columns()
	keys := catalog.Keys()
	denominators := catalog.Denominators()

	scopeByLabel := make(map[schema.Key]authority.Scope, len(scopes))
	for _, value := range scopes {
		name := relcompile.NewName(owner, value.Name())
		if !value.Available() || value.ID().Owner() != ownerID || value.Token() != value.ID().Content() || !value.Name().Available() {
			return projectionState{}, refuse(name, relcompile.KindScope, relcompile.ReasonUnavailable)
		}
		if _, exists := scopeByLabel[value.Name()]; exists {
			return projectionState{}, refuse(name, relcompile.KindScope, relcompile.ReasonDuplicateName)
		}
		scopeByLabel[value.Name()] = value
	}

	columnByLabel := make(map[schema.Key]authority.Column, len(columns))
	for _, value := range columns {
		name := relcompile.NewName(owner, value.Name())
		if !value.Available() || value.ID().Owner() != ownerID || value.Token() != value.ID().Content() || !value.Name().Available() {
			return projectionState{}, refuse(name, relcompile.KindColumn, relcompile.ReasonUnavailable)
		}
		if _, exists := columnByLabel[value.Name()]; exists {
			return projectionState{}, refuse(name, relcompile.KindColumn, relcompile.ReasonDuplicateName)
		}
		columnByLabel[value.Name()] = value
	}

	keyByLabel := make(map[schema.Key]authority.Key, len(keys))
	for _, value := range keys {
		name := relcompile.NewName(owner, value.Name())
		if !value.Available() || value.ID().Owner() != ownerID || value.Token() != value.ID().Content() || !value.Name().Available() {
			return projectionState{}, refuse(name, relcompile.KindKey, relcompile.ReasonUnavailable)
		}
		if _, exists := keyByLabel[value.Name()]; exists {
			return projectionState{}, refuse(name, relcompile.KindKey, relcompile.ReasonDuplicateName)
		}
		keyByLabel[value.Name()] = value
	}

	relationByLabel := make(map[schema.Key]authority.Relation, len(relations))
	for _, value := range relations {
		name := relcompile.NewName(owner, value.Name())
		if !value.Available() || value.ID().Owner() != ownerID || value.Token() != value.ID().Content() || !value.Name().Available() {
			return projectionState{}, refuse(name, relcompile.KindRelation, relcompile.ReasonUnavailable)
		}
		if _, exists := relationByLabel[value.Name()]; exists {
			return projectionState{}, refuse(name, relcompile.KindRelation, relcompile.ReasonDuplicateName)
		}
		if _, scopeKnown := scopeByLabel[value.Scope()]; !scopeKnown {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Scope()), relcompile.KindScope, relcompile.ReasonUnknown)
		}
		relationByLabel[value.Name()] = value
	}

	// Validate the owner-local graph again at this crossing boundary. The
	// sealed constructor already proves it, but these checks keep a malformed
	// or hostile value from being trusted merely because it reports Available.
	for _, value := range columns {
		if relation, known := relationByLabel[value.Relation()]; !known {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Relation()), relcompile.KindRelation, relcompile.ReasonUnknown)
		} else if value.ID().Relation() != relation.ID() {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindColumn, relcompile.ReasonForeign)
		}
	}
	for _, value := range keys {
		relation, known := relationByLabel[value.Relation()]
		if !known {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Relation()), relcompile.KindRelation, relcompile.ReasonUnknown)
		}
		if value.ID().Relation() != relation.ID() {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindKey, relcompile.ReasonForeign)
		}
		seen := make(map[schema.Key]struct{}, len(value.Columns()))
		for _, label := range value.Columns() {
			if _, duplicate := seen[label]; duplicate {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonDuplicateName)
			}
			seen[label] = struct{}{}
			if column, known := columnByLabel[label]; !known {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonUnknown)
			} else if column.Relation() != value.Relation() {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonForeign)
			}
		}
	}
	for _, value := range scopes {
		seen := make(map[schema.Key]struct{}, len(value.Dimensions()))
		for _, label := range value.Dimensions() {
			if _, duplicate := seen[label]; duplicate {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonDuplicateName)
			}
			seen[label] = struct{}{}
			if _, known := columnByLabel[label]; !known {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonUnknown)
			}
		}
	}

	for _, value := range relations {
		expectedColumns := make([]schema.Key, 0)
		for _, column := range columns {
			if column.Relation() == value.Name() {
				expectedColumns = append(expectedColumns, column.Name())
			}
		}
		if !equalLabels(value.Columns(), expectedColumns) {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindRelation, relcompile.ReasonForeign)
		}
		seenColumns := make(map[schema.Key]struct{}, len(value.Columns()))
		for _, label := range value.Columns() {
			if _, duplicate := seenColumns[label]; duplicate {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonDuplicateName)
			}
			seenColumns[label] = struct{}{}
			column, known := columnByLabel[label]
			if !known {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonUnknown)
			}
			if column.Relation() != value.Name() {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindColumn, relcompile.ReasonForeign)
			}
		}
		seenKeys := make(map[schema.Key]struct{}, len(value.Keys()))
		for _, label := range value.Keys() {
			if _, duplicate := seenKeys[label]; duplicate {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindKey, relcompile.ReasonDuplicateName)
			}
			seenKeys[label] = struct{}{}
			key, known := keyByLabel[label]
			if !known {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindKey, relcompile.ReasonUnknown)
			}
			if key.Relation() != value.Name() {
				return projectionState{}, refuse(relcompile.NewName(owner, label), relcompile.KindKey, relcompile.ReasonForeign)
			}
		}
		expectedKeys := make([]schema.Key, 0)
		for _, key := range keys {
			if key.Relation() == value.Name() {
				expectedKeys = append(expectedKeys, key.Name())
			}
		}
		if !equalLabels(value.Keys(), expectedKeys) {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindRelation, relcompile.ReasonForeign)
		}
		seenCoordinates := make(map[authority.Coordinate]struct{}, len(value.Addressing()))
		seenAddressColumns := make(map[schema.Key]struct{}, len(value.Addressing()))
		for _, address := range value.Addressing() {
			coordinate, coordinateOK := mapCoordinate(address.Coordinate)
			if !address.Available() || !coordinateOK {
				return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindAddress, relcompile.ReasonUnavailable)
			}
			if _, duplicate := seenCoordinates[address.Coordinate]; duplicate {
				return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindAddress, relcompile.ReasonDuplicateName)
			}
			seenCoordinates[address.Coordinate] = struct{}{}
			if _, duplicate := seenAddressColumns[address.Column]; duplicate {
				return projectionState{}, refuse(relcompile.NewName(owner, address.Column), relcompile.KindColumn, relcompile.ReasonDuplicateName)
			}
			seenAddressColumns[address.Column] = struct{}{}
			column, known := columnByLabel[address.Column]
			if !known {
				return projectionState{}, refuse(relcompile.NewName(owner, address.Column), relcompile.KindColumn, relcompile.ReasonUnknown)
			}
			if column.Relation() != value.Name() || column.ID().Relation() != value.ID() {
				return projectionState{}, refuse(relcompile.NewName(owner, address.Column), relcompile.KindColumn, relcompile.ReasonForeign)
			}
			state.coordinates = append(state.coordinates, coordinateItem{
				relationName: relcompile.NewName(owner, value.Name()), coordinate: coordinate,
				columnName: relcompile.NewName(owner, address.Column), column: column,
			})
		}
		if publication, present := value.PublicationKey(); present {
			key, known := keyByLabel[publication]
			if !known {
				return projectionState{}, refuse(relcompile.NewName(owner, publication), relcompile.KindKey, relcompile.ReasonUnknown)
			}
			if key.Relation() != value.Name() || key.ID().Relation() != value.ID() {
				return projectionState{}, refuse(relcompile.NewName(owner, publication), relcompile.KindKey, relcompile.ReasonForeign)
			}
			state.publications = append(state.publications, publicationItem{
				relationName: relcompile.NewName(owner, value.Name()), keyName: relcompile.NewName(owner, publication), key: key,
			})
		}
	}

	denominatorByLabel := make(map[schema.Key]struct{}, len(denominators))
	for _, value := range denominators {
		name := relcompile.NewName(owner, value.Name())
		if !value.Available() {
			return projectionState{}, refuse(name, relcompile.KindDenominator, relcompile.ReasonUnavailable)
		}
		if _, exists := denominatorByLabel[value.Name()]; exists {
			return projectionState{}, refuse(name, relcompile.KindDenominator, relcompile.ReasonDuplicateName)
		}
		denominatorByLabel[value.Name()] = struct{}{}
		relation, relationKnown := relationByLabel[value.Relation()]
		if !relationKnown {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Relation()), relcompile.KindRelation, relcompile.ReasonUnknown)
		}
		key, keyKnown := keyByLabel[value.Key()]
		if !keyKnown {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Key()), relcompile.KindKey, relcompile.ReasonUnknown)
		}
		if relation.Name() != value.Relation() || key.Relation() != value.Relation() || value.Reference() != (mustDenominator(relation.ID(), key.ID())) {
			return projectionState{}, refuse(name, relcompile.KindDenominator, relcompile.ReasonForeign)
		}
		state.denominators = append(state.denominators, denominatorItem{
			name: name, denominator: value, relationName: relcompile.NewName(owner, value.Relation()), keyName: relcompile.NewName(owner, value.Key()),
		})
	}

	for _, value := range scopes {
		item := scopeItem{name: relcompile.NewName(owner, value.Name()), scope: value}
		for _, label := range value.Dimensions() {
			item.dimensionNames = append(item.dimensionNames, relcompile.NewName(owner, label))
			item.dimensionIDs = append(item.dimensionIDs, columnByLabel[label].ID())
		}
		state.scopes = append(state.scopes, item)
	}
	for _, value := range relations {
		state.relations = append(state.relations, relationItem{
			name: relcompile.NewName(owner, value.Name()), relation: value,
			scopeName: relcompile.NewName(owner, value.Scope()),
		})
	}
	for _, value := range columns {
		if resolver == nil {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindType, relcompile.ReasonUnavailable)
		}
		typeName, resolved := resolver(value.Type())
		if !resolved {
			return projectionState{}, refuse(relcompile.NewName(owner, value.Name()), relcompile.KindType, relcompile.ReasonUnknown)
		}
		if !typeName.Available() {
			return projectionState{}, refuse(typeName, relcompile.KindType, relcompile.ReasonUnavailable)
		}
		resolvedType, typeErr := registry.Type(site("column.type"), typeName)
		if typeErr != nil {
			return projectionState{}, typeErr
		}
		if resolvedType != value.Type() {
			return projectionState{}, refuse(typeName, relcompile.KindType, relcompile.ReasonForeign)
		}
		state.columns = append(state.columns, columnItem{
			name: relcompile.NewName(owner, value.Name()), column: value,
			relationName: relcompile.NewName(owner, value.Relation()), typeName: typeName,
		})
	}
	for _, value := range keys {
		item := keyItem{name: relcompile.NewName(owner, value.Name()), key: value, relationName: relcompile.NewName(owner, value.Relation())}
		for _, label := range value.Columns() {
			item.columnNames = append(item.columnNames, relcompile.NewName(owner, label))
		}
		state.keys = append(state.keys, item)
	}
	return state, nil
}

func mustDenominator(relation model.RelationID, key model.KeyID) model.DenominatorRef {
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		return model.DenominatorRef{}
	}
	return value
}

func equalLabels(left, right []schema.Key) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func scopeDimensionsEqual(registry *relcompile.Registry, name relcompile.Name, id model.ScopeID, want []model.ColumnID) bool {
	declaration := registry.Declaration(model.SchemaID{})
	for _, scope := range declaration.Scopes {
		if scope.ID() == id {
			got := scope.Dimensions()
			if len(got) != len(want) {
				return false
			}
			for index := range want {
				if got[index] != want[index] {
					return false
				}
			}
			return true
		}
	}
	_ = name
	return false
}

// mapCoordinate is intentionally exhaustive. There is no default arm: a new
// authority coordinate must force this adapter to be updated at compile time
// review, while malformed values fall through to the explicit unavailable
// result below.
func mapCoordinate(value authority.Coordinate) (relcompile.Coordinate, bool) {
	switch value {
	case authority.CoordinateAddress:
		return relcompile.CoordinateAddress, true
	case authority.CoordinateParent:
		return relcompile.CoordinateParent, true
	case authority.CoordinateOrdinal:
		return relcompile.CoordinateOrdinal, true
	case authority.CoordinateTag:
		return relcompile.CoordinateTag, true
	case authority.CoordinateDestination:
		return relcompile.CoordinateDestination, true
	case authority.CoordinateOccurrence:
		return relcompile.CoordinateOccurrence, true
	}
	return relcompile.CoordinateInvalid, false
}

func site(path string) relcompile.Site { return relcompile.Site{Path: "authorityprojection." + path} }

func refuse(name relcompile.Name, kind relcompile.EntryKind, reason relcompile.ReasonKind) error {
	return relcompile.Refusal{Site: site(kind.String()), Name: name, Kind: kind, Reason: reason}
}
