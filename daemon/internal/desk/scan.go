package desk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

var errAdoptNotFound = errors.New("not found")

type pairResult struct {
	ID     string
	Name   string
	Token  string
	Status int
}

func (r *Registry) replaceSeen(seen []reverse.Seen) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = seen
}

func (r *Registry) lookupSeen(id, url string) (reverse.Seen, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.seen {
		if id != "" && string(s.ID) == id {
			return s, true
		}
		if url != "" && s.URL == url {
			return s, true
		}
	}
	return reverse.Seen{}, false
}

func (r *Registry) dropSeen(id, url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.seen[:0]
	for _, s := range r.seen {
		if id != "" && string(s.ID) == id {
			continue
		}
		if url != "" && s.URL == url {
			continue
		}
		out = append(out, s)
	}
	r.seen = out
}

func (r *Registry) Adopt(ctx context.Context, id, url string) error {
	seen, ok := r.lookupSeen(id, url)
	if !ok {
		return errAdoptNotFound
	}
	reclaim := r.reclaim
	if reclaim == nil {
		reclaim = reclaimKid
	}
	pr, err := reclaim(ctx, seen.URL)
	if err != nil {
		return err
	}
	kidID := string(seen.ID)
	if kidID == "" {
		kidID = pr.ID
	}
	if kidID == "" && id != "" {
		kidID = id
	}
	name := pr.Name
	if name == "" {
		name = string(seen.Name)
	}
	if kidID == "" || pr.Token == "" {
		return fmt.Errorf("reclaim: missing id or token")
	}
	kids, err := household.Load(r.path)
	if err != nil {
		return err
	}
	row := reverse.Record{
		ID:       reverse.KidID(kidID),
		Name:     reverse.KidName(name),
		URL:      seen.URL,
		Token:    pr.Token,
		PairedAt: time.Now().UTC(),
	}
	if err := household.Save(r.path, household.Upsert(kids, row)); err != nil {
		return err
	}
	r.dropSeen(kidID, seen.URL)
	go r.pushPin(reverse.KidID(kidID))
	return nil
}

func (r *Registry) handleAdopt(w http.ResponseWriter, req *http.Request) {
	var body struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if req.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(req.Body, 1<<16))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
		}
	}
	if err := r.Adopt(req.Context(), body.ID, body.URL); err != nil {
		if errors.Is(err, errAdoptNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	kids, err := household.Load(r.path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, r.Household(kids))
}

func reclaimKid(ctx context.Context, rawURL string) (pairResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(rawURL, "/")+"/v1/reclaim", strings.NewReader("{}"))
	if err != nil {
		return pairResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 1500 * time.Millisecond}).Do(req)
	if err != nil {
		return pairResult{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	out := pairResult{Status: resp.StatusCode}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("reclaim: %s", resp.Status)
	}
	var doc struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return out, err
	}
	out.ID = doc.ID
	out.Name = doc.Name
	out.Token = doc.Token
	return out, nil
}
