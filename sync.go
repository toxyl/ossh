package main

import (
	"bytes"
	"fmt"

	"golang.org/x/crypto/ssh"
)

func executeSSHCommand(host string, port int, usr, pwd, cmd string) string {
	config := &ssh.ClientConfig{
		User: usr,
		Auth: []ssh.AuthMethod{
			ssh.Password(pwd),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		Log('x', "Executing SSH command '%s' on %s failed, dial error: %s\n",
			colorWrap(cmd, 229),
			colorWrap(fmt.Sprintf("%s:%d", host, port), 229),
			colorWrap(err.Error(), 51),
		)
		return ""
	}
	session, err := conn.NewSession()
	if err != nil {
		Log('x', "Executing SSH command '%s' on %s failed, session error: %s\n",
			colorWrap(cmd, 229),
			colorWrap(fmt.Sprintf("%s:%d", host, port), 229),
			colorWrap(err.Error(), 51),
		)
		return ""
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf
	err = session.Run(cmd)
	if err != nil && err.Error() != "wait: remote command exited without exit status or exit signal" {
		Log('x', "Executing SSH command '%s' on %s failed, command error: %s\n",
			colorWrap(cmd, 229),
			colorWrap(fmt.Sprintf("%s:%d", host, port), 229),
			colorWrap(err.Error(), 51),
		)
		return ""
	}
	return buf.String()
}
