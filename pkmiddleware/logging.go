package pkmiddleware

import (
	"context"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"math/rand"
	"time"
)

const loggerContextKey = "GRPEAKEC_LOGGER"

type loggingOpts struct {
	logger     zerolog.Logger
	logLevel   zerolog.Level
	logRPC     bool
	logReq     bool
	logResp    bool
	logErrors  bool
	errorTrace bool
}

// NewLoggingMiddleware creates unary and streaming middleware for logging requests.
//
// logger is the zerolog.Logger to use.
//
// logRpcLevel is what level to log rpcCalls at. The method name, duration and id of the
// request will be logged.
//
// logReqLevel will log the req value on the rpc log if the logger is equal to or under
// the level cutoff.
//
// logRespLevel will log the resp value in the rpc log if the logger is equal to ot
// under the level cutoff.
//
// logErrors: when tue, errors will be logged at an error level if they occur.
//
// errorTrace: when true, a Trace() will be added to error logs.
func NewLoggingMiddleware(
	logger zerolog.Logger,
	logRPCLevel zerolog.Level,
	logReqLevel zerolog.Level,
	logRespLevel zerolog.Level,
	logErrors bool,
	errorTrace bool,
) (unary UnaryServerMiddleware, stream StreamServerMiddleware) {
	opts := setupOpts(
		logger, logRPCLevel, logReqLevel, logRespLevel, logErrors, errorTrace,
	)

	// Create our logging middleware.
	unary = newUnaryLoggingMiddlewareFromOpts(opts)
	stream = newStreamLoggingMiddlewareFromOpts(opts)

	return unary, stream
}

// NewUnaryLoggingMiddleware creates a new UnaryServerMiddleware for logging gRPC
// server unary requests.
func NewUnaryLoggingMiddleware(
	logger zerolog.Logger,
	logRPCLevel zerolog.Level,
	logReqLevel zerolog.Level,
	logRespLevel zerolog.Level,
	logErrors bool,
	errorTrace bool,
) UnaryServerMiddleware {
	opts := setupOpts(
		logger, logRPCLevel, logReqLevel, logRespLevel, logErrors, errorTrace,
	)

	return newUnaryLoggingMiddlewareFromOpts(opts)
}

// NewStreamLoggingMiddleware creates a new StreamServerMiddleware for logging gRPC
// server streams.
func NewStreamLoggingMiddleware(
	logger zerolog.Logger,
	logRPCLevel zerolog.Level,
	logReqLevel zerolog.Level,
	logRespLevel zerolog.Level,
	logErrors bool,
	errorTrace bool,
) StreamServerMiddleware {
	opts := setupOpts(
		logger, logRPCLevel, logReqLevel, logRespLevel, logErrors, errorTrace,
	)

	return newStreamLoggingMiddlewareFromOpts(opts)
}

func setupOpts(
	logger zerolog.Logger,
	logRPCLevel zerolog.Level,
	logReqLevel zerolog.Level,
	logRespLevel zerolog.Level,
	logErrors bool,
	errorTrace bool,
) loggingOpts {
	logRPC := logger.GetLevel() <= logRPCLevel
	logReq := logRPC && logger.GetLevel() <= logReqLevel
	logResp := logRPC && logger.GetLevel() <= logRespLevel

	return loggingOpts{
		logger:     logger,
		logLevel:   logRPCLevel,
		logRPC:     logRPC,
		logReq:     logReq,
		logResp:    logResp,
		logErrors:  logErrors,
		errorTrace: errorTrace,
	}
}

// newUnaryLoggingMiddlewareFromOpts creates a UnaryServerMiddleware for logging.
func newUnaryLoggingMiddlewareFromOpts(opts loggingOpts) UnaryServerMiddleware {
	return func(next UnaryServerHandler) UnaryServerHandler {
		return func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
		) (resp interface{}, err error) {
			// Create a logger with both the method and the request traced.
			methodLogger, startTime := createMethodLogger(
				opts, info.FullMethod, "unary",
			)

			// We're going to add some more info to our middleware logger that we
			// don't want to have on the logger
			rpcLogger := methodLogger
			if opts.logReq {
				rpcLogger = rpcLogger.With().
					Interface("REQ", req).
					Logger()
			}

			// Add the logger to the context.
			ctx = context.WithValue(ctx, loggerContextKey, methodLogger)

			// Invoke the handler.
			resp, err = next(ctx, req, info)

			// If we are logging responses, add the response.
			if opts.logResp {
				rpcLogger = rpcLogger.With().
					Interface("RESP", resp).
					Logger()
			}

			logRPCMessage(rpcLogger, opts, startTime, err)

			// Return the response if there is no error.
			return resp, err
		}
	}
}

// loggingServerStream wraps grpc.ServerStream with logging abilities.
type loggingServerStream struct {
	grpc.ServerStream

	ctx    context.Context
	logger zerolog.Logger
	opts   loggingOpts
}

// Context returns a ctx with a logger value embedded.
func (stream loggingServerStream) Context() context.Context {
	return stream.ctx
}

// SendMsg logs the message to be send if logResp is on.
func (stream loggingServerStream) SendMsg(m interface{}) error {
	err := stream.ServerStream.SendMsg(m)
	if !stream.opts.logResp && (err == nil || !stream.opts.logErrors) {
		return err
	}

	message := "send stream message"
	logger := stream.logger
	if stream.opts.logResp {
		logger = stream.logger.With().Interface("MESSAGE", m).Logger()
	}

	if err != nil && stream.opts.logErrors {
		event := buildErrEvent(err, stream.opts, logger)
		event.Msg(message)
	} else {
		logger.WithLevel(stream.opts.logLevel).Msg(message)
	}

	return err
}

// RecvMsg logs the message to be send if logReq is on.
func (stream loggingServerStream) RecvMsg(m interface{}) error {
	err := stream.ServerStream.RecvMsg(m)

	if !stream.opts.logReq && (err == nil || !stream.opts.logErrors) {
		return err
	}

	message := "receive stream message"
	logger := stream.logger
	if stream.opts.logReq {
		logger = stream.logger.With().Interface("MESSAGE", m).Logger()
	}

	if err != nil && stream.opts.logErrors {
		event := buildErrEvent(err, stream.opts, logger)
		event.Msg(message)
	} else {
		logger.WithLevel(stream.opts.logLevel).Msg(message)
	}

	return err
}

// newStreamLoggingMiddlewareFromOpts creates a logging middleware for streaming
// requests.
func newStreamLoggingMiddlewareFromOpts(opts loggingOpts) StreamServerMiddleware {
	return func(next StreamServerHandler) StreamServerHandler {
		return func(
			srv interface{},
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
		) (err error) {
			// Create a logger with both the method and the request traced.
			rpcLogger, startTime := createMethodLogger(
				opts, info.FullMethod, "stream",
			)

			if ss != nil {
				// Wrap our server stream.
				ss = loggingServerStream{
					ServerStream: ss,
					ctx:          addLoggerToContext(ss.Context(), rpcLogger),
					logger:       rpcLogger,
					opts:         opts,
				}
			}

			// Call our next middleware, passing our server stream into it.
			err = next(srv, ss, info)

			// Log the result of the overall rpc.
			logRPCMessage(rpcLogger, opts, startTime, err)

			// Return the error.
			return err
		}
	}
}

func logRPCMessage(
	rpcLogger zerolog.Logger,
	opts loggingOpts,
	startTime time.Time,
	err error,
) {
	// If we are not logging RPCs and not logging errors, or the error is nil, we can
	// return immediately.
	if !opts.logRPC && (!opts.logErrors || err == nil) {
		return
	}

	// If we are logging RPCs, add the duration.
	if opts.logRPC {
		duration := time.Now().UTC().Sub(startTime)
		rpcLogger = rpcLogger.With().
			Dur("DURATION", duration).
			Logger()
	}

	// If logErrors is true, log any errors.
	if opts.logErrors && err != nil {
		event := buildErrEvent(err, opts, rpcLogger)
		event.Msg("")
		return
	}

	// Otherwise, log the RPC at the configured log level.
	rpcLogger.WithLevel(opts.logLevel).Msg("rpc completed")
}

// buildErrEvent builds the logger event for an error.
func buildErrEvent(err error, opts loggingOpts, logger zerolog.Logger) *zerolog.Event {
	event := logger.Error()
	if opts.errorTrace {
		event = event.Stack()
	}
	return event.Err(err)
}

// createMethodLogger creates a new logger with the rpc method context and returns a
// start time for adding the call duration.
func createMethodLogger(
	opts loggingOpts, fullMethod string, methodKind string,
) (logger zerolog.Logger, start time.Time) {
	logger = opts.logger.With().
		Str("GRPC_METHOD", fullMethod).
		Str("METHOD_KIND", methodKind).
		Int("RPC_ID", rand.Int()).
		Logger()

	// If our log level is at our under our access log cutoff, log this request.
	if opts.logRPC {
		start = time.Now().UTC()
	}

	return logger, start
}

func addLoggerToContext(ctx context.Context, logger zerolog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// LoggerFromCtx returns a method or stream-specific logger with context fields already
// set by the logging middleware.
//
// If logging middleware is not in use, this method panic
func LoggerFromCtx(ctx context.Context) zerolog.Logger {
	value := ctx.Value(loggerContextKey)
	logger, ok := value.(zerolog.Logger)
	if !ok {
		panic(
			"logger could not be extracted from context. Please make sure logging" +
				" pkmiddleware is registered.",
		)
	}

	return logger
}
