package main

import (
	"errors"
	"syscall"
	"time"
	"unsafe"
)

// kernel32.dll is a known Windows system DLL. Go loads it from the system
// directory. No cgo or additional module is required for these Windows APIs.
var (
	clockKernel32  = syscall.NewLazyDLL("kernel32.dll")
	queryCounter   = clockKernel32.NewProc("QueryPerformanceCounter")
	queryFrequency = clockKernel32.NewProc("QueryPerformanceFrequency")
)

func readPerformanceCounter() (int64, error) {
	var ticks int64
	ok, _, _ := queryCounter.Call(uintptr(unsafe.Pointer(&ticks)))
	if ok == 0 {
		return 0, errors.New("QueryPerformanceCounter failed")
	}
	return ticks, nil
}

func newLatencyClock() (latencyClock, error) {
	if err := queryCounter.Find(); err != nil {
		return latencyClock{}, errors.New("QueryPerformanceCounter unavailable")
	}
	if err := queryFrequency.Find(); err != nil {
		return latencyClock{}, errors.New("QueryPerformanceFrequency unavailable")
	}
	var frequency int64
	ok, _, _ := queryFrequency.Call(uintptr(unsafe.Pointer(&frequency)))
	if ok == 0 || frequency <= 0 {
		return latencyClock{}, errors.New("invalid QueryPerformanceFrequency result")
	}
	origin, err := readPerformanceCounter()
	if err != nil {
		return latencyClock{}, err
	}
	return latencyClock{
		name:      "Windows QueryPerformanceCounter",
		frequency: frequency,
		now: func() (time.Duration, error) {
			ticks, err := readPerformanceCounter()
			if err != nil {
				return 0, err
			}
			return counterDuration(ticks-origin, frequency)
		},
	}, nil
}
