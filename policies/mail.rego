# DMR opa_policy add-on for dmr-plugin-mail tools (mailSend, mailList, mailRead, mailMove, mailDelete).
#
# Load next to the embedded default policy, e.g. in your DMR YAML:
#
#   plugins:
#     - name: opa_policy
#       enabled: true
#       config:
#         policies:
#           - /absolute/path/to/dmr-plugin-mail/mail.rego
#
# input.tool / input.args match the tool name and JSON arguments. input.context has tape, workspace.
# If default.rego is loaded in the same bundle, helpers like reason_with_scope exist in package dmr.

package dmr

# ---------------------------------------------------------------------------
# Tunables (edit for your org)
# ---------------------------------------------------------------------------

# SMTP recipients (to + cc + bcc) must use addresses in these domains (lowercase, no leading @).
# Leave empty to skip domain checks; mailSend still uses require_approval below.
allowed_recipient_domains := {
	"example.com",
}

# If true, mailRead calls need human approval (mailList stays on default.rego only).
mail_read_require_approval := false

# IMAP write tools: strongly recommended true (destructive / high impact).
mail_move_require_approval := true
mail_delete_require_approval := true

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

domain_enforcement_enabled if {
	count(allowed_recipient_domains) > 0
}

# Collect string addresses from to / cc / bcc (arrays only).
mail_recipients(args) := {addr |
	some field in ["to", "cc", "bcc"]
	raw := object.get(args, field, [])
	is_array(raw)
	some addr in raw
	is_string(addr)
}

recipient_domain_lower(addr) := d if {
	parts := split(addr, "@")
	count(parts) == 2
	d := lower(parts[1])
}

mail_domain_allowed(addr) if {
	d := recipient_domain_lower(addr)
	some dom in allowed_recipient_domains
	d == dom
}

# ---------------------------------------------------------------------------
# mailSend
# ---------------------------------------------------------------------------

# Deny when domain allowlist is enabled and any recipient domain is not allowed.
decision := {"action": "deny", "reason": "mailSend: recipient domain not allowed", "risk": "high"} if {
	input.tool == "mailSend"
	domain_enforcement_enabled
	recipients := mail_recipients(input.args)
	count(recipients) > 0
	some addr in recipients
	not mail_domain_allowed(addr)
}

# Otherwise outbound mail requires approval (high risk).
decision := {"action": "require_approval", "reason": reason_with_scope("mail send"), "risk": "high"} if {
	input.tool == "mailSend"
}

# ---------------------------------------------------------------------------
# mailRead (optional)
# ---------------------------------------------------------------------------

decision := {"action": "require_approval", "reason": reason_with_scope("mail read"), "risk": "medium"} if {
	input.tool == "mailRead"
	mail_read_require_approval
}

# ---------------------------------------------------------------------------
# mailMove / mailDelete (IMAP)
# ---------------------------------------------------------------------------

decision := {"action": "require_approval", "reason": reason_with_scope("mail move"), "risk": "high"} if {
	input.tool == "mailMove"
	mail_move_require_approval
}

decision := {"action": "require_approval", "reason": reason_with_scope("mail delete"), "risk": "high"} if {
	input.tool == "mailDelete"
	mail_delete_require_approval
}

# Optional stricter batch cap (example — keep mutually exclusive with rules above):
#   mail_modify_uids(args) := count(object.get(args, "uids", [])) if { is_array(object.get(args, "uids", [])) }
#   decision = {"action": "deny", ...} if { input.tool == "mailMove"; mail_modify_uids(input.args) > 10 }
