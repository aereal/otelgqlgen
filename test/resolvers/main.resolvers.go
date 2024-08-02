package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.44

import (
	"context"
	"errors"

	"github.com/aereal/otelgqlgen/test/execschema"
	"github.com/aereal/otelgqlgen/test/model"
)

// RegisterUser is the resolver for the registerUser field.
func (r *mutationResolver) RegisterUser(ctx context.Context, name string) (bool, error) {
	return true, nil
}

// User is the resolver for the user field.
func (r *queryResolver) User(ctx context.Context, name string) (*model.User, error) {
	if name == "forbidden" {
		return nil, ForbiddenError{}
	}
	age := 17
	return &model.User{Name: name, Age: &age}, nil
}

// Root is the resolver for the root field.
func (r *queryResolver) Root(ctx context.Context, num *int, rootInput *model.RootInput) (bool, error) {
	return true, nil
}

// Name is the resolver for the name field.
func (r *userResolver) Name(ctx context.Context, obj *model.User) (string, error) {
	if obj.Name == "invalid" {
		return "", errors.New("invalid name")
	}
	return obj.Name, nil
}

// Age is the resolver for the age field.
func (r *userResolver) Age(ctx context.Context, obj *model.User) (*int, error) {
	if obj.Name == "invalid" {
		return nil, errors.New("invalid age")
	}
	age := 17
	return &age, nil
}

// Mutation returns execschema.MutationResolver implementation.
func (r *Resolver) Mutation() execschema.MutationResolver { return &mutationResolver{r} }

// Query returns execschema.QueryResolver implementation.
func (r *Resolver) Query() execschema.QueryResolver { return &queryResolver{r} }

// User returns execschema.UserResolver implementation.
func (r *Resolver) User() execschema.UserResolver { return &userResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type userResolver struct{ *Resolver }
