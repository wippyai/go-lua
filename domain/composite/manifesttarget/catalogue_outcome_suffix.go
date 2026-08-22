package manifesttarget

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// outcomeSuffixCarriage refuses an outcome result suffix the sealed result
// vocabulary has no coordinate for.
//
// A result vector is a fixed prefix, an open tail of one element type, and an
// end-anchored suffix. A closed vector has no end-relative coordinate to keep,
// so the target seal folds its suffix into the fixed prefix and every result
// keeps its declared type and its own OutcomeResultID.
//
// Behind an open tail the suffix says something about the last k results of a
// sequence whose length is decided when the call runs, and no sealed row
// carries that relation. The Target publishes OutcomeValueSlots as the prefix
// width and mints an OutcomeResultID per prefix ordinal only, so a suffix
// result is unaddressable; on the consumer side an open-multiplicity
// CallResult publishes the canonical empty slot span and admits every result
// ordinal, so there is no slot for an end-anchored type to reach. Sealing the
// declaration would keep the suffix in the content identity and drop the
// distinction from every reader, so the declaration is refused by name.
//
// Carrying it needs an end-anchored coordinate on both sides: a suffix span on
// CallResult with an anchor discriminant on CallResultSlot (a slot-identity
// change), an admission law for end-anchored ordinals, and OutcomeResultID
// minting over the suffix ordinals in the Target relation identity.
func outcomeSuffixCarriage(catalogue *authoredCatalogue) error {
	names := make(map[operationRef]string, len(catalogue.names))
	for name, ref := range catalogue.names {
		names[ref] = name
	}
	var refusals []error
	for index := range catalogue.operations {
		ref := operationRef(index + 1)
		operation := catalogue.at(ref)
		for outcome, declared := range operation.Outcomes {
			if len(declared.Values.Suffix) == 0 || declared.Values.Tail == vocabulary.ValuesClosed {
				continue
			}
			refusals = append(refusals, fmt.Errorf(
				"target catalogue: %s outcome %d declares a %d-value result suffix behind an open result tail; the sealed result vocabulary addresses a result by its fixed ordinal only, so an end-anchored result has no slot to reach and the declaration is refused rather than dropped",
				names[ref], outcome, len(declared.Values.Suffix),
			))
		}
	}
	return errors.Join(refusals...)
}
