//go:build !linux

package rsswatch

func rssSupported() bool { return false }

func currentRSS() (uint64, error) { return 0, nil }
