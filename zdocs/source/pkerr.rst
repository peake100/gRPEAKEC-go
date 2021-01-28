pkerrs
======

The ``pkerrs`` package offers a system for defining, returning, and handling rich
errors between gRPC services and clients.

Defining Sentinel Errors
------------------------

Sentinel errors in Go are errors like ``io.EOF``: predefined errors that can be wrapped
in additional context. Once wrapped, ``errors.Is()`` can be used to see if an error is
a wrapped sentinel error, like so:

.. code-block:: go

    // %w wraps an error
    someErr := fmt.Errorf("could not fetch additional data: %w", io.EOF)

    fmt.Println("OUR ERROR:", someErr)

    // Even though our error is wrapped, we can check if it contains io.EOF anywhere in
    // it's chain.
    if errors.Is(someErr, io.EOF) {
        fmt.Println("error is io.EOF!")
    }

    // Output
    // could not fetch additional data: EOF
    // error is io.EOF!

pkerr defines the ``SentinelError`` type for creating custom sentinel errors geared
towards server-style errors:

.. code-block:: go

    fmt.Println("NAME     :", pkerr.ErrUnknown.Name)
    fmt.Println("CODE     :", pkerr.ErrUnknown.Code)
    fmt.Println("ISSUER   :", pkerr.ErrUnknown.Issuer)
    fmt.Println("gRPC CODE:", pkerr.ErrUnknown.GrpcCode)
    fmt.Println("MESSAGE  :", pkerr.ErrUnknown.DefaultMessage)

    // Output:
    // NAME     : Unknown
    // CODE     : 1000
    // ISSUER   : GRPEAKEC
    // gRPC CODE: Unknown
    // MESSAGE  : an unknown error occurred

An error class is defined by it's ISSUER and CODE, rather than just it's code alone.
This allows two identical codes from different backends / services to be considered
distinct.

We can make new sentinel errors using a ``SentinelIssuer``:

.. code-block:: go

    import (
        "fmt"
        "github.com/peake100/gRPEAKEC-go/pkerr"
        "google.golang.org/grpc/codes"
    )

    // sentinels is a Sentinel Issuer for the Hogwarts backend
    var sentinels = pkerr.NewSentinelIssuer(
        "Hogwarts",
        true,
    )

    // ErrSoulCloudy is returned when SortingHat cannot peer deep enough into a pupil's soul
    // to sort them.
    var ErrSoulCloudy = sentinels.NewSentinel(
        "SoulCloudy",
        2000,
        codes.FailedPrecondition,
        "The pupil's soul is to opaque to be sorted",
    )

    func main() {
        fmt.Println("NAME     :", ErrSoulCloudy.Name)
        fmt.Println("CODE     :", ErrSoulCloudy.Code)
        fmt.Println("ISSUER   :", ErrSoulCloudy.Issuer)
        fmt.Println("gRPC CODE:", ErrSoulCloudy.GrpcCode)
        fmt.Println("MESSAGE  :", ErrSoulCloudy.DefaultMessage)

        // Output:
        // NAME     : SoulCloudy
        // CODE     : 2000
        // ISSUER   : Hogwarts
        // gRPC CODE: FailedPrecondition
        // MESSAGE  : The pupil's soul is to opaque to be sorted
    }

Two SentinelError values are considered the same if and only if both the issuer and
the error code are the same.

The name of a sentinel error is for human consumption only, and does not figure into
sentinel equality. We can wrap and check sentinel errors with errors.Is():

.. code-block:: go

    err := fmt.Errorf("could not sort student Harry Potter: %w", ErrSoulCloudy)

    fmt.Println("ERROR:", err)

    // This is an ErrSoulCloudy
    if errors.Is(err, ErrSoulCloudy) {
        fmt.Println("This error is a ErrSoulCloudy error")
    }

    // Even though they are the same type, it is not an ErrUnknown:
    if !errors.Is(err, pkerr.ErrUnknown) {
        fmt.Println("This is not an unknown error")
    }

    // Output:
    // ERROR: could not sort student Harry Potter: Hogwarts SoulCloudy (2000): The
    //    pupil's soul is to opaque to be sorted
    // This error is a ErrSoulCloudy error
    // This is not an unknown error

Generating Specific Errors
--------------------------

The core type of pkerrs is the ``ErrorGenerator`` type. We will use the generator to
create new errors that can be used to send rich error details between servers and
clients.

We can set up a new generator as follows:

.. code-block:: go

    var host, _ = os.Hostname()

    var errorGen = pkerr.NewErrGenerator(
        "SortingHat", // appName
        host, // appHost
        true, // addStackTrace
        true, // sendContext
        true, // sendSource
    )

``appName`` and ``appHost`` are added to the trace frame of any errors created or
processed by this generator and interceptors created from it.

``addStackTrace`` adds the return of ``debug.StackTrace()`` to the error if true.

``sendContext`` will include any additional context the error is wrapped in as it gets
passed up the chain to the error data returned by a server interceptor.

``sendSource`` will include the Error() string of a source error this error is being
created from in the error data returned by a server interceptor.

Let's make an error with it:

.. code-block:: go

    // Returns an error
    err := errorGen.NewErr(
        ErrSoulCloudy, // Sentinel
        "could not sort student Harry Potter", // message
        []proto.Message{
            wrapperspb.String("Harry Potter"), // details
        },
        io.EOF,
    )

    fmt.Println("ERROR:", err)

    // This error is an APIError
    apiErr := err.(pkerr.APIError)
    fmt.Println("SENTINEL:", apiErr.Sentinel)
    fmt.Println("SOURCE:", apiErr.Source)

    // The error contains a protobuf message that describes it.
    fmt.Println("PROTO ERR:", apiErr.Proto)

    protoErr := apiErr.Proto
    fmt.Println("DETAILS:", protoErr.Details)

    // Output:
    // ERROR: (SoulCloudy | Hogwarts | 2000) The pupil's soul is to opaque to be sorted: could not sort student Harry Potter | from: EOF
    // SENTINEL: Hogwarts SoulCloudy (2000): The pupil's soul is to opaque to be sorted
    // SOURCE: EOF
    // PROTO ERR: (SoulCloudy | Hogwarts | 2000) The pupil's soul is to opaque to be sorted: could not sort student Harry Potter
    // DETAILS: [[type.googleapis.com/google.protobuf.StringValue]:{value:"Harry Potter"}]

The ``*Error`` protobuf type is defined as follows:

.. code-block:: go

    syntax = "proto3";
    package pkerr;
    option go_package = "github.com/peake100/gRPEAKEC-go/pkerr/protogen";

    import "cereal_proto/uuid.proto";
    import "google/protobuf/any.proto";
    import "google/protobuf/timestamp.proto";

    // Error holds information about an error.
    message Error {
      // id is a uuid that uniquely identifies this error.
      cereal.UUID id = 1;

      // issuer is the issuer of a code. If multiple services use this library, they
      // can differentiate their error codes by having unique issuers. If a number of
      // services working together in the same backend coordinate to ensure their error
      // definitions do not overlap, they can share an Issuer value.
      string issuer = 2;

      // code is the API Error code.
      uint32 code = 3;

      // grpc_code is the grpc status code associated with this error.
      int32 grpc_code = 4;

      // name is the human-readable API Error name tied to code.
      string name = 5;

      // message is the error message.
      string message  = 6;

      // source_err is a string representation of the original native error that lead to
      // this gRPEAKEC Error. Can be blank.
      string source_error = 7;

      // source_type is the type of source_err. Can be blank.
      string source_type = 8;

      // time is the time that the error occurred
      google.protobuf.Timestamp time = 9;

      // details are arbitrary information about the error.
      repeated google.protobuf.Any details = 10;

      // Trace holds a stack of TraceInfo objects. Each time an app encounters an error,
      // it can add it's TraceInfo object to the end of the trace.
      repeated TraceInfo trace = 11;
    }

    // Trace info holds information about where an error occurred.
    message TraceInfo {
      // app name is the name of the service or process that generated the error.
      string app_name = 1;

      // Identifier for the host the app is running on.
      string app_host = 2;

      // stack_trace of the error (can be ""). No enforced format. This is meant for human
      // reference).
      string stack_trace = 3;

      // Some languages let an error continue to gather context, with wrapping errors in go.
      // This field can be used to store additional context added to the error after it
      // was created.
      string additional_context = 4;
    }

This message offers a ton of rich context for errors. But how do we transmit this error
between client and server?

Error Interceptors
------------------

Lets start by implementing a simple gRPC service.

.. code-block:: go

    // Sorting hat implements protogen.SortingHat.
    type SortingHat struct {}

    // Sort sorts pupils into a house.
    func (s SortingHat) Sort(
        ctx context.Context, pupil *protogen.Pupil,
    ) (*protogen.Sorted, error) {
        if pupil.Name == "Harry Potter" {
            return nil, errorGen.NewErr(
                ErrSoulCloudy, // Sentinel
                "could not sort student Harry Potter", // message
                []proto.Message{pupil}, // details
                nil, // source
            )
        }

        return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
    }

If the pupil is "Harry Potter" the rpc method will return an APIError made from our
error generator.

Let's see how this error would normally be returned to the client.

.. code-block:: go

    // Set up the gRPC server
    listener, err := net.Listen("tcp", ":50051")
    if err != nil {
        panic(err)
    }
    defer listener.Close()

    server := grpc.NewServer()

    // Register our service
    protogen.RegisterSortingHatServer(server, SortingHat{})

    // Serve gRPC
    serveErr := make(chan error)
    go func() {
        defer close(serveErr)
        serveErr <- server.Serve(listener)
    }()

    clientConn, err := grpc.Dial(":50051", grpc.WithInsecure())
    if err != nil {
        panic(err)
    }
    defer clientConn.Close()

    hatClient := protogen.NewSortingHatClient(clientConn)

    // Try and sort the client
    _, err = hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Print the error
    fmt.Println("ERROR:", err)
    fmt.Println("ERROR TYPE:", reflect.TypeOf(err))

    // Get a status from the error
    respStatus, ok := status.FromError(err)
    if !ok {
        panic("error was not status")
    }

    // Code
    fmt.Println("STATUS CODE:", respStatus.Code())

    // Output:
    // ERROR: rpc error: code = Unknown desc = (SoulCloudy | Hogwarts | 2000) The pupil's soul is to opaque to be sorted: could not sort student Harry Potter | from: EOF
    // ERROR TYPE: *status.statusError
    // STATUS CODE: Unknown

This default method leaves a lot to be desired. We have to do a lot of error examination
after each call to even get at the status, and it returning a status error is likewise
cumbersome, with a lot of boilerplate required to set the right code, add any details,
extract those details on the other side, etc.

Luckily, our ErrorGenerator can create interceptors for both servers and clients that
will handle packing and unpacking our error into the details fields, as well as setting
the status message and code:

.. code-block:: go

    // Set up the gRPC server
    listener, err := net.Listen("tcp", address)
    if err != nil {
        panic(err)
    }

    // We can use the pkmiddleware package to create our error middleware.
    unaryServerInterceptor := pkmiddleware.NewUnaryServerMiddlewareInterceptor(
        errorGen.UnaryServerMiddleware,
    )

    streamServerInterceptor := pkmiddleware.NewStreamServerMiddlewareInterceptor(
        errorGen.StreamServerMiddleware,
    )

    server := grpc.NewServer(
        grpc.UnaryInterceptor(unaryServerInterceptor),
        grpc.StreamInterceptor(streamServerInterceptor),
    )

    // Register our service
    protogen.RegisterSortingHatServer(server, SortingHat2{})

    // Serve gRPC
    serveErr := make(chan error)
    go func() {
        defer close(serveErr)
        defer listener.Close()
        serveErr <- server.Serve(listener)
    }()

    complete.Add(1)
    go func() {
        defer complete.Done()
        defer server.GracefulStop()
        <-ctx.Done()
    }()

    // We are going to make a new errorGen for our client with the client app name.
    // All other errors will be the same.
    clientErrs := errorGen.WithAppName("ClientApp")

    // We can use the pkmiddleware package to create our error middleware.
    unaryClientInterceptor := pkmiddleware.NewUnaryClientMiddlewareInterceptor(
        clientErrs.UnaryClientMiddleware,
    )

    streamClientInterceptor := pkmiddleware.NewStreamClientMiddlewareInterceptor(
        clientErrs.StreamClientMiddleware,
    )

    // Use the client error generator to make client interceptors.
    clientConn, err := grpc.Dial(
        address,
        grpc.WithInsecure(),
        grpc.WithUnaryInterceptor(unaryClientInterceptor),
        grpc.WithStreamInterceptor(streamClientInterceptor),
    )
    if err != nil {
        panic(err)
    }
    defer clientConn.Close()

    complete.Add(1)
    go func() {
        defer complete.Done()
        defer clientConn.Close()
        <-ctx.Done()
    }()

    hatClient := protogen.NewSortingHatClient(clientConn)

Now lets see how that effects our error returns:

.. code-block:: go

    // Try and sort the client
    _, err = hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Print the error
    fmt.Println("ERROR:", err)
    fmt.Println("ERROR TYPE:", reflect.TypeOf(err))

    // Output:
    // ERROR: (SoulCloudy | Hogwarts | 2000) The pupil's soul is to opaque to be sorted: could not sort student Harry Potter | from: grpc error 'FailedPrecondition'
    // ERROR TYPE: pkerr.APIError

That's way more useful, we are now getting native errors!

Error Checking
--------------

Native errors mean we can use errors.Is() and errors.As() directly.

.. code-block:: go

    // Extract the error
    var apiErr pkerr.APIError
    if errors.As(err, &apiErr) && errors.Is(err, ErrSoulCloudy) {
        fmt.Println("API ERROR FOUND")
    }

    // Output:
    // API ERROR FOUND

Notice that our APIError can be compared directly against the SentinelError error type
using errors.Is().

APIError also supports comparison against gRPC status codes by wrapping them in a
GrpcCodeErr if we want to check for what status code was returned:

.. code-block:: go

    if errors.Is(err, pkerr.GrpcCodeErr(codes.FailedPrecondition)) {
        fmt.Println("APIError gRPC code is codes.FailedPrecondition")
    }

    // Output:
    // APIError gRPC code is codes.FailedPrecondition

.. note::

    These comparisons against non-APIError types are only implemented for errors.Is(),
    not errors.As()

Error Details
-------------

Lets take a quick look at all the details we get back from our APIError:

.. code-block:: go

    fmt.Println("NAME       :", apiErr.Proto.Name)
    fmt.Println("CODE       :", apiErr.Proto.Code)
    fmt.Println("ISSUER     :", apiErr.Proto.Issuer)
    fmt.Println("MESSAGE    :", apiErr.Proto.Message)
    fmt.Println("gRPC CODE  :", apiErr.Proto.GrpcCode)
    fmt.Println("TIME       :", apiErr.Proto.Time)
    fmt.Println("DETAILS    :", apiErr.Proto.Details)
    fmt.Println("SOURCE ERR :", apiErr.Proto.SourceError)
    fmt.Println("SOURCE TYPE:", apiErr.Proto.SourceType)

    // Output:
    // NAME       : SoulCloudy
    // CODE       : 2000
    // ISSUER     : Hogwarts
    // MESSAGE    : The pupil's soul is to opaque to be sorted: could not sort student Harry Potter
    // gRPC CODE  : 9
    // TIME       : seconds:1609462861 nanos:1
    // DETAILS    : [[type.googleapis.com/sortinghat.Pupil]:{name:"Harry Potter"}]
    // SOURCE ERR : EOF
    // SOURCE TYPE: *errors.errorString

TraceInfo
---------

Returned errors also contain a slice of ``*TraceInfo`` proto messages. Each time an
error generator encounters a new message, it will add it's own ``TraceInfo`` value
si we can trace the error through apps:

.. code-block:: go

    fmt.Println("TRACE COUNT:", len(apiErr.Proto.Trace))

    for i, thisTrace := range apiErr.Proto.Trace {
        // remove this code from example
        thisTrace.AppHost = "Williams-MacBook-Pro-2.local"
        thisTrace.StackTrace = "[debug.Stack() output]"

        fmt.Println()
        fmt.Println("TRACE INDEX  :", i)
        fmt.Println("APP NAME     :", thisTrace.AppName)
        fmt.Println("APP HOST     :", thisTrace.AppHost)
        fmt.Printf("CONTEXT      :%v\n", thisTrace.AdditionalContext)
        fmt.Println("TRACEBACK    :", thisTrace.StackTrace)
    }

    // Output:
    // TRACE COUNT: 2
    //
    // TRACE INDEX  : 0
    // APP NAME     : SortingHat
    // APP HOST     : Williams-MacBook-Pro-2.local
    // CONTEXT      :
    // TRACEBACK    : [debug.Stack() output]
    //
    // TRACE INDEX  : 1
    // APP NAME     : ClientApp
    // APP HOST     : Williams-MacBook-Pro-2.local
    // CONTEXT      :
    // TRACEBACK    : [debug.Stack() output]


Adding Context
--------------

Adding context as an error is passed up a chain can be incredibly useful in Go,
especially since errors do no contain traces.

Out interceptors support unwrapping an APIError from any error type that implements
xerrors.Wrapper. This means you can add as much context as you want!

Let's alter our handler code:

.. code-block:: go

    // Sort sorts pupils into a house.
    func (s SortingHat) Sort(
        ctx context.Context, pupil *protogen.Pupil,
    ) (*protogen.Sorted, error) {
        if pupil.Name == "Harry Potter" {
            err := errorGen.NewErr(
                ErrSoulCloudy, // Sentinel
                "could not sort student Harry Potter", // message
                []proto.Message{pupil}, // details
                io.EOF, // source
            )

            // Add some additional context.
            return nil, fmt.Errorf("hmmm, that's interesting...: %w", err)
        }

        return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
    }

This additional context will show up in the TraceInfo of the error:

.. code-block:: go

    // Try and sort the pupil
    _, err := hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Extract our APIError
    var apiErr pkerr.APIError
    if !errors.As(err, &apiErr) {
        panic("error is not APIError")
    }

    // Now we have some info in the context of the TraceInfo
    serverTrace := apiErr.Proto.Trace[0]

    fmt.Println("ADDITIONAL CONTEXT:", serverTrace.AdditionalContext)

    // Output:
    // ADDITIONAL CONTEXT: hmmm, that's interesting...: [error]

Panics
------

The interceptors will also recover panics:

.. code-block:: go

    // Sort sorts pupils into a house.
    func (s SortingHat) Sort(
        ctx context.Context, pupil *protogen.Pupil,
    ) (*protogen.Sorted, error) {
        if pupil.Name == "Harry Potter" {
            err := errorGen.NewErr(
                ErrSoulCloudy, // Sentinel
                "could not sort student Harry Potter", // message
                []proto.Message{pupil}, // details
                io.EOF, // source
            )

            // Panic with the error
            panic(err)
        }

        return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
    }

.. code-block:: go

    // Try and sort the pupil
    _, err := hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Extract our APIError
    var apiErr pkerr.APIError
    if !errors.As(err, &apiErr) {
        panic("error is not APIError")
    }

    // Now we have some info in the context of the TraceInfo
    serverTrace := apiErr.Proto.Trace[0]

    fmt.Println("ADDITIONAL CONTEXT:", serverTrace.AdditionalContext)

    // Output:
    // ADDITIONAL CONTEXT: panic recovered: [error]

Sentinel Returns
----------------

Sentinel errors can be returned directly or wrapped, which is a little more convenient
for method authors:

.. code-block:: go

    // Sort sorts pupils into a house.
    func (s SortingHat) Sort(
        ctx context.Context, pupil *protogen.Pupil,
    ) (*protogen.Sorted, error) {
        if pupil.Name == "Harry Potter" {
            return nil, fmt.Errorf(
                "could not sort student Harry Potter: %w", ErrSoulCloudy,
            )
        }

        return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
    }

.. code-block:: go

    // Try and sort the pupil
    _, err := hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Extract our APIError
    var apiErr pkerr.APIError
    if !errors.As(err, &apiErr) {
        panic("error is not APIError")
    }

    fmt.Println("ERROR:", err)

    // Now we have some info in the context of the TraceInfo
    serverTrace := apiErr.Proto.Trace[0]

    fmt.Println("ADDITIONAL CONTEXT:", serverTrace.AdditionalContext)

    // Output:
    // ERROR: (SoulCloudy | Hogwarts | 2000) The pupil's soul is to opaque to be sorted | from: grpc error 'FailedPrecondition'
    // ADDITIONAL CONTEXT: could not sort student Harry Potter: [error]

ErrUnknown
----------

When an error is returned to the interceptors that is NOT an APIError or SentinelError,
the error will be wrapped in a an APIError with pkerrs.ErrUnknown SentinelError
code / name values:

.. code-block:: go

    // Sort sorts pupils into a house.
    func (s SortingHat) Sort(
        ctx context.Context, pupil *protogen.Pupil,
    ) (*protogen.Sorted, error) {
        if pupil.Name == "Harry Potter" {
            return nil, io.EOF
        }

        return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
    }

.. code-block:: go

    // Try and sort the pupil
    _, err := hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Extract our APIError
    var apiErr pkerr.APIError
    if !errors.As(err, &apiErr) {
        panic("error is not APIError")
    }

    fmt.Println("ERROR:", err)

    // Output:
    // ERROR: (Unknown | GRPEAKEC | 1000) an unknown error occurred: EOF | from: grpc error 'Unknown'

Unknown will be returned for panics that do not return an APIError or SentinelError, or
any error value at all!

.. code-block:: go

    // Sort sorts pupils into a house.
    func (s SortingHat) Sort(
        ctx context.Context, pupil *protogen.Pupil,
    ) (*protogen.Sorted, error) {
		if pupil.Name == "Harry Potter" {
			panic("Harry Potter")
		}

		return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
    }

.. code-block:: go

    // Try and sort the pupil
    _, err := hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

    // Extract our APIError
    var apiErr pkerr.APIError
    if !errors.As(err, &apiErr) {
        panic("error is not APIError")
    }

    fmt.Println("ERROR:", err)

    // Output:
    // ERROR: (Unknown | GRPEAKEC | 1000) an unknown error occurred: panic recovered: Harry Potter | from: grpc error 'Unknown'

Reissuing Sentinels
-------------------

You may be asking yourself: Why do we need to define our SentinelErrors via a
SentinelIssuer? Why not just set their values directly?

If a backend is using multiple services all issuing their own Sentinels, we may want
them all to appear to the client as if they are coming from the same issuer, in order
to feel more cohesive / branded.

The SentinelIssuer keeps pointers to the original errors and can "reissue" these errors
with a new Issuer value.

Let's try re-issuing our generic errors:

.. code-block::

    sentinels.ApplyNewIssuer(
        "MyBackend", // issuer
            1000, // offset
    )

    fmt.Println("ISSUER:", ErrSoulCloudy.Issuer)
    fmt.Println("CODE:", ErrSoulCloudy.Code)

    // Output:
    // ISSUER: MyBackend
    // CODE: 3000

Our sentinels now have a new issuer!

We have also applied an offset to all of our error codes, this is useful if your backend
already has an error code 2000 in it's namespace. Our new error code is 3000.

SentinelIssuer Env Vars
-----------------------

Issuer and offset settings can also be set through Environmental Variables:

- **[ORIGINAL_ISSUER]_ERROR_ISSUER** will be applied to the issuer

- **[ORIGINAL_ISSUER]_ERROR_CODE_OFFSET** will apply an offset to any errors issued by
  a SentinelIssuer

If applyEnvSettings is true when calling NewSentinelIssuer, these environmental
variables will be checked and applied.

End-users can now configure the error Issuer and Codes returned by your service.
