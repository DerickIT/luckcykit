package identity

import (
	"context"
	"strings"
)

type ProviderIdentity struct {
	Provider string
	Subject  string
}

type Provider interface {
	Resolve(context.Context, string) (ProviderIdentity, error)
}

type SandboxProvider struct{}

func (SandboxProvider) Resolve(_ context.Context, subject string) (ProviderIdentity, error) {
	subject = strings.TrimSpace(subject)
	if len(subject) == 0 || len(subject) > 80 {
		return ProviderIdentity{}, ErrInvalidSubject
	}
	return ProviderIdentity{Provider: "sandbox", Subject: subject}, nil
}
