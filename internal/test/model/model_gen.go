// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

type Mutation struct {
}

type NestedInput struct {
	Val *string `json:"val,omitempty"`
}

type Query struct {
}

type RootInput struct {
	Nested *NestedInput `json:"nested"`
}

type User struct {
	Name    string `json:"name"`
	Age     *int   `json:"age,omitempty"`
	IsAdmin bool   `json:"isAdmin"`
}