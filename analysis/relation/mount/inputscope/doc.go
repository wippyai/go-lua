// Package inputscope is the mount-side reading of one owner-issued relation
// input bundle.
//
// It answers the decision scopes a mounted rule stands in and states which
// physical region may be mounted for a named scope. It issues no identity,
// resolves no physical coordinate, holds no mount fence, and implements no
// inventory: the bundle's owner already decided every answer, and this
// package only reads that decision back.
package inputscope
