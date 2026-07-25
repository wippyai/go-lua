package engine

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// publishedPublicationIdentities publishes the stable identity already assigned
// to each source-anchored WIR operation.  The row belongs to the ordinary value
// closure, so a native consumer reads it through the same cut as every other
// conclusion: an instruction without a source span is withheld because it
// cannot be joined back to authored code.
func publishedPublicationIdentities(root front.Compilation) []equation.Fact {
	var rows []equation.Fact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		body := compilation.WIR
		if body != nil {
			for index := 0; index < body.Len(); index++ {
				instruction := body.Instr(index)
				if instruction.Op == wir.OpEntry || instruction.Op == wir.OpExit || instruction.Op == wir.OpNoop || !instruction.ExprSpan.Valid() {
					continue
				}
				rows = append(rows, equation.Fact{
					Key:   fmt.Sprintf("publication_identity/%x/op-%08d", compilation.Body, index),
					Value: []byte("executable_body=present function_generation=present identity=stable_cross_module point=present publication_order=deterministic site_ordinal=present source_span=present"),
				})
			}
		}
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}
