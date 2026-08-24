// Package impl is a synthetic fixture standing in for a domain package's
// runtime implementation, used only by render_test.go. It deliberately does
// not declare FixtureMissing, the symbol memberdefinition/contribution.go
// names.
package impl

// Value is the fixture's fact type.
type Value struct{ N int }
