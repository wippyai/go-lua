package definition

import "github.com/wippyai/go-lua/analysis/schema"

// ScheduledDeath is one authored relation derivation, registered against the
// day it stops being authored.
//
// A RelationDerivation's Build has the status a reducer has: authored domain
// logic behind a sealed derived contract, fenced by a signature the
// declaration derives, an output every row of which normalizes through the
// declared axis binding, and exactly one invocation per rule invocation. What
// separates it from a reducer is that a reducer is lattice judgment and stays
// authored forever, while a derivation is RELATIONAL - and relational
// derivation is precisely what a declaration emits once the reduction algebra
// lands. Every authored Build is therefore scheduled to die.
//
// The set of them is a table because a comment admits stragglers. A migration
// that has to be found by grepping is a migration with no completion
// condition; this one completes when the table is empty.
type ScheduledDeath struct {
	// Axis and Relation name the declaration that carries the derivation, so a
	// row is resolved against the source rather than against a symbol name that
	// two axes could both spell.
	Axis     schema.Key
	Relation schema.Key
	// Build is the authored symbol the emitter replaces. It is the whole
	// migration unit: State, Count and At are the shape Build's result is read
	// through, and they go when it goes.
	Build GoSymbol
}

// scheduledDeaths is the whole migration set. Rows are added when a domain
// declares an authored derivation and removed only when that derivation stops
// being authored - never to quiet a refusal.
var scheduledDeaths = []ScheduledDeath{
	{
		Axis:     "placement",
		Relation: "placement/store/storage-routes",
		Build: GoSymbol{
			PackagePath: "github.com/wippyai/go-lua/domain/placement/store",
			Name:        "DeriveRoutes",
			ResultIndex: 0,
		},
	},
	{
		Axis:     "heap",
		Relation: "heap/formal-freeze/routes",
		Build: GoSymbol{
			PackagePath: "github.com/wippyai/go-lua/domain/heap/formalfreeze",
			Name:        "DeriveFreezeRoutes",
			ResultIndex: 0,
		},
	},
	{
		Axis:     "placement",
		Relation: "placement/return-escape/routes",
		Build: GoSymbol{
			PackagePath: "github.com/wippyai/go-lua/domain/placement/returnescape",
			Name:        "DeriveReturnRoutes",
			ResultIndex: 0,
		},
	},
	{
		Axis:     "call",
		Relation: "call/dispatch/routes",
		Build: GoSymbol{
			PackagePath: "github.com/wippyai/go-lua/domain/call/dispatch/route",
			Name:        "Derive",
			ResultIndex: 0,
		},
	},
}

// ScheduledDeaths returns the migration set. The copy is deliberate: the
// ledger is a declaration, and a consumer that could append to it would be a
// second authority over which derivations are authored.
func ScheduledDeaths() []ScheduledDeath {
	return append([]ScheduledDeath(nil), scheduledDeaths...)
}

// scheduledForDeath reports whether one axis's relation derivation is
// registered. An authored Build that is not is a derivation the migration set
// does not know about, which is the one way this table stops being complete.
func scheduledForDeath(axis, relation schema.Key, build GoSymbol) bool {
	for _, row := range scheduledDeaths {
		if row.Axis == axis && row.Relation == relation && sameOwnerSymbolIdentity(row.Build, build) {
			return true
		}
	}
	return false
}

// sameOwnerSymbolIdentity compares two symbol descriptors by the identity a
// ledger row is keyed on: the package that declares the symbol and its name.
func sameOwnerSymbolIdentity(left, right GoSymbol) bool {
	return left.PackagePath == right.PackagePath && left.Name == right.Name
}
