package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrAuthNotConfigured  = errors.New("demo auth is not configured")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRoleMismatch       = errors.New("role does not match user")
)

type credential struct {
	UserID   string
	Password string
	Role     string
}

// ValidateCredentials checks user credentials against environment-backed demo auth config.
func ValidateCredentials(userID, password string) error {
	if userID == "" || password == "" {
		return ErrInvalidCredentials
	}

	credentials := loadCredentials("DEMO_AUTH_CREDENTIALS", false)
	if len(credentials) == 0 {
		if !isKnownDemoUser(userID) {
			return ErrAuthNotConfigured
		}

		sharedPassword := strings.TrimSpace(os.Getenv("DEMO_AUTH_PASSWORD"))
		if sharedPassword == "" {
			return ErrAuthNotConfigured
		}
		if password != sharedPassword {
			return ErrInvalidCredentials
		}
		return nil
	}

	for _, cred := range credentials {
		if cred.UserID == userID && cred.Password == password {
			return nil
		}
	}

	return ErrInvalidCredentials
}

// ValidateRoleCredentials checks role-based credentials for multi-role demo login.
func ValidateRoleCredentials(userID, password, role string) error {
	if !IsValidRole(role) {
		return ErrRoleMismatch
	}

	if err := ValidateCredentials(userID, password); err != nil {
		return err
	}

	roleCredentials := loadCredentials("DEMO_AUTH_ROLE_CREDENTIALS", true)
	if len(roleCredentials) == 0 {
		if roleMatchesUserID(userID, role) {
			return nil
		}
		return ErrRoleMismatch
	}

	for _, cred := range roleCredentials {
		if cred.UserID == userID && cred.Password == password && cred.Role == role {
			return nil
		}
	}

	return ErrRoleMismatch
}

func loadCredentials(envKey string, withRole bool) []credential {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return nil
	}

	var credentials []credential
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		parts := strings.Split(item, ":")
		if withRole {
			if len(parts) != 3 {
				continue
			}
			credentials = append(credentials, credential{
				UserID:   strings.TrimSpace(parts[0]),
				Password: strings.TrimSpace(parts[1]),
				Role:     strings.TrimSpace(parts[2]),
			})
			continue
		}

		if len(parts) != 2 {
			continue
		}
		credentials = append(credentials, credential{
			UserID:   strings.TrimSpace(parts[0]),
			Password: strings.TrimSpace(parts[1]),
		})
	}

	return credentials
}

func roleMatchesUserID(userID, role string) bool {
	return strings.HasPrefix(strings.ToLower(userID), strings.ToLower(role)+"_")
}

func isKnownDemoUser(userID string) bool {
	_, exists := knownDemoUsers[userID]
	return exists
}

var knownDemoUsers = map[string]struct{}{
	"caregiver_001":   {},
	"caregiver_002":   {},
	"caregiver_wang":  {},
	"caregiver_li":    {},
	"caregiver_zhang": {},
	"caregiver_liu":   {},
	"doctor_001":      {},
	"doctor_wang":     {},
	"doctor_li":       {},
	"doctor_zhang":    {},
	"doctor_liu":      {},
	"family_001":      {},
	"family_002":      {},
	"family_003":      {},
	"family_zhang":    {},
	"family_li":       {},
	"family_wang":     {},
	"family_zhao":     {},
	"elder_001":       {},
	"elder_002":       {},
	"elder_003":       {},
	"elder_zhang":     {},
	"elder_li":        {},
	"elder_wang":      {},
	"elder_zhao":      {},
	"eval_user_001":   {}, // Experiment evaluation user
	"admin":           {},
	"demo":            {},
}

func AuthConfigHint() string {
	return fmt.Sprintf("set %s or %s", "DEMO_AUTH_PASSWORD", "DEMO_AUTH_CREDENTIALS")
}
