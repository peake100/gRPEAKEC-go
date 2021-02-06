package pkerr_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkmiddleware"
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/peake100/gRPEAKEC-go/pktesting"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io"
	"os"
	"sync"
	"testing"
)

var sentinelIssuer = pkerr.NewSentinelIssuer(
	"Mock",
	false,
)

// Create a test error def.
var ErrCustom = sentinelIssuer.NewSentinel(
	"ErrCustom",
	2000,
	codes.DataLoss,
	"data loss occurred",
)

const (
	caseNew = iota
	caseNewPanic
	caseNewWrap
	caseNewWrapPanic
	caseDef
	caseDefPanic
	caseDefWrap
	caseDefWrapPanic
	caseGeneric
	caseGenericPanic
	caseNonErrPanic
)

// This mock service will return errors to test.
type MockService struct {
	errors *pkerr.ErrorGenerator
}

func (mock *MockService) Id() string {
	return "MockService"
}

func (mock *MockService) Setup(
	resourcesCtx context.Context,
	resourcesReleased *sync.WaitGroup,
	shutdownCtx context.Context,
	logger zerolog.Logger,
) error {
	return nil
}

func (mock *MockService) RegisterOnServer(server *grpc.Server) {
	pktesting.RegisterMockUnaryServiceServer(server, mock)
	pktesting.RegisterMockStreamServiceServer(server, mock)
	pktesting.RegisterMockUnaryStreamServiceServer(server, mock)
	pktesting.RegisterMockStreamUnaryServiceServer(server, mock)
}

// General method for creating an error.
func (mock *MockService) createError(msg *anypb.Any) error {
	var err error

	caseVal, err := msg.UnmarshalNew()
	if err != nil {
		return err
	}

	caseInt32 := caseVal.(*wrapperspb.Int32Value)

	switch caseInt32.GetValue() {
	case caseNew:
		err = mock.errors.NewErr(
			ErrCustom, "returning error directly", nil, nil,
		)
	case caseNewPanic:
		panicErr := mock.errors.NewErr(
			ErrCustom, "returning error directly", nil, nil,
		)
		panic(panicErr)
	case caseNewWrap:
		err = mock.errors.NewErr(
			ErrCustom, "returning error directly", nil, nil,
		)
		err = fmt.Errorf("additional context: %w", err)
	case caseNewWrapPanic:
		panicErr := mock.errors.NewErr(
			ErrCustom, "returning error directly", nil, nil,
		)
		panicErr = fmt.Errorf("panic context: %w", panicErr)
		panic(panicErr)
	case caseDef:
		err = ErrCustom
	case caseDefWrap:
		err = fmt.Errorf("%w: additional context", ErrCustom)
	case caseDefPanic:
		panicErr := ErrCustom
		panic(panicErr)
	case caseDefWrapPanic:
		panic(fmt.Errorf("%w: panic context", ErrCustom))
	case caseGeneric:
		err = errors.New("generic error")
	case caseGenericPanic:
		panic(errors.New("generic error"))
	case caseNonErrPanic:
		panic(11)
	default:
		return nil
	}

	return err
}

// This method returns an error based on the test case request.
func (mock *MockService) Unary(
	ctx context.Context, msg *anypb.Any,
) (*anypb.Any, error) {
	err := mock.createError(msg)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// This method returns an error based on the test case request.
func (mock *MockService) Stream(server pktesting.MockStreamService_StreamServer) error {
	msg, err := server.Recv()
	if err != nil {
		return err
	}

	err = mock.createError(msg)
	if err != nil {
		return err
	}

	return server.Send(msg)
}

func (mock *MockService) UnaryStream(
	msg *any.Any, server pktesting.MockUnaryStreamService_UnaryStreamServer,
) error {
	err := mock.createError(msg)
	if err != nil {
		return err
	}

	return server.Send(msg)
}

func (mock *MockService) StreamUnary(
	server pktesting.MockStreamUnaryService_StreamUnaryServer,
) error {
	msg, err := server.Recv()
	if err != nil {
		return err
	}

	err = mock.createError(msg)
	if err != nil {
		return err
	}

	return server.SendAndClose(msg)
}

func (mock *MockService) Dummy(
	ctx context.Context, empty *empty.Empty,
) (*empty.Empty, error) {
	panic("implement me")
}

func (mock *MockService) DummyStream(server pktesting.MockStreamService_DummyStreamServer) error {
	panic("implement me")
}

func (mock *MockService) DummyUnaryStream(
	e *empty.Empty, server pktesting.MockUnaryStreamService_DummyUnaryStreamServer,
) error {
	panic("implement me")
}

func (mock *MockService) DummyStreamUnary(server pktesting.MockStreamUnaryService_DummyStreamUnaryServer) error {
	panic("implement me")
}

func NewMockService() *MockService {
	errorGen := pkerr.NewErrGenerator(
		"MockService",
		true,
		true,
		true,
		true,
	)

	return &MockService{errors: errorGen}
}

type MockClient struct {
	pktesting.MockUnaryServiceClient
	pktesting.MockStreamServiceClient
	pktesting.MockUnaryStreamServiceClient
	pktesting.MockStreamUnaryServiceClient
}

type InterceptorSuite struct {
	pktesting.ManagerSuite

	clientConn grpc.ClientConnInterface
	client     MockClient
}

func (suite *InterceptorSuite) SetupSuite() {
	mockService := NewMockService()

	managerOpts := pkservices.NewManagerOpts().
		WithGrpcServerAddress(grpcTestAddress).
		WithErrorGenerator(mockService.errors)

	suite.Manager = pkservices.NewManager(managerOpts, mockService)
	suite.ManagerSuite.SetupSuite()

	// Create a new error generator for the client.
	clientErrs := mockService.errors.
		WithAppName("Client")

	// Create a client to connect to the server with our interceptors.
	var err error

	unaryInterceptor := pkmiddleware.NewUnaryClientMiddlewareInterceptor(
		clientErrs.UnaryClientMiddleware,
	)
	streamInterceptor := pkmiddleware.NewStreamClientMiddlewareInterceptor(
		clientErrs.StreamClientMiddleware,
	)

	suite.clientConn, err = grpc.Dial(
		grpcTestAddress,
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
		grpc.WithInsecure(),
	)
	if !suite.NoError(err, "get client conn") {
		suite.FailNow("could not get client connection")
	}

	suite.client = MockClient{
		MockUnaryServiceClient:  pktesting.NewMockUnaryServiceClient(suite.clientConn),
		MockStreamServiceClient: pktesting.NewMockStreamServiceClient(suite.clientConn),
		MockUnaryStreamServiceClient: pktesting.NewMockUnaryStreamServiceClient(
			suite.clientConn,
		),
		MockStreamUnaryServiceClient: pktesting.NewMockStreamUnaryServiceClient(
			suite.clientConn,
		),
	}
}

func (suite *InterceptorSuite) TearDownSuite() {
	closer, ok := suite.clientConn.(io.Closer)
	if ok {
		defer closer.Close()
	}

	suite.ManagerSuite.TearDownSuite()
}

type sendMethodCase int

const (
	caseUnaryUnary sendMethodCase = iota
	caseStreamStream
	caseUnaryStream
	caseStreamUnary
)

func (method sendMethodCase) String() string {
	switch method {
	case caseUnaryUnary:
		return "UnaryUnary"
	case caseStreamStream:
		return "StreamStream"
	case caseUnaryStream:
		return "UnaryStream"
	case caseStreamUnary:
		return "StreamUnary"
	default:
		panic("unknown send method case value")
	}
}

func (suite *InterceptorSuite) TestErrorReturns() {
	// Set up our test cases. Each case will be repeated against every gRPC send method.
	testCases := []struct {
		Name            string
		CaseType        int32
		ExpectedDef     *pkerr.SentinelError
		ExpectedMessage string
		ExpectedContext string
		SendStream      sendMethodCase
	}{
		{
			Name:            "GeneratorAPIError",
			CaseType:        caseNew,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
		},
		{
			Name:            "GeneratorAPIErrorPanic",
			CaseType:        caseNewPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "panic recovered: [error]",
		},
		{
			Name:            "GeneratorAPIErrorWrapped",
			CaseType:        caseNewWrap,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "additional context: [error]",
		},
		{
			Name:            "GeneratorAPIErrorWrappedPanic",
			CaseType:        caseNewWrapPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "panic recovered: panic context: [error]",
		},
		{
			Name:            "Sentinel",
			CaseType:        caseDef,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "",
		},
		{
			Name:            "ErrorDefPanic",
			CaseType:        caseDefPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "panic recovered: [error]",
		},
		{
			Name:            "ErrorDefWrapped",
			CaseType:        caseDefWrap,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "[error]: additional context",
		},
		{
			Name:            "ErrorDefWrappedPanic",
			CaseType:        caseDefWrapPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "panic recovered: [error]: panic context",
		},
		{
			Name:            "Generic",
			CaseType:        caseGeneric,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: generic error",
		},
		{

			Name:        "GenericPanic",
			CaseType:    caseGenericPanic,
			ExpectedDef: pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: panic recovered: " +
				"generic error",
		},
		{
			Name:            "NonErrPanic",
			CaseType:        caseNonErrPanic,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: panic recovered: 11",
		},
	}

	hostname, _ := os.Hostname()

	// Define transport methods for each type of transport test we are going to do.
	sendUnaryUnary := func(t *testing.T, msg *anypb.Any) error {
		ctx, cancel := pktesting.New3SecondCtx()
		defer cancel()

		_, err := suite.client.Unary(ctx, msg)
		return err
	}

	sendStreamStream := func(t *testing.T, msg *anypb.Any) error {
		ctx, cancel := pktesting.New3SecondCtx()
		defer cancel()

		stream, streamErr := suite.client.Stream(ctx)
		if !assert.NoError(t, streamErr, "get stream") {
			t.FailNow()
		}
		streamErr = stream.Send(msg)
		if !assert.NoError(t, streamErr, "send stream msg") {
			t.FailNow()
		}
		_, err := stream.Recv()
		return err
	}

	sendUnaryStream := func(t *testing.T, msg *anypb.Any) error {
		ctx, cancel := pktesting.New3SecondCtx()
		defer cancel()

		stream, err := suite.client.UnaryStream(ctx, msg)
		if !assert.NoError(t, err, "send unary") {
			t.FailNow()
		}

		_, err = stream.Recv()
		return err
	}

	sendStreamUnary := func(t *testing.T, msg *anypb.Any) error {
		ctx, cancel := pktesting.New3SecondCtx()
		defer cancel()

		stream, streamErr := suite.client.StreamUnary(ctx)
		if !assert.NoError(t, streamErr, "get stream") {
			t.FailNow()
		}
		streamErr = stream.Send(msg)
		if !assert.NoError(t, streamErr, "send stream msg") {
			t.FailNow()
		}

		_, err := stream.CloseAndRecv()
		return err
	}

	// Make a map of our sendMethodCase values to our send functions.
	sendFunctions := map[sendMethodCase]func(t *testing.T, msg *anypb.Any) error{
		caseUnaryUnary:   sendUnaryUnary,
		caseStreamStream: sendStreamStream,
		caseUnaryStream:  sendUnaryStream,
		caseStreamUnary:  sendStreamUnary,
	}

	// Iterate over each case
	for _, thisCase := range testCases {

		// for each case, iterate over the send methods.
		for sendCase, sendFunc := range sendFunctions {
			// Combine the name with the send method string for a full name.
			testName := fmt.Sprintf("%v_%v", thisCase.Name, sendCase)

			// Run the test.
			suite.T().Run(testName, func(t *testing.T) {
				msg, err := anypb.New(wrapperspb.Int32(thisCase.CaseType))
				if !assert.NoError(t, err) {
					t.FailNow()
				}

				err = sendFunc(t, msg)

				t.Log("ERROR RECEIVED:", err)
				assert := pktesting.NewAssertAPIErr(t, err)
				assert.Sentinel(thisCase.ExpectedDef, false)
				assert.Message(thisCase.ExpectedMessage)

				assert.TraceLength(2)

				assertTrace := assert.TraceIndex(0)
				assertTrace.AppName("MockService")
				assertTrace.AppHost(hostname)
				assertTrace.HasStackTrace(true)
				assertTrace.AdditionalContext(thisCase.ExpectedContext)

				assertTrace = assert.TraceIndex(1)
				assertTrace.AppName("Client")
				assertTrace.AppHost(hostname)
				assertTrace.HasStackTrace(true)
				assertTrace.AdditionalContext("")
			})
		}
	}
}

func TestBasicErrorSuite(t *testing.T) {
	suite.Run(t, &InterceptorSuite{
		ManagerSuite: pktesting.NewManagerSuite(nil),
	})
}
