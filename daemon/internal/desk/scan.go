package desk

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

var errAdoptNotFound = errors.New("not found")

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

func (r *Registry) noteSeen(o reverse.Offer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := reverse.Seen{ID: o.ID, Name: o.Name, Claimed: true}
	for i, old := range r.seen {
		if old.ID != "" && old.ID == o.ID {
			s.URL = old.URL
			r.seen[i] = s
			return
		}
	}
	r.seen = append(r.seen, s)
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

func (r *Registry) Adopt(id, url string) error {
	seen, ok := r.lookupSeen(id, url)
	if !ok {
		return errAdoptNotFound
	}
	kidID := string(seen.ID)
	if kidID == "" {
		kidID = id
	}
	if kidID == "" {
		return errAdoptNotFound
	}
	name := string(seen.Name)
	kids, err := household.Load(r.path)
	if err != nil {
		return err
	}
	row := reverse.Record{
		ID:       reverse.KidID(kidID),
		Name:     reverse.KidName(name),
		URL:      seen.URL,
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
	if err := r.Adopt(body.ID, body.URL); err != nil {
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
