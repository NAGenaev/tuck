//go:build !windows

package audit

import (
	"fmt"
	"log/syslog"
	"strings"
)

// SyslogSupported reports whether this build can forward audit records to
// syslog (it cannot on Windows).
const SyslogSupported = true

var syslogFacilities = map[string]syslog.Priority{
	"kern": syslog.LOG_KERN, "user": syslog.LOG_USER, "mail": syslog.LOG_MAIL,
	"daemon": syslog.LOG_DAEMON, "auth": syslog.LOG_AUTH, "syslog": syslog.LOG_SYSLOG,
	"authpriv": syslog.LOG_AUTHPRIV,
	"local0":   syslog.LOG_LOCAL0, "local1": syslog.LOG_LOCAL1, "local2": syslog.LOG_LOCAL2,
	"local3": syslog.LOG_LOCAL3, "local4": syslog.LOG_LOCAL4, "local5": syslog.LOG_LOCAL5,
	"local6": syslog.LOG_LOCAL6, "local7": syslog.LOG_LOCAL7,
}

// ValidSyslogFacility reports whether f names a supported facility ("" = default).
func ValidSyslogFacility(f string) bool {
	if f == "" {
		return true
	}
	_, ok := syslogFacilities[strings.ToLower(f)]
	return ok
}

// NewSyslogSinkFromOptions builds a syslog sink from stored sink options:
// "network" (udp|tcp|"" for the local socket), "addr" (host:port) and
// "facility" (e.g. local0; default local0).
func NewSyslogSinkFromOptions(opts map[string]string) (Sink, error) {
	fac := syslog.LOG_LOCAL0
	if f := opts["facility"]; f != "" {
		p, ok := syslogFacilities[strings.ToLower(f)]
		if !ok {
			return nil, fmt.Errorf("unknown syslog facility %q", f)
		}
		fac = p
	}
	return NewSyslogSink(opts["network"], opts["addr"], fac)
}
