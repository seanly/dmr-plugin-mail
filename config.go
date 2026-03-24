package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	tlsNone      = "none"
	tlsStartTLS  = "starttls"
	tlsImplicit  = "tls"
	defaultLimit = 50
)

// MailConfig is loaded from InitRequest.ConfigJSON (YAML becomes map then JSON in DMR).
type MailConfig struct {
	ConfigBaseDir string `json:"config_base_dir"`
	Workspace     string `json:"workspace"`

	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	SMTPTLS  string `json:"smtp_tls"` // none | starttls | tls
	// SMTPAuth: plain (default), login, or auto — server-dependent; try auto/login if you get 535.
	SMTPAuth string `json:"smtp_auth"`

	IMAPHost string `json:"imap_host"`
	IMAPPort int    `json:"imap_port"`
	IMAPTLS  string `json:"imap_tls"`

	Username string `json:"username"`
	Password string `json:"password"`

	FromAddress       string `json:"from_address"`
	IMAPFolderDefault string `json:"imap_folder_default"`

	AttachmentRoot     string `json:"attachment_root"`
	MaxAttachmentBytes int64  `json:"max_attachment_bytes"`
	MaxBodyChars       int    `json:"max_body_chars"`
	ListDefaultLimit   int    `json:"list_default_limit"`
}

func defaultMailConfig() MailConfig {
	return MailConfig{
		SMTPPort:           587,
		SMTPTLS:            tlsStartTLS,
		IMAPPort:           993,
		IMAPTLS:            tlsImplicit,
		MaxAttachmentBytes: 30 << 20,
		MaxBodyChars:       32000,
		ListDefaultLimit:   defaultLimit,
		IMAPFolderDefault:  "INBOX",
	}
}

func parseMailConfigJSON(raw string) (MailConfig, error) {
	cfg := defaultMailConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, fmt.Errorf("mail: empty config")
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("mail: parse config: %w", err)
	}
	if cfg.SMTPTLS == "" {
		if cfg.SMTPPort == 465 {
			cfg.SMTPTLS = tlsImplicit
		} else {
			cfg.SMTPTLS = tlsStartTLS
		}
	}
	if cfg.IMAPTLS == "" {
		cfg.IMAPTLS = tlsImplicit
	}
	if cfg.ListDefaultLimit <= 0 {
		cfg.ListDefaultLimit = defaultLimit
	}
	if cfg.MaxBodyChars <= 0 {
		cfg.MaxBodyChars = 32000
	}
	if cfg.MaxAttachmentBytes <= 0 {
		cfg.MaxAttachmentBytes = 30 << 20
	}
	if strings.TrimSpace(cfg.IMAPFolderDefault) == "" {
		cfg.IMAPFolderDefault = "INBOX"
	}
	cfg.Username = strings.TrimSpace(cfg.Username)
	return cfg, nil
}

func (c MailConfig) validateAuth() error {
	if strings.TrimSpace(c.Username) == "" {
		return fmt.Errorf("mail: username is required")
	}
	if c.Password == "" {
		return fmt.Errorf("mail: password is required")
	}
	return nil
}

func (c MailConfig) validateSMTPAuthMode() error {
	s := strings.ToLower(strings.TrimSpace(c.SMTPAuth))
	switch s {
	case "", "plain", "login", "auto", "autodiscover":
		return nil
	default:
		return fmt.Errorf("mail: smtp_auth must be plain, login, or auto, got %q", c.SMTPAuth)
	}
}

func (c MailConfig) effectiveFrom() string {
	if s := strings.TrimSpace(c.FromAddress); s != "" {
		return s
	}
	return strings.TrimSpace(c.Username)
}

func (c MailConfig) validateHosts() error {
	if strings.TrimSpace(c.SMTPHost) == "" {
		return fmt.Errorf("mail: smtp_host is required")
	}
	if strings.TrimSpace(c.IMAPHost) == "" {
		return fmt.Errorf("mail: imap_host is required")
	}
	return nil
}

// resolveLocalPath resolves a path for attachments: under AttachmentRoot if set, else Workspace, else config base.
func (c MailConfig) resolveLocalPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	}
	var root string
	if r := strings.TrimSpace(c.AttachmentRoot); r != "" {
		root = r
		if !filepath.IsAbs(root) && c.ConfigBaseDir != "" {
			root = filepath.Join(c.ConfigBaseDir, root)
		}
		root = filepath.Clean(root)
	} else if w := strings.TrimSpace(c.Workspace); w != "" {
		root = filepath.Clean(w)
	} else if c.ConfigBaseDir != "" {
		root = filepath.Clean(c.ConfigBaseDir)
	}
	if !filepath.IsAbs(p) {
		if root == "" {
			var err error
			p, err = filepath.Abs(p)
			if err != nil {
				return "", err
			}
		} else {
			p = filepath.Clean(filepath.Join(root, p))
		}
	} else {
		p = filepath.Clean(p)
	}
	if root != "" {
		rel, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path escapes attachment_root: %q", p)
		}
	}
	return p, nil
}
