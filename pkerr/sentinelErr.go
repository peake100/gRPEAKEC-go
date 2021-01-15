package pkerr

import (
	"fmt"
	"google.golang.org/grpc/codes"
)

// SentinelError is used for creating sentinel errors within the go implementation of a
// backend for given Error codes for quick errors.Is() checking.
//
// Issuer "", API coded 1000-1999 are reserved by grpcErr for creating a set of default
// error codes.
//
// The base code for unknown error is 1000.
type SentinelError struct {
	// Issuer is the issuer of a code. If multiple services use this library, they
	// can differentiate their error codes by having unique issuers. If a number of
	// services working together in the same backend coordinate to ensure their error
	// definitions do not overlap, they can share an Issuer value.
	Issuer string
	// The error code that defines this error.
	Code uint32
	// Human-readable name tied to the error Code.
	Name string
	// GrpcCode is the status.Status.Code that grpc servers should set when reporting
	// this error.
	GrpcCode codes.Code
	// The default message to be returned by the sentinel version of this def.
	DefaultMessage string

	// codeOriginal stores the original code value this sentinel was created with
	// for applying offsets multiple times.
	codeOriginal uint32
}

// Error implements builtins.error
func (code *SentinelError) Error() string {
	return fmt.Sprintf(
		"%v %v (%v): %v", code.Issuer, code.Name, code.Code, code.DefaultMessage,
	)
}

// As allows extracting a wrapped SentinelError as an APIError.
func (code *SentinelError) As(target interface{}) bool {
	errorDef, ok := target.(*APIError)
	if !ok {
		return false
	}

	*errorDef = newAPIErrBasic(code, "", nil, nil)
	return true
}
