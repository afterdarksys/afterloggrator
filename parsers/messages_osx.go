//go:build darwin

package parsers

func GetSystemLogPaths() []string {
	return []string{
		"/var/log/system.log",
	}
}
