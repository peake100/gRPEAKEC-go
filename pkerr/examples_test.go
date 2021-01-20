package pkerr_test

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"os"
)

func ExampleNewErrGenerator() {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	// This error generator can be used to create sentinels and errors on the client
	// side.
	_ = pkerr.NewErrGenerator(
		"PingServer", // appName
		hostname,     // appHost
		true,         // addStackTrace
	)
}
