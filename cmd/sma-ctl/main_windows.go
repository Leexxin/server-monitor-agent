package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"sma/internal/buildinfo"
	"sma/internal/securefile"
	"sma/internal/winagent"
)

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Println("SMA Windows management\n  status | doctor | start | stop | restart | enable | disable | logs\n  config show | config listen IP:PORT\n  token show | token reset --yes\n  version\n  Install/upgrade: install-sma.ps1; uninstall: uninstall-sma.ps1 [-Purge] [-Yes]\nRun management commands in an elevated terminal. Tokens are shown only by token show.")
		return nil
	}
	if args[0] == "version" {
		data, _ := json.Marshal(buildinfo.Current())
		fmt.Println(string(data))
		return nil
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("administrator privileges required; open PowerShell as Administrator")
	}
	switch args[0] {
	case "provision":
		return provision()
	case "configure-recovery":
		return withService(func(s *mgr.Service) error {
			if err := s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}, {Type: mgr.ServiceRestart, Delay: 60 * time.Second}}, 86400); err != nil {
				return err
			}
			return s.SetRecoveryActionsOnNonCrashFailures(true)
		})
	case "start", "stop", "restart":
		if err := serviceAction(args[0]); err != nil {
			return err
		}
		if args[0] != "stop" {
			return doctor()
		}
		return nil
	case "enable", "disable":
		return withService(func(s *mgr.Service) error {
			c, err := s.Config()
			if err != nil {
				return err
			}
			c.StartType = mgr.StartAutomatic
			if args[0] == "disable" {
				c.StartType = mgr.StartDisabled
			}
			if err := s.UpdateConfig(c); err != nil {
				return err
			}
			fmt.Println("Startup:", args[0])
			return nil
		})
	case "status":
		return status()
	case "doctor":
		return doctor()
	case "logs":
		f, err := os.Open(winagent.LogPath())
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.Size() > 65536 {
			_, _ = f.Seek(-65536, io.SeekEnd)
		}
		_, err = io.Copy(os.Stdout, f)
		return err
	case "config":
		c, err := winagent.ReadConfig()
		if err != nil {
			return err
		}
		if len(args) == 2 && args[1] == "show" {
			data, _ := json.MarshalIndent(c, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		if len(args) != 3 || args[1] != "listen" {
			return errors.New("usage: config show | config listen IP:PORT")
		}
		old := c
		c.ListenAddress = args[2]
		if err := c.Validate(); err != nil {
			return err
		}
		if err := winagent.SaveConfig(c); err != nil {
			return err
		}
		if err := serviceAction("restart"); err != nil {
			return restoreConfig(old, err)
		}
		if err := doctor(); err != nil {
			return restoreConfig(old, err)
		}
		fmt.Println("Listen address updated. Configure a scoped firewall rule separately for remote access.")
		return nil
	case "token":
		c, err := winagent.ReadConfig()
		if err != nil {
			return err
		}
		if len(args) == 2 && args[1] == "show" {
			if err := securefile.Check(c.AuthTokenFile); err != nil {
				return err
			}
			data, err := os.ReadFile(c.AuthTokenFile)
			if err != nil {
				return err
			}
			fmt.Println(strings.TrimSpace(string(data)))
			return nil
		}
		if len(args) != 3 || args[1] != "reset" || args[2] != "--yes" {
			return errors.New("rotation invalidates existing clients; use token reset --yes, then token show")
		}
		old, err := os.ReadFile(c.AuthTokenFile)
		if err != nil {
			return err
		}
		if err := writeToken(c.AuthTokenFile); err != nil {
			return err
		}
		err = serviceAction("restart")
		if err == nil {
			err = doctor()
		}
		if err != nil {
			restoreErr := winagent.WriteProtected(c.AuthTokenFile, old)
			restartErr := serviceAction("restart")
			return errors.Join(fmt.Errorf("rotation failed; old token restore attempted: %w", err), restoreErr, restartErr)
		}
		fmt.Println("Token rotated. Retrieve it with: sma-ctl.exe token show")
		return nil
	}
	return errors.New("unknown command; run sma-ctl.exe help")
}

func provision() error {
	if windows.RtlGetVersion().BuildNumber < 17763 {
		return errors.New("SMA requires Windows 10 build 17763 or Windows Server 2019 or later")
	}
	for _, path := range []string{winagent.DataDir(), filepath.Dir(winagent.LogPath())} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		if err := securefile.Protect(path, true); err != nil {
			return err
		}
	}
	c, err := winagent.ReadConfig()
	if errors.Is(err, os.ErrNotExist) {
		c = winagent.Config{ListenAddress: "127.0.0.1:9108", AuthTokenFile: winagent.TokenPath()}
		// Do not silently regenerate a missing token for an existing configuration.
		if _, err := os.Stat(c.AuthTokenFile); errors.Is(err, os.ErrNotExist) {
			if err := writeToken(c.AuthTokenFile); err != nil {
				return err
			}
		}
		if err := winagent.SaveConfig(c); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := securefile.Check(c.AuthTokenFile); err != nil {
		return err
	}
	data, err := os.ReadFile(c.AuthTokenFile)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 || len(data) > 4096 {
		return errors.New("existing token is empty or too long")
	}
	return securefile.Protect(winagent.ConfigPath(), false)
}
func writeToken(path string) error {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return err
	}
	return winagent.WriteProtected(path, []byte(hex.EncodeToString(data[:])+"\n"))
}
func withService(fn func(*mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(winagent.ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	return fn(s)
}
func serviceAction(action string) error {
	return withService(func(s *mgr.Service) error {
		state, err := s.Query()
		if err != nil {
			return err
		}
		if action == "stop" || action == "restart" {
			if state.State != svc.Stopped {
				if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
					return err
				}
				if err := waitState(s, svc.Stopped); err != nil {
					return err
				}
			}
		}
		if action == "start" || action == "restart" {
			state, err = s.Query()
			if err != nil {
				return err
			}
			if state.State != svc.Running {
				if err := s.Start(); err != nil {
					return err
				}
			}
			return waitState(s, svc.Running)
		}
		return nil
	})
}
func waitState(s *mgr.Service, want svc.State) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		state, err := s.Query()
		if err != nil {
			return err
		}
		if state.State == want {
			return nil
		}
		if want == svc.Running && state.State == svc.Stopped {
			return fmt.Errorf("service stopped during startup (exit %d); check %s", state.Win32ExitCode, winagent.LogPath())
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("service transition timed out")
}
func status() error {
	return withService(func(s *mgr.Service) error {
		state, err := s.Query()
		if err != nil {
			return err
		}
		c, err := s.Config()
		if err != nil {
			return err
		}
		fmt.Printf("Service: %s\nState: %d (4=running, 1=stopped)\nAutomatic startup: %t\nConfig: %s\nLog: %s\n", winagent.ServiceName, state.State, c.StartType == mgr.StartAutomatic, winagent.ConfigPath(), winagent.LogPath())
		return nil
	})
}
func doctor() error {
	c, err := winagent.ReadConfig()
	if err != nil {
		return err
	}
	if err := securefile.Check(c.AuthTokenFile); err != nil {
		return err
	}
	data, err := os.ReadFile(c.AuthTokenFile)
	if err != nil {
		return err
	}
	host, port, _ := net.SplitHostPort(c.ListenAddress)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	base := "http://" + net.JoinHostPort(host, port)
	for _, path := range []string{"/readyz", "/v1/snapshot"} {
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		if path == "/v1/snapshot" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(data)))
		}
		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		if res.StatusCode != 200 {
			return fmt.Errorf("%s returned HTTP %d", path, res.StatusCode)
		}
		if path == "/v1/snapshot" {
			var result struct {
				Errors []struct{ Collector, Code, Message string } `json:"errors"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return err
			}
			for _, warning := range result.Errors {
				fmt.Printf("WARNING collector %s: %s (%s)\n", warning.Collector, warning.Message, warning.Code)
			}
		}
		fmt.Println("OK", path)
	}
	fmt.Printf("Agent URL: %s\nToken: protected (use token show to retrieve)\n", base)
	return nil
}
func restoreConfig(old winagent.Config, cause error) error {
	restoreErr := winagent.SaveConfig(old)
	restartErr := serviceAction("restart")
	return errors.Join(fmt.Errorf("configuration failed; previous configuration restore attempted: %w", cause), restoreErr, restartErr)
}
