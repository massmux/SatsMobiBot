package runtime

import (
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

var functionMap cmap.ConcurrentMap

func init() {
	functionMap = cmap.New()
}

var DefaultTickerDuration = time.Second * 10

// ResettableFunction will reset the user state as soon as tick is delivered.
type ResettableFunction struct {
	Ticker    *time.Ticker
	Timer     *time.Timer
	ResetChan chan struct{} // channel used to reset the ticker
	StopChan  chan struct{} // channel used to reset the ticker
	duration  time.Duration
	Started   bool
	name      string
}

type ResettableFunctionTickerOption func(*ResettableFunction)

func WithTicker(t *time.Ticker) ResettableFunctionTickerOption {
	return func(a *ResettableFunction) {
		a.Ticker = t
	}
}
func WithDuration(d time.Duration) ResettableFunctionTickerOption {
	return func(a *ResettableFunction) {
		a.duration = d
	}
}
func WithTimer(t *time.Timer) ResettableFunctionTickerOption {
	return func(a *ResettableFunction) {
		a.Timer = t
	}
}
func RemoveTicker(name string) {
	functionMap.Remove(name)
}

func Get(name string) (*ResettableFunction, bool) {
	if t, ok := functionMap.Get(name); ok {
		return t.(*ResettableFunction), ok
	}
	return nil, false
}
func GetFunction(name string, option ...ResettableFunctionTickerOption) *ResettableFunction {
	if t, ok := functionMap.Get(name); ok {
		return t.(*ResettableFunction)
	} else {
		t := NewResettableFunction(name, option...)
		functionMap.Set(name, t)
		return t
	}
}

func NewResettableFunction(name string, option ...ResettableFunctionTickerOption) *ResettableFunction {
	t := &ResettableFunction{
		ResetChan: make(chan struct{}, 1),
		StopChan:  make(chan struct{}, 1),
		name:      name,
	}
	if t.duration == 0 {
		t.duration = DefaultTickerDuration
	}

	for _, opt := range option {
		opt(t)
	}

	return t
}

// Do will listen for timers and invoke functionCallback on tick.
func (t *ResettableFunction) Do(functionCallback func()) {
	t.Started = true
	// persist function
	functionMap.Set(t.name, t)
	go func() {
		for {
			if t.Timer != nil {
				// timer is set. checking event
				select {
				case <-t.Timer.C:
					functionCallback()
					return
				default:
				}
			}
			if t.Ticker != nil {
				// ticker is set. checking event
				select {
				case <-t.Ticker.C:
					// ticker delivered signal. do function functionCallback
					functionCallback()
					return
				default:
				}
			}
			// check stop and reset channels
			select {
			case <-t.StopChan:
				if t.Timer != nil {
					t.Timer.Stop()
				}
				if t.Ticker != nil {
					t.Ticker.Stop()
				}
				return
			case <-t.ResetChan:
				// reset signal received. creating new ticker.
				if t.Ticker != nil {
					t.Ticker.Reset(t.duration)
				}
				if t.Timer != nil {
					t.Timer.Reset(t.duration)
				}
			default:
				break
			}
			time.Sleep(time.Millisecond * 500)
		}
	}()
}
