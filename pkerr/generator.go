package pkerr

import (
	"google.golang.org/protobuf/proto"
)

// ErrorGenerator generates errors with app-specific settings.
type ErrorGenerator struct {
	// appName to use for Error.Trace
	appName string
	// appHost to use for Error.Trace
	appHost string
	// addStackTrace will add the stack trace of the error call to Error.Trace if true.
	addStackTrace bool
	// sendContext will send additional context the error is wrapped in if true.
	sendContext bool
	// sendSource will send the source error text if true.
	sendSource bool
}

// revive:disable:modifies-value-receiver revive thinks we are wring to try and assign
// to a receiver value here, but we want to make a copy, so it's okay. We're going to
// disable this for the next two methods.

// WithAppName returns a copy of the ErrorGenerator with the appName setting replaced.
func (gen ErrorGenerator) WithAppName(appName string) *ErrorGenerator {
	gen.appName = appName
	return &gen
}

// WithAppHost returns a copy of the ErrorGenerator with the appHost setting replaced.
func (gen ErrorGenerator) WithAppHost(appHost string) *ErrorGenerator {
	gen.appHost = appHost
	return &gen
}

// revive:enable:modifies-value-receiver

// applySettings takes in an APIError and applies the correct information to the
// trace-settings of the host.
func (gen *ErrorGenerator) applySettings(err APIError) APIError {
	if !gen.sendSource {
		err.Proto.SourceError = ""
		err.Proto.SourceType = ""
	}

	lastTrace := err.Proto.Trace[len(err.Proto.Trace)-1]
	lastTrace.AppName = gen.appName
	lastTrace.AppHost = gen.appHost
	if !gen.addStackTrace {
		lastTrace.StackTrace = ""
	}

	if !gen.sendContext {
		lastTrace.AdditionalContext = ""
	}

	return err
}

// NewErr creates a new error with the code, name, issuer, and grpc status code of
// sentinel.
func (gen *ErrorGenerator) NewErr(
	sentinel *SentinelError,
	message string,
	details []proto.Message,
	source error,
) (apiErr error) {

	newErr := newAPIErrBasic(sentinel, message, details, source)
	apiErr = gen.applySettings(newErr)

	return newErr
}

// Creates a new Error generator.
//
// appName and hostName are both applied to *TraceInfo frames added through this
// generator's grpc client interceptors as well as the initial frame from
// ErrorGenerator.NewErr.
//
// addStackTrace will cause a full debug stacktrace to be added when a *TraceInfo value
// is created by this generator.
func NewErrGenerator(
	appName string,
	appHost string,
	addStackTrace bool,
	sendContext bool,
	sendSource bool,
) *ErrorGenerator {
	gen := &ErrorGenerator{
		appName:       appName,
		appHost:       appHost,
		addStackTrace: addStackTrace,
		sendContext:   sendContext,
		sendSource:    sendSource,
	}

	return gen
}
