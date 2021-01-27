package pkerr

import (
	"context"
	"errors"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"runtime/debug"
	"strings"
)

// clientErr wraps a status.Status.Error(), and implements a new Error() method
// so that putting the original client error in Source when receiving an error
// from a service does not cause the error message to be printed twice (once in
// ' | from: [message]')
type clientErr struct {
	statusErr error
}

// Error implements builtins.error and removes message from status error so that it
// isn't printed twice in APIError.Error() when the source error is reported.
func (err clientErr) Error() string {
	statusErr, _ := status.FromError(err.statusErr)
	return fmt.Sprintf("grpc error '%v'", statusErr.Code())
}

// Unwrap implements xerrors.Wrapper.
func (err clientErr) Unwrap() error {
	return err.statusErr
}

// extractClientErrorFromInvoker takes an error from an invocation of a grpc client
// handler and converts it to an APIError if the error contains am *Error detail.
func (gen *ErrorGenerator) extractClientErrorFromInvoker(err error) error {
	if err == nil {
		// If there was not error, return
		return nil
	}

	// If we cannot extract a grpc.status, return the error.
	errStatus, ok := status.FromError(err)
	if !ok {
		return err
	}

	// Look through the details for an Error message, and return it if we find
	// one.
	for _, thisDetail := range errStatus.Details() {
		errProto, ok := thisDetail.(*Error)
		if !ok {
			continue
		}

		// Delta this caller's trace information to the error trace.
		thisTraceInfo := &TraceInfo{
			AppName: gen.appName,
			AppHost: gen.appHost,
		}
		if gen.addStackTrace {
			thisTraceInfo.StackTrace = string(debug.Stack())
		}
		errProto.Trace = append(errProto.Trace, thisTraceInfo)

		// Return an APIError object.
		return APIError{
			// Put our error info in here.
			Proto: errProto,
			// Delta the original error so it can be unwrapped if desired.
			Source: clientErr{statusErr: err},
		}
	}

	// Otherwise return the error as-is.
	return err
}

// UnaryClientMiddleware implements pkmiddleware.UnaryClientMiddleware and handles
// decoding an *Error detail as an APIError.
//
// If an *Error detail message is found in the status of an error return, the message
// will be extracted into an APIError, and a new *TraceInfo frame will be added.
//
// *Error values are generated using the settings of ErrorGenerator.
func (gen *ErrorGenerator) UnaryClientMiddleware(
	next pkmiddleware.UnaryClientHandler,
) pkmiddleware.UnaryClientHandler {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		cc *grpc.ClientConn,
		opts ...grpc.CallOption,
	) (reply interface{}, err error) {
		reply, err = next(ctx, method, req, cc, opts...)
		if err != nil {
			err = gen.extractClientErrorFromInvoker(err)
		}
		return reply, err
	}
}

// clientStream wraps grpc.ClientStream and converts incoming errors to APIError.
type clientStream struct {
	grpc.ClientStream
	gen *ErrorGenerator
}

// RecvMsg wraps errors originating from the embedded stream's RecvMsg method as
// APIError.
func (stream *clientStream) RecvMsg(m interface{}) (err error) {
	err = stream.ClientStream.RecvMsg(m)
	return stream.gen.extractClientErrorFromInvoker(err)
}

// StreamClientMiddleware implements pkmiddleware.StreamClientMiddleware and
// converts incoming errors to an APIError if the error contains an *Error detail, and a
// new *TraceInfo frame will be added.
//
// *Error values are generated using the settings of ErrorGenerator.
func (gen *ErrorGenerator) StreamClientMiddleware(
	next pkmiddleware.StreamClientHandler,
) pkmiddleware.StreamClientHandler {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		stream, err := next(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}

		return &clientStream{
			ClientStream: stream, gen: gen,
		}, nil
	}
}

// errToAPIErrConvertType converts a generic error to an APIError()
//
// The pkText of the error is the part of the full error.Error() our APIError is
// responsible for.
func (gen *ErrorGenerator) errToAPIErrConvertType(
	source error,
) (apiErr APIError, pkText string) {
	var sentinelErr *SentinelError
	if errors.As(source, &sentinelErr) {
		newErr := gen.NewErr(sentinelErr, "", nil, source)
		apiErr = newErr.(APIError)
		if sentinelErr.Error() != source.Error() {
			pkText = sentinelErr.Error()
		}
	} else if errors.As(source, &apiErr) {
		// If this is an APIError that has been wrapped, multiple times, put the full
		// error in additional context.
		apiErr = gen.applySettings(apiErr)
		if apiErr.Error() != source.Error() {
			pkText = apiErr.Error()
		}
	} else {
		newErr := gen.NewErr(ErrUnknown, source.Error(), nil, source)
		apiErr = newErr.(APIError)
	}

	return apiErr, pkText
}

// errToAPIErr takes in a raw error and converts it to an APIError.
func (gen *ErrorGenerator) errToAPIErr(source error) APIError {
	// Convert the error an get the part of the error message our error is
	// responsible for.
	apiErr, pkText := gen.errToAPIErrConvertType(source)

	// If baseText is not empty, replace it with "[error]" and set that to
	// "Additional Context"
	if pkText != "" {
		latestTrace := apiErr.Proto.Trace[len(apiErr.Proto.Trace)-1]
		latestTrace.AdditionalContext = strings.Replace(
			source.Error(), pkText, "[error]", -1,
		)
	}

	return apiErr
}

// errToStatus converts returned error to status.
func (gen *ErrorGenerator) errToStatus(source error) error {
	// The first thing we need to do if convert the error to an APIError
	apiErr := gen.errToAPIErr(source)

	// Create a new status with the correct error code and message.
	thisStatus := status.New(
		codes.Code(apiErr.Proto.GrpcCode),
		apiErr.Error(),
	)

	// Delta our *Error message as a detail.
	withDetails, err := thisStatus.WithDetails(apiErr.Proto)
	if err == nil {
		// If there was no error, make it our returned status
		thisStatus = withDetails
	}

	// Return the status with our error information.
	return thisStatus.Err()
}

// NewUnaryServerInterceptor implements pkmiddleware.UnaryServerMiddleware that can
// handle wrapping all errors and panics in an APIError and transforming them into a
// status.Status.
func (gen *ErrorGenerator) UnaryServerMiddleware(
	next pkmiddleware.UnaryServerHandler,
) pkmiddleware.UnaryServerHandler {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
	) (resp interface{}, err error) {
		defer func() {
			if err != nil {
				err = gen.errToStatus(err)
			}
		}()

		// Catch the panic if it occurs.
		err = CatchPanic(func() error {
			// Invoke the handler.
			var handlerErr error
			resp, handlerErr = next(ctx, req, info)
			return handlerErr
		})

		return resp, err
	}
}

// StreamServerMiddleware implements pkmiddleware.StreamServerMiddleware that can handle
// wrapping all errors and panics in an APIError and transforming them into a
// status.Status.
func (gen *ErrorGenerator) StreamServerMiddleware(
	next pkmiddleware.StreamServerHandler,
) pkmiddleware.StreamServerHandler {
	return func(
		srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
	) (err error) {
		defer func() {
			if err != nil {
				err = gen.errToStatus(err)
			}
		}()

		// Catch the panic if it occurs.
		err = CatchPanic(func() error {
			return next(srv, ss, info)
		})

		return err
	}
}
