package openwechat

import (
	"bytes"
	"os"
	"testing"
)

func TestFileHotReloadStorageUsesPrivatePermissions(t *testing.T) {
	path := t.TempDir() + "/hotreload.json"
	storage := NewFileHotReloadStorage(path)
	if _, err := storage.Write([]byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("hot reload file mode = %o, want 600", got)
	}

	reader := NewFileHotReloadStorage(path)
	defer func() { _ = reader.Close() }()
	buf := make([]byte, 6)
	if _, err := reader.Read(buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, []byte("secret")) {
		t.Fatalf("read %q, want %q", buf, "secret")
	}
}
