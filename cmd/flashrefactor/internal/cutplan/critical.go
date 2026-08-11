package cutplan

import (
	"fmt"
	"sort"
	"strings"
)

// validateCriticalPairs enforces the operational DPO-style law for this
// deliberately small language. Independent cuts may share reads, but every
// read/write or write/write overlap must be made explicit. Two operations may
// never both write a path: combine them into one authority cut instead.
func validateCriticalPairs(operations []Operation) error {
	reach, err := dependencyReachability(operations)
	if err != nil {
		return err
	}
	for left := 0; left < len(operations); left++ {
		for right := left + 1; right < len(operations); right++ {
			firstIndex, secondIndex := left, right
			if reach[right][left] {
				firstIndex, secondIndex = right, left
			}
			first, second := operations[firstIndex], operations[secondIndex]
			reasons := criticalPairReasons(first, second)
			if len(reasons) == 0 {
				continue
			}
			if !reach[left][right] && !reach[right][left] {
				return fmt.Errorf("unordered operations %s and %s conflict: %s", operations[left].ID, operations[right].ID, strings.Join(reasons, ", "))
			}
			if intersects(first.Footprint.Write, second.Footprint.Write) {
				return fmt.Errorf("ordered operations %s then %s both write one path; merge them into one authority cut", first.ID, second.ID)
			}
			if intersects(second.Footprint.Write, first.Footprint.Read) {
				return fmt.Errorf("ordered operations %s then %s read a later write", first.ID, second.ID)
			}
			if intersects(first.Footprint.Write, second.Footprint.Read) && first.Authority.To != second.Authority.From {
				return fmt.Errorf("ordered operations %s then %s consume a written path without an authority chain", first.ID, second.ID)
			}
		}
	}
	return nil
}

func dependencyReachability(operations []Operation) ([][]bool, error) {
	byID := make(map[string]int, len(operations))
	for index, operation := range operations {
		byID[operation.ID] = index
	}
	reach := make([][]bool, len(operations))
	for index := range reach {
		reach[index] = make([]bool, len(operations))
	}
	for dependent, operation := range operations {
		for _, predecessor := range operation.After {
			index, exists := byID[predecessor]
			if !exists {
				return nil, fmt.Errorf("operation %s depends on unknown operation %s", operation.ID, predecessor)
			}
			reach[index][dependent] = true
		}
	}
	for middle := range operations {
		for from := range operations {
			if !reach[from][middle] {
				continue
			}
			for to := range operations {
				reach[from][to] = reach[from][to] || reach[middle][to]
			}
		}
	}
	return reach, nil
}

func criticalPairReasons(left, right Operation) []string {
	var reasons []string
	if intersects(left.Footprint.Write, right.Footprint.Write) {
		reasons = append(reasons, "write/write footprint")
	}
	if intersects(left.Footprint.Write, right.Footprint.Read) || intersects(right.Footprint.Write, left.Footprint.Read) {
		reasons = append(reasons, "read/write footprint")
	}
	return reasons
}

func intersects(left, right []string) bool {
	if len(left) > len(right) {
		left, right = right, left
	}
	set := make(map[string]bool, len(left))
	for _, value := range left {
		set[value] = true
	}
	for _, value := range right {
		if set[value] {
			return true
		}
	}
	return false
}

// CriticalPairReport is diagnostic only. Validation remains the authority.
func CriticalPairReport(intent Intent) []string {
	var result []string
	for left := 0; left < len(intent.Operations); left++ {
		for right := left + 1; right < len(intent.Operations); right++ {
			reasons := criticalPairReasons(intent.Operations[left], intent.Operations[right])
			if len(reasons) != 0 {
				result = append(result, intent.Operations[left].ID+"/"+intent.Operations[right].ID+": "+strings.Join(reasons, ", "))
			}
		}
	}
	sort.Strings(result)
	return result
}
