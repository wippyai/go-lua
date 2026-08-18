package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// TypeRefResolution preserves the authored binder result independently from
// its source spelling. It is not an inferred resolution result.
type TypeRefResolution uint8

const (
	TypeRefUnresolved TypeRefResolution = iota + 1
	TypeRefDeclaration
	TypeRefCanonicalPath
)

// TypeRef retains the complete authored spelling and its binder disposition.
// A declaration target and a canonical path are mutually exclusive.
type TypeRef struct {
	Resolution TypeRefResolution
	Target     keyspace.Term
	Root       keyspace.Term
	Source     []keyspace.Key
	Canonical  []keyspace.Key
}

// ReferencesInput is the complete authored TypeRef denominator. Source and
// canonical paths retain key handles only; Source/keyspace membership is a
// later joint-seal obligation.
type ReferencesInput struct{ TypeRef []TypeRef }

type referenceStore struct {
	rows      []typeRefRow
	source    []keyspace.Key
	canonical []keyspace.Key
}

type typeRefRow struct {
	resolution TypeRefResolution
	target     keyspace.Term
	root       keyspace.Term
	source     poolRange
	canonical  poolRange
}

type References struct {
	component *Component
	state     *draftState
}
