package cmd

import (
	"bytes"
	"testing"
)

func TestInstall_AsAndAll_MutuallyExclusive(t *testing.T) {
	root := NewRoot("test")

	// Set up the command with both --as and --all
	root.SetArgs([]string{"install", "owner/repo", "--as", "my-alias", "--all"})
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --as and --all are both set")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("--as")) || !bytes.Contains([]byte(err.Error()), []byte("--all")) {
		t.Errorf("error should mention both --as and --all, got: %v", err)
	}
}
