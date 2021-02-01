pkservices
==========

package pkservices offers a lightweight framework for managing the lifetime of multiple
services and their resource dependencies (db connector, etc).

Quickstart
----------

First we'll start by implementing a simple gRPC server. The service we are implementing
is pkservices.PingServer.

We also need to implement the pkservices.GrpcService interface for our server manager
to know what to do with our service.

.. code-block::

  // pingService is a basic implementation of PingServer that the manager can use to test
  // connectivity to the server.
  type PingService struct {
  }

  // Id implements Service and returns "gPEAKERC Ping".
  func (ping pingService) Id() string {
    return "gPEAKERC Ping"
  }

  // Setup implements Service.
  func (ping pingService) Setup(
    resourcesCtx context.Context,
    resourcesReleased *sync.WaitGroup,
    shutdownCtx context.Context
    logger zerolog.Logger,
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

The ``Setup()`` method allows us to spin up any resources our service needs to run. The
``resourcesCtx`` passed to the method will not be released until after our gRPC server
has gracefully shutdown, and the manager we are about to register our service with will
not fully exit until ``resourcesReleased`` is fully closed.

Let's run out service in a ``Manager``:

.. code-block::

  func main() {
    // Our manager options.
    managerOpts := pkservices.NewManagerOpts().
      // We are adding our own implementation of Ping, so we don't need to register
      // The default one.
      WithGrpcPingService(false)

    // Create a new manager to manage our service.
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
    err = <- errChan
    if err != nil {
      panic(err)
    }

    // Exit.

    // Output:
    // PING RECEIVED!
  }

This top-level logic is all you need to run your services!

Service Interfaces
------------------

pkservices defines three interfaces for declaring services: ``Service``, ``GrpcService``
and ``GenericService``.

These interfaces are designed for the quick declaration of services, which can then
be handed off to the ``pkservices.Manager`` type for running. The end result is
having to write very little code for the boilerplate of managing the lifetime of the
service.

.. note::

  ``Service`` acts as a base for more specific service types. A service must implement
  one of the more specific types (like ``ServiceGrpc``), and not just ``Service`` for
  the manager to run it.

The base ``Service`` interface looks like this:

.. code-block::

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
    // logger is a zerolog.Logger with a
    // zerolog.Logger.WithString("SERVICE", [Service.Id()]) entry already on it.
    Setup(
      resourcesCtx context.Context,
      resourcesReleased *sync.WaitGroup,
      logger zerolog.Logger,
    ) error
  }

Testing Methods
---------------

The ``Manager`` type exposes a number of useful tasting methods, which can be accessed
through ``Manager.Test()``. See the API docs for more details.
