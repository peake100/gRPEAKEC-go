package pkerr

import (
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"os"
	"strconv"
)

// ErrorGenerator generates errors with app-specific settings.
type ErrorGenerator struct {
	// appName to use for Error.Trace
	appName string
	// appHost to use for Error.Trace
	appHost string
	// addStackTrace will add the stack trace of the error call to Error.Trace if true.
	addStackTrace bool

	// sentinelIssuer the issuer we will use for making new ErrorDefs
	sentinelIssuer string

	// sentinelOffset is the offset that will be applied to any Sentinel error created
	// with ErrorGenerator.NewSentinel
	sentinelOffset int

	// createdSentinels is a list of SentinelError pointers this generator has created.
	createdSentinels []*SentinelError
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

// applyTraceSettings takes in an APIError and applies the correct information to the
// trace-settings of the host.
func (gen *ErrorGenerator) applyTraceSettings(err APIError) APIError {
	lastTrace := err.Err.Trace[len(err.Err.Trace)-1]
	lastTrace.AppName = gen.appName
	lastTrace.AppHost = gen.appHost
	if !gen.addStackTrace {
		lastTrace.StackTrace = ""
	}

	return err
}

// NewSentinel creates a new *SentinelError and saves the pointer so it can be altered /
// offset later.
func (gen *ErrorGenerator) NewSentinel(
	name string,
	code uint32,
	grpcCode codes.Code,
	defaultMessage string,
) *SentinelError {
	sentinel := &SentinelError{
		Issuer: gen.sentinelIssuer,
		// We need to cast to int then cast back to uint32 in case the offset is
		// negative.
		Code:           uint32(int64(code) + int64(gen.sentinelOffset)),
		Name:           name,
		GrpcCode:       grpcCode,
		DefaultMessage: defaultMessage,

		// Store the original, non-offset code.
		codeOriginal: code,
	}

	gen.createdSentinels = append(gen.createdSentinels, sentinel)

	return sentinel
}

// ApplyNewIssuer rewrites all issued sentinels to have the given issuer and offset.
func (gen *ErrorGenerator) ApplyNewIssuer(issuer string, offset int) {
	// Set the new issuer and offset
	gen.sentinelIssuer = issuer
	gen.sentinelOffset = offset

	for _, sentinel := range gen.createdSentinels {
		sentinel.Issuer = issuer
		sentinel.Code = uint32(int64(sentinel.codeOriginal) + int64(offset))
	}
}

// applyEnvSettings loads issuer and code offset information from environmental
// variables and applies them to the generator, using the current settings as default
// if no env var is found.
//
// The environmental variables are the following:
//
// - [AppName]_ERROR_ISSUER
// - [AppName]_ERROR_CODE_OFFSET
//
// By having these settings configurable as environmental variables, two generic
// services that both use this error library can be merged into a single backend, and
// appear more uniform to a caller by having the same issuer.
//
// The code offset ensures that error codes can be shifted so they do not collide when
// two generic services are used in the same backend.
//
// When this method is called, the new issuer and offset are applied to all previously
// created sentinel errors. This method should not be called more than once, as it will
// cause the offset to be applied multiple times.
func (gen *ErrorGenerator) applyEnvSettings() *ErrorGenerator {
	issuerEnvKey := gen.appName + "_ERROR_ISSUER"
	offsetEnvKey := gen.appName + "_ERROR_CODE_OFFSET"

	issuer, ok := os.LookupEnv(issuerEnvKey)
	if !ok {
		issuer = gen.sentinelIssuer
	}

	offsetStr, ok := os.LookupEnv(offsetEnvKey)
	if !ok {
		offsetStr = strconv.Itoa(gen.sentinelOffset)
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		err = fmt.Errorf(
			"%v error generator could not parse value of '%v' env var: %w",
			gen.appName,
			offsetEnvKey,
			err,
		)
		panic(err)
	}

	gen.ApplyNewIssuer(issuer, offset)
	return gen
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
	apiErr = gen.applyTraceSettings(newErr)

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
//
// sentinelIssuer is the value to use for all SentinelError values created with
// ErrorGenerator.NewSentinel.
//
// applyEnvSettings loads issuer and code offset information from environmental
// variables and applies them to the generator, using the current settings as default
// if no env var is found.
//
// The environmental variables are the following:
//
// - [AppName]_ERROR_ISSUER
// - [AppName]_ERROR_CODE_OFFSET
//
// By having these settings configurable as environmental variables, two generic
// services that both use this error library can be merged into a single backend, and
// appear more uniform to a caller by having the same issuer.
//
// The code offset ensures that error codes can be shifted so they do not collide when
// two generic services are used in the same backend.
func NewErrGenerator(
	appName string,
	appHost string,
	addStackTrace bool,
	sentinelIssuer string,
	applyEnvSettings bool,
) *ErrorGenerator {
	gen := &ErrorGenerator{
		appName:          appName,
		appHost:          appHost,
		addStackTrace:    addStackTrace,
		sentinelIssuer:   sentinelIssuer,
		createdSentinels: nil,
	}

	if applyEnvSettings {
		gen.applyEnvSettings()
	}

	return gen
}
