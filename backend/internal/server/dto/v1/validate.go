// Defines the validation interface for requests.

package dto

// Validatable is implemented by request types that can validate their fields.
// The Wrap functions in handler_wrapper.go use this interface as a type
// constraint to ensure all request types provide validation.
type Validatable interface {
	Validate() error
}
