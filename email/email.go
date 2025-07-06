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
		return nil, err
	}
	if err := c.Auth(username, password); err != nil {
		return nil, err
	}

	return c, nil

}
