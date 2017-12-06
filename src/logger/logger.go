package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type Logger struct {
	File  string
	Mutex sync.Mutex
}

func Build(filename string) *Logger {
	return &Logger{
		File: filename,
	}
}

func (logger *Logger) Append(message string) error {
	logger.Mutex.Lock()
	defer logger.Mutex.Unlock()
	logger_file, err := os.OpenFile(logger.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logger_file.Close()
	if _, err = logger_file.WriteString(fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), message)); err != nil {
		return err
	}
	return nil
}
