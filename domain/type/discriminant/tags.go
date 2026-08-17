package discriminant

import (
	"github.com/wippyai/go-lua/domain/type/literal"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

func (d *Detector) HasRequiredTag(t typ.Type) bool {
	return d.requiredTags(t).len() > 0
}

func (d *Detector) RequiredTags(t typ.Type) map[string]uint64 {
	return d.requiredTags(t).copyMap()
}

// ForEachRequiredTag calls fn for each required literal tag in t without
// exposing detector-owned cache storage. Iteration stops when fn returns false.
func (d *Detector) ForEachRequiredTag(t typ.Type, fn func(path string, hash uint64) bool) {
	if fn == nil {
		return
	}
	d.requiredTags(t).forEach(fn)
}

func (d *Detector) requiredTags(t typ.Type) requiredTagSet {
	t = unwrap.Annotated(t)
	if t == nil {
		return requiredTagSet{}
	}
	if d == nil {
		d = NewDetector()
	}
	if d.tags != nil {
		if cached, ok := d.tags[t]; ok {
			return cached
		}
	}
	needsCycleGuard := typ.ContainsRecursive(t)
	if needsCycleGuard {
		if d.active != nil && d.active[t] {
			return requiredTagSet{}
		}
		if d.active == nil {
			d.active = make(map[typ.Type]bool)
		}
		d.active[t] = true
		defer delete(d.active, t)
	}

	tags := d.collectRequiredTags(t)
	if tags.len() == 0 {
		return tags
	}
	if d.tags == nil {
		d.tags = make(map[typ.Type]requiredTagSet)
	}
	d.tags[t] = tags
	return tags
}

func (d *Detector) collectRequiredTags(t typ.Type) requiredTagSet {
	t = unwrap.Annotated(t)
	switch v := t.(type) {
	case *typ.Alias:
		return d.requiredTags(v.Target)
	case *typ.Recursive:
		return d.requiredTags(v.Body)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return requiredTagSet{}
		}
		return d.requiredTags(expanded)
	case *typ.Record:
		var tags requiredTagSet
		for _, field := range v.Fields {
			if field.Optional {
				continue
			}
			if lit, ok := literal.ExtractAliasOnly(field.Type); ok {
				tags.add(field.Name, typ.EqualityHash(lit))
				continue
			}
			tags.addPrefixed(field.Name, d.requiredTags(field.Type))
		}
		for _, member := range v.StaticMembers {
			if member.Optional {
				continue
			}
			path := staticMemberPath(member)
			if lit, ok := literal.ExtractAliasOnly(member.Type); ok {
				tags.add(path, typ.EqualityHash(lit))
				continue
			}
			tags.addPrefixed(path, d.requiredTags(member.Type))
		}
		return tags
	case *typ.Union:
		return d.commonUnionTags(v)
	}
	return requiredTagSet{}
}

func (d *Detector) commonUnionTags(u *typ.Union) requiredTagSet {
	if u == nil || len(u.Members) == 0 {
		return requiredTagSet{}
	}
	var common requiredTagSet
	for i, member := range u.Members {
		memberTags := d.requiredTags(member)
		if i == 0 {
			common = memberTags.clone()
			continue
		}
		common.keepMatching(memberTags)
	}
	return common
}

type requiredTagSet struct {
	count int
	path  string
	hash  uint64
	many  map[string]uint64
}

func (s requiredTagSet) len() int {
	return s.count
}

func (s requiredTagSet) lookup(path string) (uint64, bool) {
	switch s.count {
	case 0:
		return 0, false
	case 1:
		if s.path == path {
			return s.hash, true
		}
		return 0, false
	default:
		hash, ok := s.many[path]
		return hash, ok
	}
}

func (s requiredTagSet) forEach(fn func(path string, hash uint64) bool) {
	if s.count == 0 {
		return
	}
	if s.count == 1 {
		fn(s.path, s.hash)
		return
	}
	for path, hash := range s.many {
		if !fn(path, hash) {
			return
		}
	}
}

func (s requiredTagSet) copyMap() map[string]uint64 {
	if s.count == 0 {
		return nil
	}
	dst := make(map[string]uint64, s.count)
	s.forEach(func(path string, hash uint64) bool {
		dst[path] = hash
		return true
	})
	return dst
}

func (s requiredTagSet) clone() requiredTagSet {
	if s.count <= 1 {
		return s
	}
	clone := requiredTagSet{count: s.count, many: make(map[string]uint64, s.count)}
	for path, hash := range s.many {
		clone.many[path] = hash
	}
	return clone
}

func (s *requiredTagSet) add(path string, hash uint64) {
	switch s.count {
	case 0:
		s.count = 1
		s.path = path
		s.hash = hash
	case 1:
		if s.path == path {
			s.hash = hash
			return
		}
		s.many = make(map[string]uint64, 2)
		s.many[s.path] = s.hash
		s.many[path] = hash
		s.path = ""
		s.hash = 0
		s.count = 2
	default:
		if _, exists := s.many[path]; !exists {
			s.count++
		}
		s.many[path] = hash
	}
}

func (s *requiredTagSet) addPrefixed(prefix string, src requiredTagSet) {
	src.forEach(func(path string, hash uint64) bool {
		s.add(joinPath(prefix, path), hash)
		return true
	})
}

func (s *requiredTagSet) keepMatching(other requiredTagSet) {
	switch s.count {
	case 0:
		return
	case 1:
		if hash, ok := other.lookup(s.path); !ok || hash != s.hash {
			*s = requiredTagSet{}
		}
	default:
		for path, hash := range s.many {
			if otherHash, ok := other.lookup(path); !ok || otherHash != hash {
				delete(s.many, path)
				s.count--
			}
		}
		s.compact()
	}
}

func (s *requiredTagSet) compact() {
	if s.count > 1 {
		return
	}
	if s.count == 0 {
		*s = requiredTagSet{}
		return
	}
	for path, hash := range s.many {
		*s = requiredTagSet{count: 1, path: path, hash: hash}
		return
	}
	*s = requiredTagSet{}
}
