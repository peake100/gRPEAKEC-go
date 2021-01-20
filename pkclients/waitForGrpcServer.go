package pkclients

import (
	"context"
	"errors"
	"github.com/peake100/gRPEAKEC-go/pktesting/protogen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var retryCodes = [2]codes.Code{codes.Unavailable, codes.NotFound}

func retryPing(
	ctx context.Context, client protogen.MockUnaryServiceClient,
) (err error, retry bool) {
	// Ping the server
	_, err = client.Dummy(ctx, new(emptypb.Empty))

	// If there was no error, we successfully pinged the server, and can return.
	if err == nil {
		return nil, false
	}

	// If the error returns as a context error, our context expired, we should log
	// thee error and return.
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return err, false
	}

	// Check if we got a status message back. If we did not, then the error was on the
	// connection level, and we need to try again.
	responseStatus, ok := status.FromError(err)
	if !ok {
		return err, true
	}

	// Check if the status code is one of our retry codes.
	for _, retryCode := range retryCodes {
		if responseStatus.Code() == retryCode {
			return err, true
		}
	}

	// Otherwise, if we got any other error, it's an error from the method handler /
	// server, which means it's up an running, even if there was an error accepting
	// the request.
	return nil, false
}

// WaitForGrpcServer blocks until the gRPC server clientConn is connected to is
// handling requests.
func WaitForGrpcServer(
	ctx context.Context, clientConn grpc.ClientConnInterface,
) error {
	client := protogen.NewMockUnaryServiceClient(clientConn)

	var err error
	for retry := true; retry; {
		err, retry = retryPing(ctx, client)
	}

	return err
}
