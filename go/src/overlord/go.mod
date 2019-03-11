module overlord

go 1.11

require (
	github.com/googollee/go-socket.io v1.0.1
	github.com/gorilla/mux v1.7.0
	github.com/gorilla/websocket v1.4.0
	github.com/kr/pretty v0.1.0 // indirect
	github.com/kr/pty v1.1.3
	github.com/pkg/term v0.0.0-20190109203006-aa71e9d9e942
	github.com/satori/go.uuid v1.2.0
	golang.org/x/crypto v0.0.0-20190308221718-c2843e01d9a2
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)

replace github.com/satori/go.uuid v1.2.0 => github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b
