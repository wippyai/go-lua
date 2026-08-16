package owner

// coordinate is intentionally package-local. Its nominal identity is part of
// Value's Factor algebra authority and must not be erased to uint32 merely to
// cross an owner boundary.
type coordinate uint32
