package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize   = 16
	keySize    = 32
	iterations = 100000
)

func Encrypt(data []byte, password string) ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := pbkdf2.Key([]byte(password), salt, iterations, keySize, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Result: Salt + Nonce + Ciphertext
	result := append(salt, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

func Decrypt(data []byte, password string) ([]byte, error) {
	if len(data) < saltSize+12 { // Min size for salt + nonce (12 for GCM)
		return nil, fmt.Errorf("invalid encrypted data")
	}

	salt := data[:saltSize]
	nonceSize := 12 // Default GCM nonce size
	nonce := data[saltSize : saltSize+nonceSize]
	ciphertext := data[saltSize+nonceSize:]

	key := pbkdf2.Key([]byte(password), salt, iterations, keySize, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
