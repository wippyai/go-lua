package equation

import "github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"

// NewBranchGuard is the sole bridge from a declared branch edge into equation
// guard syntax. A guard is usable only when its body is valid and factkey can
// parse the exact encoding back to the same decision; invalid inputs therefore
// produce the zero guard and fail closed at equation validation.
func NewBranchGuard(body BodyID, branch factkey.BranchGuard) Guard {
	parsed, ok := factkey.ParseBranchGuard(branch.Encoding())
	if !body.Valid() || !ok || parsed != branch {
		return Guard{}
	}
	size := len(factkey.BranchGuardPrefix) + len(branch.Name) + 1 + len(branch.Edge)
	encoding := branch.AppendEncoding(make([]byte, 0, size))
	return Guard{Body: body, Encoding: encoding}
}

// GuardSet preserves the declared guard vocabulary at call sites without
// reopening Guard's representation merely to construct a slice.
func GuardSet(guards ...Guard) []Guard { return guards }
