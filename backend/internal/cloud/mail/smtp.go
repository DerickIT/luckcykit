package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type SMTPMailer struct{ config SMTPConfig }

func NewSMTPMailer(config SMTPConfig) (*SMTPMailer, error) {
	if strings.TrimSpace(config.Host) == "" || strings.TrimSpace(config.Port) == "" || strings.TrimSpace(config.From) == "" {
		return nil, fmt.Errorf("SMTP host, port, and sender are required")
	}
	return &SMTPMailer{config: config}, nil
}

func (m *SMTPMailer) SendVerificationCode(ctx context.Context, to, code string) error {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(m.config.Host, m.config.Port), &tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.config.Host})
	if err != nil {
		return fmt.Errorf("connect SMTP over TLS: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}
	client, err := smtp.NewClient(conn, m.config.Host)
	if err != nil {
		return fmt.Errorf("initialize SMTP client: %w", err)
	}
	defer client.Close()
	if m.config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)); err != nil {
			return fmt.Errorf("authenticate SMTP client: %w", err)
		}
	}
	if err := client.Mail(m.config.From); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP message: %w", err)
	}
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: LuckyTV verification code\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nYour LuckyTV verification code is %s. It expires in 10 minutes.\r\n", m.config.From, to, code)
	if _, err := writer.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	return client.Quit()
}
