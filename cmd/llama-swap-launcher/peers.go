package main

// llama-swap's peers: other llama-swaps whose models this one serves too,
// as "peer/model". The app learns of them only from the models llama-swap
// reports; nothing in its API lists the peers or their addresses.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// direct is the app's HTTP client for llama-swap, which ignores proxy
// settings; see health.NewClient.
var direct = health.NewClient()

// peerTimeout bounds each peer check: a peer host that's off can leave a
// connection hanging for much longer.
const peerTimeout = 3 * time.Second

// checkPeers reports whether each peer answers, checked through
// llama-swap's own proxy: /upstream/<peer>/<model>/health is answered by
// the peer's llama-swap. So a failure carries llama-swap's own reason,
// such as "no route to host" when macOS blocks local network access. The
// checks run at once, so a peer that's off costs peerTimeout, not more.
func (a *app) checkPeers(ctx context.Context, st supervisor.Status, snap health.Snapshot) []control.Peer {
	first := map[string]string{} // a model of each peer
	for _, m := range snap.Models {
		if m.PeerID != "" && first[m.PeerID] == "" {
			first[m.PeerID] = m.ID
		}
	}
	peers := make([]control.Peer, 0, len(first))
	for id := range first {
		peers = append(peers, control.Peer{ID: id})
	}
	slices.SortFunc(peers, func(a, b control.Peer) int { return strings.Compare(a.ID, b.ID) })
	var wg sync.WaitGroup
	for i := range peers {
		wg.Go(func() {
			err := checkPeer(ctx, st, first[peers[i].ID])
			peers[i].OK = err == nil
			if err != nil {
				peers[i].Error = err.Error()
			}
		})
	}
	wg.Wait()
	return peers
}

func checkPeer(ctx context.Context, st supervisor.Status, model string) error {
	ctx, cancel := context.WithTimeout(ctx, peerTimeout)
	defer cancel()
	req, err := upstreamRequest(ctx, st, model, "/health")
	if err != nil {
		return err
	}
	// Like the health checks, kept out of the app's copy of llama-swap's
	// log: one line per peer for every llsl status would be noise.
	req.Header.Set("User-Agent", health.CheckUserAgent)
	resp, err := direct.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("didn't answer within %s", peerTimeout)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
	msg := llamaSwapError(body)
	if strings.Contains(msg, "no route to host") {
		msg += ". If the peer is on, macOS may be blocking the app: allow it in System Settings → Privacy & Security → Local Network"
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
}

// llamaSwapError returns the message from one of llama-swap's JSON errors
// ({"error":{"message":…}}), or the body as it is.
func llamaSwapError(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return strings.TrimSpace(string(body))
}

// upstreamRequest makes a request for path on model's server, through
// llama-swap's /upstream route, with the app's API key if it has one.
func upstreamRequest(ctx context.Context, st supervisor.Status, model, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+localAddr(st.Listen)+"/upstream/"+escapeModel(model)+path, nil)
	if err == nil && st.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+st.APIKey)
	}
	return req, err
}

// escapeModel escapes a model ID for a URL path, a part at a time: a
// peer's model is "peer/model", and its slash is part of the route.
func escapeModel(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// noRedirects is the client for Fetch Models: direct, but a redirect comes
// back as it is. The request carries the peer's key, and Go drops only
// Authorization when a redirect leaves the host, not x-api-key, so
// following one could hand the key to wherever the peer points. llama-swap's
// own proxy never follows a redirect either; a peer behind a load balancer
// should be reached through it, at one address.
var noRedirects = &http.Client{
	Transport:     direct.Transport,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// fetchTimeout bounds Fetch models in Settings: long enough for a provider
// with a big list, short enough for a peer that's switched off.
const fetchTimeout = 15 * time.Second

// fetchPeerModels asks a peer for its models (GET /v1/models), for the
// Settings dialog's Fetch models. The list is OpenAI's, whose IDs are what
// llama-swap's peers config names. Another llama-swap also lists its own
// peers' models, marked as such (meta.llamaswap.type "peer"); those are
// left out, since a chain of peers is for the user to set up by hand. The
// key, if any, is sent as llama-swap sends it to peers. The IDs come back
// sorted, for finding one in a long list, with the number left out because
// they can't name a model in llama-swap's config (prefs.UsableModelID): a
// peer could send anything.
func fetchPeerModels(ctx context.Context, base, key string) (ids []string, skipped int, err error) {
	// As Save refuses (TODO item 40): Go would send the URL's password as
	// Basic authentication, a secret outside the keychain.
	if u, err := url.Parse(base); err == nil && u.User != nil {
		return nil, 0, errors.New("the URL has a user name or password in it. Put the key under Secrets, " +
			"as a secret variable, and choose that as the peer's key")
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/v1/models", nil)
	if err != nil {
		return nil, 0, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
	}
	resp, err := noRedirects.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // a long list is still a few MB at most
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		if to, err := resp.Location(); err == nil {
			return nil, 0, fmt.Errorf("the peer redirects to %s; use that address instead (HTTP %d)", to.Redacted(), resp.StatusCode)
		}
		return nil, 0, fmt.Errorf("the peer answered with a redirect (HTTP %d); use the address it moved to", resp.StatusCode)
	case (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && key == "":
		return nil, 0, fmt.Errorf("the peer wants an API key (HTTP %d). Choose the secret variable that holds it", resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, 0, fmt.Errorf("the peer didn't accept the key (HTTP %d)", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		msg := llamaSwapError(body)
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	var list struct {
		Data []struct {
			ID   string `json:"id"`
			Meta struct {
				LlamaSwap struct {
					Type string `json:"type"`
				} `json:"llamaswap"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, 0, fmt.Errorf("its reply isn't a /v1/models list: %w", err)
	}
	seen := map[string]bool{}
	ids = []string{}
	for _, m := range list.Data {
		switch {
		case m.ID == "" || m.Meta.LlamaSwap.Type == "peer" || seen[m.ID]:
		case !prefs.UsableModelID(m.ID):
			skipped++
		default:
			seen[m.ID] = true
			ids = append(ids, m.ID)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return ids, skipped, nil
}
