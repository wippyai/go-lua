package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// MathRoot is the authored mount selector of the math library.
const MathRoot = "math"

// mathExports is the authored export inventory of the math library, in
// canonical order. Each name is one direct export of the contract root.
//
// The inventory is authored rather than derived, for the reason the string
// library states: an instance that computed its members from the table it
// replaces would agree with that table by construction instead of by check. The
// drift law holds this list against the modeled standard library.
//
// The library publishes no metatable edge: a number reaches no method through
// math, and attaching a metatable to the number primitive is the environment's
// business in any case.
var mathExports = []string{
	"abs", "acos", "asin", "atan", "atan2", "ceil", "cos", "cosh", "deg", "exp",
	"floor", "fmod", "frexp", "ldexp", "log", "log10", "max", "min", "mod",
	"modf", "pow", "rad", "random", "randomseed", "sin", "sinh", "sqrt", "tan",
	"tanh", "tointeger", "type", "ult",
}

// mathConstants are the exported values of the math library that are not
// callables, in authored order. Each is one value from the closed literal domain
// of the language, and each terminates the path it is reached by: there is no
// member of math.pi to address.
//
// The two floats are written as their exact IEEE-754 bits rather than as a
// decimal spelling. math.huge has no decimal spelling at all, and pi's nearest
// double is not the number a reader would write down, so the bits are the value
// and a text codec would be a second, lossy spelling of it.
//
// The mutability is the library's own statement about its export, as it is for
// the root: the language places no seal on math.pi, and whether the host boots
// the aggregate frozen is the initial environment's business.
var mathConstants = []valueExport{
	constantExport("huge", contract.Constant{Kind: contract.ConstantFloat, FloatBits: 0x7ff0000000000000}, contract.MutabilityMutable),
	constantExport("maxinteger", contract.Constant{Kind: contract.ConstantInteger, Integer: 9223372036854775807}, contract.MutabilityMutable),
	constantExport("mininteger", contract.Constant{Kind: contract.ConstantInteger, Integer: -9223372036854775808}, contract.MutabilityMutable),
	constantExport("pi", contract.Constant{Kind: contract.ConstantFloat, FloatBits: 0x400921fb54442d18}, contract.MutabilityMutable),
}

// MathExports returns a copy of the authored export inventory.
func MathExports() []string { return copyNames(mathExports) }

// MathContract authors the math library contract instance against one declared
// library kind. Every export is a callable and carries its typed application
// envelope, the four published constants carry the values they are, and the root
// carries what the library is: a mutable aggregate. Nothing is deferred.
func MathContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:       MathRoot,
		Exports:    mathExports,
		Signatures: mathSignatures,
		Aggregate:  contract.Aggregate(contract.MutabilityMutable),
		Values:     mathConstants,
	}.instance(kind)
}
