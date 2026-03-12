package backend

import (
	"fmt"
	"net/smtp"
)

func (a *App) sendResetEmail(to, link string) error {
	cfg := a.configSnapshot()
	if cfg.SMTP.Host == "" || cfg.SMTP.From == "" {
		a.logger.Printf("password reset link for %s: %s", to, link)
		return nil
	}

	auth := smtp.PlainAuth("", cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.Host)
	message := []byte(fmt.Sprintf("To: %s\r\nSubject: OpenClaw Deploy 密码重置\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n请访问以下链接重置密码：\r\n%s\r\n", to, link))
	address := fmt.Sprintf("%s:%d", cfg.SMTP.Host, cfg.SMTP.Port)
	return smtp.SendMail(address, auth, cfg.SMTP.From, []string{to}, message)
}
