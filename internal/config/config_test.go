package config

import "testing"

func TestGenerateAccessCode(t *testing.T) {
	code, err := GenerateAccessCode(6)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("len(code) = %d, want 6", len(code))
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			t.Fatalf("code contains non-digit: %q", code)
		}
	}
}

func TestNormalize(t *testing.T) {
	cfg := Normalize(Config{Port: -1})
	if cfg.Port != DefaultPort {
		t.Fatalf("Port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.StorageDir == "" {
		t.Fatal("StorageDir should be set")
	}
	if cfg.AccessCode == "" {
		t.Fatal("AccessCode should be set")
	}
}

func TestIsLegacyRootStorageDir(t *testing.T) {
	if !IsLegacyRootStorageDir("/data/uploads") {
		t.Fatal("expected /data/uploads to be treated as legacy root storage")
	}
	if IsLegacyRootStorageDir(DefaultStorageDir()) {
		t.Fatal("default storage dir should not be legacy root storage")
	}
}
