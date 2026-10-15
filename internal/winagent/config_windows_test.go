package winagent

import (
	"os"
	"path/filepath"
	"testing"

	"sma/internal/securefile"
)

func TestProtectedConfigurationRoundTrip(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	if err := os.MkdirAll(DataDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := securefile.Protect(DataDir(), true); err != nil {
		t.Fatal(err)
	}
	c := Config{ListenAddress: "127.0.0.1:9108", AuthTokenFile: filepath.Join(DataDir(), "token")}
	if err := SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	c.ListenAddress = "0.0.0.0:9108"
	if err := SaveConfig(c); err != nil {
		t.Fatal("replace existing config:", err)
	}
	actual, err := ReadConfig()
	if err != nil || actual != c {
		t.Fatalf("round trip: %+v %v", actual, err)
	}
	if err := securefile.Check(ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if err := WriteProtected(c.AuthTokenFile, []byte("test-only-secret")); err != nil {
		t.Fatal(err)
	}
	if err := securefile.Check(c.AuthTokenFile); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(), []byte(`{"listenAddress":"127.0.0.1:9108","authTokenFile":"C:\\token","unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(); err == nil {
		t.Fatal("unknown configuration fields must be rejected")
	}
}
