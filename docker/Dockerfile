FROM golang:1.22-bookworm AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
		git gcc ca-certificates
		
WORKDIR /go/src/github.com/elastic/machinebeat

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN go build 
RUN chmod go-w /go/src/github.com/elastic/machinebeat/machinebeat.yml
RUN chmod go-w /go/src/github.com/elastic/machinebeat/modules.d/opcua.yml

CMD [ "machinebeat -e" ]
