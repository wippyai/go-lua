package lua

// Type implements LValue - returns LTUserData.
func (e *Error) Type() LValueType {
	return LTUserData
}

// String implements LValue - returns the message for tostring() and concat.
func (e *Error) String() string {
	return e.Message
}

// AsError extracts an Error from an LValue if possible.
func AsError(v LValue) (*Error, bool) {
	if e, ok := v.(*Error); ok {
		return e, true
	}
	return nil, false
}
