FROM golang:1.15.7

ADD . /
RUN go build /tendermintClient.go
ENTRYPOINT [ "/tendermintClient" ]