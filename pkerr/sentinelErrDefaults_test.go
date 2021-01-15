package pkerr_test

import (
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReissueDefaultSentinels(t *testing.T) {
	assert := assert.New(t)

	t.Cleanup(func() {
		pkerr.ReissueDefaultSentinels(pkerr.DefaultsIssuer, 0)
	})

	if !assert.Equal(
		pkerr.DefaultsIssuer,
		pkerr.ErrUnknown.Issuer,
		"starting issuer expected",
	) {
		t.FailNow()
	}
	if !assert.Equal(
		uint32(1000),
		pkerr.ErrUnknown.Code,
		"starting code expected",
	) {
		t.FailNow()
	}

	// Reissue the sentinels made with this generator.
	pkerr.ReissueDefaultSentinels("NewIssuer", 1000)

	assert.Equal(
		"NewIssuer", pkerr.ErrUnknown.Issuer, "new issuer took",
	)
	assert.Equal(uint32(2000), pkerr.ErrUnknown.Code, "new code took")

	// Restore the sentinels made with this generator
	pkerr.ReissueDefaultSentinels(pkerr.DefaultsIssuer, 0)

	if !assert.Equal(
		pkerr.DefaultsIssuer,
		pkerr.ErrUnknown.Issuer,
		"original issuer restored",
	) {
		t.FailNow()
	}
	if !assert.Equal(
		uint32(1000),
		pkerr.ErrUnknown.Code,
		"starting code restored",
	) {
		t.FailNow()
	}
}
