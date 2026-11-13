//go:build !windows

package main

import "time"

func newLatencyClock() (latencyClock, error) {
	origin := time.Now()
	return latencyClock{
		name:      "Go monotonic clock",
		frequency: int64(time.Second),
		now:       func() (time.Duration, error) { return time.Since(origin), nil },
	}, nil
}
