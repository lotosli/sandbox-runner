package proc

import "regexp"

var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9\-\._~\+\/]+=*`),
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret)=([^\s]+)`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)=([^\s]+)`),
}

func Redact(line string) string {
	redacted := line
	for _, pattern := range redactPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}
