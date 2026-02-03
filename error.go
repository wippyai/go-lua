package lua

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

// StackFrame represents a single frame in the Lua stack (for error reporting).
type StackFrame struct {
	Level       int
	Source      string
	CurrentLine int
	Name        string
	FuncType    string
}

func (sf StackFrame) String() string {
	if sf.Name != "" {
		return fmt.Sprintf("%s:%d (%s)", sf.Source, sf.CurrentLine, sf.Name)
	}
	return fmt.Sprintf("%s:%d", sf.Source, sf.CurrentLine)
}

// StackTrace represents a Lua stack trace.
type StackTrace struct {
	ThreadID string
	Frames   []StackFrame
}

func (st StackTrace) String() string {
	var sb strings.Builder
	for _, frame := range st.Frames {
		sb.WriteString("  ")
		sb.WriteString(frame.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

// Kind categorizes errors semantically.
type Kind string

const (
	Unknown          Kind = ""
	NotFound         Kind = "NotFound"
	AlreadyExists    Kind = "AlreadyExists"
	Invalid          Kind = "Invalid"
	PermissionDenied Kind = "PermissionDenied"
	Unavailable      Kind = "Unavailable"
	Internal         Kind = "Internal"
	Canceled         Kind = "Canceled"
	Conflict         Kind = "Conflict"
	Timeout          Kind = "Timeout"
	RateLimited      Kind = "RateLimited"
)

func (k Kind) String() string {
	if k == "" {
		return "Unknown"
	}
	return string(k)
}

// Ternary represents three-state logic for composable error handling.
type Ternary int8

const (
	TernaryUnknown Ternary = 0
	TernaryTrue    Ternary = 1
	TernaryFalse   Ternary = 2
)

func (t Ternary) Bool() bool {
	return t == TernaryTrue
}

func (t Ternary) String() string {
	switch t {
	case TernaryUnknown:
		return "Unknown"
	case TernaryTrue:
		return "True"
	case TernaryFalse:
		return "False"
	default:
		return "Unknown"
	}
}

// Error is the unified error type for go-lua.
// It implements both error and LValue interfaces.
// Behaves like a string for concatenation and tostring().
type Error struct {
	Err       error          // Wrapped error (error chain)
	Message   string         // Error message
	LuaStack  *StackTrace    // Lua stack at wrap point
	goStack   []uintptr      // Go stack at wrap point
	Context   string         // Context description
	kind      Kind           // Error category
	retryable *bool          // nil=unknown, true/false
	details   map[string]any // Structured metadata
}

// NewError creates a new error with the given message.
func NewError(message string) *Error {
	e := &Error{
		Message: message,
	}
	e.captureGoStack()
	return e
}

// NewErrorf creates a new error with formatted message.
func NewErrorf(format string, args ...any) *Error {
	return NewError(fmt.Sprintf(format, args...))
}

// WrapError wraps an existing Go error with context.
func WrapError(err error, context string) *Error {
	if err == nil {
		return nil
	}
	e := &Error{
		Err:     err,
		Message: err.Error(),
		Context: context,
	}
	e.captureGoStack()

	// Preserve metadata from wrapped *Error if it has any
	var we *Error
	if errors.As(err, &we) {
		if we.kind != "" {
			e.kind = we.kind
		}
		if we.retryable != nil {
			e.retryable = we.retryable
		}
		if len(we.details) > 0 {
			e.details = we.details
		}
	}

	// Extract metadata from errors implementing Kind/Retryable/Details methods.
	// This handles apierror.Error and similar interfaces across package boundaries.
	// Only extract if not already set from wrapped *Error
	if e.kind == "" || e.retryable == nil {
		extractErrorMetadata(err, e)
	}

	return e
}

// extractKind uses reflection to call Kind() method on an error and convert
// the result to Kind. This handles the case where different packages define
// their own Kind type but all have String() method.
func extractKind(err error) Kind {
	v := reflect.ValueOf(err)
	if !v.IsValid() {
		return ""
	}

	// Look for Kind() method
	m := v.MethodByName("Kind")
	if !m.IsValid() {
		return ""
	}

	// Call the method (should take no args)
	if m.Type().NumIn() != 0 {
		return ""
	}

	results := m.Call(nil)
	if len(results) != 1 {
		return ""
	}

	result := results[0]
	if !result.IsValid() {
		return ""
	}

	// Unwrap interface{} if needed
	if result.Kind() == reflect.Interface && !result.IsNil() {
		result = result.Elem()
	}

	if !result.IsValid() {
		return ""
	}

	// Try to get string value
	// First check if it's directly a string
	if result.Kind() == reflect.String {
		if s := result.String(); s != "" {
			return Kind(s)
		}
		return ""
	}

	// Check if it has a String() method
	if sm := result.MethodByName("String"); sm.IsValid() && sm.Type().NumIn() == 0 {
		strResults := sm.Call(nil)
		if len(strResults) == 1 && strResults[0].Kind() == reflect.String {
			if s := strResults[0].String(); s != "" {
				return Kind(s)
			}
		}
	}

	return ""
}

// extractErrorMetadata extracts kind, retryable, and details from errors
// that implement the corresponding methods. This enables cross-package
// error metadata preservation (e.g., apierror.Error -> lua.Error).
// Walks the error chain to find the first error with each piece of metadata.
func extractErrorMetadata(err error, target *Error) {
	// Walk the error chain to find metadata
	for e := err; e != nil; e = errors.Unwrap(e) {
		// Extract Kind if not already set
		if target.kind == "" {
			target.kind = extractKind(e)
		}

		// Extract Retryable if not already set
		if target.retryable == nil {
			if re, ok := e.(interface {
				Retryable() interface{ Bool() bool }
			}); ok {
				b := re.Retryable().Bool()
				target.retryable = &b
			} else if re, ok := e.(interface{ Retryable() bool }); ok {
				b := re.Retryable()
				target.retryable = &b
			} else if re, ok := e.(interface{ Retryable() interface{} }); ok {
				if r := re.Retryable(); r != nil {
					switch v := r.(type) {
					case bool:
						target.retryable = &v
					case interface{ Bool() bool }:
						b := v.Bool()
						target.retryable = &b
					case int, int8, int16, int32, int64:
						var intVal int64
						switch iv := v.(type) {
						case int:
							intVal = int64(iv)
						case int8:
							intVal = int64(iv)
						case int16:
							intVal = int64(iv)
						case int32:
							intVal = int64(iv)
						case int64:
							intVal = iv
						}
						switch intVal {
						case 1:
							b := true
							target.retryable = &b
						case 2:
							b := false
							target.retryable = &b
						}
					}
				}
			}
		}

		// Extract Details if not already set
		if len(target.details) == 0 {
			if de, ok := e.(interface{ Details() interface{} }); ok {
				if d := de.Details(); d != nil {
					switch v := d.(type) {
					case map[string]any:
						target.details = v
					case interface{ AsMap() map[string]any }:
						target.details = v.AsMap()
					}
				}
			}
		}

		// Stop if we've found all metadata
		if target.kind != "" && target.retryable != nil && len(target.details) > 0 {
			break
		}
	}
}

// WrapErrorWithLua wraps an error, captures Lua stack trace, and sets the error metatable.
func WrapErrorWithLua(l *LState, err error, context string) *Error {
	e := WrapError(err, context)
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

func (e *Error) captureGoStack() {
	const maxDepth = 32
	e.goStack = make([]uintptr, maxDepth)
	n := runtime.Callers(3, e.goStack) // skip Callers, captureGoStack, and caller
	e.goStack = e.goStack[:n]
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Context != "" {
		return e.Context + ": " + e.Message
	}
	return e.Message
}

// Type implements LValue - returns LTUserData.
func (e *Error) Type() LValueType {
	return LTUserData
}

// String implements LValue - returns the message for tostring() and concat.
func (e *Error) String() string {
	return e.Message
}

// Kind returns the error category.
func (e *Error) Kind() Kind {
	if e.kind == "" {
		return Unknown
	}
	return e.kind
}

// Retryable returns whether the operation should be retried.
func (e *Error) Retryable() Ternary {
	if e.retryable == nil {
		return TernaryUnknown
	}
	if *e.retryable {
		return TernaryTrue
	}
	return TernaryFalse
}

// Details returns structured metadata about the error.
func (e *Error) Details() map[string]any {
	return e.details
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *Error) Unwrap() error {
	return e.Err
}

// GetError extracts a *Error from an error chain using errors.As.
// Returns nil if no *Error is found in the chain.
func GetError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// WithKind sets the error category.
func (e *Error) WithKind(k Kind) *Error {
	e.kind = k
	return e
}

// WithRetryable sets the retry status.
func (e *Error) WithRetryable(r bool) *Error {
	e.retryable = &r
	return e
}

// WithDetails sets structured metadata.
func (e *Error) WithDetails(d map[string]any) *Error {
	e.details = d
	return e
}

// WithContext adds context description.
func (e *Error) WithContext(ctx string) *Error {
	e.Context = ctx
	return e
}

// Stack returns a formatted stack trace including both Lua and Go stacks.
func (e *Error) Stack() string {
	var sb strings.Builder

	// Walk error chain
	current := e
	for current != nil {
		if current.Context != "" {
			sb.WriteString(current.Context)
			sb.WriteString("\n")
		}

		// Lua stack
		if current.LuaStack != nil && len(current.LuaStack.Frames) > 0 {
			for _, frame := range current.LuaStack.Frames {
				_, _ = fmt.Fprintf(&sb, "  %s:%d (%s)\n", frame.Source, frame.CurrentLine, frame.Name)
			}
		}

		// Go stack
		if len(current.goStack) > 0 {
			frames := runtime.CallersFrames(current.goStack)
			for {
				frame, more := frames.Next()
				// Skip runtime internals
				if !strings.Contains(frame.File, "runtime/") {
					_, _ = fmt.Fprintf(&sb, "  %s:%d (%s)\n", frame.File, frame.Line, frame.Function)
				}
				if !more {
					break
				}
			}
		}

		// Move to wrapped error
		var we *Error
		if errors.As(current.Err, &we) {
			current = we
		} else {
			break
		}
	}

	return sb.String()
}

// IsErrorKind checks if an error matches a specific kind.
func IsErrorKind(err error, kind Kind) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind() == kind
	}
	return false
}

// GetErrorKind extracts the kind from an error.
func GetErrorKind(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind()
	}
	return Unknown
}

// AsError extracts an Error from an LValue if possible.
func AsError(v LValue) (*Error, bool) {
	if e, ok := v.(*Error); ok {
		return e, true
	}
	return nil, false
}
