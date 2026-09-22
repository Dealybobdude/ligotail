//go:build !tailcat

package main

import "github.com/nicocha30/ligolo-ng/pkg/controller"

func startTailcatIfEnabled(c *controller.Controller, verbose bool) {}
