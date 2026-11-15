package lua

import (
	"math"
	"math/rand/v2"
)

func OpenMath(L *LState) int {
	mod := L.RegisterGoModule(MathLibName, mathFuncs).(*LTable)
	mod.RawSetString("pi", LNumber(math.Pi))
	mod.RawSetString("huge", LNumber(math.Inf(1)))
	mod.RawSetString("maxinteger", LInteger(math.MaxInt64))
	mod.RawSetString("mininteger", LInteger(math.MinInt64))
	L.Push(mod)
	return 1
}

var mathFuncs = map[string]LGoFunc{
	"abs":        mathAbs,
	"acos":       mathAcos,
	"asin":       mathAsin,
	"atan":       mathAtan,
	"atan2":      mathAtan2,
	"ceil":       mathCeil,
	"cos":        mathCos,
	"cosh":       mathCosh,
	"deg":        mathDeg,
	"exp":        mathExp,
	"floor":      mathFloor,
	"fmod":       mathFmod,
	"frexp":      mathFrexp,
	"ldexp":      mathLdexp,
	"log":        mathLog,
	"log10":      mathLog10,
	"max":        mathMax,
	"min":        mathMin,
	"mod":        mathMod,
	"modf":       mathModf,
	"pow":        mathPow,
	"rad":        mathRad,
	"random":     mathRandom,
	"randomseed": mathRandomSeed,
	"sin":        mathSin,
	"sinh":       mathSinh,
	"sqrt":       mathSqrt,
	"tan":        mathTan,
	"tanh":       mathTanh,
	"tointeger":  mathToInteger,
	"type":       mathType,
	"ult":        mathUlt,
}

func mathAbs(L *LState) int {
	if n, ok := L.Get(1).(LInteger); ok {
		if n < 0 {
			n = -n
		}
		L.Push(lintegerToValue(n))
		return 1
	}
	L.Push(LNumber(math.Abs(float64(L.CheckNumber(1)))))
	return 1
}

func mathAcos(L *LState) int {
	L.Push(LNumber(math.Acos(float64(L.CheckNumber(1)))))
	return 1
}

func mathAsin(L *LState) int {
	L.Push(LNumber(math.Asin(float64(L.CheckNumber(1)))))
	return 1
}

func mathAtan(L *LState) int {
	L.Push(LNumber(math.Atan(float64(L.CheckNumber(1)))))
	return 1
}

func mathAtan2(L *LState) int {
	L.Push(LNumber(math.Atan2(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

func mathCeil(L *LState) int {
	if n, ok := L.Get(1).(LInteger); ok {
		L.Push(lintegerToValue(n))
		return 1
	}
	L.Push(integralFloatValue(LNumber(math.Ceil(float64(L.CheckNumber(1))))))
	return 1
}

func mathCos(L *LState) int {
	L.Push(LNumber(math.Cos(float64(L.CheckNumber(1)))))
	return 1
}

func mathCosh(L *LState) int {
	L.Push(LNumber(math.Cosh(float64(L.CheckNumber(1)))))
	return 1
}

func mathDeg(L *LState) int {
	L.Push(LNumber(float64(L.CheckNumber(1)) * 180 / math.Pi))
	return 1
}

func mathExp(L *LState) int {
	L.Push(LNumber(math.Exp(float64(L.CheckNumber(1)))))
	return 1
}

func mathFloor(L *LState) int {
	if n, ok := L.Get(1).(LInteger); ok {
		L.Push(lintegerToValue(n))
		return 1
	}
	L.Push(integralFloatValue(LNumber(math.Floor(float64(L.CheckNumber(1))))))
	return 1
}

func mathFmod(L *LState) int {
	if a, ok := L.Get(1).(LInteger); ok {
		if d, ok := L.Get(2).(LInteger); ok {
			switch d {
			case 0:
				L.ArgError(2, "zero")
			case -1:
				L.Push(lintegerToValue(0))
			default:
				L.Push(lintegerToValue(a % d))
			}
			return 1
		}
	}
	L.Push(LNumber(math.Mod(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

func mathFrexp(L *LState) int {
	v1, v2 := math.Frexp(float64(L.CheckNumber(1)))
	L.Push(LNumber(v1))
	L.Push(LNumber(v2))
	return 2
}

func mathLdexp(L *LState) int {
	L.Push(LNumber(math.Ldexp(float64(L.CheckNumber(1)), L.CheckInt(2))))
	return 1
}

func mathLog(L *LState) int {
	L.Push(LNumber(math.Log(float64(L.CheckNumber(1)))))
	return 1
}

func mathLog10(L *LState) int {
	L.Push(LNumber(math.Log10(float64(L.CheckNumber(1)))))
	return 1
}

func mathMax(L *LState) int {
	top := L.GetTop()
	if top < 1 {
		L.ArgError(1, "value expected")
	}
	maxVal := L.Get(1)
	for i := 2; i <= top; i++ {
		if v := L.Get(i); lessThan(L, maxVal, v) {
			maxVal = v
		}
	}
	L.Push(maxVal)
	return 1
}

func mathMin(L *LState) int {
	top := L.GetTop()
	if top < 1 {
		L.ArgError(1, "value expected")
	}
	minVal := L.Get(1)
	for i := 2; i <= top; i++ {
		if v := L.Get(i); lessThan(L, v, minVal) {
			minVal = v
		}
	}
	L.Push(minVal)
	return 1
}

func mathMod(L *LState) int {
	lhs := L.CheckNumber(1)
	rhs := L.CheckNumber(2)
	L.Push(luaModulo(lhs, rhs))
	return 1
}

func mathModf(L *LState) int {
	if n, ok := L.Get(1).(LInteger); ok {
		L.Push(lintegerToValue(n))
		L.Push(LNumber(0))
		return 2
	}
	n := float64(L.CheckNumber(1))
	ip := math.Trunc(n)
	L.Push(integralFloatValue(LNumber(ip)))
	if n == ip {
		L.Push(LNumber(0))
	} else {
		L.Push(LNumber(n - ip))
	}
	return 2
}

func mathPow(L *LState) int {
	L.Push(LNumber(math.Pow(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

func mathRad(L *LState) int {
	L.Push(LNumber(float64(L.CheckNumber(1)) * math.Pi / 180))
	return 1
}

func mathRandom(L *LState) int {
	var low, up LInteger
	switch L.GetTop() {
	case 0:
		L.Push(LNumber(rand.Float64()))
		return 1
	case 1:
		low, up = 1, checkInteger(L, 1)
		if up == 0 {
			L.Push(lintegerToValue(LInteger(rand.Uint64())))
			return 1
		}
	case 2:
		low, up = checkInteger(L, 1), checkInteger(L, 2)
	default:
		L.RaiseError("wrong number of arguments")
	}
	if low > up {
		L.ArgError(1, "interval is empty")
	}
	L.Push(lintegerToValue(low + LInteger(randomBelowOrEqual(uint64(up)-uint64(low)))))
	return 1
}

// randomBelowOrEqual returns a uniform value in [0, n].
func randomBelowOrEqual(n uint64) uint64 {
	if n == math.MaxUint64 {
		return rand.Uint64()
	}
	return rand.Uint64N(n + 1)
}

// integralFloatValue converts an integral float to an integer when it fits,
// as pushnumint does in lmathlib.c; other floats stay floats.
func integralFloatValue(f LNumber) LValue {
	if n, ok := floatToInteger(f); ok {
		return lintegerToValue(n)
	}
	return f
}

func mathRandomSeed(L *LState) int {
	// rand/v2 doesn't require explicit seeding - it auto-seeds from crypto/rand
	// This function is kept for Lua compatibility but is effectively a no-op
	// If explicit seeding is needed, use a custom rand.Source
	return 0
}

func mathSin(L *LState) int {
	L.Push(LNumber(math.Sin(float64(L.CheckNumber(1)))))
	return 1
}

func mathSinh(L *LState) int {
	L.Push(LNumber(math.Sinh(float64(L.CheckNumber(1)))))
	return 1
}

func mathSqrt(L *LState) int {
	L.Push(LNumber(math.Sqrt(float64(L.CheckNumber(1)))))
	return 1
}

func mathTan(L *LState) int {
	L.Push(LNumber(math.Tan(float64(L.CheckNumber(1)))))
	return 1
}

func mathTanh(L *LState) int {
	L.Push(LNumber(math.Tanh(float64(L.CheckNumber(1)))))
	return 1
}

func mathToInteger(L *LState) int {
	if n, ok := exactInteger(L.Get(1)); ok {
		L.Push(n)
	} else {
		L.Push(LNil)
	}
	return 1
}

func mathType(L *LState) int {
	v := L.Get(1)
	switch v.(type) {
	case LInteger:
		L.Push(LString("integer"))
	case LNumber:
		L.Push(LString("float"))
	default:
		L.Push(LNil)
	}
	return 1
}

func mathUlt(L *LState) int {
	m := uint64(L.CheckInt64(1))
	n := uint64(L.CheckInt64(2))
	L.Push(LBool(m < n))
	return 1
}

//
