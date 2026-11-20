package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/config"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/store"
)

type httpFakeMailer struct{ Messages []httpMailMessage }

type httpMailMessage struct{ Email, Code string }

func (m *httpFakeMailer) SendVerificationCode(_ context.Context, email, code string) error {
	m.Messages = append(m.Messages, httpMailMessage{Email: email, Code: code})
	return nil
}

func TestEmailOTPHTTPFlowReturnsNeutralRequestAndSessionOnVerify(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "email-http.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	services, err := cloudapp.New(db, time.Hour, false, nil, nil, nil, nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	mailer := &httpFakeMailer{}
	otp := services.Identity
	emailOTP := identity.NewEmailOTPService(db, otp, mailer, []byte("01234567890123456789012345678901"))
	handler := NewRouter(config.Config{}, services.Identity, nil, emailOTP, services.Catalog, services.Ledger, services.Payment, nil)

	requestCode := request(handler, http.MethodPost, "/api/v1/auth/email/code", "", "127.0.0.1:50000", `{"email":"creator@example.com"}`)
	if requestCode.Code != http.StatusAccepted || len(mailer.Messages) != 1 {
		t.Fatalf("request code status=%d messages=%d body=%s", requestCode.Code, len(mailer.Messages), requestCode.Body.String())
	}
	verify := request(handler, http.MethodPost, "/api/v1/auth/email/verify", "", "127.0.0.1:50000", `{"email":"creator@example.com","code":"`+mailer.Messages[0].Code+`"}`)
	if verify.Code != http.StatusCreated {
		t.Fatalf("verify status=%d body=%s", verify.Code, verify.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(verify.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("missing email session: %v %s", err, verify.Body.String())
	}
	invalid := request(handler, http.MethodPost, "/api/v1/auth/email/verify", "", "127.0.0.1:50000", `{"email":"unknown@example.com","code":"000000"}`)
	if invalid.Code != http.StatusUnauthorized || invalid.Body.String() != `{"error":"invalid_verification_code"}` {
		t.Fatalf("invalid verification response leaked state: %d %s", invalid.Code, invalid.Body.String())
	}
}
