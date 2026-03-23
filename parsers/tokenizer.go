package parsers

func IsIPv4Fast(s string) bool {
	parts := 0
	num := 0
	hasDigits := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if !hasDigits || parts == 3 {
				return false
			}
			if num > 255 {
				return false
			}
			parts++
			num = 0
			hasDigits = false
		} else if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
			hasDigits = true
		} else {
			return false
		}
	}
	return parts == 3 && hasDigits && num <= 255
}

func IsMACFast(s string) bool {
	if len(s) != 17 {
		return false
	}
	sep := s[2]
	if sep != ':' && sep != '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i%3 == 2 {
			if s[i] != sep {
				return false
			}
		} else {
			c := s[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func FindTokensFast(line string) []string {
	var tokens []string
	start := -1
	for i := 0; i <= len(line); i++ {
		isBoundary := true
		if i < len(line) {
			c := line[i]
			if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '.' || c == ':' || c == '-' {
				isBoundary = false
			}
		}

		if isBoundary {
			if start != -1 {
				word := line[start:i]
				if IsIPv4Fast(word) || IsMACFast(word) {
					tokens = append(tokens, word)
				}
				start = -1
			}
		} else {
			if start == -1 {
				start = i
			}
		}
	}
	return tokens
}
