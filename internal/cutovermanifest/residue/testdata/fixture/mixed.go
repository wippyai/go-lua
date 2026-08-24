package fixture

// legacyHelper is unexported plumbing a legacy caller used; it is not
// itself residue, but it is not exported either.
func legacyHelper() {}

// Current is genuine current-generation exported surface: this file must
// not be classified as pure residue even though it also mentions
// RegisterRule in a comment below for the census to find.
//
// RegisterRule is mentioned here only to exercise the census; Current is
// real surface unrelated to it.
type Current struct{}
