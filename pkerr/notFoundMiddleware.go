package pkerr

import (
	"context"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"google.golang.org/grpc"
	"reflect"
)

// NewErrNotFoundMiddleware returns ErrNotFound on any unary calls that return both a
// nil response and a nil error.
//
// By using a generator we assure that any included stacktrace points back to this
// middleware definitively.
func NewErrNotFoundMiddleware(
	errGen *ErrorGenerator,
) pkmiddleware.UnaryServerMiddleware {
	return func(next pkmiddleware.UnaryServerHandler) pkmiddleware.UnaryServerHandler {
		return func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
		) (resp interface{}, err error) {
			resp, err = next(ctx, req, info)
			if err == nil && reflect.ValueOf(resp).IsNil() {
				// We want to use the error gen here so the stacktrace points back to
				// this middleware definitively.
				return nil, errGen.NewErr(
					ErrNotFound,
					"",
					nil,
					nil,
				)
			}
			return resp, err
		}
	}
}
