package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"golang.org/x/crypto/pbkdf2"
)

type Encryptor struct {
	key []byte
	salt []byte
}

func NewEncryptor(password string) (*Encryptor, error) {
	return NewEncryptorWithSalt(password, nil)
}

func NewEncryptorWithSalt(password string, salt []byte) (*Encryptor, error) {
	if len(salt) == 0 {
		// 使用密码哈希作为确定性salt，确保不同密码有不同salt
		// 同时保证同一密码总是产生相同salt
		h := sha256.Sum256([]byte("contextkeeper-salt-" + password))
		salt = h[:16]
	}
	key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
	return &Encryptor{key: key, salt: salt}, nil
}

func (e *Encryptor) GetSalt() []byte {
	return e.salt
}

func (e *Encryptor) Encrypt(plainText string) (string, error) {
	block, err := aes.NewCipher(e.key)
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

	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func (e *Encryptor) Decrypt(cipherText string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("cipher text too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

type SecureStorage interface {
	Store(key string, data interface{}) error
	Retrieve(key string, dest interface{}) error
	Delete(key string) error
	Exists(key string) bool
}

type AESStorage struct {
	encryptor *Encryptor
	data     map[string][]byte
}

func NewAESStorage(password string) (*AESStorage, error) {
	enc, err := NewEncryptor(password)
	if err != nil {
		return nil, err
	}
	return &AESStorage{
		encryptor: enc,
		data:     make(map[string][]byte),
	}, nil
}

func (s *AESStorage) Store(key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	cipherText, err := s.encryptor.Encrypt(string(jsonData))
	if err != nil {
		return err
	}

	s.data[key] = []byte(cipherText)
	return nil
}

func (s *AESStorage) Retrieve(key string, dest interface{}) error {
	cipherText, ok := s.data[key]
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}

	plainText, err := s.encryptor.Decrypt(string(cipherText))
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(plainText), dest)
}

func (s *AESStorage) Delete(key string) error {
	delete(s.data, key)
	return nil
}

func (s *AESStorage) Exists(key string) bool {
	_, ok := s.data[key]
	return ok
}

func EncryptJSON(data interface{}, password string) (string, error) {
	enc, err := NewEncryptor(password)
	if err != nil {
		return "", err
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return enc.Encrypt(string(jsonData))
}

func DecryptJSON(cipherText string, password string, dest interface{}) error {
	enc, err := NewEncryptor(password)
	if err != nil {
		return err
	}

	plainText, err := enc.Decrypt(cipherText)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(plainText), dest)
}