package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/traefik/genconf/dynamic"
)

// TestClient_emit covers the delivery guarantees FetchRaw depends on: results
// must still reach the aggregator on the ordinary path, but a send must never
// block forever once the poll has been abandoned.
func TestClient_emit(t *testing.T) {
	cli := new(Client)

	t.Run("delivers while the buffer has room", func(t *testing.T) {
		out := make(chan *dynamic.Configuration, 1)

		cli.emit(t.Context(), out, nil)
		require.Len(t, out, 1)
	})

	t.Run("delivers with a nil context", func(t *testing.T) {
		out := make(chan *dynamic.Configuration, 1)

		cli.emit(nil, out, nil) // nolint:staticcheck
		require.Len(t, out, 1)
	})

	t.Run("delivers with an already-cancelled context when there is room", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		out := make(chan *dynamic.Configuration, 1)

		cli.emit(ctx, out, nil)
		require.Len(
			t,
			out,
			1,
			"a cancelled context must not drop a result the aggregator can still accept",
		)
	})

	t.Run("waits for room when the context is nil", func(t *testing.T) {
		// Pre-fill the buffer so the fast path is forced to fall through
		// deterministically, rather than racing a waiting receiver.
		out := make(chan *dynamic.Configuration, 1)
		out <- nil

		done := make(chan struct{})
		go func() {
			defer close(done)

			cli.emit(nil, out, nil) // nolint:staticcheck
		}()

		// Give emit time to fall through the fast path and park on the send;
		// draining immediately would let it take the fast path instead.
		time.Sleep(100 * time.Millisecond)
		require.Empty(t, done, "emit should still be blocked on the full buffer")

		<-out // free a slot, unblocking the send

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("emit did not complete once the buffer drained with a nil context")
		}

		require.Len(t, out, 1)
	})

	t.Run("abandons the send when nobody is collecting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		out := make(chan *dynamic.Configuration) // unbuffered, no receiver

		done := make(chan struct{})
		go func() {
			defer close(done)

			cli.emit(ctx, out, nil)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("emit blocked forever with no receiver and a cancelled context")
		}

		require.Empty(t, out)
	})

	t.Run("waits for room that frees up before the context ends", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		// Pre-filled so the fast path falls through to the blocking select.
		out := make(chan *dynamic.Configuration, 1)
		out <- nil

		done := make(chan struct{})
		go func() {
			defer close(done)

			cli.emit(ctx, out, nil)
		}()

		// Let emit park on the blocking select rather than taking the fast path.
		time.Sleep(100 * time.Millisecond)

		<-out // free a slot before the context expires

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("emit did not deliver once the buffer drained")
		}

		require.Len(t, out, 1)
	})
}
