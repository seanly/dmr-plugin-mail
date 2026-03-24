// mail-imap-demo: standalone IMAP check using the same stack as dmr-plugin-mail (go-imap).
//
// Default: connect + TLS + LOGIN, then LOGOUT. Optional folder listing or SELECT stats.
//
// Examples:
//
//	Aliyun / typical 993 SSL:
//	  export MAIL_IMAP_PASSWORD='your-password'
//	  go run ./cmd/mail-imap-demo/ -host imap.aliyun.com -port 993 -tls tls -user 'you@aliyun.com'
//
//	List mailboxes after login:
//	  go run ./cmd/mail-imap-demo/ -host imap.aliyun.com -user 'you@aliyun.com' -list-folders
//
//	Read-only SELECT INBOX and print message counts:
//	  go run ./cmd/mail-imap-demo/ -host imap.aliyun.com -user 'you@aliyun.com' -select INBOX
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

const (
	tlsNone     = "none"
	tlsStartTLS = "starttls"
	tlsImplicit = "tls"
)

func main() {
	log.SetFlags(0)
	host := flag.String("host", "", "IMAP host (required)")
	port := flag.Int("port", 993, "IMAP port (993 tls, 143 often starttls)")
	user := flag.String("user", "", "IMAP username (usually full email)")
	password := flag.String("password", "", "Password (or MAIL_IMAP_PASSWORD / MAIL_SMTP_PASSWORD)")
	tlsMode := flag.String("tls", "tls", "TLS: none | starttls | tls")
	listFolders := flag.Bool("list-folders", false, "After login, LIST all mailboxes")
	selectFolder := flag.String("select", "", "After login, EXAMINE (read-only) this folder and print counts (e.g. INBOX)")
	timeout := flag.Duration("timeout", 45*time.Second, "Connection / command timeout")
	flag.Parse()

	if strings.TrimSpace(*host) == "" || strings.TrimSpace(*user) == "" {
		fmt.Fprintln(os.Stderr, "usage: need -host and -user (see -h)")
		os.Exit(2)
	}
	pass := strings.TrimSpace(*password)
	if pass == "" {
		pass = os.Getenv("MAIL_IMAP_PASSWORD")
	}
	if pass == "" {
		pass = os.Getenv("MAIL_SMTP_PASSWORD")
	}
	if pass == "" {
		fmt.Fprintln(os.Stderr, "need -password or MAIL_IMAP_PASSWORD (or MAIL_SMTP_PASSWORD)")
		os.Exit(2)
	}

	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
	tlsCfg := &tls.Config{ServerName: *host}

	c, err := imapDial(addr, *tlsMode, tlsCfg)
	if err != nil {
		log.Fatalf("imap dial: %v", err)
	}
	defer func() { _ = c.Logout() }()

	c.Timeout = *timeout

	if err := c.Login(*user, pass); err != nil {
		log.Fatalf("imap login: %v", err)
	}
	fmt.Println("OK: IMAP dial + TLS + LOGIN succeeded.")

	if *listFolders {
		if err := runListFolders(c); err != nil {
			log.Fatalf("list folders: %v", err)
		}
	}

	if s := strings.TrimSpace(*selectFolder); s != "" {
		if err := runExamine(c, s); err != nil {
			log.Fatalf("examine: %v", err)
		}
	}
}

func imapDial(addr, mode string, tlsCfg *tls.Config) (*client.Client, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case tlsNone:
		return client.Dial(addr)
	case tlsStartTLS:
		c, err := client.Dial(addr)
		if err != nil {
			return nil, err
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			_ = c.Logout()
			return nil, fmt.Errorf("starttls: %w", err)
		}
		return c, nil
	default:
		return client.DialTLS(addr, tlsCfg)
	}
}

func runListFolders(c *client.Client) error {
	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", ch)
	}()
	var names []string
	for m := range ch {
		if m != nil {
			names = append(names, m.Name)
		}
	}
	if err := <-done; err != nil {
		return err
	}
	fmt.Printf("mailboxes (%d):\n", len(names))
	for _, n := range names {
		fmt.Println("  ", n)
	}
	return nil
}

func runExamine(c *client.Client, folder string) error {
	mbox, err := c.Select(folder, true)
	if err != nil {
		return err
	}
	fmt.Printf("folder %q: Messages=%d Recent=%d Unseen=%d UidNext=%d UidValidity=%d\n",
		folder,
		mbox.Messages,
		mbox.Recent,
		mbox.Unseen,
		mbox.UidNext,
		mbox.UidValidity,
	)
	return nil
}
