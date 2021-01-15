package pkerr

import (
	"context"
	"errors"
	"fmt"
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
			Err: errProto,
			// Delta the original error so it can be unwrapped if desired.
			Source: clientErr{statusErr: err},
		}
	}

	// Otherwise return the error as-is.
	return err
}

// NewUnaryClientInterceptor returns a new client interceptor for handing APIErrors.
// If an *Error detail message is found in the status of an error return, the message
// will be extracted into an APIError, and a new *TraceInfo frame will be added.
//
// *Error values are generated using the settings of ErrorGenerator.
func (gen *ErrorGenerator) NewUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Get the error from the invoker.
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Extract an APIError from the return if it exists.
		return gen.extractClientErrorFromInvoker(err)
	}
}

type clientStream struct {
	grpc.ClientStream
	gen *ErrorGenerator
}

func (stream *clientStream) RecvMsg(m interface{}) (err error) {
	err = stream.ClientStream.RecvMsg(m)
	return stream.gen.extractClientErrorFromInvoker(err)
}

// NewStreamClientInterceptor returns a new grpc.StreamClientInterceptor that converts
// incoming errors to an APIError if the error contains an *Error detail, and a new
// *TraceInfo frame will be added.
//
// *Error values are generated using the settings of ErrorGenerator.
func (gen *ErrorGenerator) NewStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Get and convert the error.

		stream, err := streamer(ctx, desc, cc, method, opts...)
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
		apiErr = gen.applyTraceSettings(apiErr)
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
		latestTrace := apiErr.Err.Trace[len(apiErr.Err.Trace)-1]
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
		codes.Code(apiErr.Err.GrpcCode),
		apiErr.Error(),
	)

	// Delta our *Error message as a detail.
	withDetails, err := thisStatus.WithDetails(apiErr.Err)
	if err == nil {
		// If there was no error, make it our returned status
		thisStatus = withDetails
	}

	// Return the status with our error information.
	return thisStatus.Err()
}

// panicToStatus converts a recovered panic to an error. Do not pass in nil values.
func (gen *ErrorGenerator) panicToStatus(recovered interface{}) error {
	// Check if the recovered object is an error.
	err, ok := recovered.(error)
	if !ok {
		// If the recovered data is not an error, just format the value as a string.
		err = fmt.Errorf("%+v", recovered)
	}

	// Convert the error to a status.
	return gen.errToStatus(err)
}

func (gen *ErrorGenerator) handleServerError(recovered interface{}, err error) error {
	if recovered != nil {
		return gen.panicToStatus(recovered)
	} else if err != nil {
		return gen.errToStatus(err)
	}

	return nil
}

// NewUnaryServerInterceptor returns a grpc.UnaryServerInterceptor that can handle
// wrapping all errors and panics in an APIError and transforming them into a
// status.Status.
func (gen *ErrorGenerator) NewUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		// Handle panics and errors.
		defer func() {
			err = gen.handleServerError(recover(), err)
		}()

		// Invoke the handler.
		resp, err = handler(ctx, req)
		return resp, err
	}
}

// NewStreamServerInterceptor returns a grpc.StreamServerInterceptor that can handle
// wrapping all errors and panics in an APIError and transforming them into a
// status.Status.
func (gen *ErrorGenerator) NewStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		// Handle panics and errors.
		defer func() {
			err = gen.handleServerError(recover(), err)
		}()

		err = handler(srv, ss)
		return err
	}
}
