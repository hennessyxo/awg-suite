package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
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

// KeepAlive sends an SSH keepalive request so an idle connection isn't dropped
// by the server or a NAT/firewall timeout.
func (c *Client) KeepAlive() error {
	_, _, err := c.c.SendRequest("keepalive@openssh.com", true, nil)
	return err
}

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

// WriteFile writes content to remotePath on the server (via `cat >`).
func (c *Client) WriteFile(remotePath, content string) error {
	sess, err := c.c.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(content)
	return sess.Run("cat > " + shellQuote(remotePath))
}

// Interactive runs cmd with a PTY, wiring the local terminal to it so the user
// can use a remote interactive program (e.g. the installer menu).
func (c *Client) Interactive(cmd string) error {
	sess, err := c.c.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	fd := int(os.Stdin.Fd())
	w, h := 80, 24
	if term.IsTerminal(fd) {
		if tw, th, e := term.GetSize(fd); e == nil {
			w, h = tw, th
		}
		if old, e := term.MakeRaw(fd); e == nil {
			defer func() { _ = term.Restore(fd, old) }()
		}
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", h, w, modes); err != nil {
		return err
	}
	sess.Stdin = os.Stdin
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	if err := sess.Start(cmd); err != nil {
		return err
	}
	return sess.Wait()
}
