package structure

import "github.com/wippyai/go-lua/analysis/schema"

// The native publication vocabularies. A native row publishes typed columns,
// and each column whose value is a member of a closed vocabulary names that
// member by its declared ordinal. The spellings live here for the same reason
// the diagnostic severities do: a renderer resolves the member at the ordinal
// and reads its declared name rather than holding a switch of its own, and a
// consumer that compares a column against authored text compares against this
// declaration.
//
// Every category below states which owner's ordinals it is pinned to. A pinned
// category numbers its members exactly as its owner enumerates them, so the
// projection from a published column to a member is the ordinal itself and no
// translation table stands between them.

type nativePublicationMember struct {
	key      schema.Key
	spelling string
}

// nativeNumericRepresentations are the carriers a proven numeric column is
// published under. The ordinals are programschema.NumericRepresentation's own
// numbering.
var nativeNumericRepresentations = [...]nativePublicationMember{
	{"native/representation/integer", "integer"},
	{"native/representation/float", "float"},
	{"native/representation/number", "number"},
}

// nativeScalarRepresentations are the carriers a proven exact scalar is
// published under. The ordinals are keyspace.LiteralKind's own numbering, with
// Lua nil appended: nil retains its own identity and has no literal kind.
var nativeScalarRepresentations = [...]nativePublicationMember{
	{"native/scalar/boolean", "boolean"},
	{"native/scalar/integer", "integer"},
	{"native/scalar/float", "float"},
	{"native/scalar/string", "string"},
	{"native/scalar/nil", "nil"},
}

// nativeArithmeticOperators are the primitive binary arithmetic operators a
// published operator column names. The ordinals are flowkind.BinaryOp's own
// numbering, whose arithmetic members occupy the first contiguous segment of
// that vocabulary.
var nativeArithmeticOperators = [...]nativePublicationMember{
	{"native/operator/add", "add"},
	{"native/operator/sub", "sub"},
	{"native/operator/mul", "mul"},
	{"native/operator/div", "div"},
	{"native/operator/idiv", "idiv"},
	{"native/operator/mod", "mod"},
	{"native/operator/pow", "pow"},
}

// nativeUnaryOperators are the Lua unary operators a published operator column
// names. The ordinals are flowkind.UnaryOp's own numbering, and the spellings
// are Lua's own operator names.
var nativeUnaryOperators = [...]nativePublicationMember{
	{"native/unary/unm", "unm"},
	{"native/unary/not", "not"},
	{"native/unary/len", "len"},
	{"native/unary/bnot", "bnot"},
}

// nativeDivisorProperties are the divisor proofs a published divisor column
// names. The first two ordinals are programschema.ArithmeticDivisorProperty's
// own numbering of its proved members; the absent property publishes no column
// at all, and not-applicable is the operator-level answer for a division whose
// result carries no integer divisor obligation.
var nativeDivisorProperties = [...]nativePublicationMember{
	{"native/divisor/nonzero", "nonzero"},
	{"native/divisor/nonzero-not-minus-one", "nonzero_not_minus_one"},
	{"native/divisor/not-applicable", "not_applicable"},
}

// nativeTruthinessClasses are the branch-condition truth verdicts. The
// unobserved member is the incomplete fold: a condition whose evidence set was
// not read out at every point is not the same answer as a condition proved to
// take both truths, and the two are distinct members here so that no consumer
// has to tell them apart by a missing column.
var nativeTruthinessClasses = [...]nativePublicationMember{
	{"native/truthiness/always-truthy", "always_truthy"},
	{"native/truthiness/always-falsy", "always_falsy"},
	{"native/truthiness/dynamic", "dynamic_nil_or_false"},
	{"native/truthiness/unobserved", "unobserved"},
}

// nativeBranchPartitions are the branch geometry verdicts derived from the
// truthiness class: which arm the proof admits, or that no partition was
// proved. Its unobserved member carries the same distinction the truthiness
// vocabulary states.
var nativeBranchPartitions = [...]nativePublicationMember{
	{"native/partition/always-taken", "always_taken"},
	{"native/partition/always-not-taken", "always_not_taken"},
	{"native/partition/dynamic", "dynamic"},
	{"native/partition/unobserved", "unobserved"},
}

// nativeBranchArms are the two arms of a Lua conditional. A partition proof
// names the arm it proves dead.
var nativeBranchArms = [...]nativePublicationMember{
	{"native/arm/then", "then"},
	{"native/arm/else", "else"},
}

// nativeNumericOverflows are the arithmetic overflow disciplines a published
// overflow column names. The ordinals are value.NumericOverflow's own
// numbering.
var nativeNumericOverflows = [...]nativePublicationMember{
	{"native/overflow/closed-integer", "closed_integer"},
	{"native/overflow/promote-integer-to-number", "promote_integer_to_number"},
	{"native/overflow/ieee754", "ieee754"},
}

// NativePublicationSpecs returns the canonical structural declarations of the
// native publication vocabularies. The returned slice is detached so callers
// cannot mutate the inventory owned by this package.
func NativePublicationSpecs() []Spec {
	declarations := []struct {
		category Category
		members  []nativePublicationMember
	}{
		{CategoryNativeNumericRepresentation, nativeNumericRepresentations[:]},
		{CategoryNativeScalarRepresentation, nativeScalarRepresentations[:]},
		{CategoryNativeArithmeticOperator, nativeArithmeticOperators[:]},
		{CategoryNativeUnaryOperator, nativeUnaryOperators[:]},
		{CategoryNativeDivisorProperty, nativeDivisorProperties[:]},
		{CategoryNativeTruthinessClass, nativeTruthinessClasses[:]},
		{CategoryNativeBranchPartition, nativeBranchPartitions[:]},
		{CategoryNativeBranchArm, nativeBranchArms[:]},
		{CategoryNativeNumericOverflow, nativeNumericOverflows[:]},
	}
	specs := make([]Spec, 0, 32)
	for _, declaration := range declarations {
		for index, member := range declaration.members {
			specs = append(specs, Spec{
				Key:      member.key,
				Category: declaration.category,
				Ordinal:  uint16(index + 1),
				Spelling: member.spelling,
				Accepted: true,
			})
		}
	}
	return specs
}
