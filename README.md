# dmr-plugin-mail

External [DMR](https://github.com/seanly/dmr) plugin (HashiCorp go-plugin) that exposes **SMTP send** and **IMAP** tools: `mailSend`, `mailList`, `mailRead`, `mailMove`, `mailDelete`.

## Build

From this repository (with `dmr` checked out as a sibling directory so the `replace` in `go.mod` resolves):

```bash
go build -o dmr-plugin-mail .
```

## Register in DMR

Point `plugins[].path` at the built binary.

```yaml
plugins:
  - name: mail
    enabled: true
    path: /absolute/path/to/dmr-plugin-mail
    config:
      smtp_host: smtp.office365.com
      smtp_port: 587
      smtp_tls: starttls   # none | starttls | tls — port 465 = SMTPS, use tls; 587 usually starttls
      smtp_auth: plain     # plain (default) | login | auto — if SMTP returns 535, try login or auto
      imap_host: outlook.office365.com
      imap_port: 993
      imap_tls: tls        # none | starttls | tls
      username: "user@example.com"
      password: "enc:..."   # or plaintext; DMR decrypts enc:/secret: before Init
      from_address: "user@example.com"
      imap_folder_default: INBOX
      attachment_root: ./mail-attachments   # relative to config file directory
      max_attachment_bytes: 31457280
      max_body_chars: 32000
      list_default_limit: 50
```

Paths under `attachment_root` (or workspace / `config_base_dir` when `attachment_root` is empty) are enforced for `mailSend` attachments; `..` escapes are rejected.

### Auth in config

`username` and `password` are read only from plugin config (after DMR YAML processing). Use DMR’s **`enc:`** / **`secret:`** indirection in YAML if you do not want plaintext in the file (see DMR docs on config secret encryption).

## Tools

| Tool         | Purpose |
| ------------ | ------- |
| `mailSend`   | Send mail (plain, optional HTML multipart, optional file attachments). |
| `mailList`   | IMAP `SEARCH` + `FETCH` envelope metadata; `limit` capped at **200**; optional `unreadOnly`, `since`, `before` (RFC3339 or `YYYY-MM-DD`). Each item includes **`uid`** (use with `mailMove` / `mailDelete` in the **same** `folder`). |
| `mailRead`   | Fetch one message by **`uid`** or **`messageId`**; body prefers `text/plain` then `text/html`, truncated to `max_body_chars` (per-arg or config). Uses `BODY.PEEK[]` so messages are not marked read. Returns **`uid`** for follow-up moves/deletes. |
| `mailMove`   | Move messages by IMAP UID: `folder`, **`targetFolder`**, **`uids`** (array). Up to **50** UIDs per call. Server may use RFC `MOVE` or `COPY` + `\Deleted` + `EXPUNGE`. |
| `mailDelete` | Delete by UID: **`uids`** + optional `folder`. Marks `\Deleted` then **`EXPUNGE`**. **Important:** `EXPUNGE` removes **all** messages already marked deleted in that mailbox, not only the UIDs you passed, if others were left flagged. Up to **50** UIDs per batch (each marked then one expunge). |

### Demo CLIs (local SMTP/IMAP checks)

- `go run ./cmd/mail-smtp-demo/` — TLS + SMTP auth probe or test send.
- `go run ./cmd/mail-imap-demo/` — TLS + IMAP login; optional `-list-folders` / `-select INBOX`.
- `make demo-build` / `make imap-demo-build` compile `mail-smtp-demo` and `mail-imap-demo` binaries.

## Provider notes (Gmail / Microsoft 365)

- Prefer **app passwords** or OAuth-capable flows where the provider allows; many tenants disable basic auth on IMAP/SMTP.
- Gmail: often `smtp_tls: tls` on port 465 or `starttls` on 587; IMAP typically `imap_tls: tls` on 993.
- Microsoft 365: commonly SMTP `starttls` on 587 and IMAP `tls` on 993; exact settings depend on tenant policy.

### 飞书企业邮箱 / Feishu Mail

- **`535 Error: authentication failed, system busy`** almost always means **账号/密码不被 SMTP 接受**，而不是 DMR 代码问题：常见原因包括未开启客户端 SMTP、使用了错误密码（登录密码 ≠ **客户端专用密码**）、或管理员策略限制。
- 在飞书邮箱设置里确认 **SMTP / 第三方客户端** 已开启，并按官方说明使用 **授权码或客户端密码** 填到 `password`（不要用网页登录密码，除非文档明确允许）。
- 端口 **465** 使用 **`smtp_tls: tls`**（SMTPS），不要用 `starttls`。
- 若仍 535，可依次尝试 **`smtp_auth: login`** 或 **`smtp_auth: auto`**（让库按服务器能力选择机制）。

## Operations

- Keep mailbox secrets in config (`enc:` / `secret:`), not in tool arguments.
- Tune `max_body_chars` and `list_default_limit` to keep model context small.
- IMAP has no notion of “already processed by DMR”; use unread flags, `since`, or external tracking if needed.

## OPA (`mail.rego`)

This repo ships [`mail.rego`](mail.rego) for `opa_policy`: recipient checks for **`mailSend`**, optional approval for **`mailRead`**, and by default **`require_approval`** for **`mailMove`** and **`mailDelete`** (`mail_move_require_approval` / `mail_delete_require_approval`). Copy or include it under your `policies:` path and tune the flags.

## OPA example (`mailSend`)

Sending mail is high risk. Extend your Rego policy (e.g. alongside DMR’s `plugins/opapolicy/policies/default.rego`) to require human approval for `mailSend`, or to deny unless recipients are internal:

```rego
# Require approval for any outbound email tool call
decision = {"action": "require_approval", "reason": "mail send", "risk": "high"} if {
    input.tool == "mailSend"
}

# Example: deny when any recipient is outside your domain (adjust domain)
decision = {"action": "deny", "reason": "external recipient not allowed", "risk": "high"} if {
    input.tool == "mailSend"
    to := input.args.to
    is_array(to)
    some addr in to
    is_string(addr)
    not endswith(addr, "@example.com")
}
```

`input.args` mirrors the JSON object passed to the tool (`to`, `cc`, `subject`, etc.).

## Development

```bash
go test ./...
```

The module uses `replace github.com/seanly/dmr => ../dmr` for local development; publish or vendor as needed for your layout.
