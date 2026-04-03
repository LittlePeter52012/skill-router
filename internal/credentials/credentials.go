// Package credentials manages secure API key storage for SKRT.
//
// Security model:
//   - Keys are stored in ~/.skrt/credentials with 0600 permissions (owner-only read/write)
//   - The credentials file is a simple key=value format
//   - Lookup order: environment variable → credentials file
//   - The config.json only stores the env var name, NEVER the actual key
//   - The credentials file is excluded from git via .gitignore
package credentials

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	credDir  = ".skrt"
	credFile = "credentials"
)

// credPath returns the path to the credentials file.
func credPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, credDir, credFile)
}

// Resolve looks up an API key using a two-tier strategy:
//
//  1. Check the environment variable specified by envName
//  2. If not found, check the credentials file (~/.skrt/credentials)
//
// Returns the key and its source ("env" or "file"), or empty if not found.
func Resolve(envName string) (key, source string) {
	// Tier 1: Environment variable
	if val := os.Getenv(envName); val != "" {
		return val, "env"
	}

	// Tier 2: Credentials file
	if val := readFromFile(envName); val != "" {
		return val, "file"
	}

	return "", ""
}

// Store saves an API key to the credentials file with strict permissions.
// The file is created with 0600 (owner read/write only).
func Store(envName, key string) error {
	path := credPath()
	dir := filepath.Dir(path)

	// Create directory if needed
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}

	// Read existing entries
	entries := readAllEntries()

	// Update or add the entry
	entries[envName] = key

	// Write all entries back
	return writeEntries(path, entries)
}

// Remove deletes an API key from the credentials file.
func Remove(envName string) error {
	entries := readAllEntries()
	delete(entries, envName)
	return writeEntries(credPath(), entries)
}

// readFromFile reads a specific key from the credentials file.
func readFromFile(envName string) string {
	entries := readAllEntries()
	return entries[envName]
}

// readAllEntries reads all key=value pairs from the credentials file.
func readAllEntries() map[string]string {
	entries := make(map[string]string)

	f, err := os.Open(credPath())
	if err != nil {
		return entries
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			entries[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return entries
}

// writeEntries writes all entries to the credentials file with strict permissions.
func writeEntries(path string, entries map[string]string) error {
	var sb strings.Builder
	sb.WriteString("# SKRT Credentials — DO NOT COMMIT THIS FILE\n")
	sb.WriteString("# This file contains API keys with restricted permissions (0600)\n")
	sb.WriteString("# Format: ENV_VAR_NAME=api_key_value\n\n")

	for name, key := range entries {
		sb.WriteString(fmt.Sprintf("%s=%s\n", name, key))
	}

	// Write with strict permissions: owner read/write only
	if err := os.WriteFile(path, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	// Ensure permissions are correct even if file existed
	return os.Chmod(path, 0600)
}
