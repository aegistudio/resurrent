package resurrent

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/url"

	"github.com/pkg/errors"
	"github.com/secure-io/sio-go"
)

const (
	filterTypeSioAES256GCM = "sio-aes256-gcm"
)

var (
	sioAES256GCMNonceSize int
)

func init() {
	key := make([]byte, 32)
	s, err := sio.AES_256_GCM.Stream(key)
	if err != nil {
		panic(err)
	}
	sioAES256GCMNonceSize = s.NonceSize()
}

type filterSioAES256GCM struct {
	key   []byte
	nonce []byte
}

func (f *filterSioAES256GCM) encode(w io.Writer, r io.Reader) error {
	stream, err := sio.AES_256_GCM.Stream(f.key)
	if err != nil {
		return errors.Wrap(err, "create key stream")
	}
	writer := stream.EncryptWriter(w, f.nonce, nil)
	if _, err := io.Copy(writer, r); err != nil {
		return errors.Wrap(err, "encrypt data")
	}
	if err := writer.Close(); err != nil {
		return errors.Wrap(err, "close encrypter")
	}
	return nil
}

func (f *filterSioAES256GCM) decode(w io.Writer, r io.Reader) error {
	stream, err := sio.AES_256_GCM.Stream(f.key)
	if err != nil {
		return errors.Wrap(err, "create key stream")
	}
	reader := stream.DecryptReader(r, f.nonce, nil)
	if _, err := io.Copy(w, reader); err != nil {
		return errors.Wrap(err, "decrypt data")
	}
	return nil
}

func (f *filterSioAES256GCM) serialize() (string, error) {
	var result url.URL
	result.Scheme = filterTypeSioAES256GCM
	queries := make(url.Values)
	queries.Set("key", base64.URLEncoding.EncodeToString(f.key))
	queries.Set("nonce", base64.URLEncoding.EncodeToString(f.nonce))
	result.RawQuery = queries.Encode()
	return result.String(), nil
}

var _ filter = (*filterSioAES256GCM)(nil)

func randomFilterSioAES256GCM() (*filterSioAES256GCM, error) {
	var key [32]byte
	if _, err := rand.Reader.Read(key[:]); err != nil {
		return nil, errors.Wrap(err, "allocate key")
	}
	nonce := make([]byte, sioAES256GCMNonceSize)
	if _, err := rand.Reader.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "alloc nonce")
	}
	return &filterSioAES256GCM{
		key:   key[:],
		nonce: nonce,
	}, nil
}

func init() {
	registerFilter(filterTypeSioAES256GCM, func(
		spec *url.URL,
	) (filter, error) {
		query := spec.Query()
		key, err := base64.URLEncoding.DecodeString(
			query.Get("key"),
		)
		if err == nil && len(key) != 32 {
			err = errors.New("invalid key size")
		}
		if err != nil {
			return nil, errors.Wrap(err, "decode key")
		}
		nonce, err := base64.URLEncoding.DecodeString(
			query.Get("nonce"),
		)
		if err == nil && len(nonce) != sioAES256GCMNonceSize {
			err = errors.New("invalid nonce size")
		}
		if err != nil {
			return nil, errors.Wrap(err, "decode nonce")
		}
		return &filterSioAES256GCM{
			key:   key,
			nonce: nonce,
		}, nil
	})
}
