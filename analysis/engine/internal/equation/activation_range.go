package equation

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// PairLocator is the closed symbolic locator for one dynamic relation. It is
// deliberately only a seal input: a caller may describe a triple, but only a
// sealed binding can turn an axis-member triple into a Member.
type PairLocator struct {
	Application composition.Key
	Target      composition.Key
	Endpoint    composition.Key
}

func (locator PairLocator) Available() bool {
	return locator.Application.Available() && locator.Target.Available() && locator.Endpoint.Available()
}

// Member is an unforgeable topology-owned solver tuple. Its identity is the
// typed (binding, application, target, endpoint) tuple itself, never a
// portable hash or a candidate ordinal.
type Member struct {
	owner   *Topology
	binding composition.Key
	locator PairLocator
}

type memberTuple struct {
	binding     composition.Key
	application composition.Key
	target      composition.Key
	endpoint    composition.Key
}

func memberToken(member Member) memberTuple {
	if !member.Available() {
		return memberTuple{}
	}
	return memberTuple{binding: member.binding, application: member.locator.Application, target: member.locator.Target, endpoint: member.locator.Endpoint}
}

func writeMemberTuple(writer *canonical.DigestWriter, member Member) bool {
	tuple := memberToken(member)
	return tuple.binding.Available() && writeKey(writer, tuple.binding) && writeKey(writer, tuple.application) && writeKey(writer, tuple.target) && writeKey(writer, tuple.endpoint)
}

func (member Member) Available() bool {
	return member.owner != nil && member.binding.Available() && member.locator.Available()
}

func (member Member) Binding() composition.Key {
	if !member.Available() {
		return composition.Key{}
	}
	return member.binding
}

// Locator returns the exact accepted relation coordinates. It is available
// only to the engine package through this internal boundary; domain code gets
// its opaque projection from a live Access or RuleDerivation instead of an
// equation Member capability.
func (member Member) Locator() (PairLocator, bool) {
	if !member.Available() {
		return PairLocator{}, false
	}
	return member.locator, true
}

func (member Member) ownedBy(topology *Topology) bool {
	return member.Available() && topology != nil && member.owner == topology && topology.ownsMember(member)
}

func compareMember(left, right Member) int {
	if comparison := compareKey(left.binding, right.binding); comparison != 0 {
		return comparison
	}
	if comparison := compareKey(left.locator.Application, right.locator.Application); comparison != 0 {
		return comparison
	}
	if comparison := compareKey(left.locator.Target, right.locator.Target); comparison != 0 {
		return comparison
	}
	return compareKey(left.locator.Endpoint, right.locator.Endpoint)
}

func sameMember(left, right Member) bool {
	return left.Available() && right.Available() && left.owner == right.owner && compareMember(left, right) == 0
}

func (member Member) Compare(other Member) (int, bool) {
	if !member.Available() || !other.Available() || member.owner != other.owner {
		return 0, false
	}
	return compareMember(member, other), true
}

func (member Member) Same(other Member) bool { return sameMember(member, other) }

type AcceptedMember struct {
	member   Member
	premise  Expr
	evidence composition.Key
}

func (accepted AcceptedMember) Available() bool {
	return accepted.member.Available() && accepted.premise.Available() && accepted.evidence.Available()
}

func (accepted AcceptedMember) Member() Member {
	if !accepted.Available() {
		return Member{}
	}
	return accepted.member
}

func (accepted AcceptedMember) Evidence() composition.Key {
	if !accepted.Available() {
		return composition.Key{}
	}
	return accepted.evidence
}

// Premise is the exact accepted selection condition. It is equation data,
// rather than a carrier formula hash, so expansion and runtime lowering have
// one auditable guard authority.
func (accepted AcceptedMember) Premise() Expr {
	if !accepted.Available() {
		return Expr{}
	}
	return accepted.premise
}

func lessAcceptedMember(left, right AcceptedMember) bool {
	return compareMember(left.member, right.member) < 0
}
