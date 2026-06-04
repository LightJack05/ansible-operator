package ssh

import "golang.org/x/crypto/ssh"

func ValidatePrivateSSHKey(key string) error {
	_, err := ssh.ParsePrivateKey([]byte(key))
	return err
}
