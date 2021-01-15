package pkservices

import (
	"context"
	"google.golang.org/grpc"
	"sync"
)

// Service is an interface that a service must implement to be run by the Manager
type Service interface {
	// Setup is called before Run to set up any resources the service requires.
	//
	// resourcesCtx will be cancelled AFTER Run returns to signal that all the main
	// process has finished, and it is safe to release resources.
	//
	// resourcesReleased should be incremented and decremented by individual resources,
	// and the Manager will block on it until the context passed to Shutdown cancels.
	Setup(resourcesCtx context.Context, resourcesReleased *sync.WaitGroup) error
}

// Generic framework for registering non-grpc based services to the monitor, so multiple
// services can be run in parallel by the same monitor.
type GenericService interface {
	// Run is called to run the main service. Run should not exit until an irrecoverable
	// error occurs or StartGracefulShutdown is called.
	//
	// After Run returns, the resourcesCtx passed to Setup will be cancelled.
	Run(runCtx context.Context) error

	// StartGracefulShutdown is called when the manager gets a signal to exit.
	// StartGracefulShutdown should not block, but begin the graceful shutdown of the
	// main server.
	//
	// When shutdownCtx cancels, the manger will stop waiting for a graceful shutdown
	// and move towards a forced shutdown.
	//
	// StartGracefulShutdown does not return an error. Any errors that occur during
	// shutdown should be returned by Run.
	StartGracefulShutdown(shutdownCtx context.Context)
}

// GrpcService is a Service with methods to support running a gRPC service.
//
// grpc Server options are passed to the monitor, which is responsible for the lifetime
// and spin up, and shutdown of the server hosting and GrpcService values.
//
// The service just needs to expose a generic server registration method that calls
// the specific protoc generated registration method for that service.
type GrpcService interface {
	Service
	// RegisterOnServer is invoked before Service.Setup to allow the service to
	// register itself with the server.
	//
	// RegisterOnServer need only call the protoc generated registration function.
	RegisterOnServer(server *grpc.Server)
}
