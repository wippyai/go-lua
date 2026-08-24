// Package impl is a synthetic fixture standing in for a domain package's
// runtime implementation, used only by inventory_test.go.
package impl

// Value is the fixture's fact type.
type Value struct{ N int }

// Key is the fixture's candidate key type.
type Key struct{ N int }

// FixtureKey returns the key half of a Value, matching the
// fixtureMethod("FixtureKey", "Key", -1) accessor declared in
// memberdefinition/contribution.go.
func (k Key) FixtureKey() Key { return k }

// FixtureFact is the fixture's reducer implementation, matching the
// Implementation GoSymbol declared in memberdefinition/contribution.go.
func FixtureFact(key Key, predecessor Value) Value {
	return predecessor
}
