package utils

import (
	"crypto/tls"
	"math/rand"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
}

var acceptLangs = []string{"en-US,en;q=0.9", "en-GB,en;q=0.9", "fr-FR,fr;q=0.9", "de-DE,de;q=0.9"}

type Fingerprint struct {
	UserAgent      string
	Accept         string
	AcceptLanguage string
	AcceptEncoding string
	SecChUA        string
	ViewportWidth  string
	DPR            string
	DeviceMemory   string
	Referer        string
}

func RandomFingerprint() Fingerprint {
	return Fingerprint{
		UserAgent:      userAgents[rand.Intn(len(userAgents))],
		Accept:         "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		AcceptLanguage: acceptLangs[rand.Intn(len(acceptLangs))],
		AcceptEncoding: "gzip, deflate, br",
		SecChUA:        `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
		ViewportWidth:  "1920",
		DPR:            "1",
		DeviceMemory:   "8",
		Referer:        "https://www.google.com/",
	}
}

func RandomTLSConfig() *tls.Config {
	ciphers := []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	}
	return &tls.Config{
		CipherSuites: []uint16{ciphers[rand.Intn(len(ciphers))]},
		MinVersion:   tls.VersionTLS12,
	}
}
