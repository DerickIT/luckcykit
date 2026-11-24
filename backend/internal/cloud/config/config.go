package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultListenAddr = "127.0.0.1:8081"
const defaultDataDir = "../.local/cloud-service-sandbox"
const defaultWorkspaceDataDir = "data"

type HostedModel struct {
	ID                     string `json:"id"`
	UpstreamID             string `json:"upstreamId"`
	PriceVersion           string `json:"priceVersion"`
	MaxInputTokens         int64  `json:"maxInputTokens"`
	MaxOutputTokens        int64  `json:"maxOutputTokens"`
	InputPointsPerMillion  int64  `json:"inputPointsPerMillion"`
	OutputPointsPerMillion int64  `json:"outputPointsPerMillion"`
}

type Config struct {
	Environment           string
	ListenAddr            string
	DataDir               string
	DatabaseURL           string
	LocalWorkspaceDataDir string
	EmailOTPHMACSecret    string
	SMTPHost              string
	SMTPPort              string
	SMTPUsername          string
	SMTPPassword          string
	SMTPFrom              string
	AdminBootstrapSecret  string
	AdminBootstrapEmail   string
	AdminInitialSecret    string
	WeChatAppID           string
	WeChatAppSecret       string
	WeChatCallbackURL     string
	SandboxEnabled        bool
	HostedModelsEnabled   bool
	OneAPIBaseURL         string
	OneAPIAPIKey          string
	HostedModels          []HostedModel
}

func (m HostedModel) Validate() error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.UpstreamID) == "" || strings.TrimSpace(m.PriceVersion) == "" {
		return fmt.Errorf("hosted model id, upstreamId, and priceVersion are required")
	}
	if m.MaxInputTokens <= 0 || m.MaxOutputTokens <= 0 || m.InputPointsPerMillion < 0 || m.OutputPointsPerMillion < 0 || m.InputPointsPerMillion == 0 && m.OutputPointsPerMillion == 0 {
		return fmt.Errorf("hosted model %q has invalid token limits or point rates", m.ID)
	}
	return nil
}

func Load() (Config, error) { return LoadFrom(os.Getenv) }

func LoadFrom(getenv func(string) string) (Config, error) {
	cfg := Config{
		Environment:           strings.ToLower(strings.TrimSpace(value(getenv, "LUCKYTV_CLOUD_ENV", "development"))),
		ListenAddr:            strings.TrimSpace(value(getenv, "LUCKYTV_CLOUD_ADDR", defaultListenAddr)),
		DataDir:               filepath.Clean(strings.TrimSpace(value(getenv, "LUCKYTV_CLOUD_DATA_DIR", defaultDataDir))),
		DatabaseURL:           strings.TrimSpace(getenv("LUCKYTV_CLOUD_DATABASE_URL")),
		LocalWorkspaceDataDir: strings.TrimSpace(value(getenv, "CANVAS_BACKEND_DATA_DIR", defaultWorkspaceDataDir)),
		EmailOTPHMACSecret:    strings.TrimSpace(getenv("LUCKYTV_CLOUD_EMAIL_OTP_HMAC_SECRET")),
		SMTPHost:              strings.TrimSpace(getenv("LUCKYTV_CLOUD_SMTP_HOST")),
		SMTPPort:              strings.TrimSpace(getenv("LUCKYTV_CLOUD_SMTP_PORT")),
		SMTPUsername:          strings.TrimSpace(getenv("LUCKYTV_CLOUD_SMTP_USERNAME")),
		SMTPPassword:          getenv("LUCKYTV_CLOUD_SMTP_PASSWORD"),
		SMTPFrom:              strings.TrimSpace(getenv("LUCKYTV_CLOUD_SMTP_FROM")),
		AdminBootstrapSecret:  getenv("LUCKYTV_CLOUD_ADMIN_BOOTSTRAP_SECRET"),
		AdminBootstrapEmail:   strings.TrimSpace(getenv("LUCKYTV_CLOUD_ADMIN_BOOTSTRAP_EMAIL")),
		AdminInitialSecret:    getenv("LUCKYTV_CLOUD_ADMIN_INITIAL_SECRET"),
		WeChatAppID:           strings.TrimSpace(getenv("LUCKYTV_CLOUD_WECHAT_APP_ID")),
		WeChatAppSecret:       getenv("LUCKYTV_CLOUD_WECHAT_APP_SECRET"),
		WeChatCallbackURL:     strings.TrimSpace(getenv("LUCKYTV_CLOUD_WECHAT_CALLBACK_URL")),
	}
	hostedValue := strings.TrimSpace(getenv("LUCKYTV_CLOUD_HOSTED_MODELS"))
	if hostedValue != "" {
		parsed, err := strconv.ParseBool(hostedValue)
		if err != nil {
			return Config{}, fmt.Errorf("LUCKYTV_CLOUD_HOSTED_MODELS must be true or false")
		}
		cfg.HostedModelsEnabled = parsed
	}
	cfg.OneAPIBaseURL = strings.TrimRight(strings.TrimSpace(getenv("LUCKYTV_CLOUD_ONEAPI_BASE_URL")), "/")
	cfg.OneAPIAPIKey = strings.TrimSpace(getenv("LUCKYTV_CLOUD_ONEAPI_API_KEY"))
	modelsValue := strings.TrimSpace(getenv("LUCKYTV_CLOUD_MODELS_JSON"))
	if modelsValue != "" {
		if err := json.Unmarshal([]byte(modelsValue), &cfg.HostedModels); err != nil {
			return Config{}, fmt.Errorf("LUCKYTV_CLOUD_MODELS_JSON must be valid JSON")
		}
	}

	sandboxValue := strings.TrimSpace(getenv("LUCKYTV_CLOUD_SANDBOX"))
	if sandboxValue != "" {
		parsed, err := strconv.ParseBool(sandboxValue)
		if err != nil {
			return Config{}, fmt.Errorf("LUCKYTV_CLOUD_SANDBOX must be true or false")
		}
		cfg.SandboxEnabled = parsed
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return fmt.Errorf("LUCKYTV_CLOUD_ENV must be development, test, or production")
	}
	if c.Environment == "production" {
		if c.SandboxEnabled {
			return fmt.Errorf("LUCKYTV_CLOUD_SANDBOX must be false in production")
		}
		if strings.TrimSpace(c.DatabaseURL) == "" {
			return fmt.Errorf("LUCKYTV_CLOUD_DATABASE_URL is required in production")
		}
		parsed, err := url.Parse(c.DatabaseURL)
		if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || parsed.User == nil || parsed.Fragment != "" {
			return fmt.Errorf("LUCKYTV_CLOUD_DATABASE_URL must be a PostgreSQL URL")
		}
		if strings.TrimSpace(c.WeChatAppID) == "" || strings.TrimSpace(c.WeChatAppSecret) == "" {
			return fmt.Errorf("WeChat App ID and App Secret are required in production")
		}
		callback, err := url.Parse(c.WeChatCallbackURL)
		if err != nil || callback.Scheme != "https" || callback.Host == "" || callback.User != nil || callback.RawQuery != "" || callback.Fragment != "" {
			return fmt.Errorf("LUCKYTV_CLOUD_WECHAT_CALLBACK_URL must be an HTTPS URL without credentials, query, or fragment")
		}
		if len(c.AdminBootstrapSecret) < 32 || c.AdminBootstrapEmail == "" || len(c.AdminInitialSecret) < 32 {
			return fmt.Errorf("production admin bootstrap secret, email, and initial secret are required; secrets must contain at least 32 bytes")
		}
	}
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("LUCKYTV_CLOUD_ADDR is required")
	}
	if strings.TrimSpace(c.DataDir) == "" || c.DataDir == "." {
		return fmt.Errorf("LUCKYTV_CLOUD_DATA_DIR must name an isolated data directory")
	}
	if c.HostedModelsEnabled {
		parsed, err := url.Parse(c.OneAPIBaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("LUCKYTV_CLOUD_ONEAPI_BASE_URL must be an absolute HTTP(S) URL without credentials or query")
		}
		if strings.TrimSpace(c.OneAPIAPIKey) == "" {
			return fmt.Errorf("LUCKYTV_CLOUD_ONEAPI_API_KEY is required when hosted models are enabled")
		}
		if len(c.HostedModels) == 0 {
			return fmt.Errorf("at least one hosted model is required when hosted models are enabled")
		}
		seen := make(map[string]struct{}, len(c.HostedModels))
		for _, model := range c.HostedModels {
			if err := model.Validate(); err != nil {
				return err
			}
			if _, exists := seen[model.ID]; exists {
				return fmt.Errorf("hosted model id %q is duplicated", model.ID)
			}
			seen[model.ID] = struct{}{}
		}
	}
	adminBootstrapSet := c.AdminBootstrapSecret != "" || c.AdminBootstrapEmail != "" || c.AdminInitialSecret != ""
	if adminBootstrapSet && (len(c.AdminBootstrapSecret) < 32 || c.AdminBootstrapEmail == "" || len(c.AdminInitialSecret) < 32) {
		return fmt.Errorf("admin bootstrap secret, email, and initial secret must be configured together; secrets must contain at least 32 bytes")
	}
	if c.Environment != "test" && c.EmailOTPHMACSecret != "" && len(c.EmailOTPHMACSecret) < 32 {
		return fmt.Errorf("LUCKYTV_CLOUD_EMAIL_OTP_HMAC_SECRET must contain at least 32 bytes")
	}
	if (c.SMTPHost == "") != (c.SMTPPort == "") || (c.SMTPHost == "") != (c.SMTPFrom == "") {
		return fmt.Errorf("SMTP host, port, and sender must be configured together")
	}
	if c.SMTPHost != "" {
		port, err := strconv.Atoi(c.SMTPPort)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("LUCKYTV_CLOUD_SMTP_PORT must be between 1 and 65535")
		}
	}
	if sameOrNestedPath(c.DataDir, c.LocalWorkspaceDataDir) {
		return fmt.Errorf("cloud data directory must not overlap CANVAS_BACKEND_DATA_DIR")
	}
	if !isLoopbackListenAddr(c.ListenAddr) {
		return fmt.Errorf("cloud sandbox runtime must listen on a loopback IP address")
	}
	return nil
}

func sameOrNestedPath(first, second string) bool {
	if strings.TrimSpace(second) == "" {
		return false
	}
	firstCanonical, firstErr := canonicalPath(first)
	secondCanonical, secondErr := canonicalPath(second)
	if firstErr != nil || secondErr != nil {
		return true
	}
	within := func(path, parent string) bool {
		rel, err := filepath.Rel(parent, path)
		return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
	}
	return within(firstCanonical, secondCanonical) || within(secondCanonical, firstCanonical)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	probe := abs
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(probe)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		if _, statErr := os.Lstat(probe); statErr == nil {
			return "", resolveErr
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", resolveErr
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = parent
	}
}

func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}
