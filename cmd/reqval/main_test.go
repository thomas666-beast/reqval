package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldRequest("demo", dir, "StorePostRequest", false); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	path := filepath.Join(dir, "storepostrequest.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"package demo",
		"type StorePostRequest struct",
		"func (r *StorePostRequest) Rules() map[string][]string",
		"func (r *StorePostRequest) Messages() map[string]string",
		"func (r *StorePostRequest) Authorize(ctx context.Context) error",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestScaffoldRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldRequest("demo", dir, "StorePostRequest", false); err != nil {
		t.Fatal(err)
	}
	if err := scaffoldRequest("demo", dir, "StorePostRequest", false); err == nil {
		t.Fatal("want error on overwrite, got nil")
	}
	if err := scaffoldRequest("demo", dir, "StorePostRequest", true); err != nil {
		t.Fatalf("force overwrite failed: %v", err)
	}
}

func TestScaffoldRejectsBadName(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldRequest("demo", dir, "lowercase", false); err == nil {
		t.Fatal("want error for non-exported name, got nil")
	}
}
