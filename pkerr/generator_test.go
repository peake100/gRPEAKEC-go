package pkerr_test

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"os"
	"testing"
)

// Tests that we can change the Issuer and Code of sentinel errors after the fact.
func TestSentinelIssuer_ApplyNewIssuer(t *testing.T) {
	assert := assert.New(t)

	issuer := pkerr.NewSentinelIssuer(
		"testIssuer",
		false,
	)

	// Create a sentinel error.
	sentinelOne := issuer.NewSentinel(
		"SentinelOne",
		2000, codes.Canceled,
		"sentinel one message",
	)

	assert.Equal(
		"testIssuer", sentinelOne.Issuer, "original issuer",
	)

	assert.Equal(
		uint32(2000), sentinelOne.Code, "original code",
	)

	// Reissue the sentinels with a new issuer and offset
	issuer.ApplyNewIssuer("testIssuer2", 1000)

	assert.Equal(
		"testIssuer2", sentinelOne.Issuer, "second issuer",
	)

	assert.Equal(
		uint32(3000), sentinelOne.Code, "second code",
	)

	// Reissue the sentinels again with a new issuer and offset. This second offset
	// should be relative to the original codes, not the re-issued ones.
	issuer.ApplyNewIssuer("testIssuer3", 2000)

	assert.Equal(
		"testIssuer3", sentinelOne.Issuer, "second issuer",
	)

	assert.Equal(
		uint32(4000), sentinelOne.Code, "second code",
	)
}

func TestSentinelIssuer_EnvVars(t *testing.T) {
	assert := assert.New(t)

	err := os.Setenv("OriginalIssuer"+"_ERROR_ISSUER", "EnvIssuer")
	if !assert.NoError(err, "set issuer env var") {
		t.FailNow()
	}

	err = os.Setenv("OriginalIssuer"+"_ERROR_CODE_OFFSET", "4000")
	if !assert.NoError(err, "set issuer env var") {
		t.FailNow()
	}

	issuer := pkerr.NewSentinelIssuer(
		"OriginalIssuer",
		true,
	)

	err = issuer.NewSentinel("TestErr", 1000, 0, "")
	sentinelErr := err.(*pkerr.SentinelError)

	assert.Equal(sentinelErr.Issuer, "EnvIssuer", "issuer changed")
	assert.Equal(sentinelErr.Code, uint32(5000), "code offset")
}
