package pkerr_test

import (
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"google.golang.org/grpc/codes"
	"os"
)

// Create new sentinel errors.
func ExampleErrorGenerator_NewSentinel() {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	// This error generator can be used to create sentinels and
	generator := pkerr.NewErrGenerator(
		"PingServer", // appName
		hostname,     // hostName
		true,         // addStackTrace
		"Backend",    // sentinelIssuer
		false,        // applyEnvSettings
	)

	// This error acts as a sentinel error, like io.EOF.
	var ErrNotATeapot = generator.NewSentinel(
		"NotATeapot",             // name
		2000,                     // code
		codes.FailedPrecondition, // grpcCode
		"target is not a teapot", // defaultMessage
	)

	fmt.Println("SENTINEL:", ErrNotATeapot)
}
