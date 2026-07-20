//go:build linux

package rsswatch

import (
	"fmt"
	"os"
)

func rssSupported() bool { return true }

func currentRSS() (uint64, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	residentPages, ok := secondUint(data)
	if !ok {
		return 0, fmt.Errorf("rsswatch: malformed /proc/self/statm")
	}
	return residentPages * uint64(os.Getpagesize()), nil
}

func secondUint(data []byte) (uint64, bool) {
	field := 0
	var value uint64
	digits := false
	for _, char := range data {
		if char >= '0' && char <= '9' {
			if field == 1 {
				value = value*10 + uint64(char-'0')
				digits = true
			}
			continue
		}
		if char != ' ' && char != '\t' && char != '\n' && char != '\r' {
			return 0, false
		}
		if field == 1 && digits {
			return value, true
		}
		if field == 0 {
			field = 1
		}
	}
	return value, field == 1 && digits
}
