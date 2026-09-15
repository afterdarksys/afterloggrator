package parsers

import (
	"net"
	"strings"
	"unicode"
)

func IsIPv4Fast(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil && !strings.Contains(s, ":")
}
func IsMACFast(s string) bool {
	if len(s) != 17 {
		return false
	}
	_, err := net.ParseMAC(s)
	return err == nil
}

// FindTokensFast extracts complete address/domain tokens. Names and addresses
// are canonicalized to make joins insensitive to case and IPv6 spelling.
func FindTokensFast(line string) []string {
	seen := map[string]bool{}
	var tokens []string
	for _, word := range strings.FieldsFunc(line, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == ':' || r == '-' || r == '_')
	}) {
		word = strings.Trim(word, ".")
		token := ""
		if ip := net.ParseIP(word); ip != nil {
			token = ip.String()
		} else if IsMACFast(word) {
			mac, _ := net.ParseMAC(word)
			token = mac.String()
		} else {
			candidate := word
			if h, _, err := net.SplitHostPort(word); err == nil {
				candidate = h
			}
			if ip := net.ParseIP(candidate); ip != nil {
				token = ip.String()
			} else if domain(candidate) {
				token = strings.ToLower(candidate)
			}
		}
		if token != "" && !seen[token] {
			tokens = append(tokens, token)
			seen[token] = true
		}
	}
	return tokens
}
func domain(s string) bool {
	if len(s) > 253 {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 63 || p[0] == '-' || p[len(p)-1] == '-' {
			return false
		}
		for _, c := range p {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	for _, c := range parts[len(parts)-1] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return len(parts[len(parts)-1]) >= 2
}
