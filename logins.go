package main

import "sync"

type Login struct {
	failure uint
	success uint
	lock    *sync.Mutex
}

func (ls *Login) AddFailure() {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	ls.failure++
}

func (ls *Login) AddSuccess() {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	ls.success++
}

func (ls *Login) GetSuccesses() uint {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	return ls.success
}

func (ls *Login) GetFailures() uint {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	return ls.failure
}

func (ls *Login) GetAttempts() uint {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	return ls.success + ls.failure
}

type Logins struct {
	logins map[string]*Login
	lock   *sync.Mutex
}

func (ls *Logins) Has(host string) bool {
	ls.lock.Lock()
	defer ls.lock.Unlock()

	if _, ok := ls.logins[host]; !ok {
		return false
	}
	return true
}

func (ls *Logins) Get(host string) *Login {
	if !ls.Has(host) {
		ls.lock.Lock()
		ls.logins[host] = &Login{
			failure: 0,
			success: 0,
			lock:    &sync.Mutex{},
		}
		ls.lock.Unlock()
	}
	ls.lock.Lock()
	defer ls.lock.Unlock()
	return ls.logins[host]
}

func (ls *Logins) GetSuccesses() uint {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	n := uint(0)
	for _, l := range ls.logins {
		n += l.GetSuccesses()
	}
	return n
}

func (ls *Logins) GetFailures() uint {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	n := uint(0)
	for _, l := range ls.logins {
		n += l.GetFailures()
	}
	return n
}

func (ls *Logins) GetAttempts() uint {
	return ls.GetSuccesses() + ls.GetFailures()
}

func NewLogins() *Logins {
	return &Logins{
		logins: map[string]*Login{},
		lock:   &sync.Mutex{},
	}
}
