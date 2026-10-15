// Package winagent owns the Windows installation layout and managed configuration.
package winagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"sma/internal/securefile"
)

const ServiceName = "SMA"

func DataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "SMA")
}
func ConfigPath() string { return filepath.Join(DataDir(), "config.json") }
func TokenPath() string  { return filepath.Join(DataDir(), "token") }
func LogPath() string    { return filepath.Join(DataDir(), "logs", "sma.log") }

type Config struct {
	ListenAddress string `json:"listenAddress"`
	AuthTokenFile string `json:"authTokenFile"`
}

func ReadConfig() (Config, error) {
	f, err := os.Open(ConfigPath())
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 65537))
	decoder.DisallowUnknownFields()
	var c Config
	if err := decoder.Decode(&c); err != nil {
		return c, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c, errors.New("unexpected trailing configuration data")
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	host, port, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return fmt.Errorf("listenAddress: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || (host != "" && net.ParseIP(host) == nil) {
		return errors.New("listenAddress must contain an IP address and port 1..65535")
	}
	if !filepath.IsAbs(c.AuthTokenFile) {
		return errors.New("authTokenFile must be an absolute path")
	}
	return nil
}
func SaveConfig(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteProtected(ConfigPath(), append(data, '\n'))
}

func WriteProtected(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".sma-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := securefile.Protect(tmp, false); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// os.Rename uses MoveFileEx(REPLACE_EXISTING) on Windows.
	return os.Rename(tmp, path)
}
