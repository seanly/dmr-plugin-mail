// dmr-plugin-mail is an external DMR plugin for SMTP send and IMAP list/read.
package main

import (
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/seanly/dmr/pkg/plugin/proto"
)

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: proto.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dmr-plugin": &proto.DMRPlugin{Impl: NewMailPlugin()},
		},
	})
}
