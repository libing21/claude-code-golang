package model

import (
	"net/url"
	"os"
	"strings"
)

type APIProvider string

const (
	ProviderFirstParty APIProvider = "firstParty"
	ProviderBedrock    APIProvider = "bedrock"
	ProviderVertex     APIProvider = "vertex"
	ProviderFoundry    APIProvider = "foundry"
)

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func GetAPIProvider() APIProvider {
	switch {
	case envTruthy("CLAUDE_CODE_USE_BEDROCK"):
		return ProviderBedrock
	case envTruthy("CLAUDE_CODE_USE_VERTEX"):
		return ProviderVertex
	case envTruthy("CLAUDE_CODE_USE_FOUNDRY"):
		return ProviderFoundry
	default:
		return ProviderFirstParty
	}
}

// Mirrors the external-user branch of the TS implementation.
func IsFirstPartyAnthropicBaseURL() bool {
	baseURL := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
	if baseURL == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return host == "api.anthropic.com"
}

