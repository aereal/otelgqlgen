package resolvers

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct{}

type ForbiddenError struct{}

func (ForbiddenError) Error() string {
	return "forbidden"
}

type NotFoundError struct{}

func (NotFoundError) Error() string { return "not found" }
