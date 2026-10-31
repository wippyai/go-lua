package lua

// Debug hook masks, matching the C API constants (lua.h LUA_MASKxxx).
const (
	HookMaskCall   uint8 = 1 << iota // "c"
	HookMaskReturn                   // "r"
	HookMaskLine                     // "l"
	HookMaskCount                    // set when a count > 0 is given
)

// callHook is invoked from the interpreter dispatch loop before each
// instruction executes when a debug hook or coverage is active. It records
// coverage before dispatching "count" and "line" debug events. User callbacks
// cannot recursively trigger themselves while L.inHook is set.
func (ls *LState) callHook(cf *callFrame) {
	// Coverage consumes the same dispatch as debug hooks, including dispatches
	// inside a callback. User hook masks and reentrancy must not disable it.
	pc := int(cf.Pc - 1)
	line := int32(-1)
	if (covOn || ls.hookMask&HookMaskLine != 0) && cf.Fn != nil {
		positions := cf.Fn.Proto.DbgSourcePositions
		if pc >= 0 && pc < len(positions) {
			line = int32(positions[pc])
			if covOn {
				covRecordHit(cf.Fn.Proto.SourceName, int(line))
			}
		}
	}
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

	if ls.hookMask&HookMaskLine != 0 && line >= 0 {
		// A source line alone cannot identify a dispatch location: two
		// chunks can use the same line number, and loops revisit a line.
		fire := cf.Fn.Proto != ls.hookLastProto || pc <= ls.hookLastPC || line != ls.hookLastLine
		ls.hookLastProto = cf.Fn.Proto
		ls.hookLastPC = pc
		ls.hookLastLine = line
		if fire {
			ls.fireHook("line", line)
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

// clearHook releases callbacks and dispatch history before disabling or pooling a state.
func (ls *LState) clearHook() {
	ls.hook = nil
	ls.hookMask = 0
	ls.hookCount = 0
	ls.hookCounter = 0
	ls.hookLastLine = 0
	ls.hookLastProto = nil
	ls.hookLastPC = 0
}

// debugSetHook implements debug.sethook([hook, mask [, count]]).
// Only "line" and "count" events are emitted; requesting "call"/"return"
// hooks is rejected rather than silently ignored.
func debugSetHook(L *LState) int {
	if L.GetTop() == 0 {
		L.clearHook()
		return 0
	}

	hook := L.Get(1)
	if hook == LNil {
		L.clearHook()
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
