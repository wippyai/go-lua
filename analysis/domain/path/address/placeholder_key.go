package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// PlaceholderKey is the opaque carrier for a parameter placeholder path,
// such as $0 or $0.member. It is distinct from RootPlaceholderKey because some
// summary facts need to rehydrate a member below an argument, while others apply
// only to the whole parameter object.
type PlaceholderKey struct {
	key pathdom.PathKey
}

// PathKey returns the shared canonical carrier.
func (k PlaceholderKey) PathKey() pathdom.PathKey { return k.key }

func (k PlaceholderKey) String() string { return string(k.key) }

// Valid reports whether k is a non-empty placeholder path key.
func (k PlaceholderKey) Valid() bool {
	_, ok := PlaceholderPathFromKey(k.PathKey())
	return ok
}

// Path parses k into its structured placeholder path.
func (k PlaceholderKey) Path() (pathdom.Path, bool) {
	return PlaceholderPathFromKey(k.PathKey())
}

// RootPlaceholderIndex returns the parameter index when k has no member suffix.
func (k PlaceholderKey) RootPlaceholderIndex() (int, bool) {
	path, ok := k.Path()
	if !ok || len(path.Segments) != 0 {
		return 0, false
	}
	index := path.PlaceholderIndex()
	if index < 0 {
		return 0, false
	}
	return index, true
}

// PlaceholderKeyFromPath validates and narrows path to a placeholder key.
func PlaceholderKeyFromPath(path pathdom.Path) (PlaceholderKey, bool) {
	key := path.Key()
	if !path.IsPlaceholder() || key == "" {
		return PlaceholderKey{}, false
	}
	return PlaceholderKey{key: key}, true
}

// RootPlaceholderKey is the opaque carrier for a whole parameter
// placeholder such as $0. It deliberately rejects member suffixes: facts that
// carry this key apply to the parameter object itself, not one of its fields.
type RootPlaceholderKey struct {
	key pathdom.PathKey
}

// PathKey returns the shared canonical carrier.
func (k RootPlaceholderKey) PathKey() pathdom.PathKey { return k.key }

func (k RootPlaceholderKey) String() string { return string(k.key) }

// Valid reports whether k is a non-empty root placeholder key with no suffix.
func (k RootPlaceholderKey) Valid() bool {
	path, ok := PlaceholderPathFromKey(k.PathKey())
	return ok && len(path.Segments) == 0
}

// PlaceholderIndex returns the parameter index encoded by k.
func (k RootPlaceholderKey) PlaceholderIndex() (int, bool) {
	return PlaceholderKey{key: k.key}.RootPlaceholderIndex()
}

// RootPlaceholderKeyFromPath validates and narrows path to a root placeholder
// key.
func RootPlaceholderKeyFromPath(path pathdom.Path) (RootPlaceholderKey, bool) {
	key := path.Key()
	if !path.IsPlaceholder() || len(path.Segments) != 0 || key == "" {
		return RootPlaceholderKey{}, false
	}
	return RootPlaceholderKey{key: key}, true
}
