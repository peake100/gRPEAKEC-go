package pkservices

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"time"
)

// grpcLoggingOpts holds options for logging middleware.
type grpcLoggingOpts struct {
	logRPCLevel  zerolog.Level
	logReqLevel  zerolog.Level
	logRespLevel zerolog.Level
	logErrors    bool
	errorTrace   bool
}

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
	// addGrpcLoggingMiddleware: if true, gRPC logging middleware should be added to the
	// gRPC server.
	addGrpcLoggingMiddleware bool
	// addErrNotFoundMiddleware: if true, pkerr.NewErrNotFoundMiddleware is added to the
	// grpcUnaryMiddleware as the first middleware.
	addErrNotFoundMiddleware bool
	// grpcLoggingOpts are the options to use when creating logging middleware
	grpcLoggingOpts grpcLoggingOpts

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

// WithGrpcLogging will add logging middleware to the grpc server using
// pkmiddleware.NewLoggingMiddleware.
//
// Default:
//
// logRPCLevel: zerolog.DebugLevel
//
// logReqLevel: zerolog.Disabled + 1 (won't be logged)
//
// logRespLevel: zerolog.Disabled + 1 (won't be logged)
//
// logErrors: true
//
// errorTrace: true
func (opts *ManagerOpts) WithGrpcLogging(
	logRPCLevel zerolog.Level,
	logReqLevel zerolog.Level,
	logRespLevel zerolog.Level,
	logErrors bool,
	errorTrace bool,
) *ManagerOpts {
	opts.addGrpcLoggingMiddleware = true
	opts.grpcLoggingOpts = grpcLoggingOpts{
		logRPCLevel:  logRPCLevel,
		logReqLevel:  logReqLevel,
		logRespLevel: logRespLevel,
		logErrors:    logErrors,
		errorTrace:   errorTrace,
	}

	return opts
}

// WithoutGrpcLogging keeps the logging middleware from being added to the gRPC server.
func (opts *ManagerOpts) WithoutGrpcLogging() *ManagerOpts {
	opts.addGrpcLoggingMiddleware = false
	return opts
}

// WithGrpcUnaryServerMiddleware adds unary server middlewares for the gRPC server.
func (opts *ManagerOpts) WithGrpcUnaryServerMiddleware(
	middleware ...pkmiddleware.UnaryServerMiddleware,
) *ManagerOpts {
	opts.grpcUnaryMiddleware = append(opts.grpcUnaryMiddleware, middleware...)
	return nil
}

// WithGrpcStreamServerMiddleware adds stream server middlewares for the gRPC
// server.
//
// Default: nil.
func (opts *ManagerOpts) WithGrpcStreamServerMiddleware(
	middleware ...pkmiddleware.StreamServerMiddleware,
) {
	opts.grpcStreamMiddleware = append(opts.grpcStreamMiddleware, middleware...)
}

// WithErrNotFoundMiddleware adds pkerr.NewErrNotFoundMiddleware to the unary
// middlewares.
//
// Default: false.
func (opts *ManagerOpts) WithErrNotFoundMiddleware() *ManagerOpts {
	opts.addErrNotFoundMiddleware = true
	return opts
}

// createUnaryMiddlewareInterceptor creates a grpc.UnaryServerInterceptor that invokes
// all configured middleware on each rpc call.
func (
	opts *ManagerOpts,
) createUnaryMiddlewareInterceptor() grpc.UnaryServerInterceptor {
	middleware := make(
		[]pkmiddleware.UnaryServerMiddleware, 0, len(opts.grpcUnaryMiddleware),
	)

	// We have to add NewErrNotFoundMiddleware before the error middleware so that the
	// full error middleware can transform it into a full blown error.
	if opts.addErrNotFoundMiddleware {
		notFoundMiddleware := pkerr.NewErrNotFoundMiddleware(opts.errGenerator)
		middleware = append(middleware, notFoundMiddleware)
	}

	// The error middleware should come now so any remaining middleware can inspect the
	// converted errors.
	if opts.errGenerator != nil {
		middleware = append(middleware, opts.errGenerator.UnaryServerMiddleware)
	}

	// Next add the user-created middleware.
	for _, thisMiddleware := range opts.grpcUnaryMiddleware {
		middleware = append(middleware, thisMiddleware)
	}

	// Add logging middleware if needed. This should be the last middleware added so it
	// captures the raw request from the client and the final error returned from
	// previous middleware.
	if opts.addGrpcLoggingMiddleware {
		loggingMiddleware := pkmiddleware.NewUnaryLoggingMiddleware(
			opts.logger,
			opts.grpcLoggingOpts.logRPCLevel,
			opts.grpcLoggingOpts.logReqLevel,
			opts.grpcLoggingOpts.logRespLevel,
			opts.grpcLoggingOpts.logErrors,
			opts.grpcLoggingOpts.errorTrace,
		)
		middleware = append(middleware, loggingMiddleware)
	}

	// Wrap our middleware in an interceptor.
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

	// The error middleware should come now so any remaining middleware can inspect the
	// converted errors.
	if opts.errGenerator != nil {
		middleware = append(middleware, opts.errGenerator.StreamServerMiddleware)
	}

	// Next add the user-created middleware.
	for _, thisMiddleware := range opts.grpcStreamMiddleware {
		middleware = append(middleware, thisMiddleware)
	}

	// Add logging middleware if needed. This should be the last middleware added so it
	// captures the raw request from the client and the final error returned from
	// previous middleware.
	if opts.addGrpcLoggingMiddleware {
		loggingMiddleware := pkmiddleware.NewStreamLoggingMiddleware(
			opts.logger,
			opts.grpcLoggingOpts.logRPCLevel,
			opts.grpcLoggingOpts.logReqLevel,
			opts.grpcLoggingOpts.logRespLevel,
			opts.grpcLoggingOpts.logErrors,
			opts.grpcLoggingOpts.errorTrace,
		)
		middleware = append(middleware, loggingMiddleware)
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
		WithMaxShutdownDuration(30*time.Second).
		WithGrpcPingService(true).
		WithLogger(zerolog.Logger{}).
		WithGrpcLogging(
			// RPC method calls will only be logged for a Debug-level logger.
			zerolog.DebugLevel,
			// Requests message values will never be logged.
			zerolog.Disabled+1,
			// Response message values will never be logged.
			zerolog.Disabled+1,
			// Errors will be logged.
			true,
			// Trace() will be called on errors.
			true,
		)
}
