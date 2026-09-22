//go:build tailcat

package controller

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tailscale/tailcat"
)

var tailcatServer *tailcat.Server

func (c *Controller) ListenTailcat(port uint16, verbose bool) error {
	tailcatServer = &tailcat.Server{}
	if !verbose {
		tailcatServer.Logf = func(string, ...any) {}
	}

	ctx := context.Background()
	ln, err := tailcatServer.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}

	logrus.Infof("Tailcat address: %s", tailcatServer.TailcatAddr())

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				logrus.Errorf("tailcat accept error: %v", err)
				return
			}
			c.Connection <- conn
		}
	}()
	return nil
}

func (c *Controller) TailcatAddr() string {
	if tailcatServer == nil {
		return ""
	}
	return string(tailcatServer.TailcatAddr())
}
