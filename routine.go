package traefik_provider

import (
	"context"
	"sync"
)

type routine struct {
	*sync.Once
	*sync.WaitGroup
	context.Context

	cancel context.CancelCauseFunc
}

type Routine interface {
	Go(func(context.Context) error)
	Wait() error
	Cancel(error)
}

func newRunner(top context.Context) Routine {
	ctx, cancel := context.WithCancelCause(top)

	return &routine{
		Once:      new(sync.Once),
		WaitGroup: new(sync.WaitGroup),
		Context:   ctx,
		cancel:    cancel,
	}
}

func (r *routine) Go(handle func(context.Context) error) {
	if r == nil {
		panic("empty routine")
	}

	if r.WaitGroup == nil {
		panic("empty wait group")
	}

	if r.Once == nil {
		panic("empty sync.Once")
	}

	if r.Context == nil {
		panic("empty context")
	}

	r.WaitGroup.Add(1)
	go func() {
		if err := handle(r.Context); err != nil {
			r.Once.Do(func() { r.cancel(err) })
		}

		r.WaitGroup.Done()
	}()
}

func (r *routine) Cancel(err error) { r.Do(func() { r.cancel(err) }) }

func (r *routine) Wait() error {
	r.WaitGroup.Wait()
	r.Cancel(context.Canceled)

	return context.Cause(r.Context)
}
