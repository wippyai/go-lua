package ops

import (
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// LogicalAndTyped synthesizes the type of a Lua 'and' expression.
//
// Lua semantics: `a and b` returns `a` if `a` is falsy, otherwise returns `b`.
// The expression short-circuits: `b` is not evaluated if `a` is falsy.
//
// Type analysis:
//   - If left is definitely truthy: result is right's type
//   - If left is definitely falsy: result is left's type
//   - Otherwise: union of (falsy part of left) and right
//
// Examples:
//   - `true and x` -> type of x
//   - `nil and x` -> nil
//   - `string? and number` -> nil | number
func LogicalAndTyped(left, right typ.Type) typ.Type {
	left = ExtractFirstValue(left)
	right = ExtractFirstValue(right)

	// Unknown left type: cannot determine result
	if left == nil {
		return typ.Unknown
	}

	// Never type propagates (unreachable code)
	if left.Kind().IsNever() {
		return typ.Never
	}

	if right != nil && right.Kind().IsNever() {
		return typ.Never
	}

	// If left is definitely truthy (cannot be nil or false), result is right
	if !CanBeFalsy(left) {
		return right
	}

	// If left is definitely falsy (nil or false), result is left
	if IsFalsy(left) {
		return left
	}

	// Otherwise, could be either the falsy part of left or right
	falsyLeft := narrow.ToFalsy(left)
	if falsyLeft == nil || falsyLeft.Kind().IsNever() {
		return right
	}

	return typ.NewUnion(falsyLeft, right)
}

// LogicalOrTyped synthesizes the type of a Lua 'or' expression.
//
// Lua semantics: `a or b` returns `a` if `a` is truthy, otherwise returns `b`.
// The expression short-circuits: `b` is not evaluated if `a` is truthy.
//
// Type analysis:
//   - If left is definitely truthy: result is left's type
//   - If left is definitely falsy: result is right's type
//   - Otherwise: union of (truthy part of left) and right
//
// Special case: For `any? or concrete`, prefers the concrete type.
// This handles the common default value pattern: `x = x or default`.
func LogicalOrTyped(left, right typ.Type) typ.Type {
	left = ExtractFirstValue(left)
	right = ExtractFirstValue(right)

	// Unknown left type: cannot determine result
	if left == nil {
		return typ.Unknown
	}

	// Never type propagates (unreachable code)
	if left.Kind().IsNever() {
		return typ.Never
	}

	if right != nil && right.Kind().IsNever() {
		return typ.Never
	}

	// If left is definitely truthy, result is left
	if !CanBeFalsy(left) {
		return left
	}

	// If left is definitely falsy, result is right
	if IsFalsy(left) {
		return right
	}

	// Special case: when left is optional any (any?) and right is concrete,
	// prefer right type. This handles the common `x = x or default` pattern.
	if opt, ok := left.(*typ.Optional); ok {
		if typ.IsAny(opt.Inner) {
			if right != nil && right.Kind().IsConcrete() {
				return right
			}
		}
	}
	// Soft optional default: when left is an optional soft annotation
	// (e.g., any[]?) and right is concrete, prefer right to avoid
	// contaminating defaults with placeholders.
	if opt, ok := left.(*typ.Optional); ok {
		if opt.Inner != nil && typ.IsSoft(opt.Inner, typ.SoftAnnotationPolicy) {
			if right != nil && right.Kind().IsConcrete() {
				return right
			}
		}
	}

	// Otherwise, could be either the truthy part of left or right
	truthyLeft := narrow.ToTruthy(left)
	if truthyLeft == nil || truthyLeft.Kind().IsNever() {
		return right
	}

	return typ.NewUnion(truthyLeft, right)
}
