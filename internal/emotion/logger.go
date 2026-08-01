package emotion

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type ResponseLogger struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

func OpenResponseLogger(path string) (*ResponseLogger, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open emotion response log: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set emotion response log permissions: %w", err)
	}
	return &ResponseLogger{file: file, encoder: json.NewEncoder(file)}, nil
}

func (l *ResponseLogger) Write(entry ResponseLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.encoder.Encode(entry)
}

func (l *ResponseLogger) Close() error {
	return l.file.Close()
}
