
all: assets.go
	go install

assets.go: go-bindata
	go-bindata -prefix "`pwd`/web-frontend" -nocompress -nomemcopy -o assets.go web-frontend/...

go-bindata:
	go get github.com/jteeuwen/go-bindata/...
