package spotitube

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
)

const (
	LogNormal  = iota
	LogDebug   = iota
	LogWarning = iota
	LogFatal   = iota
)

type Logger struct {
	Color func(a ...interface{}) string
	File  string
}

func NewLogger() *Logger {
	return &Logger{
		Color: color.New(SHELL_COLOR_DEFAULT).SprintFunc(),
		File:  fmt.Sprintf(DEFAULT_LOG_PATH, time.Now().Format("20060102150405")),
	}
}
func (logger *Logger) Prefix(parameters ...string) string {
	name := SHELL_NAME_DEFAULT
	if len(parameters) > 0 {
		name = parameters[0]
	}
	space_pre := strings.Repeat(" ", ((SHELL_NAME_MIN_LENGTH - len(name)) / 2))
	space_post := space_pre
	if len(name)%2 == 1 {
		space_post = space_post + " "
	}
	return "[" + space_pre + strings.ToUpper(name) + space_post + "]"
}

func (logger *Logger) ColoredPrefix(color func(a ...interface{}) string, parameters ...string) string {
	if len(parameters) > 0 {
		return color(logger.Prefix(parameters[0]))
	} else {
		return color(logger.Prefix())
	}
}

func (logger *Logger) LogOpt(message string, level int) {
	runtime_caller_name := SHELL_NAME_DEFAULT
	runtime_caller_col := logger.Color

	for index := range []int{1, 2, 3, 4, 5} {
		runtime_caller, _, _, runtime_ok := runtime.Caller(index)
		if runtime_caller_details := runtime.FuncForPC(runtime_caller); runtime_ok && runtime_caller_details != nil {
			if strings.Contains(strings.ToLower(runtime_caller_details.Name()), "spotify") {
				runtime_caller_name = "spotify"
				runtime_caller_col = color.New(SHELL_COLOR_SPOTIFY).SprintFunc()
				break
			} else if strings.Contains(strings.ToLower(runtime_caller_details.Name()), "youtube") {
				runtime_caller_name = "youtube"
				runtime_caller_col = color.New(SHELL_COLOR_YOUTUBE).SprintFunc()
				break
			}
		}
	}

	if !(*opt_debug) && level == LogDebug {
		return
	}
	if *opt_logfile {
		logger.LogWrite(logger.Prefix(runtime_caller_name), message)
	}
	if level == LogDebug {
		message = color.MagentaString(message)
	} else if level == LogWarning {
		message = color.YellowString(message)
	} else if level == LogFatal {
		message = color.RedString(message)
	}
	fmt.Println(logger.ColoredPrefix(runtime_caller_col, runtime_caller_name), message)
	if level == LogFatal {
		os.Exit(1)
	}
}

func (logger *Logger) Log(message string) {
	logger.LogOpt(message, LogNormal)
}

func (logger *Logger) Debug(message string) {
	logger.LogOpt(message, LogDebug)
}

func (logger *Logger) Warn(message string) {
	logger.LogOpt(message, LogWarning)
}

func (logger *Logger) Fatal(message string) {
	logger.LogOpt(message, LogFatal)
}

func (logger *Logger) LogWrite(prefix string, message string) {
	logfile, err := os.OpenFile(logger.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer logfile.Close()
	if _, err = logfile.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " +
		logger.Prefix() + " " +
		message + "\n"); err != nil {
		panic(err)
	}
}

func (logger *Logger) SetFile(path string) {
	logger.EnableLogFile()
	logger.File = path
}

func (logger *Logger) EnableLogFile() {
	opt_logfile = GetBoolPointer(true)
}

func (logger *Logger) EnableDebug() {
	opt_debug = GetBoolPointer(true)
}
