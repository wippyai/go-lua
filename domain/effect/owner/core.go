package owner

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
