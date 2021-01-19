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
	"io"
	"sync"
	"testing"
	"time"
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

type MockGenericService struct {
	SetupCalled       chan struct{}
	ResourcesReleased chan struct{}
	RunCalled         chan struct{}
	ServicesReleased  chan struct{}

	UnblockSetup     chan struct{}
	UnblockRun       chan struct{}
	UnblockResources chan struct{}
}

func (mock *MockGenericService) CheckStageSignaled(
	t *testing.T, stage string, signal chan struct{}, timer *time.Timer,
) {
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("%v not signaled before timeout", stage)
	}
}

func (mock *MockGenericService) CheckStagedBlocked(
	t *testing.T, stage string, signal chan struct{},
) {
	select {
	case <-signal:
		t.Fatalf("%v signaled too early", stage)
	default:
	}
}

func (mock *MockGenericService) Id() string {
	return "MockGenericService"
}

func (mock *MockGenericService) Setup(
	resourcesCtx context.Context, resourcesReleased *sync.WaitGroup,
) error {
	resourcesReleased.Add(1)
	go func() {
		// Close the resources released WaitGroup on exit.
		defer resourcesReleased.Done()

		// Signal that setup has been called.
		close(mock.SetupCalled)

		// Wait for the resources context to be cancelled.
		<-resourcesCtx.Done()

		// Signal that resources have been released.
		close(mock.ResourcesReleased)

		// Block until the tester signals we should return. That way we can test that
		// run will not return until resources are released.
		<-mock.UnblockResources
	}()

	// Block until the tester tells us we can advance. This allows us to test that
	// Run is not called until after setup is complete.
	<-mock.UnblockSetup

	return nil
}

func (mock *MockGenericService) Run(
	runCtx context.Context, shutdownCtx context.Context,
) error {
	defer close(mock.RunCalled)

	go func() {
		// Wait for run to be released
		<-runCtx.Done()

		// Signal that the services have been released
		close(mock.ServicesReleased)

		// Block again until the tester allows us to continue. This lets us test that
		// resources are not released until run returns.
		<-mock.UnblockRun
	}()

	return nil
}

// This test runs a generic service that tracks where in the lifecycle of the service
// the manager is and signals on each stage, as well as blocking the manager from
// advancing until the tester is done inspecting the current state.
//
// This will allow us to ensure that events are not happening until they are supposed
// to.
func TestManager_genericLifetime(t *testing.T) {
	// Setup our services and manager.
	service := &MockGenericService{
		SetupCalled:       make(chan struct{}),
		ResourcesReleased: make(chan struct{}),
		RunCalled:         make(chan struct{}),
		ServicesReleased:  make(chan struct{}),
		UnblockSetup:      make(chan struct{}),
		UnblockRun:        make(chan struct{}),
		UnblockResources:  make(chan struct{}),
	}

	manager := pkservices.NewManager(nil, service)
	defer manager.StartShutdown()

	managerComplete := make(chan struct{})

	// Run our manager.
	go func() {
		defer close(managerComplete)

		err := manager.Run()
		assert.NoError(t, err, "run manager")
	}()

	// Setup a timeout for this test.
	timeout := time.NewTimer(3 * time.Second)

	// Check that setup is called.
	service.CheckStageSignaled(t, "Setup", service.SetupCalled, timeout)
	// Check that run has not yet been called
	service.CheckStagedBlocked(t, "Run", service.RunCalled)

	// Allow Setup to return
	close(service.UnblockSetup)
	// Check that run is called.
	service.CheckStageSignaled(t, "Run", service.RunCalled, timeout)
	// Check that resources have bot been released yet, this should invoke the default
	// method since service.ResourcesReleased should not yet be closed.
	service.CheckStagedBlocked(t, "Resources Released", service.ResourcesReleased)

	// Likewise, check that services have not been released yet.
	service.CheckStagedBlocked(t, "Services Released", service.ServicesReleased)

	// start shutdown.
	manager.StartShutdown()

	// Check that services are released
	service.CheckStageSignaled(
		t, "Services Released", service.ServicesReleased, timeout,
	)
	// Check that resources have bot been released yet since we are not yet letting
	// run return.
	service.CheckStagedBlocked(t, "Resources Released", service.ResourcesReleased)
	// Double check that run has not returned yet.
	service.CheckStagedBlocked(t, "Manger Exit", managerComplete)

	// release run
	close(service.UnblockRun)

	// Check that services are released now that run has finished.
	service.CheckStageSignaled(
		t, "Resources Released", service.ResourcesReleased, timeout,
	)
	// Double check that run has not returned yet, since resources are still blocked.
	service.CheckStagedBlocked(t, "Manger Exit", managerComplete)

	// allow the resource to exit
	close(service.UnblockResources)

	// The hanging resource should have been the final thing blocking the manager from
	// exiting.
	service.CheckStageSignaled(
		t, "Manger Exit", managerComplete, timeout,
	)

	// We're done! We've inspected the full lifecycle flow of the manger
}

type GenericErrService struct {
	ErrOnSetup bool
	ErrOnRun   bool
}

func (s GenericErrService) Id() string {
	return "GenericErrService"
}

func (s GenericErrService) Setup(
	resourcesCtx context.Context, resourcesReleased *sync.WaitGroup,
) error {
	if s.ErrOnSetup {
		return io.EOF
	}

	return nil
}

func (s GenericErrService) Run(
	runCtx context.Context, shutdownCtx context.Context,
) error {
	if s.ErrOnRun {
		return io.EOF
	}

	return nil
}

type managerStages int

const (
	managerStagesSetup managerStages = iota
	managerStagesRun
	managerStagesShutdown
)

func checkManagerErr(
	t *testing.T, service pkservices.GenericService, stage managerStages,
) {
	assert := assert.New(t)

	manager := pkservices.NewManager(nil, service)
	t.Cleanup(func() {
		manager.StartShutdown()
		manager.WaitForShutdown()
	})

	runErr := manager.Run()
	if !assert.Error(runErr, "run had error") {
		t.FailNow()
	}

	t.Log("MANAGER ERR:", runErr)

	managerErr := pkservices.ManagerError{}
	if !assert.ErrorAs(runErr, &managerErr, "error is manager error") {
		t.FailNow()
	}

	var stageErr error
	switch stage {
	case managerStagesSetup:
		stageErr = managerErr.SetupErr
		assert.NoError(managerErr.RunErr, "no error on run")
		assert.NoError(managerErr.ShutdownErr, "no error on shutdown")
	case managerStagesRun:
		stageErr = managerErr.RunErr
		assert.NoError(managerErr.SetupErr, "no error on setup")
		assert.NoError(managerErr.ShutdownErr, "no error on shutdown")
	case managerStagesShutdown:
		stageErr = managerErr.ShutdownErr
		assert.NoError(managerErr.SetupErr, "no error on setup")
		assert.NoError(managerErr.RunErr, "no error on run")
	}

	if !assert.Errorf(stageErr, "%v has error", stage) {
		t.FailNow()
	}

	servicesErrors := pkservices.ServicesErrors{}
	if !assert.ErrorAs(
		stageErr, &servicesErrors, "setup is ServicesError",
	) {
		t.FailNow()
	}

	if !assert.Len(servicesErrors.Errs, 1, "1 services error") {
		t.FailNow()
	}

	underlyingErr := servicesErrors.Errs[0]

	serviceErr := pkservices.ServiceError{}
	if !assert.ErrorAs(underlyingErr, &serviceErr, "err is service err") {
		t.FailNow()
	}
}

func TestNewManager_ServiceErrs(t *testing.T) {
	testCases := []struct {
		Name    string
		Service GenericErrService
		Stage   managerStages
	}{
		{
			Name: "Setup_ReturnErr",
			Service: GenericErrService{
				ErrOnSetup: true,
			},
			Stage: managerStagesSetup,
		},
		{
			Name: "Run_ReturnErr",
			Service: GenericErrService{
				ErrOnRun: true,
			},
			Stage: managerStagesRun,
		},
	}

	for _, thisCase := range testCases {
		t.Run(thisCase.Name, func(t *testing.T) {
			checkManagerErr(t, thisCase.Service, thisCase.Stage)
		})
	}
}
