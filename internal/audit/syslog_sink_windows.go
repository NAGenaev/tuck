//go:build windows

package audit

import "errors"

// SyslogSink is not supported on Windows.
type SyslogSink struct{}

// NewSyslogSink always returns an error on Windows.
func NewSyslogSink(network, addr string, _ uint) (*SyslogSink, error) {
	return nil, errors.New("syslog is not supported on Windows")
}

func (s *SyslogSink) Send(_ Entry) error { return errors.New("syslog unsupported") }
func (s *SyslogSink) Close() error       { return nil }
