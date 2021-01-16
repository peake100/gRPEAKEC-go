package pkerr

import (
	"fmt"
	"github.com/illuscio-dev/protoCereal-go/cerealMessages"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"runtime/debug"
	"time"
)

// GrpcCodeErr can be used to wrap codes.Code when checking if an APIError is from a
// specific codes.Code using errors.Is.
type GrpcCodeErr codes.Code

// Error implements builtins.error.
func (code GrpcCodeErr) Error() string {
	return fmt.Sprint(uint32(code))
}

// APIError holds the protobuf *Error and adds some runtime context to it. In general,
// callers should not be working directly with the *Error type, and should be using
// APIError instead.
type APIError struct {
	Err    *Error
	Source error
}

// Error implements builtins.error
func (err APIError) Error() string {
	if err.Source != nil {
		return fmt.Sprintf("%v | from: %v", err.Err.Error(), err.Source.Error())
	}

	return err.Err.Error()
}

// Unwrap implements xerrors.Wrapper
func (err APIError) Unwrap() error {
	return err.Source
}

// Is allows comparison to APIError, SentinelError and *Error values through
// errors.Is().
func (err APIError) Is(target error) bool {
	// If the issuer and code are the same, it is the same error.
	switch otherInfo := target.(type) {
	// For APIError, *SentinelError, and *Error, we just need to make sure the error
	// code and issuer are the same
	case APIError:
		return err.Err.Code == otherInfo.Err.Code &&
			err.Err.Issuer == otherInfo.Err.Issuer
	case *SentinelError:
		return err.Err.Code == otherInfo.Code && err.Err.Issuer == otherInfo.Issuer
	case *Error:
		return err.Err.Code == otherInfo.Code && err.Err.Issuer == otherInfo.Issuer
	// If we are comparing against a gRPC code, then we need to take another tact.
	case GrpcCodeErr:
		// Wrap the code in a GrpcCodeErr and compare it to the one coming in.
		return GrpcCodeErr(err.Err.GrpcCode) == otherInfo
	default:
		return false
	}
}

// newAPIErrBasic creates a new *Error object without applying data that is normally
// determined by the error generator.
func newAPIErrBasic(
	sentinel *SentinelError,
	message string,
	details []proto.Message,
	source error,
) APIError {
	var detailsPacked []*anypb.Any
	for _, thisDetail := range details {
		packed, err := anypb.New(thisDetail)
		if err != nil {
			continue
		}
		details = append(details, packed)
	}

	// Add our instance message to our sentinel message.
	fullMessage := sentinel.DefaultMessage
	if message != "" {
		fullMessage = fullMessage + ": " + message
	}

	newProto := &Error{
		Id:       cerealMessages.MustUUIDRandom(),
		Issuer:   sentinel.Issuer,
		Code:     sentinel.Code,
		GrpcCode: int32(sentinel.GrpcCode),
		Name:     sentinel.Name,
		Message:  fullMessage,
		// Time will be the current time as UTC.
		Time:    timestamppb.New(time.Now().UTC()),
		Details: detailsPacked,
		Trace: []*TraceInfo{
			{
				AppName:           "",
				AppHost:           "",
				StackTrace:        string(debug.Stack()),
				AdditionalContext: "",
			},
		},
	}

	newErr := APIError{
		Err:    newProto,
		Source: source,
	}

	return newErr
}
