package relationconstructor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile/authorityprojection"
)

// TypeDeclaration is one carrier's type as a construction installs it: the
// carrier it stands for, the member name it is installed under, and the token
// its owner issued for it.
//
// Types are the carrier surface's authority, so a construction is handed them
// rather than deriving them. Both the identity the model issues and the name
// the registry resolves come from this one statement, so the two can never
// disagree about which type a column has.
type TypeDeclaration struct {
	Carrier carrier.Key
	Name    schema.Key
	Token   identity.ContentID
}

// Available reports whether this declaration names one carrier, one member,
// and one issued token.
func (declaration TypeDeclaration) Available() bool {
	return declaration.Carrier.Available() && declaration.Name.Available() && declaration.Token.Available()
}

// Axis is one axis's whole contribution to a construction: the entry that owns
// it, the fence its identities are issued under, the members it declared, and
// the types its columns carry.
type Axis struct {
	Entry   schema.EntryReference
	Owner   authority.Owner
	Catalog member.Catalog
	Types   []TypeDeclaration
}

// Available reports whether the axis states an owner fence matching its entry
// and a complete member catalog.
func (axis Axis) Available() bool {
	if !axis.Entry.Available() || !axis.Owner.Available() || axis.Owner.Entry != axis.Entry {
		return false
	}
	if !axis.Catalog.Complete() {
		return false
	}
	for _, declaration := range axis.Types {
		if !declaration.Available() {
			return false
		}
	}
	return true
}

// Install builds the one cross-owner registry a construction resolves through.
//
// Every axis installs its own owner, its own types, and the relational
// attachment its declaration already states; every scope a rule is placed at
// installs under the structure entry that names it. Nothing is resolved across
// owners here: the registry is that authority, and this only fills it.
//
// The order is the dependency order. An owner precedes the types it issues, a
// type precedes the columns that carry it, and the attachment precedes the
// rules resolved against it. A duplicate or foreign declaration refuses the
// whole installation rather than leaving a registry half filled.
func Install(axes []Axis, placements []relcompile.Placement) (*relcompile.Registry, bool) {
	if len(axes) == 0 {
		return nil, false
	}
	registry := relcompile.NewRegistry()
	for _, axis := range axes {
		if !axis.Available() {
			return nil, false
		}
		if err := registry.InstallOwner(axis.Entry, axis.Owner.Token); err != nil {
			return nil, false
		}
		for _, declaration := range axis.Types {
			name := relcompile.NewName(axis.Entry, declaration.Name)
			if err := registry.InstallType(name, declaration.Token); err != nil {
				return nil, false
			}
		}
	}
	for _, axis := range axes {
		catalog, ok := Authority(axis.Entry, axis.Catalog, axis.Owner, axis.typeResolver())
		if !ok || !catalog.Available() {
			return nil, false
		}
		if err := authorityprojection.Project(registry, catalog, axis.typeNames()); err != nil {
			return nil, false
		}
	}
	if !installScopes(registry, placements) {
		return nil, false
	}
	return registry, true
}

// typeResolver answers the identity the model issues for one carrier, under
// this axis's own owner fence.
func (axis Axis) typeResolver() TypeResolver {
	return func(key carrier.Key) (model.TypeID, bool) {
		for _, declaration := range axis.Types {
			if declaration.Carrier != key {
				continue
			}
			return model.IssueTypeID(axis.Owner.ID(), declaration.Token)
		}
		return model.TypeID{}, false
	}
}

// typeNames answers the name one already-installed type resolves under. It is
// the inverse of typeResolver over the same single statement, so a column's
// identity and its name are two readings of one declaration.
func (axis Axis) typeNames() authorityprojection.TypeNameResolver {
	return func(typeID model.TypeID) (relcompile.Name, bool) {
		for _, declaration := range axis.Types {
			issued, ok := model.IssueTypeID(axis.Owner.ID(), declaration.Token)
			if !ok || issued != typeID {
				continue
			}
			return relcompile.NewName(axis.Entry, declaration.Name), true
		}
		return relcompile.Name{}, false
	}
}

// installScopes installs every decision scope the admitted rules are placed
// at. A scope is owned by the structure entry that names it, so each installs
// its own owner once; a scope named by two rules is one scope and is installed
// once.
func installScopes(registry *relcompile.Registry, placements []relcompile.Placement) bool {
	seen := make(map[relcompile.Name]struct{}, len(placements)*2)
	for _, placement := range placements {
		names := append([]relcompile.Name{placement.Candidate}, placement.Ports...)
		for _, name := range names {
			if !name.Available() {
				return false
			}
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			ownerToken, ok := scopeToken("scope-owner", name)
			if !ok {
				return false
			}
			if err := registry.InstallOwner(name.Entry, ownerToken); err != nil {
				return false
			}
			token, ok := scopeToken("scope", name)
			if !ok {
				return false
			}
			// A decision scope declares no dimension of its own, so its region
			// is the empty conjunction. Physical extent is lowered at mount.
			if err := registry.InstallScope(name, token, region.True()); err != nil {
				return false
			}
		}
	}
	return true
}

// scopeToken derives one decision scope's token from the name the declaration
// already determines, so two constructions of one program install the same
// scope identity.
func scopeToken(kind string, name relcompile.Name) (identity.ContentID, bool) {
	return identity.DeriveContentID(authorityTokenDomain, []byte(kind), []byte(name.String()))
}

