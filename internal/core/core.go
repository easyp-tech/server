// Package core contains business logic.
package core

// Core manages business logic methods.
type Core struct {
	store Store
}

// New build and returns Core of system.
func New(store Store) *Core {
	return &Core{
		store: store,
	}
}
