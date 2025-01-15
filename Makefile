.PHONY: build run test clean

build:
	go build -ldflags "-s -w" -o dhcpd cmd/dhcpd/main.go

run: build
	./dhcp

test:
	go test -v ./tests/

clean:
	rm -rf dhcp