/*
package psync offers some sync helper types and functions.
*/
package pksync

import (
	"context"
	"errors"
	"fmt"
)

// ErrWaitGroupClosed is returned when Delta() or Done() are called on a closed
// CtxWaitGroup.
var ErrWaitGroupClosed = errors.New("CtxWaitGroup is closed")

// waitEvent is used internally to send / receive responses from the routine handling
// the internal counter.
type waitEvent struct {
	// Delta is the delta to apply to the internal counter of the WaitGroup.
	Delta int
	// Success chan will close if the operation resolved successfully.
	Success chan struct{}
}

// CtxWaitGroup is a WaitGroup where Wait will exit with an error if a context
// cancels. TimeoutWaitGroup cannot be re-used once it closes.
type CtxWaitGroup struct {
	// ctx is derived from the context passed in from the caller, and will cause Wait
	// to return with ctx.Error() if cancelled before the WaitGroup closes.
	ctx context.Context

	// closedCtx is used internally to signal that the WaitGroup has been closed.
	closedCtx context.Context
	// signalClose is used to cancel closedCtx.
	signalClose context.CancelFunc

	// panicCtx is closed if the internal counter drops below zero. All future / current
	// calls to Add(), Done() and Wait() will panic if panicCtx is closed.
	panicCtx context.Context
	// signalPanic closes panicCtx
	signalPanic context.CancelFunc

	// events is an internal queue of Add() and Done() events to be applied to the
	// counter.
	events chan waitEvent

	// counter is the internal tracker for Add() and Done() operations.
	counter int
}

func (group *CtxWaitGroup) handleEvent(event waitEvent) (stop bool) {
	// Delta the event to the counter
	group.counter += event.Delta

	// If the counter is greater than or equal to 0, the operation is a success.
	if group.counter >= 0 {
		// Return a nil result to the caller, indicating success. We need to send an
		// event here so we block until the caller gets the result. If we were to just
		// close the channel, the caller might select the closed context before seeing
		// the closed success channel.
		event.Success <- struct{}{}

		// If the counter has hit 0, signalClose the WaitGroup.
		if group.counter == 0 {
			// Close the group
			group.signalClose()
			return true
		}

		// Otherwise, continue.
		return false
	}

	// Otherwise the counter is below zero, return an error to the caller and
	// signalClose the WaitGroup.

	// Signal all methods to panic.
	group.signalPanic()
	return true
}

// listen manages calls to add and done.
func (group *CtxWaitGroup) listen() {
	// On exit set the event queue to nil so future operations cannot send to it, but
	// also do not panic
	defer func() {
		group.events = nil
	}()

	// We do not want to signalClose the group if the context cancelled so that the
	// cancellation error is forced to return rather than the closed error.

	// Loop over events.
	for stop := false; !stop; {
		select {
		case <-group.ctx.Done():
			// If the context cancels, stop.
			stop = true
		case event := <-group.events:
			stop = group.handleEvent(event)
		}
	}
}

// Sends a wait event to the listener and gets the response back
func (group *CtxWaitGroup) sendWaitEvent(delta int, method string) error {
	success := make(chan struct{})

	// Send the event.
	select {
	case group.events <- waitEvent{Delta: delta, Success: success}:
	case <-group.closedCtx.Done():
		return fmt.Errorf(
			"cannot %v(): %w",
			method,
			ErrWaitGroupClosed,
		)
	case <-group.ctx.Done():
		return fmt.Errorf(
			"cannot %v(): CtxWaitGroup context cancelled: %w",
			method,
			group.ctx.Err(),
		)
	case <-group.panicCtx.Done():
		panic("internal counter dropped below 0")
	}

	// Get the result.
	select {
	case <-success:
	case <-group.panicCtx.Done():
		panic("internal counter dropped below 0")
	case <-group.closedCtx.Done():
		return fmt.Errorf(
			"cannot %v(): %w",
			method,
			ErrWaitGroupClosed,
		)
	case <-group.ctx.Done():
		// If the context has expires, return an error.
		return fmt.Errorf(
			"cannot %v(): CtxWaitGroup context cancelled: %w",
			method,
			group.ctx.Err(),
		)
	}

	return nil
}

// Add adds delta to the WaitGroup. Returns an error if the passed context has been
// cancelled. Panics id the internal counter drops below -1.
func (group *CtxWaitGroup) Add(delta int) error {
	return group.sendWaitEvent(delta, "Delta")
}

// Done decrements the WaitGroup by 1. Returns an error is the context has been
// cancelled. Panics if the internal counter drops below -1.
func (group *CtxWaitGroup) Done() error {
	return group.sendWaitEvent(-1, "Done")
}

// Wait waits until the WaitGroup closes OR the ctx expires.
func (group *CtxWaitGroup) Wait() error {
	select {
	case <-group.closedCtx.Done():
		return nil
	case <-group.ctx.Done():
		return group.ctx.Err()
	case <-group.panicCtx.Done():
		panic("internal counter dropped below 0")
	}
}

// NewCtxWaitGroup creates a new CtxWaitGroup that will signalClose when it's internal
// counter hits 0 OR ctx is cancelled.
func NewCtxWaitGroup(ctx context.Context) *CtxWaitGroup {
	closedCtx, signalClose := context.WithCancel(context.Background())
	panicCtx, signalPanic := context.WithCancel(context.Background())

	waitGroup := &CtxWaitGroup{
		ctx:         ctx,
		closedCtx:   closedCtx,
		signalClose: signalClose,
		panicCtx:    panicCtx,
		signalPanic: signalPanic,
		events:      make(chan waitEvent, 16),
	}

	go waitGroup.listen()
	return waitGroup
}
