package main

import (
	"context"
	"strings"
	"sync"

	"s3browser/internal/desktop"
)

// Stored secrets carry this prefix followed by base64 ciphertext.
const secretPrefix = "enc:v1:"

// secretCipher protects credentials at rest with a key held outside the profile file.
type secretCipher interface {
	Available() bool
	Encrypt(plain string) (string, error)
	Decrypt(sealed string) (string, error)
}

func isSealed(value string) bool { return strings.HasPrefix(value, secretPrefix) }

// hostCipher delegates to the desktop host, which uses the operating
// system's credential store (Keychain, DPAPI, libsecret or KWallet).
type hostCipher struct {
	ctx       context.Context
	once      sync.Once
	available bool
	mu        sync.Mutex
	opened    map[string]string // decrypted values by ciphertext, to avoid repeated host calls
}

func (h *hostCipher) Available() bool {
	h.once.Do(func() { _ = desktop.Call(h.ctx, "secretAvailable", nil, &h.available) })
	return h.available
}

func (h *hostCipher) Encrypt(plain string) (string, error) {
	var sealed string
	if err := desktop.Call(h.ctx, "encryptSecret", plain, &sealed); err != nil {
		return "", err
	}
	return secretPrefix + sealed, nil
}

func (h *hostCipher) Decrypt(sealed string) (string, error) {
	h.mu.Lock()
	plain, ok := h.opened[sealed]
	h.mu.Unlock()
	if ok {
		return plain, nil
	}
	if err := desktop.Call(h.ctx, "decryptSecret", strings.TrimPrefix(sealed, secretPrefix), &plain); err != nil {
		return "", err
	}
	h.mu.Lock()
	if h.opened == nil {
		h.opened = make(map[string]string)
	}
	h.opened[sealed] = plain
	h.mu.Unlock()
	return plain, nil
}

func secretFields(p *Profile) []*string { return []*string{&p.SecretKey, &p.SessionToken} }

// seal encrypts the secrets of the profiles for writing. Without a usable
// credential store they are written unchanged.
func (s *profileStore) seal(profiles []Profile) ([]Profile, error) {
	if s.cipher == nil || !s.cipher.Available() {
		return profiles, nil
	}
	sealed := append(make([]Profile, 0, len(profiles)), profiles...)
	for i := range sealed {
		for _, field := range secretFields(&sealed[i]) {
			if *field == "" || isSealed(*field) {
				continue
			}
			value, err := s.cipher.Encrypt(*field)
			if err != nil {
				return nil, err
			}
			*field = value
		}
	}
	return sealed, nil
}

// unseal decrypts the secrets after reading and reports whether any were
// stored in plaintext. A value that cannot be decrypted stays sealed, so a
// later save writes it back unchanged instead of destroying it.
func (s *profileStore) unseal(profiles []Profile) (plaintext bool) {
	for i := range profiles {
		for _, field := range secretFields(&profiles[i]) {
			switch {
			case *field == "":
			case !isSealed(*field):
				plaintext = true
			case s.cipher != nil:
				if value, err := s.cipher.Decrypt(*field); err == nil {
					*field = value
				}
			}
		}
	}
	return plaintext
}
