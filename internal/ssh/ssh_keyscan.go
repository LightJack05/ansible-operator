package ssh

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var errKeyCapturedConnAborted = errors.New("key captured, connection aborted")
var errNoKeysFound = errors.New("no keys found")

const sshDialTimeout = 20 * time.Second

var sshKeyProbeGroups = [][]string{
	{ssh.KeyAlgoECDSA256},
	{ssh.KeyAlgoECDSA384},
	{ssh.KeyAlgoECDSA521},
	{ssh.KeyAlgoED25519},
	{ssh.KeyAlgoRSA, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512},
}

func scanHostWithAlgos(host string, port int, keyTypes []string) (ssh.PublicKey, error) {
	var hostKey ssh.PublicKey
	config := &ssh.ClientConfig{
		User: "nobody",
		// Callback that captures the host key and then aborts the connection
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			hostKey = key
			// Bail out so we don't trigger fail2ban etc.
			return errKeyCapturedConnAborted
		},
		HostKeyAlgorithms: keyTypes,
	}

	connection, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), sshDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}
	defer func() {
		cerr := connection.Close()
		if cerr != nil && err == nil {
			err = fmt.Errorf("failed to close connection: %w", cerr)
		}
	}()
	connection.SetDeadline(time.Now().Add(sshDialTimeout))

	sshConn, channel, request, err := ssh.NewClientConn(connection, fmt.Sprintf("%s:%d", host, port), config)
	if err == nil {
		// This shouldn't happen, but just in case...
		go ssh.DiscardRequests(request)
		for range channel {
		}
		sshConn.Close()
	}
	if hostKey == nil {
		return nil, fmt.Errorf("failed to capture host key from %s:%d: %w", host, port, err)
	}
	return hostKey, nil
}

func ScanHost(host string, port int) ([]ssh.PublicKey, error) {
	// The returned keys
	var keys []ssh.PublicKey

	type result struct {
		key ssh.PublicKey
		err error
	}

	// Create a channel for the ssh keys
	results := make(chan result, len(sshKeyProbeGroups))
	// Start a goroutine for each group of key types
	for _, group := range sshKeyProbeGroups {
		go func(group []string) {
			key, err := scanHostWithAlgos(host, port, group)
			results <- result{key: key, err: err}
		}(group)
	}

	// Collect and dedup the keys
	var seen = map[string]bool{}
	var firsterror error
	for range sshKeyProbeGroups {
		res := <-results
		if res.key == nil {
			if res.err != nil && firsterror == nil {
				firsterror = res.err
			}
			continue
		}
		key := string(res.key.Marshal())
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, res.key)
	}

	// No keys means something went wrong...
	if len(keys) == 0 {
		if firsterror != nil {
			return nil, firsterror
		}
		return nil, errNoKeysFound
	}

	return keys, nil
}

func HostKeyToString(key ssh.PublicKey) string {
	return string(ssh.MarshalAuthorizedKey(key))
}

func HostKeysToSortedString(keys []ssh.PublicKey) (string, error) {
	var keyStrings = make([]string, 0, len(keys))
	var keyString strings.Builder

	for _, key := range keys {
		keyStrings = append(keyStrings, HostKeyToString(key))
	}

	slices.Sort(keyStrings)

	for _, key := range keyStrings {
		keyString.WriteString(key)
	}

	return keyString.String(), nil
}
