package emailconfig

import (
	"fmt"

	"luckytv/backend/internal/cloud/config"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/mail"
)

func Build(cfg config.Config) (identity.VerificationMailer, []byte, error) {
	if cfg.EmailOTPHMACSecret == "" && cfg.SMTPHost == "" && cfg.SMTPPort == "" && cfg.SMTPFrom == "" && cfg.SMTPUsername == "" && cfg.SMTPPassword == "" {
		return nil, nil, nil
	}
	if len(cfg.EmailOTPHMACSecret) < 32 {
		return nil, nil, fmt.Errorf("email OTP HMAC secret must contain at least 32 bytes")
	}
	sender, err := mail.NewSMTPMailer(mail.SMTPConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword, From: cfg.SMTPFrom,
	})
	if err != nil {
		return nil, nil, err
	}
	return sender, []byte(cfg.EmailOTPHMACSecret), nil
}
