package lua

// Debug hook masks, matching the C API constants (lua.h LUA_MASKxxx).
const (
	HookMaskCall  uint8 = 1 << iota // "c"
	HookMaskReturn                  // "r"
	HookMaskLine                    // "l"
	HookMaskCount                   // set when a count > 0 is given
)

// callHook is invoked from the interpreter dispatch loop before each
// instruction executes, but only when L.hookMask != 0. It emits the "count"
// and "line" debug events. It is a no-op while a hook is already running
// (L.inHook) so a hook cannot recursively trigger itself.
func (ls *LState) callHook(cf *callFrame) {
	if ls.inHook {
		return
	}

	if ls.hookMask&HookMaskCount != 0 && ls.hookCount > 0 {
		ls.hookCounter++
		if ls.hookCounter >= ls.hookCount {
			ls.hookCounter = 0
			ls.fireHook("count", -1)
			cf = ls.currentFrame
		}
	}

	if ls.hookMask&HookMaskLine != 0 && cf.Fn != nil {
		positions := cf.Fn.Proto.DbgSourcePositions
		pc := int(cf.Pc - 1)
		if pc >= 0 && pc < len(positions) {
			line := int32(positions[pc])
			if line != ls.hookLastLine {
				ls.hookLastLine = line
				ls.fireHook("line", line)
			}
		}
	}
}

// fireHook calls the registered hook function as hook(event, line). For events
// without a line ("count") the second argument is nil. Reentrancy is blocked
// via ls.inHook for the duration of the call. The register top is saved and
// restored so the hook is transparent to the interrupted instruction.
func (ls *LState) fireHook(event string, line int32) {
	if ls.hook == nil || ls.hook == LNil {
		return
	}

	top := ls.reg.Top()
	ls.inHook = true
	defer func() {
		ls.inHook = false
		ls.reg.SetTop(top)
	}()

	ls.Push(ls.hook)
	ls.Push(LString(event))
	if line >= 0 {
		ls.Push(LNumber(line))
	} else {
		ls.Push(LNil)
	}
	ls.Call(2, 0)
}

func parseHookMask(mask string, count int) uint8 {
	var m uint8
	for _, c := range mask {
		switch c {
		case 'c':
			m |= HookMaskCall
		case 'r':
			m |= HookMaskReturn
		case 'l':
			m |= HookMaskLine
		}
	}
	if count > 0 {
		m |= HookMaskCount
	}
	return m
}

func hookMaskString(mask uint8) string {
	s := ""
	if mask&HookMaskCall != 0 {
		s += "c"
	}
	if mask&HookMaskReturn != 0 {
		s += "r"
	}
	if mask&HookMaskLine != 0 {
		s += "l"
	}
	return s
}

// debugSetHook implements debug.sethook([hook, mask [, count]]).
// Only "line" and "count" events are emitted; requesting "call"/"return"
// hooks is rejected rather than silently ignored.
func debugSetHook(L *LState) int {
	if L.GetTop() == 0 {
		L.hook = LNil
		L.hookMask = 0
		L.hookCount = 0
		L.hookCounter = 0
		return 0
	}

	hook := L.Get(1)
	if hook == LNil {
		L.hook = LNil
		L.hookMask = 0
		L.hookCount = 0
		L.hookCounter = 0
		return 0
	}
	if _, ok := hook.(*LFunction); !ok {
		L.ArgError(1, "function expected")
	}

	mask := L.CheckString(2)
	count := L.OptInt(3, 0)
	for _, c := range mask {
		if c == 'c' || c == 'r' {
			L.RaiseError("debug.sethook: call/return hooks are not supported (line/count only)")
		}
	}

	L.hook = hook
	L.hookMask = parseHookMask(mask, count)
	L.hookCount = count
	L.hookCounter = 0
	L.hookLastLine = 0
	return 0
}

// debugGetHook implements debug.gethook(), returning
// (hook function, mask string, count).
func debugGetHook(L *LState) int {
	if L.hookMask == 0 || L.hook == nil {
		L.Push(LNil)
		L.Push(LString(""))
		L.Push(LNumber(0))
		return 3
	}
	L.Push(L.hook)
	L.Push(LString(hookMaskString(L.hookMask)))
	L.Push(LNumber(L.hookCount))
	return 3
}
