package owner

// validCoordinateCount bounds a census to the dense coordinate space the axis
// publishes: one row per coordinate, and no more rows than that space holds.
func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
