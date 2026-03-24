package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wneessen/go-mail"
)

func (p *MailPlugin) execMailSend(args map[string]any) (any, error) {
	to := stringSliceArg(args, "to")
	if len(to) == 0 {
		return nil, fmt.Errorf("to is required (non-empty list of addresses)")
	}
	subject := strings.TrimSpace(stringArg(args, "subject"))
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	textBody := stringArg(args, "textBody")
	htmlBody := stringArg(args, "htmlBody")
	if strings.TrimSpace(textBody) == "" && strings.TrimSpace(htmlBody) == "" {
		return nil, fmt.Errorf("textBody or htmlBody is required")
	}

	from := p.cfg.effectiveFrom()
	if from == "" {
		return nil, fmt.Errorf("from_address or username is required for sending")
	}

	cc := stringSliceArg(args, "cc")
	bcc := stringSliceArg(args, "bcc")
	attachRaw := stringSliceArg(args, "attachments")

	opts := p.smtpClientOpts()
	client, err := mail.NewClient(p.cfg.SMTPHost, opts...)
	if err != nil {
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	msg := mail.NewMsg()
	if err := msg.From(from); err != nil {
		return nil, err
	}
	toAdd := make([]string, 0, len(to))
	for _, a := range to {
		if s := strings.TrimSpace(a); s != "" {
			toAdd = append(toAdd, s)
		}
	}
	if err := msg.To(toAdd...); err != nil {
		return nil, err
	}
	var ccAdd []string
	for _, a := range cc {
		if s := strings.TrimSpace(a); s != "" {
			ccAdd = append(ccAdd, s)
		}
	}
	if len(ccAdd) > 0 {
		if err := msg.Cc(ccAdd...); err != nil {
			return nil, err
		}
	}
	var bccAdd []string
	for _, a := range bcc {
		if s := strings.TrimSpace(a); s != "" {
			bccAdd = append(bccAdd, s)
		}
	}
	if len(bccAdd) > 0 {
		if err := msg.Bcc(bccAdd...); err != nil {
			return nil, err
		}
	}
	msg.Subject(subject)

	if strings.TrimSpace(htmlBody) != "" && strings.TrimSpace(textBody) != "" {
		msg.SetBodyString(mail.TypeTextPlain, textBody)
		msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)
	} else if strings.TrimSpace(htmlBody) != "" {
		msg.SetBodyString(mail.TypeTextHTML, htmlBody)
	} else {
		msg.SetBodyString(mail.TypeTextPlain, textBody)
	}

	maxA := p.cfg.MaxAttachmentBytes
	for _, rel := range attachRaw {
		path, err := p.cfg.resolveLocalPath(rel)
		if err != nil {
			return nil, fmt.Errorf("attachment %q: %w", rel, err)
		}
		st, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("attachment %q: %w", rel, err)
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("attachment %q: not a regular file", rel)
		}
		if maxA > 0 && st.Size() > maxA {
			return nil, fmt.Errorf("attachment %q: size %d exceeds max_attachment_bytes", rel, st.Size())
		}
		msg.AttachFile(path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	return map[string]any{
		"ok":       true,
		"to":       to,
		"subject":  subject,
		"from":     from,
		"attached": len(attachRaw),
	}, nil
}

func smtpAuthMechanism(configured string) mail.SMTPAuthType {
	switch strings.ToLower(strings.TrimSpace(configured)) {
	case "login":
		return mail.SMTPAuthLogin
	case "auto", "autodiscover":
		return mail.SMTPAuthAutoDiscover
	default:
		return mail.SMTPAuthPlain
	}
}

// smtpEffectiveTLS maps config to the TLS mode go-mail should use.
// Port 465 is SMTPS (implicit TLS); STARTTLS on 465 fails on most servers (e.g. Feishu).
func smtpEffectiveTLS(port int, configured string) string {
	t := strings.ToLower(strings.TrimSpace(configured))
	if port == 465 && t == tlsStartTLS {
		return tlsImplicit
	}
	return t
}

func (p *MailPlugin) smtpClientOpts() []mail.Option {
	cfg := p.cfg
	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
		mail.WithSMTPAuth(smtpAuthMechanism(cfg.SMTPAuth)),
		mail.WithUsername(p.cfg.Username),
		mail.WithPassword(p.cfg.Password),
		mail.WithTimeout(30 * time.Second),
	}
	switch smtpEffectiveTLS(cfg.SMTPPort, cfg.SMTPTLS) {
	case tlsNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	case tlsStartTLS:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSOpportunistic))
	case tlsImplicit:
		opts = append(opts, mail.WithSSL())
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSOpportunistic))
	}
	return opts
}

func stringArg(m map[string]any, k string) string {
	v, ok := m[k].(string)
	if !ok {
		return ""
	}
	return v
}

func stringSliceArg(m map[string]any, k string) []string {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		parts := strings.Split(x, ",")
		var out []string
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return x
	default:
		return nil
	}
}

func intArg(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func uint32Arg(m map[string]any, k string) (uint32, bool) {
	switch v := m[k].(type) {
	case float64:
		if v < 0 || v > 0xffffffff {
			return 0, false
		}
		return uint32(v), true
	case int:
		if v < 0 || v > 0xffffffff {
			return 0, false
		}
		return uint32(v), true
	case int64:
		if v < 0 || v > 0xffffffff {
			return 0, false
		}
		return uint32(v), true
	default:
		return 0, false
	}
}

func boolArg(m map[string]any, k string) bool {
	switch v := m[k].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	default:
		return false
	}
}
