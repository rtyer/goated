package cli

import "testing"

func TestApplySecretKey_BackspaceClearsEntireSecret(t *testing.T) {
	secret := []rune{}
	var action secretInputAction

	for _, r := range "supersecret" {
		secret, action = applySecretKey(secret, r)
		if action != secretInputAppend {
			t.Fatalf("append %q: got action %v", r, action)
		}
	}

	if got := len(secret); got != len([]rune("supersecret")) {
		t.Fatalf("before clear: got len %d", got)
	}

	secret, action = applySecretKey(secret, '\b')
	if action != secretInputClear {
		t.Fatalf("backspace action = %v, want %v", action, secretInputClear)
	}
	if len(secret) != 0 {
		t.Fatalf("backspace should clear full secret, got len %d", len(secret))
	}
}

func TestApplySecretKey_NewlineSubmitsWithoutMutatingSecret(t *testing.T) {
	secret := []rune("abc123")

	next, action := applySecretKey(secret, '\n')
	if action != secretInputSubmit {
		t.Fatalf("newline action = %v, want %v", action, secretInputSubmit)
	}
	if string(next) != "abc123" {
		t.Fatalf("newline mutated secret: got %q", string(next))
	}
}

func TestFormatMaskedSecretPrompt_IncludesLockIconAndMaskLength(t *testing.T) {
	got := formatMaskedSecretPrompt("Slack bot token (xoxb-...)", 4)
	want := "  Slack bot token (xoxb-...): 🔒 ****"
	if got != want {
		t.Fatalf("formatMaskedSecretPrompt() = %q, want %q", got, want)
	}
}
