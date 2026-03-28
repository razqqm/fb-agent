package embedded

import _ "embed"

//go:embed enrich.lua
var EnrichLua []byte

//go:embed parsers-custom.conf
var ParsersConf []byte

//go:embed fluent-bit.conf.tmpl
var FluentBitConfTmpl []byte

//go:embed fb-agent.service.tmpl
var FBAgentServiceTmpl []byte
