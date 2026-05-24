package clients

import (
	"encoding/json"
	"net/http"

	"github.com/mongodb-forks/digest"
	"go.mongodb.org/ops-manager/opsmngr"
)

// Credentials holds the public/private API key pair read from the K8s secret.
type Credentials struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

// ParseCredentials unmarshals raw secret data into a Credentials struct.
func ParseCredentials(data []byte) (*Credentials, error) {
	creds := &Credentials{}
	return creds, json.Unmarshal(data, creds)
}

// NewClient creates an authenticated Ops Manager client using HTTP Digest Auth.
// The Ops Manager API authenticates via digest auth with publicKey as the
// username and privateKey as the password.
func NewClient(baseURL string, creds *Credentials) (*opsmngr.Client, error) {
	transport := digest.NewTransport(creds.PublicKey, creds.PrivateKey)
	httpClient := &http.Client{Transport: transport}

	return opsmngr.New(httpClient, opsmngr.SetBaseURL(baseURL))
}
