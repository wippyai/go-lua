package program

import "github.com/wippyai/go-lua/analysis/program/keyspace"

func programSemanticSourceCountFits(value int) bool {
	return value >= 0 && uint64(value) <= uint64(keyspace.MaxTermOrdinal)
}

func programSemanticSourceCountsFit(values ...int) bool {
	for _, value := range values {
		if !programSemanticSourceCountFits(value) {
			return false
		}
	}
	return true
}

func addProgramSemanticSourceMeasure(total *int, value int) bool {
	if total == nil || value < 0 || *total < 0 || value > int(^uint(0)>>1)-*total {
		return false
	}
	*total += value
	return true
}

func sumProgramSemanticSourceMeasures(values ...int) (int, bool) {
	total := 0
	for _, value := range values {
		if !addProgramSemanticSourceMeasure(&total, value) {
			return 0, false
		}
	}
	return total, true
}
