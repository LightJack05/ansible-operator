package ssh

import "golang.org/x/crypto/ssh"

func ValidatePrivateSSHKey(key string) (bool, error) {
	_, err := ssh.ParsePrivateKey([]byte(key))
	return err == nil, err
}
