package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// PlaceholderKey is the typed string carrier for a parameter placeholder path,
// such as $0 or $0.member. It is distinct from RootPlaceholderKey because some
// summary facts need to rehydrate a member below an argument, while others apply
// only to the whole parameter object.
type PlaceholderKey pathdom.PathKey

// PathKey returns the legacy string carrier for compatibility boundaries.
func (k PlaceholderKey) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

func (k PlaceholderKey) String() string { return string(k) }

// Valid reports whether k is a non-empty placeholder path key.
func (k PlaceholderKey) Valid() bool {
	_, ok := PlaceholderKeyFromPathKey(k.PathKey())
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
	if !path.IsPlaceholder() {
		return "", false
	}
	return PlaceholderKey(path.Key()), true
}

// PlaceholderKeyFromPathKey validates and narrows key to the placeholder path
// grammar.
func PlaceholderKeyFromPathKey(key pathdom.PathKey) (PlaceholderKey, bool) {
	if key == "" {
		return "", false
	}
	if _, ok := PlaceholderPathFromKey(key); !ok {
		return "", false
	}
	return PlaceholderKey(key), true
}

// RootPlaceholderKey is the typed string carrier for a whole parameter
// placeholder such as $0. It deliberately rejects member suffixes: facts that
// carry this key apply to the parameter object itself, not one of its fields.
type RootPlaceholderKey pathdom.PathKey

// PathKey returns the legacy string carrier for compatibility boundaries.
func (k RootPlaceholderKey) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

func (k RootPlaceholderKey) String() string { return string(k) }

// Valid reports whether k is a non-empty root placeholder key with no suffix.
func (k RootPlaceholderKey) Valid() bool {
	_, ok := RootPlaceholderKeyFromPathKey(k.PathKey())
	return ok
}

// PlaceholderIndex returns the parameter index encoded by k.
func (k RootPlaceholderKey) PlaceholderIndex() (int, bool) {
	return PlaceholderKey(k).RootPlaceholderIndex()
}

// RootPlaceholderKeyFromPath validates and narrows path to a root placeholder
// key.
func RootPlaceholderKeyFromPath(path pathdom.Path) (RootPlaceholderKey, bool) {
	if !path.IsPlaceholder() || len(path.Segments) != 0 {
		return "", false
	}
	return RootPlaceholderKey(path.Key()), true
}

// RootPlaceholderKeyForIndex formats a root placeholder key for index.
func RootPlaceholderKeyForIndex(index int) (RootPlaceholderKey, bool) {
	return RootPlaceholderKeyFromPath(pathdom.NewPlaceholder(index))
}

// RootPlaceholderKeyFromPathKey validates and narrows key to the root
// placeholder grammar.
func RootPlaceholderKeyFromPathKey(key pathdom.PathKey) (RootPlaceholderKey, bool) {
	placeholder, ok := PlaceholderKeyFromPathKey(key)
	if !ok {
		return "", false
	}
	if _, ok := placeholder.RootPlaceholderIndex(); !ok {
		return "", false
	}
	return RootPlaceholderKey(key), true
}
