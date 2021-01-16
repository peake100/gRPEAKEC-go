package pkservices

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
	"time"
)

// ManagerTesting exposes testing methods for testing a service manager. These methods
// are not safe to use in production code.
type ManagerTesting struct {
	t       *testing.T
	manager *Manager
}

// Services returns the list of registered services. This method is for testing purposes
// only and returned values should not be modified unless you know what you are doing!
func (tester ManagerTesting) Services() []Service {
	return tester.manager.services
}

// GrpcClientConn generates a grpc.ClientConn with the passed opts connected to the gRPC
// server address in the manager options.
//
// If Dialing the server results in an error, an error will be added to the test and
// t.FailNow() is called to exit the test immediately.
//
// If cleanup is set to true then a cleanup function will be registered that closes
// the connection on test completion.
//
// If an error generator was passed to the manager for server interceptors, the
// corresponding client interceptors will be added to the grpc.ClientConn.
func (tester ManagerTesting) GrpcClientConn(
	cleanup bool, opts ...grpc.DialOption,
) *grpc.ClientConn {
	// Add client interceptors to opts if we are using them.
	if tester.manager.opts.errGenerator != nil {
		errGen := tester.manager.opts.errGenerator
		opts = append(
			opts,
			grpc.WithUnaryInterceptor(errGen.NewUnaryClientInterceptor()),
			grpc.WithStreamInterceptor(errGen.NewStreamClientInterceptor()),
		)
	}

	conn, err := grpc.Dial(tester.manager.opts.grpcServiceAddress, opts...)
	if !assert.NoError(tester.t, err, "dial gRPC server address") {
		tester.t.FailNow()
	}

	if cleanup {
		// Register a cleanup function that closes the connection.
		tester.t.Cleanup(func() { conn.Close() })
	}

	return conn
}

// GrpcPingClient returns a PingClient connected to the PingServer running on the gRPC
// server. If ManagerOpts.WithGrpcPingService was set to false, an error will be logged
// to the test and t.FailNow() will be called,
//
// If cleanup is set to true then a cleanup function will be registered that closes
// the underlying clientConn on test completion.
func (tester ManagerTesting) GrpcPingClient(
	cleanup bool, opts ...grpc.DialOption,
) PingClient {
	if !assert.True(
		tester.t, tester.manager.opts.addPingService, "ping service in use",
	) {
		tester.t.FailNow()
	}

	clientConn := tester.GrpcClientConn(cleanup, opts...)
	return NewPingClient(clientConn)
}

func (tester ManagerTesting) retryPing(
	ctx context.Context, client PingClient, retryCount int,
) (ok bool) {
	// Ping the server
	_, err := client.Ping(ctx, new(emptypb.Empty))

	// If there was no error, we successfully pinged the server, and can return.
	if err == nil {
		return true
	}

	// If the error returns as a context error, our context expired, we should log
	// thee error and return.
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {

		assert.NoError(
			tester.t, err, "ping gRPC server after %v tries", retryCount,
		)
		tester.t.FailNow()
	}

	// Check if we got a status message back
	responseStatus, ok := status.FromError(err)
	if !ok {
		return false
	}

	// If we got a status back, and the status is unimplemented, then that means the
	// server is up, it just does not implement Ping, which is fine.
	if responseStatus.Code() == codes.Unimplemented {
		return true
	}

	return false
}

// PingGrpcServer will continually ping the gRPC server's PingServer.Ping method until
// a connection is established or the passed context times out. All errors will be
// ignored and ping will be tried again on failure.
//
// If this method returns, the server is up an running and ready to take requests.
//
// If ctx expires, the ctx.Error() will be logged and FailNow() called on the test.
//
// If ctx is nil, a default 3-second context will be used.
func (tester ManagerTesting) PingGrpcServer(ctx context.Context) {
	if ctx == nil {
		defaultCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		ctx = defaultCtx
	}

	client := tester.GrpcPingClient(true, grpc.WithInsecure())

	retries := 0

	for ok := false; !ok; {
		retries++
		ok = tester.retryPing(ctx, client, retries)
	}
}
