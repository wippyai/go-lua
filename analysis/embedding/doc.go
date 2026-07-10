// Package embedding defines the stable, protocol-neutral vocabulary used to
// embed the checker in an editor, registry runtime, or other host.
//
// It deliberately contains only immutable identity and snapshot DTOs. Hosts
// own I/O, URI/display policy, overlays, resolution policy, scheduling, and
// runtime facts; the checker consumes materialized inputs.
package embedding
