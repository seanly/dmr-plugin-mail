# DMR opa_policy add-on for dmr-plugin-mail tools (mailSend, mailList, mailRead).
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

import future.keywords.if
import future.keywords.in

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
decision = {"action": "deny", "reason": "mailSend: recipient domain not allowed", "risk": "high"} if {
	input.tool == "mailSend"
	domain_enforcement_enabled
	recipients := mail_recipients(input.args)
	count(recipients) > 0
	some addr in recipients
	not mail_domain_allowed(addr)
}

# Otherwise outbound mail requires approval (high risk).
decision = {"action": "require_approval", "reason": reason_with_scope("mail send"), "risk": "high"} if {
	input.tool == "mailSend"
	not mail_send_domain_denied
}

# True when the deny rule above would apply (keeps require_approval mutually exclusive).
mail_send_domain_denied if {
	input.tool == "mailSend"
	domain_enforcement_enabled
	recipients := mail_recipients(input.args)
	count(recipients) > 0
	some addr in recipients
	not mail_domain_allowed(addr)
}

# ---------------------------------------------------------------------------
# mailRead (optional)
# ---------------------------------------------------------------------------

decision = {"action": "require_approval", "reason": reason_with_scope("mail read"), "risk": "medium"} if {
	input.tool == "mailRead"
	mail_read_require_approval
}
