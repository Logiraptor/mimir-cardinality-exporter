FROM golang:bullseye

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /mimir-cardinality-exporter

ENTRYPOINT [ "/mimir-cardinality-exporter" ]