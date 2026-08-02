// Package inventory owns the reviewed, declarative built-in semantic inventory
// used to generate owner-local wiring. The checked-in builtins.yaml file uses
// the JSON subset of YAML 1.2 so generation remains standard-library-only.
package inventory

//go:generate go run ./cmd/inventorygen -inventory builtins.yaml -bindings bindings.yaml -registry ../../domain/value/registry/zz_generated_registry.go
