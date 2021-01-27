package pkmiddleware

import (
	"context"
	"google.golang.org/grpc"
)

// UnaryClientHandler is the handler signature for a unary client rpc.
type UnaryClientHandler = func(
	ctx context.Context,
	method string,
	req interface{},
	cc *grpc.ClientConn,
	opts ...grpc.CallOption,
) (reply interface{}, err error)

// UnaryClientHandler is the handler signature for unary client middleware.
type UnaryClientMiddleware = func(next UnaryClientHandler) UnaryClientHandler

// newCoreUnaryClientHandler creates the core UnaryClientHandler for a unary client rpc
func newCoreUnaryClientHandler(
	invoker grpc.UnaryInvoker,
	reply interface{},
) UnaryClientHandler {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		cc *grpc.ClientConn,
		opts ...grpc.CallOption,
	) (middleWareReply interface{}, err error) {
		err = invoker(ctx, method, req, reply, cc, opts...)
		return reply, err
	}
}

// NewUnaryClientMiddlewareInterceptor creates a new grpc.UnaryClientInterceptor which
// invokes the passed middleware on each unary rpc.
func NewUnaryClientMiddlewareInterceptor(
	middleware ...UnaryClientMiddleware,
) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		pkHandler := newCoreUnaryClientHandler(invoker, reply)
		for _, thisMiddleware := range middleware {
			pkHandler = thisMiddleware(pkHandler)
		}

		_, err := pkHandler(ctx, method, req, cc, opts...)
		return err
	}
}

// StreamClientHandler is the handler signature for a stream client rpc.
type StreamClientHandler = func(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error)

// StreamClientHandler is the handler signature for stream client middleware.
type StreamClientMiddleware = func(next StreamClientHandler) StreamClientHandler

// newCoreStreamClientHandler creates the core StreamClientHandler for a stream client
// rpc.
func newCoreStreamClientHandler(streamer grpc.Streamer) StreamClientHandler {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// NewStreamClientMiddlewareInterceptor returns a grpc.StreamClientInterceptor that
// invokes all middleware on each client streaming rpc method.
func NewStreamClientMiddlewareInterceptor(
	middleware ...StreamClientMiddleware,
) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		pkHandler := newCoreStreamClientHandler(streamer)
		for _, thisMiddleware := range middleware {
			pkHandler = thisMiddleware(pkHandler)
		}

		return pkHandler(ctx, desc, cc, method, opts...)
	}
}
