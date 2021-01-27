//revive:disable
package pktesting

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"testing"
)

type TraceInfoAssert struct {
	assertions *assert.Assertions

	traceInfo *pkerr.TraceInfo
	index     int
}

func (traceAssert *TraceInfoAssert) AppName(expected string) bool {
	return traceAssert.assertions.Equalf(
		expected, traceAssert.traceInfo.GetAppName(),
		"app name, trace index %v",
		traceAssert.index,
	)
}

func (traceAssert *TraceInfoAssert) AppHost(expected string) bool {
	return traceAssert.assertions.Equalf(
		expected, traceAssert.traceInfo.GetAppHost(),
		"app host, trace index %v",
		traceAssert.index,
	)
}

func (traceAssert *TraceInfoAssert) AdditionalContext(expected string) bool {
	return traceAssert.assertions.Equalf(
		expected, traceAssert.traceInfo.GetAdditionalContext(),
		"additional context, trace index %v",
		traceAssert.index,
	)
}

func (traceAssert *TraceInfoAssert) HasStackTrace(expected bool) bool {
	if expected {
		return traceAssert.assertions.NotEmptyf(
			traceAssert.traceInfo.GetStackTrace(),
			"has traceback, trace index %v",
			traceAssert.index,
		)
	} else {
		return traceAssert.assertions.Emptyf(
			expected, traceAssert.traceInfo.GetStackTrace(),
			"no traceback, trace index %v",
			traceAssert.index,
		)
	}
}

// AssertErr is a test helper for making assertions about the expected value of an API
// error.
type AssertErr struct {
	t          *testing.T
	assertions *assert.Assertions

	err pkerr.APIError
}

func (asserter *AssertErr) Is(expected error) bool {
	return asserter.assertions.ErrorIs(
		asserter.err, expected, "errors.Is()",
	)
}

func (asserter *AssertErr) Name(expected string) bool {
	return asserter.assertions.Equal(
		expected, asserter.err.Proto.GetName(), "error name",
	)
}

func (asserter *AssertErr) Code(expected uint32) bool {
	return asserter.assertions.Equal(
		expected, asserter.err.Proto.GetCode(), "error code",
	)
}

func (asserter *AssertErr) Issuer(expected string) bool {
	return asserter.assertions.Equal(
		expected, asserter.err.Proto.GetIssuer(), "error code issuer",
	)
}

func (asserter *AssertErr) GrpcCode(expected codes.Code) bool {
	return asserter.assertions.Equal(
		int32(expected), asserter.err.Proto.GetIssuer(), "grpc code",
	)
}

func (asserter *AssertErr) Message(expected string) bool {
	return asserter.assertions.Equal(
		expected, asserter.err.Proto.GetMessage(), "message is equal",
	)
}

func (asserter *AssertErr) MessageContains(contains string) bool {
	return asserter.assertions.Contains(
		asserter.err.Proto.GetMessage(), contains, "message contains",
	)
}

func (asserter *AssertErr) ErrorDef(
	expected *pkerr.SentinelError, fullMessage bool,
) (result bool) {
	result = true

	if !asserter.assertions.ErrorIs(asserter.err, expected) {
		result = false
	}
	if !asserter.Name(expected.Name) {
		result = false
	}
	if !asserter.Code(expected.Code) {
		result = false
	}
	if !asserter.Issuer(expected.Issuer) {
		result = false
	}
	if !asserter.MessageContains(expected.DefaultMessage) {
		result = false
	}
	if fullMessage && !asserter.Message(expected.DefaultMessage) {
		result = false
	}

	return result
}

func (asserter *AssertErr) TraceLength(expected int) bool {
	return asserter.assertions.Len(
		asserter.err.Proto.Trace, expected, "trace length",
	)
}

func (asserter *AssertErr) TraceIndex(index int) TraceInfoAssert {
	if !asserter.assertions.GreaterOrEqual(
		len(asserter.err.Proto.Trace), index+1, "trace has index",
	) {
		asserter.t.FailNow()
	}

	return TraceInfoAssert{
		assertions: asserter.assertions,
		traceInfo:  asserter.err.Proto.Trace[index],
		index:      index,
	}
}

// NewAssertAPIErr returns an assert value with methods for testing errors.
func NewAssertAPIErr(t *testing.T, err error) *AssertErr {
	assertions := assert.New(t)

	apiErr := pkerr.APIError{}
	if !assertions.Error(err, "error is not nil") {
		t.FailNow()
	}
	if !assertions.ErrorAs(err, &apiErr, "errors.As() APIError") {
		t.FailNow()
	}

	return &AssertErr{
		t:          t,
		assertions: assertions,
		err:        apiErr,
	}
}
