package webui

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestClientIsRootedAtItsPage(t *testing.T) {
	t.Parallel()
	files := client(fstest.MapFS{"dist/index.html": {Data: []byte("page")}})
	if files == nil {
		t.Fatal("client = nil")
	}
	if data, err := fs.ReadFile(files, "index.html"); err != nil || string(data) != "page" {
		t.Errorf("index.html = %q, %v", data, err)
	}
}

// What git holds: the folder, with a placeholder so go:embed has something.
func TestNoClientWithoutAPage(t *testing.T) {
	t.Parallel()
	if files := client(fstest.MapFS{"dist/.gitkeep": {}}); files != nil {
		t.Errorf("client = %v, want nil", files)
	}
}
