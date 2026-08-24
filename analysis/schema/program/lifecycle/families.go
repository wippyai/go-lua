package lifecycle

import (
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

// The lifecycle plane owns its canonical manifest definitions after the
// static-node plane. Each accessor binds directly to the child catalog; no
// root package declaration can create a second slot/name authority.
func StorageCellLifetimeFamily() programfamily.Family[StorageCellLifetime] {
	return programfamily.New[StorageCellLifetime](programcatalog.StorageCellLifetime())
}

func SubjectYieldBoundaryFamily() programfamily.Family[SubjectYieldBoundary] {
	return programfamily.New[SubjectYieldBoundary](programcatalog.SubjectYieldBoundary())
}

func SubjectLivenessSpanFamily() programfamily.Family[SubjectLivenessSpan] {
	return programfamily.New[SubjectLivenessSpan](programcatalog.SubjectLivenessSpan())
}

func SubjectEventFamily() programfamily.Family[SubjectEvent] {
	return programfamily.New[SubjectEvent](programcatalog.SubjectEvent())
}

func SubjectAliasRouteScopeFamily() programfamily.Family[SubjectAliasRouteScope] {
	return programfamily.New[SubjectAliasRouteScope](programcatalog.SubjectAliasRouteScope())
}

func SubjectAliasRouteScopeMemberFamily() programfamily.Family[SubjectAliasRouteScopeMember] {
	return programfamily.New[SubjectAliasRouteScopeMember](programcatalog.SubjectAliasRouteScopeMember())
}

func SubjectAliasCandidateFamily() programfamily.Family[SubjectAliasCandidate] {
	return programfamily.New[SubjectAliasCandidate](programcatalog.SubjectAliasCandidate())
}
