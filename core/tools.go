//go:build tools

// Package tools pins golang.org/x/mobile, which `gomobile bind` needs to find in this module.
package tools

import _ "golang.org/x/mobile/bind"
