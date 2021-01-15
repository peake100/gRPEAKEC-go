package pktesting

import (
	"context"
	"time"
)

// New3SecondCtx returns a context with a 3-second timeout. Useful for testing rpc
// calls.
func New3SecondCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
