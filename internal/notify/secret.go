// Package notify — уведомления «Контроля спринта руководителей»: события после пересчёта, колокольчик в сервисе
// и личный Telegram-бот руководителя (токен из @BotFather, хранится зашифрованным).
package notify

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// ErrNoKey — на сервере не задан SECRETS_KEY: токены ботов хранить нельзя.
var ErrNoKey = errors.New("на сервере не задан SECRETS_KEY — подключить Telegram-бота нельзя")

// Box шифрует секреты пользователей (токены ботов) ключом из SECRETS_KEY: AES-256-GCM, ключ — SHA-256 от строки.
type Box struct{ aead cipher.AEAD }

func NewBox(secret string) (*Box, error) {
	if len(secret) < 16 {
		return nil, ErrNoKey
	}
	k := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead}, nil
}

func (b *Box) Seal(plain string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (b *Box) Open(sealed []byte) (string, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("повреждённый секрет")
	}
	p, err := b.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return "", errors.New("секрет не расшифровывается — сменился SECRETS_KEY? подключите бота заново")
	}
	return string(p), nil
}
