package main

import (
	"bufio"
	"fmt"
	"github.com/a696385/go-meter/http"
	"net"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"
)

type Connection struct {
	conn    net.Conn
	manager *ConnectionManager

	queue chan *http.Request

	responses chan *RequestStats
}

type ConnectionManager struct {
	conns  []*Connection
	config *Config
	C      chan *Connection
}

func NewConnectionManager(config *Config) (result *ConnectionManager) {

	result = &ConnectionManager{
		config: config,
		conns:  make([]*Connection, config.Connections),
		C:      make(chan *Connection, config.Connections),
	}
	for i := 0; i < config.Connections; i++ {
		connection := &Connection{
			manager:   result,
			queue:     make(chan *http.Request, 120),
			responses: config.RequestStats,
		}
		result.conns[i] = connection
		if err := connection.Dial(); err != nil {
			atomic.AddInt32(&ConnectionErrors, 1)
			fmt.Printf("ERROR: %s\n", err.Error())
		} else {
			connection.Return()
		}
	}
	return
}

func (this *Connection) Dial() error {
	if this.IsConnected() {
		return nil
	}
	host := this.manager.config.Url.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	conn, err := net.Dial("tcp4", host)
	if err == nil {
		this.conn = conn
		bf := bufio.NewReader(conn)
		tp := textproto.NewReader(bf)

		//Response resiver
		go func(this *Connection) {
			for {
				req := <-this.queue
				n := time.Now()
				t, res, err := http.ReadResponse(bf, tp)
				duration := t.Sub(n)
				if err != nil {
					atomic.AddInt32(&ReadErrors, 1)
					continue
				} else {
					res.Request = req
				}
				result := &RequestStats{}
				result.Duration = duration
				result.NetOut = res.Request.BufferSize
				result.NetIn = res.BufferSize
				result.ResponseCode = res.StatusCode
				res.Request.Body = nil
				this.responses <- result
			}
		}(this)
	}
	return err
}

func (this *Connection) IsConnected() bool {
	return this.conn != nil
}

func (this *Connection) Take() {

}

func (this *Connection) Return() {
	this.manager.C <- this
}

func (this *Connection) Exec(req *http.Request, resp chan *RequestStats) {
	this.queue <- req
	err := req.Write(this.conn)
	if err != nil {
		atomic.AddInt32(&WriteErrors, 1)
	} else {
		this.Return()
	}
}
