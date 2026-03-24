package main

import (
	"testing"

	"github.com/wneessen/go-mail"
)

func TestSmtpAuthMechanism(t *testing.T) {
	if smtpAuthMechanism("") != mail.SMTPAuthPlain {
		t.Fatal("empty -> plain")
	}
	if smtpAuthMechanism("login") != mail.SMTPAuthLogin {
		t.Fatal("login")
	}
	if smtpAuthMechanism("auto") != mail.SMTPAuthAutoDiscover {
		t.Fatal("auto")
	}
}

func TestSmtpEffectiveTLS(t *testing.T) {
	if got := smtpEffectiveTLS(465, "starttls"); got != tlsImplicit {
		t.Fatalf("465+starttls want %q got %q", tlsImplicit, got)
	}
	if got := smtpEffectiveTLS(465, "tls"); got != tlsImplicit {
		t.Fatalf("465+tls want %q got %q", tlsImplicit, got)
	}
	if got := smtpEffectiveTLS(587, "starttls"); got != tlsStartTLS {
		t.Fatalf("587+starttls want %q got %q", tlsStartTLS, got)
	}
}
