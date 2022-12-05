package internal

import (
	"context"
	"math/rand"
	"time"
)

// RandomDuration returns a random duration between 0 and max.
func RandomDuration(max time.Duration) time.Duration {
	if max == 0 {
		return 0
	}

	sleepns := rand.Int63n(max.Nanoseconds())

	return time.Duration(sleepns)
}

// SleepContext sleeps until the context is closed or the duration is reached.
func SleepContext(ctx context.Context, duration time.Duration) error {
	if duration == 0 {
		return nil
	}

	t := time.NewTimer(duration)
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		t.Stop()
		return ctx.Err()
	}
}

// AlignDuration returns the duration until next aligned interval.
// If the current time is aligned a 0 duration is returned.
func AlignDuration(tm time.Time, interval time.Duration) time.Duration {
	return AlignTime(tm, interval).Sub(tm)
}

// AlignTime returns the time of the next aligned interval.
// If the current time is aligned the current time is returned.
func AlignTime(tm time.Time, interval time.Duration) time.Time {
	truncated := tm.Truncate(interval)
	if truncated == tm {
		return tm
	}
	return truncated.Add(interval)
}
