package pksync_test

import (
	"context"
	"github.com/peake100/gRPEAKEC-go/pksync"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func TestNewCtxWaitGroup_Basic(t *testing.T) {
	assert := assert.New(t)

	group := pksync.NewCtxWaitGroup(context.Background())

	results := [10]int{}

	for i := range results {
		err := group.Add(1)
		assert.NoErrorf(err, "add to group %v", i)

		go func(i int) {
			defer group.Done()
			time.Sleep(10 * time.Millisecond)
			results[i] = i
		}(i)
	}

	err := group.Wait()
	assert.NoError(err, "wait for signalClose.")

	for i, val := range results {
		assert.Equal(i, val, "result set")
	}

	err = group.Add(1)
	assert.ErrorIs(err, pksync.ErrWaitGroupClosed, "Delta() after signalClose")

	err = group.Done()
	assert.ErrorIs(err, pksync.ErrWaitGroupClosed, "Done() after signalClose")

}

// Tests that cancelling context returns error for Wait, Done, and Err
func TestNewCtxWaitGroup_ContextCancelled(t *testing.T) {
	assert := assert.New(t)

	block := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	group := pksync.NewCtxWaitGroup(ctx)
	finished := new(sync.WaitGroup)

	for i := 0; i < 10; i++ {
		err := group.Add(1)
		assert.NoErrorf(err, "add to group %v", i)

		finished.Add(1)
		go func(i int) {
			defer finished.Done()

			<-block

			err := group.Done()
			assert.Error(err, "done %v", i)
			assert.ErrorIs(err, context.Canceled, "done %v", i)
		}(i)
	}

	cancel()

	err := group.Wait()
	assert.Error(err, "Wait()")
	assert.ErrorIs(err, context.Canceled, "Wait()")

	close(block)

	finished.Wait()

	err = group.Add(1)
	assert.Error(err, "Delta()")
	assert.ErrorIs(err, context.Canceled, "Delta()")

}

// Tests that reducing the internal context below zero causes a panic.
func TestNewCtxWaitGroup_SubZeroPanic(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	group := pksync.NewCtxWaitGroup(ctx)

	testCases := []struct {
		Name string
		Done func() error
		Add  func(int) error
		Wait func() error
	}{
		{
			Name: "Done",
			Done: group.Done,
		},
		{
			Name: "Delta",
			Add:  group.Add,
		},
		{
			Name: "Wait",
			Wait: group.Wait,
		},
		{
			Name: "Delta",
			Add:  group.Add,
		},
		{
			Name: "Delta",
			Add:  group.Add,
		},
	}

	for _, thisCase := range testCases {
		panicFunc := func() {
			if thisCase.Done != nil {
				thisCase.Done()
			} else if thisCase.Add != nil {
				thisCase.Add(1)
			} else {
				thisCase.Wait()
			}
		}

		t.Run(thisCase.Name, func(t *testing.T) {
			assert.Panicsf(
				panicFunc,
				"%v()", thisCase.Name,
			)
		})
	}

	assert.Panics(
		func() {
			group.Add(1)
		},
		"Delta()",
	)

	assert.Panics(
		func() {
			group.Wait()
		},
		"Wait()",
	)
}
