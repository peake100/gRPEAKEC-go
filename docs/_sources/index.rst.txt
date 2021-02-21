gRPEAKEC-go
===========

gRPEAKEC is a collection of service utilities for handling all the boilerplate of
running gRPC and other services. This documentation will be broken into one section for
each package of the module.

.. toctree::
    :maxdepth: 2
    :caption: Contents:

    ./pkservices.rst
    ./pkerr.rst

QuickStart
==========

An example of running a service manager with error handling interceptors:

.. code-block::

    // Get our hostname
    host, _ := os.Hostname()

    // Set up our logger
    zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
    logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
        Level(zerolog.DebugLevel).
        With().
        Timestamp().
        Str("HOST", host).
        Logger()

    // errGen will be used to make rich errors for our service
    errGen := pkerr.NewErrGenerator(
        "SortingHat", // appName
        host,         // hostName
        true,         // addStackTrace
        true,         // sendContext
        true,         // sendSource
    )

    // Create our manager options
    opts := pkservices.NewManagerOpts().
        // Add our logger.
        WithLogger(logger).
        // Set up logging middleware. These setting will log each rpc at an Info level
        // and if the logger level is set to debug or less, add in the req and resp
        // objects as fields.
        WithGrpcLogging(
            zerolog.InfoLevel, // logRPCLevel
            zerolog.DebugLevel, // logReqLevel
            zerolog.DebugLevel, // logRespLevel
            true, // logErrors
            true, // errorTrace
        ).
        // Pass our error generator to create error middleware.
        WithErrorGenerator(errGen).
        // Set our gRPC server address.
        WithGrpcServerAddress(":50051")

    // Create our service
    sortingHat := &SortingHat{
        errGen: errGen,
    }

    // Create a new service manager.
    manager := pkservices.NewManager(opts, sortingHat)
    defer manager.StartShutdown()

    // Run our manager in a routine. This is all we have to do, our service and resource
    // lifetime management is handled for us. The manager will also listen for os
    // signals and trigger a shutdown if they occur.
    managerResult := make(chan error)
    go func() {
        defer close(managerResult)
        managerResult <- manager.Run()
    }()

    // Only one unary and one stream interceptor can normally be registered on a client
    // and server. pkmiddleware offers interceptors that take in an unlimited amount
    // of middleware, enabling a higher degree of customization.
    unaryInterceptor := pkmiddleware.NewUnaryClientMiddlewareInterceptor(
        errorGen.UnaryClientMiddleware,
    )
    streamInterceptor := pkmiddleware.NewStreamClientMiddlewareInterceptor(
        errorGen.StreamClientMiddleware,
    )

    // Create a new gRPC client connection.
    clientConn, err := grpc.Dial(
        ":50051",
        grpc.WithInsecure(),
        grpc.WithUnaryInterceptor(unaryInterceptor),
        grpc.WithStreamInterceptor(streamInterceptor),
    )
    if err != nil {
        panic(err)
    }

    // We can wait until the gRPC server is giving good responses.
    err = pkclients.WaitForGrpcServer(context.Background(), clientConn)
    if err != nil {
        panic(err)
    }

    // Make our service client.
    hatClient := protogen.NewSortingHatClient(clientConn)

    pupils := []*protogen.Pupil{
        {
            Name: "Harry Potter",
        },
        {
            Name: "Draco Malfoy",
        },
    }

    // Sort our pupils.
    for _, thisPupil := range pupils {
        // Call the service.
        sorted, err := hatClient.Sort(context.Background(), thisPupil)

        // log this error if it is a ErrRosterUpdate. We can use native error handling
        // here thanks to our interceptors.
        if errors.Is(err, ErrRosterUpdate) {
            // log the error
            logger.Err(err).Msg("client got error")
            continue
        }

        // Otherwise log a success
        logger.Info().
            Str("PUPIL", thisPupil.Name).
            Stringer("HOUSE",sorted.House).
            Msg("client sorted into house")
    }

    // Start shutdown of the manager and wait for it to finish.
    manager.StartShutdown()

    // Wait for our manager run result.
    if err := <-managerResult; err != nil {
        panic(err)
    }

Outputs:

.. code-block:: text

    1:59AM INF running service manager HOST=Williams-MacBook-Pro-2.local SETTING_ADD_PING_SERVICE=true SETTING_MAX_SHUTDOWN=30000
    1:59AM INF running setup HOST=Williams-MacBook-Pro-2.local SERVICE="gPEAKERC Ping"
    1:59AM INF setup complete HOST=Williams-MacBook-Pro-2.local SERVICE="gPEAKERC Ping"
    1:59AM INF running setup HOST=Williams-MacBook-Pro-2.local SERVICE=SortingHat
    1:59AM INF setup complete HOST=Williams-MacBook-Pro-2.local SERVICE=SortingHat
    1:59AM INF serving gRPC HOST=Williams-MacBook-Pro-2.local SERVER_ADDRESS=:50051
    1:59AM INF rpc completed DURATION=0.031 GRPC_METHOD=/sortinghat.SortingHat/Sort HOST=Williams-MacBook-Pro-2.local METHOD_KIND=unary REQ={"name":"Harry Potter"} RESP={} RPC_ID=5577006791947779410
    1:59AM INF client sorted into house HOST=Williams-MacBook-Pro-2.local HOUSE=Gryffindor PUPIL="Harry Potter"
    1:59AM ERR  error="rpc error: code = Internal desc = (RosterUpdate | Hogwarts | 2000) roster could not be updated with student: sorted student could not be stored | from: roster is full" DURATION=0.26 GRPC_METHOD=/sortinghat.SortingHat/Sort HOST=Williams-MacBook-Pro-2.local METHOD_KIND=unary REQ={"name":"Draco Malfoy"} RESP=null RPC_ID=8674665223082153551
    1:59AM ERR client got error error="(RosterUpdate | Hogwarts | 2000) roster could not be updated with student: sorted student could not be stored | from: grpc error 'Internal'" HOST=Williams-MacBook-Pro-2.local
    1:59AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
    1:59AM INF gRPC server shutdown HOST=Williams-MacBook-Pro-2.local
    1:59AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
    1:59AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
    1:59AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
    1:59AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local

API Docs
========

Full Golang API docs can be found on
`pkg.go.dev <https://pkg.go.dev/github.com/peake100/gRPEAKEC-go>`_

Protobuf API
============

This module contains some useful protobuf types and gRPC service definitions. API Docs
for these types and services can be `found here <_static/proto.html>`_
