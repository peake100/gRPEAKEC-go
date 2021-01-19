package pksync

import (
	"context"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"reflect"
	"sync"
)

// ConcurrentMapError is returned by ConcurrentMap and contains the source error, the
// value, and it's index in the values slice passed to ConcurrentMap.
type ConcurrentMapError struct {
	// MapFuncErr is the error returned by mapFunc. This error wil never be nil. If
	// mapFunc panicked, the panic value will be wrapped in a pkerr.PanicError then
	// stored here.
	MapFuncErr error

	// Value is the value passed to mapFunc. NOTE: if mapFunc mutates this value, Value
	// will be in the value's current state, NOT it's original state.
	Value interface{}

	// ValueIndex is the index of the values where Value was stored.
	ValueIndex int
}

// Unwrap implements xerrors.Wrapper.
func (err ConcurrentMapError) Unwrap() error {
	return err.MapFuncErr
}

// Error implements builtins.error.
func (err ConcurrentMapError) Error() string {
	return fmt.Sprintf(
		"error calling mapFunc on value %v: %v", err.ValueIndex, err.MapFuncErr,
	)
}

// ConcurrentMapFunc is the signature fop the mapFunc parameter of ConcurrentMap.
type ConcurrentMapFunc = func(ctx context.Context, value interface{}, index int) error

// ConcurrentMap calls mapFunc on each value in value in values. Each mapFunc call is
// made concurrently in it's own goroutines wrapped in a pkerr.CatchPanic to catch and
// wrap panics as errors..
//
// Non-nil errors will be wrapped in a ConcurrentMapError and sent through the returned
// channel.
//
// Panics will be wrapped in a pkerr.PanicError before being wrapped in a
// ConcurrentMapError
//
// The returned channel will be closed when all ConcurrentMapFunc invocations have
// exited, allowing the caller to ranger over it.
//
// ctx is passed to each mapFunc invocations as-is. Context-cancelling is left to the
// caller.
//
// Cancelling ctx will stop the mapping process.
func ConcurrentMap(
	ctx context.Context,
	values interface{},
	mapFunc ConcurrentMapFunc,
) <-chan error {
	// Check if the values list is a slice or an array.
	sliceVal := reflect.ValueOf(values)
	if sliceVal.Kind() != reflect.Slice && sliceVal.Kind() != reflect.Array {
		panic("'values' passed to ConcurrentMap must be a slice or array.")
	}
	// Make our return error chan.
	errChan := make(chan error)

	// Apply the mapping in a goroutine so we can return the error channel immediately
	// an let the caller start handling errors.
	go mapFuncToValues(ctx, sliceVal, mapFunc, errChan)

	// Return our error channel for our caller to start collecting from.
	return errChan
}

// mapFuncToValues applies mapFunc to each value of values concurrently.
func mapFuncToValues(
	ctx context.Context,
	values reflect.Value,
	mapFunc ConcurrentMapFunc,
	errChan chan<- error,
) {
	// Launch a goroutine to close the error channel when the map is complete.
	defer close(errChan)

	// Create a WaitGroup to be closed when the map has been applied to all values.
	mapComplete := new(sync.WaitGroup)

	// Range over our values.
	for i := 0; i < values.Len(); i++ {
		// If the caller has cancelled the context, stop.
		if ctx.Err() != nil {
			// Return the context error to the error chan and stop mapping.
			errChan <- ctx.Err()
			break
		}

		// Get the value.
		thisValue := values.Index(i)

		// Launch a goroutine and apply mapFunc to the value inside.
		mapComplete.Add(1)
		go runSingleMap(ctx, mapFunc, i, thisValue, errChan, mapComplete)
	}

	// Wait for all launched goroutines to exit.
	mapComplete.Wait()
}

// runSingleMap applies mapFunc to a single value.
func runSingleMap(
	ctx context.Context,
	mapFunc ConcurrentMapFunc,
	valueIndex int,
	value reflect.Value,
	errChan chan<- error,
	complete *sync.WaitGroup,
) {
	// Decrement the WaitGroup on exit.
	defer complete.Done()

	// Catch any panics.
	err := pkerr.CatchPanic(func() error {
		return mapFunc(ctx, value.Interface(), valueIndex)
	})

	// If there was not error, return.
	if err == nil {
		return
	}

	// Wrap the error in a ConcurrentMapError and return.
	errChan <- ConcurrentMapError{
		MapFuncErr: err,
		Value:      value.Interface(),
		ValueIndex: valueIndex,
	}
}
