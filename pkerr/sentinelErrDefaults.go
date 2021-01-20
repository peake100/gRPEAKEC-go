package pkerr

import "google.golang.org/grpc/codes"

const DefaultsIssuer = "GRPEAKEC"

// sentinelIssuer is the default generator we are going to use to make our errors.
var sentinelIssuer = NewSentinelIssuer(
	DefaultsIssuer,
	true,
)

// ErrUnknown is the default fallback error that is returned from a server interceptor
// when an incoming error is not an APIError, *Error, or SentinelError value.
var ErrUnknown = sentinelIssuer.NewSentinel(
	"Unknown",
	1000,
	codes.Unknown,
	"an unknown error occurred",
)

// ErrNotFound is the default error for requesting a resource that does not exist.
var ErrNotFound = sentinelIssuer.NewSentinel(
	"ResourceExists",
	1001,
	codes.NotFound,
	"requested resource not found",
)

// ErrAlreadyExists is the default error for requesting a resource that already exists.
var ErrAlreadyExists = sentinelIssuer.NewSentinel(
	"AlreadyExists",
	1002,
	codes.AlreadyExists,
	"resource already exists",
)

// ErrValidation should be returned when a request fails validation.
var ErrValidation = sentinelIssuer.NewSentinel(
	"Validation",
	1003,
	codes.InvalidArgument,
	"client argument failed validation",
)

// ReissueDefaultSentinels applies the issuer and offset to all the default
// SentinelError values found in this package, such as ErrUnknown.
func ReissueDefaultSentinels(issuer string, offset int) {
	sentinelIssuer.ApplyNewIssuer(issuer, offset)
}
