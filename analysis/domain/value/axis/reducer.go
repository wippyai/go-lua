package axis

import "fmt"

// Reader and Writer are the erased view reducers use to inspect and update a
// value product without depending on the product package.
type Reader interface {
	GetAny(key string) (any, bool)
}

type Writer interface {
	Reader
	SetAny(key string, value any)
}

// Reducer is an optional registry hook for reduced products. Its Spec declares
// (or derives) every axis it reads and writes. Writes must be reductive: each
// exact change is less than or equal to the previous value. Reducers must be
// monotone over their declared inputs. On finite-height axes, fair dependency
// scheduling therefore reaches the unique greatest common fixed point beneath
// the input product without a pass cap.
//
// The bool result is retained for source compatibility. Scheduling is derived
// exclusively from exact product changes, so a stale return value cannot cause
// either missed work or a blind extra pass.
type Reducer func(Writer) bool

// Get reads a typed value from a reducer view.
func Get[T any](r Reader, key Key[T]) (T, bool) {
	var zero T
	if r == nil {
		return zero, false
	}
	v, ok := r.GetAny(key.ID())
	if !ok {
		return zero, false
	}
	tv, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("axis %q: reader returned %T, want typed key value", key.ID(), v))
	}
	return tv, true
}

// Set writes a typed value to a reducer view.
func Set[T any](w Writer, key Key[T], value T) {
	if w == nil {
		return
	}
	w.SetAny(key.ID(), value)
}
