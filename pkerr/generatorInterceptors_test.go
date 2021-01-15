package pkerr_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/peake100/gRPEAKEC-go/pktesting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io"
	"sync"
	"testing"
)

const testAddress = ":7090"

var errorGenerator = pkerr.NewErrGenerator(
	"MockService",
	"server hostname",
	true,
	"Mock",
	false,
)

// Create a test error def.
var ErrCustom = errorGenerator.NewSentinel(
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

func (mock *MockService) Setup(
	resourcesCtx context.Context, resourcesReleased *sync.WaitGroup,
) error {
	return nil
}

func (mock *MockService) RegisterOnServer(server *grpc.Server) {
	pktesting.RegisterMockUnaryServiceServer(server, mock)
	pktesting.RegisterMockStreamServiceServer(server, mock)
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

	return nil
}

func (mock *MockService) Dummy(ctx context.Context, empty *empty.Empty) (*empty.Empty, error) {
	panic("implement me")
}

func (mock *MockService) DummyStream(server pktesting.MockStreamService_DummyStreamServer) error {
	panic("implement me")
}

func NewMockService() *MockService {
	errorGen := pkerr.NewErrGenerator(
		"MockService",
		"server hostname",
		true,
		"Test",
		false,
	)

	return &MockService{errors: errorGen}
}

type MockClient struct {
	pktesting.MockUnaryServiceClient
	pktesting.MockStreamServiceClient
}

type InterceptorSuite struct {
	pktesting.ManagerSuite

	clientConn grpc.ClientConnInterface
	client     MockClient
}

func (suite *InterceptorSuite) SetupSuite() {
	mockService := NewMockService()

	managerOpts := pkservices.NewManagerOpts().
		WithGrpcServerAddress(testAddress).
		WithPkErrInterceptors(mockService.errors)

	suite.Manager = pkservices.NewManager(managerOpts, mockService)
	suite.ManagerSuite.SetupSuite()

	// Create a new error generator for the client.
	clientErrs := mockService.errors.
		WithAppName("Client").
		WithAppHost("client hostname")

	// Create a client to connect to the server with our interceptors.
	var err error
	suite.clientConn, err = grpc.Dial(
		testAddress,
		grpc.WithUnaryInterceptor(clientErrs.NewUnaryClientInterceptor()),
		grpc.WithStreamInterceptor(clientErrs.NewStreamClientInterceptor()),
		grpc.WithInsecure(),
	)
	if !suite.NoError(err, "get client conn") {
		suite.FailNow("could not get client connection")
	}

	suite.client = MockClient{
		MockUnaryServiceClient:  pktesting.NewMockUnaryServiceClient(suite.clientConn),
		MockStreamServiceClient: pktesting.NewMockStreamServiceClient(suite.clientConn),
	}

	suite.Manager.Test(suite.T()).PingGrpcServer(nil)
}

func (suite *InterceptorSuite) TearDownSuite() {
	if suite.Manager != nil {
		defer suite.Manager.StartShutdown()
	}

	closer, ok := suite.clientConn.(io.Closer)
	if ok {
		defer closer.Close()
	}

	suite.ManagerSuite.TearDownSuite()
}

func (suite *InterceptorSuite) TestErrorReturns() {
	testCases := []struct {
		Name            string
		CaseType        int32
		ExpectedDef     *pkerr.SentinelError
		ExpectedMessage string
		ExpectedContext string
		SendStream      bool
	}{
		{
			Name:            "GeneratorAPIErrorUnary",
			CaseType:        caseNew,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
		},
		{
			Name:            "GeneratorAPIErrorStream",
			CaseType:        caseNew,
			ExpectedDef:     ErrCustom,
			SendStream:      true,
			ExpectedMessage: "data loss occurred: returning error directly",
		},
		{
			Name:            "GeneratorAPIErrorPanicUnary",
			CaseType:        caseNewPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
		},
		{
			Name:            "GeneratorAPIErrorPanicStream",
			CaseType:        caseNewPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			SendStream:      true,
		},
		{
			Name:            "GeneratorAPIErrorWrappedUnary",
			CaseType:        caseNewWrap,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "additional context: [error]",
		},
		{
			Name:            "GeneratorAPIErrorWrappedStream",
			CaseType:        caseNewWrap,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "additional context: [error]",
			SendStream:      true,
		},
		{
			Name:            "GeneratorAPIErrorWrappedPanicUnary",
			CaseType:        caseNewWrapPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "panic context: [error]",
		},
		{
			Name:            "GeneratorAPIErrorWrappedPanicStream",
			CaseType:        caseNewWrapPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred: returning error directly",
			ExpectedContext: "panic context: [error]",
			SendStream:      true,
		},
		{
			Name:            "ErrorDefUnary",
			CaseType:        caseDef,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "",
		},
		{
			Name:            "ErrorDefStream",
			CaseType:        caseDef,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "",
			SendStream:      true,
		},
		{
			Name:            "ErrorDefPanicUnary",
			CaseType:        caseDefPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
		},
		{
			Name:            "ErrorDefPanicStream",
			CaseType:        caseDefPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			SendStream:      true,
		},
		{
			Name:            "ErrorDefWrappedUnary",
			CaseType:        caseDefWrap,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "[error]: additional context",
		},
		{
			Name:            "ErrorDefWrappedStream",
			CaseType:        caseDefWrap,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "[error]: additional context",
			SendStream:      true,
		},
		{
			Name:            "ErrorDefWrappedPanicUnary",
			CaseType:        caseDefWrapPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "[error]: panic context",
		},
		{
			Name:            "ErrorDefWrappedPanicStream",
			CaseType:        caseDefWrapPanic,
			ExpectedDef:     ErrCustom,
			ExpectedMessage: "data loss occurred",
			ExpectedContext: "[error]: panic context",
			SendStream:      true,
		},
		{
			Name:            "GenericUnary",
			CaseType:        caseGeneric,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: generic error",
		},
		{
			Name:            "GenericStream",
			CaseType:        caseGeneric,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: generic error",
			SendStream:      true,
		},
		{

			Name:            "GenericPanicUnary",
			CaseType:        caseGenericPanic,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: generic error",
		},
		{
			Name:            "GenericPanicStream",
			CaseType:        caseGenericPanic,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: generic error",
			SendStream:      true,
		},
		{
			Name:            "NonErrPanicUnary",
			CaseType:        caseNonErrPanic,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: 11",
		},
		{
			Name:            "NonErrPanicStream",
			CaseType:        caseNonErrPanic,
			ExpectedDef:     pkerr.ErrUnknown,
			ExpectedMessage: "an unknown error occurred: 11",
			SendStream:      true,
		},
	}

	for _, thisCase := range testCases {
		suite.T().Run(thisCase.Name, func(t *testing.T) {
			msg, err := anypb.New(wrapperspb.Int32(thisCase.CaseType))
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			if thisCase.SendStream {
				stream, streamErr := suite.client.Stream(context.Background())
				if !assert.NoError(t, streamErr, "get stream") {
					t.FailNow()
				}
				streamErr = stream.Send(msg)
				if !assert.NoError(t, streamErr, "send stream msg") {
					t.FailNow()
				}
				_, err = stream.Recv()
			} else {
				_, err = suite.client.Unary(context.Background(), msg)
			}

			t.Log("ERROR:", err)
			assert := pktesting.NewAssertErr(t, err)
			assert.ErrorDef(thisCase.ExpectedDef, false)
			assert.Message(thisCase.ExpectedMessage)

			assert.TraceLength(2)

			assertTrace := assert.TraceIndex(0)
			assertTrace.AppName("MockService")
			assertTrace.AppHost("server hostname")
			assertTrace.HasStackTrace(true)
			assertTrace.AdditionalContext(thisCase.ExpectedContext)

			assertTrace = assert.TraceIndex(1)
			assertTrace.AppName("Client")
			assertTrace.AppHost("client hostname")
			assertTrace.HasStackTrace(true)
			assertTrace.AdditionalContext("")
		})
	}
}

func TestBasicErrorSuite(t *testing.T) {
	suite.Run(t, new(InterceptorSuite))
}
