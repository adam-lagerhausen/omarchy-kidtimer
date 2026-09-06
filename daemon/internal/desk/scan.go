package desk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

const scanInterval = 3 * time.Second

var errAdoptNotFound = errors.New("not found")

type pairResult struct {
	ID     string
	Name   string
	Token  string
	Status int
}

func (r *Registry) scanLoop(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.scanTick(ctx)
			t.Reset(scanInterval)
		}
	}
}

func (r *Registry) scanTick(ctx context.Context) {
	var found []advertise.Found
	seen := map[string]bool{}
	add := func(f advertise.Found) {
		if f.URL == "" {
			return
		}
		key := f.ID
		if key == "" {
			key = f.URL
		}
		if seen[key] {
			return
		}
		seen[key] = true
		found = append(found, f)
	}

	bctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ch := make(chan advertise.Found, 16)
	done := make(chan struct{})
	go func() {
		browse := r.browse
		if browse == nil {
			browse = advertise.Browse
		}
		_ = browse(bctx, ch)
		close(done)
	}()
	for {
		select {
		case f := <-ch:
			add(f)
		case <-done:
			for {
				select {
				case f := <-ch:
					add(f)
				default:
					goto probes
				}
			}
		case <-bctx.Done():
			goto probes
		}
	}
probes:
	probe := r.probe
	if probe == nil {
		probe = householdKidURLs
	}
	urls := probe(ctx)
	var live []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if liveKid(ctx, u) {
				mu.Lock()
				live = append(live, u)
				mu.Unlock()
			}
		}(u)
	}
	wg.Wait()
	for _, u := range live {
		add(advertise.Found{URL: u})
	}
	var elsewhere []reverse.Seen
	for _, f := range found {
		claimed, err := r.Claim(ctx, f)
		if err == nil && claimed {
			elsewhere = append(elsewhere, seenFromFound(f))
		}
	}
	r.replaceSeen(elsewhere)
}

func (r *Registry) replaceSeen(seen []reverse.Seen) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = seen
}

func (r *Registry) Claim(ctx context.Context, f advertise.Found) (claimedElsewhere bool, err error) {
	if !advertise.URLHousehold(f.URL) {
		return false, nil
	}
	kids, err := household.Load(r.path)
	if err != nil {
		return false, err
	}
	if f.ID != "" {
		if rec, ok := household.Lookup(kids, reverse.KidID(f.ID)); ok && rec.Token != "" {
			rec.URL = f.URL
			if f.Name != "" {
				rec.Name = reverse.KidName(f.Name)
			}
			return false, household.Save(r.path, household.Upsert(kids, rec))
		}
	}
	for _, k := range kids {
		if k.URL == f.URL && k.Token != "" {
			return false, nil
		}
	}
	pair := r.pair
	if pair == nil {
		pair = pairKid
	}
	pr, err := pair(ctx, f.URL)
	if pr.Status == http.StatusConflict {
		for _, k := range r.pairTokens(kids) {
			if statusOK(ctx, f.URL, k.Token) {
				k.URL = f.URL
				if f.Name != "" {
					k.Name = reverse.KidName(f.Name)
				}
				return false, household.Save(r.path, household.Upsert(kids, k))
			}
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	id := f.ID
	if id == "" {
		id = pr.ID
	}
	name := f.Name
	if name == "" {
		name = pr.Name
	}
	if id == "" || pr.Token == "" {
		return false, fmt.Errorf("pair: missing id or token")
	}
	row := reverse.Record{
		ID:       reverse.KidID(id),
		Name:     reverse.KidName(name),
		URL:      f.URL,
		Token:    pr.Token,
		PairedAt: time.Now().UTC(),
	}
	kids, err = household.Load(r.path)
	if err != nil {
		return false, err
	}
	if err := household.Save(r.path, household.Upsert(kids, row)); err != nil {
		return false, err
	}
	go r.pushPin(reverse.KidID(id))
	return false, nil
}

func (r *Registry) pairTokens(kids []reverse.Record) []reverse.Record {
	legacy, _ := household.Load(household.Path(household.LegacyDir(filepath.Dir(r.path))))
	return household.TokenRows(kids, legacy)
}

func seenFromFound(f advertise.Found) reverse.Seen {
	name := f.Name
	if name == "" {
		if ip := advertise.HostIP(f.URL); ip != nil {
			name = ip.String()
		} else {
			name = "Kid computer"
		}
	}
	return reverse.Seen{
		ID:      reverse.KidID(f.ID),
		Name:    reverse.KidName(name),
		URL:     f.URL,
		Claimed: true,
	}
}

func pairKid(ctx context.Context, rawURL string) (pairResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(rawURL, "/")+"/v1/pair", strings.NewReader("{}"))
	if err != nil {
		return pairResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := scanHTTP().Do(req)
	if err != nil {
		return pairResult{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	out := pairResult{Status: resp.StatusCode}
	if resp.StatusCode == http.StatusConflict {
		return out, nil
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("pair: %s", resp.Status)
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

func liveKid(ctx context.Context, rawURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rawURL, "/")+"/v1/status", nil)
	if err != nil {
		return false
	}
	resp, err := scanHTTP().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return bytes.Contains(body, []byte(`"unauthorized"`))
}

func statusOK(ctx context.Context, rawURL, token string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rawURL, "/")+"/v1/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := scanHTTP().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func scanHTTP() *http.Client {
	return &http.Client{Timeout: 1500 * time.Millisecond}
}

func reclaimKid(ctx context.Context, rawURL string) (pairResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(rawURL, "/")+"/v1/reclaim", strings.NewReader("{}"))
	if err != nil {
		return pairResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := scanHTTP().Do(req)
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
