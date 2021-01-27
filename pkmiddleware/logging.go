package pkmiddleware

import (
	"context"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"math/rand"
	"time"
)

const loggerContextKey = "GRPEAKEC_LOGGER"

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
	logRpcLevel zerolog.Level,
	logReqLevel zerolog.Level,
	logRespLevel zerolog.Level,
	logErrors bool,
	errorTrace bool,
) (unary UnaryServerMiddleware, stream StreamServerMiddleware) {
	// Create our unary logger.
	unary = func(next UnaryServerHandler) UnaryServerHandler {
		return func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
		) (resp interface{}, err error) {
			// Create a logger with both the method and the request traced.
			methodLogger := logger.With().
				Str("GRPC_METHOD", info.FullMethod).
				Int("REQ_ID", rand.Int()).
				Logger()

			// We're going to add some more info to our middleware logger that we
			// don't want to have on the logger
			rpcLogger := methodLogger
			logRpc := methodLogger.GetLevel() <= logRpcLevel
			if logRpc && rpcLogger.GetLevel() <= logReqLevel {
				rpcLogger = rpcLogger.With().
					Interface("REQ", req).
					Logger()
			}

			// Add the logger to the context.
			ctx = context.WithValue(ctx, loggerContextKey, methodLogger)

			// If our log level is at our under our access log cutoff, log this request.
			var startTime time.Time
			if logRpc {
				startTime = time.Now().UTC()
			}

			// Invoke the handler.
			resp, err = next(ctx, req, info)
			if logRpc {
				duration := time.Now().UTC().Sub(startTime)
				rpcLogger = rpcLogger.With().
					Dur("DURATION", duration).
					Logger()

				if methodLogger.GetLevel() <= logRespLevel {
					rpcLogger = rpcLogger.With().
						Interface("RESP", resp).
						Logger()
				}
			}

			// If logErrors is true, log the error.
			if logErrors && err != nil {
				event := rpcLogger.Err(err)
				if errorTrace {
					event = event.Stack()
				}
				event.Msg("")
			} else if logRpc {
				rpcLogger.WithLevel(logRpcLevel).Msg("")
			}

			// Return the error if there is an error.
			if err != nil {
				return resp, err
			}

			// Return the response if there is no error.
			return resp, nil
		}
	}

	return unary, stream
}
