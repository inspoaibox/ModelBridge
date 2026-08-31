package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
)

type SMTPPasswordResetNotifier struct {
	address string
	from    string
	host    string
	auth    smtp.Auth
	baseURL string
}

func NewSMTPPasswordResetNotifier(address, from, username, password, baseURL string) (*SMTPPasswordResetNotifier, error) {
	address = strings.TrimSpace(address)
	from = strings.TrimSpace(from)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if address == "" || from == "" || baseURL == "" {
		return nil, errors.New("smtp address, sender, and public base URL are required")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, errors.New("smtp address must include host and port")
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return nil, errors.New("smtp sender must be a valid email address")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("public base URL must be an HTTPS URL")
	}
	username = strings.TrimSpace(username)
	if (username == "") != (password == "") {
		return nil, errors.New("smtp username and password must be configured together")
	}
	host, _, _ := net.SplitHostPort(address)
	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	return &SMTPPasswordResetNotifier{address: address, from: fromAddress.Address, host: host, auth: auth, baseURL: baseURL}, nil
}

func (n *SMTPPasswordResetNotifier) SendPasswordReset(ctx context.Context, email, token string) error {
	if n == nil || strings.TrimSpace(email) == "" || strings.TrimSpace(token) == "" {
		return errors.New("password reset notifier is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	recipient, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return err
	}
	link := n.baseURL + "/#reset?token=" + url.QueryEscape(token)
	subject := "AI Token Gateway password reset"
	body := "Use this link to reset your password. It expires in 30 minutes.\r\n\r\n" + link + "\r\n"
	message := []byte("To: " + recipient.Address + "\r\nFrom: " + n.from + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", n.address)
	if err != nil {
		return err
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, n.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("smtp server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: n.host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if n.auth != nil {
		if err := client.Auth(n.auth); err != nil {
			return err
		}
	}
	if err := client.Mail(n.from); err != nil {
		return err
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

var _ PasswordResetNotifier = (*SMTPPasswordResetNotifier)(nil)
