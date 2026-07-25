package engine

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// publicationIdentityFacts exposes the stable identity already assigned to
// each source-anchored WIR operation.  The row is metadata about the published
// occurrence, not a second value analysis: an instruction without a source
// span is withheld because it cannot be joined back to authored code.
func publicationIdentityFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		body := compilation.WIR
		if body != nil {
			for index := 0; index < body.Len(); index++ {
				instruction := body.Instr(index)
				if instruction.Op == wir.OpEntry || instruction.Op == wir.OpExit || instruction.Op == wir.OpNoop || !instruction.ExprSpan.Valid() {
					continue
				}
				occurrence := fmt.Sprintf("op-%08d", index)
				rows = append(rows, NativeFact{
					Lane: NativeLaneValues, Family: "publication_identity",
					Key:        fmt.Sprintf("publication_identity/%x/%s", compilation.Body, occurrence),
					Value:      "executable_body=present function_generation=present identity=stable_cross_module point=present publication_order=deterministic site_ordinal=present source_span=present",
					Occurrence: occurrence, Trust: NativeTrustProven,
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
