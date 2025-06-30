package email

import (
	"time"

	"github.com/knadh/go-pop3"
)

func InitPop3(serverAddr, username, password string, timeout time.Duration) (*pop3.Conn, error) {

	p := pop3.New(pop3.Opt{
		Host:        serverAddr,
		Port:        995,
		TLSEnabled:  true,
		DialTimeout: timeout,
	})

	c, err := p.NewConn()
	if err != nil {
		// log.Println("Connection error:", err)
		return nil, err
	}

	// Аутентификация

	if err := c.Auth(username, password); err != nil {
		// log.Println("Auth error:", err)
		return nil, err
	}

	return c, nil

}
