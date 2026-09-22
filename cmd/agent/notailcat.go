//go:build !tailcat

package main

func checkTailcat(verbose bool, retry bool, retryDelay int, reconnect bool, reconnectDelay int, reconnectTimeout int) bool {
	return false
}
