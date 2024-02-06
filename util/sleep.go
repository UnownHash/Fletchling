package util

import (
	"context"
	"time"
)

func SleepContext(ctx context.Context, dur time.Duration) (err error) {
	timer := time.NewTimer(dur)
	defer func() {
		if err != nil && !timer.Stop() {
			<-timer.C
		}
	}()
	select {
	case <-ctx.Done():
		err = context.Cause(ctx)
		return
	case <-timer.C:
		return
	}
}
