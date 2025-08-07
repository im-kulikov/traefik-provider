package traefik_provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRoutine(t *testing.T) {
	t.Run("one errored", func(t *testing.T) {
		run := newRunner(t.Context())
		for i := range 10 {
			run.Go(func(ctx context.Context) error {
				if i == 0 {
					return fmt.Errorf("error from %d", i)
				}

				<-ctx.Done()

				return fmt.Errorf("from %d: %w", i, context.Cause(ctx))
			})
		}

		require.EqualError(t, run.Wait(), "error from 0")
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
		defer cancel()

		run := newRunner(ctx)
		for i := range 10 {
			run.Go(func(ctx context.Context) error {
				<-ctx.Done()

				return fmt.Errorf("from %d: %w", i, context.Cause(ctx))
			})
		}

		require.ErrorIs(t, run.Wait(), context.DeadlineExceeded)
	})
}
