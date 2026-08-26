// Package authority owns the schema-neutral, owner-local attachment of
// relational declarations.
//
// An authority Catalog is not a cross-owner resolver and is not a second
// declaration registry. It is one sealed attachment carried by a declaration
// surface: owner-local named accessors select exact sealed rows for that
// owner's typed projections, while RelationID, ColumnID, KeyID, ScopeID, and
// DenominatorRef are issued by analysis/relation/schema/model. The
// relcompile.Registry remains the one cross-owner resolver and the one place
// that projects these rows into a compiled declaration.
//
// NewDeclaration is the owner-independent content boundary. Declaration.Seal
// then copies every input slice, rejects malformed or foreign local
// references, issues every local identity under the exact Owner fence, and
// returns an immutable ordered view. Column types are carried as the final
// model.TypeID; this package does not introduce a second type-reference
// system. An empty declaration/catalog is valid when attached to a valid
// owner.
package authority
