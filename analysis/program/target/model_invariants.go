package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// noTypeValueCapture marks a produced row that captures no type value.
const noTypeValueCapture = ^uint32(0)

// crossActivationOutcomes is the closed number of outcomes that can leave an
// activation boundary, and so the width of every cross-activation row.
const crossActivationOutcomes = 5

// spawnSiblingAlternatives is the closed number of legal sibling orderings a
// spawn authority admits.
const spawnSiblingAlternatives = 2

func initialBindingClassForValue(kind vocabulary.InitialValueKind) vocabulary.InitialBindingClass {
	switch kind {
	case vocabulary.InitialValueOperation:
		return vocabulary.InitialBindingAdmitted
	case vocabulary.InitialValueDeniedOperation:
		return vocabulary.InitialBindingDenied
	case vocabulary.InitialValueNil, vocabulary.InitialValueBoolean, vocabulary.InitialValueInteger, vocabulary.InitialValueFloat, vocabulary.InitialValueString, vocabulary.InitialValueRoot, vocabulary.InitialValueAbsent:
		return vocabulary.InitialBindingOrdinary
	default:
		return vocabulary.InitialBindingInvalid
	}
}
