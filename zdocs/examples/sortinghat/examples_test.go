package examples_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"github.com/peake100/gRPEAKEC-go/zdocs/examples/protogen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io"
	"net"
	"os"
	"reflect"
	"sync"
	"time"
)

// ErrSoulCloudy is returned when SortingHat cannot peer deep enough into a pupil's soul
// to sort them.
var ErrSoulCloudy = sentinels.NewSentinel(
	"SoulCloudy",
	2000,
	codes.FailedPrecondition,
	"The pupil's soul is to opaque to be sorted",
)

func ExamplePrintDefault() {
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
}

func ExamplePrintSentinel() {
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

func ExampleSentineErrorIs() {
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
	// ERROR: could not sort student Harry Potter: Hogwarts SoulCloudy (2000): The pupil's soul is to opaque to be sorted
	// This error is a ErrSoulCloudy error
	// This is not an unknown error
}

var host, _ = os.Hostname()

var errorGen = pkerr.NewErrGenerator(
	"SortingHat", // appName
	host,         // appHost
	true,         // addStackTrace
	true,         // sendContext
	true,         // sendSource
)

func ExampleNewErr() {
	// Returns an error
	err := errorGen.NewErr(
		ErrSoulCloudy,                         // Sentinel
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
}

var sortHandler = func(
	ctx context.Context, pupil *protogen.Pupil,
) (*protogen.Sorted, error) {
	if pupil.Name == "Harry Potter" {
		return nil, errorGen.NewErr(
			ErrSoulCloudy,                         // Sentinel
			"could not sort student Harry Potter", // message
			[]proto.Message{pupil},                // details
			io.EOF,                                // source
		)
	}

	return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
}

// Sorting hat implements protogen.SortingHat.
type SortingHat2 struct{}

// Sort sorts pupils into a house.
func (hat SortingHat2) Sort(
	ctx context.Context, pupil *protogen.Pupil,
) (*protogen.Sorted, error) {
	return sortHandler(ctx, pupil)
}

func ExampleNormalError() {
	// Set up the gRPC server
	listener, err := net.Listen("tcp", ":50052")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	server := grpc.NewServer()

	// Register our service
	protogen.RegisterSortingHatServer(server, SortingHat2{})

	// Serve gRPC
	serveErr := make(chan error)
	go func() {
		defer close(serveErr)
		serveErr <- server.Serve(listener)
	}()

	// Get a client connection
	clientConn, err := grpc.Dial(":50052", grpc.WithInsecure())
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
}

// runService sets up the gRPC service and returns a client as well as a shutdown
// function that will release all resources and block until complete when called.
func runService(
	address string,
) (client protogen.SortingHatClient, shutdown func()) {
	ctx, cancel := context.WithCancel(context.Background())
	complete := new(sync.WaitGroup)

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

	complete.Add(1)
	go func() {
		defer complete.Done()
		defer clientConn.Close()
		<-ctx.Done()
	}()

	hatClient := protogen.NewSortingHatClient(clientConn)

	shutdown = func() {
		defer listener.Close()
		cancel()
		complete.Wait()
		if err := <-serveErr; err != nil {
			panic(err)
		}
	}

	return hatClient, shutdown
}

func ExampleInterceptedError() {
	hatClient, shutdown := runService(":50053")
	defer shutdown()

	// Try and sort the pupil
	_, err := hatClient.Sort(context.Background(), &protogen.Pupil{Name: "Harry Potter"})

	// Print the error
	fmt.Println("ERROR:", err)
	fmt.Println("ERROR TYPE:", reflect.TypeOf(err))

	// Extract the error
	var apiErr pkerr.APIError
	if errors.As(err, &apiErr) && errors.Is(err, ErrSoulCloudy) {
		fmt.Println("API ERROR FOUND")
	}

	if errors.Is(err, pkerr.GrpcCodeErr(codes.FailedPrecondition)) {
		fmt.Println("APIError gRPC code is codes.FailedPrecondition")
	}

	// We won't copy this part into the ocs, but we need to replace the time with a
	// static value to make the output deterministic
	staticTime := time.Date(
		2021, 1, 1, 1, 1, 1, 1, time.UTC,
	)
	apiErr.Proto.Time = timestamppb.New(staticTime)

	fmt.Println()
	fmt.Println("NAME       :", apiErr.Proto.Name)
	fmt.Println("CODE       :", apiErr.Proto.Code)
	fmt.Println("ISSUER     :", apiErr.Proto.Issuer)
	fmt.Println("MESSAGE    :", apiErr.Proto.Message)
	fmt.Println("gRPC CODE  :", apiErr.Proto.GrpcCode)
	fmt.Println("TIME       :", apiErr.Proto.Time.AsTime())
	fmt.Println("DETAILS    :", apiErr.Proto.Details)
	fmt.Println("SOURCE ERR :", apiErr.Proto.SourceError)
	fmt.Println("SOURCE TYPE:", apiErr.Proto.SourceType)

	fmt.Println()
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
	// ERROR: (SoulCloudy | Hogwarts | 2000) The pupil's soul is to opaque to be sorted: could not sort student Harry Potter | from: grpc error 'FailedPrecondition'
	// ERROR TYPE: pkerr.APIError
	// API ERROR FOUND
	// APIError gRPC code is codes.FailedPrecondition
	//
	// NAME       : SoulCloudy
	// CODE       : 2000
	// ISSUER     : Hogwarts
	// MESSAGE    : The pupil's soul is to opaque to be sorted: could not sort student Harry Potter
	// gRPC CODE  : 9
	// TIME       : 2021-01-01 01:01:01.000000001 +0000 UTC
	// DETAILS    : [[type.googleapis.com/sortinghat.Pupil]:{name:"Harry Potter"}]
	// SOURCE ERR : EOF
	// SOURCE TYPE: *errors.errorString
	//
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
}

func ExampleWrappedAPIError() {
	sortHandler = func(
		ctx context.Context, pupil *protogen.Pupil,
	) (*protogen.Sorted, error) {
		if pupil.Name == "Harry Potter" {
			err := errorGen.NewErr(
				ErrSoulCloudy,                         // Sentinel
				"could not sort student Harry Potter", // message
				[]proto.Message{pupil},                // details
				io.EOF,                                // source
			)

			// Add some additional context.
			return nil, fmt.Errorf("hmmm, that's interesting...: %w", err)
		}

		return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
	}

	hatClient, shutdown := runService(":50054")
	defer shutdown()

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
}

func ExamplePanic() {
	sortHandler = func(
		ctx context.Context, pupil *protogen.Pupil,
	) (*protogen.Sorted, error) {
		if pupil.Name == "Harry Potter" {
			err := errorGen.NewErr(
				ErrSoulCloudy,                         // Sentinel
				"could not sort student Harry Potter", // message
				[]proto.Message{pupil},                // details
				io.EOF,                                // source
			)

			panic(err)
		}

		return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
	}

	hatClient, shutdown := runService(":50055")
	defer shutdown()

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
}

func ExampleReturnSentinel() {
	sortHandler = func(
		ctx context.Context, pupil *protogen.Pupil,
	) (*protogen.Sorted, error) {
		if pupil.Name == "Harry Potter" {
			return nil, fmt.Errorf(
				"could not sort student Harry Potter: %w", ErrSoulCloudy,
			)
		}

		return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
	}

	hatClient, shutdown := runService(":50056")
	defer shutdown()

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
}

func ExampleUnknownError() {
	sortHandler = func(
		ctx context.Context, pupil *protogen.Pupil,
	) (*protogen.Sorted, error) {
		if pupil.Name == "Harry Potter" {
			return nil, io.EOF
		}

		return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
	}

	hatClient, shutdown := runService(":50057")
	defer shutdown()

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
}

func ExampleUnknownPanic() {
	sortHandler = func(
		ctx context.Context, pupil *protogen.Pupil,
	) (*protogen.Sorted, error) {
		if pupil.Name == "Harry Potter" {
			panic("Harry Potter")
		}

		return &protogen.Sorted{House: protogen.House_Gryffindor}, nil
	}

	hatClient, shutdown := runService(":50058")
	defer shutdown()

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
}

func ExampleReissueSentinels() {
	sentinels.ApplyNewIssuer(
		"MyBackend", // issuer
		1000,        // offset
	)

	fmt.Println("ISSUER:", ErrSoulCloudy.Issuer)
	fmt.Println("CODE:", ErrSoulCloudy.Code)

	// Output:
	// ISSUER: MyBackend
	// CODE: 3000
}
