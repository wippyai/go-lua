package pathevidence

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ReadPathStaticMember reads a must static-member fact.
func (l Lane) ReadPathStaticMember(pathKey pathaddr.LocalKey) (product.Value, bool) {
	if pathKey == "" || l.staticMembersBottom {
		return product.Value{}, false
	}
	v, ok := l.staticMembers[pathKey]
	return v, ok
}

// WritePathStaticMember returns a lane with a must static-member fact recorded
// and whether this write made the surrounding state reachable.
func (l Lane) WritePathStaticMember(pathKey pathaddr.LocalKey, value product.Value) (Lane, bool) {
	if pathKey == "" {
		return l, false
	}
	if !l.staticMembersBottom {
		if existing, ok := l.staticMembers[pathKey]; ok && existing == value {
			return l, false
		}
	}
	members := cloneLocalValueMap(l.staticMembers)
	if members == nil {
		members = make(map[pathaddr.LocalKey]product.Value, 1)
	}
	members[pathKey] = value
	out := l.Reachable()
	out.staticMembers = members
	return out, true
}
