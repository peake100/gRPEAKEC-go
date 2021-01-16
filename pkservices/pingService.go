package pkservices

import (
	"context"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/peake100/gRPEAKEC-go/pkservices/protogen"
	"google.golang.org/grpc"
	"sync"
)

// PingServer is a type alias to protogen.PingServer
type PingServer = protogen.PingServer

// PingClient is a type alias to protogen.PingClient
type PingClient = protogen.PingClient

// NewPingClient is a type alias to protogen.NewPingClient
var NewPingClient = protogen.NewPingClient

// pingService is a basic implementation of PingServer that the manager can use to test
// connectivity to the server.
type pingService struct{}

// Id implements Service and returns "gPEAKERC Ping".
func (ping pingService) Id() string {
	return "gPEAKERC Ping"
}

// Setup implements Service.
func (ping pingService) Setup(
	resourcesCtx context.Context, resourcesReleased *sync.WaitGroup,
) error {
	return nil
}

// RegisterOnServer implements GrpcService.
func (ping pingService) RegisterOnServer(server *grpc.Server) {
	protogen.RegisterPingServer(server, ping)
}

// Ping implements PingServer. It receives an empty message and returns the
// result.
func (ping pingService) Ping(
	ctx context.Context, msg *empty.Empty,
) (*empty.Empty, error) {
	return msg, nil
}
