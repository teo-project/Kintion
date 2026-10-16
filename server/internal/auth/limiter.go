package auth

import (
	"sync"
	"time"
)

// limiter — ограничение частоты неудачных попыток входа: не больше max
// неудач за окно по каждому ключу (адрес клиента, логин). В памяти процесса.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
	now    func() time.Time
}

func newLimiter(maxFails int, window time.Duration) *limiter {
	return &limiter{max: maxFails, window: window, fails: map[string][]time.Time{}, now: time.Now}
}

func (l *limiter) recent(key string) []time.Time {
	cut := l.now().Add(-l.window)
	ts := l.fails[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.fails, key)
		return nil
	}
	l.fails[key] = ts
	return ts
}

func (l *limiter) allow(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		if len(l.recent(k)) >= l.max {
			return false
		}
	}
	return true
}

func (l *limiter) fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.fails) > 10000 { // чистка устаревших ключей
		for k := range l.fails {
			l.recent(k)
		}
	}
	for _, k := range keys {
		l.fails[k] = append(l.recent(k), l.now())
	}
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}
