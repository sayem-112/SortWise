package credentials

import (
	"errors"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const service = "Sortwise"

type Manager interface {
	Get(provider string) (key, source string, err error)
	Set(provider, key string) error
	Delete(provider string) error
}

type WindowsManager struct{}

func New() *WindowsManager { return &WindowsManager{} }

func credentialUser(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gemini":
		return "Gemini API key", nil
	case "groq":
		return "Groq API key", nil
	default:
		return "", errors.New("unsupported AI provider")
	}
}

func (m *WindowsManager) Get(provider string) (string, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "gemini" {
		if value := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")); value != "" {
			return value, "GOOGLE_API_KEY", nil
		}
		if value := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); value != "" {
			return value, "GEMINI_API_KEY", nil
		}
	} else if provider == "groq" {
		if value := strings.TrimSpace(os.Getenv("GROQ_API_KEY")); value != "" {
			return value, "GROQ_API_KEY", nil
		}
	}
	user, err := credentialUser(provider)
	if err != nil {
		return "", "none", err
	}
	value, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", "none", nil
	}
	if err != nil {
		return "", "none", err
	}
	return strings.TrimSpace(value), "windows_credential_manager", nil
}

func (m *WindowsManager) Set(provider, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("API key cannot be empty")
	}
	user, err := credentialUser(provider)
	if err != nil {
		return err
	}
	return keyring.Set(service, user, value)
}

func (m *WindowsManager) Delete(provider string) error {
	user, err := credentialUser(provider)
	if err != nil {
		return err
	}
	err = keyring.Delete(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
