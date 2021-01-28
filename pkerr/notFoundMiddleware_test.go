package pkerr_test

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/peake100/gRPEAKEC-go/pktesting"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io"
	"sync"
	"testing"
)

// These tests is getting tripped up hitting up a service from another concurrent
// test in another package, so we are going to run it on a different port.
const grpcTestAddress = ":50052"

type MockNotFoundService struct{}

func (mock MockNotFoundService) Id() string {
	return "MockNotFoundService"
}

func (mock MockNotFoundService) Setup(
	resourcesCtx context.Context,
	resourcesReleased *sync.WaitGroup,
	shutdownCtx context.Context,
	logger zerolog.Logger,
) error {
	return nil
}

func (mock MockNotFoundService) RegisterOnServer(server *grpc.Server) {
	pktesting.RegisterMockUnaryServiceServer(server, mock)
}

func (mock MockNotFoundService) Unary(
	ctx context.Context, any *any.Any,
) (*any.Any, error) {
	return any, nil
}

func (mock MockNotFoundService) Dummy(
	ctx context.Context, empty *empty.Empty,
) (*empty.Empty, error) {
	return nil, nil
}

type ErrNotFoundSuite struct {
	pktesting.ManagerSuite
	client     pktesting.MockUnaryServiceClient
	clientConn io.Closer
}

func (suite *ErrNotFoundSuite) SetupSuite() {
	errGen := pkerr.NewErrGenerator(
		"ErrNotFoundSuite",
		true,
		true,
		true,
		true,
	)

	opts := pkservices.NewManagerOpts().
		WithErrorGenerator(errGen).
		WithErrNotFoundMiddleware().
		WithGrpcServerAddress(grpcTestAddress)

	suite.Manager = pkservices.NewManager(opts, MockNotFoundService{})
	suite.ManagerSuite.SetupSuite()

	clientConn := suite.Manager.Test(suite.T()).
		GrpcClientConn(false, grpc.WithInsecure())

	suite.clientConn = clientConn
	suite.client = pktesting.NewMockUnaryServiceClient(clientConn)
}

func (suite *ErrNotFoundSuite) TearDownSuite() {
	defer suite.clientConn.Close()
	suite.ManagerSuite.TearDownSuite()
}

func (suite *ErrNotFoundSuite) TestErrNotFound() {
	ctx, cancel := pktesting.New3SecondCtx()
	defer cancel()

	_, err := suite.client.Dummy(ctx, new(emptypb.Empty))
	fmt.Println("ERR:", err)
	if !suite.Error(err, "got error") {
		suite.T().FailNow()
	}
	errAssert := pktesting.NewAssertAPIErr(suite.T(), err)
	errAssert.Sentinel(pkerr.ErrNotFound, true)
}

func (suite *ErrNotFoundSuite) TestNoError() {
	msg, err := anypb.New(wrapperspb.String("message"))
	if !suite.NoError(err, "pack message") {
		suite.T().FailNow()
	}

	ctx, cancel := pktesting.New3SecondCtx()
	defer cancel()
	result, err := suite.client.Unary(ctx, msg)
	if !suite.NoError(err, "make call") {
		suite.T().FailNow()
	}

	value, err := result.UnmarshalNew()
	if !suite.NoError(err, "unpack response") {
		suite.T().FailNow()
	}

	stringWrapper, ok := value.(*wrapperspb.StringValue)
	if !suite.True(ok, "value is string wrapper") {
		suite.T().FailNow()
	}

	suite.Equal("message", stringWrapper.Value)
}

func TestErrNotFoundSuite(t *testing.T) {
	suite.Run(t, new(ErrNotFoundSuite))
}
