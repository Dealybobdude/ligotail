//go:build tailcat

package app

import (
	"context"
	"errors"
	"time"

	"github.com/desertbit/grumble"
	"github.com/hashicorp/yamux"
	"github.com/nicocha30/ligolo-ng/pkg/controller"
	"github.com/sirupsen/logrus"
	"github.com/tailscale/tailcat"
)

func registerTailcatCommands() {
	App.AddCommand(&grumble.Command{
		Name:  "tailcat_address",
		Help:  "Show the proxy's tailcat address (if tailcat is enabled)",
		Usage: "tailcat_address",
		Run: func(c *grumble.Context) error {
			addr := ProxyController.TailcatAddr()
			if addr == "" {
				return errors.New("tailcat is not enabled (start proxy with --tailcat)")
			}
			logrus.Printf("Tailcat address: %s\n", addr)
			return nil
		},
	})

	App.AddCommand(&grumble.Command{
		Name:  "connect_agent_tailcat",
		Help:  "Connect to a bind-mode agent via its tailcat address",
		Usage: "connect_agent_tailcat --addr tc... [--port 11601]",
		Flags: func(f *grumble.Flags) {
			f.StringL("addr", "", "The agent's tailcat address (tc...)")
			f.IntL("port", 11601, "The agent's tailcat listener port")
		},
		Run: func(c *grumble.Context) error {
			addr := c.Flags.String("addr")
			if addr == "" {
				return errors.New("please specify the agent's tailcat address with --addr")
			}
			port := c.Flags.Int("port")

			client := tailcat.NewClient(tailcat.Addr(addr))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			remoteConn, err := client.DialTCPPort(ctx, uint16(port))
			if err != nil {
				return err
			}

			yamuxConn, err := yamux.Client(remoteConn, nil)
			if err != nil {
				return err
			}

			agent, err := controller.NewAgent(yamuxConn)
			if err != nil {
				logrus.Errorf("could not register agent, error: %v", err)
				return err
			}

			logrus.WithFields(logrus.Fields{"name": agent.Name, "id": agent.SessionID}).Info("Agent connected via tailcat.")

			if err := RegisterAgent(agent); err != nil {
				logrus.Errorf("could not register agent: %s", err.Error())
			}
			return nil
		},
	})
}
