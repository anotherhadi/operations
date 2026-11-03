package operations

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"strings"
)

func cryptoOps() []*Operation {
	return []*Operation{xor, aesEncrypt, aesDecrypt, jwtDecode, jwtSign}
}

func formatArg(id, name, def string) ArgDef {
	return selectArg(id, name, def,
		Option{"hex", "Hex"},
		Option{"utf8", "UTF-8 text"},
		Option{"base64", "Base64 (standard or URL-safe)"},
	)
}

func decodeValue(what, value, format string) ([]byte, error) {
	var b []byte
	var err error
	switch format {
	case "hex":
		b, err = fromHex.Run([]byte(value), nil)
	case "base64":
		b, err = decodeBase64([]byte(strings.NewReplacer("-", "+", "_", "/").Replace(value)), base64.StdEncoding, "+/", false)
	default:
		b = []byte(value)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", what, err)
	}
	return b, nil
}

var xor = &Operation{
	ID:          "xor",
	Name:        "XOR",
	Description: "XORs the input with a key, repeated over the whole input. It is its own inverse.",
	Category:    Crypto,
	Output:      BinaryOutput,
	Aliases:     []string{"exclusive or", "xor cipher"},
	Inverse:     "xor",
	InfoURL:     "https://wikipedia.org/wiki/XOR_cipher",
	Args: []ArgDef{
		stringArg("key", "Key", ""),
		formatArg("key-format", "Key format", "hex"),
	},
	Examples: []Example{
		{Input: "hello", Args: Args{"key": "20"}, Output: "HELLO"},
		{Input: "Hello", Args: Args{"key": " ", "key-format": "utf8"}, Output: "hELLO"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		key, err := decodeValue("key", a.String("key"), a.String("key-format"))
		if err != nil {
			return nil, err
		}
		if len(key) == 0 {
			return nil, errors.New("a key is required")
		}
		out := make([]byte, len(in))
		for i, b := range in {
			out[i] = b ^ key[i%len(key)]
		}
		return out, nil
	},
}

func aesArgs(ivDescription string) []ArgDef {
	return []ArgDef{
		stringArg("key", "Key", "").describe("16, 24 or 32 bytes, for AES-128, AES-192 or AES-256."),
		formatArg("key-format", "Key format", "hex"),
		selectArg("mode", "Mode", "cbc",
			Option{"cbc", "CBC, with PKCS#7 padding"},
			Option{"ecb", "ECB, with PKCS#7 padding and no IV (insecure)"},
			Option{"ctr", "CTR"},
			Option{"gcm", "GCM, authenticated: the 16-byte tag follows the ciphertext"},
		),
		stringArg("iv", "IV", "").describe(ivDescription),
		formatArg("iv-format", "IV format", "hex"),
	}
}

// "hello" with aesExample, checked with openssl enc.
const aesCiphertext = "\x18\x15\x47\xce\x6a\x54\x38\xcb\x22\x24\x18\x56\xb3\x5c\xc4\x89"

var aesExample = Args{"key": "000102030405060708090a0b0c0d0e0f", "iv": "0f0e0d0c0b0a09080706050403020100"}

var aesEncrypt = &Operation{
	ID:          "aes-encrypt",
	Name:        "AES Encrypt",
	Description: "Encrypts the input with AES. Without an IV, a random one is generated and written before the ciphertext.",
	Category:    Crypto,
	Output:      BinaryOutput,
	Aliases:     []string{"aes", "rijndael", "encrypt"},
	Inverse:     "aes-decrypt",
	InfoURL:     "https://wikipedia.org/wiki/Advanced_Encryption_Standard",
	Args:        aesArgs("16 bytes (12 for GCM). Leave empty for a random IV, written before the ciphertext."),
	Examples: []Example{
		{Input: "hello", Args: aesExample, Output: aesCiphertext},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		block, mode, iv, err := aesSetup(a)
		if err != nil {
			return nil, err
		}
		var prefix []byte
		if iv == nil && mode != "ecb" {
			iv = make([]byte, ivSize(mode))
			rand.Read(iv)
			prefix = iv
		}
		var out []byte
		switch mode {
		case "cbc":
			out = pkcs7Pad(in)
			cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, out)
		case "ecb":
			out = pkcs7Pad(in)
			for i := 0; i < len(out); i += aes.BlockSize {
				block.Encrypt(out[i:], out[i:])
			}
		case "ctr":
			out = make([]byte, len(in))
			cipher.NewCTR(block, iv).XORKeyStream(out, in)
		case "gcm":
			gcm, _ := cipher.NewGCM(block)
			out = gcm.Seal(nil, iv, in, nil)
		}
		return append(prefix, out...), nil
	},
}

var aesDecrypt = &Operation{
	ID:          "aes-decrypt",
	Name:        "AES Decrypt",
	Description: "Decrypts AES ciphertext. Without an IV, it is read from the start of the input, as aes-encrypt writes it.",
	Category:    Crypto,
	Output:      BinaryOutput,
	Aliases:     []string{"aes", "rijndael", "decrypt"},
	Inverse:     "aes-encrypt",
	InfoURL:     "https://wikipedia.org/wiki/Advanced_Encryption_Standard",
	Args:        aesArgs("16 bytes (12 for GCM). Leave empty to read it from the start of the input."),
	Examples: []Example{
		{Input: aesCiphertext, Args: aesExample, Output: "hello"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		block, mode, iv, err := aesSetup(a)
		if err != nil {
			return nil, err
		}
		if iv == nil && mode != "ecb" {
			n := ivSize(mode)
			if len(in) < n {
				return nil, fmt.Errorf("input too short to start with a %d-byte IV", n)
			}
			iv, in = in[:n], in[n:]
		}
		out := make([]byte, len(in))
		switch mode {
		case "cbc", "ecb":
			if len(in)%aes.BlockSize != 0 {
				return nil, fmt.Errorf("ciphertext length (%d bytes) is not a multiple of 16", len(in))
			}
			if mode == "cbc" {
				cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, in)
			} else {
				for i := 0; i < len(in); i += aes.BlockSize {
					block.Decrypt(out[i:], in[i:])
				}
			}
			return pkcs7Unpad(out)
		case "ctr":
			cipher.NewCTR(block, iv).XORKeyStream(out, in)
			return out, nil
		default:
			gcm, _ := cipher.NewGCM(block)
			out, err := gcm.Open(nil, iv, in, nil)
			if err != nil {
				return nil, errors.New("authentication failed: wrong key, IV or data")
			}
			return out, nil
		}
	},
}

func ivSize(mode string) int {
	if mode == "gcm" {
		return 12
	}
	return aes.BlockSize
}

// The IV is nil when none is given.
func aesSetup(a Args) (cipher.Block, string, []byte, error) {
	key, err := decodeValue("key", a.String("key"), a.String("key-format"))
	if err != nil {
		return nil, "", nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", nil, fmt.Errorf("invalid key: AES needs 16, 24 or 32 bytes, got %d", len(key))
	}
	mode := a.String("mode")
	if a.String("iv") == "" {
		return block, mode, nil, nil
	}
	if mode == "ecb" {
		return nil, "", nil, errors.New("ECB does not use an IV")
	}
	iv, err := decodeValue("IV", a.String("iv"), a.String("iv-format"))
	if err != nil {
		return nil, "", nil, err
	}
	if len(iv) != ivSize(mode) {
		return nil, "", nil, fmt.Errorf("invalid IV: %s needs %d bytes, got %d", strings.ToUpper(mode), ivSize(mode), len(iv))
	}
	return block, mode, iv, nil
}

func pkcs7Pad(b []byte) []byte {
	n := aes.BlockSize - len(b)%aes.BlockSize
	return append(bytes.Clone(b), bytes.Repeat([]byte{byte(n)}, n)...)
}

func pkcs7Unpad(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return b, nil
	}
	n := int(b[len(b)-1])
	if n == 0 || n > aes.BlockSize || n > len(b) || !bytes.Equal(b[len(b)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return nil, errors.New("invalid padding: wrong key or IV, or not PKCS#7")
	}
	return b[:len(b)-n], nil
}

const jwtExample = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

var jwtDecode = &Operation{
	ID:          "jwt-decode",
	Name:        "JWT Decode",
	Description: `Decodes a JSON Web Token into {"header", "payload", "signature"}, without verifying the signature.`,
	Category:    Crypto,
	Output:      BinaryOutput,
	Aliases:     []string{"jwt", "json web token", "jws"},
	InfoURL:     "https://datatracker.ietf.org/doc/html/rfc7519",
	Args:        []ArgDef{jsonIndentArg},
	Examples: []Example{
		{
			Input:  jwtExample,
			Args:   Args{"indent": "minify"},
			Output: `{"header":{"alg":"HS256","typ":"JWT"},"payload":{"sub":"1234567890","name":"John Doe","iat":1516239022},"signature":"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"}`,
		},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		token := strings.TrimSpace(string(in))
		if len(token) > 7 && strings.EqualFold(token[:7], "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		parts := strings.Split(token, ".")
		if len(parts) == 5 {
			return nil, errors.New("encrypted tokens (JWE) are not supported")
		}
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid JWT: expected 3 parts separated by dots, got %d", len(parts))
		}
		header, err := jwtPart("header", parts[0])
		if err != nil {
			return nil, err
		}
		if !json.Valid(header) {
			return nil, errors.New("invalid JWT: the header is not JSON")
		}
		payload, err := jwtPart("payload", parts[1])
		if err != nil {
			return nil, err
		}
		// a JWS payload need not be JSON
		if !json.Valid(payload) {
			payload = jsonString(string(payload))
		}
		signature := jsonString(parts[2])
		out := fmt.Sprintf(`{"header":%s,"payload":%s,"signature":%s}`, header, payload, signature)
		return formatJSON([]byte(out), jsonIndents[a.String("indent")])
	},
}

func jwtPart(name, part string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(part, "="))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT: the %s is not Base64url", name)
	}
	return b, nil
}

var jwtAlgorithms = map[string]func() hash.Hash{"HS256": sha256.New, "HS384": sha512.New384, "HS512": sha512.New}

var jwtSign = &Operation{
	ID:          "jwt-sign",
	Name:        "JWT Sign",
	Description: `Makes a JSON Web Token from a JSON payload, signed with HMAC or unsigned ("none").`,
	Category:    Crypto,
	Aliases:     []string{"jwt", "json web token", "jws", "jwt encode"},
	InfoURL:     "https://datatracker.ietf.org/doc/html/rfc7519",
	Args: []ArgDef{
		selectArg("algorithm", "Algorithm", "HS256",
			Option{"HS256", "HMAC with SHA-256"},
			Option{"HS384", "HMAC with SHA-384"},
			Option{"HS512", "HMAC with SHA-512"},
			Option{"none", "None: unsigned token"},
		),
		stringArg("key", "Secret", ""),
		formatArg("key-format", "Secret format", "utf8"),
	},
	Examples: []Example{
		{
			Input:  `{"sub":"1234567890","name":"John Doe","iat":1516239022}`,
			Args:   Args{"key": "your-256-bit-secret"},
			Output: jwtExample,
		},
		{Input: `{"admin":true}`, Args: Args{"algorithm": "none"}, Output: "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJhZG1pbiI6dHJ1ZX0."},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		payload, err := formatJSON(in, "")
		if err != nil {
			return nil, err
		}
		alg := a.String("algorithm")
		enc := base64.RawURLEncoding
		signed := enc.EncodeToString(fmt.Appendf(nil, `{"alg":%q,"typ":"JWT"}`, alg)) + "." + enc.EncodeToString(payload)
		if alg == "none" {
			return []byte(signed + "."), nil
		}
		key, err := decodeValue("secret", a.String("key"), a.String("key-format"))
		if err != nil {
			return nil, err
		}
		mac := hmac.New(jwtAlgorithms[alg], key)
		mac.Write([]byte(signed))
		return []byte(signed + "." + enc.EncodeToString(mac.Sum(nil))), nil
	},
}
