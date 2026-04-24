package service

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type windsurfPathReplacement struct {
	re          *regexp.Regexp
	replacement string
}

type windsurfPathSanitizeStream struct {
	buffer string
}

func newWindsurfPathSanitizeStream() *windsurfPathSanitizeStream {
	return &windsurfPathSanitizeStream{}
}

func (s *windsurfPathSanitizeStream) Feed(delta string) string {
	if s == nil || delta == "" {
		return ""
	}
	s.buffer += delta
	cut := s.safeCutPoint()
	if cut <= 0 {
		return ""
	}
	out := sanitizeWindsurfText(s.buffer[:cut])
	s.buffer = s.buffer[cut:]
	return out
}

func (s *windsurfPathSanitizeStream) Flush() string {
	if s == nil || s.buffer == "" {
		return ""
	}
	out := sanitizeWindsurfText(s.buffer)
	s.buffer = ""
	return out
}

func (s *windsurfPathSanitizeStream) safeCutPoint() int {
	if s == nil || s.buffer == "" {
		return 0
	}
	buf := s.buffer
	cut := len(buf)

	for _, lit := range windsurfSensitivePathLiterals() {
		searchFrom := 0
		for searchFrom < len(buf) {
			idx := strings.Index(buf[searchFrom:], lit)
			if idx < 0 {
				break
			}
			idx += searchFrom
			end := idx + len(lit)
			for end < len(buf) && isWindsurfPathBodyChar(buf[end]) {
				end++
			}
			if end == len(buf) && idx < cut {
				cut = idx
			}
			searchFrom = end + 1
		}
	}

	for _, lit := range windsurfSensitivePathLiterals() {
		maxPrefixLen := min(len(lit)-1, len(buf))
		for plen := maxPrefixLen; plen > 0; plen-- {
			if strings.HasSuffix(buf, lit[:plen]) {
				start := len(buf) - plen
				if start < cut {
					cut = start
				}
				break
			}
		}
	}

	return cut
}

func sanitizeWindsurfText(text string) string {
	if text == "" {
		return ""
	}
	out := text
	for _, replacement := range windsurfSensitivePathReplacements() {
		out = replacement.re.ReplaceAllString(out, replacement.replacement)
	}
	return out
}

func sanitizeWindsurfToolCall(call apicompat.ChatToolCall) apicompat.ChatToolCall {
	call.Function.Arguments = sanitizeWindsurfText(call.Function.Arguments)
	return call
}

func containsWindsurfInternalPath(text string) bool {
	if text == "" {
		return false
	}
	for _, lit := range windsurfSensitivePathLiterals() {
		if strings.Contains(text, lit) {
			return true
		}
	}
	return false
}

func windsurfSensitivePathReplacements() []windsurfPathReplacement {
	replacements := make([]windsurfPathReplacement, 0, 5)
	replacements = append(replacements, windsurfPathReplacement{
		re:          regexp.MustCompile(`/home/user/projects/workspace-[^/\s"'` + "`" + `<>)}\],*;]*(/[^\s"'` + "`" + `<>)}\],*;]*)?`),
		replacement: `.$1`,
	})
	for _, lit := range windsurfWorkspacePathLiterals() {
		replacements = append(replacements, windsurfPathReplacement{
			re:          regexp.MustCompile(regexp.QuoteMeta(lit) + `/workspace-[^/\s"'` + "`" + `<>)}\],*;]*(/[^\s"'` + "`" + `<>)}\],*;]*)?`),
			replacement: `.$1`,
		})
	}
	for _, lit := range windsurfWorkspacePathLiterals() {
		replacements = append(replacements, windsurfPathReplacement{
			re:          regexp.MustCompile(regexp.QuoteMeta(lit) + `(/[^\s"'` + "`" + `<>)}\],*;]*)?`),
			replacement: `.$1`,
		})
	}
	for _, lit := range []string{"/opt/windsurf", "/root/WindsurfAPI"} {
		replacements = append(replacements, windsurfPathReplacement{
			re:          regexp.MustCompile(regexp.QuoteMeta(lit) + `(/[^\s"'` + "`" + `<>)}\],*;]*)?`),
			replacement: `[internal]`,
		})
	}
	return replacements
}

func windsurfSensitivePathLiterals() []string {
	literals := append([]string{}, windsurfWorkspacePathLiterals()...)
	literals = append(literals, "/home/user/projects/workspace-")
	literals = append(literals, "/opt/windsurf", "/root/WindsurfAPI")
	return literals
}

func windsurfWorkspacePathLiterals() []string {
	workspaceDir := firstNonEmptyString(os.Getenv("WINDSURF_LS_WORKSPACE_DIR"), filepath.Join(os.TempDir(), "windsurf-workspace"))
	workspaceDir = filepath.ToSlash(strings.TrimSpace(workspaceDir))

	values := []string{
		workspaceDir,
		"/tmp/windsurf-workspace",
		"/windsurf-workspace",
	}
	if base := strings.TrimSpace(filepath.Base(workspaceDir)); base != "" && base != "." && base != "/" {
		values = append(values, "/"+base)
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(filepath.ToSlash(value))
		if value == "" || value == "." || value == "/" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.SortFunc(out, func(a, b string) int {
		return len(b) - len(a)
	})
	return out
}

func isWindsurfPathBodyChar(ch byte) bool {
	if ch <= ' ' {
		return false
	}
	return !strings.ContainsRune("\"'`<>)}],*;", rune(ch))
}
