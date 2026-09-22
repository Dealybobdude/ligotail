//go:build tailcat

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tailscale/tailcat"
)

var (
	tailcatAddr = flag.String("tailcat", "", "connect to proxy via tailcat address (tc...)")
	tailcatPort = flag.Int("tailcat-port", 11601, "tailcat port (used with -tailcat or -tailcat-bind)")
	tailcatBind = flag.String("tailcat-bind", "", "bind mode via tailcat (specify listener port, e.g. :11601)")
)

func checkTailcat(verbose bool, retry bool, retryDelay int, reconnect bool, reconnectDelay int, reconnectTimeout int) bool {
	if *tailcatBind != "" {
		if !verbose {
			log.SetOutput(io.Discard)
		}
		tailcatBindMode(*tailcatBind, verbose)
		return true
	}
	if *tailcatAddr != "" {
		if !verbose {
			log.SetOutput(io.Discard)
		}
		tailcatConnect(tailcat.Addr(*tailcatAddr), uint16(*tailcatPort), verbose, retry, retryDelay, reconnect, reconnectDelay, reconnectTimeout)
		return true
	}
	return false
}

func tailcatConnect(addr tailcat.Addr, port uint16, verbose bool, retry bool, retryDelay int, reconnect bool, reconnectDelay int, reconnectTimeout int) {
	connectionEstablished := false
	var reconnectStartTime time.Time
	var reconnectAttempt int

	for {
		client := tailcat.NewClient(addr)
		if !verbose {
			client.Logf = func(string, ...any) {}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		conn, err := client.DialTCPPort(ctx, port)
		cancel()

		var connSuccess bool
		if err == nil {
			connSuccess, err = connect(conn)
		}

		if connSuccess {
			connectionEstablished = true
			reconnectStartTime = time.Time{}
			reconnectAttempt = 0
		}

		if err != nil {
			logrus.Errorf("Tailcat connection error: %v", err)
		}

		if !connectionEstablished {
			if retry {
				logrus.Infof("Retrying initial tailcat connection in %d seconds.", retryDelay)
				time.Sleep(time.Duration(retryDelay) * time.Second)
				continue
			} else {
				logrus.Fatal("Initial tailcat connection failed. Use -retry flag to enable automatic retry.")
			}
		} else {
			if reconnect {
				if reconnectStartTime.IsZero() {
					logrus.Warn("Tailcat connection lost. Attempting to reconnect...")
					reconnectStartTime = time.Now()
					reconnectAttempt = 0
				}

				reconnectAttempt++
				elapsed := time.Since(reconnectStartTime)

				if elapsed.Seconds() >= float64(reconnectTimeout) {
					logrus.Fatalf("Reconnection timeout reached after %d attempts over %.0f seconds. Giving up.", reconnectAttempt, elapsed.Seconds())
				}

				logrus.Infof("Reconnection attempt %d (elapsed: %.0fs/%.0fs). Waiting %d seconds...", reconnectAttempt, elapsed.Seconds(), float64(reconnectTimeout), reconnectDelay)
				time.Sleep(time.Duration(reconnectDelay) * time.Second)
			} else {
				logrus.Fatal("Tailcat connection lost and reconnect is disabled.")
			}
		}
	}
}

func tailcatBindMode(listenAddr string, verbose bool) {
	srv := &tailcat.Server{}
	if !verbose {
		srv.Logf = func(string, ...any) {}
	}

	ctx := context.Background()
	ln, err := srv.Listen(ctx, "tcp", listenAddr)
	if err != nil {
		logrus.Fatalf("tailcat bind listen error: %v", err)
	}

	logrus.Infof("Tailcat bind address: %s", srv.TailcatAddr())
	fmt.Printf("Tailcat address: %s\n", srv.TailcatAddr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			logrus.Errorf("tailcat accept error: %v", err)
			continue
		}
		logrus.Infof("Tailcat connection from: %s", conn.RemoteAddr())
		go func() {
			if _, err := connect(conn); err != nil {
				logrus.Errorf("tailcat session error: %v", err)
			}
		}()
	}
}
