package main

import (
	"errors"
	"math/bits"
	"time"
)

// latencyClock measures intervals. It is independent of wall-clock timestamps
// and the timers used to schedule requests.
type latencyClock struct {
	name      string
	frequency int64
	now       func() (time.Duration, error)
}

func (c latencyClock) since(start time.Duration) (time.Duration, error) {
	end, err := c.now()
	if err != nil {
		return 0, err
	}
	if end < start {
		return 0, errors.New("measurement clock moved backwards")
	}
	return end - start, nil
}

// counterDuration converts elapsed counter ticks to nanoseconds without
// overflowing ticks*1e9 or rounding a large absolute counter through float64.
func counterDuration(ticks, frequency int64) (time.Duration, error) {
	if ticks < 0 || frequency <= 0 {
		return 0, errors.New("invalid performance counter reading")
	}
	const maxDuration = uint64(1<<63 - 1)
	seconds := uint64(ticks / frequency)
	if seconds > maxDuration/uint64(time.Second) {
		return 0, errors.New("performance counter duration overflow")
	}
	hi, lo := bits.Mul64(uint64(ticks%frequency), uint64(time.Second))
	fraction, _ := bits.Div64(hi, lo, uint64(frequency))
	whole := seconds * uint64(time.Second)
	if fraction > maxDuration-whole {
		return 0, errors.New("performance counter duration overflow")
	}
	return time.Duration(whole + fraction), nil
}
