package namedlocks

import (
	"sync"
)

type Unlocker interface {
	Unlock()
}

type namedLocks struct {
	m      sync.Mutex
	byName map[string]*sync.Mutex
}

func (l *namedLocks) Lock(name string) Unlocker { //nolint:ireturn
	m := l.lock(name)

	m.Lock()

	return m
}

func (l *namedLocks) lock(name string) *sync.Mutex {
	l.m.Lock()
	defer l.m.Unlock()

	if m, ok := l.byName[name]; ok {
		return m
	}

	m := &sync.Mutex{}
	l.byName[name] = m

	return m
}

func New(size int) *namedLocks {
	return &namedLocks{ //nolint:exhaustruct
		byName: make(map[string]*sync.Mutex, size),
	}
}
