//go:build !windows

package audit

import (
	"encoding/json"
	"fmt"
	"log/syslog"
)

// SyslogSink sends audit entries to a syslog daemon via UDP or TCP.
type SyslogSink struct {
	w *syslog.Writer
}

// NewSyslogSink connects to a syslog daemon.
// network may be "udp", "tcp", or "" (local Unix socket).
// addr may be empty for local syslog (e.g. "/dev/log").
// facility is a syslog.Priority constant; 0 defaults to syslog.LOG_LOCAL0.
func NewSyslogSink(network, addr string, facility syslog.Priority) (*SyslogSink, error) {
	if facility == 0 {
		facility = syslog.LOG_LOCAL0
	}
	w, err := syslog.Dial(network, addr, facility|syslog.LOG_INFO, "tuck-audit")
	if err != nil {
		return nil, fmt.Errorf("syslog dial: %w", err)
	}
	return &SyslogSink{w: w}, nil
}

func (s *SyslogSink) Send(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.w.Info(string(data))
}

func (s *SyslogSink) Close() error { return s.w.Close() }
