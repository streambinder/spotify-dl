package system

import (
	"io"
	"math/rand"
	"os"
	"time"
)

func Dir(path string) bool {
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

func FileTouch(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	file.Close()
	return nil
}

func FileCopy(path_from string, path_to string) error {
	path_from_open, err := os.Open(path_from)
	if err != nil {
		return err
	}
	defer path_from_open.Close()

	path_to_open, err := os.Create(path_to)
	if err != nil {
		return err
	}

	if _, err := io.Copy(path_to_open, path_from_open); err != nil {
		path_to_open.Close()
		return err
	}

	return path_to_open.Close()
}