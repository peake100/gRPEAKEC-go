package pkservices

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"time"
)

// ManagerOpts are the options for running a manager
type ManagerOpts struct {
	// grpcServiceAddress is the address our gRPC server will be listening on.
	grpcServiceAddress string
	// grpcServerOpts will be passed to grpc.NewServer when creating the grpc server.
	grpcServerOpts []grpc.ServerOption
	// grpcStreamMiddleware is a list of pkmiddleware.StreamServerMiddleware to use.
	grpcStreamMiddleware []pkmiddleware.StreamServerMiddleware
	// grpcUnaryMiddleware is a list of pkmiddleware.UnaryServerMiddleware to use.=
	grpcUnaryMiddleware []pkmiddleware.UnaryServerMiddleware

	// maxShutdownDuration is the maximum amount of time a shutdown can take before the
	// manager force-exits.
	maxShutdownDuration time.Duration
	// addPingService tells the manager whether to add the gRPC ping service to the gRPC
	// server.
	addPingService bool

	// errGenerator holds the pkerr.ErrorGenerator we should use to create grpc.Server
	// interceptors for error handling.
	errGenerator *pkerr.ErrorGenerator

	// Logger to use.
	logger zerolog.Logger
}

// WithGrpcServerAddress sets the address the gRPC server should handle rpc calls on.
//
// Default: ':5051'.
func (opts *ManagerOpts) WithGrpcServerAddress(address string) *ManagerOpts {
	opts.grpcServiceAddress = address
	return opts
}

// WithGrpcServerOpts stores server opts to create the gRPC server with. When called
// multiple times, new grpc.ServerOption values are added to list, rather than replacing
// values from the previous call.
//
// Default: nil.
func (opts *ManagerOpts) WithGrpcServerOpts(
	grpcOpts ...grpc.ServerOption,
) *ManagerOpts {
	opts.grpcServerOpts = append(opts.grpcServerOpts, grpcOpts...)
	return opts
}

// WithGrpcPingService adds a protogen.PingServer service to the server that can be used
// to test if the server is taking requests.
//
// If true, the service will be added regardless of whether there are any other gRPC
// services the manager is handling.
//
// Default: true.
func (opts *ManagerOpts) WithGrpcPingService(addService bool) *ManagerOpts {
	opts.addPingService = addService
	return opts
}

// WithMaxShutdownDuration sets the maximum duration the manager should allow for the
// services and resources to shut down gracefully before cancelling the shutdown
// context and force-exiting.
func (opts *ManagerOpts) WithMaxShutdownDuration(max time.Duration) *ManagerOpts {
	opts.maxShutdownDuration = max
	return opts
}

// WithErrorGenerator adds error-handling middleware from the passed generator
// to the list of gRPC server options.
//
// Test clients will have corresponding interceptors added to them as well.
//
// Default: nil
func (opts *ManagerOpts) WithErrorGenerator(
	errGen *pkerr.ErrorGenerator,
) *ManagerOpts {
	opts.errGenerator = errGen
	return opts
}

// WithGrpcUnaryServerMiddleware adds unary server middlewares for the gRPC server.
func (opts *ManagerOpts) WithGrpcUnaryServerMiddleware(
	middleware ...pkmiddleware.UnaryServerMiddleware,
) *ManagerOpts {
	opts.grpcUnaryMiddleware = append(opts.grpcUnaryMiddleware, middleware...)
	return nil
}

// WithGrpcStreamServerMiddleware adds stream server middlewares for the gRPC server.
func (opts *ManagerOpts) WithGrpcStreamServerMiddleware(
	middleware ...pkmiddleware.StreamServerMiddleware,
) {
	opts.grpcStreamMiddleware = append(opts.grpcStreamMiddleware, middleware...)
}

// createUnaryMiddlewareInterceptor creates a grpc.UnaryServerInterceptor that invokes
// all configured middleware on each rpc call.
func (
	opts *ManagerOpts,
) createUnaryMiddlewareInterceptor() grpc.UnaryServerInterceptor {
	middleware := make(
		[]pkmiddleware.UnaryServerMiddleware, 0, len(opts.grpcUnaryMiddleware),
	)
	if opts.errGenerator != nil {
		middleware = append(middleware, opts.errGenerator.UnaryServerMiddleware)
	}

	for _, thisMiddleware := range opts.grpcUnaryMiddleware {
		middleware = append(middleware, thisMiddleware)
	}

	return pkmiddleware.NewUnaryServerMiddlewareInterceptor(middleware...)
}

// createStreamMiddlewareInterceptor creates a grpc.StreamServerInterceptor that invokes
// all configured middleware on each rpc call.
func (
	opts *ManagerOpts,
) createStreamMiddlewareInterceptor() grpc.StreamServerInterceptor {
	middleware := make(
		[]pkmiddleware.StreamServerMiddleware, 0, len(opts.grpcUnaryMiddleware),
	)
	if opts.errGenerator != nil {
		middleware = append(middleware, opts.errGenerator.StreamServerMiddleware)
	}

	for _, thisMiddleware := range opts.grpcStreamMiddleware {
		middleware = append(middleware, thisMiddleware)
	}

	return pkmiddleware.NewStreamServerMiddlewareInterceptor(middleware...)
}

// WithLogger sets a zerolog.Logger to use for the manger.
//
// Default: Global Logger.
func (opts *ManagerOpts) WithLogger(logger zerolog.Logger) *ManagerOpts {
	opts.logger = logger
	return opts
}

// NewManagerOpts creates a new *ManagerOpts value with default options set.
func NewManagerOpts() *ManagerOpts {
	return new(ManagerOpts).
		WithGrpcServerAddress(DefaultGrpcAddress).
		WithMaxShutdownDuration(30 * time.Second).
		WithGrpcPingService(true).
		WithLogger(zerolog.Logger{})
}
