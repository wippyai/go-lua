// Package grammar owns the compiler-only grammar capability. Its internal
// import boundary prevents analysis siblings and the future engine lowerer
// from obtaining or constructing compiler authority.
package grammar

import "github.com/wippyai/go-lua/analysis/identity"

const ABIVersion = uint64(5)

type issuer struct{}

var soleIssuer = &issuer{}

// Capability is an opaque exact grammar compiler proof. Only packages below
// analysis/program/artifact can import its issuer package.
type Capability struct {
	schema identity.ContentID
	abi    uint64
	issuer *issuer
}

// Issue is visible only inside the programartifact package tree by Go's
// internal import rule. The schema adapter calls it only after validating an
// exact grammar CompilationReceipt.
func Issue(schema identity.ContentID, abi uint64) (Capability, bool) {
	if !schema.Available() || abi != ABIVersion {
		return Capability{}, false
	}
	capability := Capability{schema: schema, abi: abi, issuer: soleIssuer}
	return capability, capability.Available()
}

func (capability Capability) Available() bool {
	return capability.schema.Available() && capability.abi == ABIVersion && capability.issuer == soleIssuer
}

func (capability Capability) SchemaDigest() identity.ContentID {
	if !capability.Available() {
		return identity.ContentID{}
	}
	return capability.schema
}

func (capability Capability) ABIVersion() uint64 {
	if !capability.Available() {
		return 0
	}
	return capability.abi
}
