package main

import (
	"encoding/json"
	"fmt"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

// MailPlugin implements proto.DMRPluginInterface (SMTP send + IMAP list/read/move/delete).
type MailPlugin struct {
	cfg MailConfig
}

func NewMailPlugin() *MailPlugin {
	return &MailPlugin{}
}

func (p *MailPlugin) Init(req *proto.InitRequest, resp *proto.InitResponse) error {
	cfg, err := parseMailConfigJSON(req.ConfigJSON)
	if err != nil {
		return err
	}
	if err := cfg.validateHosts(); err != nil {
		return err
	}
	if err := cfg.validateAuth(); err != nil {
		return err
	}
	if err := cfg.validateSMTPAuthMode(); err != nil {
		return err
	}
	p.cfg = cfg
	return nil
}

func (p *MailPlugin) Shutdown(req *proto.ShutdownRequest, resp *proto.ShutdownResponse) error {
	return nil
}

func (p *MailPlugin) RequestApproval(req *proto.ApprovalRequest, resp *proto.ApprovalResult) error {
	resp.Choice = 0
	resp.Comment = "mail plugin does not handle approvals"
	return nil
}

func (p *MailPlugin) RequestBatchApproval(req *proto.BatchApprovalRequest, resp *proto.BatchApprovalResult) error {
	resp.Choice = 0
	return nil
}

func (p *MailPlugin) ProvideTools(req *proto.ProvideToolsRequest, resp *proto.ProvideToolsResponse) error {
	resp.Tools = []proto.ToolDef{
		{
			Name:           "mailSend",
			Description:    "Send email via configured SMTP (plain + optional HTML multipart, optional attachments under attachment_root).",
			ParametersJSON: `{"type": "object", "properties": {"to": {"type": "array", "items": {"type": "string"}, "description": "Recipient addresses"}, "cc": {"type": "array", "items": {"type": "string"}, "description": "Optional CC"}, "bcc": {"type": "array", "items": {"type": "string"}, "description": "Optional BCC"}, "subject": {"type": "string"}, "textBody": {"type": "string", "description": "Plain text body"}, "htmlBody": {"type": "string", "description": "Optional HTML body (multipart if both text and html)"}, "attachments": {"type": "array", "items": {"type": "string"}, "description": "Paths resolved under attachment_root / workspace"}}, "required": ["to", "subject"]}`,
			Group:          "extended",
			SearchHint:     "mail, email, send, smtp, message, 邮件, 发送, 邮箱",
		},
		{
			Name:           "mailList",
			Description:    "List messages in an IMAP folder using SEARCH (unread, date range); returns summaries capped by limit (max 200).",
			ParametersJSON: `{"type": "object", "properties": {"folder": {"type": "string", "description": "IMAP mailbox name, default INBOX"}, "limit": {"type": "integer", "description": "Max messages (default from config, hard cap 200)"}, "since": {"type": "string", "description": "RFC3339 or YYYY-MM-DD; internal SINCE"}, "before": {"type": "string", "description": "RFC3339 or YYYY-MM-DD; internal BEFORE"}, "unreadOnly": {"type": "boolean", "description": "If true, SEARCH UNSEEN"}}}`,
			Group:          "extended",
			SearchHint:     "mail, email, list, inbox, unread, imap, 邮件, 列表, 收件箱, 未读",
		},
		{
			Name:           "mailRead",
			Description:    "Read one message by IMAP UID or Message-Id; body truncated to max_body_chars (default from config).",
			ParametersJSON: `{"type": "object", "properties": {"folder": {"type": "string"}, "uid": {"type": "integer", "description": "IMAP UID"}, "messageId": {"type": "string", "description": "Alternative to uid; exact Message-ID header value"}, "max_body_chars": {"type": "integer", "description": "Override config max_body_chars"}}}`,
			Group:          "extended",
			SearchHint:     "mail, email, read, message, content, body, 邮件, 读取, 内容",
		},
		{
			Name:           "mailMove",
			Description:    "Move messages by IMAP UID from folder to targetFolder (RFC MOVE or COPY+DELETE fallback). UIDs come from mailList/mailRead for the same folder. At most 50 UIDs per call.",
			ParametersJSON: `{"type": "object", "properties": {"folder": {"type": "string", "description": "Source mailbox; default from config"}, "targetFolder": {"type": "string", "description": "Destination mailbox name (e.g. Trash, Junk)"}, "uids": {"type": "array", "items": {"type": "integer"}, "description": "IMAP UIDs to move"}}, "required": ["targetFolder", "uids"]}`,
			Group:          "extended",
			SearchHint:     "mail, email, move, folder, trash, junk, 邮件, 移动, 文件夹",
		},
		{
			Name:           "mailDelete",
			Description:    "Permanently delete messages by IMAP UID: marks \\Deleted then EXPUNGE. May expunge other messages already marked deleted in the same folder. UIDs from mailList/mailRead. At most 50 UIDs per call.",
			ParametersJSON: `{"type": "object", "properties": {"folder": {"type": "string", "description": "Mailbox; default from config"}, "uids": {"type": "array", "items": {"type": "integer"}, "description": "IMAP UIDs to delete"}}, "required": ["uids"]}`,
			Group:          "extended",
			SearchHint:     "mail, email, delete, remove, expunge, 邮件, 删除, 移除",
		},
	}
	return nil
}

func (p *MailPlugin) CallTool(req *proto.CallToolRequest, resp *proto.CallToolResponse) error {
	var args map[string]any
	if err := json.Unmarshal([]byte(req.ArgsJSON), &args); err != nil {
		resp.Error = fmt.Sprintf("parse args: %v", err)
		return nil
	}

	var result any
	var err error
	switch req.Name {
	case "mailSend":
		result, err = p.execMailSend(args)
	case "mailList":
		result, err = p.execMailList(args)
	case "mailRead":
		result, err = p.execMailRead(args)
	case "mailMove":
		result, err = p.execMailMove(args)
	case "mailDelete":
		result, err = p.execMailDelete(args)
	default:
		err = fmt.Errorf("unknown tool: %s", req.Name)
	}
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	b, _ := json.Marshal(result)
	resp.ResultJSON = string(b)
	return nil
}
