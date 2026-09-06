package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type exportSpan struct {
	Kind      string `json:"kind"`
	Start     int    `json:"start"`
	StartUnix int64  `json:"start_unix"`
	Dur       int    `json:"dur"`
	Label     string `json:"label"`
}

func writeExport(resp *http.Response, asJSON bool) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		if _, werr := os.Stdout.Write(body); werr != nil {
			return werr
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if asJSON {
		out, err := exportTodayJSON(body)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(out)
		return err
	}
	spans, err := exportSpans(body)
	if err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, formatTodayTable(spans))
	return err
}

func exportTodayJSON(statusBody []byte) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(statusBody, &doc); err != nil {
		return nil, err
	}
	today, ok := doc["today"]
	if !ok || len(today) == 0 || string(today) == "null" {
		return []byte("[]\n"), nil
	}
	if today[len(today)-1] != '\n' {
		return append(append([]byte{}, today...), '\n'), nil
	}
	return today, nil
}

func exportSpans(statusBody []byte) ([]exportSpan, error) {
	var doc struct {
		Today []exportSpan `json:"today"`
	}
	if err := json.Unmarshal(statusBody, &doc); err != nil {
		return nil, err
	}
	if doc.Today == nil {
		return []exportSpan{}, nil
	}
	return doc.Today, nil
}

func formatTodayTable(spans []exportSpan) string {
	var b strings.Builder
	for _, s := range spans {
		label := strings.TrimSpace(s.Label)
		if label == "" {
			label = s.Kind
		}
		if label == "" {
			label = "on"
		}
		fmt.Fprintf(&b, "%s  %s  %s\n", clockExport(s), label, minutesCLI(s.Dur))
	}
	return b.String()
}

func clockExport(s exportSpan) string {
	if s.StartUnix > 0 {
		t := time.Unix(s.StartUnix, 0).Local()
		return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
	}
	return clock24Min(s.Start)
}

func clock24Min(min int) string {
	t := min % 1440
	if t < 0 {
		t += 1440
	}
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

func minutesCLI(n int) string {
	if n < 0 {
		n = 0
	}
	if n >= 60 {
		h := n / 60
		m := n % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", n)
}
