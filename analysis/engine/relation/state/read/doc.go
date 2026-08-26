// Package read defines the immutable state-to-operator row stream.
//
// State owns key-column extraction, physical arrangements, and row
// authentication. Operators consume this package and never become the
// implementation dependency of state. The package contains no store or
// relation implementation.
package read
