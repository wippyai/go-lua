package front

import (
	"strconv"
	"strings"
)

// BoundCallIdentity is the opaque authored path whose root the binder resolved
// to a global declaration. Lowering mints it; artifact consumers can only
// decode the value carried by the dedicated operand role.
type BoundCallIdentity struct{ spelling string }

func boundCallIdentity(spelling string) (BoundCallIdentity, bool) {
	if spelling == "" || strings.ContainsAny(spelling, "\x00\r\n") {
		return BoundCallIdentity{}, false
	}
	return BoundCallIdentity{spelling: spelling}, true
}

// DecodeBoundCallIdentity admits one lowering-owned global call operand.
func DecodeBoundCallIdentity(encoded []byte) (BoundCallIdentity, bool) {
	return boundCallIdentity(string(encoded))
}

func (identity BoundCallIdentity) Valid() bool  { return identity.spelling != "" }
func (identity BoundCallIdentity) Name() string { return identity.spelling }
func (identity BoundCallIdentity) Matches(name string) bool {
	return identity.spelling != "" && identity.spelling == name
}
func (identity BoundCallIdentity) wire() string { return identity.spelling }

// CallMethodIdentity is the opaque method selector decoded from the
// lowering-owned method operand.
type CallMethodIdentity struct{ name string }

// DecodeCallMethodIdentity is the sole method-wire parser.
func DecodeCallMethodIdentity(encoded []byte) (CallMethodIdentity, bool) {
	text, found := strings.CutPrefix(string(encoded), "method/")
	if !found || text == "" {
		return CallMethodIdentity{}, false
	}
	name, err := strconv.Unquote(text)
	if err != nil || name == "" {
		return CallMethodIdentity{}, false
	}
	return CallMethodIdentity{name: name}, true
}

func (identity CallMethodIdentity) Valid() bool  { return identity.name != "" }
func (identity CallMethodIdentity) Name() string { return identity.name }
func (identity CallMethodIdentity) Matches(name string) bool {
	return identity.name != "" && identity.name == name
}
