package auth

import (
	"sort"
	"strings"
	"sync"
)

var (
	authMu         sync.RWMutex
	authenticators = map[string]Authenticator{}
)

func Register(name string, auth Authenticator) {
	authMu.Lock()
	defer authMu.Unlock()
	authenticators[strings.ToLower(name)] = auth
}

func Get(name string) (Authenticator, bool) {
	authMu.RLock()
	defer authMu.RUnlock()
	a, ok := authenticators[strings.ToLower(name)]
	return a, ok
}

func List() []Authenticator {
	authMu.RLock()
	defer authMu.RUnlock()
	seen := map[string]bool{}
	var list []Authenticator
	for _, a := range authenticators {
		if !seen[a.Name()] {
			seen[a.Name()] = true
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list
}
