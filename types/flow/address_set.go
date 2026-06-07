package flow

import "github.com/wippyai/go-lua/types/constraint"

// stableAddressSet is the canonical membership container for flow addresses.
// Callers should not choose between Equal, Key, or raw maps locally.
type stableAddressSet struct {
	seen map[constraint.PathKey]struct{}
}

func (s *stableAddressSet) Add(addr StableAddress) bool {
	key := addr.Key()
	if key == "" {
		return false
	}
	if s.seen == nil {
		s.seen = make(map[constraint.PathKey]struct{})
	}
	if _, ok := s.seen[key]; ok {
		return false
	}
	s.seen[key] = struct{}{}
	return true
}

func (s *stableAddressSet) Contains(addr StableAddress) bool {
	key := addr.Key()
	if key == "" || s.seen == nil {
		return false
	}
	_, ok := s.seen[key]
	return ok
}

// stableAddressList preserves first-seen order while de-duplicating by
// StableAddress canonical identity.
type stableAddressList struct {
	set    stableAddressSet
	values []StableAddress
}

func (l *stableAddressList) Add(addr StableAddress) bool {
	if !l.set.Add(addr) {
		return false
	}
	l.values = append(l.values, addr)
	return true
}

func (l *stableAddressList) Contains(addr StableAddress) bool {
	return l.set.Contains(addr)
}

func (l *stableAddressList) Values() []StableAddress {
	if len(l.values) == 0 {
		return nil
	}
	return l.values
}

// pathKeySet is for storage-domain keys that are already canonical PathKeys,
// not for source-level path interpretation.
type pathKeySet struct {
	seen map[constraint.PathKey]struct{}
}

func (s *pathKeySet) Add(key constraint.PathKey) bool {
	if key == "" {
		return false
	}
	if s.seen == nil {
		s.seen = make(map[constraint.PathKey]struct{})
	}
	if _, ok := s.seen[key]; ok {
		return false
	}
	s.seen[key] = struct{}{}
	return true
}

func (s *pathKeySet) Contains(key constraint.PathKey) bool {
	if key == "" || s.seen == nil {
		return false
	}
	_, ok := s.seen[key]
	return ok
}

type pathKeyList struct {
	set    pathKeySet
	values []constraint.PathKey
}

func (l *pathKeyList) Add(key constraint.PathKey) bool {
	if !l.set.Add(key) {
		return false
	}
	l.values = append(l.values, key)
	return true
}

func (l *pathKeyList) AddList(keys []constraint.PathKey) {
	for _, key := range keys {
		l.Add(key)
	}
}

func (l *pathKeyList) Values() []constraint.PathKey {
	if len(l.values) == 0 {
		return nil
	}
	return l.values
}

func (l *pathKeyList) SortedValues() []constraint.PathKey {
	if l.set.seen == nil {
		return nil
	}
	return constraint.SortedPathKeys(l.set.seen)
}
