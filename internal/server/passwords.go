package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

func (a *App) passwordKey() []byte {
	k := sha256.Sum256([]byte("tempmail-passwords:" + a.cfg.JWTSecret))
	return k[:]
}

func (a *App) encryptUserPassword(plain string) (string, error) {
	block, err := aes.NewCipher(a.passwordKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (a *App) decryptUserPassword(encoded string) (string, error) {
	if encoded == "" {
		return "", errors.New("password is not available")
	}
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(a.passwordKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return "", errors.New("invalid password ciphertext")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
