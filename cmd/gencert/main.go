package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("out-dir", "cert", "输出目录")
	days := flag.Int("days", 3650, "服务器证书有效期（天）")
	force := flag.Bool("force", false, "已存在也重新生成")
	flag.Parse()

	caPath := filepath.Join(*dir, "ca.crt")
	srvPath := filepath.Join(*dir, "server.pem")
	if !*force {
		if _, err := os.Stat(caPath); err == nil {
			if _, err := os.Stat(srvPath); err == nil {
				fmt.Printf("已存在，跳过：%s / %s（要重新生成加 -force）\n", caPath, srvPath)
				return
			}
		}
	}

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	caSerial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	caTmpl := x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "FO3 Launcher Local CA", Organization: []string{"FO3 Launcher"}},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().AddDate(20, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTmpl, &caTmpl, &caKey.PublicKey, caKey)
	must(err)
	caCert, err := x509.ParseCertificate(caDER)
	must(err)

	srvKey, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	srvSerial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	srvTmpl := x509.Certificate{
		SerialNumber: srvSerial,
		Subject:      pkix.Name{CommonName: "APSEODESK172", Organization: []string{"FO3 Launcher"}},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, *days),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"APSEODESK172", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, &srvTmpl, caCert, &srvKey.PublicKey, caKey)
	must(err)

	must(os.MkdirAll(*dir, 0o755))
	writePEM(caPath, []pem.Block{{Type: "CERTIFICATE", Bytes: caDER}})
	writePEM(srvPath, []pem.Block{
		{Type: "CERTIFICATE", Bytes: srvDER},
		{Type: "CERTIFICATE", Bytes: caDER},
		{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(srvKey)},
	})

	must(os.WriteFile(filepath.Join(*dir, "ca.cer"), caDER, 0o644))

	fmt.Printf("已生成：\n  %s\n  %s（CN=APSEODESK172 + SAN APSEODESK172/localhost/127.0.0.1）\n  %s\n",
		caPath, srvPath, filepath.Join(*dir, "ca.cer"))
}

func writePEM(path string, blocks []pem.Block) {
	f, err := os.Create(path)
	must(err)
	defer f.Close()
	for i := range blocks {
		must(pem.Encode(f, &blocks[i]))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
