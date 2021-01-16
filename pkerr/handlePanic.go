package pkerr

import "fmt"

// PanicErr wraps a recover() panic value as an error.
type PanicErr struct {
	// Source is the recovered error. If the value from recover() is not an error type
	// it will be coerce into an error string with format: "%+v"
	Recovered interface{}
}

// Error implements builtins.error.
func (err PanicErr) Error() string {
	return fmt.Sprintf("panic recovered: %+v", err.Recovered)
}

// Unwrap implements xerrors.Wrapper. If the underlying Recovered value is an error,
// it will be returned, otherwise an error-string will be returned.
func (err PanicErr) Unwrap() error {
	recoveredErr, ok := err.Recovered.(error)
	if !ok {
		recoveredErr = fmt.Errorf("%+v", err.Recovered)
	}

	return recoveredErr
}

// checkPanic takes in the input of recovered() and an error value, then returns
// an error if either is not nil.
//
// If both recovered and error are non-nil, recovered takes precedence.
func checkPanic(recovered interface{}, err error) error {
	if recovered != nil {
		return PanicErr{Recovered: recovered}
	}
	return err
}

// CatchPanic takes in a function with an error return, catches any panics that occur,
// and converts them to PanicErr. Returned errors are passed up as-is.
func CatchPanic(run func() error) (err error) {
	defer func() {
		// Handle any panics.
		err = checkPanic(recover(), err)
	}()

	err = run()
	return err
}
