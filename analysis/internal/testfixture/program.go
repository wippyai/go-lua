// Package testfixture provides sealed canonical Programs and the one closed
// checked-in corpus Project-to-Link constructor for cross-package laws. It
// never exposes Builder, private fixture paths, or internal terms.
package testfixture

import (
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
)

// Minimal returns one sealed canonical chunk with an ordinary fixed Values
// return through the production source ingress.
func Minimal() (*program.Program, error) {
	return lower.Lower(lower.Source{
		Name: "semantic-fixture.lua",
		Text: []byte("return 1"),
	})
}

// PositionalTransfer returns a sealed Program whose three-cell Bind consumes a
// one-member closed Values relation. It carries an exact nonzero nil-fill
// adjustment through the production source ingress.
func PositionalTransfer() (*program.Program, error) {
	return lower.Lower(lower.Source{
		Name: "positional-transfer-fixture.lua",
		Text: []byte("local left, middle, right = 1"),
	})
}
