// Package authorityprojection adapts one sealed owner-local authority catalog
// to the canonical relcompile registry.
//
// The adapter is deliberately a one-shot projection boundary. It does not
// create a Registry, retain a Catalog, or become another resolver. The caller
// must install the catalog owner in the target registry first; the only
// transient lookup supplied by the caller is a TypeNameResolver for the
// complete model.TypeID carried by a column.
package authorityprojection
