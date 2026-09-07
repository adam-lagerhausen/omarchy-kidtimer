#!/usr/bin/python3 -I
import json
import os
import pwd
import re
import secrets
import stat
import sys

MAX_BYTES = 65536
_COMPONENT = re.compile(r"[A-Za-z0-9._-]+")
_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}")
SHARE = (".local", "share", "kidtimer")
ALLOWED = {
    "role",
    "kids.json",
    "prefs.json",
    "kid-bar.json",
    "setup-error",
}


def _ok_component(name):
    return bool(_COMPONENT.fullmatch(name)) and name not in (".", "..")


def open_dir_chain(parts):
    if not parts or not all(_ok_component(p) for p in parts):
        raise PermissionError("refusing directory chain")
    home = pwd.getpwuid(os.geteuid()).pw_dir
    fd = os.open(home, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for i, name in enumerate(parts):
            try:
                nfd = os.open(
                    name,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                    dir_fd=fd,
                )
            except FileNotFoundError:
                try:
                    os.mkdir(name, 0o700, dir_fd=fd)
                except FileExistsError:
                    pass
                nfd = os.open(
                    name,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                    dir_fd=fd,
                )
            os.close(fd)
            fd = nfd
            st = os.fstat(fd)
            if not stat.S_ISDIR(st.st_mode) or st.st_uid != os.geteuid():
                raise PermissionError(f"untrusted directory component {name}")
            if i == len(parts) - 1 and st.st_mode & 0o077:
                os.fchmod(fd, 0o700)
        return fd
    except BaseException:
        os.close(fd)
        raise


def read_bounded(dirfd, name):
    try:
        fd = os.open(
            name,
            os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC,
            dir_fd=dirfd,
        )
    except FileNotFoundError:
        return None
    try:
        st = os.fstat(fd)
        if (
            not stat.S_ISREG(st.st_mode)
            or st.st_uid != os.geteuid()
            or st.st_nlink != 1
            or st.st_mode & 0o077
            or st.st_size > MAX_BYTES
        ):
            raise PermissionError("refusing state file (expected 0600, owner-only, one link)")
        os.set_blocking(fd, True)
        data = b""
        while len(data) <= MAX_BYTES:
            chunk = os.read(fd, min(65536, MAX_BYTES + 1 - len(data)))
            if not chunk:
                break
            data += chunk
        if len(data) > MAX_BYTES:
            raise PermissionError("state file grew past the limit")
        return data
    finally:
        os.close(fd)


def write_atomic(dirfd, name, data: bytes):
    if len(data) > MAX_BYTES:
        raise ValueError("payload too large")
    tmp = f".{name}.{secrets.token_hex(8)}.tmp"
    fd = os.open(
        tmp,
        os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
        0o600,
        dir_fd=dirfd,
    )
    try:
        os.fchmod(fd, 0o600)
        view = memoryview(data)
        while view:
            n = os.write(fd, view)
            view = view[n:]
        os.fsync(fd)
        os.rename(tmp, name, src_dir_fd=dirfd, dst_dir_fd=dirfd)
        os.fsync(dirfd)
    except BaseException:
        try:
            os.unlink(tmp, dir_fd=dirfd)
        except OSError:
            pass
        raise
    finally:
        os.close(fd)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(2)
    op, name = sys.argv[1], sys.argv[2]
    if not _NAME.fullmatch(name) or name in (".", "..") or name not in ALLOWED:
        sys.exit(2)
    dirfd = open_dir_chain(SHARE)
    try:
        if op == "read":
            raw = read_bounded(dirfd, name)
            sys.stdout.buffer.write(raw if raw else b"")
        elif op == "write":
            payload = sys.stdin.buffer.read(MAX_BYTES + 1)
            if len(payload) > MAX_BYTES:
                sys.exit(3)
            json.loads(payload)
            write_atomic(dirfd, name, payload)
        else:
            sys.exit(2)
    finally:
        os.close(dirfd)
