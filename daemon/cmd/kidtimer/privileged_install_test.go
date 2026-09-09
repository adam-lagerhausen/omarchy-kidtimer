package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivilegedInstallCopiesReviewedBytes(t *testing.T) {
	src, bin, destBin, destShare := privInstallTree(t)
	manifest := privManifest(t, src, bin)
	runPrivInstall(t, src, bin, destBin, destShare, manifest, 0)
	got, err := os.ReadFile(destBin)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary-v1\n" {
		t.Fatalf("dest bin %q", got)
	}
	qml, err := os.ReadFile(filepath.Join(destShare, "BarWidget.qml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(qml) != "widget-v1\n" {
		t.Fatalf("share qml %q", qml)
	}
	if _, err := os.Stat(filepath.Join(destShare, "packaging", "config.kid.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destShare, "kidtimer")); !os.IsNotExist(err) {
		t.Fatal("binary must not land in the share tree")
	}
}

func TestPrivilegedInstallRejectsDigestMismatch(t *testing.T) {
	src, bin, destBin, destShare := privInstallTree(t)
	manifest := privManifest(t, src, bin)
	if err := os.WriteFile(filepath.Join(src, "BarWidget.qml"), []byte("swapped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runPrivInstallErr(t, src, bin, destBin, destShare, manifest)
	if !strings.Contains(out, "digest mismatch") {
		t.Fatalf("want digest mismatch, got %s", out)
	}
	if _, err := os.Stat(destBin); !os.IsNotExist(err) {
		t.Fatal("dest bin written after mismatch")
	}
	if _, err := os.Stat(destShare); !os.IsNotExist(err) {
		t.Fatal("dest share written after mismatch")
	}
}

func TestPrivilegedInstallRejectsSymlink(t *testing.T) {
	src, bin, destBin, destShare := privInstallTree(t)
	manifest := privManifest(t, src, bin)
	qml := filepath.Join(src, "BarWidget.qml")
	target := filepath.Join(src, "payload")
	if err := os.WriteFile(target, []byte("evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(qml); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, qml); err != nil {
		t.Fatal(err)
	}
	out := runPrivInstallErr(t, src, bin, destBin, destShare, manifest)
	if !strings.Contains(out, "not a regular file") && !strings.Contains(strings.ToLower(out), "too many") && !strings.Contains(out, "No such file") {
		t.Fatalf("want symlink reject, got %s", out)
	}
}

func TestPrivilegedInstallRejectsGroupWritable(t *testing.T) {
	src, bin, destBin, destShare := privInstallTree(t)
	if err := os.Chmod(filepath.Join(src, "BarWidget.qml"), 0o664); err != nil {
		t.Fatal(err)
	}
	manifest := privManifest(t, src, bin)
	out := runPrivInstallErr(t, src, bin, destBin, destShare, manifest)
	if !strings.Contains(out, "group/other writable") {
		t.Fatalf("want writable reject, got %s", out)
	}
}

func TestPrivilegedInstallRejectsHardLink(t *testing.T) {
	src, bin, destBin, destShare := privInstallTree(t)
	qml := filepath.Join(src, "BarWidget.qml")
	if err := os.Link(qml, filepath.Join(src, "BarWidget.link")); err != nil {
		t.Fatal(err)
	}
	manifest := privManifest(t, src, bin)
	out := runPrivInstallErr(t, src, bin, destBin, destShare, manifest)
	if !strings.Contains(out, "hard links") {
		t.Fatalf("want hard link reject, got %s", out)
	}
}

func TestPrivilegedInstallRunsSetupAfterVerify(t *testing.T) {
	src, _, destBin, destShare := privInstallTree(t)
	marker := filepath.Join(t.TempDir(), "setup-ran")
	bin := filepath.Join(src, "fake-bin")
	script := "#!/bin/bash\nprintf '%s\\n' \"$*\" >" + marker + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := privManifest(t, src, bin)
	runPrivInstall(t, src, bin, destBin, destShare, manifest, 1)
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	want := "setup kid -repo " + destShare + "\n"
	if string(got) != want {
		t.Fatalf("setup argv %q", got)
	}
}

func TestInstallLibHashesBeforeSudo(t *testing.T) {
	lib := readPlugin(t, repoRoot(t), "helpers/install-lib.sh")
	mustContain(t, lib, "sha256sum", "digest manifest")
	mustContain(t, lib, "KIDTIMER_INSTALL_MANIFEST", "pass digest to root")
	mustContain(t, lib, "privileged-install.py", "root-side installer")
	mustContain(t, lib, `/usr/bin/python3 -I -S -`, "python from stdin")
	mustContain(t, lib, `printf '%s\n' "$installer" | /usr/bin/sudo`, "pipe installer, do not reopen path")
	if strings.Contains(lib, "sudo /usr/bin/install") || strings.Contains(lib, "sudo /usr/bin/cp") {
		t.Fatal("must not sudo install/cp")
	}
	if strings.Contains(lib, "mktemp") {
		t.Fatal("must not stage in user mktemp")
	}
	py := readPlugin(t, repoRoot(t), "helpers/privileged-install.py")
	mustContain(t, py, "O_NOFOLLOW", "O_NOFOLLOW")
	mustContain(t, py, "S_ISREG", "regular-file check")
	mustContain(t, py, "source owner mismatch", "owner check")
	mustContain(t, py, "group/other writable", "mode check")
	mustContain(t, py, "tempfile.mkdtemp", "root-owned stage")
	mustContain(t, py, "installed digest mismatch", "verify before exec")
	mustContain(t, py, "os.fstat", "held descriptor stat")
}

func TestKidInstallPathsSharePrivilegedCopy(t *testing.T) {
	apply := readPlugin(t, repoRoot(t), "helpers/apply-role.sh")
	pack := readPlugin(t, repoRoot(t), "packaging/install.sh")
	for name, src := range map[string]string{"apply-role.sh": apply, "install.sh": pack} {
		if !strings.Contains(src, "kidtimer_run_privileged_install") {
			t.Fatalf("%s missing privileged install", name)
		}
		if strings.Contains(src, "sudo /usr/bin/install") || strings.Contains(src, "sudo /usr/bin/cp") || strings.Contains(src, "sudo install") || strings.Contains(src, "sudo cp") {
			t.Fatalf("%s still sudo-copies from a user path", name)
		}
	}
}

func privInstallTree(t *testing.T) (src, bin, destBin, destShare string) {
	t.Helper()
	root := t.TempDir()
	src = filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "packaging"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"manifest.json":               `{"id":"io.github.adam-lagerhausen.kidtimer"}` + "\n",
		"BarWidget.qml":               "widget-v1\n",
		"packaging/config.kid.toml":   "kid_name = \"x\"\n",
		"packaging/kidtimer.service":  "[Service]\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(src, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin = filepath.Join(src, "kidtimer.bin")
	if err := os.WriteFile(bin, []byte("binary-v1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	destBin = filepath.Join(dest, "bin", "kidtimer")
	destShare = filepath.Join(dest, "share")
	return src, bin, destBin, destShare
}

func privManifest(t *testing.T, src, bin string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(fileDigest(t, bin))
	b.WriteString("  kidtimer\n")
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if path == bin {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "kidtimer.bin" {
			return nil
		}
		b.WriteString(fileDigest(t, path))
		b.WriteString("  ")
		b.WriteString(rel)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func TestInstallLibManifestListsPluginFiles(t *testing.T) {
	src, bin, _, _ := privInstallTree(t)
	cmd := exec.Command("/usr/bin/bash", "-c", `
set -euo pipefail
. "$1"
kidtimer_manifest "$2" "$3"
`, "manifest", filepath.Join(repoRoot(t), "helpers", "install-lib.sh"), src, bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("manifest: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "  kidtimer\n") {
		t.Fatalf("missing binary digest: %s", got)
	}
	if !strings.Contains(got, "  BarWidget.qml\n") {
		t.Fatalf("missing qml: %s", got)
	}
	if !strings.Contains(got, "  packaging/config.kid.toml\n") {
		t.Fatalf("missing packaging: %s", got)
	}
	if strings.Contains(got, "kidtimer.bin") {
		t.Fatal("must name the binary kidtimer, not the source filename")
	}
}

func runPrivInstall(t *testing.T, src, bin, destBin, destShare, manifest string, runSetup int) {
	t.Helper()
	out, err := privInstallCmd(t, src, bin, destBin, destShare, manifest, runSetup).CombinedOutput()
	if err != nil {
		t.Fatalf("privileged-install: %v\n%s", err, out)
	}
}

func runPrivInstallErr(t *testing.T, src, bin, destBin, destShare, manifest string) string {
	t.Helper()
	out, err := privInstallCmd(t, src, bin, destBin, destShare, manifest, 0).CombinedOutput()
	if err == nil {
		t.Fatalf("expected fail: %s", out)
	}
	return string(out)
}

func privInstallCmd(t *testing.T, src, bin, destBin, destShare, manifest string, runSetup int) *exec.Cmd {
	t.Helper()
	args := []string{"-I", "-S", filepath.Join(repoRoot(t), "helpers", "privileged-install.py"),
		"--src", src,
		"--bin", bin,
		"--dest-bin", destBin,
		"--dest-share", destShare,
		"--allow-unprivileged",
	}
	if runSetup != 0 {
		args = append(args, "--run-setup")
	}
	cmd := exec.Command("/usr/bin/python3", args...)
	cmd.Env = append(os.Environ(), "KIDTIMER_INSTALL_MANIFEST="+manifest)
	return cmd
}
