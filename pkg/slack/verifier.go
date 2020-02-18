package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"
)

type Verifier struct {
	Secret  string
	Version string
	Logger  *zap.Logger
}

func (sv Verifier) generateSignature(body string, timestamp string) (string, error) {
	sig_basestring := fmt.Sprintf("%s:%s:%s", sv.Version, timestamp, body)
	sv.Logger.Debug(sig_basestring)
	h := hmac.New(sha256.New, []byte(sv.Secret))
	_, err := h.Write([]byte(sig_basestring))
	if err != nil {
		return "", err
	}
	sha := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s=%s", sv.Version, sha), nil
}

func (sv Verifier) IsValid(body, timestamp, signature string) bool {
	sig, err := sv.generateSignature(body, timestamp)
	if err != nil {
		sv.Logger.Error(err.Error())
		return false
	}
	sv.Logger.Debug(fmt.Sprintf("slack signature %s", signature))
	sv.Logger.Debug(fmt.Sprintf("generated signature %s", sig))
	return sig == signature
}
