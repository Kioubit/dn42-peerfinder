package kauth

import (
	"crypto/ecdsa"
	"crypto/sha512"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"peerfinder/config"
	"time"
)

//go:embed auth_public_key.pem
var pemPubKey []byte

type AuthenticationInfo struct {
	ASN          string `json:"asn"`
	Time         int64  `json:"time"`
	EffectiveMnt string `json:"effective_mnt"`
	Domain       string `json:"domain"`
}

func VerifyAuthToken(signature, params string, sessionTimeoutSec int) (userData AuthenticationInfo, err error) {
	// Read public key
	blockPub, _ := pem.Decode(pemPubKey)
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, err := x509.ParsePKIXPublicKey(x509EncodedPub)
	if err != nil {
		err = errors.New("internal server error")
		return
	}
	publicKey := genericPublicKey.(*ecdsa.PublicKey)

	// Hash parameters
	hash := sha512.Sum512([]byte(params))

	// Decode base64 signature
	signatureBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		err = errors.New("failed to decode signature")
		return
	}

	// Verify signature
	if !ecdsa.VerifyASN1(publicKey, hash[:], signatureBytes) {
		err = errors.New("invalid signature")
		return
	}

	// Decode parameters
	parameterBytes, err := base64.StdEncoding.DecodeString(params)
	if err != nil {
		err = fmt.Errorf("failed decoding verified parameters: %w", err)
		return
	}

	err = json.Unmarshal(parameterBytes, &userData)
	if err != nil {
		log.Println("Unmarshal verified parameters", err)
		err = fmt.Errorf("failed unmarshaling verified parameters: %w", err)
		return
	}

	abs := userData.Time - time.Now().Unix()
	if abs < 0 {
		abs = -abs
	}

	if abs > int64(sessionTimeoutSec) {
		err = errors.New("the request has expired")
		return
	}

	if userData.Domain != config.Global.MyDomain {
		err = errors.New("this request is for a different domain")
		return
	}

	return
}
