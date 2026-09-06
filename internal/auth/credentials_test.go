package auth

import "testing"

func TestValidateCredentialsWithExplicitCredentials(t *testing.T) {
	t.Setenv("DEMO_AUTH_CREDENTIALS", "demo:s3cret,admin:adminpass")
	t.Setenv("DEMO_AUTH_PASSWORD", "")

	if err := ValidateCredentials("demo", "s3cret"); err != nil {
		t.Fatalf("expected credential validation to pass, got %v", err)
	}

	if err := ValidateCredentials("demo", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestValidateCredentialsWithSharedDemoPassword(t *testing.T) {
	t.Setenv("DEMO_AUTH_CREDENTIALS", "")
	t.Setenv("DEMO_AUTH_PASSWORD", "shared-pass")

	if err := ValidateCredentials("doctor_001", "shared-pass"); err != nil {
		t.Fatalf("expected shared password validation to pass, got %v", err)
	}

	if err := ValidateCredentials("unknown_user", "shared-pass"); err != ErrAuthNotConfigured {
		t.Fatalf("expected ErrAuthNotConfigured for unknown demo user, got %v", err)
	}
}

func TestValidateRoleCredentials(t *testing.T) {
	t.Setenv("DEMO_AUTH_CREDENTIALS", "doctor_001:doctor-pass")
	t.Setenv("DEMO_AUTH_ROLE_CREDENTIALS", "doctor_001:doctor-pass:doctor")

	if err := ValidateRoleCredentials("doctor_001", "doctor-pass", "doctor"); err != nil {
		t.Fatalf("expected role validation to pass, got %v", err)
	}

	if err := ValidateRoleCredentials("doctor_001", "doctor-pass", "family"); err != ErrRoleMismatch {
		t.Fatalf("expected ErrRoleMismatch, got %v", err)
	}
}
