package util

import (
	"golang.org/x/sync/semaphore"
)

type CallBack func(interface{})

// SafeReleaseSemaphore releases semaphore safely without panic
func SafeReleaseSemaphore(sem *semaphore.Weighted, n int64, onError CallBack) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil && onError != nil {
				onError(r)
			}
		}()
		sem.Release(n)
	}
}
