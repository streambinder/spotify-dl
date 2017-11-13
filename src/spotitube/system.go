package spotitube

import (
	"bufio"
	"errors"
	"math/rand"
	"os"
	"strings"
	"syscall"
	"time"
)

var (
	opt_interactive *bool = GetBoolPointer(false)
	opt_logfile     *bool = GetBoolPointer(false)
	opt_debug       *bool = GetBoolPointer(false)

	logger *Logger = NewLogger()

	opt_download_path string // TODO: fire this away
)

func IsDir(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	file_stat, err := file.Stat()
	if err != nil {
		return false
	}
	return file_stat.IsDir()
}

func MakeRange(min, max int) []int {
	a := make([]int, max-min+1)
	for i := range a {
		a[i] = min + i
	}
	return a
}

func GetBoolPointer(value bool) *bool {
	return &value
}

func RandString(n int) string {
	var src = rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	for i, cache, remain := n-1, src.Int63(), SYSTEM_LETTER_IDX_MAX; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), SYSTEM_LETTER_IDX_MAX
		}
		if idx := int(cache & SYSTEM_LETTER_IDX_MASK); idx < len(SYSTEM_LETTER_BYTES) {
			b[i] = SYSTEM_LETTER_BYTES[idx]
			i--
		}
		cache >>= SYSTEM_LETTER_IDX_BITS
		remain--
	}

	return string(b)
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func SyscallLimit(limit *syscall.Rlimit) error {
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, limit); err != nil {
		return err
	}
	return nil
}

func WaitForInput(input_prompt string) string {
	logger.Prompt(input_prompt)
	input_scanner := bufio.NewScanner(os.Stdin)
	input_scanner.Scan()
	return input_scanner.Text()
}

func WaitForConfirmation(input_prompt string, input_default bool) (bool, error) {
	if input_default {
		input_prompt = input_prompt + " [Y/n] "
	} else {
		input_prompt = input_prompt + " [y/N] "
	}
	input_user := strings.ToLower(string(WaitForInput(input_prompt)[0:1]))
	if input_user == "y" {
		return true, nil
	} else if input_user == "n" {
		return false, nil
	}
	return false, errors.New("Input not allowed, only [yYnN] permitted")
}
