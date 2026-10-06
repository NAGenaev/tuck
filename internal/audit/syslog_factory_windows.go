//go:build windows

package audit

import "errors"

// SyslogSupported reports whether this build can forward audit records to
// syslog (it cannot on Windows).
const SyslogSupported = false

// ValidSyslogFacility is accepted for any value on Windows; building the sink
// fails with a clear error instead.
func ValidSyslogFacility(string) bool { return true }

// NewSyslogSinkFromOptions always fails on Windows.
func NewSyslogSinkFromOptions(map[string]string) (Sink, error) {
	return nil, errors.New("syslog sink is not supported on Windows")
}
