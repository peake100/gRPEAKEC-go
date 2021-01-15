package protogen

import (
	"fmt"
)

// Error implements builtins.error
func (x *Error) Error() string {
	return fmt.Sprintf("(%v | %v | %v) %v", x.Name, x.Issuer, x.Code, x.Message)
}

