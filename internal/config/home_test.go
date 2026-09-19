package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHomeUsesINFERENESIA_HOMEOverride(t *testing.T) {
	t.Setenv(EnvHome, "/tmp/inferenesia-test-home-override")
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	want := filepath.Clean("/tmp/inferenesia-test-home-override")
	if got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
}

func TestHomeUsesLegacyYURA_AI_HOMEOverride(t *testing.T) {
	t.Setenv(EnvHome, "")
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "/tmp/inferenesia-legacy-home-override")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	want := filepath.Clean("/tmp/inferenesia-legacy-home-override")
	if got != want {
		t.Fatalf("Home() = %q, want %q (legacy env)", got, want)
	}
}

func TestHomePrefersNewEnvOverLegacy(t *testing.T) {
	t.Setenv(EnvHome, "/tmp/inferenesia-pref")
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "/tmp/inferenesia-legacy-should-lose")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	want := filepath.Clean("/tmp/inferenesia-pref")
	if got != want {
		t.Fatalf("Home() = %q, want new env %q (legacy must lose)", got, want)
	}
}

func TestHomeIgnoresWhitespaceOnlyOverride(t *testing.T) {
	t.Setenv(EnvHome, "   ")
	t.Setenv(EnvHomeTypo, "   ")
	t.Setenv(EnvHomeLegacy, "   ")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	userHome, _ := os.UserHomeDir()
	preferred := filepath.Join(userHome, DefaultHomeDirName)
	typo := filepath.Join(userHome, TypoHomeDirName)
	want := preferred
	if st, err := os.Stat(preferred); err != nil || !st.IsDir() {
		if st2, err2 := os.Stat(typo); err2 == nil && st2.IsDir() {
			want = typo
		}
	}
	if got != want {
		t.Fatalf("Home() = %q, want %q (whitespace env ignored)", got, want)
	}
}

func TestHomeDefaultsToInferenesiaDir(t *testing.T) {
	t.Setenv(EnvHome, "")
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	preferred := filepath.Join(userHome, DefaultHomeDirName)
	typo := filepath.Join(userHome, TypoHomeDirName)
	want := preferred
	if st, err := os.Stat(preferred); err != nil || !st.IsDir() {
		if st2, err2 := os.Stat(typo); err2 == nil && st2.IsDir() {
			want = typo
		}
	}
	if got != want {
		t.Fatalf("Home() = %q, want %q (preferred %q, typo dual-read %q)", got, want, preferred, typo)
	}
}

func TestHomeResolvesRelativeOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv(EnvHome, "rel-inferenesia-home")
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	want := filepath.Join(tmp, "rel-inferenesia-home")
	if got != want {
		t.Fatalf("Home() = %q, want abs %q", got, want)
	}
}

func TestConfigPathJoinsHomeAndConfigFile(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "cfg-home")
	t.Setenv(EnvHome, home)
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")
	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := filepath.Join(home, ConfigFileName)
	if got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestEnsureHomeCreatesDirAndConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "inferenesia-home")
	t.Setenv(EnvHome, home)
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")

	got, err := EnsureHome()
	if err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	if got != home {
		t.Fatalf("EnsureHome path = %q, want %q", got, home)
	}

	info, err := os.Stat(home)
	if err != nil {
		t.Fatalf("stat home: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("home is not a directory")
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != DirPerm {
			t.Fatalf("home mode = %o, want %o", mode, DirPerm)
		}
	}

	cfgPath := filepath.Join(home, ConfigFileName)
	cfgInfo, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config.yaml: %v", err)
	}
	if runtime.GOOS != "windows" && cfgInfo.Mode().Perm() != FilePerm {
		t.Fatalf("config.yaml mode = %o, want %o", cfgInfo.Mode().Perm(), FilePerm)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("config.yaml is empty")
	}
	if !strings.Contains(string(data), "Inferenesia") {
		t.Fatalf("default config missing brand comment; got:\n%s", data)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Fatalf("default config missing version; got:\n%s", data)
	}
}

func TestEnsureHomeCreatesConfigWhenDirAlreadyExists(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "preexisting")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv(EnvHome, home)
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")

	if _, err := EnsureHome(); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(home)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode != DirPerm {
			t.Fatalf("home mode after EnsureHome = %o, want %o", mode, DirPerm)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ConfigFileName)); err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}
}

func TestEnsureHomeIdempotentDoesNotOverwriteConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "inferenesia-home")
	t.Setenv(EnvHome, home)
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")

	if _, err := EnsureHome(); err != nil {
		t.Fatalf("first EnsureHome: %v", err)
	}
	cfgPath := filepath.Join(home, ConfigFileName)
	custom := []byte("# custom\nversion: 99\n")
	if err := os.WriteFile(cfgPath, custom, FilePerm); err != nil {
		t.Fatalf("write custom: %v", err)
	}
	if _, err := EnsureHome(); err != nil {
		t.Fatalf("second EnsureHome: %v", err)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != string(custom) {
		t.Fatalf("config overwritten; got %q", data)
	}
}

// VAL-FOUND-004: override path is used; default ~/.inferenesia is never created or touched.
func TestEnsureHomeOverrideDoesNotTouchDefaultHome(t *testing.T) {
	defaultHome, err := func() (string, error) {
		prev := os.Getenv(EnvHome)
		prevLegacy := os.Getenv(EnvHomeLegacy)
		t.Setenv(EnvHome, "")
		t.Setenv(EnvHomeTypo, "")
		t.Setenv(EnvHomeLegacy, "")
		h, e := Home()
		_ = prev
		_ = prevLegacy
		return h, e
	}()
	if err != nil {
		t.Fatalf("default Home: %v", err)
	}

	var (
		defaultExisted bool
		defaultModTime int64
		defaultCfgData []byte
		defaultCfgMode os.FileMode
	)
	if info, err := os.Stat(defaultHome); err == nil {
		defaultExisted = true
		defaultModTime = info.ModTime().UnixNano()
		if cfgInfo, err := os.Stat(filepath.Join(defaultHome, ConfigFileName)); err == nil {
			defaultCfgMode = cfgInfo.Mode()
			defaultCfgData, _ = os.ReadFile(filepath.Join(defaultHome, ConfigFileName))
			_ = defaultCfgMode
		}
	}

	tmp := t.TempDir()
	override := filepath.Join(tmp, "unique-inferenesia-home")
	t.Setenv(EnvHome, override)
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")

	got, err := EnsureHome()
	if err != nil {
		t.Fatalf("EnsureHome under override: %v", err)
	}
	if got != override {
		t.Fatalf("EnsureHome = %q, want override %q", got, override)
	}

	if _, err := os.Stat(filepath.Join(override, ConfigFileName)); err != nil {
		t.Fatalf("override config missing: %v", err)
	}

	if !defaultExisted {
		if _, err := os.Stat(defaultHome); !os.IsNotExist(err) {
			if err == nil {
				t.Fatalf("default home %s was created despite override", defaultHome)
			}
		}
		return
	}

	info, err := os.Stat(defaultHome)
	if err != nil {
		t.Fatalf("default home disappeared: %v", err)
	}
	if info.ModTime().UnixNano() != defaultModTime {
		t.Logf("default home mtime changed (%d -> %d); checking config content",
			defaultModTime, info.ModTime().UnixNano())
	}
	if defaultCfgData != nil {
		now, err := os.ReadFile(filepath.Join(defaultHome, ConfigFileName))
		if err != nil {
			t.Fatalf("read default config after override EnsureHome: %v", err)
		}
		if string(now) != string(defaultCfgData) {
			t.Fatalf("default config.yaml content changed under override")
		}
	}
}

func TestEnsureHomeOnlyWritesUnderOverride(t *testing.T) {
	tmp := t.TempDir()
	sibling := filepath.Join(tmp, "not-the-home")
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	before, err := os.ReadDir(sibling)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	home := filepath.Join(tmp, "the-home")
	t.Setenv(EnvHome, home)
	t.Setenv(EnvHomeTypo, "")
	t.Setenv(EnvHomeLegacy, "")
	if _, err := EnsureHome(); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}

	after, err := os.ReadDir(sibling)
	if err != nil {
		t.Fatalf("readdir after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("sibling dir changed; before=%d after=%d", len(before), len(after))
	}
}
