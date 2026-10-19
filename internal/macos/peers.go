package macos

// Fetch models, on the Settings dialog's Peers tab, asks the app's Go code
// for a peer's models: the dialog calls lsl_fetch_peer_models, exported
// below, which runs the function the app set with SetPeerModelsFetcher. A
// file with an //export may only declare things in its C preamble.

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"sync"
)

var (
	fetcherMu sync.Mutex
	fetcher   func(url, key string) ([]string, int, error)
)

// SetPeerModelsFetcher sets what Fetch models runs: given a peer's URL and
// its key ("" for none), it returns the peer's models, and how many it left
// out because their names couldn't be used.
func SetPeerModelsFetcher(f func(url, key string) ([]string, int, error)) {
	fetcherMu.Lock()
	defer fetcherMu.Unlock()
	fetcher = f
}

//export lsl_fetch_peer_models
func lsl_fetch_peer_models(url, key *C.char) *C.char {
	fetcherMu.Lock()
	f := fetcher
	fetcherMu.Unlock()
	var reply struct {
		Models  []string `json:"models"`
		Skipped int      `json:"skipped"`
		Error   string   `json:"error,omitempty"`
	}
	models, skipped, err := []string(nil), 0, errors.New("fetching isn't set up")
	if f != nil {
		models, skipped, err = f(C.GoString(url), C.GoString(key))
	}
	if err != nil {
		reply.Error = err.Error()
	}
	reply.Models, reply.Skipped = models, skipped
	b, _ := json.Marshal(reply) // strings always marshal
	return C.CString(string(b))
}
