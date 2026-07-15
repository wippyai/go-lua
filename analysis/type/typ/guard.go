package typ

// DefaultRecursionDepth is retained temporarily for callers outside
// analysis/type that have not yet migrated to cycle-aware graph traversal.
// Type-core operations do not consult it.
//
// Deprecated: use node/node-pair cycle detection instead of a depth limit.
const DefaultRecursionDepth = 64
