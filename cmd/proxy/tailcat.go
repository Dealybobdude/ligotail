//go:build tailcat

package main

import (
	"flag"
	"io"
	"log"

	"github.com/nicocha30/ligolo-ng/pkg/controller"
	"github.com/sirupsen/logrus"
)

var (
	enableTailcat = flag.Bool("tailcat", false, "also listen via tailcat (WireGuard + NAT traversal)")
	tailcatPort   = flag.Int("tailcat-port", 11601, "tailcat listener port")
)

func startTailcatIfEnabled(c *controller.Controller, verbose bool) {
	if !*enableTailcat {
		return
	}
	if !verbose {
		log.SetOutput(io.Discard)
	}
	if err := c.ListenTailcat(uint16(*tailcatPort), verbose); err != nil {
		logrus.Fatalf("tailcat listener failed: %v", err)
	}
}
