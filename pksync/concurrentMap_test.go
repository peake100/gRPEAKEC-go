package pksync_test

import (
	"context"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pksync"
	"github.com/peake100/gRPEAKEC-go/pktesting"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func TestConcurrentMap(t *testing.T) {
	assert := assert.New(t)

	values := []int{1, 2}
	results := make([]int, len(values))

	ctx, cancel := pktesting.New3SecondCtx()
	defer cancel()

	complete := pksync.NewCtxWaitGroup(ctx)
	err := complete.Add(len(values))
	if !assert.NoError(err, "add to WaitGroup") {
		t.FailNow()
	}

	var mapFunc pksync.ConcurrentMapFunc = func(
		ctx context.Context, value interface{}, index int,
	) error {
		intVal := value.(int)
		results[index] = intVal + 1

		// We'll mark done and then wait on the other operations, which means these
		// had to be done concurrently.
		err := complete.Done()
		if err != nil {
			return err
		}
		return complete.Wait()
	}

	errChan := pksync.ConcurrentMap(ctx, values, mapFunc)

	// Range over channel until it is closed.
	for err := range errChan {
		assert.NoError(err, "no error from channel")
	}

	assert.Equal(2, results[0], "result 0 expected")
	assert.Equal(3, results[1], "result 1 expected")
}

func TestConcurrentMap_Error(t *testing.T) {
	assert := assert.New(t)

	values := []int{1, 2}

	var mapFunc pksync.ConcurrentMapFunc = func(
		ctx context.Context, value interface{}, index int,
	) error {
		intVal := value.(int)
		return fmt.Errorf("could not process value '%v': %w", intVal, io.EOF)
	}

	ctx, cancel := pktesting.New3SecondCtx()
	defer cancel()

	mapErrs := make([]error, 0, len(values))
	errChan := pksync.ConcurrentMap(ctx, values, mapFunc)
	for mapErr := range errChan {
		mapErrs = append(mapErrs, mapErr)
		assert.Error(mapErr, "map err is error")
		assert.ErrorIs(mapErr, io.EOF)
	}

	if !assert.Len(mapErrs, 2) {
		t.FailNow()
	}

	errStrings := []string{mapErrs[0].Error(), mapErrs[1].Error()}
	assert.Contains(
		errStrings,
		"error calling mapFunc on value 0: could not process value '1': EOF",
	)
	assert.Contains(
		errStrings,
		"error calling mapFunc on value 1: could not process value '2': EOF",
	)
}

func TestConcurrentMap_CtxCancel(t *testing.T) {
	assert := assert.New(t)

	routinesRun := false

	var mapFunc pksync.ConcurrentMapFunc = func(
		ctx context.Context, value interface{}, index int,
	) error {
		routinesRun = true
		return nil
	}

	// Cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	values := []int{1, 2}

	mapErrs := make([]error, 0)
	for mapErr := range pksync.ConcurrentMap(ctx, values, mapFunc) {
		mapErrs = append(mapErrs, mapErr)
		assert.ErrorIs(mapErr, context.Canceled, "err is cancelled")
	}

	assert.False(routinesRun, "no routines run")
	assert.Len(mapErrs, 1, "1 error returned")
}

func TestConcurrentMap_NonSlicePanic(t *testing.T) {
	assert := assert.New(t)

	assert.Panics(func() {
		pksync.ConcurrentMap(context.Background(), 5, nil)
	}, "non-slice values panics")
}
