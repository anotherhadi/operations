package operations

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"regexp"

	//lint:ignore SA1019 bcrypt is built on Blowfish
	"golang.org/x/crypto/blowfish"
)

func hashOps() []*Operation {
	const sha2, sha3URL = "https://wikipedia.org/wiki/SHA-2", "https://wikipedia.org/wiki/SHA-3"
	return []*Operation{
		hashOp("md5", "MD5", md5.New, "https://wikipedia.org/wiki/MD5", "5d41402abc4b2a76b9719d911017c592"),
		hashOp("sha1", "SHA-1", sha1.New, "https://wikipedia.org/wiki/SHA-1", "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"),
		hashOp("sha224", "SHA-224", sha256.New224, sha2, "ea09ae9cc6768c50fcee903ed054556e5bfc8347907f12598aa24193"),
		hashOp("sha256", "SHA-256", sha256.New, sha2, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"),
		hashOp("sha384", "SHA-384", sha512.New384, sha2, "59e1748777448c69de6b800d7a33bbfb9ff1b463e44354c3553bcdb9c666fa90125a3c79f90397bdf5f6a13de828684f"),
		hashOp("sha512", "SHA-512", sha512.New, sha2, "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"),
		hashOp("sha3-224", "SHA3-224", sha3hash(sha3.New224), sha3URL, "b87f88c72702fff1748e58b87e9141a42c0dbedc29a78cb0d4a5cd81"),
		hashOp("sha3-256", "SHA3-256", sha3hash(sha3.New256), sha3URL, "3338be694f50c5f338814986cdf0686453a888b84f424d792af4b9202398f392"),
		hashOp("sha3-384", "SHA3-384", sha3hash(sha3.New384), sha3URL, "720aea11019ef06440fbf05d87aa24680a2153df3907b23631e7177ce620fa1330ff07c0fddee54699a4c3ee0ee9d887"),
		hashOp("sha3-512", "SHA3-512", sha3hash(sha3.New512), sha3URL, "75d527c368f2efe848ecf6b073a36767800805e9eef2b1857d5f984f036eb6df891d75f72d9b154518c1cd58835286d1da9a38deba3de98b5a53e5ed78a84976"),
		bcryptOp,
	}
}

func sha3hash(f func() *sha3.SHA3) func() hash.Hash {
	return func() hash.Hash { return f() }
}

func hashOp(id, name string, h func() hash.Hash, infoURL, hello string) *Operation {
	return &Operation{
		ID:          id,
		Name:        name,
		Description: "Computes the " + name + " digest of the input, as lowercase hex.",
		Category:    Hashing,
		Aliases:     []string{id + "sum", name},
		InfoURL:     infoURL,
		Examples:    []Example{{Input: "hello", Output: hello}},
		Run: func(in []byte, _ Args) ([]byte, error) {
			d := h()
			d.Write(in)
			return hex.AppendEncode(nil, d.Sum(nil)), nil
		},
	}
}

const bcryptAlphabet = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var (
	bcryptEncoding = base64.NewEncoding(bcryptAlphabet).WithPadding(base64.NoPadding)
	bcryptSalt     = regexp.MustCompile(`^[./A-Za-z0-9]{22}$`)
)

var bcryptOp = &Operation{
	ID:          "bcrypt",
	Name:        "Bcrypt",
	Description: "Hashes the input (a password) with bcrypt, using a random salt unless one is given. Only the first 72 bytes are used.",
	Category:    Hashing,
	Aliases:     []string{"password hash", "blowfish"},
	InfoURL:     "https://wikipedia.org/wiki/Bcrypt",
	Args: []ArgDef{
		intArg("rounds", "Cost", 10).between(4, 31).
			describe("Log2 of the number of rounds: each step doubles the computing time."),
		stringArg("salt", "Salt", "").describe("22 characters from ./A-Za-z0-9. Leave empty for a random salt."),
	},
	Examples: []Example{
		{
			Input:  "hello",
			Args:   Args{"rounds": 4, "salt": "abcdefghijklmnopqrstuu"},
			Output: "$2b$04$abcdefghijklmnopqrstuuwHJMEGjfAzmL1lWmUmlphguIbWfjYey",
		},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		salt := a.String("salt")
		if salt == "" {
			raw := make([]byte, 16)
			rand.Read(raw)
			salt = bcryptEncoding.EncodeToString(raw)
		} else if !bcryptSalt.MatchString(salt) {
			return nil, errors.New("invalid salt: expected 22 characters from ./A-Za-z0-9")
		}
		return bcrypt(in, a.Int("rounds"), salt), nil
	},
}

// OpenBSD $2b$: the key includes its trailing NUL and is cut at 72 bytes.
func bcrypt(password []byte, cost int, salt string) []byte {
	rawSalt, _ := base64.NewEncoding(bcryptAlphabet).DecodeString(salt + "==")
	key := append(password[:len(password):len(password)], 0)
	if len(key) > 72 {
		key = key[:72]
	}
	c, _ := blowfish.NewSaltedCipher(key, rawSalt)
	for range 1 << cost {
		blowfish.ExpandKey(key, c)
		blowfish.ExpandKey(rawSalt, c)
	}
	data := []byte("OrpheanBeholderScryDoubt")
	for i := 0; i < 24; i += 8 {
		for range 64 {
			c.Encrypt(data[i:i+8], data[i:i+8])
		}
	}
	// Only 23 of the 24 bytes, like every other implementation. The last salt
	// character only carries 2 bits: it is written canonical, as OpenBSD does.
	return fmt.Appendf(nil, "$2b$%02d$%s%s", cost, bcryptEncoding.EncodeToString(rawSalt), bcryptEncoding.EncodeToString(data[:23]))
}
