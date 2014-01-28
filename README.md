Goloris - slowloris[1] for nginx. Written in Go
===============================================

FAQ
---

 Q: How it works?
 A: It tries occupying and keeping busy as much tcp connections
    to the victim as possible by using as low network bandwidth as possible.
    If goloris is lucky enough, then eventually it should occupy all available
    connections to the victim's TCP address, so no other client could connect
    to it.
    See the source code for more insights.

 Q: How quickly it can take down unprotected nginx with default config?
 A: In a few minutes with default options.

 Q: How to protect nginx against goloris?
 A: I know the following options:
    - Limit the number of simultaneous TCP connections from the same
      source ip. See, for example, connlimit in iptables
      or http://nginx.org/en/docs/http/ngx_http_limit_conn_module.html
    - Deny POST requests.
    - Patch nginx, so it drops connection if the client sends POST
      body at very slow rate.

 Q: How to use it?
 A: go get -u -a github.com/valyala/goloris
    go build github.com/valyala/goloris
    ./goloris -help

P.S. Don't forget adjusting `ulimit -n` before experimenting.

And remember - goloris is published for educational purposes only.

[1] http://ha.ckers.org/slowloris/
