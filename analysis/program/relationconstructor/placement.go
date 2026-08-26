package relationconstructor

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// DecisionScopes derives the decision scopes one declared rule is placed at.
//
// A scope is composition data rather than rule-declaration data, but which
// scopes a rule stands in is not a free choice: the declaration already
// determines them. A rule decides its candidate rows once, so it has one
// candidate scope, and it observes one scope per declared input port. Naming
// them from the declaration is therefore a reading of structure that already
// exists, not an issuance of new identity.
//
// The derivation is stated here and nowhere else. Two compilations of one rule
// catalog produce identical names by construction, so a mount does not have to
// carry a scope table for its answers to be comparable with another mount of
// the same program.
//
//	candidate  = <rule key>
//	port p     = <rule key>/port/<read key>/<p>, or <rule key>/port/<p> when
//	             no declared read occupies that port
//
// Every name is a structure-surface entry, so a scope never collides with the
// rule surface entry of the rule it belongs to. A rule whose ports are not a
// contiguous run refuses rather than being placed at a scope vector with a
// hole in it.
func DecisionScopes(spec rule.Spec) (relcompile.Placement, bool) {
	if !spec.Key.Available() {
		return relcompile.Placement{}, false
	}
	program := spec.Program
	ports := program.InputCount()
	if ports < 0 {
		return relcompile.Placement{}, false
	}
	placement := relcompile.Placement{Candidate: scopeName(spec.Key)}
	if ports == 0 {
		return placement, true
	}
	reads, ok := portReads(program)
	if !ok {
		return relcompile.Placement{}, false
	}
	placement.Ports = make([]relcompile.Name, 0, ports)
	for port := 0; port < ports; port++ {
		placement.Ports = append(placement.Ports, scopeName(portScopeKey(spec.Key, reads[port], port)))
	}
	return placement, true
}

// scopeName addresses one derived scope on the structure surface, which is the
// surface a decision scope is declared on.
func scopeName(key schema.Key) relcompile.Name {
	return relcompile.EntryName(schema.SurfaceKindStructure, key)
}

// portScopeKey spells one port's scope. The read's own declared key is carried
// so that two ports reading different keys are visibly different scopes rather
// than two ordinals a reader has to resolve to tell apart.
func portScopeKey(ruleKey schema.Key, read schema.Key, port int) schema.Key {
	ordinal := strconv.Itoa(port)
	if !read.Available() {
		return ruleKey + "/port/" + schema.Key(ordinal)
	}
	return ruleKey + "/port/" + read + "/" + schema.Key(ordinal)
}

// portReads answers the declared key each input port is read through.
//
// A port is occupied by the join that declares it, so the answer comes from
// the declaration rather than from join order. Two joins claiming one port is
// a malformed program: the port would carry two reads and no single scope.
func portReads(program ruleprogram.Program) (map[int]schema.Key, bool) {
	reads := make(map[int]schema.Key, program.JoinCount())
	for index := 0; index < program.JoinCount(); index++ {
		join, ok := program.JoinAt(index)
		if !ok {
			return nil, false
		}
		port := int(join.Read.Input.Uint64())
		if port < 0 {
			return nil, false
		}
		if _, occupied := reads[port]; occupied {
			return nil, false
		}
		reads[port] = join.Key.Member
	}
	return reads, true
}
