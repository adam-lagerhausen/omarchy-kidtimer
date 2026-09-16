package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

type stayAwake struct {
	ours bool
	path string
	uid  int
	gid  int
}

func (s *stayAwake) Sync(uid int, overlay bool) {
	if uid <= 0 {
		if s.ours {
			s.release()
		}
		return
	}
	if s.path == "" || s.uid != uid {
		home, err := userHome(uid)
		if err != nil {
			return
		}
		s.path = filepath.Join(home, ".local", "state", "omarchy", "indicators", "stay-awake")
		s.uid = uid
		s.gid = userGid(uid)
	}
	if overlay {
		s.hold()
		return
	}
	s.release()
}

func (s *stayAwake) hold() {
	if s.path == "" {
		return
	}
	if _, err := os.Lstat(s.path); err == nil {
		if !s.ours {
			return
		}
		return
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if s.uid > 0 {
		_ = os.Chown(dir, s.uid, s.gid)
	}
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return
	}
	_ = f.Close()
	if s.uid > 0 {
		_ = os.Chown(s.path, s.uid, s.gid)
	}
	s.ours = true
}

func (s *stayAwake) release() {
	if !s.ours || s.path == "" {
		s.ours = false
		return
	}
	_ = os.Remove(s.path)
	s.ours = false
}

func userHome(uid int) (string, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func userGid(uid int) int {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return uid
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return uid
	}
	return gid
}
