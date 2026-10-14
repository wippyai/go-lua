package lua

import "reflect"

const defaultArrayCap = 32
const defaultHashCap = 32
const maxArrayGap = defaultArrayCap

// iterationPointerKey is only a map key, never a Lua value. It lets deleted
// pointer/channel keys remain valid cursors without retaining their objects.
// A caller using such a cursor still owns the original key. Live keys keep
// their original map identity and are checked first, so a recycled address
// cannot shadow a newly inserted key before dead cursors are compacted.
type iterationPointerKey struct {
	typ     reflect.Type
	address uintptr
}

func (iterationPointerKey) String() string   { return "<iteration pointer>" }
func (iterationPointerKey) Type() LValueType { return LTNil }

func iterationKey(key LValue) LValue {
	v := reflect.ValueOf(key)
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Chan {
		return iterationPointerKey{typ: v.Type(), address: v.Pointer()}
	}
	return key
}

type lValueArraySorter struct {
	L      *LState
	Fn     *LFunction
	Values []LValue
}

func (lv lValueArraySorter) Len() int {
	return len(lv.Values)
}

func (lv lValueArraySorter) Swap(i, j int) {
	lv.Values[i], lv.Values[j] = lv.Values[j], lv.Values[i]
}

func (lv lValueArraySorter) Less(i, j int) bool {
	if lv.Fn != nil {
		lv.L.Push(lv.Fn)
		lv.L.Push(lv.Values[i])
		lv.L.Push(lv.Values[j])
		lv.L.Call(2, 1)
		return LVAsBool(lv.L.reg.Pop())
	}
	return lessThan(lv.L, lv.Values[i], lv.Values[j])
}

func newLTable(acap int, hcap int) *LTable {
	if acap < 0 {
		acap = 0
	}
	if hcap < 0 {
		hcap = 0
	}
	tb := &LTable{}
	tb.Metatable = LNil
	tb.Immutable = false
	if acap != 0 {
		tb.Array = make([]LValue, 0, acap)
	}
	if hcap != 0 {
		tb.Strdict = make(map[string]LValue, hcap)
	}
	// LAZY OPTIMIZATION: Keys, K2i, and Dict are nil until actually needed
	// Keys/K2i only created when Next() is called
	// Dict only created when non-string keys are used
	return tb
}

func CreateTable(acap, hcap int) *LTable {
	return newLTable(acap, hcap)
}

// ensureIterationOrder creates Keys/K2i only when needed (Next() called)
func (tb *LTable) ensureIterationOrder() {
	if tb.Keys != nil {
		return // Already initialized
	}

	// Initialize iteration order tracking
	totalKeys := len(tb.Strdict)
	if tb.Dict != nil {
		totalKeys += len(tb.Dict)
	}

	tb.Keys = make([]LValue, 0, totalKeys)
	tb.K2i = make(map[LValue]int)

	// Add all existing string keys in arbitrary order (Lua compliant)
	for k := range tb.Strdict {
		lkey := LString(k)
		tb.K2i[lkey] = len(tb.Keys)
		tb.Keys = append(tb.Keys, lkey)
	}

	// Add all existing non-string keys (if Dict exists)
	if tb.Dict != nil {
		for k := range tb.Dict {
			tb.K2i[k] = len(tb.Keys)
			tb.Keys = append(tb.Keys, k)
		}
	}
}

// deleteIterationKey leaves a stable slot and cursor for next(t, key), even
// after t[key] = nil. Moving another key into this slot would skip that key.
func (tb *LTable) deleteIterationKey(key LValue) {
	if index, ok := tb.K2i[key]; ok {
		// Release the slice reference; K2i keeps only the cursor identity until
		// a new field is inserted. Deletion alone must not invalidate cursors,
		// including cursors in nested traversals of the same table.
		tb.Keys[index] = nil
		if cursor := iterationKey(key); cursor != key {
			delete(tb.K2i, key)
			tb.K2i[cursor] = index
		}
	}
}

// addIterationKey is called before storing a non-nil hash value.
func (tb *LTable) addIterationKey(key LValue) {
	if tb.Keys == nil {
		return
	}
	if index, ok := tb.K2i[key]; ok && tb.Keys[index] != nil {
		return
	}
	// Inserting a previously absent field may invalidate traversal in Lua.
	// Reclaim deleted cursors once at least half the slots are dead. This
	// bounds metadata while amortizing the rebuild over intervening mutations;
	// compacting after every deletion makes ordinary hash churn quadratic.
	// Starting another traversal must not compact it: an outer traversal may
	// still hold a deleted cursor.
	live := len(tb.Strdict) + len(tb.Dict)
	if len(tb.Keys) > live && len(tb.Keys)-live >= live {
		clear(tb.K2i)
		n := 0
		for _, existing := range tb.Keys {
			if existing != nil {
				tb.K2i[existing] = n
				tb.Keys[n] = existing
				n++
			}
		}
		clear(tb.Keys[n:])
		tb.Keys = tb.Keys[:n]
	}
	tb.K2i[key] = len(tb.Keys)
	tb.Keys = append(tb.Keys, key)
}

// forgetDeletedArrayKey retires old sparse cursors when their position becomes
// part of the array. They must not reappear if Remove later shrinks the array.
// Live explicit hash keys keep their own cursor, including integer Go keys.
func (tb *LTable) forgetDeletedArrayKey(key int) {
	if len(tb.K2i) == 0 {
		return
	}
	for _, candidate := range [2]LValue{LNumber(key), LInteger(key)} {
		if index, ok := tb.K2i[candidate]; ok && tb.Keys[index] == nil {
			delete(tb.K2i, candidate)
		}
	}
}

// Len returns length of this LTable without using __len.
func (tb *LTable) Len() int {
	if tb.Array == nil {
		return 0
	}
	var prev = LNil
	for i := len(tb.Array) - 1; i >= 0; i-- {
		v := tb.Array[i]
		if prev == LNil && v != LNil {
			return i + 1
		}
		prev = v
	}
	return 0
}

// Append appends a given LValue to this LTable.
func (tb *LTable) Append(value LValue) bool {
	if tb.Immutable {
		return false
	}
	if value == LNil {
		return true
	}
	if tb.Array == nil {
		tb.Array = make([]LValue, 0, defaultArrayCap)
	}
	if len(tb.Array) == 0 || tb.Array[len(tb.Array)-1] != LNil {
		tb.Array = append(tb.Array, value)
		tb.forgetDeletedArrayKey(len(tb.Array))
		if len(tb.Dict) > 0 {
			tb.pullHashed()
		}
	} else {
		i := len(tb.Array) - 2
		for ; i >= 0; i-- {
			if tb.Array[i] != LNil {
				break
			}
		}
		tb.Array[i+1] = value
		tb.forgetDeletedArrayKey(i + 2)
	}
	return true
}

// Insert inserts a given LValue at position `i` in this table.
func (tb *LTable) Insert(i int, value LValue) bool {
	if tb.Immutable {
		return false
	}
	length := tb.Len()
	if i > length {
		return tb.RawSetInt(i, value)
	}
	if i <= 0 {
		return tb.RawSet(LNumber(i), value)
	}
	i -= 1
	if length == len(tb.Array) {
		tb.Array = append(tb.Array, LNil)
	}
	copy(tb.Array[i+1:length+1], tb.Array[i:length])
	tb.Array[i] = value
	tb.forgetDeletedArrayKey(length + 1)
	tb.pullHashed()
	return true
}

// MaxN returns the largest present integer key in the array-index range,
// including sparse keys held in the hash part.
func (tb *LTable) MaxN() int {
	max := 0
	for i := len(tb.Array) - 1; i >= 0; i-- {
		if tb.Array[i] != LNil {
			max = i + 1
			break
		}
	}
	for key, value := range tb.Dict {
		if value == LNil {
			continue
		}
		switch k := key.(type) {
		case LNumber:
			if isArrayKey(k) && int(k) > max {
				max = int(k)
			}
		case LInteger:
			if k > LInteger(max) && k < LInteger(MaxArrayIndex) {
				max = int(k)
			}
		}
	}
	return max
}

// Remove removes from this table the element at a given position.
func (tb *LTable) Remove(pos int) (LValue, bool) {
	if tb.Immutable {
		return LNil, false
	}
	if tb.Array == nil {
		return LNil, true
	}
	larray := tb.Len()
	if larray == 0 {
		return LNil, true
	}
	i := pos - 1
	oldval := LNil
	switch {
	case i >= larray:
		// nothing to do
		return oldval, true
	case i == larray-1 || i < 0:
		oldval = tb.Array[larray-1]
		tb.Array[larray-1] = LNil
		tb.Array = tb.Array[:larray-1]
	default:
		oldval = tb.Array[i]
		copy(tb.Array[i:larray-1], tb.Array[i+1:larray])
		tb.Array[larray-1] = LNil
		tb.Array = tb.Array[:larray-1]
	}
	tb.forgetDeletedArrayKey(larray)
	return oldval, true
}

// RawSet sets a given LValue to a given index without the __newindex metamethod.
// It is recommended to use `RawSetString` or `RawSetInt` for performance
// if you already know the given LValue is a string or number.
func (tb *LTable) RawSet(key LValue, value LValue) bool {
	if tb.Immutable {
		return false
	}
	switch v := key.(type) {
	case LNumber:
		if isArrayKey(v) {
			index := int(v) - 1
			switch {
			case index < len(tb.Array):
				tb.Array[index] = value
				tb.forgetDeletedArrayKey(int(v))
			case index == len(tb.Array) && tb.Array != nil && value != LNil && len(tb.Dict) == 0:
				tb.Array = append(tb.Array, value)
				tb.forgetDeletedArrayKey(int(v))
			default:
				tb.setSparseIntKey(int(v), value)
			}
			return true
		}
	case LInteger:
		if v > 0 && v < LInteger(MaxArrayIndex) {
			iv := int(v)
			index := iv - 1
			switch {
			case index < len(tb.Array):
				tb.Array[index] = value
				tb.forgetDeletedArrayKey(iv)
			case index == len(tb.Array) && tb.Array != nil && value != LNil && len(tb.Dict) == 0:
				tb.Array = append(tb.Array, value)
				tb.forgetDeletedArrayKey(iv)
			default:
				tb.setSparseIntKey(iv, value)
			}
			return true
		}
	case LString:
		return tb.RawSetString(string(v), value)
	}

	return tb.RawSetH(key, value)
}

// RawSetInt sets a given LValue at a position `key` without the __newindex metamethod.
func (tb *LTable) RawSetInt(key int, value LValue) bool {
	if tb.Immutable {
		return false
	}
	if key < 1 || key >= MaxArrayIndex {
		return tb.RawSetH(LNumber(key), value)
	}
	index := key - 1
	switch {
	case index < len(tb.Array):
		tb.Array[index] = value
		tb.forgetDeletedArrayKey(key)
	case index == len(tb.Array) && tb.Array != nil && value != LNil && len(tb.Dict) == 0:
		tb.Array = append(tb.Array, value)
		tb.forgetDeletedArrayKey(key)
	default:
		tb.setSparseIntKey(key, value)
	}
	return true
}

func (tb *LTable) setSparseIntKey(key int, value LValue) {
	index := key - 1
	alen := len(tb.Array)
	gap := index - alen
	// A bounded gap keeps allocated holes proportional to writes. Comparing
	// against array length alone lets a few geometric keys allocate huge arrays.
	_, alreadyHashed := tb.Dict[LNumber(key)]
	// Updating an existing field must keep its iteration position. Proximity
	// to the array alone does not authorize promotion during an overwrite.
	if value == LNil || gap > maxArrayGap || alreadyHashed {
		sparseOrigin := tb.Dict == nil || tb.sparseArrayHash
		tb.RawSetH(LNumber(key), value)
		if sparseOrigin {
			tb.sparseArrayHash = true
			tb.releaseEmptySparseHash()
		}
		return
	}
	if tb.Array == nil {
		tb.Array = make([]LValue, 0, defaultArrayCap)
	}
	for position := alen + 1; position < key; position++ {
		tb.Array = append(tb.Array, tb.takeHashed(position))
	}
	tb.takeHashed(key)
	tb.Array = append(tb.Array, value)
	tb.pullHashed()
}

func (tb *LTable) getIntKey(key int) LValue {
	index := key - 1
	if index < len(tb.Array) {
		return tb.Array[index]
	}
	if tb.Dict == nil {
		return LNil
	}
	if v, ok := tb.Dict[LNumber(key)]; ok {
		return v
	}
	return LNil
}

func (tb *LTable) takeHashed(key int) LValue {
	tb.forgetDeletedArrayKey(key)
	if len(tb.Dict) == 0 {
		return LNil
	}
	hashed := LNumber(key)
	v, ok := tb.Dict[hashed]
	if !ok {
		return LNil
	}
	tb.RawSetH(hashed, LNil)
	// Promotion makes this an array cursor, not a deleted hash cursor.
	delete(tb.K2i, hashed)
	return v
}

func (tb *LTable) pullHashed() {
	for len(tb.Dict) > 0 {
		v := tb.takeHashed(len(tb.Array) + 1)
		if v == LNil {
			return
		}
		tb.Array = append(tb.Array, v)
	}
}

func (tb *LTable) releaseEmptySparseHash() {
	if tb.sparseArrayHash && len(tb.Dict) == 0 {
		// Numeric-only hash storage is an array implementation detail. Retaining
		// an empty Dict would make existing encoders classify it as an object.
		tb.Dict = nil
		tb.sparseArrayHash = false
	}
}

// RawSetString sets a given LValue to a given string index without the __newindex metamethod.
func (tb *LTable) RawSetString(key string, value LValue) bool {
	if tb.Immutable {
		return false
	}

	if tb.Strdict == nil {
		tb.Strdict = make(map[string]LValue, defaultHashCap)
	}

	lkey := LString(key)
	if value == LNil {
		delete(tb.Strdict, key)
		tb.deleteIterationKey(lkey)
	} else {
		tb.addIterationKey(lkey)
		tb.Strdict[key] = value
	}
	return true
}

// RawSetH sets a given LValue to a given index without the __newindex metamethod.
// OPTIMIZED: No longer creates Keys/K2i or Dict until actually needed
func (tb *LTable) RawSetH(key LValue, value LValue) bool {
	if tb.Immutable {
		return false
	}
	if s, ok := key.(LString); ok {
		return tb.RawSetString(string(s), value)
	}

	if value == LNil {
		// LAZY: Only delete if Dict exists
		if tb.Dict != nil {
			delete(tb.Dict, key)
		}
		tb.deleteIterationKey(key)
		tb.releaseEmptySparseHash()
	} else {
		// Explicit hash keys preserve the existing hash/object storage origin.
		tb.sparseArrayHash = false
		// LAZY: Only create Dict when storing non-string, non-int key
		if tb.Dict == nil {
			tb.Dict = make(map[LValue]LValue, 8) // Start small for rare use case
		}
		tb.addIterationKey(key)
		tb.Dict[key] = value
	}
	return true
}

// RawGet returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGet(key LValue) LValue {
	switch v := key.(type) {
	case LNumber:
		if isArrayKey(v) {
			return tb.getIntKey(int(v))
		}
	case LInteger:
		if v > 0 && v < LInteger(MaxArrayIndex) {
			return tb.getIntKey(int(v))
		}
	case LString:
		if tb.Strdict == nil {
			return LNil
		}
		if ret, ok := tb.Strdict[string(v)]; ok {
			return ret
		}
		return LNil
	}
	if tb.Dict == nil {
		return LNil
	}
	if v, ok := tb.Dict[key]; ok {
		return v
	}
	return LNil
}

// RawGetInt returns an LValue at position `key` without __index metamethod.
func (tb *LTable) RawGetInt(key int) LValue {
	if key < 1 {
		return LNil
	}
	return tb.getIntKey(key)
}

// RawGetH returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGetH(key LValue) LValue {
	if s, sok := key.(LString); sok {
		if tb.Strdict == nil {
			return LNil
		}
		if v, vok := tb.Strdict[string(s)]; vok {
			return v
		}
		return LNil
	}
	if tb.Dict == nil {
		return LNil
	}
	if v, ok := tb.Dict[key]; ok {
		return v
	}
	return LNil
}

// RawGetString returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGetString(key string) LValue {
	if tb.Strdict == nil {
		return LNil
	}
	if v, vok := tb.Strdict[key]; vok {
		return v
	}
	return LNil
}

// ForEach iterates over this table of elements, yielding each in turn to a given function.
func (tb *LTable) ForEach(cb func(LValue, LValue)) {
	if tb.Array != nil {
		for i, v := range tb.Array {
			if v != LNil {
				cb(LNumber(i+1), v)
			}
		}
	}
	if tb.Strdict != nil {
		for k, v := range tb.Strdict {
			if v != LNil {
				cb(LString(k), v)
			}
		}
	}
	if tb.Dict != nil {
		for k, v := range tb.Dict {
			if v != LNil {
				cb(k, v)
			}
		}
	}
}

// Next is equivalent to lua_next ( http://www.lua.org/manual/5.1/manual.html#lua_next ).
func (tb *LTable) Next(key LValue) (LValue, LValue) {
	// Lazy initialization - only create ordering when Next() is actually used
	tb.ensureIterationOrder()

	if k, ok := key.(LInteger); ok && k > 0 && k < LInteger(MaxArrayIndex) {
		if _, hashed := tb.K2i[key]; !hashed {
			key = LNumber(k)
		}
	}

	hashIndex := 0
	arrayIndex := 0
	if key != LNil {
		index, hashed := tb.K2i[key]
		if !hashed {
			index, hashed = tb.K2i[iterationKey(key)]
		}
		if hashed {
			// Sparse positive keys are hash keys, not a request to restart
			// array traversal. Continue from their actual iteration slot.
			hashIndex = index + 1
			arrayIndex = len(tb.Array)
		} else if k, ok := key.(LNumber); ok && isArrayKey(k) {
			arrayIndex = int(k)
		} else {
			return LNil, LNil
		}
	}

	for ; arrayIndex < len(tb.Array); arrayIndex++ {
		if value := tb.Array[arrayIndex]; value != LNil {
			return LNumber(arrayIndex + 1), value
		}
	}
	for i := hashIndex; i < len(tb.Keys); i++ {
		key := tb.Keys[i]
		if key == nil {
			continue
		}
		if v := tb.RawGetH(key); v != LNil {
			return key, v
		}
	}
	return LNil, LNil
}
