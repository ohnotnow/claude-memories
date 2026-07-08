package main

import _ "embed"

// recceInstructions is handed to MCP clients as the server-level instructions
// (main.go wires it into ServerOptions). It describes the session-start recce
// workflow so the behaviour ships with the server rather than depending on
// each user's hand-edited prompt. See PRD §4.3.
//
//go:embed recce.md
var recceInstructions string
