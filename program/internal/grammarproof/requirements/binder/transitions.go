// Package binder owns cold requirements for public lexical-binding evidence.
// It is intentionally independent of parser fixture census and Program shape.
package binder

// Transition is a closed public binder disposition that Program lowering may
// rely on. These are proof obligations over existing bind.Result evidence,
// not a second binder vocabulary.
type Transition uint8

const (
	TransitionInvalid Transition = iota
	TransitionTypeDeclaration
	TransitionTypeParameter
	TransitionUnresolvedTypeReference
	TransitionQualifiedTypeRoot
	TransitionRuntimePrimitive
	TransitionRuntimeDeclaration
	TransitionRuntimeShadowRejected
	TransitionStaticPublicationPair
	TransitionDirectRequireGlobal
)

// Requirement names one binder transition that must be exercised through its
// typed public Result query before the Program projection layer may rely on it.
type Requirement struct{ Transition Transition }

// Required is the fixed binder disposition denominator. Parser syntax does
// not create or shrink it; each item reflects an existing public bind.Result
// semantic query or typed absence.
func Required() []Requirement {
	return []Requirement{
		{Transition: TransitionTypeDeclaration},
		{Transition: TransitionTypeParameter},
		{Transition: TransitionUnresolvedTypeReference},
		{Transition: TransitionQualifiedTypeRoot},
		{Transition: TransitionRuntimePrimitive},
		{Transition: TransitionRuntimeDeclaration},
		{Transition: TransitionRuntimeShadowRejected},
		{Transition: TransitionStaticPublicationPair},
		{Transition: TransitionDirectRequireGlobal},
	}
}
