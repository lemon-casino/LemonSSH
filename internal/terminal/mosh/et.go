package mosh

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// ET 6.2.10 SshSetupHandler.cpp / TerminalMain.cpp: etterminal accepts
// id/passkey_TERM on stdin and replies IDPASSKEY:<16 chars>/<32 chars>.
// et has no --id/--passkey options. Its SSH bootstrap consumes this reply.
// Never include the response in errors or logs.
func ETBootstrapInput() (string, error) {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	id := "XXX" + hex.EncodeToString(data[:])[:13]
	return id + "/" + hex.EncodeToString(data[8:]) + "_xterm-256color\n", nil
}

func ETServerCommand(path, fifo string) string {
	if path == "" {
		path = "etterminal"
	}
	command := shellQuote(path)
	if fifo != "" {
		command += " --serverfifo=" + shellQuote(fifo)
	}
	return command
}

func ReadETConnect(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 64*1024))
	for scanner.Scan() {
		line := scanner.Text()
		index := strings.Index(line, "IDPASSKEY:")
		if index < 0 {
			continue
		}
		pair := strings.TrimSpace(line[index+10:])
		if len(pair) != 49 || pair[16] != '/' {
			return "", errors.New("et: invalid bootstrap response")
		}
		for i, b := range []byte(pair) {
			if i != 16 && !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9') {
				return "", errors.New("et: invalid bootstrap response")
			}
		}
		return pair, nil
	}
	return "", errors.New("et: bootstrap did not return IDPASSKEY")
}
