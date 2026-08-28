package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type operationBarrierRegistry struct {
	mu     sync.Mutex
	active map[string]chan struct{}
}

func newOperationBarrierRegistry() *operationBarrierRegistry {
	return &operationBarrierRegistry{active: make(map[string]chan struct{})}
}

func (r *operationBarrierRegistry) begin(name string) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[name]; exists {
		return nil, fmt.Errorf("operation is already active")
	}
	done := make(chan struct{})
	r.active[name] = done
	return func() {
		r.mu.Lock()
		if current, ok := r.active[name]; ok && current == done {
			delete(r.active, name)
			close(done)
		}
		r.mu.Unlock()
	}, nil
}

func (r *operationBarrierRegistry) wait(ctx context.Context, name string) error {
	r.mu.Lock()
	done := r.active[name]
	r.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func detachedCandidateLaunchContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

var coordinatedCandidateLaunches = newOperationBarrierRegistry()
var containerStops = newOperationBarrierRegistry()
