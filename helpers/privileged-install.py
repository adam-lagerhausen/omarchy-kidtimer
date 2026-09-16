#!/usr/bin/python3 -I
"""Root-side kid install. Copy reviewed bytes from held FDs, never reopen user paths."""

import argparse
import hashlib
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile

MAX_FILE = 64 * 1024 * 1024
CHUNK = 1024 * 1024
DIGEST = re.compile(r"[0-9a-f]{64}")
REL = re.compile(r"[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*")


def fail(msg):
    print(msg, file=sys.stderr)
    raise SystemExit(1)


def write_all(fd, data):
    view = memoryview(data)
    while view:
        n = os.write(fd, view)
        if n <= 0:
            fail("short write")
        view = view[n:]


def parse_manifest(text):
    out = {}
    if len(text) > 1_000_000:
        fail("manifest too large")
    for raw in text.splitlines():
        line = raw.strip()
        if not line:
            continue
        parts = line.split(None, 1)
        if len(parts) != 2:
            fail("bad manifest line")
        digest, rel = parts
        if not DIGEST.fullmatch(digest):
            fail("bad digest")
        if rel != "kidtimer" and not REL.fullmatch(rel):
            fail("bad path %s" % rel)
        if rel in out:
            fail("duplicate path %s" % rel)
        out[rel] = digest
    if "kidtimer" not in out:
        fail("manifest missing kidtimer")
    return out


def expect_uid():
    raw = os.environ.get("SUDO_UID")
    if raw:
        try:
            return int(raw)
        except ValueError:
            fail("bad SUDO_UID")
    return os.getuid()


def check_source(st, uid):
    if not stat.S_ISREG(st.st_mode):
        fail("not a regular file")
    if st.st_uid != uid:
        fail("source owner mismatch")
    if st.st_mode & 0o022:
        fail("source is group/other writable")
    if st.st_nlink != 1:
        fail("source has extra hard links")


def open_under(dirfd, rel, uid):
    parts = rel.split("/")
    if not parts or any(p in ("", ".", "..") for p in parts):
        fail("bad path %s" % rel)
    fd = dirfd
    try:
        for i, name in enumerate(parts):
            flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC
            if i < len(parts) - 1:
                flags |= os.O_DIRECTORY
            nfd = os.open(name, flags, dir_fd=fd)
            if fd != dirfd:
                os.close(fd)
            fd = nfd
        st = os.fstat(fd)
        check_source(st, uid)
        return fd, st
    except BaseException:
        if fd != dirfd:
            os.close(fd)
        raise


def open_bin(path, uid):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        st = os.fstat(fd)
        check_source(st, uid)
        return fd, st
    except BaseException:
        os.close(fd)
        raise


def copy_hashed(in_fd, out_fd, want):
    h = hashlib.sha256()
    total = 0
    while True:
        chunk = os.read(in_fd, CHUNK)
        if not chunk:
            break
        total += len(chunk)
        if total > MAX_FILE:
            fail("file too large")
        h.update(chunk)
        write_all(out_fd, chunk)
    if h.hexdigest() != want:
        fail("digest mismatch")


def hash_fd(fd):
    h = hashlib.sha256()
    total = 0
    while True:
        chunk = os.read(fd, CHUNK)
        if not chunk:
            break
        total += len(chunk)
        if total > MAX_FILE:
            fail("file too large")
        h.update(chunk)
    return h.hexdigest()


def file_mode(st):
    if st.st_mode & 0o111:
        return 0o755
    return 0o644


def write_stage_file(stage, rel, in_fd, st, want):
    dest = os.path.join(stage, rel)
    os.makedirs(os.path.dirname(dest), mode=0o700, exist_ok=True)
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC
    out = os.open(dest, flags, 0o600)
    try:
        copy_hashed(in_fd, out, want)
        os.fchmod(out, file_mode(st))
        os.fsync(out)
    finally:
        os.close(out)


def stage_sources(src, bin_path, manifest, uid):
    stage = tempfile.mkdtemp(prefix="kidtimer-stage-")
    os.chmod(stage, 0o700)
    src_fd = os.open(src, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for rel, want in manifest.items():
            if rel == "kidtimer":
                fd, st = open_bin(bin_path, uid)
            else:
                fd, st = open_under(src_fd, rel, uid)
            try:
                write_stage_file(stage, rel, fd, st, want)
            finally:
                os.close(fd)
    except BaseException:
        os.close(src_fd)
        shutil.rmtree(stage, ignore_errors=True)
        raise
    os.close(src_fd)
    return stage


def lstat_or_none(path):
    try:
        return os.lstat(path)
    except FileNotFoundError:
        return None


def remove_path(path, require_root_owner):
    st = lstat_or_none(path)
    if st is None:
        return
    if stat.S_ISLNK(st.st_mode):
        fail("%s is a symlink" % path)
    if require_root_owner and (st.st_uid != 0 or st.st_gid != 0):
        fail("%s is not root-owned" % path)
    if stat.S_ISDIR(st.st_mode):
        shutil.rmtree(path)
        return
    if not stat.S_ISREG(st.st_mode):
        fail("%s is not a regular file" % path)
    os.unlink(path)


def chown_root(path):
    if os.geteuid() == 0:
        os.chown(path, 0, 0)


def install_tree(stage, dest, require_root_owner):
    remove_path(dest, require_root_owner)
    os.makedirs(dest, mode=0o755, exist_ok=False)
    os.chmod(dest, 0o755)
    chown_root(dest)
    for dirpath, dirnames, filenames in os.walk(stage, followlinks=False):
        rel = os.path.relpath(dirpath, stage)
        for d in list(dirnames):
            srcd = os.path.join(dirpath, d)
            if os.path.islink(srcd):
                fail("symlink in stage")
            sub = d if rel == "." else os.path.join(rel, d)
            outd = os.path.join(dest, sub)
            os.mkdir(outd, 0o755)
            os.chmod(outd, 0o755)
            chown_root(outd)
        for name in filenames:
            srcf = os.path.join(dirpath, name)
            if os.path.islink(srcf):
                fail("symlink in stage")
            out_rel = name if rel == "." else os.path.join(rel, name)
            if out_rel == "kidtimer":
                continue
            dstf = os.path.join(dest, out_rel)
            shutil.copyfile(srcf, dstf, follow_symlinks=False)
            mode = 0o755 if os.stat(srcf).st_mode & 0o111 else 0o644
            os.chmod(dstf, mode)
            chown_root(dstf)


def install_bin(stage, dest, require_root_owner):
    src = os.path.join(stage, "kidtimer")
    parent = os.path.dirname(dest)
    os.makedirs(parent, mode=0o755, exist_ok=True)
    tmp = dest + ".new"
    remove_path(tmp, require_root_owner=False)
    shutil.copyfile(src, tmp, follow_symlinks=False)
    os.chmod(tmp, 0o755)
    chown_root(tmp)
    fd = os.open(tmp, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)
    remove_path(dest, require_root_owner)
    os.rename(tmp, dest)
    chown_root(dest)


def verify_installed(dest_bin, dest_share, manifest):
    root = os.geteuid() == 0
    for rel, want in manifest.items():
        if rel == "kidtimer":
            path = dest_bin
        else:
            path = os.path.join(dest_share, rel)
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
        try:
            st = os.fstat(fd)
            if not stat.S_ISREG(st.st_mode):
                fail("installed %s is not a regular file" % rel)
            if root and (st.st_uid != 0 or st.st_gid != 0):
                fail("installed %s is not root-owned" % rel)
            got = hash_fd(fd)
        finally:
            os.close(fd)
        if got != want:
            fail("installed digest mismatch for %s" % rel)


def main():
    os.umask(0o077)
    p = argparse.ArgumentParser(add_help=False)
    p.add_argument("--src", required=True)
    p.add_argument("--bin", required=True)
    p.add_argument("--dest-bin", required=True)
    p.add_argument("--dest-share", required=True)
    p.add_argument("--expect-uid", type=int)
    p.add_argument("--run-setup", action="store_true")
    p.add_argument("--allow-unprivileged", action="store_true")
    args = p.parse_args()
    if os.geteuid() != 0 and not args.allow_unprivileged:
        fail("must run as root")
    uid = args.expect_uid if args.expect_uid is not None else expect_uid()
    text = os.environ.get("KIDTIMER_INSTALL_MANIFEST", "")
    if not text.strip():
        fail("missing KIDTIMER_INSTALL_MANIFEST")
    manifest = parse_manifest(text)
    require_root = os.geteuid() == 0
    old_umask = os.umask(0o077)
    try:
        stage = stage_sources(args.src, args.bin, manifest, uid)
    finally:
        os.umask(old_umask)
    try:
        install_tree(stage, args.dest_share, require_root)
        install_bin(stage, args.dest_bin, require_root)
        verify_installed(args.dest_bin, args.dest_share, manifest)
    finally:
        shutil.rmtree(stage, ignore_errors=True)
    if not args.run_setup:
        return
    os.umask(0o022)
    r = subprocess.run(
        [args.dest_bin, "setup", "kid", "-repo", args.dest_share],
        check=False,
    )
    raise SystemExit(r.returncode)


if __name__ == "__main__":
    main()
