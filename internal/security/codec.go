// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20"

	baseCodec "masterdnsvpn-go/internal/basecodec"
	"masterdnsvpn-go/internal/config"
)

var (
	ErrInvalidCodecMethod = errors.New("invalid encryption method")
	ErrInvalidCiphertext  = errors.New("invalid ciphertext")
)

const (
	chachaNonceSize = 16
	aesNonceSize    = 12
)

type Codec struct {
	method  int
	key     []byte
	encrypt func([]byte) ([]byte, error)
	decrypt func([]byte) ([]byte, error)
}

func NewCodecFromConfig(cfg config.ServerConfig, rawKey string) (*Codec, error) {
	return NewCodec(cfg.DataEncryptionMethod, rawKey)
}

func NewCodec(method int, rawKey string) (*Codec, error) {
	if method < 0 || method > 5 {
		return nil, ErrInvalidCodecMethod
	}

	derivedKey := deriveKey(method, rawKey)
	codec := &Codec{
		method: method,
		key:    derivedKey,
	}

	switch method {
	case 0:
		codec.encrypt = codec.noCrypto
		codec.decrypt = codec.noCrypto
	case 1:
		codec.encrypt = codec.xorCrypto
		codec.decrypt = codec.xorCrypto
	case 2:
		codec.encrypt = codec.chachaEncrypt
		codec.decrypt = codec.chachaDecrypt
	case 3, 4, 5:
		aead, err := newAESGCM(derivedKey)
		if err != nil {
			return nil, err
		}
		codec.encrypt = codec.makeAESEncryptor(aead)
		codec.decrypt = codec.makeAESDecryptor(aead)
	default:
		return nil, ErrInvalidCodecMethod
	}

	return codec, nil
}

func (c *Codec) Encrypt(data []byte) ([]byte, error) {
	if c == nil {
		return nil, ErrInvalidCodecMethod
	}
	return c.encrypt(data)
}

func (c *Codec) Decrypt(data []byte) ([]byte, error) {
	if c == nil {
		return nil, ErrInvalidCodecMethod
	}
	return c.decrypt(data)
}

func (c *Codec) Method() int {
	if c == nil {
		return 0
	}
	return c.method
}

func (c *Codec) EncryptAndEncodeLowerBase36(data []byte) (string, error) {
	if c == nil {
		return "", ErrInvalidCodecMethod
	}
	if c.method == 0 {
		return baseCodec.EncodeLowerBase36(data), nil
	}

	encrypted, err := c.encrypt(data)
	if err != nil {
		return "", err
	}
	return baseCodec.EncodeLowerBase36(encrypted), nil
}

func (c *Codec) DecodeLowerBase36AndDecrypt(data []byte) ([]byte, error) {
	if c == nil {
		return nil, ErrInvalidCodecMethod
	}

	decoded, err := baseCodec.DecodeLowerBase36(data)
	if err != nil {
		return nil, err
	}
	if c.method == 0 {
		return decoded, nil
	}
	return c.decrypt(decoded)
}

func (c *Codec) DecodeLowerBase36StringAndDecrypt(data string) ([]byte, error) {
	if c == nil {
		return nil, ErrInvalidCodecMethod
	}

	decoded, err := baseCodec.DecodeLowerBase36String(data)
	if err != nil {
		return nil, err
	}
	if c.method == 0 {
		return decoded, nil
	}
	return c.decrypt(decoded)
}

func (c *Codec) noCrypto(data []byte) ([]byte, error) {
	return data, nil
}

func (c *Codec) xorCrypto(data []byte) ([]byte, error) {
	key := c.key
	keyLen := len(key)
	if len(data) == 0 || keyLen == 0 {
		return data, nil
	}

	out := make([]byte, len(data))
	if keyLen == 1 {
		mask := key[0]
		for i := 0; i < len(data); i++ {
			out[i] = data[i] ^ mask
		}
		return out, nil
	}

	fullBlocks := len(data) / keyLen
	offset := 0
	for block := 0; block < fullBlocks; block++ {
		for i := 0; i < keyLen; i++ {
			out[offset+i] = data[offset+i] ^ key[i]
		}
		offset += keyLen
	}
	for i := offset; i < len(data); i++ {
		out[i] = data[i] ^ key[i-offset]
	}
	return out, nil
}

func (c *Codec) chachaEncrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	out := make([]byte, chachaNonceSize+len(data))
	nonce := out[:chachaNonceSize]
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate chacha20 nonce: %w", err)
	}

	stream, err := chacha20.NewUnauthenticatedCipher(c.key, nonce[4:])
	if err != nil {
		return nil, err
	}
	stream.SetCounter(binary.LittleEndian.Uint32(nonce[:4]))
	stream.XORKeyStream(out[chachaNonceSize:], data)
	return out, nil
}

func (c *Codec) chachaDecrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	if len(data) <= chachaNonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce := data[:chachaNonceSize]
	ciphertext := data[chachaNonceSize:]
	stream, err := chacha20.NewUnauthenticatedCipher(c.key, nonce[4:])
	if err != nil {
		return nil, err
	}
	stream.SetCounter(binary.LittleEndian.Uint32(nonce[:4]))

	out := make([]byte, len(ciphertext))
	stream.XORKeyStream(out, ciphertext)
	return out, nil
}

func (c *Codec) makeAESEncryptor(aead cipher.AEAD) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			return data, nil
		}

		out := make([]byte, aesNonceSize, aesNonceSize+len(data)+aead.Overhead())
		nonce := out[:aesNonceSize]
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("generate aes-gcm nonce: %w", err)
		}

		out = aead.Seal(out, nonce, data, nil)
		return out, nil
	}
}

func (c *Codec) makeAESDecryptor(aead cipher.AEAD) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			return data, nil
		}
		if len(data) <= aesNonceSize {
			return nil, ErrInvalidCiphertext
		}

		nonce := data[:aesNonceSize]
		ciphertext := data[aesNonceSize:]
		plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return nil, ErrInvalidCiphertext
		}
		return plaintext, nil
	}
}

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create aes-gcm: %w", err)
	}
	return aead, nil
}

func deriveKey(method int, rawKey string) []byte {
	bKey := []byte(rawKey)
	targetLen := requiredDerivedKeyLength(method)

	switch method {
	case 2, 5:
		sum := sha256.Sum256(bKey)
		return sum[:]
	case 3:
		sum := md5.Sum(bKey)
		return sum[:]
	default:
		key := make([]byte, targetLen)
		copy(key, bKey)
		return key
	}
}

func requiredDerivedKeyLength(method int) int {
	switch method {
	case 2, 5:
		return 32
	case 3:
		return 16
	case 4:
		return 24
	default:
		return 32
	}
}
