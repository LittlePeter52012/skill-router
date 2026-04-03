package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreAndResolve(t *testing.T) {
	// Use a temp directory for testing
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Ensure no env var interference
	os.Unsetenv("TEST_SKRT_KEY")

	// Store a key
	err := Store("TEST_SKRT_KEY", "test-api-key-12345")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify file permissions
	path := filepath.Join(tmpDir, credDir, credFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("credentials file not found: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}

	// Resolve from file
	key, source := Resolve("TEST_SKRT_KEY")
	if key != "test-api-key-12345" {
		t.Errorf("expected 'test-api-key-12345', got '%s'", key)
	}
	if source != "file" {
		t.Errorf("expected source 'file', got '%s'", source)
	}
}

func TestResolveEnvTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Store a key in file
	err := Store("TEST_SKRT_PRIO", "file-key")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Set env var — should take precedence
	t.Setenv("TEST_SKRT_PRIO", "env-key")

	key, source := Resolve("TEST_SKRT_PRIO")
	if key != "env-key" {
		t.Errorf("expected env key 'env-key', got '%s'", key)
	}
	if source != "env" {
		t.Errorf("expected source 'env', got '%s'", source)
	}
}

func TestResolveNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	os.Unsetenv("NONEXISTENT_KEY_XYZ")

	key, source := Resolve("NONEXISTENT_KEY_XYZ")
	if key != "" {
		t.Errorf("expected empty key, got '%s'", key)
	}
	if source != "" {
		t.Errorf("expected empty source, got '%s'", source)
	}
}

func TestRemove(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	os.Unsetenv("TEST_RM_KEY")

	// Store then remove
	Store("TEST_RM_KEY", "to-be-removed")
	err := Remove("TEST_RM_KEY")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	key, _ := Resolve("TEST_RM_KEY")
	if key != "" {
		t.Errorf("expected empty after remove, got '%s'", key)
	}
}

func TestStoreMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	os.Unsetenv("KEY_A")
	os.Unsetenv("KEY_B")

	Store("KEY_A", "value-a")
	Store("KEY_B", "value-b")

	keyA, _ := Resolve("KEY_A")
	keyB, _ := Resolve("KEY_B")

	if keyA != "value-a" {
		t.Errorf("KEY_A: expected 'value-a', got '%s'", keyA)
	}
	if keyB != "value-b" {
		t.Errorf("KEY_B: expected 'value-b', got '%s'", keyB)
	}
}
