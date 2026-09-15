package registry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const AccountingDelegationHeader = "X-Threadify-Accounting-Delegation"

var ErrDelegation = errors.New("invalid or replayed accounting delegation")

type accountingProof struct {
	Method            string `json:"method"`
	Target            string `json:"target"`
	AuthorizationHash string `json:"authorization_hash"`
	APIKeyHash        string `json:"api_key_hash"`
	BodyHash          string `json:"body_hash"`
	BodyBytes         int64  `json:"body_bytes"`
	ContentType       string `json:"content_type"`
	ContentEncoding   string `json:"content_encoding"`
	Timestamp         int64  `json:"timestamp"`
	Nonce             string `json:"nonce"`
	InstallationID    string `json:"installation_id"`
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func requestTarget(req *http.Request) string {
	target := req.URL.EscapedPath()
	if target == "" {
		target = "/"
	}
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	return target
}

// SignRequest delegates accounting for one API-to-Engine HTTP hop. The API
// must meter the external request/response itself with Wrap. Only WrapEngine
// accepts these proofs; external callers cannot select a metering exemption.
// Bodies must be replayable (http.NewRequest supplies GetBody for buffers).
func (r *Runtime) SignRequest(req *http.Request) error {
	if r == nil {
		return ErrUnverified
	}
	if _, err := r.Snapshot(); err != nil {
		return err
	}
	if req.Header.Get("Upgrade") != "" {
		return errors.New("accounting delegation does not support upgraded connections")
	}
	bodyHash := hashString("")
	var bodyBytes int64
	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return errors.New("accounting delegation requires a replayable body")
		}
		reader, err := req.GetBody()
		if err != nil {
			return err
		}
		hash := sha256.New()
		bodyBytes, err = io.Copy(hash, reader)
		reader.Close()
		if err != nil {
			return err
		}
		bodyHash = hex.EncodeToString(hash.Sum(nil))
	}
	proof := accountingProof{Method: req.Method, Target: requestTarget(req), AuthorizationHash: hashString(req.Header.Get("Authorization")), APIKeyHash: hashString(req.Header.Get("X-API-Key")), BodyHash: bodyHash, BodyBytes: bodyBytes, ContentType: req.Header.Get("Content-Type"), ContentEncoding: req.Header.Get("Content-Encoding"), Timestamp: time.Now().Unix(), Nonce: newID(), InstallationID: r.cfg.InstallationID}
	data, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(r.cfg.LicenseKey))
	mac.Write([]byte("threadify-accounting-v1\n"))
	mac.Write(data)
	req.Header.Set(AccountingDelegationHeader, base64.RawURLEncoding.EncodeToString(data)+"."+hex.EncodeToString(mac.Sum(nil)))
	return nil
}

// acceptDelegation verifies and consumes a one-use proof before any handler
// runs. Signed bodies are spooled to disk and hashed before exposing them so a
// mismatched/truncated body can never cause an unmetered mutation.
func (r *Runtime) acceptDelegation(req *http.Request) (func(), error) {
	if _, err := r.Snapshot(); err != nil {
		return nil, err
	}
	header := req.Header.Get(AccountingDelegationHeader)
	if len(header) > 4096 {
		return nil, ErrDelegation
	}
	parts := strings.Split(header, ".")
	if len(parts) != 2 {
		return nil, ErrDelegation
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrDelegation
	}
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, ErrDelegation
	}
	mac := hmac.New(sha256.New, []byte(r.cfg.LicenseKey))
	mac.Write([]byte("threadify-accounting-v1\n"))
	mac.Write(data)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, ErrDelegation
	}
	var proof accountingProof
	if err = json.Unmarshal(data, &proof); err != nil {
		return nil, ErrDelegation
	}
	now := time.Now().Unix()
	if proof.Timestamp < now-30 || proof.Timestamp > now+5 || proof.Nonce == "" || proof.BodyBytes < 0 || proof.BodyBytes == int64(^uint64(0)>>1) || proof.ContentType != req.Header.Get("Content-Type") || proof.ContentEncoding != req.Header.Get("Content-Encoding") || proof.InstallationID != r.cfg.InstallationID || proof.Method != req.Method || proof.Target != requestTarget(req) || proof.AuthorizationHash != hashString(req.Header.Get("Authorization")) || proof.APIKeyHash != hashString(req.Header.Get("X-API-Key")) || req.Header.Get("Upgrade") != "" {
		return nil, ErrDelegation
	}
	tag, err := r.pool.Exec(req.Context(), `INSERT INTO threadify_registry_delegations(nonce,expires_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, proof.Nonce, time.Unix(proof.Timestamp+60, 0))
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrDelegation
	}
	if req.Body == nil || req.Body == http.NoBody {
		if proof.BodyBytes != 0 || proof.BodyHash != hashString("") {
			return nil, ErrDelegation
		}
		return func() {}, nil
	}
	file, err := os.CreateTemp("", "threadify-accounting-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() { file.Close(); os.Remove(file.Name()) }
	hash := sha256.New()
	copied, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(req.Body, proof.BodyBytes+1))
	req.Body.Close()
	if copyErr != nil {
		cleanup()
		return nil, copyErr
	}
	if copied != proof.BodyBytes || hex.EncodeToString(hash.Sum(nil)) != proof.BodyHash {
		cleanup()
		return nil, ErrDelegation
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	req.Body = file
	return cleanup, nil
}
