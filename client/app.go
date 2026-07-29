package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// App struct
type App struct {
	ctx       context.Context
	serverURL string
	configPath string
}

// Config structure stored locally
type ClientConfig struct {
	Email           string `json:"email"`
	LastUpdateCheck string `json:"last_update_check"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	// Setup user config path
	homeDir, err := os.UserHomeDir()
	var configPath string
	if err != nil {
		configPath = "dft_config.json"
	} else {
		configPath = filepath.Join(homeDir, ".dft_config.json")
	}

	return &App{
		serverURL:  "http://localhost:8080",
		configPath: configPath,
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetHWID generates a unique, platform-specific SHA-256 Hardware ID
func (a *App) GetHWID() string {
	var rawID string
	switch runtime.GOOS {
	case "windows":
		// Get BIOS UUID
		cmd := exec.Command("wmic", "csproduct", "get", "uuid")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.EqualFold(line, "uuid") {
					rawID += line
					break
				}
			}
		}
		// Also append Processor ID
		cmdCpu := exec.Command("wmic", "cpu", "get", "processorid")
		outputCpu, err := cmdCpu.Output()
		if err == nil {
			lines := strings.Split(string(outputCpu), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.EqualFold(line, "processorid") {
					rawID += line
					break
				}
			}
		}
	case "linux":
		// Read machine-id
		data, err := os.ReadFile("/etc/machine-id")
		if err != nil {
			data, err = os.ReadFile("/var/lib/dbus/machine-id")
		}
		if err == nil {
			rawID = strings.TrimSpace(string(data))
		} else {
			hostname, _ := os.Hostname()
			rawID = hostname
		}
	case "darwin":
		// macOS UUID
		cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, "IOPlatformUUID") {
					parts := strings.Split(line, "=")
					if len(parts) == 2 {
						rawID = strings.Trim(strings.TrimSpace(parts[1]), "\"")
						break
					}
				}
			}
		}
	default:
		rawID = "unknown-os-hwid"
	}

	if rawID == "" {
		rawID = "fallback-default-hwid"
	}

	hash := sha256.Sum256([]byte(rawID))
	return hex.EncodeToString(hash[:])
}

// SaveEmail stores the logged in user's email locally
func (a *App) SaveEmail(email string) bool {
	var cfg ClientConfig
	data, err := os.ReadFile(a.configPath)
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	cfg.Email = email
	newData, err := json.Marshal(cfg)
	if err != nil {
		return false
	}
	err = os.WriteFile(a.configPath, newData, 0600)
	return err == nil
}

// GetSavedEmail retrieves the stored user email if present
func (a *App) GetSavedEmail() string {
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return ""
	}
	var cfg ClientConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return ""
	}
	return cfg.Email
}

// ClearSavedSession deletes the local user session config
func (a *App) ClearSavedSession() bool {
	_ = os.Remove(a.configPath)
	return true
}

// RegisterOnServer contacts the central server to register a new user
func (a *App) RegisterOnServer(email, password string) (string, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	resp, err := http.Post(fmt.Sprintf("%s/api/register", a.serverURL), "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", errors.New("unable to connect to server")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		var errData map[string]string
		_ = json.Unmarshal(body, &errData)
		if errMsg, exists := errData["error"]; exists {
			return "", errors.New(errMsg)
		}
		return "", fmt.Errorf("registration failed: status %d", resp.StatusCode)
	}

	return string(body), nil
}

// LoginOnServer logs the user in on the central server
func (a *App) LoginOnServer(email, password string) (string, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	resp, err := http.Post(fmt.Sprintf("%s/api/login", a.serverURL), "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", errors.New("unable to connect to server")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.Unmarshal(body, &errData)
		if errMsg, exists := errData["error"]; exists {
			return "", errors.New(errMsg)
		}
		return "", fmt.Errorf("login failed: status %d", resp.StatusCode)
	}

	// Save session locally on successful login
	a.SaveEmail(email)

	return string(body), nil
}

// CheckLicenseOnServer verifies license and active trial status on the server
func (a *App) CheckLicenseOnServer(email string) (string, error) {
	hwid := a.GetHWID()
	reqBody, _ := json.Marshal(map[string]string{
		"email": email,
		"hwid":  hwid,
	})

	resp, err := http.Post(fmt.Sprintf("%s/api/check-license", a.serverURL), "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", errors.New("unable to connect to server")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.Unmarshal(body, &errData)
		if errMsg, exists := errData["error"]; exists {
			return "", errors.New(errMsg)
		}
		return "", fmt.Errorf("license verification failed: status %d", resp.StatusCode)
	}

	return string(body), nil
}

// ChargeCredits deductions
func (a *App) ChargeCredits(email string, amount float64, description string) (string, error) {
	hwid := a.GetHWID()
	reqBody, _ := json.Marshal(map[string]interface{}{
		"email":       email,
		"hwid":        hwid,
		"amount":      amount,
		"description": description,
	})

	resp, err := http.Post(fmt.Sprintf("%s/api/charge", a.serverURL), "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", errors.New("unable to connect to server")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.Unmarshal(body, &errData)
		if errMsg, exists := errData["error"]; exists {
			return "", errors.New(errMsg)
		}
		return "", fmt.Errorf("charge operation failed: status %d", resp.StatusCode)
	}

	return string(body), nil
}

// PerformMockOperation triggers a premium mobile repair action (e.g. FRP bypass, IMEI repair)
func (a *App) PerformMockOperation(email string, opName string, cost float64) (string, error) {
	// Deduct balance on server
	res, err := a.ChargeCredits(email, cost, fmt.Sprintf("Operation: %s", opName))
	if err != nil {
		return "", err
	}

	// Simulate device repair steps
	time.Sleep(1 * time.Second)
	return res, nil
}

// CheckForUpdates handles checking with the server for any software updates.
// If force is true, it ignores the 7/15 day interval check.
func (a *App) CheckForUpdates(force bool, intervalDays int) (string, error) {
	var cfg ClientConfig
	configData, err := os.ReadFile(a.configPath)
	if err == nil {
		_ = json.Unmarshal(configData, &cfg)
	}

	now := time.Now()
	shouldCheck := force

	if !shouldCheck && cfg.LastUpdateCheck != "" {
		lastCheckTime, err := time.Parse(time.RFC3339, cfg.LastUpdateCheck)
		if err == nil {
			duration := now.Sub(lastCheckTime)
			daysElapsed := int(duration.Hours() / 24)
			if daysElapsed >= intervalDays {
				shouldCheck = true
			}
		} else {
			shouldCheck = true
		}
	} else if cfg.LastUpdateCheck == "" {
		shouldCheck = true
	}

	if !shouldCheck {
		return "skipped", nil
	}

	// Make request to update server
	resp, err := http.Get(fmt.Sprintf("%s/api/update/check", a.serverURL))
	if err != nil {
		return "", errors.New("unable to connect to update server")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Update the check timestamp in configuration
	cfg.LastUpdateCheck = now.Format(time.RFC3339)
	newData, err := json.Marshal(cfg)
	if err == nil {
		_ = os.WriteFile(a.configPath, newData, 0600)
	}

	return string(body), nil
}
