package pkservices_test

import (
	"context"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/peake100/gRPEAKEC-go/pktesting"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
	"testing"
)

// MockService is a dead simple service for testing the Manager.
type MockService struct {
	ResourcesSpunUp   bool
	ResourcesReleased bool
}

func (service *MockService) Id() string {
	return "MockService"
}

func (service *MockService) Setup(
	resourcesCtx context.Context, resourcesReleased *sync.WaitGroup,
) error {
	resourcesReleased.Add(1)
	// Set up a dummy resource that reports it has been closed.
	go func() {
		defer resourcesReleased.Done()
		defer func() {
			service.ResourcesReleased = true
		}()
		<-resourcesCtx.Done()
	}()

	service.ResourcesSpunUp = true

	return nil
}

func (service *MockService) RegisterOnServer(server *grpc.Server) {
	pktesting.RegisterMockUnaryServiceServer(server, service)
}

func (service *MockService) Unary(ctx context.Context, any *any.Any) (*any.Any, error) {
	panic("implement me")
}

// This is the method handler we will invoke to test the manager
func (service *MockService) Dummy(
	ctx context.Context, empty *empty.Empty,
) (*empty.Empty, error) {
	return empty, nil
}

func TestManager_gRPCLifetime(t *testing.T) {
	assert := assert.New(t)

	// Create the mock service and manager
	mockService := new(MockService)
	opts := pkservices.NewManagerOpts().WithGrpcServerAddress(":7091")

	manager := pkservices.NewManager(opts, mockService)

	// Run the manager in a routine. The runComplete channel will be closed when the
	// manager shuts down.
	runComplete := make(chan struct{})
	go func() {
		defer close(runComplete)
		err := manager.Run()
		assert.NoError(err, "run service")
	}()

	// Ping the server, this will block until the server successfully responds or
	// three seconds expire.
	manager.Test(t).PingGrpcServer(nil)

	// Test that the resources spun up but not yet released.
	assert.True(mockService.ResourcesSpunUp, "resources spun up")
	assert.False(mockService.ResourcesReleased, "resources not released")

	// Get a new client connection and service client.
	clientConn := manager.Test(t).GrpcClientConn(true, grpc.WithInsecure())
	defer clientConn.Close()

	client := pktesting.NewMockUnaryServiceClient(clientConn)

	ctx, cancel := pktesting.New3SecondCtx()
	defer cancel()

	// Make a test request to our server..
	_, err := client.Dummy(ctx, new(emptypb.Empty))
	assert.NoError(err, "make gRPC call to MockService")

	// Shut down the manager
	manager.StartShutdown()

	// Block until the manager exits or our timeout expires.
	ctx, cancel = pktesting.New3SecondCtx()
	defer cancel()

	select {
	case <-ctx.Done():
		assert.NoError(ctx.Err(), "shutdown timed out")
	case <-runComplete:
	}

	// Check that resources were released
	assert.True(mockService.ResourcesReleased, "resources released set")
}

type MockGeneralGeneric struct {
	SetupCalled        chan struct{}
	ResourcesReleased  chan struct{}
	RunCalled          chan struct{}
	ServicesReleased   chan struct{}
	StatShutdownCalled chan struct{}
}

func (mock *MockGeneralGeneric) Setup(
	resourcesCtx context.Context, resourcesReleased *sync.WaitGroup,
) error {
	defer close(mock.SetupCalled)

	resourcesReleased.Add(1)
	go func() {
		defer close(mock.ResourcesReleased)
		defer resourcesReleased.Done()
		<-resourcesCtx.Done()
	}()

	return nil
}

func (mock *MockGeneralGeneric) Run(
	runCtx context.Context,
) error {
	defer close(mock.RunCalled)

	go func() {
		defer close(mock.ServicesReleased)
		<-runCtx.Done()
	}()

	return nil
}

func (mock *MockGeneralGeneric) StartGracefulShutdown(shutdownCtx context.Context) {
	panic("implement me")
}
