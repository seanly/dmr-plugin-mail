package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
)

const (
	maxMailListLimit   = 200
	maxMailModifyUIDs  = 50 // per mailDelete / mailMove call
)

func (p *MailPlugin) imapConnect() (*client.Client, error) {
	addr := net.JoinHostPort(p.cfg.IMAPHost, strconv.Itoa(p.cfg.IMAPPort))
	tlsCfg := &tls.Config{ServerName: p.cfg.IMAPHost}

	var c *client.Client
	var err error
	switch strings.ToLower(strings.TrimSpace(p.cfg.IMAPTLS)) {
	case tlsNone:
		c, err = client.Dial(addr)
	case tlsStartTLS:
		c, err = client.Dial(addr)
		if err != nil {
			return nil, err
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			_ = c.Logout()
			return nil, fmt.Errorf("imap starttls: %w", err)
		}
	default: // tls (implicit)
		c, err = client.DialTLS(addr, tlsCfg)
	}
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	c.Timeout = 45 * time.Second
	if err := c.Login(p.cfg.Username, p.cfg.Password); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("imap login: %w", err)
	}
	return c, nil
}

func (p *MailPlugin) execMailList(args map[string]any) (any, error) {
	folder := strings.TrimSpace(stringArg(args, "folder"))
	if folder == "" {
		folder = p.cfg.IMAPFolderDefault
	}
	limit := intArg(args, "limit")
	if limit <= 0 {
		limit = p.cfg.ListDefaultLimit
	}
	if limit > maxMailListLimit {
		limit = maxMailListLimit
	}
	unreadOnly := boolArg(args, "unreadOnly")

	var sinceT, beforeT time.Time
	if s := strings.TrimSpace(stringArg(args, "since")); s != "" {
		t, err := parseFlexibleTime(s)
		if err != nil {
			return nil, fmt.Errorf("since: %w", err)
		}
		sinceT = imapDateUTC(t)
	}
	if s := strings.TrimSpace(stringArg(args, "before")); s != "" {
		t, err := parseFlexibleTime(s)
		if err != nil {
			return nil, fmt.Errorf("before: %w", err)
		}
		beforeT = imapDateUTC(t)
	}

	c, err := p.imapConnect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	if _, err := c.Select(folder, true); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}

	criteria := imap.NewSearchCriteria()
	if unreadOnly {
		criteria.WithoutFlags = []string{imap.SeenFlag}
	}
	if !sinceT.IsZero() {
		criteria.Since = sinceT
	}
	if !beforeT.IsZero() {
		criteria.Before = beforeT
	}

	uids, err := c.UidSearch(criteria)
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}

	slices.SortFunc(uids, func(a, b uint32) int {
		if a > b {
			return -1
		}
		if a < b {
			return 1
		}
		return 0
	})
	if len(uids) > limit {
		uids = uids[:limit]
	}

	if len(uids) == 0 {
		return map[string]any{"folder": folder, "messages": []any{}}, nil
	}

	seq := new(imap.SeqSet)
	seq.AddNum(uids...)
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, imap.FetchUid}
	ch := make(chan *imap.Message, 16)
	go func() {
		_ = c.UidFetch(seq, items, ch)
	}()

	var out []map[string]any
	for m := range ch {
		if m.Envelope == nil {
			continue
		}
		seen := false
		for _, f := range m.Flags {
			if f == imap.SeenFlag {
				seen = true
				break
			}
		}
		out = append(out, map[string]any{
			"uid":         m.Uid,
			"messageId":   strings.TrimSpace(m.Envelope.MessageId),
			"subject":     m.Envelope.Subject,
			"from":        formatAddresses(m.Envelope.From),
			"date":        m.Envelope.Date.Format(time.RFC3339),
			"internalDate": m.InternalDate.Format(time.RFC3339),
			"seen":        seen,
		})
	}

	slices.SortFunc(out, func(a, b map[string]any) int {
		ua, _ := a["uid"].(uint32)
		ub, _ := b["uid"].(uint32)
		if ua > ub {
			return -1
		}
		if ua < ub {
			return 1
		}
		return 0
	})

	return map[string]any{
		"folder":   folder,
		"limit":    limit,
		"messages": out,
	}, nil
}

func (p *MailPlugin) execMailRead(args map[string]any) (any, error) {
	folder := strings.TrimSpace(stringArg(args, "folder"))
	if folder == "" {
		folder = p.cfg.IMAPFolderDefault
	}
	maxChars := intArg(args, "max_body_chars")
	if maxChars <= 0 {
		maxChars = p.cfg.MaxBodyChars
	}

	c, err := p.imapConnect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	if _, err := c.Select(folder, true); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}

	var uid uint32
	if mid := strings.TrimSpace(stringArg(args, "messageId")); mid != "" {
		cr := imap.NewSearchCriteria()
		cr.Header.Set("Message-Id", mid)
		uids, err := c.UidSearch(cr)
		if err != nil {
			return nil, fmt.Errorf("imap search messageId: %w", err)
		}
		if len(uids) == 0 {
			return nil, fmt.Errorf("no message with Message-Id %q", mid)
		}
		uid = uids[0]
	} else if u, ok := uint32Arg(args, "uid"); ok && u > 0 {
		uid = u
	} else {
		return nil, fmt.Errorf("uid or messageId is required")
	}

	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{
		section.FetchItem(),
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchInternalDate,
		imap.FetchUid,
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)
	ch := make(chan *imap.Message, 1)
	go func() {
		_ = c.UidFetch(seq, items, ch)
	}()

	var msg *imap.Message
	for m := range ch {
		msg = m
	}
	if msg == nil {
		return nil, fmt.Errorf("message uid %d not found", uid)
	}

	lit := msg.GetBody(section)
	if lit == nil {
		return nil, fmt.Errorf("empty body for uid %d", uid)
	}

	plain, html, attachMeta, err := extractBodiesFromRFC822(lit)
	if err != nil {
		return nil, fmt.Errorf("parse body: %w", err)
	}

	body := plain
	bodyMime := "text/plain"
	if body == "" && html != "" {
		body = html
		bodyMime = "text/html"
	}
	truncated := false
	if maxChars > 0 {
		r := []rune(body)
		if len(r) > maxChars {
			body = string(r[:maxChars])
			truncated = true
		}
	}

	seen := false
	for _, f := range msg.Flags {
		if f == imap.SeenFlag {
			seen = true
			break
		}
	}

	env := msg.Envelope
	res := map[string]any{
		"folder":       folder,
		"uid":          msg.Uid,
		"seen":         seen,
		"internalDate": msg.InternalDate.Format(time.RFC3339),
		"bodyMime":     bodyMime,
		"body":         body,
		"truncated":    truncated,
		"attachments":  attachMeta,
	}
	if env != nil {
		res["messageId"] = strings.TrimSpace(env.MessageId)
		res["subject"] = env.Subject
		res["from"] = formatAddresses(env.From)
		res["to"] = formatAddresses(env.To)
		res["cc"] = formatAddresses(env.Cc)
		res["date"] = env.Date.Format(time.RFC3339)
	}
	return res, nil
}

func parseMailUIDs(args map[string]any) ([]uint32, error) {
	v, ok := args["uids"]
	if !ok || v == nil {
		return nil, fmt.Errorf("uids is required (non-empty array)")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("uids must be a JSON array of integers")
	}
	seen := make(map[uint32]struct{}, len(arr))
	var out []uint32
	for _, e := range arr {
		u, err := anyToUint32(e)
		if err != nil {
			return nil, fmt.Errorf("uids: %w", err)
		}
		if u == 0 {
			return nil, fmt.Errorf("uids: uid must be positive")
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("uids is required (non-empty array)")
	}
	if len(out) > maxMailModifyUIDs {
		return nil, fmt.Errorf("uids: at most %d per call (got %d)", maxMailModifyUIDs, len(out))
	}
	slices.Sort(out)
	return out, nil
}

func anyToUint32(e any) (uint32, error) {
	switch x := e.(type) {
	case float64:
		if x < 0 || x > 0xffffffff || x != float64(uint32(x)) {
			return 0, fmt.Errorf("invalid uid %v", e)
		}
		return uint32(x), nil
	case int:
		if x < 0 || x > 0xffffffff {
			return 0, fmt.Errorf("invalid uid %v", e)
		}
		return uint32(x), nil
	case int64:
		if x < 0 || x > 0xffffffff {
			return 0, fmt.Errorf("invalid uid %v", e)
		}
		return uint32(x), nil
	default:
		return 0, fmt.Errorf("invalid uid type %T", e)
	}
}

func (p *MailPlugin) execMailDelete(args map[string]any) (any, error) {
	folder := strings.TrimSpace(stringArg(args, "folder"))
	if folder == "" {
		folder = p.cfg.IMAPFolderDefault
	}
	uids, err := parseMailUIDs(args)
	if err != nil {
		return nil, err
	}

	c, err := p.imapConnect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	if _, err := c.Select(folder, false); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}

	seq := new(imap.SeqSet)
	seq.AddNum(uids...)
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	val := []interface{}{imap.DeletedFlag}
	if err := c.UidStore(seq, item, val, nil); err != nil {
		return nil, fmt.Errorf("imap uid store \\Deleted: %w", err)
	}
	if err := c.Expunge(nil); err != nil {
		return nil, fmt.Errorf("imap expunge: %w", err)
	}

	return map[string]any{
		"ok":     true,
		"folder": folder,
		"uids":   uids,
		"count":  len(uids),
	}, nil
}

func (p *MailPlugin) execMailMove(args map[string]any) (any, error) {
	folder := strings.TrimSpace(stringArg(args, "folder"))
	if folder == "" {
		folder = p.cfg.IMAPFolderDefault
	}
	target := strings.TrimSpace(stringArg(args, "targetFolder"))
	if target == "" {
		return nil, fmt.Errorf("targetFolder is required")
	}
	uids, err := parseMailUIDs(args)
	if err != nil {
		return nil, err
	}

	c, err := p.imapConnect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	if _, err := c.Select(folder, false); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}

	seq := new(imap.SeqSet)
	seq.AddNum(uids...)
	if err := c.UidMove(seq, target); err != nil {
		return nil, fmt.Errorf("imap uid move: %w", err)
	}

	return map[string]any{
		"ok":            true,
		"folder":        folder,
		"targetFolder":  target,
		"uids":          uids,
		"count":         len(uids),
	}, nil
}

func imapDateUTC(t time.Time) time.Time {
	y, m, d := t.In(time.UTC).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func parseFlexibleTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
		"2006-01-02 15:04:05",
	}
	var last error
	for _, l := range layouts {
		t, err := time.Parse(l, s)
		if err == nil {
			return t, nil
		}
		last = err
	}
	return time.Time{}, fmt.Errorf("parse time %q: %w", s, last)
}

func formatAddresses(addrs []*imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, a := range addrs {
		if i > 0 {
			b.WriteString(", ")
		}
		if a.PersonalName != "" {
			b.WriteString(a.PersonalName)
			b.WriteString(" ")
		}
		b.WriteString("<")
		b.WriteString(a.Address())
		b.WriteString(">")
	}
	return b.String()
}

func extractBodiesFromRFC822(r io.Reader) (plain, html string, attachments []map[string]string, err error) {
	e, rerr := message.Read(r)
	if rerr != nil && !message.IsUnknownCharset(rerr) && !message.IsUnknownEncoding(rerr) {
		return "", "", nil, rerr
	}
	walkErr := e.Walk(func(_ []int, ent *message.Entity, werr error) error {
		if werr != nil {
			return nil
		}
		mt, _, _ := ent.Header.ContentType()
		disp, dispParams, derr := ent.Header.ContentDisposition()
		if derr == nil && disp == "attachment" {
			fn := dispParams["filename"]
			if fn != "" {
				attachments = append(attachments, map[string]string{
					"filename":    fn,
					"contentType": mt,
				})
			}
		}
		if strings.HasPrefix(mt, "text/plain") && plain == "" {
			b, _ := io.ReadAll(ent.Body)
			plain = string(b)
		}
		if strings.HasPrefix(mt, "text/html") && html == "" {
			b, _ := io.ReadAll(ent.Body)
			html = string(b)
		}
		return nil
	})
	if walkErr != nil {
		return "", "", attachments, walkErr
	}
	return plain, html, attachments, nil
}
