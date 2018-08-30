# Mumblet

A web-based Mumble client (listen only). Just a fun experiment i.e. very alpha!!

It uses websockets for signalling & WebRTC for audio.

## Usage

```
$ go get github.com/porjo/mumblet
$ ./mumblet -webRoot $GOPATH/src/github.com/porjo/mumblet/html -port 8080
```

Point your web browser at http://localhost:8080/

Join a Mumble Server, and wait for somebody to speak :)
