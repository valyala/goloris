Goloris - slowloris[1] for nginx DoS
===============================================

## FAQ

* **Features**

  - Uses as low network bandwidth as possible.
  - Low CPU and memory usage.
  - Automatically and silently eats all the available TCP connections
    to the server.
  - Supports https.
  - Easily hackable thanks to clear and concise Go syntax
    and powerful Golang features.


* **Limitations**

  - Can eat up to 64K TCP connections from a single IP due to TCP limitations.
    Just use proxies if you want overcoming this limitation :)


* **How it works?**

  It tries occupying and keeping busy as much tcp connections
  to the victim as possible by using as low network bandwidth as possible.
  If goloris is lucky enough, then eventually it should eat all the available
  connections to the victim, so no other client could connect to it.
  See the source code for more insights.


* **How quickly it can take down unprotected nginx with default settings?**

  In a few minutes with default config options.


* **How to protect nginx against goloris?**

  I know the following options:
  - Limit the number of simultaneous TCP connections from the same
    source ip. See, for example, connlimit in iptables
    or http://nginx.org/en/docs/http/ngx_http_limit_conn_module.html
  - Deny POST requests.
  - Patch nginx, so it drops connection if the client sends POST
    body at very slow rate.


* **How to use it?**

  ```
go get -u -a github.com/valyala/goloris
go build github.com/valyala/goloris
./goloris -help
  ```

P.S. Don't forget adjusting `ulimit -n` before experimenting.

And remember - goloris is published for educational purposes only.

[1] http://ha.ckers.org/slowloris/
