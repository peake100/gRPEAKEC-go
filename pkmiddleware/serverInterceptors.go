package pkmiddleware

import (
	"context"
	"google.golang.org/grpc"
)

// UnaryServerHandler is the handler signature for unary server calls.
type UnaryServerHandler = func(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
) (resp interface{}, err error)

// UnaryServerMiddleware is the middleware signature for unary server calls.
type UnaryServerMiddleware = func(next UnaryServerHandler) UnaryServerHandler

// newCoreUnaryServerHandler returns the core UnaryServerHandler
func newCoreUnaryServerHandler(handler grpc.UnaryHandler) UnaryServerHandler {
	return func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
	) (resp interface{}, err error) {
		return handler(ctx, req)
	}
}

// NewUnaryServeMiddlewareInterceptor creates an interceptor that calls invokes
// multiple middlewares on each request.
func NewUnaryServerMiddlewareInterceptor(
	middleware ...UnaryServerMiddleware,
) grpc.UnaryServerInterceptor {

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		// Build our handler with the middlewares.
		pkHandler := newCoreUnaryServerHandler(handler)
		for _, thisMiddleware := range middleware {
			pkHandler = thisMiddleware(pkHandler)
		}

		// Run the handler and all it's middleware
		return pkHandler(ctx, req, info)
	}
}

// StreamServerHandler is the handler signature for stream server calls.
type StreamServerHandler = func(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
) (err error)

// StreamServerMiddleware is the middleware signature for stream server calls.
type StreamServerMiddleware = func(next StreamServerHandler) StreamServerHandler

// newCoreStreamServerHandler returns the core StreamServerHandler
func newCoreStreamServerHandler(handler grpc.StreamHandler) StreamServerHandler {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
	) (err error) {
		return handler(srv, ss)
	}
}

func NewStreamServerMiddlewareInterceptor(
	middleware ...StreamServerMiddleware,
) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Build our handler with the middlewares.
		pkHandler := newCoreStreamServerHandler(handler)
		for _, thisMiddleware := range middleware {
			pkHandler = thisMiddleware(pkHandler)
		}

		// Run the handler and all it's middleware
		return pkHandler(srv, ss, info)
	}
}
