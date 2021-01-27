package examples_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkclients"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/peake100/gRPEAKEC-go/zdocs/examples/protogen"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"math/big"
	"os"
	"sync"
)

type DBConnection struct{}

func (db *DBConnection) StoreStudent(pupil *protogen.Pupil, house protogen.House) error {
	if pupil.Name == "Draco Malfoy" {
		return errors.New("roster is full")
	}

	return nil
}

func (db *DBConnection) Close() error {
	return nil
}

func NewDBConnection(ctx context.Context) (*DBConnection, error) {
	return new(DBConnection), nil
}

var sentinels = pkerr.NewSentinelIssuer(
	"Hogwarts", // issuer
	true,       // applyEnvSettings
)

// ErrRosterUpdate is returned when there is an error updating the Roster
var ErrRosterUpdate = sentinels.NewSentinel(
	"RosterUpdate", // name
	2000,           // codde
	codes.Internal, // grpcCode
	"roster could not be updated with student",
)

// SortingHat is a gRPC service that sorts incoming pupils into houses.
type SortingHat struct {
	// errGen generates our api errors
	errGen *pkerr.ErrorGenerator

	// db holds our database connector.
	db *DBConnection
}

// Id implements pkservices.Service and returns  "SortingHat".
func (hat SortingHat) Id() string {
	return "SortingHat"
}

// Setup implements pkservices.Service and spins up resources.
func (hat SortingHat) Setup(
	resourcesCtx context.Context,
	resourcesReleased *sync.WaitGroup,
	shutdownCtx context.Context,
	logger zerolog.Logger,
) error {
	// Get our DB connection.
	var err error
	hat.db, err = NewDBConnection(resourcesCtx)
	if err != nil {
		return fmt.Errorf("could not connect to db: %w", err)
	}

	// Start a routine to close the connection when the resourceCtx is cancelled.
	resourcesReleased.Add(1)
	go func() {
		// Close the resourcesReleased WaitGroup, this will signal to the Manager that
		// all resources have been released.
		defer resourcesReleased.Done()
		defer hat.db.Close()
		<-resourcesCtx.Done()
	}()

	return nil
}

// RegisterOnServer implements pkservices.GrpcService and registers the service on a
// gRPC server.
func (hat SortingHat) RegisterOnServer(server *grpc.Server) {
	protogen.RegisterSortingHatServer(server, hat)
}

// Sort implements protogen.SortingHatServer and
func (hat SortingHat) Sort(
	ctx context.Context, pupil *protogen.Pupil,
) (*protogen.Sorted, error) {
	houseInt, err := rand.Int(rand.Reader, big.NewInt(4))
	if err != nil {
		return nil, err
	}

	house := []protogen.House{
		protogen.House_Gryffindor,
		protogen.House_Hufflepuff,
		protogen.House_Ravenclaw,
		protogen.House_Slytherin,
	}[houseInt.Int64()]

	// Try to store our pupil in a house.
	err = hat.db.StoreStudent(pupil, house)

	// Return an APIError on error.
	if err != nil {
		return nil, hat.errGen.NewErr(
			ErrRosterUpdate,                      //sentinel
			"sorted student could not be stored", // message
			[]proto.Message{pupil},               // details
			err,                                  // source
		)
	}

	// Return the house on a success.
	return &protogen.Sorted{House: house}, nil
}

func ExampleQuickStart() {
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
		// Pass our error generator to create error interceptors.
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

		// Print this error if it is a ErrRosterUpdate. We can use native error handling
		// here thanks to our interceptors.
		if errors.Is(err, ErrRosterUpdate) {
			fmt.Println("ERROR:", err)
			continue
		} else if err != nil {
			// Panic if it is any other type of error
			panic(err)
		}

		// Print the name of the sorted pupil
		fmt.Printf("%v sorted into %v\n", thisPupil.Name, sorted.House)
	}

	// Start shutdown of the manager and wait for it to finish.
	manager.StartShutdown()

	// Wait for our manager run result.
	if err := <-managerResult; err != nil {
		panic(err)
	}

	// Outputs:
	// 12:52AM INF running service manager HOST=Williams-MacBook-Pro-2.local SETTING_ADD_PING_SERVICE=true SETTING_MAX_SHUTDOWN=30000
	// 12:52AM INF running setup HOST=Williams-MacBook-Pro-2.local SERVICE="gPEAKERC Ping"
	// 12:52AM INF running setup HOST=Williams-MacBook-Pro-2.local SERVICE=SortingHat
	// 12:52AM INF setup complete HOST=Williams-MacBook-Pro-2.local SERVICE="gPEAKERC Ping"
	// 12:52AM INF setup complete HOST=Williams-MacBook-Pro-2.local SERVICE=SortingHat
	// 12:52AM INF serving gRPC HOST=Williams-MacBook-Pro-2.local SERVER_ADDRESS=:50051
	// Harry Potter sorted into Gryffindor
	// ERROR: (RosterUpdate | MyBackend | 3000) roster could not be updated with student: sorted student could not be stored | from: grpc error 'Internal'
	// 12:52AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
	// 12:52AM INF gRPC server shutdown HOST=Williams-MacBook-Pro-2.local
	// 12:52AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
	// 12:52AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
	// 12:52AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
	// 12:52AM INF shutdown order triggered HOST=Williams-MacBook-Pro-2.local
}
