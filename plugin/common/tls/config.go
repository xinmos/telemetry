package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

const TLSMinVersionDefault = tls.VersionTLS12

// ClientConfig represents the standard client TLS config.
type ClientConfig struct {
	TLSCA              string `json:"tls_ca"`
	TLSCert            string `json:"tls_cert"`
	TLSKey             string `json:"tls_key"`
	TLSKeyPwd          string `json:"tls_key_pwd"`
	TLSMinVersion      string `json:"tls_min_version"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	ServerName         string `json:"tls_server_name"`
}

// ServerConfig represents the standard server TLS config.
type ServerConfig struct {
	TLSCert            string   `json:"tls_cert"`
	TLSKey             string   `json:"tls_key"`
	TLSKeyPwd          string   `json:"tls_key_pwd"`
	TLSAllowedCACerts  []string `json:"tls_allowed_cacerts"`
	TLSCipherSuites    []string `json:"tls_cipher_suites"`
	TLSMinVersion      string   `json:"tls_min_version"`
	TLSMaxVersion      string   `json:"tls_max_version"`
	TLSAllowedDNSNames []string `json:"tls_allowed_dns_names"`
}

func (c *ClientConfig) TLSConfig() (*tls.Config, error) {
	if c.TLSCA == "" && c.TLSKey == "" && c.TLSCert == "" && !c.InsecureSkipVerify && c.ServerName == "" {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		Renegotiation:      tls.RenegotiateNever,
	}

	if c.TLSCA != "" {
		pool, err := makeCertPool([]string{c.TLSCA})
		if err != nil {
			return nil, err
		}
		tlsConfig.ClientCAs = pool
	}

	if c.TLSCert != "" && c.TLSKey != "" {
		err := loadCertificate(tlsConfig, c.TLSCert, c.TLSKey)
		if err != nil {
			return nil, err
		}
	}

	// Explicitly and consistently set the minimal accepted version using the
	// defined default. We use this setting for both clients and servers
	// instead of relying on Golang's default that is different for clients
	// and servers and might change over time.
	tlsConfig.MinVersion = TLSMinVersionDefault
	if c.TLSMinVersion != "" {
		version, err := ParseTLSVersion(c.TLSMinVersion)
		if err != nil {
			return nil, fmt.Errorf("could not parse tls min version %q: %w", c.TLSMinVersion, err)
		}
		tlsConfig.MinVersion = version
	}

	if c.ServerName != "" {
		tlsConfig.ServerName = c.ServerName
	}

	return tlsConfig, nil
}

func (s *ServerConfig) TLSConfig() (*tls.Config, error) {
	if s.TLSCert == "" && s.TLSKey == "" && len(s.TLSAllowedCACerts) == 0 {
		return nil, nil
	}

	tlsConfig := &tls.Config{}

	if len(s.TLSAllowedCACerts) != 0 {
		pool, err := makeCertPool(s.TLSAllowedCACerts)
		if err != nil {
			return nil, err
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if s.TLSCert != "" && s.TLSKey != "" {
		err := loadCertificate(tlsConfig, s.TLSCert, s.TLSKey)
		if err != nil {
			return nil, err
		}
	}

	if len(s.TLSCipherSuites) != 0 {
		cipherSuites, err := ParseCiphers(s.TLSCipherSuites)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse server cipher suites %s: %v", strings.Join(s.TLSCipherSuites, ","), err)
		}
		tlsConfig.CipherSuites = cipherSuites
	}

	if s.TLSMaxVersion != "" {
		version, err := ParseTLSVersion(s.TLSMaxVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse tls max version %q: %v", s.TLSMaxVersion, err)
		}
		tlsConfig.MaxVersion = version
	}

	// Explicitly and consistently set the minimal accepted version using the
	// defined default. We use this setting for both clients and servers
	// instead of relying on Golang's default that is different for clients
	// and servers and might change over time.
	tlsConfig.MinVersion = TLSMinVersionDefault
	if s.TLSMinVersion != "" {
		version, err := ParseTLSVersion(s.TLSMinVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse tls min version %q: %v", s.TLSMinVersion, err)
		}
		tlsConfig.MinVersion = version
	}

	if tlsConfig.MinVersion != 0 && tlsConfig.MaxVersion != 0 && tlsConfig.MinVersion > tlsConfig.MaxVersion {
		return nil, fmt.Errorf(
			"tls min version %q can't be greater than tls max version %q", tlsConfig.MinVersion, tlsConfig.MaxVersion)
	}

	// Since clientAuth is tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	// there must be certs to validate.
	if len(s.TLSAllowedCACerts) > 0 && len(s.TLSAllowedDNSNames) > 0 {
		tlsConfig.VerifyPeerCertificate = s.verifyPeerCertificate
	}

	return tlsConfig, nil
}

func loadCertificate(config *tls.Config, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf(
			"could not load keypair %s:%s: %v", certFile, keyFile, err)
	}

	config.Certificates = []tls.Certificate{cert}
	return nil
}

func makeCertPool(certFiles []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, certFile := range certFiles {
		pem, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("could not read certificate %q: %v", certFile, err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("could not parse any PEM certificates %q: %v", certFile, err)
		}
	}
	return pool, nil
}

func (s *ServerConfig) verifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	// The certificate chain is client + intermediate + root.
	// Let's review the client certificate.
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("could not validate peer certificate: %v", err)
	}

	for _, name := range cert.DNSNames {
		if contains(name, s.TLSAllowedDNSNames) {
			return nil
		}
	}

	return fmt.Errorf("peer certificate not in allowed DNS Name list: %v", cert.DNSNames)
}

func contains(choice string, choices []string) bool {
	for _, item := range choices {
		if item == choice {
			return true
		}
	}
	return false
}
