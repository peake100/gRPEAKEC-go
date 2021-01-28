package pkerr_test

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
)

func ExampleNewErrGenerator() {
	// This error generator can be used to create sentinels and errors on the client
	// side.
	_ = pkerr.NewErrGenerator(
		"PingServer", // appName
		true,         // addHost
		true,         // addStackTrace
		true,         // sendContext
		true,         // sendSource
	)
}
