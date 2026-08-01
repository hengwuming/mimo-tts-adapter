package emotion

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestResponseLoggerAppendsJSONWithRestrictedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emotion.jsonl")
	logger, err := OpenResponseLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(ResponseLogEntry{Time: time.Unix(1, 0).UTC(), Status: "success", Attempts: 1, Content: "第一条"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	logger, err = OpenResponseLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(ResponseLogEntry{Time: time.Unix(2, 0).UTC(), Status: "error", Attempts: 2, Content: "第二条"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		if runtime.GOOS != "windows" || info.Mode().Perm() != 0o666 {
			t.Fatalf("permissions = %o", info.Mode().Perm())
		}
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var first, second ResponseLogEntry
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&ResponseLogEntry{}); err != io.EOF {
		t.Fatalf("third decode error = %v", err)
	}
	if first.Content != "第一条" || second.Content != "第二条" {
		t.Fatalf("entries = %#v, %#v", first, second)
	}
}

func TestOpenResponseLoggerRejectsDirectory(t *testing.T) {
	if _, err := OpenResponseLogger(t.TempDir()); err == nil {
		t.Fatal("expected directory open error")
	}
}
