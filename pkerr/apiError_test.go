package pkerr_test

import (
	"fmt"
	"github.com/illuscio-dev/protoCereal-go/cerealMessages"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"io"
	"reflect"
	"testing"
)

var ErrTest = &pkerr.SentinelError{
	Issuer:   "testing",
	Code:     2000,
	Name:     "TestError",
	GrpcCode: codes.Aborted,
}

func TestAPIError_Is(t *testing.T) {
	apiErr := pkerr.APIError{
		Proto: &pkerr.Error{
			Id:       cerealMessages.MustUUIDRandom(),
			Issuer:   pkerr.ErrUnknown.Issuer,
			Code:     pkerr.ErrUnknown.Code,
			GrpcCode: int32(pkerr.ErrUnknown.GrpcCode),
			Name:     pkerr.ErrUnknown.Name,
			Message:  "a test error occurred",
			Details:  nil,
			Trace:    nil,
		},
		Source: io.EOF,
	}

	testCases := []struct {
		Other    error
		Expected bool
	}{
		{
			Other: pkerr.APIError{
				Proto: &pkerr.Error{
					Id:       cerealMessages.MustUUIDRandom(),
					Issuer:   pkerr.ErrUnknown.Issuer,
					Code:     pkerr.ErrUnknown.Code,
					GrpcCode: int32(pkerr.ErrUnknown.GrpcCode),
					Message:  "",
					Details:  nil,
					Trace:    nil,
				},
				Source: io.ErrNoProgress,
			},
			Expected: true,
		},
		{
			Other: pkerr.APIError{
				Proto: &pkerr.Error{
					Id:       cerealMessages.MustUUIDRandom(),
					Issuer:   ErrTest.Issuer,
					Code:     ErrTest.Code,
					GrpcCode: int32(ErrTest.GrpcCode),
					Message:  "",
					Details:  nil,
					Trace:    nil,
				},
				Source: io.EOF,
			},
			Expected: false,
		},
		{
			Other: &pkerr.Error{
				Id:       cerealMessages.MustUUIDRandom(),
				Issuer:   pkerr.ErrUnknown.Issuer,
				Code:     pkerr.ErrUnknown.Code,
				GrpcCode: int32(pkerr.ErrUnknown.GrpcCode),
				Message:  "",
				Details:  nil,
				Trace:    nil,
			},
			Expected: true,
		},
		{
			Other: &pkerr.Error{
				Id:       cerealMessages.MustUUIDRandom(),
				Issuer:   ErrTest.Issuer,
				Code:     ErrTest.Code,
				GrpcCode: int32(ErrTest.GrpcCode),
				Message:  "",
				Details:  nil,
				Trace:    nil,
			},
			Expected: false,
		},
		{
			Other:    pkerr.ErrUnknown,
			Expected: true,
		},
		{
			Other:    ErrTest,
			Expected: false,
		},
		{
			Other:    pkerr.GrpcCodeErr(codes.Unknown),
			Expected: true,
		},
		{
			Other:    pkerr.GrpcCodeErr(codes.DeadlineExceeded),
			Expected: false,
		},
		{
			Other:    io.EOF,
			Expected: true,
		},
		{
			Other:    io.ErrClosedPipe,
			Expected: false,
		},
	}

	for _, thisCase := range testCases {
		name := fmt.Sprintf(
			"%v_%v", reflect.TypeOf(thisCase.Other), thisCase.Expected,
		)

		t.Run(name, func(t *testing.T) {
			if thisCase.Expected {
				assert.ErrorIs(t, apiErr, thisCase.Other)
			} else {
				assert.NotErrorIs(t, apiErr, thisCase.Other)
			}
		})
	}
}

func TestErrorDef_As(t *testing.T) {
	assert := assert.New(t)

	err := fmt.Errorf("%w: some context", ErrCustom)

	var apiErr pkerr.APIError
	if !assert.ErrorAs(err, &apiErr, "extract error") {
		t.FailNow()
	}

	assert.Equal(ErrCustom.Code, apiErr.Proto.Code, "code")
	assert.Equal(ErrCustom.Name, apiErr.Proto.Name, "name")
	assert.Equal(ErrCustom.Issuer, apiErr.Proto.Issuer, "issuer")
	assert.Equal(
		int32(ErrCustom.GrpcCode), apiErr.Proto.GrpcCode, "grpc code",
	)
	assert.Contains(
		apiErr.Proto.Message, ErrCustom.DefaultMessage, "default message",
	)
	assert.Len(apiErr.Proto.Trace, 1, "trace length")
}
