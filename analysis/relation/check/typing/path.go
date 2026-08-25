package typing

import checkregistry "github.com/wippyai/go-lua/analysis/relation/check/registry"

// Keep the checker vocabulary bound to the shared canonical path functions;
// no pass-specific formatting can drift from the registry's issue paths.
var (
	relationPath   = checkregistry.RelationPath
	columnPath     = checkregistry.ColumnPath
	keyPath        = checkregistry.KeyPath
	scopePath      = checkregistry.ScopePath
	expressionPath = checkregistry.ExpressionPath
	dependencyPath = checkregistry.DependencyPath
	signaturePath  = checkregistry.SignaturePath
)
