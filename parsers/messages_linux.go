//go:build linux

package parsers

func GetSystemLogPaths() []string {
	return []string{
		"/var/log/syslog",
		"/var/log/messages",
		"/var/log/auth.log",
		"/var/log/secure",
	}
}
