package desk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

const scanInterval = 3 * time.Second

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
	for _, f := range found {
		_ = r.Claim(ctx, f)
	}
}

func (r *Registry) Claim(ctx context.Context, f advertise.Found) error {
	if !advertise.URLHousehold(f.URL) {
		return nil
	}
	kids, err := household.Load(r.path)
	if err != nil {
		return err
	}
	if f.ID != "" {
		if rec, ok := household.Lookup(kids, reverse.KidID(f.ID)); ok {
			rec.URL = f.URL
			if f.Name != "" {
				rec.Name = reverse.KidName(f.Name)
			}
			return household.Save(r.path, household.Upsert(kids, rec))
		}
	}
	for _, k := range kids {
		if k.URL == f.URL && k.Token != "" {
			return nil
		}
	}
	pair := r.pair
	if pair == nil {
		pair = pairKid
	}
	pr, err := pair(ctx, f.URL)
	if pr.Status == http.StatusConflict {
		for _, k := range kids {
			if k.Token == "" {
				continue
			}
			if statusOK(ctx, f.URL, k.Token) {
				k.URL = f.URL
				if f.Name != "" {
					k.Name = reverse.KidName(f.Name)
				}
				return household.Save(r.path, household.Upsert(kids, k))
			}
		}
		return nil
	}
	if err != nil {
		return err
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
		return fmt.Errorf("pair: missing id or token")
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
		return err
	}
	if err := household.Save(r.path, household.Upsert(kids, row)); err != nil {
		return err
	}
	go r.pushPin(reverse.KidID(id))
	return nil
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
