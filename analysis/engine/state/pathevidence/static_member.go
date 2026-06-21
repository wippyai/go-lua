package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ReadPathStaticMember reads a must static-member fact.
func (l Lane) ReadPathStaticMember(pathKey keyspace.Key) (product.Value, bool) {
	if pathKey.Kind == keyspace.KindInvalid || l.staticMembersBottom {
		return product.Value{}, false
	}
	v, ok := l.staticMembers[pathKey]
	return v, ok
}

// WritePathStaticMember returns a lane with a must static-member fact recorded
// and whether this write made the surrounding state reachable.
func (l Lane) WritePathStaticMember(pathKey keyspace.Key, value product.Value) (Lane, bool) {
	if pathKey.Kind == keyspace.KindInvalid {
		return l, false
	}
	if !l.staticMembersBottom {
		if existing, ok := l.staticMembers[pathKey]; ok && existing == value {
			return l, false
		}
	}
	members := cloneLocalValueMap(l.staticMembers)
	if members == nil {
		members = make(map[keyspace.Key]product.Value, 1)
	}
	members[pathKey] = value
	out := l.Reachable()
	out.staticMembers = members
	return out, true
}
