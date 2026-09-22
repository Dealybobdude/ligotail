//go:build !tailcat

package controller

import "errors"

func (c *Controller) ListenTailcat(port uint16, verbose bool) error {
	return errors.New("tailcat support not compiled in (build with -tags tailcat)")
}

func (c *Controller) TailcatAddr() string {
	return ""
}
