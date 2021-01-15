package pkservices

import (
	"context"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pksync"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"sync"
	"testing"
)

const DefaultGrpcAddress = "0.0.0.0:50051"

// Service errors returns errors from multiple services as a single error.
type ServiceErrors struct {
	Errs []error
}

// Error implements builtins.errors, and reports the number of errors.
func (s ServiceErrors) Error() string {
	return fmt.Sprintf("%v errors occured", len(s.Errs))
}

// managerSync holds all the sync objects for Manager.
type managerSync struct {
	// masterCtx is the master context of the Manager.
	masterCtx context.Context
	// masterCancel cancels ctx and signals the manager to shutdown.
	masterCancel context.CancelFunc

	// resourcesCtx is the context passed to Service.Setup() to signal resources to
	// shutdown.
	resourcesCtx context.Context
	// resourcesCancel cancels resourcesCtx
	resourcesCancel context.CancelFunc
	// resourcesReleased should be added to when a resource spins up and closed when it
	// spins down.
	resourcesReleased *sync.WaitGroup

	// servicesCtx is ghe context passed to Service.Run() to signal services to being
	// shutdown.
	servicesCtx context.Context
	// servicesCancel cancels servicesCtx.
	servicesCancel context.CancelFunc

	// shutdownCtx is cancelled when the manager should stop waiting for services /
	// resources to exit.
	shutdownCtx context.Context
	// shutdownCancel cancels shutdownCtx.
	shutdownCancel context.CancelFunc

	// shutdownComplete complete is closed when the manager finishes shutting down.
	shutdownComplete chan struct{}
}

// Manager manages the lifetime of the service.
type Manager struct {
	// sync holds all the sync objects for Manager.
	sync managerSync

	// services is all the services the manager is tasked with running.
	services []Service
	// opts are the options to run the manager with.
	opts *ManagerOpts
}

// mapServices maps action across every service concurrently.
func (manager *Manager) mapServices(action func(service Service) error) error {
	// We're going to store the results of each start up in this array.
	errs := make([]error, len(manager.services))

	// Create a CtxWaitGroup to mark when setup is complete
	setupComplete := pksync.NewCtxWaitGroup(manager.sync.shutdownCtx)
	for i, service := range manager.services {
		setupComplete.Add(1)

		// Run the setup.
		go func(service Service, i int) {
			defer setupComplete.Done()
			// Store the result of this setup in the index passed to the routine.
			errs[i] = action(service)
		}(service, i)
	}

	// Wait for all the setup routines to complete.
	err := setupComplete.Wait()
	if err != nil {
		return fmt.Errorf(
			"shutdown timed out: %w", err,
		)
	}

	// Collect our results into a single error.
	err = manager.collectServiceErrors(errs)
	if err != nil {
		return err
	}

	// Otherwise, return.
	return nil

}

// setupServices starts all the services
func (manager *Manager) setupServices() error {
	return manager.mapServices(func(service Service) error {
		return service.Setup(
			manager.sync.resourcesCtx, manager.sync.resourcesReleased,
		)
	})
}

// runGrpcServices runs all gRPC services.
func (manager *Manager) runGrpcServices() error {
	// Get all of the gRPC services from our services list.
	var grpcServices []GrpcService

	for _, service := range manager.services {
		if grpcService, ok := service.(GrpcService); ok {
			grpcServices = append(grpcServices, grpcService)
		}
	}

	// If there are none, exit.
	if len(grpcServices) == 0 {
		return nil
	}

	// Allow them all to register themselves on the server.
	server := grpc.NewServer(manager.opts.grpcServerOpts...)
	for _, service := range grpcServices {
		service.RegisterOnServer(server)
	}

	// get a listener
	listener, err := net.Listen("tcp", manager.opts.grpcServiceAddress)
	if err != nil {
		return fmt.Errorf("error getting tcp listener: %w", err)
	}

	// Launch a monitor routine that shuts down the server on the service context
	// cancelling.
	go func() {
		// Close the listener on exit,
		defer listener.Close()
		// Gracefully stop the server on exit
		defer server.GracefulStop()

		// When the service context closes, run the deferred functions
		<-manager.sync.servicesCtx.Done()
	}()

	// Serve the gRPC services.
	err = server.Serve(listener)
	if err != nil {
		return fmt.Errorf("error serving gRPC: %w", err)
	}

	return nil
}

// genericServiceSignalShutdown calls GenericService.StartGracefulShutdown on all
// registered GenericService values.
func (manager *Manager) genericServiceSignalShutdown() {
	_ = manager.mapServices(func(service Service) error {
		serviceGeneric, ok := service.(GenericService)
		if !ok {
			return nil
		}
		serviceGeneric.StartGracefulShutdown(manager.sync.shutdownCtx)
		return nil
	})
}

// genericServicesRun runs all generic services
func (manager *Manager) genericServicesRun() error {
	go func() {
		defer manager.genericServiceSignalShutdown()
		<-manager.sync.servicesCtx.Done()
	}()

	return manager.mapServices(func(service Service) error {
		serviceGeneric, ok := service.(GenericService)
		if !ok {
			return nil
		}
		return serviceGeneric.Run(manager.sync.servicesCtx)
	})
}

// collectServiceErrors takes in an slice of error results from goroutine launches and
// collects any non-nil errors into a ServiceErrors.
//
// if results contains only nil values (no errors), a nil value is returned.
func (manager *Manager) collectServiceErrors(results []error) error {
	// We're going to store our errors in here.
	var errorList []error

	// Check the returns for non-nil errors.
	for _, err := range results {
		if err == nil {
			continue
		}

		// Extract service errors and add their internal error list to our list.
		if thisErr, ok := err.(ServiceErrors); ok {
			errorList = append(errorList, thisErr.Errs...)
			continue
		}

		// Otherwise add the error.
		errorList = append(errorList, err)
	}

	// If there were errors, report them in a service error
	if len(errorList) > 0 {
		return ServiceErrors{Errs: errorList}
	}

	return nil
}

// runAllServices runs all services.
func (manager *Manager) runAllServices() error {
	defer manager.StartShutdown()

	// We're going to store results of the service runs in this array.
	runErrors := make([]error, 2)
	runsComplete := pksync.NewCtxWaitGroup(manager.sync.shutdownCtx)

	// Run the Generic services in one routine.
	runsComplete.Add(1)

	go func() {
		defer runsComplete.Done()
		err := manager.genericServicesRun()
		if err != nil {
			runErrors[0] = fmt.Errorf("error running grpc: %w", err)
		}
	}()

	// Run our gRPC services in another.
	runsComplete.Add(1)
	go func() {
		defer runsComplete.Done()
		err := manager.runGrpcServices()
		if err != nil {
			runErrors[1] = fmt.Errorf("error running grpc: %w", err)
		}
	}()

	err := runsComplete.Wait()
	if err != nil {
		return fmt.Errorf("error waiting on service run return: %w", err)
	}

	// Collect our results.
	err = manager.collectServiceErrors(runErrors)
	if err != nil {
		return fmt.Errorf("error running services: %w", err)
	}

	return nil
}

// waitForShutdownComplete waits for the shutdown of the manager to be complete.
func (manager *Manager) waitForShutdownComplete() error {
	// Launch a goroutine to wait on the resource WaitGroup and close a channel when it
	// releases.
	resourcesReleased := make(chan struct{})
	go func() {
		defer close(resourcesReleased)
		manager.sync.resourcesReleased.Wait()
	}()

	// Wait for the service resources to release OR our shutdown to time out.
	select {
	case <-resourcesReleased:
	case <-manager.sync.shutdownCtx.Done():
		return fmt.Errorf(
			"shutdown timed out: %w", manager.sync.shutdownCtx.Err(),
		)
	}

	return nil
}

// listenForSignal listens for a os.Interrupt or os.Kill signal and beings graceful
// shutdown on receiving one.
func (manager *Manager) listenForSignal() {
	// Start shutdown on exit.
	defer manager.StartShutdown()

	// Listen for os.Interrupt and os.Kill.
	events := make(chan os.Signal, 1)
	signal.Notify(events, os.Interrupt)
	signal.Notify(events, os.Kill)

	// Wait for an interrupt to happen OR our master context to be cancelled.
	select {
	case <-events:
	case <-manager.sync.masterCtx.Done():
	}
}

// Reset resets the manager for a new run.
func (manager *Manager) Reset() {
	masterCtx, masterCancel := context.WithCancel(context.Background())
	resourcesCtx, resourcesCancel := context.WithCancel(context.Background())
	servicesCtx, servicesCancel := context.WithCancel(context.Background())
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	manager.sync = managerSync{
		masterCtx:         masterCtx,
		masterCancel:      masterCancel,
		resourcesCtx:      resourcesCtx,
		resourcesCancel:   resourcesCancel,
		resourcesReleased: new(sync.WaitGroup),
		servicesCtx:       servicesCtx,
		servicesCancel:    servicesCancel,
		shutdownCtx:       shutdownCtx,
		shutdownCancel:    shutdownCancel,
		shutdownComplete:  make(chan struct{}),
	}
}

// Run runs the manager and all it's services / resources. Run blocks until the manager
// has fully shut down.
func (manager *Manager) Run() error {
	// Release the resources and start shutdown on exit in case of panic.
	defer close(manager.sync.shutdownComplete)
	defer manager.StartShutdown()
	defer manager.sync.resourcesCancel()

	// Launch a routine to listen for an interrupt signal and exit.
	go manager.listenForSignal()

	// Setup the services.
	err := manager.setupServices()
	if err != nil {
		return fmt.Errorf("error setting up services: %w", err)
	}

	// Run the services.
	err = manager.runAllServices()
	if err != nil {
		return fmt.Errorf("error running services: %w", err)
	}

	// Release resources now that the services are done running.
	manager.sync.resourcesCancel()

	// Wait for the services to shut down.
	err = manager.waitForShutdownComplete()
	if err != nil {
		return fmt.Errorf("error shutting down: %w", err)
	}

	// Return
	return nil
}

// StartShutdown begins shutdown of the manager. Can be called multiple times. This
// methods returns immediately rather than blocking until the manager shuts down.
func (manager *Manager) StartShutdown() {
	manager.sync.masterCancel()
	manager.sync.servicesCancel()
}

// WaitForShutdown blocks until the manager is fully shutdown.
func (manager *Manager) WaitForShutdown() {
	<-manager.sync.shutdownComplete
}

// Test returns a test harness with helper methods for testing the manager.
func (manager *Manager) Test(t *testing.T) ManagerTesting {
	return ManagerTesting{t: t, manager: manager}
}

// NewManager creates a new Manager to run the given Service values with the passed
// ManagerOpts. If opts is nil, NewManagerOpts will be used to generate default options.
func NewManager(opts *ManagerOpts, services ...Service) *Manager {
	if opts == nil {
		opts = NewManagerOpts()
	}

	// If we are adding a ping service, append it to the services.
	if opts.addPingService {
		services = append(services, pingService{})
	}

	manager := &Manager{
		services: services,
		opts:     opts,
	}
	manager.Reset()

	return manager
}
