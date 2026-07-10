package lua

import "fmt"

// WrapErrorWithLua wraps an error, captures Lua stack trace, and sets the error metatable.
func WrapErrorWithLua(l *LState, err error, context string) *Error {
	e := wrapError(err, context, currentErrorMetadataExtractor())
	if e != nil && l != nil {
		e.LuaStack = captureStackTrace(l)
		SetErrorMetatable(l, e)
	}
	return e
}

// captureStackTrace captures a Lua stack trace from an LState.
func captureStackTrace(l *LState) *StackTrace {
	trace := &StackTrace{
		ThreadID: fmt.Sprintf("%p", l),
	}

	for level := 0; ; level++ {
		ar, ok := l.GetStack(level)
		if !ok {
			break
		}

		if _, err := l.GetInfo("nSluf", ar, nil); err != nil {
			break
		}

		frame := StackFrame{
			Level:       level,
			Source:      ar.Source,
			CurrentLine: ar.CurrentLine,
			Name:        ar.Name,
			FuncType:    ar.What,
		}
		trace.Frames = append(trace.Frames, frame)
	}

	return trace
}
