// mail-smtp-demo: standalone SMTP check using the same stack as dmr-plugin-mail (go-mail).
//
// Default: connect + TLS + AUTH only (no MAIL/DATA). Use -send to deliver one test message.
//
// Examples:
//
//	Feishu 465 SMTPS (password from env to avoid shell history):
//	  export MAIL_SMTP_PASSWORD='your-client-password'
//	  go run ./cmd/mail-smtp-demo/ -host smtp.feishu.cn -port 465 -tls tls -user 'you@g7.com.cn' -probe
//
//	Office365-style 587 STARTTLS:
//	  go run ./cmd/mail-smtp-demo/ -host smtp.office365.com -port 587 -tls starttls -user you@outlook.com -send -from you@outlook.com -to you@outlook.com
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wneessen/go-mail"
)

const (
	tlsNone     = "none"
	tlsStartTLS = "starttls"
	tlsImplicit = "tls"
)

func main() {
	host := flag.String("host", "", "SMTP host (required)")
	port := flag.Int("port", 587, "SMTP port")
	user := flag.String("user", "", "SMTP username (usually full email)")
	password := flag.String("password", "", "SMTP password (or set MAIL_SMTP_PASSWORD)")
	tlsMode := flag.String("tls", "starttls", "TLS: none | starttls | tls (465 usually tls)")
	authMode := flag.String("auth", "plain", "Auth: plain | login | auto")
	probe := flag.Bool("probe", true, "Only connect+TLS+AUTH; no email sent")
	send := flag.Bool("send", false, "Send one test email (implies -probe=false)")
	from := flag.String("from", "", "From address (-send); default: -user")
	to := flag.String("to", "", "To address (-send, required with -send)")
	subject := flag.String("subject", "dmr-plugin-mail smtp demo", "Subject (-send)")
	body := flag.String("body", "This is a test message from cmd/mail-smtp-demo.", "Body (-send)")
	timeout := flag.Duration("timeout", 45*time.Second, "Dial/send timeout")
	flag.Parse()

	if strings.TrimSpace(*host) == "" || strings.TrimSpace(*user) == "" {
		fmt.Fprintln(os.Stderr, "usage: need -host and -user (see -h)")
		os.Exit(2)
	}
	pass := *password
	if pass == "" {
		pass = os.Getenv("MAIL_SMTP_PASSWORD")
	}
	if pass == "" {
		fmt.Fprintln(os.Stderr, "need -password or MAIL_SMTP_PASSWORD")
		os.Exit(2)
	}

	doSend := *send
	if doSend && strings.TrimSpace(*to) == "" {
		fmt.Fprintln(os.Stderr, "-send requires -to")
		os.Exit(2)
	}

	opts := smtpOptions(*port, *tlsMode, *authMode, *user, pass)
	client, err := mail.NewClient(*host, opts...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NewClient:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if doSend {
		if err := sendTest(ctx, client, *from, *user, *to, *subject, *body); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("OK: test email sent.")
		return
	}
	if *probe {
		if err := client.DialWithContext(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "DialWithContext (TLS+AUTH):", err)
			os.Exit(1)
		}
		fmt.Println("OK: SMTP dial + TLS + AUTH succeeded (no mail sent).")
		return
	}
	fmt.Fprintln(os.Stderr, "nothing to do: use default probe, or -send with -to (do not pass -probe=false alone)")
	os.Exit(2)

}

func sendTest(ctx context.Context, client *mail.Client, from, user, to, subject, body string) error {
	fromAddr := strings.TrimSpace(from)
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(user)
	}
	msg := mail.NewMsg()
	if err := msg.From(fromAddr); err != nil {
		return fmt.Errorf("From: %w", err)
	}
	if err := msg.To(strings.TrimSpace(to)); err != nil {
		return fmt.Errorf("To: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("DialAndSendWithContext: %w", err)
	}
	return nil
}

func smtpEffectiveTLS(port int, configured string) string {
	t := strings.ToLower(strings.TrimSpace(configured))
	if port == 465 && t == tlsStartTLS {
		return tlsImplicit
	}
	return t
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

func smtpOptions(port int, tlsMode, authMode, user, pass string) []mail.Option {
	opts := []mail.Option{
		mail.WithPort(port),
		mail.WithSMTPAuth(smtpAuthMechanism(authMode)),
		mail.WithUsername(user),
		mail.WithPassword(pass),
		mail.WithTimeout(30 * time.Second),
	}
	switch smtpEffectiveTLS(port, tlsMode) {
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
