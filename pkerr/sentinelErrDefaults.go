package pkerr

import "google.golang.org/grpc/codes"

const DefaultsIssuer = "GRPEAKEC"

// errorGen is the default generator we are going to use to make our errors.
var errorGen = NewErrGenerator(
	DefaultsIssuer,
	"",
	false,
	DefaultsIssuer,
	true,
)

// ErrUnknown is the default fallback error that is returned from a server interceptor
// when an incoming error is not an APIError, *Error, or SentinelError value.
var ErrUnknown = errorGen.NewSentinel(
	"Unknown",
	1000,
	codes.Unknown,
	"an unknown error occurred",
)

// ReissueDefaultSentinels applies the issuer and offset to all the default
// SentinelError values found in this package, such as ErrUnknown.
func ReissueDefaultSentinels(issuer string, offset int) {
	errorGen.ApplyNewIssuer(issuer, offset)
}
