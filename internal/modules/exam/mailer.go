package exam

import (
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"
)

type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

type Mailer struct {
	cfg SMTPConfig
}

func NewMailer(cfg SMTPConfig) *Mailer {
	return &Mailer{cfg: cfg}
}

// Enabled indica si hay credenciales SMTP suficientes para enviar.
func (m *Mailer) Enabled() bool {
	return m.cfg.Host != "" && m.cfg.Port != "" && m.cfg.User != "" && m.cfg.Pass != "" && m.cfg.From != ""
}

// SendHTML envía un correo HTML a un único destinatario.
func (m *Mailer) SendHTML(to, subject, htmlBody string) error {
	if !m.Enabled() {
		return fmt.Errorf("SMTP no configurado")
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("destinatario vacío")
	}

	addr := m.cfg.Host + ":" + m.cfg.Port
	auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)

	var b strings.Builder
	b.WriteString("From: " + m.cfg.From + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + encodeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)

	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, []byte(b.String()))
}

// encodeHeader codifica el asunto en MIME (RFC 2047) para soportar acentos.
func encodeHeader(s string) string {
	if isASCII(s) {
		return s
	}
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
