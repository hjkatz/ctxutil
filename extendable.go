package ctxutil

import (
	"context"
	"sync"
	"time"
)

// extendableContext implements the context interface.
// This context can be extended by calling the returned
// ExtendFunc with an extended deadline.
//
// Note: When the ExtendFunc is called the underlying wrapped context is replaced
//       with a new context.WithDeadline(). The cancel
//       that should expire will be ignored and only the
//
// Note: Each time the ExtendFunc is called will incur an additional
//       goroutine and a performance penalty. Use sparingly.
type extendableContext struct {
	// currCtx is the current active context
	// (if an extension has occurred, or the original context if no extension has occurred)
	currCtx      context.Context
	currCancelFn context.CancelFunc

	// origCtx is the original context that is wrapped
	origCtx context.Context

	done   chan struct{}
	err    error
	extend chan time.Time

	mux sync.Mutex
}

type ExtendFunc func(time.Time)

func WithExtension(parent context.Context, cancelFn context.CancelFunc) (context.Context, context.CancelFunc, ExtendFunc) {
	ctx := &extendableContext{
		origCtx:      parent,
		currCtx:      parent,
		currCancelFn: cancelFn,
		done:         make(chan struct{}),
	}

	extendFn := func(extension time.Time) {
		ctx.extend <- extension
	}

	go ctx.watchContext(parent)

	return ctx, ctx.cancel, extendFn
}

// Value propogates the Value call to the original context
func (ctx *extendableContext) Value(key interface{}) interface{} {
	return ctx.origCtx.Value(key)
}

func (ctx *extendableContext) Err() error {
	return ctx.currCtx.Err()
}

func (ctx *extendableContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *extendableContext) Deadline() (time.Time, bool) {
	ctx.mux.Lock()
	defer ctx.mux.Unlock()

	return ctx.currCtx.Deadline()
}

func (ctx *extendableContext) watchContext(watchedCtx context.Context) {
	select {
	case <-watchedCtx.Done():
		switch watchedCtx.Err() {
		case context.Canceled:
			// one of the watched sub-contexts has been canceled
			// the overall error (cancellation) should be from
			// this internal
			ctx.mux.Lock()
			ctx.mux.Unlock()

			// currentCtx has been canceled
			ctx.cancel()
		case context.DeadlineExceeded:
			// TODO ignore this deadline exceeded
		}
	case extension := <-ctx.extend:
		// extension requested before watched context has finished
		ctx.extendDeadline(extension)
	}
}

// extendDeadline extends the deadline by replacing the current context and current cancellation function
func (ctx *extendableContext) deadlineExpired(expiredContext context.Context) {
	ctx.mux.Lock()
	defer ctx.mux.Unlock()

	if ctx.err != nil {
		// context already done
		return
	}

	if ctx.currCtx != nil && expiredContext != nil && ctx.currCtx == expiredContext {
		// the current context expired its deadline without an extension
		close(ctx.done)
	}

	// expired context is an old context that has been extended (thus should be ignored)
	// do nothing
	return
}

// extendDeadline extends the deadline by replacing the current context and current cancellation function
func (ctx *extendableContext) extendDeadline(deadline time.Time) {
	ctx.mux.Lock()
	ctx.currCtx, ctx.currCancelFn = context.WithDeadline(context.Background(), deadline)
	ctx.mux.Unlock()

	// spawn a new watcher
	go ctx.watchContext(ctx.currCtx)
}

// cancel cancels the extendable context and the current context that it's watching,
// then closes the done channel to report that the context has finished
func (ctx *extendableContext) cancel() {
	ctx.mux.Lock()
	defer ctx.mux.Unlock()

	if ctx.err != nil {
		// already canceled, do nothing
		return
	}

	// cancel the current context too
	ctx.currCancelFn() // sets the cancellation error (or whichever) on the current context

	// now that Err() will exist, so propogate the error up to the extendable context
	ctx.err = ctx.currCtx.Err()

	// the extendable context has finished
	close(ctx.done)
}
