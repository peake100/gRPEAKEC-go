package pkerr

import "google.golang.org/grpc/codes"

import (
	"fmt"
	"os"
	"strconv"
)

// SentinelIssuer issues new sentinel errors
type SentinelIssuer struct {
	// sentinelIssuer the issuer we will use for making new ErrorDefs
	sentinelIssuer string

	// sentinelOffset is the offset that will be applied to any Sentinel error created
	// with ErrorGenerator.NewSentinel
	sentinelOffset int

	// createdSentinels is a list of SentinelError pointers this generator has created.
	createdSentinels []*SentinelError
}

// NewSentinel creates a new *SentinelError and saves the pointer so it can be altered /
// offset later.
func (sentinels *SentinelIssuer) NewSentinel(
	name string,
	code uint32,
	grpcCode codes.Code,
	defaultMessage string,
) *SentinelError {
	sentinel := &SentinelError{
		Issuer: sentinels.sentinelIssuer,
		// We need to cast to int then cast back to uint32 in case the offset is
		// negative.
		Code:           uint32(int64(code) + int64(sentinels.sentinelOffset)),
		Name:           name,
		GrpcCode:       grpcCode,
		DefaultMessage: defaultMessage,

		// Store the original, non-offset code.
		codeOriginal: code,
	}

	sentinels.createdSentinels = append(sentinels.createdSentinels, sentinel)

	return sentinel
}

// ApplyNewIssuer rewrites all issued sentinels to have the given issuer and offset.
func (sentinels *SentinelIssuer) ApplyNewIssuer(issuer string, offset int) {
	// Set the new issuer and offset
	sentinels.sentinelIssuer = issuer
	sentinels.sentinelOffset = offset

	for _, sentinel := range sentinels.createdSentinels {
		sentinel.Issuer = issuer
		sentinel.Code = uint32(int64(sentinel.codeOriginal) + int64(offset))
	}
}

// applyEnvSettings loads issuer and code offset information from environmental
// variables and applies them to the generator, using the current settings as default
// if no env var is found.
//
// The environmental variables are the following:
//
// - [AppName]_ERROR_ISSUER
// - [AppName]_ERROR_CODE_OFFSET
//
// By having these settings configurable as environmental variables, two generic
// services that both use this error library can be merged into a single backend, and
// appear more uniform to a caller by having the same issuer.
//
// The code offset ensures that error codes can be shifted so they do not collide when
// two generic services are used in the same backend.
//
// When this method is called, the new issuer and offset are applied to all previously
// created sentinel errors. This method should not be called more than once, as it will
// cause the offset to be applied multiple times.
func (sentinels *SentinelIssuer) applyEnvSettings() *SentinelIssuer {
	issuerEnvKey := sentinels.sentinelIssuer + "_ERROR_ISSUER"
	offsetEnvKey := sentinels.sentinelIssuer + "_ERROR_CODE_OFFSET"

	issuer, ok := os.LookupEnv(issuerEnvKey)
	if !ok {
		issuer = sentinels.sentinelIssuer
	}

	offsetStr, ok := os.LookupEnv(offsetEnvKey)
	if !ok {
		offsetStr = strconv.Itoa(sentinels.sentinelOffset)
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		err = fmt.Errorf(
			"%v sentinel issuer could not parse value of '%v' env var: %w",
			sentinels.sentinelIssuer,
			offsetEnvKey,
			err,
		)
		panic(err)
	}

	sentinels.ApplyNewIssuer(issuer, offset)
	return sentinels
}

// NewSentinelIssuer returns a new *SentinelIssuer.
//
// issuer is the value to use for all *SentinelError values created with
// ErrorGenerator.NewSentinel.
//
// applyEnvSettings loads issuer and code offset information from environmental
// variables and applies them to the generator, using the current settings as default
// if no env var is found.
//
// The environmental variables are the following:
//
// - [AppName]_ERROR_ISSUER
// - [AppName]_ERROR_CODE_OFFSET
//
// By having these settings configurable as environmental variables, two generic
// services that both use this error library can be merged into a single backend, and
// appear more uniform to a caller by having the same issuer.
//
// The code offset ensures that error codes can be shifted so they do not collide when
// two generic services are used in the same backend.
func NewSentinelIssuer(issuer string, applyEnvSettings bool) *SentinelIssuer {
	sentinels := &SentinelIssuer{
		sentinelIssuer:   issuer,
		sentinelOffset:   0,
		createdSentinels: nil,
	}

	if applyEnvSettings {
		sentinels.applyEnvSettings()
	}

	return sentinels
}
