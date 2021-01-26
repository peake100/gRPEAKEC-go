package pkservices

import (
	"context"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"sync"
)

// Service is an interface that a service must implement to be run by the Manager
type Service interface {
	// Id should return a unique, but human readable id for the service. This id is
	// used for both logging and ServiceError context. If two services share the same
	// id, the manger will return an error.
	Id() string

	// Setup is called before Run to set up any resources the service requires.
	//
	// resourcesCtx will be cancelled AFTER Run returns to signal that all the main
	// process has finished, and it is safe to release resources.
	//
	// resourcesReleased should be incremented and decremented by individual resources,
	// and the Manager will block on it until the context passed to Shutdown cancels.
	//
	// shutdownCtx is cancelled when the graceful shutdown limit has been reached and
	// resources should be released immediately for a forced shutdown.
	//
	// logger is a zerolog.Logger with a
	// zerolog.Logger.WithString("SERVICE", [Service.Id()]) entry already on it.
	Setup(
		resourcesCtx context.Context,
		resourcesReleased *sync.WaitGroup,
		shutdownCtx context.Context,
		logger zerolog.Logger,
	) error
}

// Generic framework for registering non-grpc based services to the monitor, so multiple
// services can be run in parallel by the same monitor.
type GenericService interface {
	Service
	// Run is called to run the main service. Run should not exit until an irrecoverable
	// error occurs or runCtx is cancelled.
	//
	// runCtx cancelling indicates that the service should begin to gracefully shutdown.
	//
	// When shutdownCtx is cancelled, the manager will stop waiting for run to return
	// and continue with the shutdown process.
	//
	// After Run returns, the resourcesCtx passed to Setup will be cancelled.
	Run(runCtx context.Context, shutdownCtx context.Context) error
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
