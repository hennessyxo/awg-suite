package deploy

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client is a thin wrapper over an SSH connection to the target server.
type Client struct {
	c *ssh.Client
}

// Dial opens an SSH connection to the target.
func Dial(t Target, auth []ssh.AuthMethod, hostKey ssh.HostKeyCallback, timeout time.Duration) (*Client, error) {
	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         timeout,
	}
	c, err := ssh.Dial("tcp", t.Addr(), cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", t.Addr(), err)
	}
	return &Client{c: c}, nil
}

// Close terminates the connection.
func (c *Client) Close() error { return c.c.Close() }

// Run executes a command and returns its combined output.
func (c *Client) Run(cmd string) (string, error) {
	sess, err := c.c.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

// RunScript pipes script to cmd's stdin (e.g. `bash -s`), streaming output to w
// while also capturing and returning it.
func (c *Client) RunScript(cmd, script string, w io.Writer) (string, error) {
	sess, err := c.c.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	sess.Stdin = strings.NewReader(script)
	var buf bytes.Buffer
	mw := io.MultiWriter(w, &buf)
	sess.Stdout = mw
	sess.Stderr = mw
	err = sess.Run(cmd)
	return buf.String(), err
}
