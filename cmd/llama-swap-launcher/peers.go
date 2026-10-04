package main

// llama-swap's peers: other llama-swaps whose models this one serves too,
// as "peer/model". The app learns of them only from the models llama-swap
// reports; nothing in its API lists the peers or their addresses.

import (
	"context"
	"encoding/json"
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
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

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
	resp, err := http.DefaultClient.Do(req)
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
