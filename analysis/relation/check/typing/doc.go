// Package typing independently checks an unchecked ExecutionSchema. It
// consumes the shared check/registry view for nominal declarations and
// validates relation membership, TypeIDs, scopes, keys,
// expression DAGs, semantic signatures, denominators, and all closed logical
// operators without importing declarations, compiler helpers, domains, or
// physical execution. Check returns a deterministic Report; later checker
// packages may compose that report into the opaque mount certificate.
package typing
