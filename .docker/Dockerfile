# syntax=docker/dockerfile:1

FROM golang:1.16-alpine

WORKDIR /app
COPY . /app

RUN go build -o /app/bin/taskmaster src/main.go
RUN go build -o /app/bin/tests test/main.go

RUN rm -rvf /app/src
RUN rm -rvf /app/test
