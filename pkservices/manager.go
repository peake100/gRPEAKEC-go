package pkservices

import (
	"context"
	"errors"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pksync"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"sync"
	"testing"
	"time"
)

// DefaultGrpcAddress is the default address the gRPC server will listen on.
const DefaultGrpcAddress = "0.0.0.0:50051"

// managerSync holds all the sync objects for Manager.
type managerSync struct {
	// resourcesCtx is the context passed to Service.Setup() to signal resources to
	// shutdown.
	resourcesCtx context.Context
	// resourcesCancel cancels resourcesCtx
	resourcesCancel context.CancelFunc
	// resourcesReleased should be added to when a resource spins up and closed when it
	// spins down.
	resourcesReleased *sync.WaitGroup

	// servicesCtx is the context passed to Service.Run() to signal services to being
	// shutdown.
	servicesCtx context.Context
	// servicesCancel cancels servicesCtx.
	servicesCancel context.CancelFunc

	// listenersCtx is the context used for event listener functions like monitoring
	// for system signals or shutdown context timeout. This context is cancelled after
	// all services and resources have been release, but before shutdownComplete is
	// called
	listenersCtx context.Context
	// listenersCancel cancels listenersCtx.
	listenersCancel context.CancelFunc
	// listenersExited is a WaitGroup for internal listeners.
	listenersExited *sync.WaitGroup

	// shutdownCtx is cancelled when the manager should stop waiting for services /
	// resources to exit.
	shutdownCtx context.Context
	// shutdownCancel cancels shutdownCtx.
	shutdownCancel context.CancelFunc

	// shutdownComplete complete is closed when the manager finishes shutting down.
	// This signal is also available to the end caller through
	// Manager.WaitForShutdown().
	shutdownComplete chan struct{}
}

// serviceInfo holds each service and any additional relevant data
type serviceInfo struct {
	// Service is the service itself.
	Service Service

	// Logger is the logger for the service.
	Logger zerolog.Logger
}

// Manager manages the lifetime of the service.
type Manager struct {
	// sync holds all the sync objects for Manager.
	sync managerSync

	// osSignals holds the channel we are accepting os signals on. We store this here
	// so we can expose it for testing.
	osSignals chan os.Signal

	// services is all the services the manager is tasked with running.
	services []serviceInfo
	// opts are the options to run the manager with.
	opts *ManagerOpts
}

// collectServicesErrors takes in an slice of error results from goroutine launches and
// collects any non-nil errors into a ServicesErrors.
//
// if results contains only nil values (no errors), a nil value is returned.
func (manager *Manager) collectServicesErrors(results []error) error {
	// We're going to store our errors in here.
	var errorList []error

	// Check the returns for non-nil errors.
	for _, err := range results {
		if err == nil {
			continue
		}

		// Extract service errors and add their internal error list to our list.
		if thisErr, ok := err.(ServicesErrors); ok {
			errorList = append(errorList, thisErr.Errs...)
			continue
		}

		// Otherwise add the error.
		errorList = append(errorList, err)
	}

	// If there were errors, report them in a service error
	if len(errorList) > 0 {
		return ServicesErrors{Errs: errorList}
	}

	return nil
}

// mapServices maps action across every service concurrently, and returns a
// ServicesError.
//
// If any action() invocation returns an error, Manager.StartShutdown is called
// immediately.
func (manager *Manager) mapServices(
	action func(service serviceInfo) error,
) error {
	// We're going to errors in this array.
	errs := make([]error, 0)

	var mapFunc pksync.ConcurrentMapFunc = func(
		ctx context.Context, value interface{}, index int,
	) error {
		info := value.(serviceInfo)
		err := pkerr.CatchPanic(func() error {
			return action(info)
		})
		if err != nil {
			return ServiceError{
				ServiceId: info.Service.Id(),
				Err:       err,
			}
		}
		return nil
	}

	// Range over our error channel, collecting errors.
	servicesCtx := manager.sync.servicesCtx
	for err := range pksync.ConcurrentMap(servicesCtx, manager.services, mapFunc) {
		// Add the underlying error to our errors list (will always be a service error)
		if mapErr, ok := err.(pksync.ConcurrentMapError); ok {
			errs = append(errs, mapErr.MapFuncErr)

			// If there is an error, start shutdown of the manager.
			manager.StartShutdown()
		}
	}

	// Collect errors.
	if len(errs) > 0 {
		return manager.collectServicesErrors(errs)
	}

	// Otherwise, return.
	return nil
}

// setupServices starts all the services
func (manager *Manager) setupServices() error {
	return manager.mapServices(func(info serviceInfo) error {
		info.Logger.Info().Msg("running setup")
		err := info.Service.Setup(
			manager.sync.resourcesCtx, manager.sync.resourcesReleased, info.Logger,
		)
		info.Logger.Info().Msg("setup complete")

		return err
	})
}

// runGrpcServices runs all gRPC services.
func (manager *Manager) runGrpcServices() error {
	defer manager.StartShutdown()

	// Iterate over the services and see if there are any grpc ones
	var hasGrpc bool
	// We know this will not result in an error.
	_ = manager.mapServices(func(info serviceInfo) error {
		_, ok := info.Service.(GrpcService)
		if ok {
			hasGrpc = true
		}
		return nil
	})

	// If there are no gRPC services, return.
	if !hasGrpc {
		return nil
	}

	// Allow them all to register themselves on the server. We are going to run this
	// in the mapServices function so that if a Registration method panics, we will
	// get the error.
	server := grpc.NewServer(manager.opts.grpcServerOpts...)
	err := manager.mapServices(func(info serviceInfo) error {
		grpcService, ok := info.Service.(GrpcService)
		if !ok {
			return nil
		}
		grpcService.RegisterOnServer(server)
		return nil
	})

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
	manager.opts.logger.Info().
		Str("SERVER_ADDRESS", manager.opts.grpcServiceAddress).
		Msg("serving gRPC")

	err = server.Serve(listener)
	if err != nil {
		return fmt.Errorf("error serving gRPC: %w", err)
	}

	return nil
}

// genericServicesRun runs all generic services
func (manager *Manager) genericServicesRun() error {
	return manager.mapServices(func(info serviceInfo) error {
		serviceGeneric, ok := info.Service.(GenericService)
		if !ok {
			return nil
		}
		info.Logger.Info().Msg("running")
		err := serviceGeneric.Run(manager.sync.servicesCtx, manager.sync.shutdownCtx)
		info.Logger.Info().Msg("run complete")
		return err
	})
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
		runErrors[0] = manager.genericServicesRun()
	}()

	// Run our gRPC services in another.
	runsComplete.Add(1)
	go func() {
		defer runsComplete.Done()

		runErrors[1] = pkerr.CatchPanic(func() error {
			return manager.runGrpcServices()
		})
	}()

	err := runsComplete.Wait()
	if err != nil {
		return fmt.Errorf("error waiting on service run return: %w", err)
	}

	// Collect our results.
	err = manager.collectServicesErrors(runErrors)
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

	// Cancel the listener routines.
	manager.sync.listenersCancel()
	// Wait for listeners to exit.
	manager.sync.listenersExited.Wait()

	// Return, we are done shutting down.
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
	case osSignal := <-events:
		manager.opts.logger.Error().
			Str("SIGNAL", osSignal.String()).
			Msg("signal received from host")
	case <-manager.sync.servicesCtx.Done():
	case <-manager.sync.listenersCtx.Done():
	}
}

// timeoutShutdown handles listening for the main shutdown request, and signaling a
// force-shutdown if the configured maximum shutdown duration expires.
func (manager *Manager) timeoutShutdown() {
	// If the shutdown time is under 0, we are going to allow an unlimited time to
	// shutdown, and should exit this routine immediately.
	if manager.opts.maxShutdownDuration < 0 {
		return
	}

	// Cancel the shutdown on the way out
	defer manager.sync.shutdownCancel()

	// Wait for a shutdown to be signaled.
	<-manager.sync.servicesCtx.Done()

	deadline := time.NewTimer(manager.opts.maxShutdownDuration)
	defer deadline.Stop()

	// Wait for either shutdown complete to be reported OR the shutdown timer to
	// time out.
	select {
	case <-manager.sync.listenersCtx.Done():
	case <-deadline.C:
		manager.opts.logger.Error().Msg("shutdown exceeded max timeout")
	}
}

// launchListeners launches listener routines that manage the lifecycle of the manager.
func (manager *Manager) launchListeners() {
	// Launch a routine that listens for shutdown to start and signals a force-shutdown
	// if the maximum shutdown duration is exceeded.
	manager.sync.listenersExited.Add(1)
	go func() {
		defer manager.sync.listenersExited.Done()
		manager.timeoutShutdown()
	}()

	// Launch a routine to listen for an interrupt signal and exit.
	manager.sync.listenersExited.Add(1)
	go func() {
		defer manager.sync.listenersExited.Done()
		manager.listenForSignal()
	}()
}

// Reset resets the manager for a new run.
func (manager *Manager) Reset() {
	resourcesCtx, resourcesCancel := context.WithCancel(context.Background())
	servicesCtx, servicesCancel := context.WithCancel(context.Background())
	listenersCtx, listenersCancel := context.WithCancel(context.Background())
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	manager.sync = managerSync{
		resourcesCtx:      resourcesCtx,
		resourcesCancel:   resourcesCancel,
		resourcesReleased: new(sync.WaitGroup),
		servicesCtx:       servicesCtx,
		servicesCancel:    servicesCancel,
		listenersCtx:      listenersCtx,
		listenersCancel:   listenersCancel,
		listenersExited:   new(sync.WaitGroup),
		shutdownCtx:       shutdownCtx,
		shutdownCancel:    shutdownCancel,
		shutdownComplete:  make(chan struct{}),
	}
}

// logServicesErrors logs a ServicesErrors for a given stage using the passed
// stageLogger.
func (manager *Manager) logServicesErrors(
	errs ServicesErrors, stageLogger zerolog.Logger,
) {
	var serviceErr ServiceError
	for _, err := range errs.Errs {
		serviceName := "unknown"
		if errors.As(err, &serviceErr) {
			serviceName = serviceErr.ServiceId
			err = serviceErr.Err
		}
		stageLogger.Err(err).Str("SERVICE", serviceName)
	}
}

// logErrors runs any errors returned from a ManagerError.
func (manager *Manager) logErrors(err ManagerError) {
	stages := []struct {
		Name string
		Err  error
	}{
		{
			Name: "setup",
			Err:  err.SetupErr,
		},
		{
			Name: "run",
			Err:  err.RunErr,
		},
		{
			Name: "shutdown",
			Err:  err.ShutdownErr,
		},
	}

	var servicesErrors ServicesErrors
	for _, stage := range stages {
		if stage.Err == nil {
			continue
		}

		stageLogger := manager.opts.logger.With().Str("STAGE", stage.Name).Logger()

		if errors.As(stage.Err, &servicesErrors) {
			manager.logServicesErrors(servicesErrors, stageLogger)
		} else {
			stageLogger.Err(stage.Err)
		}
	}
}

// Run runs the manager and all it's services / resources. Run blocks until the manager
// has fully shut down.
func (manager *Manager) Run() error {
	manager.opts.logger.Info().
		Dur("MAX_SHUTDOWN", manager.opts.maxShutdownDuration).
		Bool("WITH_PING", manager.opts.addPingService).
		Msg("running service manager")
	// We're going to store the different stage errors here.
	managerErr := ManagerError{}

	// Release all resources and start shutdown on exit in case of panic or unexpected
	// error.
	defer close(manager.sync.shutdownComplete)
	defer manager.StartShutdown()

	// Launch event listeners (such as interrupt signals and shutdown timeout)
	manager.launchListeners()

	// Setup the services. Do not advance to the run stage if there is a setup error.
	managerErr.SetupErr = manager.setupServices()
	if managerErr.SetupErr == nil {
		// If there was not error, run the services.
		managerErr.RunErr = manager.runAllServices()
	}

	// Release resources now that the services are done running.
	manager.sync.resourcesCancel()

	// Wait for the services to shut down.
	managerErr.ShutdownErr = manager.waitForShutdownComplete()

	// If our manager error has errors to report, return it.
	if managerErr.hasErrors() {
		manager.logErrors(managerErr)
		return managerErr
	}

	// Otherwise, exit.
	return nil
}

// StartShutdown begins shutdown of the manager. Can be called multiple times. This
// methods returns immediately rather than blocking until the manager shuts down.
func (manager *Manager) StartShutdown() {
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

	serviceInfos := make([]serviceInfo, len(services))
	for i, thisService := range services {
		serviceLogger := opts.logger.With().
			Str("SERVICE", thisService.Id()).
			Logger()

		thisInfo := serviceInfo{
			Service: thisService,
			Logger:  serviceLogger,
		}

		serviceInfos[i] = thisInfo
	}

	manager := &Manager{
		services: serviceInfos,
		opts:     opts,
	}
	manager.Reset()

	return manager
}
