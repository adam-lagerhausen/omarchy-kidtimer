package household

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kidtimer/daemon/internal/reverse"
)

func Path(home string) string {
	return filepath.Join(home, "kids.json")
}

func LegacyDir(dir string) string {
	return filepath.Join(filepath.Dir(dir), "allowance")
}

func HasUsableToken(kids []reverse.Record) bool {
	for _, k := range kids {
		if k.Token != "" {
			return true
		}
	}
	return false
}

func ImportIfEmpty(dir string) error {
	kids, err := Load(Path(dir))
	if err != nil {
		return err
	}
	if HasUsableToken(kids) {
		return nil
	}
	legacy := LegacyDir(dir)
	old, err := Load(Path(legacy))
	if err != nil {
		return err
	}
	if !HasUsableToken(old) {
		return nil
	}
	if err := Save(Path(dir), old); err != nil {
		return err
	}
	if _, ok, err := ReadPin(dir); err != nil || ok {
		return err
	}
	pin, ok, err := ReadPin(legacy)
	if err != nil || !ok {
		return err
	}
	return WritePin(dir, pin)
}

func PinPath(home string) string {
	return filepath.Join(home, "parent-pin")
}

func RolePath(home string) string {
	return filepath.Join(home, "role")
}

func LoadRole(home string) (reverse.Role, error) {
	b, err := os.ReadFile(RolePath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return reverse.RoleNone, nil
		}
		return reverse.RoleNone, err
	}
	switch strings.TrimSpace(string(b)) {
	case "parent":
		return reverse.RoleParent, nil
	case "kid":
		return reverse.RoleKid, nil
	default:
		return reverse.RoleNone, nil
	}
}

func WriteRole(home string, role reverse.Role) error {
	var s string
	switch role {
	case reverse.RoleParent:
		s = "parent"
	case reverse.RoleKid:
		s = "kid"
	default:
		return nil
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	return writeFile(RolePath(home), []byte(s+"\n"))
}

func Load(path string) ([]reverse.Record, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return Adopt(b)
}

func Save(path string, kids []reverse.Record) error {
	if kids == nil {
		kids = []reverse.Record{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(reverse.File{Kids: kids}, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, append(raw, '\n'))
}

func WritePin(home, encoded string) error {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	return writeFile(PinPath(home), []byte(strings.TrimSpace(encoded)+"\n"))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func ReadPin(home string) (string, bool, error) {
	b, err := os.ReadFile(PinPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

func Upsert(kids []reverse.Record, row reverse.Record) []reverse.Record {
	for i, k := range kids {
		if k.ID != "" && k.ID == row.ID {
			if row.Name != "" {
				kids[i].Name = row.Name
			}
			if row.URL != "" {
				kids[i].URL = row.URL
			}
			if row.Token != "" {
				kids[i].Token = row.Token
			}
			if row.TicketHash != "" {
				kids[i].TicketHash = row.TicketHash
			}
			if !row.PairedAt.IsZero() {
				kids[i].PairedAt = row.PairedAt
			}
			return kids
		}
	}
	return append(kids, row)
}

func TokenRows(groups ...[]reverse.Record) []reverse.Record {
	seen := map[string]bool{}
	var out []reverse.Record
	for _, kids := range groups {
		for _, k := range kids {
			if k.Token == "" || seen[k.Token] {
				continue
			}
			seen[k.Token] = true
			out = append(out, k)
		}
	}
	return out
}

func Lookup(kids []reverse.Record, id reverse.KidID) (reverse.Record, bool) {
	for _, k := range kids {
		if k.ID == id {
			return k, true
		}
	}
	return reverse.Record{}, false
}

type oldRow struct {
	ID         reverse.KidID      `json:"id"`
	Name       reverse.KidName    `json:"name"`
	URL        string             `json:"url"`
	Token      string             `json:"token"`
	TicketHash reverse.TicketHash `json:"ticket_hash"`
	PairedAt   time.Time          `json:"paired_at"`
}

func Adopt(raw []byte) ([]reverse.Record, error) {
	var f struct {
		Kids []oldRow `json:"kids"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	out := make([]reverse.Record, 0, len(f.Kids))
	for _, row := range f.Kids {
		if row.ID == "" {
			continue
		}
		out = append(out, reverse.Record{
			ID:         row.ID,
			Name:       row.Name,
			URL:        row.URL,
			Token:      row.Token,
			TicketHash: row.TicketHash,
			PairedAt:   row.PairedAt,
		})
	}
	return out, nil
}
