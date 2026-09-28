package coreclient

import "errors"

var (
	ErrMultipleJSONValues = errors.New("multiple JSON values")
)

type BusinessError struct {
	HTTPStatus int
}

func (e *BusinessError) Error() string {
	return "Core business error"
}

type ContractError struct {
	Cause error
}

func (e *ContractError) Error() string {
	return "Core contract violation"
}

func (e *ContractError) Unwrap() error {
	return e.Cause
}

type TransportError struct {
	Cause error
}

func (e *TransportError) Error() string {
	return "Core transport error"
}

func (e *TransportError) Unwrap() error {
	return e.Cause
}
