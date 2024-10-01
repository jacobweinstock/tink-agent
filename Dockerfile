FROM golang:1.23.0-alpine3.20 AS builder

WORKDIR /code
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o tink-worker

FROM scratch
COPY --from=builder /code/tink-worker /tink-worker

ENTRYPOINT ["/tink-worker"]