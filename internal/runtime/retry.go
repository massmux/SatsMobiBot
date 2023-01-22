package runtime

import (
	"context"
	"time"
)

// todo -- use function.go instead!
// var retryMap cmap.ConcurrentMap

// func init() {
// 	retryMap = cmap.New()
// }

// ResettableFunction will reset the user state as soon as tick is delivered.
type FunctionRetry struct {
	Ticker   *time.Ticker
	duration time.Duration
	ctx      context.Context
	name     string
}

type FunctionRetryOption func(*FunctionRetry)

func WithRetryDuration(d time.Duration) FunctionRetryOption {
	return func(a *FunctionRetry) {
		a.duration = d
	}
}
func NewRetryTicker(ctx context.Context, name string, option ...FunctionRetryOption) *FunctionRetry {
	t := &FunctionRetry{
		name: name,
		ctx:  ctx,
	}
	for _, opt := range option {
		opt(t)
	}
	if t.duration == 0 {
		t.duration = DefaultTickerDuration
	}
	t.Ticker = time.NewTicker(t.duration)
	return t
}

func (t *FunctionRetry) Do(f func(), cancel_f func(), deadline_f func()) {
	functionMap.Set(t.name, t)
	go func() {
		for {
			select {
			case <-t.Ticker.C:
				// ticker delivered signal. do function f
				f()
			case <-t.ctx.Done():
				if t.ctx.Err() == context.DeadlineExceeded {
					deadline_f()
				}
				if t.ctx.Err() == context.Canceled {
					cancel_f()
				}
				return
			}
		}
	}()
}
