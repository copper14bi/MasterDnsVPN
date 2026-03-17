// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package logger

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

type Logger struct {
	name  string
	level int
	base  *log.Logger
	color bool
}

const (
	levelDebug = iota
	levelInfo
	levelWarn
	levelError
)

var colorTagPattern = regexp.MustCompile(`</?([a-zA-Z_]+)>`)

func New(name, rawLevel string) *Logger {
	return &Logger{
		name:  name,
		level: parseLevel(rawLevel),
		base:  log.New(os.Stdout, "", log.LstdFlags),
		color: shouldUseColor(),
	}
}

func parseLevel(raw string) int {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DEBUG":
		return levelDebug
	case "WARNING", "WARN":
		return levelWarn
	case "ERROR", "CRITICAL":
		return levelError
	default:
		return levelInfo
	}
}

func (l *Logger) logf(level int, levelName string, format string, args ...any) {
	if l == nil || level < l.level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	appName := "[" + l.name + "]"
	levelText := "[" + levelName + "]"

	if l.color {
		if strings.IndexByte(msg, '<') >= 0 {
			msg = renderColorTags(msg)
		}
		appName = "\x1b[36m" + appName + "\x1b[0m"
		levelText = colorizeLevel(level, levelText)
	}

	l.base.Printf("%s %s %s", appName, levelText, msg)
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(levelDebug, "DEBUG", format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(levelInfo, "INFO", format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(levelWarn, "WARN", format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(levelError, "ERROR", format, args...) }

func shouldUseColor() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if strings.TrimSpace(os.Getenv("FORCE_COLOR")) != "" {
		return true
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func colorizeLevel(level int, text string) string {
	switch level {
	case levelDebug:
		return "\x1b[35m" + text + "\x1b[0m"
	case levelInfo:
		return "\x1b[32m" + text + "\x1b[0m"
	case levelWarn:
		return "\x1b[33m" + text + "\x1b[0m"
	case levelError:
		return "\x1b[31m" + text + "\x1b[0m"
	default:
		return text
	}
}

func renderColorTags(text string) string {
	return colorTagPattern.ReplaceAllStringFunc(text, func(tag string) string {
		switch strings.ToLower(tag) {
		case "<black>":
			return "\x1b[30m"
		case "<red>":
			return "\x1b[31m"
		case "<green>":
			return "\x1b[32m"
		case "<yellow>":
			return "\x1b[33m"
		case "<blue>":
			return "\x1b[34m"
		case "<magenta>":
			return "\x1b[35m"
		case "<cyan>":
			return "\x1b[36m"
		case "<white>":
			return "\x1b[37m"
		case "<gray>", "<grey>":
			return "\x1b[90m"
		case "<bold>":
			return "\x1b[1m"
		case "<reset>":
			return "\x1b[0m"
		case "</black>", "</red>", "</green>", "</yellow>", "</blue>", "</magenta>", "</cyan>", "</white>", "</gray>", "</grey>", "</bold>":
			return "\x1b[0m"
		default:
			return tag
		}
	})
}

func NowUnixNano() int64 {
	return time.Now().UnixNano()
}
