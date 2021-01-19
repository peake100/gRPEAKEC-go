package pkservices

import (
	"fmt"
	"github.com/peake100/gRPEAKEC-go/pkerr"
	"strings"
)

const indent = "    "

// PanicError is a type alias for pkerr.PanicError.
type PanicError = pkerr.PanicError

// ServiceError reports an error from a specific service.
type ServiceError struct {
	// ServiceId is the return of the service's Service.Id method.
	ServiceId string
	// The error the service threw.
	Err error
}

// Error implements builtins.error.
func (err ServiceError) Error() string {
	return fmt.Sprintf("service '%v': %v", err.ServiceId, err.Err)
}

// Unwrap implements xerrors.Wrapper and returns the underlying error.
func (err ServiceError) Unwrap() error {
	return err.Err
}

// ServicesErrors stores errors from multiple services as a single error.
type ServicesErrors struct {
	Errs []error
}

// Error implements builtins.errors, and reports the number of errors.
func (err ServicesErrors) Error() string {
	// Create a string builder.
	builder := new(strings.Builder)

	// Write the first line.
	_, _ = builder.WriteString(fmt.Sprintf("%v errors occured:", len(err.Errs)))

	// Put each sub-error on it's own indented line.
	for _, thisErr := range err.Errs {
		_, _ = builder.WriteString(fmt.Sprintf("\n%v- %v", indent, thisErr))
	}

	return builder.String()
}

// ManagerError is returned from Manager.Run, and returns errors from the various stages
// of the run.
type ManagerError struct {
	// SetupErr is an error that was returned during setup of the services. This error
	// will be a ServicesErrors if the cause of the error was one or more services.
	SetupErr error
	// RunErr is an error that occurred during running of 1 or more services.This error
	// will be a ServicesErrors if the cause of the error was one or more services.
	RunErr error
	// ShutdownErr is an error that occurred during the shutdown of 1 or more services.
	ShutdownErr error
}

// indentErr takes in an error and indents each line of its error.Error() message.
func indentErr(err error, indentCount int) string {
	lines := strings.Split(err.Error(), "\n")
	for i, thisLine := range lines {
		lines[i] = strings.Repeat(indent, indentCount) + thisLine
	}

	return strings.Join(lines, "\n")
}

// errorAddStageErr adds stage error text to builder.
func (err ManagerError) errorAddStageErr(
	builder *strings.Builder, stage string, stageErr error,
) {
	_, _ = builder.WriteString(
		fmt.Sprintf("\n%v---- %v ERRORS ----", indent, stage),
	)
	_, _ = builder.WriteString(fmt.Sprintf("\n" + indentErr(stageErr, 1)))
}

// Error implements builtins.error
func (err ManagerError) Error() string {
	// Collect the stages on which an error occurred.
	builder := new(strings.Builder)

	// Write the header.
	_, _ = builder.WriteString("errors occurred during manger run:")

	// Write the setup section.
	if err.SetupErr != nil {
		err.errorAddStageErr(builder, "SETUP", err.SetupErr)
	}

	// Write the run section.
	if err.RunErr != nil {
		err.errorAddStageErr(builder, "RUN", err.RunErr)
	}

	// Write the shutdown section.
	if err.ShutdownErr != nil {
		err.errorAddStageErr(builder, "SHUTDOWN", err.ShutdownErr)
	}

	// Return the string.
	return builder.String()
}

// hasErrors returns true if any non-nil errors are stored.
func (err ManagerError) hasErrors() bool {
	return err.SetupErr != nil || err.RunErr != nil || err.ShutdownErr != nil
}
