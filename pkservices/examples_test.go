package pkservices_test

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/peake100/gRPEAKEC-go/pkclients"
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/peake100/gRPEAKEC-go/pkservices/protogen"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
)

// pingService is a basic implementation of PingServer that the manager can use to test
// connectivity to the server.
type ExampleService struct {
}

// Id implements Service and returns "ExampleService".
func (ping ExampleService) Id() string {
	return "ExampleService"
}

// Setup implements Service.
func (ping ExampleService) Setup(
	resourcesCtx context.Context,
	resourcesReleased *sync.WaitGroup,
	shutdownCtx context.Context,
	logger zerolog.Logger,
) error {
	// Add 1 to the resourcesReleased waitGroup
	resourcesReleased.Add(1)

	// Run some sort of resource (like a db connector, message broker, etc) inside
	// a goroutine.
	go func() {
		// When resourcesCtx is cancelled, this resource should be released and the
		// WaitGroup should be decremented.
		defer resourcesReleased.Done()
		logger.Info().Msg("Resource 1 starting")
		<-resourcesCtx.Done()
		logger.Info().Msg("Resource 1 released")
	}()

	// Add another to the resource.
	resourcesReleased.Add(1)
	go func() {
		defer resourcesReleased.Done()
		logger.Info().Msg("Resource 2 starting")
		<-resourcesCtx.Done()
		logger.Info().Msg("Resource 2 released")
	}()

	// When this function returns, the manager will move on to the run stage.
	return nil
}

// RegisterOnServer implements GrpcService, and handles registering it onto the
// manager's gRPC server.
func (ping ExampleService) RegisterOnServer(server *grpc.Server) {
	protogen.RegisterPingServer(server, ping)
}

// Ping implements PingServer. It receives an empty message and returns the
// result.
func (ping ExampleService) Ping(
	ctx context.Context, msg *empty.Empty,
) (*empty.Empty, error) {
	fmt.Println("PING RECEIVED!")
	return msg, nil
}

// Run a basic gRPC service with a manger.
func ExampleManager_basic() {
	// Our manager options.
	managerOpts := pkservices.NewManagerOpts().
		// We are adding our own implementation of Ping, so we don't need to register
		// The default one.
		WithGrpcPingService(false)

	// Create a new manager to manage our service. Example service implements
	// pkservices.PingServer.
	manager := pkservices.NewManager(managerOpts, ExampleService{})

	// Run the manager in it's own goroutine and return errors to our error channel.
	errChan := make(chan error)
	go func() {
		defer close(errChan)
		errChan <- manager.Run()
	}()

	// make a gRPC client to ping the service
	clientConn, err := grpc.Dial(pkservices.DefaultGrpcAddress, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}

	// create a new client to interact with our server
	pingClient := pkservices.NewPingClient(clientConn)

	// Wait for the server to be serving requests.
	err = pkclients.WaitForGrpcServer(context.Background(), clientConn)
	if err != nil {
		panic(err)
	}

	// Send a ping
	_, err = pingClient.Ping(context.Background(), new(emptypb.Empty))
	if err != nil {
		panic(err)
	}

	// Start shutdown.
	manager.StartShutdown()

	// Grab our error from the error channel (blocks until the manager is shutdown)
	err = <-errChan
	if err != nil {
		panic(err)
	}

	// Exit.

	// Output:
	// PING RECEIVED!
}
