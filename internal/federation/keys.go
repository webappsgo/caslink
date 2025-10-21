package federation

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// KeyManager manages RSA keys for federation
type KeyManager struct {
	config     *config.FederationConfig
	logger     *logrus.Logger
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyMutex   sync.RWMutex
}

// NewKeyManager creates a new key manager
func NewKeyManager(cfg *config.FederationConfig, logger *logrus.Logger) (*KeyManager, error) {
	km := &KeyManager{
		config: cfg,
		logger: logger,
	}

	// Initialize keys
	if err := km.initializeKeys(); err != nil {
		return nil, fmt.Errorf("failed to initialize keys: %w", err)
	}

	return km, nil
}

// initializeKeys loads or generates RSA key pair
func (km *KeyManager) initializeKeys() error {
	privateKeyPath := km.config.PrivateKeyPath
	publicKeyPath := km.config.PublicKeyPath

	// Create key directory if it doesn't exist
	keyDir := filepath.Dir(privateKeyPath)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Check if keys exist
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		km.logger.Info("Federation keys not found, generating new key pair")
		if err := km.generateKeyPair(privateKeyPath, publicKeyPath); err != nil {
			return fmt.Errorf("failed to generate key pair: %w", err)
		}
	}

	// Load keys
	if err := km.loadKeys(privateKeyPath, publicKeyPath); err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	km.logger.Info("Federation keys loaded successfully")
	return nil
}

// generateKeyPair generates a new RSA key pair
func (km *KeyManager) generateKeyPair(privateKeyPath, publicKeyPath string) error {
	// Generate 2048-bit RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Save private key
	if err := km.savePrivateKey(privateKey, privateKeyPath); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	// Save public key
	if err := km.savePublicKey(&privateKey.PublicKey, publicKeyPath); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	km.logger.Info("Generated new federation key pair")
	return nil
}

// savePrivateKey saves the private key to disk
func (km *KeyManager) savePrivateKey(key *rsa.PrivateKey, path string) error {
	// Create private key file with restrictive permissions
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer file.Close()

	// Encode private key as PEM
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(key)
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	if err := pem.Encode(file, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	return nil
}

// savePublicKey saves the public key to disk
func (km *KeyManager) savePublicKey(key *rsa.PublicKey, path string) error {
	// Create public key file
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %w", err)
	}
	defer file.Close()

	// Encode public key as PEM
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	if err := pem.Encode(file, publicKeyPEM); err != nil {
		return fmt.Errorf("failed to encode public key: %w", err)
	}

	return nil
}

// loadKeys loads the RSA key pair from disk
func (km *KeyManager) loadKeys(privateKeyPath, publicKeyPath string) error {
	// Load private key
	privateKey, err := km.loadPrivateKey(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	// Load public key
	publicKey, err := km.loadPublicKey(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load public key: %w", err)
	}

	km.keyMutex.Lock()
	km.privateKey = privateKey
	km.publicKey = publicKey
	km.keyMutex.Unlock()

	return nil
}

// loadPrivateKey loads a private key from disk
func (km *KeyManager) loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	if block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("invalid private key type: %s", block.Type)
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

// loadPublicKey loads a public key from disk
func (km *KeyManager) loadPublicKey(path string) (*rsa.PublicKey, error) {
	keyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid public key type: %s", block.Type)
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return publicKey, nil
}

// GetPrivateKey returns the private key
func (km *KeyManager) GetPrivateKey() *rsa.PrivateKey {
	km.keyMutex.RLock()
	defer km.keyMutex.RUnlock()
	return km.privateKey
}

// GetPublicKey returns the public key
func (km *KeyManager) GetPublicKey() (*rsa.PublicKey, error) {
	km.keyMutex.RLock()
	defer km.keyMutex.RUnlock()

	if km.publicKey == nil {
		return nil, fmt.Errorf("public key not initialized")
	}

	return km.publicKey, nil
}

// GetPublicKeyPEM returns the public key in PEM format
func (km *KeyManager) GetPublicKeyPEM() (string, error) {
	km.keyMutex.RLock()
	defer km.keyMutex.RUnlock()

	if km.publicKey == nil {
		return "", fmt.Errorf("public key not initialized")
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(km.publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	pemBytes := pem.EncodeToMemory(publicKeyPEM)
	return string(pemBytes), nil
}

// GetPrivateKeyPEM returns the private key in PEM format
func (km *KeyManager) GetPrivateKeyPEM() (string, error) {
	km.keyMutex.RLock()
	defer km.keyMutex.RUnlock()

	if km.privateKey == nil {
		return "", fmt.Errorf("private key not initialized")
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(km.privateKey)
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	pemBytes := pem.EncodeToMemory(privateKeyPEM)
	return string(pemBytes), nil
}

// ParsePublicKey parses a public key from PEM format
func (km *KeyManager) ParsePublicKey(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid public key type: %s", block.Type)
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return publicKey, nil
}

// Sign signs data with the private key
func (km *KeyManager) Sign(data []byte) ([]byte, error) {
	km.keyMutex.RLock()
	defer km.keyMutex.RUnlock()

	if km.privateKey == nil {
		return nil, fmt.Errorf("private key not initialized")
	}

	// Hash the data
	hash := sha256.Sum256(data)

	// Sign the hash
	signature, err := rsa.SignPKCS1v15(rand.Reader, km.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign data: %w", err)
	}

	return signature, nil
}

// Verify verifies a signature using a public key
func (km *KeyManager) Verify(data, signature []byte, publicKey *rsa.PublicKey) error {
	// Hash the data
	hash := sha256.Sum256(data)

	// Verify the signature
	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// RotateKeys generates new key pair and replaces the current ones
func (km *KeyManager) RotateKeys() error {
	km.logger.Info("Rotating federation keys")

	privateKeyPath := km.config.PrivateKeyPath
	publicKeyPath := km.config.PublicKeyPath

	// Backup current keys
	backupPrivatePath := privateKeyPath + ".backup"
	backupPublicPath := publicKeyPath + ".backup"

	// Copy current keys to backup
	if err := copyFile(privateKeyPath, backupPrivatePath); err != nil {
		return fmt.Errorf("failed to backup private key: %w", err)
	}

	if err := copyFile(publicKeyPath, backupPublicPath); err != nil {
		return fmt.Errorf("failed to backup public key: %w", err)
	}

	// Generate new key pair
	if err := km.generateKeyPair(privateKeyPath, publicKeyPath); err != nil {
		// Restore backup on failure
		copyFile(backupPrivatePath, privateKeyPath)
		copyFile(backupPublicPath, publicKeyPath)
		return fmt.Errorf("failed to generate new key pair: %w", err)
	}

	// Load new keys
	if err := km.loadKeys(privateKeyPath, publicKeyPath); err != nil {
		// Restore backup on failure
		copyFile(backupPrivatePath, privateKeyPath)
		copyFile(backupPublicPath, publicKeyPath)
		km.loadKeys(privateKeyPath, publicKeyPath) // Load backup keys
		return fmt.Errorf("failed to load new keys: %w", err)
	}

	// Remove backup files
	os.Remove(backupPrivatePath)
	os.Remove(backupPublicPath)

	km.logger.Info("Federation keys rotated successfully")
	return nil
}

// ValidateKeyPair validates that the private and public keys match
func (km *KeyManager) ValidateKeyPair() error {
	km.keyMutex.RLock()
	defer km.keyMutex.RUnlock()

	if km.privateKey == nil || km.publicKey == nil {
		return fmt.Errorf("keys not initialized")
	}

	// Test data
	testData := []byte("federation key validation test")

	// Sign with private key
	signature, err := km.Sign(testData)
	if err != nil {
		return fmt.Errorf("failed to sign test data: %w", err)
	}

	// Verify with public key
	if err := km.Verify(testData, signature, km.publicKey); err != nil {
		return fmt.Errorf("key pair validation failed: %w", err)
	}

	return nil
}

// GetKeyFingerprint returns a fingerprint of the public key
func (km *KeyManager) GetKeyFingerprint() (string, error) {
	publicKeyPEM, err := km.GetPublicKeyPEM()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(publicKeyPEM))
	return fmt.Sprintf("%x", hash), nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dst, data, 0600)
}